package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// --- .map file parser ---

// ParsedPlane is a single face definition from a .map file.
type ParsedPlane struct {
	P1, P2, P3   [3]float64
	Texture      string
	XOff, YOff   float64
	Rotation     float64
	XScale, YScale float64
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
		if strings.HasPrefix(line, "\"") {
			endKey := strings.Index(line[1:], "\"")
			if endKey >= 0 {
				key := line[1 : 1+endKey]
				rest := strings.TrimSpace(line[1+endKey+1:])
				if strings.HasPrefix(rest, "\"") && strings.HasSuffix(rest, "\"") {
					ent.Properties[key] = rest[1 : len(rest)-1]
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
			continue
		}
		brush.Planes = append(brush.Planes, plane)
	}
	return brush, fmt.Errorf("unexpected EOF in brush")
}

func parsePlaneLine(line string) (ParsedPlane, error) {
	var p ParsedPlane

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

// --- Geometry helpers ---

// isSpecialTexture returns true for textures that should be excluded from
// material analysis (liquids, triggers, clip, sky).
func isSpecialTexture(tex string) bool {
	return tex == "" || strings.HasPrefix(tex, "*") ||
		tex == "trigger" || tex == "clip" || tex == "sky" || tex == "skip"
}

// planeNormalf computes the outward normal of a plane defined by three points.
func planeNormalf(p1, p2, p3 [3]float64) [3]float64 {
	u := [3]float64{p2[0] - p1[0], p2[1] - p1[1], p2[2] - p1[2]}
	v := [3]float64{p3[0] - p1[0], p3[1] - p1[1], p3[2] - p1[2]}
	return [3]float64{
		u[1]*v[2] - u[2]*v[1],
		u[2]*v[0] - u[0]*v[2],
		u[0]*v[1] - u[1]*v[0],
	}
}

// --- Bounding box ---

type BBox struct {
	MinX, MinY, MinZ float64
	MaxX, MaxY, MaxZ float64
}

func newBBox() BBox {
	return BBox{
		MinX: math.MaxFloat64, MinY: math.MaxFloat64, MinZ: math.MaxFloat64,
		MaxX: -math.MaxFloat64, MaxY: -math.MaxFloat64, MaxZ: -math.MaxFloat64,
	}
}

func (b *BBox) Expand(other BBox) {
	b.MinX = min(b.MinX, other.MinX)
	b.MinY = min(b.MinY, other.MinY)
	b.MinZ = min(b.MinZ, other.MinZ)
	b.MaxX = max(b.MaxX, other.MaxX)
	b.MaxY = max(b.MaxY, other.MaxY)
	b.MaxZ = max(b.MaxZ, other.MaxZ)
}

func (b BBox) CenterX() float64 { return (b.MinX + b.MaxX) / 2 }
func (b BBox) CenterY() float64 { return (b.MinY + b.MaxY) / 2 }
func (b BBox) Width() float64   { return b.MaxX - b.MinX }
func (b BBox) Height() float64  { return b.MaxY - b.MinY }

// BrushBounds computes the AABB of a brush by intersecting all plane triples.
func BrushBounds(b *ParsedBrush) BBox {
	type plane struct {
		n [3]float64
		d float64
	}
	planes := make([]plane, len(b.Planes))
	for i, pp := range b.Planes {
		n := planeNormalf(pp.P1, pp.P2, pp.P3)
		planes[i] = plane{n: n, d: n[0]*pp.P1[0] + n[1]*pp.P1[1] + n[2]*pp.P1[2]}
	}

	var verts [][3]float64
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
		for _, pp := range b.Planes {
			verts = append(verts, pp.P1, pp.P2, pp.P3)
		}
	}

	bb := newBBox()
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

// --- Room template extraction ---

// RoomTemplate is a group of brushes from a source .map forming a room-scale
// space, relocatable to any position.
type RoomTemplate struct {
	Source   string
	Index    int
	Brushes  []ParsedBrush
	Bounds   BBox
	Textures []string
}

func (r *RoomTemplate) Width() float64  { return r.Bounds.Width() }
func (r *RoomTemplate) Height() float64 { return r.Bounds.Height() }

type cachedBrush struct {
	brush *ParsedBrush
	bb    BBox
}

// ExtractRoomTemplates parses a .map file and clusters worldspawn brushes
// into room-scale groups.
func ExtractRoomTemplates(path string, mapName string) ([]RoomTemplate, error) {
	entities, err := ParseMapFile(path)
	if err != nil {
		return nil, err
	}

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

	var brushes []cachedBrush
	for i := range worldspawn.Brushes {
		bb := BrushBounds(&worldspawn.Brushes[i])
		if bb.Width() < 1 && bb.Height() < 1 {
			continue
		}
		brushes = append(brushes, cachedBrush{brush: &worldspawn.Brushes[i], bb: bb})
	}

	fmt.Printf("  %s: %d worldspawn brushes\n", mapName, len(brushes))

	// Flood-fill cluster brushes by spatial overlap/adjacency.
	const proximity = 32.0
	cluster := make([]int, len(brushes))
	for i := range cluster {
		cluster[i] = -1
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
			a := brushes[cur].bb
			for j := range brushes {
				if cluster[j] >= 0 {
					continue
				}
				b := brushes[j].bb
				if a.MinX <= b.MaxX+proximity && a.MaxX >= b.MinX-proximity &&
					a.MinY <= b.MaxY+proximity && a.MaxY >= b.MinY-proximity &&
					a.MinZ <= b.MaxZ+proximity && a.MaxZ >= b.MinZ-proximity {
					cluster[j] = clusterID
					queue = append(queue, j)
				}
			}
		}
		clusterID++
	}

	// Collect brushes per cluster with cached bounds.
	type clusterData struct {
		indices []int // indices into brushes slice
		bounds  BBox
	}
	clusters := map[int]*clusterData{}
	for i, cid := range cluster {
		if cid < 0 {
			continue
		}
		cd, ok := clusters[cid]
		if !ok {
			cd = &clusterData{bounds: newBBox()}
			clusters[cid] = cd
		}
		cd.indices = append(cd.indices, i)
		cd.bounds.Expand(brushes[i].bb)
	}

	// Subdivide large clusters into grid cells; small ones stay as-is.
	var rooms []RoomTemplate
	idx := 0

	for _, cd := range clusters {
		w, h := cd.bounds.Width(), cd.bounds.Height()

		if w <= 1024 && h <= 1024 {
			rooms = append(rooms, buildTemplate(mapName, idx, cd.indices, brushes))
			idx++
			continue
		}

		cellSize := 512.0
		nCellsX := max(1, min(8, int(math.Ceil(w/cellSize))))
		nCellsY := max(1, min(8, int(math.Ceil(h/cellSize))))
		if nCellsX == 8 { cellSize = w / 8 }
		if nCellsY == 8 { cellSize = h / 8 }

		cells := map[[2]int][]int{}
		for _, bi := range cd.indices {
			bb := brushes[bi].bb
			cx := max(0, min(nCellsX-1, int((bb.CenterX()-cd.bounds.MinX)/cellSize)))
			cy := max(0, min(nCellsY-1, int((bb.CenterY()-cd.bounds.MinY)/cellSize)))
			cells[[2]int{cx, cy}] = append(cells[[2]int{cx, cy}], bi)
		}

		for _, cellIndices := range cells {
			if len(cellIndices) < 3 {
				continue
			}
			rooms = append(rooms, buildTemplate(mapName, idx, cellIndices, brushes))
			idx++
		}
	}

	fmt.Printf("  %s: %d room templates extracted\n", mapName, len(rooms))
	return rooms, nil
}

// buildTemplate creates a RoomTemplate from a set of brush indices.
func buildTemplate(mapName string, idx int, indices []int, brushes []cachedBrush) RoomTemplate {
	bb := newBBox()
	texSet := map[string]bool{}
	out := make([]ParsedBrush, len(indices))

	for i, bi := range indices {
		out[i] = *brushes[bi].brush
		bb.Expand(brushes[bi].bb)
		for _, p := range brushes[bi].brush.Planes {
			if !isSpecialTexture(p.Texture) {
				texSet[p.Texture] = true
			}
		}
	}

	textures := make([]string, 0, len(texSet))
	for t := range texSet {
		textures = append(textures, t)
	}

	return RoomTemplate{
		Source:   mapName,
		Index:    idx,
		Brushes:  out,
		Bounds:   bb,
		Textures: textures,
	}
}

// --- Brush translation ---

// TranslateBrushes shifts all brush plane points by (dx, dy, dz),
// converting ParsedBrush (float64) to Brush (int) for .map output.
func TranslateBrushes(brushes []ParsedBrush, dx, dy, dz float64) []Brush {
	out := make([]Brush, len(brushes))
	for i, pb := range brushes {
		planes := make([]Plane, len(pb.Planes))
		for j, pp := range pb.Planes {
			planes[j] = Plane{
				P1:       [3]int{int(pp.P1[0] + dx), int(pp.P1[1] + dy), int(pp.P1[2] + dz)},
				P2:       [3]int{int(pp.P2[0] + dx), int(pp.P2[1] + dy), int(pp.P2[2] + dz)},
				P3:       [3]int{int(pp.P3[0] + dx), int(pp.P3[1] + dy), int(pp.P3[2] + dz)},
				Texture:  pp.Texture,
				XOff:     int(pp.XOff),
				YOff:     int(pp.YOff),
				Rotation: pp.Rotation,
				XScale:   pp.XScale,
				YScale:   pp.YScale,
			}
		}
		out[i] = Brush{Planes: planes}
	}
	return out
}

// --- Texture analysis ---

// DominantTextures classifies a room template's textures by plane orientation
// into wall/floor/ceiling categories.
func DominantTextures(t *RoomTemplate) (wall, floor, ceil string) {
	counts := [3]map[string]int{{}, {}, {}} // wall, floor, ceil

	for _, b := range t.Brushes {
		for _, p := range b.Planes {
			if isSpecialTexture(p.Texture) {
				continue
			}
			n := planeNormalf(p.P1, p.P2, p.P3)
			mag := math.Sqrt(n[0]*n[0] + n[1]*n[1] + n[2]*n[2])
			if mag < 1e-10 {
				continue
			}
			nz := n[2] / mag
			switch {
			case nz > 0.7:
				counts[1][p.Texture]++
			case nz < -0.7:
				counts[2][p.Texture]++
			default:
				counts[0][p.Texture]++
			}
		}
	}

	topOf := func(m map[string]int) string {
		best, bestN := "", 0
		for k, v := range m {
			if v > bestN {
				best, bestN = k, v
			}
		}
		return best
	}

	wall = topOf(counts[0])
	floor = topOf(counts[1])
	ceil = topOf(counts[2])
	if wall == "" { wall = "metal1_1" }
	if floor == "" { floor = wall }
	if ceil == "" { ceil = wall }
	return
}

// --- CLI support ---

// LoadMapSourceRooms loads room templates from a .map source file.
func LoadMapSourceRooms(mapDir, mapName string) ([]RoomTemplate, error) {
	path := filepath.Join(mapDir, mapName+".map")
	return ExtractRoomTemplates(path, mapName)
}

// MatchTemplatesByArea returns the closest-area source templates for a
// target area, with some randomness in the top 3.
func MatchTemplatesByArea(templates []RoomTemplate, targetArea int, n int) []int {
	type match struct {
		idx  int
		diff int
	}
	matches := make([]match, len(templates))
	for i, t := range templates {
		srcArea := int(t.Width() * t.Height())
		d := targetArea - srcArea
		if d < 0 { d = -d }
		matches[i] = match{i, d}
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].diff < matches[j].diff
	})

	topN := min(n, len(matches))
	result := make([]int, topN)
	for i := 0; i < topN; i++ {
		result[i] = matches[i].idx
	}
	return result
}
