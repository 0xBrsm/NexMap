package main

import (
	"bufio"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// --- .map file parser ---

// ParsedPlane is a single face definition from a .map file.
type ParsedPlane struct {
	P1, P2, P3    [3]float64
	Texture       string
	XOff, YOff    float64
	Rotation      float64
	XScale,YScale float64
}

// ParsedBrush is a convex solid from a .map file.
type ParsedBrush struct {
	Planes []ParsedPlane
}

// ParsedEntity is an entity block from a .map file.
type ParsedEntity struct {
	Properties map[string]string
	Brushes    []ParsedBrush
}

// ParseMapFile reads a Quake .map file and returns all entities.
func ParseMapFile(path string) ([]ParsedEntity, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entities []ParsedEntity
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "{" {
			ent, err := parseEntity(scanner)
			if err != nil {
				return nil, err
			}
			entities = append(entities, ent)
		}
	}
	return entities, scanner.Err()
}

func parseEntity(scanner *bufio.Scanner) (ParsedEntity, error) {
	ent := ParsedEntity{Properties: map[string]string{}}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "}" {
			return ent, nil
		}
		if line == "{" {
			brush, err := parseBrush(scanner)
			if err != nil {
				return ent, err
			}
			ent.Brushes = append(ent.Brushes, brush)
			continue
		}
		// Key-value pair: "key" "value" (separated by space or tab)
		if strings.HasPrefix(line, "\"") {
			// Find first closing quote, then skip whitespace to find opening quote of value.
			endKey := strings.Index(line[1:], "\"")
			if endKey >= 0 {
				key := line[1 : 1+endKey]
				rest := strings.TrimSpace(line[1+endKey+1:])
				if strings.HasPrefix(rest, "\"") && strings.HasSuffix(rest, "\"") {
					val := rest[1 : len(rest)-1]
					ent.Properties[key] = val
				}
			}
		}
	}
	return ent, fmt.Errorf("unexpected EOF in entity")
}

func parseBrush(scanner *bufio.Scanner) (ParsedBrush, error) {
	var brush ParsedBrush
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "}" {
			return brush, nil
		}
		if line == "" || line[0] == '/' {
			continue
		}
		plane, err := parsePlaneLine(line)
		if err != nil {
			continue // skip malformed lines
		}
		brush.Planes = append(brush.Planes, plane)
	}
	return brush, fmt.Errorf("unexpected EOF in brush")
}

// parsePlaneLine parses: ( x y z ) ( x y z ) ( x y z ) TEXTURE xoff yoff rot xscale yscale
func parsePlaneLine(line string) (ParsedPlane, error) {
	var p ParsedPlane

	// Extract three point groups between ( )
	points := make([][3]float64, 0, 3)
	for i := 0; i < 3; i++ {
		open := strings.Index(line, "(")
		close := strings.Index(line, ")")
		if open < 0 || close < 0 || close <= open {
			return p, fmt.Errorf("bad plane: %s", line)
		}
		coords := strings.Fields(line[open+1 : close])
		if len(coords) < 3 {
			return p, fmt.Errorf("bad coords: %s", line)
		}
		var pt [3]float64
		for j := 0; j < 3; j++ {
			pt[j], _ = strconv.ParseFloat(coords[j], 64)
		}
		points = append(points, pt)
		line = line[close+1:]
	}

	p.P1, p.P2, p.P3 = points[0], points[1], points[2]

	// Remaining: TEXTURE xoff yoff rot xscale yscale
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) >= 1 {
		p.Texture = fields[0]
	}
	if len(fields) >= 2 { p.XOff, _ = strconv.ParseFloat(fields[1], 64) }
	if len(fields) >= 3 { p.YOff, _ = strconv.ParseFloat(fields[2], 64) }
	if len(fields) >= 4 { p.Rotation, _ = strconv.ParseFloat(fields[3], 64) }
	if len(fields) >= 5 { p.XScale, _ = strconv.ParseFloat(fields[4], 64) }
	if len(fields) >= 6 { p.YScale, _ = strconv.ParseFloat(fields[5], 64) }

	return p, nil
}

