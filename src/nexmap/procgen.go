package main

import (
	"fmt"
	"math/rand/v2"
)

// --- BSP subdivision layout generator ---

// Subdivide recursively splits a rectangle into rooms via BSP.
func Subdivide(rng *rand.Rand, x0, y0, x1, y1, depth, maxDepth int) ([]Room, []Split) {
	w := x1 - x0
	h := y1 - y0

	canSplitX := w >= MinRoomSize*2+CorridorWidth
	canSplitY := h >= MinRoomSize*2+CorridorWidth
	oversized := w > MaxRoomSize || h > MaxRoomSize
	atLimit := depth >= maxDepth

	if (atLimit && !oversized) || (!canSplitX && !canSplitY) {
		nSteps := (FloorElevMax - FloorElevMin) / FloorElevStep
		floorZ := FloorElevMin + rng.IntN(nSteps+1)*FloorElevStep
		ceilH := CeilingHeights[rng.IntN(len(CeilingHeights))]
		return []Room{{X0: x0, Y0: y0, X1: x1, Y1: y1, Z0: floorZ, Z1: floorZ + ceilH}}, nil
	}

	var splitX bool
	if canSplitX && canSplitY {
		if w >= h {
			splitX = rng.Float64() < 0.7
		} else {
			splitX = rng.Float64() < 0.3
		}
	} else {
		splitX = canSplitX
	}

	if splitX {
		lo := x0 + MinRoomSize
		hi := x1 - MinRoomSize - CorridorWidth
		mid := (lo + hi) / 2
		quarter := max(1, (hi-lo)/4)
		split := rng.IntN(min(hi, mid+quarter)-max(lo, mid-quarter)+1) + max(lo, mid-quarter)

		leftRooms, leftSplits := Subdivide(rng, x0, y0, split, y1, depth+1, maxDepth)
		rightRooms, rightSplits := Subdivide(rng, split+CorridorWidth, y0, x1, y1, depth+1, maxDepth)
		mySplit := Split{Axis: "x", Pos: split, PerpLo: y0, PerpHi: y1}
		return append(leftRooms, rightRooms...), append(append(leftSplits, rightSplits...), mySplit)
	}

	lo := y0 + MinRoomSize
	hi := y1 - MinRoomSize - CorridorWidth
	mid := (lo + hi) / 2
	quarter := max(1, (hi-lo)/4)
	split := rng.IntN(min(hi, mid+quarter)-max(lo, mid-quarter)+1) + max(lo, mid-quarter)

	botRooms, botSplits := Subdivide(rng, x0, y0, x1, split, depth+1, maxDepth)
	topRooms, topSplits := Subdivide(rng, x0, split+CorridorWidth, x1, y1, depth+1, maxDepth)
	mySplit := Split{Axis: "y", Pos: split, PerpLo: x0, PerpHi: x1}
	return append(botRooms, topRooms...), append(append(botSplits, topSplits...), mySplit)
}

// ConnectRooms finds adjacent room pairs separated by a corridor-width gap
// and creates corridors between them.
func ConnectRooms(rng *rand.Rand, rooms []Room) []Corridor {
	var corridors []Corridor
	n := len(rooms)

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			a, b := &rooms[i], &rooms[j]
			iIdx, jIdx := i, j

			axis := roomsShareWall(a, b)
			if axis == "" {
				axis = roomsShareWall(b, a)
				if axis != "" {
					a, b = b, a
					iIdx, jIdx = j, i
				}
			}
			if axis == "" {
				continue
			}

			lowZ := min(a.Z0, b.Z0)
			highZ := max(a.Z0, b.Z0)
			ceilZ := min(highZ+CorridorHeadroom, a.Z1, b.Z1)

			if axis == "x" {
				ovY0 := max(a.Y0, b.Y0)
				ovY1 := min(a.Y1, b.Y1)
				cy := ovY0 + CorridorWidth/2 + rng.IntN(ovY1-ovY0-CorridorWidth+1)
				corridors = append(corridors, Corridor{
					RoomA: iIdx, RoomB: jIdx,
					X0: a.X1, Y0: cy - CorridorWidth/2,
					X1: b.X0, Y1: cy + CorridorWidth/2,
					Z0: lowZ, Z1: ceilZ,
					Z0A: rooms[iIdx].Z0, Z0B: rooms[jIdx].Z0,
					Axis: "x",
				})
			} else {
				ovX0 := max(a.X0, b.X0)
				ovX1 := min(a.X1, b.X1)
				cx := ovX0 + CorridorWidth/2 + rng.IntN(ovX1-ovX0-CorridorWidth+1)
				corridors = append(corridors, Corridor{
					RoomA: iIdx, RoomB: jIdx,
					X0: cx - CorridorWidth/2, Y0: a.Y1,
					X1: cx + CorridorWidth/2, Y1: b.Y0,
					Z0: lowZ, Z1: ceilZ,
					Z0A: rooms[iIdx].Z0, Z0B: rooms[jIdx].Z0,
					Axis: "y",
				})
			}
		}
	}
	return corridors
}

