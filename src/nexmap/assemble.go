package main

import (
	"fmt"
	"math/rand/v2"
)

// AssembleRoom builds a sealed room from library brushes, scaled and
// positioned to match the target room dimensions. Falls back to plain
// AxisAlignedBox if no suitable library brush is found.
func AssembleRoom(m *MapFile, room *Room, lib *BrushLibrary, style BrushStyle, rng *rand.Rand) {
	x0, y0, x1, y1 := room.X0, room.Y0, room.X1, room.Y1
	z0, z1 := room.Z0, room.Z1
	roomW := float64(x1 - x0)
	roomH := float64(y1 - y0)
	roomD := float64(z1 - z0)

	wallTex, floorTex, ceilTex := pickStyleTextures(lib, style, rng)

	// Floor.
	fb := pickAndPlace(lib, RoleFloor, style, roomW, roomH,
		float64(x0), float64(y0), float64(z0-Floor), float64(x1), float64(y1), float64(z0), rng)
	if fb != nil {
		m.AddBrush(*fb)
	} else {
		m.AddBrush(AxisAlignedBox(x0, y0, z0-Floor, x1, y1, z0, floorTex))
	}

	// Ceiling.
	cb := pickAndPlace(lib, RoleFloor, style, roomW, roomH,
		float64(x0), float64(y0), float64(z1), float64(x1), float64(y1), float64(z1+Floor), rng)
	if cb != nil {
		m.AddBrush(*cb)
	} else {
		m.AddBrush(AxisAlignedBox(x0, y0, z1, x1, y1, z1+Floor, ceilTex))
	}

	// Walls.
	thick := float64(Wall)
	placeWallOrFallback(m, lib, style, rng,
		float64(x0), float64(y1), float64(z0), float64(x1), float64(y1)+thick, float64(z1),
		roomW, roomD, wallTex)
	placeWallOrFallback(m, lib, style, rng,
		float64(x0), float64(y0)-thick, float64(z0), float64(x1), float64(y0), float64(z1),
		roomW, roomD, wallTex)
	placeWallOrFallback(m, lib, style, rng,
		float64(x1), float64(y0), float64(z0), float64(x1)+thick, float64(y1), float64(z1),
		roomH, roomD, wallTex)
	placeWallOrFallback(m, lib, style, rng,
		float64(x0)-thick, float64(y0), float64(z0), float64(x0), float64(y1), float64(z1),
		roomH, roomD, wallTex)
}

func placeWallOrFallback(m *MapFile, lib *BrushLibrary, style BrushStyle, rng *rand.Rand,
	x0, y0, z0, x1, y1, z1 float64, targetW, targetH float64, fallbackTex string) {
	b := pickAndPlace(lib, RoleWall, style, targetW, targetH,
		x0, y0, z0, x1, y1, z1, rng)
	if b != nil {
		m.AddBrush(*b)
	} else {
		m.AddBrush(AxisAlignedBox(int(x0), int(y0), int(z0), int(x1), int(y1), int(z1), fallbackTex))
	}
}

func pickAndPlace(lib *BrushLibrary, role BrushRole, style BrushStyle,
	targetW, targetH float64,
	x0, y0, z0, x1, y1, z1 float64, rng *rand.Rand) *Brush {

	candidates := lib.Query(role, style, targetW*0.3, targetW*3, 0, 0)
	if len(candidates) == 0 {
		candidates = lib.Query(role, StyleUnknown, targetW*0.3, targetW*3, 0, 0)
	}
	if len(candidates) == 0 {
		return nil
	}

	idx := candidates[rng.IntN(min(10, len(candidates)))]
	entry := &lib.Entries[idx]

	srcW := max(entry.Width, entry.Height)
	scale := targetW / srcW
	scale = max(0.5, min(2.0, scale))

	scaled := ScaleBrush(entry.Planes, entry.Bounds, scale)
	scaledBB := BrushBounds(&ParsedBrush{Planes: scaled})

	dx := (x0+x1)/2 - scaledBB.CenterX()
	dy := (y0+y1)/2 - scaledBB.CenterY()
	dz := (z0+z1)/2 - (scaledBB.MinZ+scaledBB.MaxZ)/2

	brushes := TranslateBrushes([]ParsedBrush{{Planes: scaled}}, dx, dy, dz)
	return &brushes[0]
}

// pickStyleTextures returns representative textures for a style.
func pickStyleTextures(lib *BrushLibrary, style BrushStyle, rng *rand.Rand) (wall, floor, ceil string) {
	wallIdx := lib.Query(RoleWall, style, 0, 0, 0, 0)
	floorIdx := lib.Query(RoleFloor, style, 0, 0, 0, 0)

	wall = "metal1_1"
	floor = "metal1_1"
	ceil = "metal1_1"

	if len(wallIdx) > 0 {
		e := &lib.Entries[wallIdx[rng.IntN(len(wallIdx))]]
		if len(e.Textures) > 0 {
			wall = e.Textures[0]
		}
	}
	if len(floorIdx) > 0 {
		e := &lib.Entries[floorIdx[rng.IntN(len(floorIdx))]]
		if len(e.Textures) > 0 {
			floor = e.Textures[0]
			ceil = e.Textures[0]
		}
	}
	return
}

// --- Full map assembly ---