// --- Brush bounding box computation ---

// BrushBounds computes the axis-aligned bounding box of a brush
// by intersecting its half-planes. Uses a simplified approach:
// sample the plane intersection points.
type BBox struct {
	MinX, MinY, MinZ float64
	MaxX, MaxY, MaxZ float64
}

func (b BBox) CenterX() float64 { return (b.MinX + b.MaxX) / 2 }
func (b BBox) CenterY() float64 { return (b.MinY + b.MaxY) / 2 }
func (b BBox) CenterZ() float64 { return (b.MinZ + b.MaxZ) / 2 }
func (b BBox) Width() float64   { return b.MaxX - b.MinX }
func (b BBox) Height() float64  { return b.MaxY - b.MinY }
func (b BBox) Depth() float64   { return b.MaxZ - b.MinZ }

func BrushBounds(b *ParsedBrush) BBox {
	// Collect all vertices by intersecting every triple of planes.
	var verts [][3]float64

	planes := make([]struct{ n [3]float64; d float64 }, len(b.Planes))
	for i, pp := range b.Planes {
		// Normal = (p2-p1) x (p3-p1)
		u := [3]float64{pp.P2[0] - pp.P1[0], pp.P2[1] - pp.P1[1], pp.P2[2] - pp.P1[2]}
		v := [3]float64{pp.P3[0] - pp.P1[0], pp.P3[1] - pp.P1[1], pp.P3[2] - pp.P1[2]}
		n := [3]float64{
			u[1]*v[2] - u[2]*v[1],
			u[2]*v[0] - u[0]*v[2],
			u[0]*v[1] - u[1]*v[0],
		}
		planes[i].n = n
		planes[i].d = n[0]*pp.P1[0] + n[1]*pp.P1[1] + n[2]*pp.P1[2]
	}

	np := len(planes)
	for i := 0; i < np-2; i++ {
		for j := i + 1; j < np-1; j++ {
			for k := j + 1; k < np; k++ {
				pt, ok := intersect3Planes(
					planes[i].n, planes[i].d,
					planes[j].n, planes[j].d,
					planes[k].n, planes[k].d,
				)
				if !ok {
					continue
				}
				// Check point is inside all planes (with tolerance).
				inside := true
				for m := range planes {
					dot := planes[m].n[0]*pt[0] + planes[m].n[1]*pt[1] + planes[m].n[2]*pt[2]
					if dot > planes[m].d+0.1 {
						inside = false
						break
					}
				}
				if inside {
					verts = append(verts, pt)
				}
			}
		}
	}

	if len(verts) == 0 {
		// Fallback: use plane definition points.
		for _, pp := range b.Planes {
			verts = append(verts, pp.P1, pp.P2, pp.P3)
		}
	}

	bb := BBox{
		MinX: math.MaxFloat64, MinY: math.MaxFloat64, MinZ: math.MaxFloat64,
		MaxX: -math.MaxFloat64, MaxY: -math.MaxFloat64, MaxZ: -math.MaxFloat64,
	}
	for _, v := range verts {
		bb.MinX = min(bb.MinX, v[0])
		bb.MinY = min(bb.MinY, v[1])
		bb.MinZ = min(bb.MinZ, v[2])
		bb.MaxX = max(bb.MaxX, v[0])
		bb.MaxY = max(bb.MaxY, v[1])
		bb.MaxZ = max(bb.MaxZ, v[2])
	}
	return bb
}

