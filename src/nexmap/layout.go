package main

// Layout constants (Quake units).
const (
	Wall          = 16
	Floor         = 16
	CorridorWidth = 96
	MinRoomSize   = 256
	MaxRoomSize   = 1024

	FloorElevMin  = -128
	FloorElevMax  = 384
	FloorElevStep = 16

	CorridorHeadroom = 112
	WalkableHeight   = 56
	WalkableClimb    = 18
	WalkableRadius   = 16

	StairDepth   = 16
	StairHeight  = 8
	MaxStairRise = 96
	MinStepRun   = 16

	PoolDepth  = 48
	PoolBorder = 64
	PoolWall   = 16
)

var CeilingHeights = []int{128, 160, 192, 224, 256, 320}

// Room is a rectangular open space.
type Room struct {
	X0, Y0, X1, Y1 int
	Z0, Z1          int
}

func (r *Room) CX() int     { return (r.X0 + r.X1) / 2 }
func (r *Room) CY() int     { return (r.Y0 + r.Y1) / 2 }
func (r *Room) Width() int  { return r.X1 - r.X0 }
func (r *Room) Height() int { return r.Y1 - r.Y0 }

// Pool is a hazard recessed into a room floor.
type Pool struct {
	X0, Y0, X1, Y1 int
	Texture         string
}

// Corridor connects two rooms through a shared wall.
type Corridor struct {
	RoomA, RoomB    int
	X0, Y0, X1, Y1 int
	Z0, Z1          int
	Z0A, Z0B        int
	Axis             string // "x" or "y"
}

// Split records a BSP subdivision gap.
type Split struct {
	Axis         string
	Pos          int
	PerpLo       int
	PerpHi       int
}

// Layout holds the complete map topology.
type Layout struct {
	Rooms     []Room
	Corridors []Corridor
	Splits    []Split
	Pools     map[int]Pool // room index -> Pool
}

// --- Geometry builders ---

func BuildRoomBrushes(m *MapFile, room *Room, pool *Pool) {
	BuildRoomBrushesThemed(m, room, pool, nil)
}

func BuildRoomBrushesThemed(m *MapFile, room *Room, pool *Pool, mat *RoomMaterials) {
	x0, y0, x1, y1 := room.X0, room.Y0, room.X1, room.Y1
	z0, z1 := room.Z0, room.Z1
	floorTex := Textures.Floor
	ceilTex := Textures.Ceiling
	if mat != nil {
		floorTex = mat.Floor
		ceilTex = mat.Ceiling
	}

	// Ceiling.
	m.AddBrush(AxisAlignedBox(x0, y0, z1, x1, y1, z1+Floor, ceilTex))

	if pool == nil {
		m.AddBrush(AxisAlignedBox(x0, y0, z0-Floor, x1, y1, z0, floorTex))
		return
	}

	p := pool
	if p.Y0 > y0 {
		m.AddBrush(AxisAlignedBox(x0, y0, z0-Floor, x1, p.Y0, z0, floorTex))
	}
	if p.Y1 < y1 {
		m.AddBrush(AxisAlignedBox(x0, p.Y1, z0-Floor, x1, y1, z0, floorTex))
	}
	if p.X0 > x0 {
		m.AddBrush(AxisAlignedBox(x0, p.Y0, z0-Floor, p.X0, p.Y1, z0, floorTex))
	}
	if p.X1 < x1 {
		m.AddBrush(AxisAlignedBox(p.X1, p.Y0, z0-Floor, x1, p.Y1, z0, floorTex))
	}

	// Pool pit.
	pitBot := z0 - PoolDepth
	m.AddBrush(AxisAlignedBox(p.X0, p.Y0, pitBot-Floor, p.X1, p.Y1, pitBot, floorTex))
	pw := PoolWall
	m.AddBrush(AxisAlignedBox(p.X0-pw, p.Y0-pw, pitBot, p.X0, p.Y1+pw, z0, floorTex))
	m.AddBrush(AxisAlignedBox(p.X1, p.Y0-pw, pitBot, p.X1+pw, p.Y1+pw, z0, floorTex))
	m.AddBrush(AxisAlignedBox(p.X0, p.Y0-pw, pitBot, p.X1, p.Y0, z0, floorTex))
	m.AddBrush(AxisAlignedBox(p.X0, p.Y1, pitBot, p.X1, p.Y1+pw, z0, floorTex))
	// Liquid brush.
	m.AddBrush(AxisAlignedBox(p.X0, p.Y0, pitBot, p.X1, p.Y1, z0, p.Texture))
}

func BuildCorridorBrushes(m *MapFile, c *Corridor, rooms []Room) {
	BuildCorridorBrushesThemed(m, c, rooms, nil)
}