func roomsShareWall(a, b *Room) string {
	if b.X0-a.X1 == CorridorWidth {
		ovY0 := max(a.Y0, b.Y0)
		ovY1 := min(a.Y1, b.Y1)
		if ovY1-ovY0 >= CorridorWidth {
			return "x"
		}
	}
	if b.Y0-a.Y1 == CorridorWidth {
		ovX0 := max(a.X0, b.X0)
		ovX1 := min(a.X1, b.X1)
		if ovX1-ovX0 >= CorridorWidth {
			return "y"
		}
	}
	return ""
}

// --- Random pool generation ---

func GeneratePools(rng *rand.Rand, rooms []Room) map[int]Pool {
	pools := map[int]Pool{}
	for i, room := range rooms {
		if room.Width() < 320 || room.Height() < 320 {
			continue
		}
		roll := rng.Float64()
		force := len(pools) < 2
		var tex string
		switch {
		case roll < 0.15:
			tex = Textures.Lava
		case roll < 0.50:
			tex = Textures.Water
		case roll < 0.55:
			tex = Textures.Slime
		case force:
			tex = Textures.Water
		default:
			continue
		}

		maxPW := min(256, room.Width()-PoolBorder*2)
		maxPH := min(256, room.Height()-PoolBorder*2)
		if maxPW < 96 || maxPH < 96 {
			continue
		}
		pw := 96 + rng.IntN(maxPW-96+1)
		ph := 96 + rng.IntN(maxPH-96+1)
		px := room.X0 + PoolBorder + rng.IntN(room.Width()-PoolBorder*2-pw+1)
		py := room.Y0 + PoolBorder + rng.IntN(room.Height()-PoolBorder*2-ph+1)
		pools[i] = Pool{X0: px, Y0: py, X1: px + pw, Y1: py + ph, Texture: tex}
	}
	return pools
}

// --- Navcheck ---

type NavcheckResult struct {
	Connected        bool
	ComponentCount   int
	UnreachableRooms []int
}

func ValidateNavigation(layout *Layout) NavcheckResult {
	n := len(layout.Rooms)
	if n == 0 {
		return NavcheckResult{}
	}

	adj := make([][]int, n)
	for _, c := range layout.Corridors {
		if c.RoomA < 0 || c.RoomA >= n || c.RoomB < 0 || c.RoomB >= n {
			continue
		}
		// Width check.
		width := c.Y1 - c.Y0
		if c.Axis == "y" {
			width = c.X1 - c.X0
		}
		if width < WalkableRadius*2 {
			continue
		}
		// Height check.
		dz := abs(c.Z0B - c.Z0A)
		if dz > 0 {
			bridged := min(dz, MaxStairRise)
			remainder := dz - bridged
			if remainder > WalkableClimb {
				continue
			}
			totalRun := c.X1 - c.X0
			if c.Axis == "y" {
				totalRun = c.Y1 - c.Y0
			}
			maxSteps := totalRun / MinStepRun
			minSteps := (bridged + WalkableClimb - 1) / WalkableClimb
			if minSteps > maxSteps {
				continue
			}
		}
		adj[c.RoomA] = append(adj[c.RoomA], c.RoomB)
		adj[c.RoomB] = append(adj[c.RoomB], c.RoomA)
	}

	// BFS from room 0.
	visited := make([]bool, n)
	queue := []int{0}
	visited[0] = true
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, nb := range adj[cur] {
			if !visited[nb] {
				visited[nb] = true
				queue = append(queue, nb)
			}
		}
	}

	var unreachable []int
	for i := range n {
		if !visited[i] {
			unreachable = append(unreachable, i)
		}
	}

	// Count components.
	compCount := 1
	for i := range n {
		if visited[i] {
			continue
		}
		compCount++
		cq := []int{i}
		visited[i] = true
		for len(cq) > 0 {
			cur := cq[0]
			cq = cq[1:]
			for _, nb := range adj[cur] {
				if !visited[nb] {
					visited[nb] = true
					cq = append(cq, nb)
				}
			}
		}
	}

	return NavcheckResult{
		Connected:        len(unreachable) == 0,
		ComponentCount:   compCount,
		UnreachableRooms: unreachable,
	}
}

