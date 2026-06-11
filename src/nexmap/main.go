package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "build":
		buildCmd(os.Args[2:])
	case "render":
		renderCmd(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `usage:
  nexmap build <map-script.py> [-o outdir] [-cams "x y z yaw pitch;..."] [-size WxH] [-gamma G] [-deploy]
      run map script -> qbsp/vis/light -> render screenshots [-> deploy to Pi]
  nexmap render <file.bsp> [-cams ...] [-size WxH] [-gamma G]
      render screenshots of an existing BSP
`)
	os.Exit(2)
}

func buildCmd(args []string) {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	outDir := fs.String("o", "out", "Output directory")
	cams := fs.String("cams", "", "Cameras as 'x y z yaw pitch[;...]' (default: overview + spawns)")
	shotSize := fs.String("size", "960x540", "Screenshot size WxH")
	gamma := fs.Float64("gamma", 1.3, "Screenshot gamma")
	deploy := fs.Bool("deploy", false, "Deploy BSP to the Pi after building")
	deployShare := fs.String("deploy-share", "//10.10.10.10/nqdev", "SMB share for deploy")
	deployDir := fs.String("deploy-dir", "game/id1/common/maps", "Path within the share")
	if len(args) < 1 || strings.HasPrefix(args[0], "-") {
		usage()
	}
	script := args[0]
	fs.Parse(args[1:])

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fatal(err)
	}

	// 1. Texture WAD must exist before qbsp resolves worldspawn "wad".
	if _, err := MaterializeTextureWAD(*outDir); err != nil {
		fatal(fmt.Errorf("texture WAD: %w", err))
	}

	// 2. Run the map script.
	fmt.Printf("== %s\n", script)
	py := exec.Command("python3", script)
	py.Stdout = os.Stdout
	py.Stderr = os.Stderr
	if err := py.Run(); err != nil {
		fatal(fmt.Errorf("map script failed: %w", err))
	}

	base := strings.TrimSuffix(filepath.Base(script), ".py")
	mapPath := filepath.Join(*outDir, base+".map")
	if _, err := os.Stat(mapPath); err != nil {
		fatal(fmt.Errorf("map script did not produce %s", mapPath))
	}

	// 3. Compile (fails loudly on invalid brushes or leaks).
	bspPath, err := CompileMap(mapPath, *outDir)
	if err != nil {
		fatal(err)
	}

	// 4. Render screenshots.
	var sw, sh int
	if _, err := fmt.Sscanf(*shotSize, "%dx%d", &sw, &sh); err != nil {
		fatal(fmt.Errorf("bad -size %q", *shotSize))
	}
	if err := RenderCLI(bspPath, *cams, sw, sh, *gamma); err != nil {
		fatal(fmt.Errorf("render: %w", err))
	}

	// 5. Deploy.
	if *deploy {
		cmd := exec.Command("smbclient", *deployShare, "-N", "-c",
			fmt.Sprintf("cd %s; put %s", *deployDir, filepath.Base(bspPath)))
		cmd.Dir = filepath.Dir(bspPath)
		out, err := cmd.CombinedOutput()
		if err != nil {
			fatal(fmt.Errorf("deploy failed: %w\n%s", err, out))
		}
		fmt.Printf("deployed %s to %s/%s\n", filepath.Base(bspPath), *deployShare, *deployDir)
	}
}

func renderCmd(args []string) {
	fs := flag.NewFlagSet("render", flag.ExitOnError)
	cams := fs.String("cams", "", "Cameras as 'x y z yaw pitch[;...]' (default: overview + spawns)")
	shotSize := fs.String("size", "960x540", "Screenshot size WxH")
	gamma := fs.Float64("gamma", 1.3, "Screenshot gamma")
	if len(args) < 1 || strings.HasPrefix(args[0], "-") {
		usage()
	}
	bspPath := args[0]
	fs.Parse(args[1:])
	var sw, sh int
	if _, err := fmt.Sscanf(*shotSize, "%dx%d", &sw, &sh); err != nil {
		fatal(fmt.Errorf("bad -size %q", *shotSize))
	}
	if err := RenderCLI(bspPath, *cams, sw, sh, *gamma); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