func BuildCorridorBrushesThemed(m *MapFile, c *Corridor, rooms []Room, mat *RoomMaterials) {
	floorTex := Textures.Floor
	ceilTex := Textures.Ceiling
	if mat != nil {
		floorTex = mat.Floor
		ceilTex = mat.Ceiling
	}

	m.AddBrush(AxisAlignedBox(c.X0, c.Y0, c.Z0-Floor, c.X1, c.Y1, c.Z0, floorTex))
	m.AddBrush(AxisAlignedBox(c.X0, c.Y0, c.Z1, c.X1, c.Y1, c.Z1+Floor, ceilTex))

	buildStairs(m, c, floorTex)
	buildThreshold(m, c, rooms, floorTex)
}

func buildStairs(m *MapFile, c *Corridor, tex string) {
	dz := c.Z0B - c.Z0A
	if dz == 0 {
		return
	}

	bridged := abs(dz)
	if bridged > MaxStairRise {
		bridged = MaxStairRise
	}

	totalRun := c.X1 - c.X0
	if c.Axis == "y" {
		totalRun = c.Y1 - c.Y0
	}

	maxSteps := totalRun / MinStepRun
	minSteps := (bridged + WalkableClimb - 1) / WalkableClimb
	nSteps := max(minSteps, min(bridged/StairHeight, maxSteps))
	if nSteps == 0 || nSteps > maxSteps {
		return
	}

	rises := make([]int, nSteps)
	base := bridged / nSteps
	remainder := bridged - base*nSteps
	for i := range nSteps {
		rises[i] = base
		if i < remainder {
			rises[i]++
		}
	}

	baseZ := c.Z0A
	slabBot := c.Z0 - Floor
	stepRun := totalRun / nSteps
	cumRise := 0

	for i := range nSteps {
		runLo := i * stepRun
		runHi := (i + 1) * stepRun
		if i == nSteps-1 {
			runHi = totalRun
		}

		var slabTop int
		if dz > 0 {
			cumRise += rises[i]
			slabTop = baseZ + cumRise
		} else {
			slabTop = baseZ - cumRise
			cumRise += rises[i]
		}

		if c.Axis == "x" {
			m.AddBrush(AxisAlignedBox(c.X0+runLo, c.Y0, slabBot, c.X0+runHi, c.Y1, slabTop, tex))
		} else {
			m.AddBrush(AxisAlignedBox(c.X0, c.Y0+runLo, slabBot, c.X1, c.Y0+runHi, slabTop, tex))
		}
	}
}

func buildThreshold(m *MapFile, c *Corridor, rooms []Room, tex string) {
	if c.Z0A != c.Z0B {
		return
	}
	type end struct {
		roomIdx int
		roomZ0  int
	}
	ends := []end{{c.RoomA, c.Z0A}, {c.RoomB, c.Z0B}}
	for _, e := range ends {
		if e.roomZ0 <= c.Z0 {
			continue
		}
		if c.Axis == "x" {
			midX := (c.X0 + c.X1) / 2
			if e.roomIdx == c.RoomA {
				m.AddBrush(AxisAlignedBox(c.X0, c.Y0, c.Z0, midX, c.Y1, e.roomZ0, tex))
			} else {
				m.AddBrush(AxisAlignedBox(midX, c.Y0, c.Z0, c.X1, c.Y1, e.roomZ0, tex))
			}
		} else {
			midY := (c.Y0 + c.Y1) / 2
			if e.roomIdx == c.RoomA {
				m.AddBrush(AxisAlignedBox(c.X0, c.Y0, c.Z0, c.X1, midY, e.roomZ0, tex))
			} else {
				m.AddBrush(AxisAlignedBox(c.X0, midY, c.Z0, c.X1, c.Y1, e.roomZ0, tex))
			}
		}
	}
}

// BuildGapFills fills BSP subdivision gaps with solid, cutting corridor holes.
func BuildGapFills(m *MapFile, layout *Layout, zLo, zHi int) {
	tex := Textures.Fill
	for _, split := range layout.Splits {
		if split.Axis == "x" {
			var corrs []Corridor
			for _, c := range layout.Corridors {
				if c.Axis == "x" && c.X0 == split.Pos {
					corrs = append(corrs, c)
				}
			}
			fillGapX(m, split, corrs, zLo, zHi, tex)
		} else {
			var corrs []Corridor
			for _, c := range layout.Corridors {
				if c.Axis == "y" && c.Y0 == split.Pos {
					corrs = append(corrs, c)
				}
			}
			fillGapY(m, split, corrs, zLo, zHi, tex)
		}
	}
}

type opening struct {
	lo, hi, z0, z1 int
}

