package main

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	blueprint := flag.String("blueprint", "", "Path to blueprint JSON file")
	shortBP := flag.String("b", "", "Path to blueprint JSON file (shorthand)")
	seed := flag.Uint64("seed", 0, "RNG seed (random if 0)")
	output := flag.String("o", "", "Output path (.map or .bsp)")
	outputDir := flag.String("output-dir", ".", "Output directory")
	bspMode := flag.Bool("bsp", false, "Output .bsp directly (no external tools needed)")
	compileMode := flag.Bool("compile", false, "Output .map then compile with qbsp/vis/light (ericw-tools)")
	extractBP := flag.String("extract-blueprints", "", "Extract shareware maps to blueprint JSON files in the given directory")

	// Procgen options.
	rooms := flag.Int("rooms", 3, "Max BSP subdivision depth (procgen mode)")
	arenaSize := flag.Int("arena-size", 3072, "Arena side length in Quake units (procgen mode)")

	flag.Parse()

	if *extractBP != "" {
		if _, err := EnsureQuakeTextures(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if err := ExtractSharewareBlueprints(*extractBP); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	s := *seed
	if s == 0 {
		s = rand.Uint64()
	}
	rng := rand.New(rand.NewPCG(s, 0))

	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	bp := *blueprint
	if bp == "" {
		bp = *shortBP
	}

	var m *MapFile
	var slug string

	if bp != "" {
		// Blueprint mode.
		resolved, err := LoadBlueprint(bp)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("blueprint: %s  rooms=%d  connections=%d  theme=%s\n",
			resolved.Name, len(resolved.Rooms), len(resolved.Connections), resolved.Theme)

		result := CompileBlueprint(resolved, rng)

		m = NewMapFile()
		m.Worldspawn.Properties["message"] = resolved.Name

		th := GetTheme(resolved.Theme)

		// If style references a source map, override the theme with its palette.
		if resolved.Style != "" {
			if lib, err := LoadChunkLibrary(); err == nil {
				if ep, ok := lib.MapPalettes[resolved.Style]; ok {
					pal := PaletteFromExtracted(ep)
					fmt.Printf("style: %s (wall=%s floor=%s ceil=%s)\n", resolved.Style, pal.Wall, pal.Floor, pal.Ceiling)
					// Override the default textures so all geometry uses this palette.
					Textures.Floor = pal.Floor
					Textures.Ceiling = pal.Ceiling
					Textures.Shell = pal.Trim
					Textures.Fill = pal.Wall
				}
			}
		}
		var roomEnvs []string
		var roomDetails [][]Detail
		for _, br := range resolved.Rooms {
			idx, ok := result.IDToIdx[br.ID]
			if !ok {
				continue
			}
			// Grow slices to fit.
			for len(roomEnvs) <= idx {
				roomEnvs = append(roomEnvs, "building")
			}
			for len(roomDetails) <= idx {
				roomDetails = append(roomDetails, nil)
			}
			roomEnvs[idx] = br.Environment
			roomDetails[idx] = br.Details
		}

		BuildBlueprintGeometryThemed(m, result.Layout, result.Grid, th, roomEnvs, roomDetails)
		PopulateFromBlueprint(m, result.Layout, resolved, result.IDToIdx, result.TeleConns, rng)

		slug = strings.ReplaceAll(strings.ToLower(resolved.Name), " ", "_")
		slug = strings.ReplaceAll(slug, "'", "")
	} else {
		// Procgen mode.
		fmt.Printf("seed=%d  arena=%d  depth=%d\n", s, *arenaSize, *rooms)
		m, _ = RunProcgen(rng, *arenaSize, *rooms)
		slug = fmt.Sprintf("gen_%d", s)
	}

	if *bspMode {
		bspPath := filepath.Join(*outputDir, slug+".bsp")
		if *output != "" {
			bspPath = *output
		}

		if _, err := EnsureQuakeTextures(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: real textures unavailable: %v\n", err)
		}

		bspData := BuildBSP(m)
		f, err := os.Create(bspPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if err := WriteBSP(f, bspData); err != nil {
			f.Close()
			fmt.Fprintf(os.Stderr, "error writing BSP: %v\n", err)
			os.Exit(1)
		}
		f.Close()
		fmt.Printf("wrote %s  (seed=%d, %d faces, %d nodes, %d leaves)\n",
			bspPath, s, len(bspData.Faces), len(bspData.Nodes), len(bspData.Leaves))
	} else {
		mapPath := filepath.Join(*outputDir, slug+".map")
		if *output != "" {
			mapPath = *output
			if *compileMode && !strings.HasSuffix(mapPath, ".map") {
				mapPath = strings.TrimSuffix(mapPath, ".bsp") + ".map"
			}
		}

		if _, err := MaterializeTextureWAD(*outputDir); err != nil {
			fmt.Fprintf(os.Stderr, "warning: texture WAD: %v\n", err)
		}

		f, err := os.Create(mapPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		m.Write(f)
		f.Close()
		fmt.Printf("wrote %s  (seed=%d)\n", mapPath, s)

		if *compileMode {
			bspPath, err := CompileMap(mapPath, *outputDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: compile: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("compiled %s\n", bspPath)
		}
	}
}
