package main

import (
	"fmt"
	"math/rand/v2"
)

const (
	ItemZAboveFloor = 32
	Margin          = 48
	MinEntitySpacing = 64
)

func itemZ(room *Room) int {
	return room.Z0 + ItemZAboveFloor
}

func pickXY(rng *rand.Rand, room *Room, pool *Pool, placement string, placed [][2]int) (int, int) {
	m := Margin

	valid := func(x, y int) bool {
		if pool != nil && x >= pool.X0 && x <= pool.X1 && y >= pool.Y0 && y <= pool.Y1 {
			return false
		}
		for _, p := range placed {
			if abs(x-p[0]) < MinEntitySpacing && abs(y-p[1]) < MinEntitySpacing {
				return false
			}
		}
		return true
	}

	safeRandInt := func(lo, hi int) int {
		if lo >= hi {
			return lo
		}
		return lo + rng.IntN(hi-lo+1)
	}

	switch placement {
	case "center":
		if valid(room.CX(), room.CY()) {
			return room.CX(), room.CY()
		}

	case "corner":
		corners := [][2]int{
			{room.X0 + m, room.Y0 + m},
			{room.X0 + m, room.Y1 - m},
			{room.X1 - m, room.Y0 + m},
			{room.X1 - m, room.Y1 - m},
		}
		rng.Shuffle(len(corners), func(i, j int) { corners[i], corners[j] = corners[j], corners[i] })
		for _, c := range corners {
			if valid(c[0], c[1]) {
				return c[0], c[1]
			}
		}

	case "wall":
		sides := [][2]int{
			{safeRandInt(room.X0+m, room.X1-m), room.Y0 + m},
			{safeRandInt(room.X0+m, room.X1-m), room.Y1 - m},
			{room.X0 + m, safeRandInt(room.Y0+m, room.Y1-m)},
			{room.X1 - m, safeRandInt(room.Y0+m, room.Y1-m)},
		}
		rng.Shuffle(len(sides), func(i, j int) { sides[i], sides[j] = sides[j], sides[i] })
		for _, s := range sides {
			if valid(s[0], s[1]) {
				return s[0], s[1]
			}
		}

	case "near_hazard":
		if pool != nil {
			for range 20 {
				var x, y int
				switch rng.IntN(4) {
				case 0:
					x = safeRandInt(pool.X0, pool.X1)
					y = pool.Y1 + 32
				case 1:
					x = safeRandInt(pool.X0, pool.X1)
					y = pool.Y0 - 32
				case 2:
					x = pool.X1 + 32
					y = safeRandInt(pool.Y0, pool.Y1)
				case 3:
					x = pool.X0 - 32
					y = safeRandInt(pool.Y0, pool.Y1)
				}
				x = clamp(x, room.X0+m, room.X1-m)
				y = clamp(y, room.Y0+m, room.Y1-m)
				if valid(x, y) {
					return x, y
				}
			}
		}

	case "hidden":
		corners := [][2]int{
			{room.X0 + m, room.Y0 + m},
			{room.X0 + m, room.Y1 - m},
			{room.X1 - m, room.Y0 + m},
			{room.X1 - m, room.Y1 - m},
		}
		// Sort by distance from center, farthest first.
		cx, cy := room.CX(), room.CY()
		for i := 1; i < len(corners); i++ {
			for j := i; j > 0; j-- {
				di := abs(corners[j][0]-cx) + abs(corners[j][1]-cy)
				dj := abs(corners[j-1][0]-cx) + abs(corners[j-1][1]-cy)
				if di > dj {
					corners[j], corners[j-1] = corners[j-1], corners[j]
				}
			}
		}
		for _, c := range corners {
			if valid(c[0], c[1]) {
				return c[0], c[1]
			}
		}
	}

	// Fallback: random.
	for range 30 {
		x := safeRandInt(room.X0+m, room.X1-m)
		y := safeRandInt(room.Y0+m, room.Y1-m)
		if valid(x, y) {
			return x, y
		}
	}
	return room.CX(), room.CY()
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// PopulateFromBlueprint places all entities according to blueprint specs.
func PopulateFromBlueprint(
	m *MapFile, layout *Layout, bp *ResolvedBlueprint,
	idToIdx map[string]int, teleConns []ResolvedConnection,
	rng *rand.Rand,
) {
	var placed [][2]int
	hasPlayerStart := false

	for _, br := range bp.Rooms {
		idx, ok := idToIdx[br.ID]
		if !ok {
			continue
		}
		room := &layout.Rooms[idx]
		p, hasPool := layout.Pools[idx]
		var pool *Pool
		if hasPool {
			pool = &p
		}

		for _, item := range br.Items {
			for range item.Count {
				if item.Classname == "light" {
					lx := room.CX() + safeRandRange(rng, -room.Width()/4, room.Width()/4)
					ly := room.CY() + safeRandRange(rng, -room.Height()/4, room.Height()/4)
					m.AddLight(lx, ly, room.Z1-16, 300)
					continue
				}

				x, y := pickXY(rng, room, pool, item.Placement, placed)
				z := itemZ(room)

				switch item.Classname {
				case "info_player_start":
					hasPlayerStart = true
					m.AddEntity(item.Classname, x, y, z, map[string]string{"angle": "0"})
				case "info_player_deathmatch":
					angles := []string{"0", "90", "180", "270"}
					m.AddEntity(item.Classname, x, y, z, map[string]string{"angle": angles[rng.IntN(4)]})
				default:
					m.AddEntity(item.Classname, x, y, z, nil)
				}
				placed = append(placed, [2]int{x, y})
			}
		}
	}

	// Ensure info_player_start.
	if !hasPlayerStart && len(layout.Rooms) > 0 {
		room := &layout.Rooms[0]
		m.AddEntity("info_player_start", room.CX(), room.CY(), itemZ(room), map[string]string{"angle": "0"})
	}

	// Corridor lights.
	for _, c := range layout.Corridors {
		cx := (c.X0 + c.X1) / 2
		cy := (c.Y0 + c.Y1) / 2
		m.AddLight(cx, cy, c.Z1-8, 200)
	}

	// Teleporters.
	placeTeleporters(m, layout, idToIdx, teleConns, rng, &placed)
}

func placeTeleporters(
	m *MapFile, layout *Layout, idToIdx map[string]int,
	conns []ResolvedConnection, rng *rand.Rand, placed *[][2]int,
) {
	for i, conn := range conns {
		srcIdx, okS := idToIdx[conn.FromID]
		dstIdx, okD := idToIdx[conn.ToID]
		if !okS || !okD {
			continue
		}

		srcRoom := &layout.Rooms[srcIdx]
		dstRoom := &layout.Rooms[dstIdx]
		var srcPool, dstPool *Pool
		if p, ok := layout.Pools[srcIdx]; ok {
			srcPool = &p
		}
		if p, ok := layout.Pools[dstIdx]; ok {
			dstPool = &p
		}

		targetName := fmt.Sprintf("tele_dest_%d", i)

		// Destination.
		dx, dy := pickXY(rng, dstRoom, dstPool, "any", *placed)
		angles := []string{"0", "90", "180", "270"}
		m.AddEntity("info_teleport_destination", dx, dy, itemZ(dstRoom), map[string]string{
			"targetname": targetName,
			"angle":      angles[rng.IntN(4)],
		})
		*placed = append(*placed, [2]int{dx, dy})

		// Teleporter pad + trigger.
		sx, sy := pickXY(rng, srcRoom, srcPool, "any", *placed)
		tw := 32
		padH := 8

		// Visible pad (raised platform with teleport texture).
		m.AddBrush(AxisAlignedBox(
			sx-tw-8, sy-tw-8, srcRoom.Z0,
			sx+tw+8, sy+tw+8, srcRoom.Z0+padH,
			"*teleport",
		))

		// Trigger volume (sits on top of pad).
		triggerBrush := AxisAlignedBox(
			sx-tw, sy-tw, srcRoom.Z0+padH,
			sx+tw, sy+tw, srcRoom.Z0+padH+64,
			"*teleport",
		)
		m.Entities = append(m.Entities, Entity{
			Properties: map[string]string{
				"classname": "trigger_teleport",
				"target":    targetName,
			},
			Brushes: []Brush{triggerBrush},
		})

		// Light above the pad.
		m.AddLight(sx, sy, srcRoom.Z0+padH+48, 200)

		*placed = append(*placed, [2]int{sx, sy})

		// Reverse if bidirectional.
		if conn.Bidirectional {
			revName := fmt.Sprintf("tele_dest_%d_rev", i)
			dx2, dy2 := pickXY(rng, srcRoom, srcPool, "any", *placed)
			m.AddEntity("info_teleport_destination", dx2, dy2, itemZ(srcRoom), map[string]string{
				"targetname": revName,
				"angle":      angles[rng.IntN(4)],
			})
			*placed = append(*placed, [2]int{dx2, dy2})

			sx2, sy2 := pickXY(rng, dstRoom, dstPool, "any", *placed)
			// Visible pad.
			m.AddBrush(AxisAlignedBox(
				sx2-tw-8, sy2-tw-8, dstRoom.Z0,
				sx2+tw+8, sy2+tw+8, dstRoom.Z0+padH,
				"*teleport",
			))
			m.Entities = append(m.Entities, Entity{
				Properties: map[string]string{
					"classname": "trigger_teleport",
					"target":    revName,
				},
				Brushes: []Brush{AxisAlignedBox(
					sx2-tw, sy2-tw, dstRoom.Z0+padH,
					sx2+tw, sy2+tw, dstRoom.Z0+padH+64,
					"*teleport",
				)},
			})
			m.AddLight(sx2, sy2, dstRoom.Z0+padH+48, 200)
			*placed = append(*placed, [2]int{sx2, sy2})
		}
	}
}

func safeRandRange(rng *rand.Rand, lo, hi int) int {
	if lo >= hi {
		return lo
	}
	return lo + rng.IntN(hi-lo+1)
}