// --- DM entity placement ---

var weapons = []string{
	"weapon_supershotgun",
	"weapon_nailgun",
	"weapon_supernailgun",
	"weapon_grenadelauncher",
	"weapon_rocketlauncher",
	"weapon_lightning",
}

var ammoForWeapon = map[string]string{
	"weapon_supershotgun":     "item_shells",
	"weapon_nailgun":          "item_spikes",
	"weapon_supernailgun":     "item_spikes",
	"weapon_grenadelauncher":  "item_rockets",
	"weapon_rocketlauncher":   "item_rockets",
	"weapon_lightning":        "item_cells",
}

func PopulateDM(m *MapFile, layout *Layout, rng *rand.Rand) {
	var placed [][2]int

	// Spawns: 1 per room.
	for i := range layout.Rooms {
		room := &layout.Rooms[i]
		x, y := safeXYPlaced(rng, room, layout.Pools[i], layout, placed)
		angle := []string{"0", "90", "180", "270"}[rng.IntN(4)]
		m.AddEntity("info_player_deathmatch", x, y, room.Z0+32, map[string]string{"angle": angle})
		placed = append(placed, [2]int{x, y})
	}
	// Required info_player_start.
	r := &layout.Rooms[0]
	x, y := safeXYPlaced(rng, r, layout.Pools[0], layout, placed)
	m.AddEntity("info_player_start", x, y, r.Z0+32, map[string]string{"angle": "0"})
	placed = append(placed, [2]int{x, y})

	// Weapons: all 6 types, extra RL if enough rooms.
	wp := make([]string, len(weapons))
	copy(wp, weapons)
	if len(layout.Rooms) >= 7 {
		wp = append(wp, "weapon_rocketlauncher")
	}
	rng.Shuffle(len(wp), func(i, j int) { wp[i], wp[j] = wp[j], wp[i] })

	roomOrder := make([]int, len(layout.Rooms))
	for i := range roomOrder {
		roomOrder[i] = i
	}
	rng.Shuffle(len(roomOrder), func(i, j int) { roomOrder[i], roomOrder[j] = roomOrder[j], roomOrder[i] })

	for i, weapon := range wp {
		ri := roomOrder[i%len(roomOrder)]
		room := &layout.Rooms[ri]
		pool, _ := layout.Pools[ri]
		wx, wy := safeXYPlaced(rng, room, pool, layout, placed)
		m.AddEntity(weapon, wx, wy, room.Z0+32, nil)
		placed = append(placed, [2]int{wx, wy})

		// 1-2 ammo near weapon.
		ammo := ammoForWeapon[weapon]
		for range 1 + rng.IntN(2) {
			ax := clamp(wx+rng.IntN(161)-80, room.X0+32, room.X1-32)
			ay := clamp(wy+rng.IntN(161)-80, room.Y0+32, room.Y1-32)
			if !inGapFill(ax, ay, layout) {
				m.AddEntity(ammo, ax, ay, room.Z0+32, nil)
				placed = append(placed, [2]int{ax, ay})
			}
		}
	}

	// Health: 2-3 per room.
	nRooms := len(layout.Rooms)
	targetHealth := max(10, min(20, nRooms*2+2+rng.IntN(3)))
	healthPlaced := 0
	for i := range layout.Rooms {
		room := &layout.Rooms[i]
		pool, _ := layout.Pools[i]
		count := 1
		if healthPlaced < targetHealth {
			count = 1 + rng.IntN(3)
		}
		for range count {
			if healthPlaced >= targetHealth {
				break
			}
			hx, hy := safeXYPlaced(rng, room, pool, layout, placed)
			m.AddEntity("item_health", hx, hy, room.Z0+32, nil)
			placed = append(placed, [2]int{hx, hy})
			healthPlaced++
		}
	}

	// Armor: 1 red, 1 yellow, 1 green + extras for larger maps.
	armorPool := []string{"item_armorInv", "item_armor2", "item_armor1"}
	if nRooms >= 5 {
		armorPool = append(armorPool, []string{"item_armor1", "item_armor2"}[rng.IntN(2)])
	}
	if nRooms >= 7 {
		armorPool = append(armorPool, []string{"item_armor1", "item_armor2"}[rng.IntN(2)])
	}
	rng.Shuffle(len(roomOrder), func(i, j int) { roomOrder[i], roomOrder[j] = roomOrder[j], roomOrder[i] })
	for i, armor := range armorPool {
		ri := roomOrder[i%nRooms]
		room := &layout.Rooms[ri]
		pool, _ := layout.Pools[ri]
		ax, ay := safeXYPlaced(rng, room, pool, layout, placed)
		m.AddEntity(armor, ax, ay, room.Z0+32, nil)
		placed = append(placed, [2]int{ax, ay})
	}

	// Powerups: 60% quad, 25% pent/ring.
	if rng.Float64() < 0.6 {
		ri := rng.IntN(nRooms)
		room := &layout.Rooms[ri]
		pool, _ := layout.Pools[ri]
		px, py := safeXYPlaced(rng, room, pool, layout, placed)
		m.AddEntity("item_artifact_super_damage", px, py, room.Z0+32, nil)
		placed = append(placed, [2]int{px, py})
	}
	if rng.Float64() < 0.25 {
		powerup := []string{"item_artifact_invulnerability", "item_artifact_invisibility"}[rng.IntN(2)]
		ri := rng.IntN(nRooms)
		room := &layout.Rooms[ri]
		pool, _ := layout.Pools[ri]
		px, py := safeXYPlaced(rng, room, pool, layout, placed)
		m.AddEntity(powerup, px, py, room.Z0+32, nil)
		placed = append(placed, [2]int{px, py})
	}

	// Lights.
	for _, room := range layout.Rooms {
		m.AddLight(room.CX(), room.CY(), room.Z1-16, 300)
		if room.Width() > 400 && room.Height() > 400 {
			qx := room.Width() / 4
			qy := room.Height() / 4
			for _, d := range [][2]int{{-qx, -qy}, {qx, -qy}, {-qx, qy}, {qx, qy}} {
				m.AddLight(room.CX()+d[0], room.CY()+d[1], room.Z1-16, 200)
			}
		}
	}
	for _, c := range layout.Corridors {
		m.AddLight((c.X0+c.X1)/2, (c.Y0+c.Y1)/2, c.Z1-8, 200)
	}
}

