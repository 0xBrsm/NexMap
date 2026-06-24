package main

// navcheck subcommand: build a Recast/Detour navmesh from a compiled BSP's
// world geometry and report walkable coverage + reachability of spawns/items.
// Geometry and entity points are extracted here; the actual navmesh build and
// pathfinding happen in the tools/navcheck C++ binary (shared with FrikBotNex).

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func navcheckCmd(args []string) {
	fs := flag.NewFlagSet("navcheck", flag.ExitOnError)
	bin := fs.String("bin", "", "path to navcheck binary (default: <repo>/tools/navcheck/build/navcheck)")
	dump := fs.Bool("dump", false, "write the navcheck input to <bsp>.navin and keep it")
	fs.Parse(args)
	if fs.NArg() < 1 {
		fatal(fmt.Errorf("usage: nexmap navcheck <file.bsp> [-bin path] [-dump]"))
	}
	bspPath := fs.Arg(0)

	bsp, err := LoadRenderBSP(bspPath)
	if err != nil {
		fatal(err)
	}

	input := buildNavInput(bsp)
	if *dump {
		_ = os.WriteFile(bspPath+".navin", []byte(input), 0644)
	}

	binPath := *bin
	if binPath == "" {
		binPath = defaultNavcheckBin()
	}
	if _, err := os.Stat(binPath); err != nil {
		fatal(fmt.Errorf("navcheck binary not found at %s (build it: cmake -S tools/navcheck -B tools/navcheck/build && cmake --build tools/navcheck/build)", binPath))
	}

	cmd := exec.Command(binPath)
	cmd.Stdin = strings.NewReader(input)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	runErr := cmd.Run()
	if errb.Len() > 0 {
		fmt.Fprint(os.Stderr, errb.String())
	}
	fmt.Print(out.String())
	if runErr != nil {
		os.Exit(1)
	}
}

