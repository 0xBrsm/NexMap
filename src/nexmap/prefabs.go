package main

import "math/rand/v2"

// Detail is a named architectural detail that can be placed in a room.
type Detail string

const (
	DetailPillars        Detail = "pillars"
	DetailPlatform       Detail = "platform"
	DetailLightRecesses  Detail = "light_recesses"
	DetailWallTrim       Detail = "wall_trim"
	DetailCrates         Detail = "crates"
	DetailStepDown       Detail = "step_down"
)

const (
	pillarSize     = 32
	pillarInset    = 16  // distance from wall to pillar edge
	platformHeight = 32
	platformInset  = 64
	recessDepth    = 16
	recessWidth    = 48
	recessHeight   = 64
	trimHeight     = 8
	trimDepth      = 8
	crateSize      = 32
)

// PlaceDetail adds architectural detail brushes to a room.
func PlaceDetail(m *MapFile, room *Room, pool *Pool, detail Detail, mat *RoomMaterials, rng *rand.Rand) {
	switch detail {
	case DetailPillars:
		placePillars(m, room, pool, mat)
	case DetailPlatform:
		placePlatform(m, room, pool, mat)
	case DetailLightRecesses:
		placeLightRecesses(m, room, mat)
	case DetailWallTrim:
		placeWallTrim(m, room, mat)
	case DetailCrates:
		placeCrates(m, room, pool, mat, rng)
	case DetailStepDown:
		placeStepDown(m, room, pool, mat)
	}
}

// --- Pillars ---
// Four columns near room corners, breaking up flat walls.

func placePillars(m *MapFile, room *Room, pool *Pool, mat *RoomMaterials) {
	if room.Width() < 256 || room.Height() < 256 {
		return
	}

	tex := mat.Wall
	ps := pillarSize
	inset := pillarInset + ps

	corners := [][2]int{
		{room.X0 + inset, room.Y0 + inset},
		{room.X0 + inset, room.Y1 - inset},
		{room.X1 - inset, room.Y0 + inset},
		{room.X1 - inset, room.Y1 - inset},
	}

	for _, c := range corners {
		if pool != nil && rectsOverlap(c[0]-ps/2, c[1]-ps/2, c[0]+ps/2, c[1]+ps/2,
			pool.X0, pool.Y0, pool.X1, pool.Y1) {
			continue
		}
		m.AddBrush(AxisAlignedBox(
			c[0]-ps/2, c[1]-ps/2, room.Z0,
			c[0]+ps/2, c[1]+ps/2, room.Z1,
			tex,
		))
	}
}

// --- Raised platform ---
// A raised area in the center of the room.

func placePlatform(m *MapFile, room *Room, pool *Pool, mat *RoomMaterials) {
	if room.Width() < 320 || room.Height() < 320 {
		return
	}

	inset := platformInset
	px0 := room.X0 + inset
	py0 := room.Y0 + inset
	px1 := room.X1 - inset
	py1 := room.Y1 - inset

	if pool != nil && rectsOverlap(px0, py0, px1, py1, pool.X0, pool.Y0, pool.X1, pool.Y1) {
		return
	}

	m.AddBrush(AxisAlignedBox(
		px0, py0, room.Z0,
		px1, py1, room.Z0+platformHeight,
		mat.Floor,
	))
}

// --- Light recesses ---
// Small alcoves cut into walls with a light inside.
// We approximate by placing a protruding frame brush on each wall
// with a light entity in front of it.