const minEntitySpacing = 64

func safeXY(rng *rand.Rand, room *Room, pool Pool, layout *Layout) (int, int) {
	return safeXYPlaced(rng, room, pool, layout, nil)
}

func safeXYPlaced(rng *rand.Rand, room *Room, pool Pool, layout *Layout, placed [][2]int) (int, int) {
	margin := 48

	valid := func(x, y int) bool {
		if inPool(x, y, &pool) {
			return false
		}
		if inGapFill(x, y, layout) {
			return false
		}
		for _, p := range placed {
			if abs(x-p[0]) < minEntitySpacing && abs(y-p[1]) < minEntitySpacing {
				return false
			}
		}
		return true
	}

	// 20 random attempts.
	for range 20 {
		x := room.X0 + margin + rng.IntN(max(1, room.Width()-margin*2))
		y := room.Y0 + margin + rng.IntN(max(1, room.Height()-margin*2))
		if valid(x, y) {
			return x, y
		}
	}

	// Center.
	if valid(room.CX(), room.CY()) {
		return room.CX(), room.CY()
	}

	// Corners.
	corners := [][2]int{
		{room.X0 + margin, room.Y0 + margin},
		{room.X0 + margin, room.Y1 - margin},
		{room.X1 - margin, room.Y0 + margin},
		{room.X1 - margin, room.Y1 - margin},
	}
	for _, c := range corners {
		if valid(c[0], c[1]) {
			return c[0], c[1]
		}
	}

	// Grid scan.
	for sy := room.Y0 + margin; sy <= room.Y1-margin; sy += 32 {
		for sx := room.X0 + margin; sx <= room.X1-margin; sx += 32 {
			if valid(sx, sy) {
				return sx, sy
			}
		}
	}

	return room.CX(), room.CY()
}

func inPool(x, y int, p *Pool) bool {
	if p == nil || (p.X0 == 0 && p.Y0 == 0 && p.X1 == 0 && p.Y1 == 0) {
		return false
	}
	return x >= p.X0 && x <= p.X1 && y >= p.Y0 && y <= p.Y1
}