// AssembleMap generates a complete map using the brush library.
// Uses procgen layout for room placement, library brushes for surfaces.
func AssembleMap(rng *rand.Rand, lib *BrushLibrary, style BrushStyle, arenaSize, maxDepth int) (*MapFile, *Layout) {
	layout := GenerateProceduralLayout(rng, arenaSize, maxDepth)

	nav := ValidateNavigation(layout)
	status := "connected"
	if !nav.Connected {
		status = fmt.Sprintf("%d unreachable", len(nav.UnreachableRooms))
	}
	fmt.Printf("rooms=%d  corridors=%d  %s  style=%s\n",
		len(layout.Rooms), len(layout.Corridors), status, style)

	m := NewMapFile()
	m.Worldspawn.Properties["message"] = fmt.Sprintf("assembled_%s", style)
	m.Worldspawn.Properties["_minlight"] = "30"

	gi := procgenGridInfo(layout)
	zLo, zHi := computeZRange(layout)
	buildShell(m, gi, zLo, zHi)
	BuildGapFills(m, layout, zLo, zHi)

	// Build rooms from library brushes.
	for i := range layout.Rooms {
		AssembleRoom(m, &layout.Rooms[i], lib, style, rng)
	}

	// Build corridors from library brushes too.
	for i := range layout.Corridors {
		AssembleCorridor(m, &layout.Corridors[i], layout.Rooms, lib, style, rng)
	}

	// Populate with DM entities.
	PopulateDM(m, layout, rng)

	// Add guaranteed-visible lights at room and corridor centers.
	// The procgen lights often end up inside library brushes, so these
	// ensure every space is lit.
	for _, room := range layout.Rooms {
		cx, cy := room.CX(), room.CY()
		floorZ := room.Z0
		ceilZ := room.Z1
		midZ := (floorZ + ceilZ) / 2
		// Bright central light.
		m.AddLight(cx, cy, midZ, 300)
		// Four corner fill lights (inset from walls).
		insetX := room.Width() / 4
		insetY := room.Height() / 4
		for _, dx := range []int{-insetX, insetX} {
			for _, dy := range []int{-insetY, insetY} {
				m.AddLight(cx+dx, cy+dy, ceilZ-32, 150)
			}
		}
	}
	for _, c := range layout.Corridors {
		cx := (c.X0 + c.X1) / 2
		cy := (c.Y0 + c.Y1) / 2
		midZ := (c.Z0 + c.Z1) / 2
		m.AddLight(cx, cy, midZ, 200)
	}

	return m, layout
}

// AssembleCorridor builds a corridor from library brushes.
func AssembleCorridor(m *MapFile, c *Corridor, rooms []Room, lib *BrushLibrary, style BrushStyle, rng *rand.Rand) {
	x0, y0, x1, y1 := c.X0, c.Y0, c.X1, c.Y1
	z0, z1 := c.Z0, c.Z1
	corrW := float64(x1 - x0)
	corrH := float64(y1 - y0)
	corrD := float64(z1 - z0)

	wallTex, floorTex, _ := pickStyleTextures(lib, style, rng)

	// Floor.
	fb := pickAndPlace(lib, RoleFloor, style, corrW, corrH,
		float64(x0), float64(y0), float64(z0-Floor), float64(x1), float64(y1), float64(z0), rng)
	if fb != nil {
		m.AddBrush(*fb)
	} else {
		m.AddBrush(AxisAlignedBox(x0, y0, z0-Floor, x1, y1, z0, floorTex))
	}

	// Ceiling.
	cb := pickAndPlace(lib, RoleFloor, style, corrW, corrH,
		float64(x0), float64(y0), float64(z1), float64(x1), float64(y1), float64(z1+Floor), rng)
	if cb != nil {
		m.AddBrush(*cb)
	} else {
		m.AddBrush(AxisAlignedBox(x0, y0, z1, x1, y1, z1+Floor, floorTex))
	}

	// Side walls.
	thick := float64(Wall)
	if c.Axis == "x" {
		placeWallOrFallback(m, lib, style, rng,
			float64(x0), float64(y1), float64(z0), float64(x1), float64(y1)+thick, float64(z1),
			corrW, corrD, wallTex)
		placeWallOrFallback(m, lib, style, rng,
			float64(x0), float64(y0)-thick, float64(z0), float64(x1), float64(y0), float64(z1),
			corrW, corrD, wallTex)
	} else {
		placeWallOrFallback(m, lib, style, rng,
			float64(x1), float64(y0), float64(z0), float64(x1)+thick, float64(y1), float64(z1),
			corrH, corrD, wallTex)
		placeWallOrFallback(m, lib, style, rng,
			float64(x0)-thick, float64(y0), float64(z0), float64(x0), float64(y1), float64(z1),
			corrH, corrD, wallTex)
	}

	// Stairs and thresholds.
	buildStairs(m, c, floorTex)
	buildThreshold(m, c, rooms, floorTex)
}

// StyleFromName parses a style name string.
func StyleFromName(name string) BrushStyle {
	switch name {
	case "base":
		return StyleBase
	case "metal":
		return StyleMetal
	case "medieval":
		return StyleMedieval
	case "wizard":
		return StyleWizard
	case "tim":
		return StyleTim
	default:
		return StyleUnknown
	}
}

// RandomStyle picks a random style.
func RandomStyle(rng *rand.Rand) BrushStyle {
	styles := []BrushStyle{StyleBase, StyleMetal, StyleMedieval, StyleWizard, StyleTim}
	return styles[rng.IntN(len(styles))]
}

