package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CompileMap runs ericw-tools (qbsp, vis, light) on a .map file to produce a
// .bsp. It fails on invalid brushes and leaks rather than producing a broken
// map. NOTE: never pass -bounce to light — the vendored build silently writes
// zero lightdata (fullbright map).
func CompileMap(mapPath string, outputDir string) (string, error) {
	mapPath, _ = filepath.Abs(mapPath)
	outputDir, _ = filepath.Abs(outputDir)

	bspPath := filepath.Join(outputDir, strings.TrimSuffix(filepath.Base(mapPath), ".map")+".bsp")

	qbsp, err := findTool("qbsp")
	if err != nil {
		return "", err
	}
	vis, err := findTool("vis")
	if err != nil {
		return "", err
	}
	light, err := findTool("light")
	if err != nil {
		return "", err
	}

	env := append(os.Environ(),
		"LD_LIBRARY_PATH="+filepath.Join(vendorDir(), "bin")+":"+os.Getenv("LD_LIBRARY_PATH"))

	run := func(name, tool string, args ...string) (string, error) {
		cmd := exec.Command(tool, args...)
		cmd.Dir = outputDir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			return string(out), fmt.Errorf("%s failed: %w\n%s", name, err, tail(string(out), 15))
		}
		return string(out), nil
	}

	// 1. qbsp — then check for invalid brushes and leaks.
	ptsPath := strings.TrimSuffix(bspPath, ".bsp") + ".pts"
	os.Remove(ptsPath)
	out, err := run("qbsp", qbsp, "-splitturb", "-splitspecial", mapPath)
	if err != nil {
		return "", err
	}
	if n := strings.Count(out, "WARNING 09"); n > 0 {
		return "", fmt.Errorf("qbsp: %d invalid brushes (WARNING 09: couldn't create brush faces) — check plane winding", n)
	}
	if strings.Contains(out, "LEAK") {
		return "", fmt.Errorf("qbsp: map leaks — pointfile at %s", ptsPath)
	}
	if _, err := os.Stat(ptsPath); err == nil {
		return "", fmt.Errorf("map leaks — pointfile at %s", ptsPath)
	}

	// 2. vis (requires the .prt qbsp writes for sealed maps).
	prtPath := strings.TrimSuffix(bspPath, ".bsp") + ".prt"
	if _, err := os.Stat(prtPath); err != nil {
		return "", fmt.Errorf("no portal file %s — map not sealed", prtPath)
	}
	if _, err := run("vis", vis, bspPath); err != nil {
		return "", err
	}

	// 3. light.
	if _, err := run("light", light, "-surflight_subdivide", "64", bspPath); err != nil {
		return "", err
	}
	if size, err := lumpSize(bspPath, LumpLighting); err != nil {
		return "", err
	} else if size == 0 {
		return "", fmt.Errorf("light produced zero lightdata (map would be fullbright)")
	}

	fmt.Printf("compiled %s\n", bspPath)
	return bspPath, nil
}

func lumpSize(bspPath string, lump int) (int, error) {
	f, err := os.Open(bspPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	hdr := make([]byte, 4+NumLumps*8)
	if _, err := f.ReadAt(hdr, 0); err != nil {
		return 0, err
	}
	off := 8 + lump*8
	return int(uint32(hdr[off]) | uint32(hdr[off+1])<<8 | uint32(hdr[off+2])<<16 | uint32(hdr[off+3])<<24), nil
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func findTool(name string) (string, error) {
	vendored := filepath.Join(vendorDir(), "bin", name)
	if info, err := os.Stat(vendored); err == nil && !info.IsDir() {
		return vendored, nil
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not found: install ericw-tools or place in vendor/ericw-tools/bin/", name)
	}
	return path, nil
}

func vendorDir() string {
	exe, err := os.Executable()
	if err == nil {
		v := filepath.Join(filepath.Dir(exe), "..", "vendor", "ericw-tools")
		if info, err := os.Stat(v); err == nil && info.IsDir() {
			return v
		}
	}
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