func inGapFill(x, y int, layout *Layout) bool {
	for _, s := range layout.Splits {
		if s.Axis == "x" {
			if x < s.Pos || x > s.Pos+CorridorWidth {
				continue
			}
			inOpening := false
			for _, c := range layout.Corridors {
				if c.Axis == "x" && c.X0 == s.Pos && c.Y0 <= y && y <= c.Y1 {
					inOpening = true
					break
				}
			}
			if !inOpening {
				return true
			}
		} else {
			if y < s.Pos || y > s.Pos+CorridorWidth {
				continue
			}
			inOpening := false
			for _, c := range layout.Corridors {
				if c.Axis == "y" && c.Y0 == s.Pos && c.X0 <= x && x <= c.X1 {
					inOpening = true
					break
				}
			}
			if !inOpening {
				return true
			}
		}
	}
	return false
}

// --- Full procgen pipeline ---

const maxAttempts = 10

func GenerateProceduralLayout(rng *rand.Rand, arenaSize, maxDepth int) *Layout {
	half := arenaSize / 2

	var bestLayout *Layout
	bestUnreachable := 1 << 30

	for attempt := range maxAttempts {
		attemptRng := rand.New(rand.NewPCG(rng.Uint64()+uint64(attempt), 0))
		rooms, splits := Subdivide(attemptRng, -half, -half, half, half, 0, maxDepth)
		corridors := ConnectRooms(attemptRng, rooms)
		ConstrainElevations(rooms, corridors)
		pools := GeneratePools(attemptRng, rooms)

		layout := &Layout{
			Rooms:     rooms,
			Corridors: corridors,
			Splits:    splits,
			Pools:     pools,
		}

		nav := ValidateNavigation(layout)
		if nav.Connected {
			bestLayout = layout
			break
		}
		if len(nav.UnreachableRooms) < bestUnreachable {
			bestUnreachable = len(nav.UnreachableRooms)
			bestLayout = layout
		}
	}

	return bestLayout
}

func RunProcgen(rng *rand.Rand, arenaSize, maxDepth int) (*MapFile, *Layout) {
	layout := GenerateProceduralLayout(rng, arenaSize, maxDepth)

	nav := ValidateNavigation(layout)
	status := "connected"
	if !nav.Connected {
		status = fmt.Sprintf("%d unreachable", len(nav.UnreachableRooms))
	}

	// Pick a random theme.
	themes := []*Theme{&ThemeTech, &ThemeCastle}
	theme := themes[rng.IntN(len(themes))]
	fmt.Printf("rooms=%d  corridors=%d  %s  theme=%s\n",
		len(layout.Rooms), len(layout.Corridors), status, theme.Name)

	m := NewMapFile()
	m.Worldspawn.Properties["message"] = "procgen"

	gi := procgenGridInfo(layout)
	zLo, zHi := computeZRange(layout)

	buildShell(m, gi, zLo, zHi)
	BuildGapFills(m, layout, zLo, zHi)

	// Pick per-room materials from theme.
	roomMats := make([]RoomMaterials, len(layout.Rooms))
	for i := range layout.Rooms {
		roomMats[i] = PickRoomMaterials(rng, theme, "building")
	}

	// Hallway materials for corridors.
	hallMat := PickRoomMaterials(rng, theme, "hallway")

	for i := range layout.Rooms {
		p, hasPool := layout.Pools[i]
		var pp *Pool
		if hasPool {
			pp = &p
		}
		BuildRoomBrushesThemed(m, &layout.Rooms[i], pp, &roomMats[i])
	}
	for i := range layout.Corridors {
		BuildCorridorBrushesThemed(m, &layout.Corridors[i], layout.Rooms, &hallMat)
	}

	PopulateDM(m, layout, rng)

	return m, layout
}

// procgenGridInfo builds a minimal GridInfo from a procgen layout
// so buildShell can compute the outer bounds.
func procgenGridInfo(layout *Layout) *GridInfo {
	minX, maxX := layout.Rooms[0].X0, layout.Rooms[0].X1
	minY, maxY := layout.Rooms[0].Y0, layout.Rooms[0].Y1
	for _, r := range layout.Rooms {
		if r.X0 < minX { minX = r.X0 }
		if r.X1 > maxX { maxX = r.X1 }
		if r.Y0 < minY { minY = r.Y0 }
		if r.Y1 > maxY { maxY = r.Y1 }
	}
	return &GridInfo{
		ColX0:      []int{minX},
		ColWidths:  []int{maxX - minX},
		RowY0:      []int{minY},
		RowHeights: []int{maxY - minY},
		NCols:      1,
		NRows:      1,
	}
}
