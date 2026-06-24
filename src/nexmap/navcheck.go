package main

// navcheck subcommand: thin launcher for the tools/navcheck C++ binary, which
// loads the compiled BSP directly (clip-hull geometry via nav_hull, entity
// query points + off-mesh links), builds a Recast/Detour navmesh, and reports
// walkable coverage + spawn/item reachability as JSON. The nav core is shared
// with FrikBotNex's bots, so "reachable" means what the bots experience.

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func navcheckCmd(args []string) {
	fs := flag.NewFlagSet("navcheck", flag.ExitOnError)
	bin := fs.String("bin", "", "path to navcheck binary (default: <repo>/tools/navcheck/build/navcheck)")
	fs.Parse(args)
	if fs.NArg() < 1 {
		fatal(fmt.Errorf("usage: nexmap navcheck <file.bsp> [-bin path]"))
	}
	bspPath := fs.Arg(0)

	binPath := *bin
	if binPath == "" {
		binPath = defaultNavcheckBin()
	}
	if _, err := os.Stat(binPath); err != nil {
		fatal(fmt.Errorf("navcheck binary not found at %s (build it: cmake -S tools/navcheck -B tools/navcheck/build && cmake --build tools/navcheck/build)", binPath))
	}

	cmd := exec.Command(binPath, bspPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
}

func defaultNavcheckBin() string {
	candidates := []string{
		"tools/navcheck/build/navcheck",
		filepath.Join("..", "tools", "navcheck", "build", "navcheck"),
	}
	if exe, err := os.Executable(); err == nil {
		// src/nexmap/nexmap -> repo root is two dirs up
		root := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
		candidates = append([]string{filepath.Join(root, "tools", "navcheck", "build", "navcheck")}, candidates...)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return candidates[0]
}