func intersect3Planes(n1 [3]float64, d1 float64, n2 [3]float64, d2 float64, n3 [3]float64, d3 float64) ([3]float64, bool) {
	// Cramer's rule for 3x3 system.
	det := n1[0]*(n2[1]*n3[2]-n2[2]*n3[1]) -
		n1[1]*(n2[0]*n3[2]-n2[2]*n3[0]) +
		n1[2]*(n2[0]*n3[1]-n2[1]*n3[0])
	if math.Abs(det) < 1e-10 {
		return [3]float64{}, false
	}
	x := (d1*(n2[1]*n3[2]-n2[2]*n3[1]) - n1[1]*(d2*n3[2]-n2[2]*d3) + n1[2]*(d2*n3[1]-n2[1]*d3)) / det
	y := (n1[0]*(d2*n3[2]-n2[2]*d3) - d1*(n2[0]*n3[2]-n2[2]*n3[0]) + n1[2]*(n2[0]*d3-d2*n3[0])) / det
	z := (n1[0]*(n2[1]*d3-d2*n3[1]) - n1[1]*(n2[0]*d3-d2*n3[0]) + d1*(n2[0]*n3[1]-n2[1]*n3[0])) / det
	return [3]float64{x, y, z}, true
}

// --- Spatial clustering of brushes into rooms ---

// RoomTemplate is a group of brushes extracted from a source .map that
// form a room-scale space, relocatable to any position.
type RoomTemplate struct {
	Source     string        // source map name
	Index      int           // room index within source
	Brushes    []ParsedBrush // original brush geometry
	Bounds     BBox          // bounding box of all brushes
	Textures   []string      // unique textures used
	FloorZ     float64       // lowest floor level
	CeilZ      float64       // highest ceiling level
}

// Width/Height in Quake XY plane.
func (r *RoomTemplate) Width() float64  { return r.Bounds.MaxX - r.Bounds.MinX }
func (r *RoomTemplate) Height() float64 { return r.Bounds.MaxY - r.Bounds.MinY }