func placeLightRecesses(m *MapFile, room *Room, mat *RoomMaterials) {
	if room.Width() < 192 || room.Height() < 192 {
		return
	}

	lightZ := room.Z0 + (room.Z1-room.Z0)*2/3
	rw := recessWidth
	rd := recessDepth
	rh := recessHeight

	// One recess per wall, centered.
	type recess struct {
		x0, y0, x1, y1, z0, z1 int
		lx, ly                  int // light position
	}

	recesses := []recess{
		// South wall
		{room.X0 + room.Width()/2 - rw/2, room.Y0, room.X0 + room.Width()/2 + rw/2, room.Y0 + rd,
			lightZ - rh/2, lightZ + rh/2,
			room.X0 + room.Width()/2, room.Y0 + rd + 8},
		// North wall
		{room.X0 + room.Width()/2 - rw/2, room.Y1 - rd, room.X0 + room.Width()/2 + rw/2, room.Y1,
			lightZ - rh/2, lightZ + rh/2,
			room.X0 + room.Width()/2, room.Y1 - rd - 8},
		// West wall
		{room.X0, room.Y0 + room.Height()/2 - rw/2, room.X0 + rd, room.Y0 + room.Height()/2 + rw/2,
			lightZ - rh/2, lightZ + rh/2,
			room.X0 + rd + 8, room.Y0 + room.Height()/2},
		// East wall
		{room.X1 - rd, room.Y0 + room.Height()/2 - rw/2, room.X1, room.Y0 + room.Height()/2 + rw/2,
			lightZ - rh/2, lightZ + rh/2,
			room.X1 - rd - 8, room.Y0 + room.Height()/2},
	}

	for _, r := range recesses {
		// Frame brush (the alcove surround).
		m.AddBrush(AxisAlignedBox(r.x0, r.y0, r.z0, r.x1, r.y1, r.z1, mat.Wall))
		// Light entity.
		m.AddLight(r.lx, r.ly, lightZ, 150)
	}
}

// --- Wall trim ---
// A small step/ledge at the base of all four walls.

func placeWallTrim(m *MapFile, room *Room, mat *RoomMaterials) {
	if room.Width() < 128 || room.Height() < 128 {
		return
	}

	z0 := room.Z0
	z1 := room.Z0 + trimHeight
	d := trimDepth

	// South wall trim
	m.AddBrush(AxisAlignedBox(room.X0, room.Y0, z0, room.X1, room.Y0+d, z1, mat.Wall))
	// North wall trim
	m.AddBrush(AxisAlignedBox(room.X0, room.Y1-d, z0, room.X1, room.Y1, z1, mat.Wall))
	// West wall trim
	m.AddBrush(AxisAlignedBox(room.X0, room.Y0+d, z0, room.X0+d, room.Y1-d, z1, mat.Wall))
	// East wall trim
	m.AddBrush(AxisAlignedBox(room.X1-d, room.Y0+d, z0, room.X1, room.Y1-d, z1, mat.Wall))
}

// --- Crates ---
// Random crate clusters near walls.

func placeCrates(m *MapFile, room *Room, pool *Pool, mat *RoomMaterials, rng *rand.Rand) {
	if room.Width() < 192 || room.Height() < 192 {
		return
	}

	cs := crateSize
	count := 2 + rng.IntN(3) // 2-4 crates

	for range count {
		margin := cs + 16
		x := room.X0 + margin + rng.IntN(max(1, room.Width()-margin*2))
		y := room.Y0 + margin + rng.IntN(max(1, room.Height()-margin*2))

		// Snap to nearest wall.
		dists := [4]int{
			x - room.X0,          // west
			room.X1 - x,          // east
			y - room.Y0,          // south
			room.Y1 - y,          // north
		}
		minDist := dists[0]
		side := 0
		for i := 1; i < 4; i++ {
			if dists[i] < minDist {
				minDist = dists[i]
				side = i
			}
		}
		switch side {
		case 0:
			x = room.X0 + 16
		case 1:
			x = room.X1 - 16 - cs
		case 2:
			y = room.Y0 + 16
		case 3:
			y = room.Y1 - 16 - cs
		}

		if pool != nil && rectsOverlap(x, y, x+cs, y+cs, pool.X0, pool.Y0, pool.X1, pool.Y1) {
			continue
		}

		// Stack height: 1 or 2 crates.
		h := cs
		if rng.IntN(3) == 0 {
			h = cs * 2
		}
		m.AddBrush(AxisAlignedBox(x, y, room.Z0, x+cs, y+cs, room.Z0+h, mat.Floor))
	}
}

// --- Step down ---
// The outer perimeter of the room is lowered, creating a sunken center.

