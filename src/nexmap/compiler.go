package main

import "math/rand/v2"

// GridInfo holds the grid layout metadata.
type GridInfo struct {
	ColX0      []int
	ColWidths  []int
	RowY0      []int
	RowHeights []int
	CellMap    map[[2]int]string // [col,row] -> room ID
	NCols      int
	NRows      int
}

func (g *GridInfo) GlobalX0() int { return g.ColX0[0] }
func (g *GridInfo) GlobalX1() int { return g.ColX0[g.NCols-1] + g.ColWidths[g.NCols-1] }
func (g *GridInfo) GlobalY0() int { return g.RowY0[0] }
func (g *GridInfo) GlobalY1() int { return g.RowY0[g.NRows-1] + g.RowHeights[g.NRows-1] }

// --- Grid assignment ---

var dirs = [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

func physicalAdj(bp *ResolvedBlueprint) map[string][]string {
	adj := map[string][]string{}
	for _, c := range bp.Connections {
		if c.Type == "teleporter" {
			continue
		}
		adj[c.FromID] = append(adj[c.FromID], c.ToID)
		if c.Bidirectional {
			adj[c.ToID] = append(adj[c.ToID], c.FromID)
		}
	}
	return adj
}

func assignGrid(bp *ResolvedBlueprint) map[string][2]int {
	adj := physicalAdj(bp)

	placed := map[string][2]int{}
	occupied := map[[2]int]bool{}

	start := bp.Rooms[0].ID
	placed[start] = [2]int{0, 0}
	occupied[[2]int{0, 0}] = true

	queue := []string{start}
	visited := map[string]bool{start: true}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		cc, cr := placed[current][0], placed[current][1]

		for _, nbr := range adj[current] {
			if _, ok := placed[nbr]; ok {
				continue
			}
			if visited[nbr] {
				continue
			}
			visited[nbr] = true

			// Gather candidate positions.
			candidates := map[[2]int]bool{}
			for _, adjID := range adj[nbr] {
				if pos, ok := placed[adjID]; ok {
					for _, d := range dirs {
						p := [2]int{pos[0] + d[0], pos[1] + d[1]}
						if !occupied[p] {
							candidates[p] = true
						}
					}
				}
			}
			if len(candidates) == 0 {
				for _, d := range dirs {
					p := [2]int{cc + d[0], cr + d[1]}
					if !occupied[p] {
						candidates[p] = true
					}
				}
			}

			// Score by adjacency count.
			bestPos := [2]int{cc + 1, cr}
			bestScore := -1
			for pos := range candidates {
				score := 0
				for _, adjID := range adj[nbr] {
					if apos, ok := placed[adjID]; ok {
						if abs(pos[0]-apos[0])+abs(pos[1]-apos[1]) == 1 {
							score++
						}
					}
				}
				if score > bestScore {
					bestScore = score
					bestPos = pos
				}
			}

			if bestScore < 0 {
				// Expand search.
				for radius := 1; radius < 20; radius++ {
					found := false
					for dc := -radius; dc <= radius; dc++ {
						for dr := -radius; dr <= radius; dr++ {
							p := [2]int{cc + dc, cr + dr}
							if !occupied[p] {
								bestPos = p
								found = true
								break
							}
						}
						if found {
							break
						}
					}
					if found {
						break
					}
				}
			}

			placed[nbr] = bestPos
			occupied[bestPos] = true
			queue = append(queue, nbr)
		}
	}

	return placed
}

func adjacencyAxis(a, b [2]int) string {
	dc := b[0] - a[0]
	dr := b[1] - a[1]
	if abs(dc) == 1 && dr == 0 {
		return "x"
	}
	if dc == 0 && abs(dr) == 1 {
		return "y"
	}
	return ""
}

// --- Compile ---

type CompileResult struct {
	Layout    *Layout
	IDToIdx   map[string]int
	TeleConns []ResolvedConnection
	Grid      *GridInfo
}