// ExtractRoomTemplates parses a .map file and clusters worldspawn brushes
// into room-scale groups using spatial proximity flood-fill.
func ExtractRoomTemplates(path string, mapName string) ([]RoomTemplate, error) {
	entities, err := ParseMapFile(path)
	if err != nil {
		return nil, err
	}

	// Find worldspawn entity.
	var worldspawn *ParsedEntity
	for i := range entities {
		if entities[i].Properties["classname"] == "worldspawn" {
			worldspawn = &entities[i]
			break
		}
	}
	if worldspawn == nil {
		return nil, fmt.Errorf("no worldspawn in %s", path)
	}

	// Compute bounding box for each brush.
	type brushInfo struct {
		brush *ParsedBrush
		bb    BBox
		idx   int
	}

	var brushes []brushInfo
	for i := range worldspawn.Brushes {
		bb := BrushBounds(&worldspawn.Brushes[i])
		// Skip degenerate brushes.
		if bb.Width() < 1 && bb.Height() < 1 && bb.Depth() < 1 {
			continue
		}
		brushes = append(brushes, brushInfo{
			brush: &worldspawn.Brushes[i],
			bb:    bb,
			idx:   i,
		})
	}

	fmt.Printf("  %s: %d worldspawn brushes\n", mapName, len(brushes))

	// Flood-fill cluster brushes by spatial overlap/adjacency.
	// Two brushes are neighbors if their bounding boxes overlap or are
	// within 32 units of each other (touching walls).
	const proximity = 32.0
	cluster := make([]int, len(brushes))
	for i := range cluster {
		cluster[i] = -1
	}

	adjacent := func(a, b BBox) bool {
		// Check if bounding boxes overlap or are within proximity on all axes.
		return a.MinX <= b.MaxX+proximity && a.MaxX >= b.MinX-proximity &&
			a.MinY <= b.MaxY+proximity && a.MaxY >= b.MinY-proximity &&
			a.MinZ <= b.MaxZ+proximity && a.MaxZ >= b.MinZ-proximity
	}

	clusterID := 0
	for i := range brushes {
		if cluster[i] >= 0 {
			continue
		}
		cluster[i] = clusterID
		queue := []int{i}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for j := range brushes {
				if cluster[j] >= 0 {
					continue
				}
				if adjacent(brushes[cur].bb, brushes[j].bb) {
					cluster[j] = clusterID
					queue = append(queue, j)
				}
			}
		}
		clusterID++
	}

	// Build room templates from clusters.
	type clusterData struct {
		brushes []*ParsedBrush
		bounds  BBox
		texSet  map[string]bool
	}
	clusters := map[int]*clusterData{}
	for i, cid := range cluster {
		if cid < 0 {
			continue
		}
		cd, ok := clusters[cid]
		if !ok {
			cd = &clusterData{
				bounds: BBox{
					MinX: math.MaxFloat64, MinY: math.MaxFloat64, MinZ: math.MaxFloat64,
					MaxX: -math.MaxFloat64, MaxY: -math.MaxFloat64, MaxZ: -math.MaxFloat64,
				},
				texSet: map[string]bool{},
			}
			clusters[cid] = cd
		}
		cd.brushes = append(cd.brushes, brushes[i].brush)
		bb := brushes[i].bb
		cd.bounds.MinX = min(cd.bounds.MinX, bb.MinX)
		cd.bounds.MinY = min(cd.bounds.MinY, bb.MinY)
		cd.bounds.MinZ = min(cd.bounds.MinZ, bb.MinZ)
		cd.bounds.MaxX = max(cd.bounds.MaxX, bb.MaxX)
		cd.bounds.MaxY = max(cd.bounds.MaxY, bb.MaxY)
		cd.bounds.MaxZ = max(cd.bounds.MaxZ, bb.MaxZ)
		for _, p := range brushes[i].brush.Planes {
			if p.Texture != "" && !strings.HasPrefix(p.Texture, "*") &&
				p.Texture != "trigger" && p.Texture != "clip" && p.Texture != "sky" {
				cd.texSet[p.Texture] = true
			}
		}
	}

	// Since most maps are fully connected geometry, the flood fill will
	// produce one giant cluster. We need to sub-divide it spatially.
	// Use a grid-based approach: divide the XY extent into cells and
	// assign brushes to the cell containing their center.
	var rooms []RoomTemplate
	idx := 0

	for _, cd := range clusters {
		// If the cluster is small enough, keep it as one room.
		w := cd.bounds.MaxX - cd.bounds.MinX
		h := cd.bounds.MaxY - cd.bounds.MinY
		if w <= 1024 && h <= 1024 {
			var texs []string
			for t := range cd.texSet {
				texs = append(texs, t)
			}
			rooms = append(rooms, RoomTemplate{
				Source:   mapName,
				Index:    idx,
				Brushes:  derefBrushes(cd.brushes),
				Bounds:   cd.bounds,
				Textures: texs,
				FloorZ:   cd.bounds.MinZ,
				CeilZ:    cd.bounds.MaxZ,
			})
			idx++
			continue
		}

		// Sub-divide large cluster into grid cells.
		cellSize := 512.0 // Quake units per cell
		// Adjust cell size to get reasonable room counts.
		nCellsX := max(1, int(math.Ceil(w/cellSize)))
		nCellsY := max(1, int(math.Ceil(h/cellSize)))
		// Cap at reasonable counts.
		if nCellsX > 8 { cellSize = w / 8; nCellsX = 8 }
		if nCellsY > 8 { cellSize = h / 8; nCellsY = 8 }

		// Assign each brush to a grid cell.
		cellBrushes := map[[2]int][]*ParsedBrush{}
		cellBounds := map[[2]int]*BBox{}
		cellTex := map[[2]int]map[string]bool{}

		for _, b := range cd.brushes {
			bb := BrushBounds(b)
			cx := int((bb.CenterX() - cd.bounds.MinX) / cellSize)
			cy := int((bb.CenterY() - cd.bounds.MinY) / cellSize)
			cx = max(0, min(nCellsX-1, cx))
			cy = max(0, min(nCellsY-1, cy))
			key := [2]int{cx, cy}
			cellBrushes[key] = append(cellBrushes[key], b)

			if cellBounds[key] == nil {
				cellBounds[key] = &BBox{
					MinX: math.MaxFloat64, MinY: math.MaxFloat64, MinZ: math.MaxFloat64,
					MaxX: -math.MaxFloat64, MaxY: -math.MaxFloat64, MaxZ: -math.MaxFloat64,
				}
				cellTex[key] = map[string]bool{}
			}
			cb := cellBounds[key]
			cb.MinX = min(cb.MinX, bb.MinX)
			cb.MinY = min(cb.MinY, bb.MinY)
			cb.MinZ = min(cb.MinZ, bb.MinZ)
			cb.MaxX = max(cb.MaxX, bb.MaxX)
			cb.MaxY = max(cb.MaxY, bb.MaxY)
			cb.MaxZ = max(cb.MaxZ, bb.MaxZ)

			for _, p := range b.Planes {
				if p.Texture != "" && !strings.HasPrefix(p.Texture, "*") &&
					p.Texture != "trigger" && p.Texture != "clip" && p.Texture != "sky" {
					cellTex[key][p.Texture] = true
				}
			}
		}

		// Convert cells with enough brushes into room templates.
		for key, cbs := range cellBrushes {
			if len(cbs) < 3 { // skip tiny fragments
				continue
			}
			cb := cellBounds[key]
			var texs []string
			for t := range cellTex[key] {
				texs = append(texs, t)
			}
			rooms = append(rooms, RoomTemplate{
				Source:   mapName,
				Index:    idx,
				Brushes:  derefBrushes(cbs),
				Bounds:   *cb,
				Textures: texs,
				FloorZ:   cb.MinZ,
				CeilZ:    cb.MaxZ,
			})
			idx++
		}
	}

	fmt.Printf("  %s: %d room templates extracted\n", mapName, len(rooms))
	return rooms, nil
}