func defaultNavcheckBin() string {
	// repo root = two levels up from this source's package dir at build time is
	// not knowable at runtime; resolve relative to cwd and the executable.
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

// buildNavInput emits the navcheck text format: world triangles (from BSP
// faces, liquids/sky excluded) + query points (spawns and pickups).
func buildNavInput(bsp *RenderBSP) string {
	var b strings.Builder

	// Vertices: reuse the BSP's shared vertex pool directly; faces index into it.
	fmt.Fprintf(&b, "verts %d\n", len(bsp.Vertices))
	for _, v := range bsp.Vertices {
		fmt.Fprintf(&b, "%g %g %g\n", v.X, v.Y, v.Z)
	}

	// Triangles: fan-triangulate world model (model 0) faces. Skip liquid/sky
	// surfaces (texSpecial) so lava/slime/water don't become false floors.
	var tris [][3]int
	if len(bsp.Models) > 0 {
		m := bsp.Models[0]
		for fi := m.FirstFace; fi < m.FirstFace+m.NumFaces; fi++ {
			f := bsp.Faces[fi]
			if int(f.TexinfoID) < len(bsp.Texinfo) && bsp.Texinfo[f.TexinfoID].Flags&texSpecial != 0 {
				continue
			}
			poly := faceVertIndices(bsp, f)
			for i := 2; i < len(poly); i++ {
				tris = append(tris, [3]int{poly[0], poly[i-1], poly[i]})
			}
		}
	}
	fmt.Fprintf(&b, "tris %d\n", len(tris))
	for _, t := range tris {
		fmt.Fprintf(&b, "%d %d %d\n", t[0], t[1], t[2])
	}

	// Query points: spawns and pickups from the entity lump.
	pts := navQueryPoints(bsp.Entities)
	fmt.Fprintf(&b, "points %d\n", len(pts))
	for _, p := range pts {
		fmt.Fprintf(&b, "%s %g %g %g\n", p.label, p.x, p.y, p.z)
	}

	// Off-mesh links: teleporters (entity-based, no collision trace needed).
	links := teleporterLinks(bsp)
	fmt.Fprintf(&b, "links %d\n", len(links))
	for _, l := range links {
		// sx sy sz ex ey ez radius bidir link_type
		fmt.Fprintf(&b, "%g %g %g %g %g %g %g %d %d\n",
			l.sx, l.sy, l.sz, l.ex, l.ey, l.ez, l.radius, l.bidir, l.linkType)
	}
	return b.String()
}

type navOffMeshLink struct {
	sx, sy, sz, ex, ey, ez float64
	radius                 float64
	bidir                  int
	linkType               int
}

const aiTelelink = 1 // AI_TELELINK in nav_mesh.h

// teleporterLinks pairs trigger_teleport brush volumes with their
// info_teleport_destination by target/targetname. Start = trigger brush
// bbox center (xy) at its floor (min z); end = destination origin.
func teleporterLinks(bsp *RenderBSP) []navOffMeshLink {
	type trigger struct {
		target string
		model  int
	}
	var triggers []trigger
	dests := map[string][3]float64{}

	for _, block := range splitEntityBlocks(bsp.Entities) {
		kv := parseEntityBlock(block)
		switch kv["classname"] {
		case "trigger_teleport":
			mdl := kv["model"]
			if !strings.HasPrefix(mdl, "*") {
				continue
			}
			n, err := strconv.Atoi(mdl[1:])
			if err != nil || n < 0 || n >= len(bsp.Models) {
				continue
			}
			triggers = append(triggers, trigger{target: kv["target"], model: n})
		case "info_teleport_destination":
			if o, ok := parseOrigin(kv["origin"]); ok {
				dests[kv["targetname"]] = o
			}
		}
	}

	var links []navOffMeshLink
	for _, t := range triggers {
		dst, ok := dests[t.target]
		if !ok {
			continue
		}
		m := bsp.Models[t.model]
		sx := float64(m.BBoxMin[0]+m.BBoxMax[0]) / 2
		sy := float64(m.BBoxMin[1]+m.BBoxMax[1]) / 2
		sz := float64(m.BBoxMin[2])
		links = append(links, navOffMeshLink{
			sx: sx, sy: sy, sz: sz,
			ex: dst[0], ey: dst[1], ez: dst[2],
			radius: 128, bidir: 0, linkType: aiTelelink,
		})
	}
	return links
}

func parseOrigin(s string) ([3]float64, bool) {
	f := strings.Fields(s)
	if len(f) != 3 {
		return [3]float64{}, false
	}
	var o [3]float64
	for i := 0; i < 3; i++ {
		v, err := strconv.ParseFloat(f[i], 64)
		if err != nil {
			return [3]float64{}, false
		}
		o[i] = v
	}
	return o, true
}

// faceVertIndices resolves a face's ordered polygon as indices into bsp.Vertices.
func faceVertIndices(bsp *RenderBSP, f BSPFace) []int {
	out := make([]int, 0, f.NumEdges)
	for e := f.FirstEdge; e < f.FirstEdge+int32(f.NumEdges); e++ {
		se := bsp.Surfedges[e]
		if se >= 0 {
			out = append(out, int(bsp.Edges[se].V0))
		} else {
			out = append(out, int(bsp.Edges[-se].V1))
		}
	}
	return out
}

type navPoint struct {
	label   string
	x, y, z float64
}

// navQueryPoints parses the entity string for spawn points and pickups.
func navQueryPoints(ents string) []navPoint {
	var pts []navPoint
	counts := map[string]int{}
	for _, block := range splitEntityBlocks(ents) {
		kv := parseEntityBlock(block)
		cls := kv["classname"]
		if !isNavRelevant(cls) {
			continue
		}
		origin, ok := kv["origin"]
		if !ok {
			continue
		}
		fields := strings.Fields(origin)
		if len(fields) != 3 {
			continue
		}
		x, _ := strconv.ParseFloat(fields[0], 64)
		y, _ := strconv.ParseFloat(fields[1], 64)
		z, _ := strconv.ParseFloat(fields[2], 64)
		label := cls
		if strings.HasPrefix(cls, "info_player") {
			label = "spawn"
		}
		counts[label]++
		pts = append(pts, navPoint{label: fmt.Sprintf("%s#%d", label, counts[label]), x: x, y: y, z: z})
	}
	return pts
}

func isNavRelevant(cls string) bool {
	return strings.HasPrefix(cls, "info_player") ||
		strings.HasPrefix(cls, "weapon_") ||
		strings.HasPrefix(cls, "item_") ||
		strings.HasPrefix(cls, "ammo_")
}

func splitEntityBlocks(ents string) []string {
	var blocks []string
	depth := 0
	start := 0
	for i, c := range ents {
		switch c {
		case '{':
			if depth == 0 {
				start = i + 1
			}
			depth++
		case '}':
			depth--
			if depth == 0 {
				blocks = append(blocks, ents[start:i])
			}
		}
	}
	return blocks
}

func parseEntityBlock(block string) map[string]string {
	kv := map[string]string{}
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\"", 5)
		// expected: ["" key "" value ...] -> parts[1]=key, parts[3]=value
		if len(parts) >= 4 {
			kv[parts[1]] = parts[3]
		}
	}
	return kv
}