func fillGapX(m *MapFile, split Split, corridors []Corridor, zLo, zHi int, tex string) {
	gx0 := split.Pos
	gx1 := split.Pos + CorridorWidth

	var opens []opening
	for _, c := range corridors {
		opens = append(opens, opening{c.Y0, c.Y1, c.Z0, c.Z1})
	}
	sortOpenings(opens)

	cursor := split.PerpLo
	for _, o := range opens {
		if o.lo > cursor {
			m.AddBrush(AxisAlignedBox(gx0, cursor, zLo, gx1, o.lo, zHi, tex))
		}
		if o.z0 > zLo {
			m.AddBrush(AxisAlignedBox(gx0, o.lo, zLo, gx1, o.hi, o.z0, tex))
		}
		if o.z1 < zHi {
			m.AddBrush(AxisAlignedBox(gx0, o.lo, o.z1, gx1, o.hi, zHi, tex))
		}
		cursor = o.hi
	}
	if cursor < split.PerpHi {
		m.AddBrush(AxisAlignedBox(gx0, cursor, zLo, gx1, split.PerpHi, zHi, tex))
	}
}

func fillGapY(m *MapFile, split Split, corridors []Corridor, zLo, zHi int, tex string) {
	gy0 := split.Pos
	gy1 := split.Pos + CorridorWidth

	var opens []opening
	for _, c := range corridors {
		opens = append(opens, opening{c.X0, c.X1, c.Z0, c.Z1})
	}
	sortOpenings(opens)

	cursor := split.PerpLo
	for _, o := range opens {
		if o.lo > cursor {
			m.AddBrush(AxisAlignedBox(cursor, gy0, zLo, o.lo, gy1, zHi, tex))
		}
		if o.z0 > zLo {
			m.AddBrush(AxisAlignedBox(o.lo, gy0, zLo, o.hi, gy1, o.z0, tex))
		}
		if o.z1 < zHi {
			m.AddBrush(AxisAlignedBox(o.lo, gy0, o.z1, o.hi, gy1, zHi, tex))
		}
		cursor = o.hi
	}
	if cursor < split.PerpHi {
		m.AddBrush(AxisAlignedBox(cursor, gy0, zLo, split.PerpHi, gy1, zHi, tex))
	}
}

func sortOpenings(o []opening) {
	// Simple insertion sort — small slices.
	for i := 1; i < len(o); i++ {
		for j := i; j > 0 && o[j].lo < o[j-1].lo; j-- {
			o[j], o[j-1] = o[j-1], o[j]
		}
	}
}

// ConstrainElevations clamps floor heights so all corridors are navigable.
func ConstrainElevations(rooms []Room, corridors []Corridor) {
	maxStepsPerCorridor := CorridorWidth / MinStepRun
	maxClimbable := maxStepsPerCorridor * WalkableClimb
	maxRise := min(MaxStairRise, maxClimbable)

	maxIters := len(rooms) * 4
	for iter := range maxIters {
		_ = iter
		changed := false
		for _, c := range corridors {
			a := &rooms[c.RoomA]
			b := &rooms[c.RoomB]
			dz := a.Z0 - b.Z0
			if abs(dz) <= maxRise {
				continue
			}
			excess := abs(dz) - maxRise
			half := excess / 2
			rest := excess - half
			headA := a.Z1 - a.Z0
			headB := b.Z1 - b.Z0
			if dz > 0 {
				a.Z0 -= half
				a.Z1 = a.Z0 + headA
				b.Z0 += rest
				b.Z1 = b.Z0 + headB
			} else {
				a.Z0 += half
				a.Z1 = a.Z0 + headA
				b.Z0 -= rest
				b.Z1 = b.Z0 + headB
			}
			changed = true
		}
		if !changed {
			break
		}
	}

	// Final enforcement.
	for _, c := range corridors {
		a := &rooms[c.RoomA]
		b := &rooms[c.RoomB]
		dz := abs(a.Z0 - b.Z0)
		if dz > maxRise {
			high, low := a, b
			if a.Z0 < b.Z0 {
				high, low = b, a
			}
			headroom := high.Z1 - high.Z0
			high.Z0 = low.Z0 + maxRise
			high.Z1 = high.Z0 + headroom
		}
	}

	// Raise ceilings for headroom.
	for _, c := range corridors {
		a := &rooms[c.RoomA]
		b := &rooms[c.RoomB]
		highZ := max(a.Z0, b.Z0)
		minCeil := highZ + CorridorHeadroom
		if a.Z1 < minCeil {
			a.Z1 = minCeil
		}
		if b.Z1 < minCeil {
			b.Z1 = minCeil
		}
	}

	// Update corridor z-values.
	for i := range corridors {
		c := &corridors[i]
		a := &rooms[c.RoomA]
		b := &rooms[c.RoomB]
		c.Z0A = a.Z0
		c.Z0B = b.Z0
		c.Z0 = min(a.Z0, b.Z0)
		highZ := max(a.Z0, b.Z0)
		c.Z1 = min(highZ+CorridorHeadroom, a.Z1, b.Z1)
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