func derefBrushes(ptrs []*ParsedBrush) []ParsedBrush {
	out := make([]ParsedBrush, len(ptrs))
	for i, p := range ptrs {
		out[i] = *p
	}
	return out
}

// --- Relocate room brushes into a target position ---

// TranslateBrushes shifts all brush plane points by (dx, dy, dz).
func TranslateBrushes(brushes []ParsedBrush, dx, dy, dz float64) []Brush {
	var out []Brush
	for _, pb := range brushes {
		var planes []Plane
		for _, pp := range pb.Planes {
			planes = append(planes, Plane{
				P1:       [3]int{int(pp.P1[0] + dx), int(pp.P1[1] + dy), int(pp.P1[2] + dz)},
				P2:       [3]int{int(pp.P2[0] + dx), int(pp.P2[1] + dy), int(pp.P2[2] + dz)},
				P3:       [3]int{int(pp.P3[0] + dx), int(pp.P3[1] + dy), int(pp.P3[2] + dz)},
				Texture:  pp.Texture,
				XOff:     int(pp.XOff),
				YOff:     int(pp.YOff),
				Rotation: pp.Rotation,
				XScale:   pp.XScale,
				YScale:   pp.YScale,
			})
		}
		out = append(out, Brush{Planes: planes})
	}
	return out
}

// --- Remix using .map source rooms ---

// SourceRoom is kept for backward compatibility but now wraps RoomTemplate data.
type SourceRoom struct {
	Source   string
	Index    int
	Width    int
	Height   int
	FloorZ   int
	CeilZ    int
	WallTex  string
	FloorTex string
	CeilTex  string
	Openings []RoomOpening
	Template *RoomTemplate // pointer to full brush data
}

type RoomOpening struct {
	Side  int
	Pos   float32
	Width float32
}

// SourceRoomsFromTemplates converts RoomTemplates to SourceRooms for
// compatibility with the existing remix pipeline.
func SourceRoomsFromTemplates(templates []RoomTemplate) []SourceRoom {
	var rooms []SourceRoom
	for i := range templates {
		t := &templates[i]
		// Pick dominant textures.
		wallTex, floorTex, ceilTex := classifyTemplateTextures(t)
		rooms = append(rooms, SourceRoom{
			Source:   t.Source,
			Index:    t.Index,
			Width:    int(t.Width()),
			Height:   int(t.Height()),
			FloorZ:   int(t.FloorZ),
			CeilZ:    int(t.CeilZ),
			WallTex:  wallTex,
			FloorTex: floorTex,
			CeilTex:  ceilTex,
			Template: t,
		})
	}
	return rooms
}

