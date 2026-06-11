package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CompileMap runs ericw-tools (qbsp, vis, light) on a .map file to produce a .bsp.
// Tools are found in vendor/ericw-tools/bin/ first, then PATH.
func CompileMap(mapPath string, outputDir string) (string, error) {
	mapPath, _ = filepath.Abs(mapPath)
	outputDir, _ = filepath.Abs(outputDir)

	bspPath := strings.TrimSuffix(mapPath, ".map") + ".bsp"
	if outputDir != filepath.Dir(mapPath) {
		bspPath = filepath.Join(outputDir, filepath.Base(strings.TrimSuffix(mapPath, ".map")+".bsp"))
	}

	qbsp, err := findTool("qbsp")
	if err != nil {
		return "", err
	}
	vis, _ := findTool("vis")   // vis is optional
	light, _ := findTool("light") // light is optional (may need embree)

	// Set LD_LIBRARY_PATH for vendored shared libs.
	env := os.Environ()
	vendorBin := filepath.Join(vendorDir(), "bin")
	env = append(env, "LD_LIBRARY_PATH="+vendorBin+":"+os.Getenv("LD_LIBRARY_PATH"))

	run := func(name, tool string, args ...string) error {
		cmd := exec.Command(tool, args...)
		cmd.Dir = outputDir
		cmd.Env = env
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		fmt.Printf("  %s %s\n", name, strings.Join(args, " "))
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s failed: %w", name, err)
		}
		return nil
	}

	// 1. qbsp
	if err := run("qbsp", qbsp, "-splitturb", "-splitspecial", mapPath); err != nil {
		return "", err
	}

	// 2. vis (optional — skip if no .prt file)
	prtPath := strings.TrimSuffix(bspPath, ".bsp") + ".prt"
	if vis != "" {
		if _, err := os.Stat(prtPath); err == nil {
			if err := run("vis", vis, bspPath); err != nil {
				fmt.Fprintf(os.Stderr, "warning: vis failed: %v\n", err)
			}
		}
	}

	// 3. light (optional — may fail without embree on some platforms)
	if light != "" {
		if err := run("light", light, "-surflight_subdivide", "64", bspPath); err != nil {
			fmt.Fprintf(os.Stderr, "warning: light failed (map will be fullbright): %v\n", err)
		}
	}

	return bspPath, nil
}

func findTool(name string) (string, error) {
	// Check vendored location first.
	vendored := filepath.Join(vendorDir(), "bin", name)
	if info, err := os.Stat(vendored); err == nil && !info.IsDir() {
		return vendored, nil
	}

	// Fall back to PATH.
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not found: install ericw-tools or place in vendor/ericw-tools/bin/", name)
	}
	return path, nil
}

func vendorDir() string {
	// Look relative to the executable, then relative to cwd.
	exe, err := os.Executable()
	if err == nil {
		v := filepath.Join(filepath.Dir(exe), "..", "vendor", "ericw-tools")
		if info, err := os.Stat(v); err == nil && info.IsDir() {
			return v
		}
	}

	// Try relative to source tree.
	cwd, _ := os.Getwd()
	for _, rel := range []string{
		"../../vendor/ericw-tools",
		"vendor/ericw-tools",
	} {
		v := filepath.Join(cwd, rel)
		if info, err := os.Stat(v); err == nil && info.IsDir() {
			return v
		}
	}

	return filepath.Join(cwd, "vendor", "ericw-tools")
}