func CompileBlueprint(bp *ResolvedBlueprint, rng *rand.Rand) *CompileResult {
	grid := assignGrid(bp)

	// Normalise to (0,0).
	minC, minR := 1<<30, 1<<30
	for _, pos := range grid {
		if pos[0] < minC {
			minC = pos[0]
		}
		if pos[1] < minR {
			minR = pos[1]
		}
	}
	for k, v := range grid {
		grid[k] = [2]int{v[0] - minC, v[1] - minR}
	}

	maxC, maxR := 0, 0
	for _, pos := range grid {
		if pos[0] > maxC {
			maxC = pos[0]
		}
		if pos[1] > maxR {
			maxR = pos[1]
		}
	}
	nCols := maxC + 1
	nRows := maxR + 1

	roomMap := map[string]*ResolvedRoom{}
	for i := range bp.Rooms {
		roomMap[bp.Rooms[i].ID] = &bp.Rooms[i]
	}

	// Cell sizes.
	colWidths := make([]int, nCols)
	rowHeights := make([]int, nRows)
	for i := range colWidths {
		colWidths[i] = 256
	}
	for i := range rowHeights {
		rowHeights[i] = 256
	}

	for rid, pos := range grid {
		br := roomMap[rid]
		w := br.SizeMin + rng.IntN(br.SizeMax-br.SizeMin+1)
		h := br.SizeMin + rng.IntN(br.SizeMax-br.SizeMin+1)
		if br.Shape == "wide" {
			w = max(w, h*3/2)
			w = min(w, br.SizeMax)
		} else if br.Shape == "tall_narrow" {
			h = max(h, w*3/2)
			h = min(h, br.SizeMax)
		}
		if w > colWidths[pos[0]] {
			colWidths[pos[0]] = w
		}
		if h > rowHeights[pos[1]] {
			rowHeights[pos[1]] = h
		}
	}

	// World coordinates.
	colX0 := make([]int, nCols)
	x := 0
	for c := range nCols {
		colX0[c] = x
		x += colWidths[c]
		if c < nCols-1 {
			x += CorridorWidth
		}
	}
	rowY0 := make([]int, nRows)
	y := 0
	for r := range nRows {
		rowY0[r] = y
		y += rowHeights[r]
		if r < nRows-1 {
			y += CorridorWidth
		}
	}

	// Centre.
	xOff := -x / 2
	yOff := -y / 2
	for i := range colX0 {
		colX0[i] += xOff
	}
	for i := range rowY0 {
		rowY0[i] += yOff
	}

	cellMap := map[[2]int]string{}
	for rid, pos := range grid {
		cellMap[pos] = rid
	}

	gi := &GridInfo{
		ColX0: colX0, ColWidths: colWidths,
		RowY0: rowY0, RowHeights: rowHeights,
		CellMap: cellMap, NCols: nCols, NRows: nRows,
	}

	// Rooms.
	rooms := make([]Room, 0, len(bp.Rooms))
	idToIdx := map[string]int{}

	for rid, pos := range grid {
		br := roomMap[rid]
		rx0 := colX0[pos[0]]
		ry0 := rowY0[pos[1]]
		rx1 := rx0 + colWidths[pos[0]]
		ry1 := ry0 + rowHeights[pos[1]]

		elevSteps := (br.ElevationMax - br.ElevationMin) / FloorElevStep
		z0 := br.ElevationMin
		if elevSteps > 0 {
			z0 += rng.IntN(elevSteps+1) * FloorElevStep
		}
		z1 := z0 + br.CeilingHeight

		idToIdx[rid] = len(rooms)
		rooms = append(rooms, Room{X0: rx0, Y0: ry0, X1: rx1, Y1: ry1, Z0: z0, Z1: z1})
	}

	// Splits (localised per cell-pair).
	var splits []Split

	for c := range nCols - 1 {
		sp := colX0[c] + colWidths[c]
		for r := range nRows {
			left := cellMap[[2]int{c, r}]
			right := cellMap[[2]int{c + 1, r}]
			if left != "" || right != "" {
				splits = append(splits, Split{
					Axis: "x", Pos: sp,
					PerpLo: rowY0[r], PerpHi: rowY0[r] + rowHeights[r],
				})
			}
		}
	}
	for r := range nRows - 1 {
		sp := rowY0[r] + rowHeights[r]
		for c := range nCols {
			bot := cellMap[[2]int{c, r}]
			top := cellMap[[2]int{c, r + 1}]
			if bot != "" || top != "" {
				splits = append(splits, Split{
					Axis: "y", Pos: sp,
					PerpLo: colX0[c], PerpHi: colX0[c] + colWidths[c],
				})
			}
		}
	}

	// Corridors.
	var corridors []Corridor
	var teleConns []ResolvedConnection

	for _, conn := range bp.Connections {
		posA, okA := grid[conn.FromID]
		posB, okB := grid[conn.ToID]
		if !okA || !okB {
			continue
		}

		axis := adjacencyAxis(posA, posB)
		if axis == "" || conn.Type == "teleporter" {
			teleConns = append(teleConns, conn)
			continue
		}

		idxA := idToIdx[conn.FromID]
		idxB := idToIdx[conn.ToID]
		ra := &rooms[idxA]
		rb := &rooms[idxB]

		// Ensure a is the lower-coordinate room.
		if axis == "x" && ra.X1 > rb.X0 {
			ra, rb = rb, ra
			idxA, idxB = idxB, idxA
		} else if axis == "y" && ra.Y1 > rb.Y0 {
			ra, rb = rb, ra
			idxA, idxB = idxB, idxA
		}

		corrW := min(conn.Width, CorridorWidth)
		lowZ := min(ra.Z0, rb.Z0)
		highZ := max(ra.Z0, rb.Z0)
		ceilZ := min(highZ+CorridorHeadroom, ra.Z1, rb.Z1)

		if axis == "x" {
			ov0 := max(ra.Y0, rb.Y0)
			ov1 := min(ra.Y1, rb.Y1)
			var cy int
			if ov1-ov0 < corrW {
				cy = (ov0 + ov1) / 2
			} else {
				cy = ov0 + corrW/2 + rng.IntN(ov1-ov0-corrW+1)
			}
			corridors = append(corridors, Corridor{
				RoomA: idxA, RoomB: idxB,
				X0: ra.X1, Y0: cy - corrW/2, X1: rb.X0, Y1: cy + corrW/2,
				Z0: lowZ, Z1: ceilZ, Z0A: rooms[idxA].Z0, Z0B: rooms[idxB].Z0,
				Axis: "x",
			})
		} else {
			ov0 := max(ra.X0, rb.X0)
			ov1 := min(ra.X1, rb.X1)
			var cx int
			if ov1-ov0 < corrW {
				cx = (ov0 + ov1) / 2
			} else {
				cx = ov0 + corrW/2 + rng.IntN(ov1-ov0-corrW+1)
			}
			corridors = append(corridors, Corridor{
				RoomA: idxA, RoomB: idxB,
				X0: cx - corrW/2, Y0: ra.Y1, X1: cx + corrW/2, Y1: rb.Y0,
				Z0: lowZ, Z1: ceilZ, Z0A: rooms[idxA].Z0, Z0B: rooms[idxB].Z0,
				Axis: "y",
			})
		}
	}

	ConstrainElevations(rooms, corridors)

	// Pools.
	pools := map[int]Pool{}
	for _, br := range bp.Rooms {
		if br.Hazard == nil {
			continue
		}
		idx, ok := idToIdx[br.ID]
		if !ok {
			continue
		}
		room := &rooms[idx]
		if room.Width() < 2*PoolBorder+96 || room.Height() < 2*PoolBorder+96 {
			continue
		}
		maxPW := min(256, room.Width()-2*PoolBorder)
		maxPH := min(256, room.Height()-2*PoolBorder)
		pw := max(96, int(float64(maxPW)*br.Hazard.Coverage))
		ph := max(96, int(float64(maxPH)*br.Hazard.Coverage))
		px := room.X0 + (room.Width()-pw)/2
		py := room.Y0 + (room.Height()-ph)/2
		pools[idx] = Pool{X0: px, Y0: py, X1: px + pw, Y1: py + ph, Texture: br.Hazard.Texture}
	}

	layout := &Layout{
		Rooms: rooms, Corridors: corridors, Splits: splits, Pools: pools,
	}

	return &CompileResult{
		Layout: layout, IDToIdx: idToIdx, TeleConns: teleConns, Grid: gi,
	}
}