// classifyTemplateTextures picks wall/floor/ceil textures from a room template
// by analyzing plane normals.
func classifyTemplateTextures(t *RoomTemplate) (wall, floor, ceil string) {
	wallCount := map[string]int{}
	floorCount := map[string]int{}
	ceilCount := map[string]int{}

	for _, b := range t.Brushes {
		for _, p := range b.Planes {
			tex := p.Texture
			if tex == "" || strings.HasPrefix(tex, "*") || tex == "trigger" || tex == "clip" || tex == "sky" {
				continue
			}
			// Compute normal from the three points.
			u := [3]float64{p.P2[0] - p.P1[0], p.P2[1] - p.P1[1], p.P2[2] - p.P1[2]}
			v := [3]float64{p.P3[0] - p.P1[0], p.P3[1] - p.P1[1], p.P3[2] - p.P1[2]}
			nz := u[0]*v[1] - u[1]*v[0]
			nx := u[1]*v[2] - u[2]*v[1]
			ny := u[2]*v[0] - u[0]*v[2]
			mag := math.Sqrt(nx*nx + ny*ny + nz*nz)
			if mag < 1e-10 {
				continue
			}
			nzNorm := nz / mag

			if math.Abs(nzNorm) > 0.7 {
				if nzNorm > 0 {
					floorCount[tex]++
				} else {
					ceilCount[tex]++
				}
			} else {
				wallCount[tex]++
			}
		}
	}

	topOf := func(m map[string]int) string {
		best, bestN := "", 0
		for k, v := range m {
			if v > bestN {
				best = k
				bestN = v
			}
		}
		return best
	}

	wall = topOf(wallCount)
	floor = topOf(floorCount)
	ceil = topOf(ceilCount)
	if wall == "" { wall = "metal1_1" }
	if floor == "" { floor = wall }
	if ceil == "" { ceil = wall }
	return
}

// ScanSourceRoomsFromMapFile extracts source rooms from a .map source file.
func ScanSourceRoomsFromMapFile(mapDir, mapName string) ([]SourceRoom, error) {
	path := filepath.Join(mapDir, mapName+".map")
	templates, err := ExtractRoomTemplates(path, mapName)
	if err != nil {
		return nil, err
	}
	if len(templates) == 0 {
		return nil, fmt.Errorf("no room templates found in %s", path)
	}
	return SourceRoomsFromTemplates(templates), nil
}

// RemixMap generates a map by placing real room brush geometry from
// source .map files into a procgen layout.
func RemixMap(rng *rand.Rand, sourceRooms []SourceRoom, arenaSize, maxDepth int) (*MapFile, *Layout) {
	layout := GenerateProceduralLayout(rng, arenaSize, maxDepth)

	nav := ValidateNavigation(layout)
	status := "connected"
	if !nav.Connected {
		status = fmt.Sprintf("%d unreachable", len(nav.UnreachableRooms))
	}
	fmt.Printf("rooms=%d  corridors=%d  %s\n",
		len(layout.Rooms), len(layout.Corridors), status)

	// Match each layout room to a source room by similar size.
	assignments := matchRoomsToSources(rng, layout.Rooms, sourceRooms)

	m := NewMapFile()
	m.Worldspawn.Properties["message"] = fmt.Sprintf("remix_%s", sourceRooms[0].Source)

	gi := procgenGridInfo(layout)
	zLo, zHi := computeZRange(layout)
	buildShell(m, gi, zLo, zHi)
	BuildGapFills(m, layout, zLo, zHi)

	// Build rooms — if source has brush data, place real brushes; otherwise
	// fall back to themed box rooms.
	for i := range layout.Rooms {
		room := &layout.Rooms[i]
		p, hasPool := layout.Pools[i]
		var pp *Pool
		if hasPool {
			pp = &p
		}

		src := assignments[i]
		mat := &RoomMaterials{
			Wall:    src.WallTex,
			Floor:   src.FloorTex,
			Ceiling: src.CeilTex,
		}

		if src.Template != nil && len(src.Template.Brushes) > 0 {
			// Place real brush geometry from the source map.
			placeTemplateBrushes(m, room, src.Template)
		} else {
			// Fallback to themed box room.
			BuildRoomBrushesThemed(m, room, pp, mat)
		}

		// Add details.
		headroom := src.CeilZ - src.FloorZ
		var details []Detail
		if headroom > 200 && room.Width() >= 320 && room.Height() >= 320 && !hasPool {
			details = append(details, DetailPlatform)
		}
		if room.Width() >= 256 && room.Height() >= 256 {
			details = append(details, DetailPillars)
		}
		if room.Width() >= 192 && room.Height() >= 192 {
			details = append(details, DetailLightRecesses)
		}
		if len(details) > 2 {
			details = details[:2]
		}
		for _, d := range details {
			PlaceDetail(m, room, pp, d, mat, rng)
		}
	}

	// Corridors.
	hallMat := &RoomMaterials{
		Wall:    sourceRooms[0].WallTex,
		Floor:   sourceRooms[0].FloorTex,
		Ceiling: sourceRooms[0].CeilTex,
	}
	for i := range layout.Corridors {
		BuildCorridorBrushesThemed(m, &layout.Corridors[i], layout.Rooms, hallMat)
	}

	PopulateDM(m, layout, rng)

	return m, layout
}