func placeStepDown(m *MapFile, room *Room, pool *Pool, mat *RoomMaterials) {
	if room.Width() < 320 || room.Height() < 320 {
		return
	}
	if pool != nil {
		return // don't combine with pools
	}

	stepW := 48
	stepH := 16
	x0, y0, x1, y1 := room.X0, room.Y0, room.X1, room.Y1

	// Raise the center by adding a platform that covers the inner area.
	// The "step down" effect is the perimeter being at the original floor level
	// while the center is elevated.
	inner := AxisAlignedBox(
		x0+stepW, y0+stepW, room.Z0,
		x1-stepW, y1-stepW, room.Z0+stepH,
		mat.Floor,
	)
	m.AddBrush(inner)
}

// --- Random detail selection for procgen ---

// RandomDetails picks appropriate details for a room based on its size.
func RandomDetails(rng *rand.Rand, room *Room, hasPool bool) []Detail {
	var candidates []Detail

	w, h := room.Width(), room.Height()

	if w >= 256 && h >= 256 {
		candidates = append(candidates, DetailPillars)
	}
	if w >= 320 && h >= 320 && !hasPool {
		candidates = append(candidates, DetailPlatform)
		candidates = append(candidates, DetailStepDown)
	}
	if w >= 192 && h >= 192 {
		candidates = append(candidates, DetailLightRecesses)
		candidates = append(candidates, DetailCrates)
	}
	if w >= 128 && h >= 128 {
		candidates = append(candidates, DetailWallTrim)
	}

	if len(candidates) == 0 {
		return nil
	}

	// Pick 1-2 non-conflicting details.
	rng.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	var picked []Detail
	hasHeight := false // platform or step_down
	for _, d := range candidates {
		if len(picked) >= 2 {
			break
		}
		if (d == DetailPlatform || d == DetailStepDown) && hasHeight {
			continue // don't combine elevation details
		}
		if d == DetailPlatform || d == DetailStepDown {
			hasHeight = true
		}
		// Pillars conflict with platform (they'd overlap).
		if d == DetailPillars && hasHeight {
			continue
		}
		picked = append(picked, d)
	}

	return picked
}

// ChunkAwareDetails selects details informed by whether the source map
// has architectural features (recesses, pillars) in its wall geometry.
func ChunkAwareDetails(rng *rand.Rand, room *Room, hasPool bool, sourceHasFeatures bool) []Detail {
	if !sourceHasFeatures {
		// Source map has flat walls — use structural details to add interest.
		return RandomDetails(rng, room, hasPool)
	}

	// Source map already has wall features — favor complementary details
	// like light recesses and trim rather than competing geometry.
	var candidates []Detail
	w, h := room.Width(), room.Height()

	// Always consider these — they complement featured walls.
	if w >= 192 && h >= 192 {
		candidates = append(candidates, DetailLightRecesses)
	}
	if w >= 128 && h >= 128 {
		candidates = append(candidates, DetailWallTrim)
	}

	// Only add floor-level details for large rooms.
	if w >= 320 && h >= 320 && !hasPool {
		candidates = append(candidates, DetailStepDown)
		candidates = append(candidates, DetailPlatform)
	}

	// Pillars work well with featured walls — they're at corners, not on walls.
	if w >= 384 && h >= 384 {
		candidates = append(candidates, DetailPillars)
	}

	if len(candidates) == 0 {
		return nil
	}

	rng.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	// Pick 1-2, respecting conflicts.
	var picked []Detail
	hasHeight := false
	for _, d := range candidates {
		if len(picked) >= 2 {
			break
		}
		if (d == DetailPlatform || d == DetailStepDown) && hasHeight {
			continue
		}
		if d == DetailPlatform || d == DetailStepDown {
			hasHeight = true
		}
		if d == DetailPillars && hasHeight {
			continue
		}
		picked = append(picked, d)
	}
	return picked
}

// chunkHasFeatures checks if a source map's walls have depth variation.
func chunkHasFeatures(lib *MapChunkLibrary, mapName string) bool {
	if lib == nil {
		return false
	}
	for _, wc := range lib.Walls {
		if wc.Source == mapName && (wc.HasRecess || wc.HasPillar) {
			return true
		}
	}
	return false
}

func rectsOverlap(ax0, ay0, ax1, ay1, bx0, by0, bx1, by1 int) bool {
	return ax0 < bx1 && ax1 > bx0 && ay0 < by1 && ay1 > by0
}