// --- Geometry building ---

func computeZRange(layout *Layout) (int, int) {
	zLo := layout.Rooms[0].Z0 - Floor
	zHi := layout.Rooms[0].Z1 + Floor
	for _, r := range layout.Rooms {
		zLo = min(zLo, r.Z0-Floor)
		zHi = max(zHi, r.Z1+Floor)
	}
	for _, c := range layout.Corridors {
		zLo = min(zLo, c.Z0-Floor)
		zHi = max(zHi, c.Z1+Floor)
	}
	for idx, p := range layout.Pools {
		_ = p
		pitBot := layout.Rooms[idx].Z0 - PoolDepth - Floor
		zLo = min(zLo, pitBot)
	}
	return zLo, zHi
}

func buildShell(m *MapFile, gi *GridInfo, zLo, zHi int) {
	x0 := gi.GlobalX0()
	x1 := gi.GlobalX1()
	y0 := gi.GlobalY0()
	y1 := gi.GlobalY1()
	s := 32
	tex := Textures.Shell

	m.AddBrush(AxisAlignedBox(x0-s, y0-s, zLo-s, x1+s, y1+s, zLo, tex))
	m.AddBrush(AxisAlignedBox(x0-s, y0-s, zHi, x1+s, y1+s, zHi+s, tex))
	m.AddBrush(AxisAlignedBox(x0-s, y0-s, zLo, x0, y1+s, zHi, tex))
	m.AddBrush(AxisAlignedBox(x1, y0-s, zLo, x1+s, y1+s, zHi, tex))
	m.AddBrush(AxisAlignedBox(x0, y0-s, zLo, x1, y0, zHi, tex))
	m.AddBrush(AxisAlignedBox(x0, y1, zLo, x1, y1+s, zHi, tex))
}