// placeTemplateBrushes translates source room brush geometry to fit within
// the target layout room's position. The brushes are scaled and shifted
// so their bounding box center aligns with the layout room center.
func placeTemplateBrushes(m *MapFile, room *Room, tmpl *RoomTemplate) {
	// Target room center (Quake units).
	targetCX := float64(room.CX())
	targetCY := float64(room.CY())
	targetCZ := float64(room.Z0)

	// Source room center.
	srcCX := tmpl.Bounds.CenterX()
	srcCY := tmpl.Bounds.CenterY()
	srcCZ := tmpl.FloorZ

	dx := targetCX - srcCX
	dy := targetCY - srcCY
	dz := targetCZ - srcCZ

	translated := TranslateBrushes(tmpl.Brushes, dx, dy, dz)
	for _, b := range translated {
		m.AddBrush(b)
	}
}

// matchRoomsToSources assigns a source room to each layout room,
// matching by similar area.
func matchRoomsToSources(rng *rand.Rand, rooms []Room, sources []SourceRoom) []SourceRoom {
	result := make([]SourceRoom, len(rooms))
	for i, room := range rooms {
		roomArea := room.Width() * room.Height()

		type match struct {
			idx  int
			diff int
		}
		var matches []match
		for j, src := range sources {
			srcArea := src.Width * src.Height
			diff := abs(roomArea - srcArea)
			matches = append(matches, match{j, diff})
		}
		// Sort by diff.
		for a := range matches {
			for b := a + 1; b < len(matches); b++ {
				if matches[b].diff < matches[a].diff {
					matches[a], matches[b] = matches[b], matches[a]
				}
			}
		}
		topN := min(3, len(matches))
		chosen := matches[rng.IntN(topN)].idx
		result[i] = sources[chosen]
	}
	return result
}

// ScanSourceRoomsFromPAK is kept for backward compat but now prefers .map files.
func ScanSourceRoomsFromPAK(pakPath, mapName string) ([]SourceRoom, error) {
	pakData, err := os.ReadFile(pakPath)
	if err != nil {
		return nil, err
	}
	entries, err := parsePAK(pakData)
	if err != nil {
		return nil, err
	}

	target := "maps/" + strings.ToLower(mapName) + ".bsp"
	for _, ent := range entries {
		if strings.ToLower(ent.name) == target {
			if ent.offset+ent.size > len(pakData) {
				continue
			}
			bsp := pakData[ent.offset : ent.offset+ent.size]
			rooms := ExtractSourceRoomsBSP(bsp, mapName)
			return rooms, nil
		}
	}
	return nil, fmt.Errorf("map %s not found in %s", mapName, pakPath)
}

// ExtractSourceRoomsBSP is the old BSP-based room extractor (stub).
// .map source files are preferred; this returns nil.
func ExtractSourceRoomsBSP(_ []byte, _ string) []SourceRoom {
	return nil
}