func buildEmptyCellFills(m *MapFile, gi *GridInfo, zLo, zHi int) {
	tex := Textures.Fill
	for c := range gi.NCols {
		for r := range gi.NRows {
			if _, ok := gi.CellMap[[2]int{c, r}]; ok {
				continue
			}
			x0 := gi.ColX0[c]
			y0 := gi.RowY0[r]
			m.AddBrush(AxisAlignedBox(x0, y0, zLo, x0+gi.ColWidths[c], y0+gi.RowHeights[r], zHi, tex))
		}
	}
}

func buildIntersectionFills(m *MapFile, gi *GridInfo, zLo, zHi int) {
	tex := Textures.Fill
	for c := range gi.NCols - 1 {
		for r := range gi.NRows - 1 {
			ix0 := gi.ColX0[c] + gi.ColWidths[c]
			iy0 := gi.RowY0[r] + gi.RowHeights[r]
			m.AddBrush(AxisAlignedBox(ix0, iy0, zLo, ix0+CorridorWidth, iy0+CorridorWidth, zHi, tex))
		}
	}
}

func BuildBlueprintGeometry(m *MapFile, layout *Layout, gi *GridInfo) {
	BuildBlueprintGeometryThemed(m, layout, gi, nil, nil, nil)
}

func BuildBlueprintGeometryThemed(m *MapFile, layout *Layout, gi *GridInfo, th *Theme, roomEnvs []string, roomDetails [][]Detail) {
	zLo, zHi := computeZRange(layout)

	buildShell(m, gi, zLo, zHi)
	BuildGapFills(m, layout, zLo, zHi)
	buildIntersectionFills(m, gi, zLo, zHi)
	buildEmptyCellFills(m, gi, zLo, zHi)

	// Use a deterministic RNG for material selection seeded from room count.
	matRng := rand.New(rand.NewPCG(uint64(len(layout.Rooms)*31), 0))

	for i := range layout.Rooms {
		p, hasPool := layout.Pools[i]
		var pp *Pool
		if hasPool {
			pp = &p
		}

		var mat *RoomMaterials
		if th != nil {
			env := "building"
			if roomEnvs != nil && i < len(roomEnvs) {
				env = roomEnvs[i]
			}
			rm := PickRoomMaterials(matRng, th, env)
			mat = &rm
		}

		BuildRoomBrushesThemed(m, &layout.Rooms[i], pp, mat)

		// Place details if specified.
		if roomDetails != nil && i < len(roomDetails) {
			for _, d := range roomDetails[i] {
				rm := RoomMaterials{Wall: Textures.Shell, Floor: Textures.Floor, Ceiling: Textures.Ceiling}
				if mat != nil {
					rm = *mat
				}
				PlaceDetail(m, &layout.Rooms[i], pp, d, &rm, matRng)
			}
		}
	}

	hallMat := (*RoomMaterials)(nil)
	if th != nil {
		hm := PickRoomMaterials(matRng, th, "hallway")
		hallMat = &hm
	}
	for i := range layout.Corridors {
		BuildCorridorBrushesThemed(m, &layout.Corridors[i], layout.Rooms, hallMat)
	}
}
