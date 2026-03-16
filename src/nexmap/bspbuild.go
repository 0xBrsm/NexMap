package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

// Polygon is a convex face in 3D space.
type Polygon struct {
	Verts   [][3]float32 // clockwise winding when viewed from front
	Normal  [3]float32
	Dist    float32
	TexName string
	TexS    [4]float32 // S axis + offset
	TexT    [4]float32 // T axis + offset
}

// --- Brush to polygon conversion (axis-aligned only for now) ---

// BrushToPolygons converts one Brush into up to 6 face polygons.
// Works for axis-aligned boxes. For general brushes, would need
// full CSG plane intersection — that's a future extension point.
func BrushToPolygons(b *Brush) []Polygon {
	if len(b.Planes) != 6 {
		return nil // Only AABBs for now
	}

	// Extract AABB bounds from the 6 planes.
	var minX, maxX, minY, maxY, minZ, maxZ float32
	foundAxes := 0

	for _, p := range b.Planes {
		nx, ny, nz := planeNormal(p.P1, p.P2, p.P3)

		if near(nx, -1) && near(ny, 0) && near(nz, 0) {
			minX = float32(p.P1[0])
			foundAxes |= 1
		} else if near(nx, 1) && near(ny, 0) && near(nz, 0) {
			maxX = float32(p.P1[0])
			foundAxes |= 2
		} else if near(ny, -1) && near(nx, 0) && near(nz, 0) {
			minY = float32(p.P1[1])
			foundAxes |= 4
		} else if near(ny, 1) && near(nx, 0) && near(nz, 0) {
			maxY = float32(p.P1[1])
			foundAxes |= 8
		} else if near(nz, -1) && near(nx, 0) && near(ny, 0) {
			minZ = float32(p.P1[2])
			foundAxes |= 16
		} else if near(nz, 1) && near(nx, 0) && near(ny, 0) {
			maxZ = float32(p.P1[2])
			foundAxes |= 32
		}
	}

	if foundAxes != 63 {
		return nil // Not a valid AABB
	}

	var polys []Polygon

	for _, p := range b.Planes {
		nx, ny, nz := planeNormal(p.P1, p.P2, p.P3)
		var norm [3]float32
		var dist float32
		var verts [4][3]float32

		if near(nx, -1) {
			norm = [3]float32{-1, 0, 0}
			dist = -minX
			verts = [4][3]float32{
				{minX, minY, maxZ}, {minX, maxY, maxZ},
				{minX, maxY, minZ}, {minX, minY, minZ},
			}
		} else if near(nx, 1) {
			norm = [3]float32{1, 0, 0}
			dist = maxX
			verts = [4][3]float32{
				{maxX, maxY, maxZ}, {maxX, minY, maxZ},
				{maxX, minY, minZ}, {maxX, maxY, minZ},
			}
		} else if near(ny, -1) {
			norm = [3]float32{0, -1, 0}
			dist = -minY
			verts = [4][3]float32{
				{maxX, minY, maxZ}, {minX, minY, maxZ},
				{minX, minY, minZ}, {maxX, minY, minZ},
			}
		} else if near(ny, 1) {
			norm = [3]float32{0, 1, 0}
			dist = maxY
			verts = [4][3]float32{
				{minX, maxY, maxZ}, {maxX, maxY, maxZ},
				{maxX, maxY, minZ}, {minX, maxY, minZ},
			}
		} else if near(nz, -1) {
			norm = [3]float32{0, 0, -1}
			dist = -minZ
			verts = [4][3]float32{
				{minX, maxY, minZ}, {maxX, maxY, minZ},
				{maxX, minY, minZ}, {minX, minY, minZ},
			}
		} else if near(nz, 1) {
			norm = [3]float32{0, 0, 1}
			dist = maxZ
			verts = [4][3]float32{
				{minX, minY, maxZ}, {maxX, minY, maxZ},
				{maxX, maxY, maxZ}, {minX, maxY, maxZ},
			}
		} else {
			continue
		}

		texS, texT := texVecsForNormal(norm, float32(p.XScale), float32(p.YScale), float32(p.XOff), float32(p.YOff))

		polys = append(polys, Polygon{
			Verts:   verts[:],
			Normal:  norm,
			Dist:    dist,
			TexName: p.Texture,
			TexS:    texS,
			TexT:    texT,
		})
	}

	return polys
}

// planeNormal computes the outward normal from 3 clockwise points.
func planeNormal(p1, p2, p3 [3]int) (float32, float32, float32) {
	ax := float64(p2[0] - p1[0])
	ay := float64(p2[1] - p1[1])
	az := float64(p2[2] - p1[2])
	bx := float64(p3[0] - p1[0])
	by := float64(p3[1] - p1[1])
	bz := float64(p3[2] - p1[2])
	nx := ay*bz - az*by
	ny := az*bx - ax*bz
	nz := ax*by - ay*bx
	ln := math.Sqrt(nx*nx + ny*ny + nz*nz)
	if ln < 1e-6 {
		return 0, 0, 1
	}
	return float32(nx / ln), float32(ny / ln), float32(nz / ln)
}

func near(a float32, b float64) bool {
	return math.Abs(float64(a)-b) < 0.01
}

// texVecsForNormal picks texture projection axes based on face normal.
func texVecsForNormal(n [3]float32, scaleS, scaleT, offS, offT float32) ([4]float32, [4]float32) {
	if scaleS == 0 {
		scaleS = 1
	}
	if scaleT == 0 {
		scaleT = 1
	}
	invS := 1.0 / scaleS
	invT := 1.0 / scaleT

	// For axis-aligned normals, choose standard texture axes.
	ax := absf(n[0])
	ay := absf(n[1])
	az := absf(n[2])

	var s, t [4]float32
	if ax >= ay && ax >= az {
		// X-facing: project onto YZ
		s = [4]float32{0, invS, 0, offS}
		t = [4]float32{0, 0, -invT, offT}
	} else if ay >= ax && ay >= az {
		// Y-facing: project onto XZ
		s = [4]float32{invS, 0, 0, offS}
		t = [4]float32{0, 0, -invT, offT}
	} else {
		// Z-facing: project onto XY
		s = [4]float32{invS, 0, 0, offS}
		t = [4]float32{0, -invT, 0, offT}
	}
	return s, t
}

func absf(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// --- Unique plane/vertex/texinfo dedup ---

type planeKey struct {
	nx, ny, nz, dist float32
}

type vertKey struct {
	x, y, z float32
}

type texinfoKey struct {
	s0, s1, s2, s3 float32
	t0, t1, t2, t3 float32
	miptex         uint32
	flags          uint32
}

// --- BSP tree builder ---

// BuildBSP converts a MapFile into a complete BSPData ready to write.
func BuildBSP(mf *MapFile) *BSPData {
	bsp := &BSPData{}

	// Edge 0 is always unused (Quake convention).
	bsp.Edges = append(bsp.Edges, BSPEdge{0, 0})
	bsp.Surfedges = append(bsp.Surfedges, 0) // placeholder

	// Leaf 0 is always the "outside" leaf — must be solid.
	bsp.Leaves = append(bsp.Leaves, BSPLeaf{Contents: ContentsSolid, VisOfs: -1})

	// Collect all polygons from worldspawn brushes.
	var allPolys []Polygon
	for i := range mf.Worldspawn.Brushes {
		polys := BrushToPolygons(&mf.Worldspawn.Brushes[i])
		allPolys = append(allPolys, polys...)
	}

	// Build texture lump.
	texNames := collectTexNames(allPolys)
	texNameToID := map[string]uint32{}
	for i, name := range texNames {
		texNameToID[name] = uint32(i)
	}
	bsp.MiptexData = buildMiptexLump(texNames)

	// Dedup maps.
	planeMap := map[planeKey]int{}
	vertMap := map[vertKey]int{}
	tinfoMap := map[texinfoKey]int{}

	getPlane := func(nx, ny, nz, dist float32) int {
		k := planeKey{nx, ny, nz, dist}
		if idx, ok := planeMap[k]; ok {
			return idx
		}
		idx := bsp.AddPlane(nx, ny, nz, dist)
		planeMap[k] = idx
		return idx
	}

	getVertex := func(x, y, z float32) int {
		k := vertKey{x, y, z}
		if idx, ok := vertMap[k]; ok {
			return idx
		}
		idx := bsp.AddVertex(x, y, z)
		vertMap[k] = idx
		return idx
	}

	getTexinfo := func(s, t [4]float32, miptex uint32, flags uint32) int {
		k := texinfoKey{s[0], s[1], s[2], s[3], t[0], t[1], t[2], t[3], miptex, flags}
		if idx, ok := tinfoMap[k]; ok {
			return idx
		}
		idx := len(bsp.Texinfo)
		bsp.Texinfo = append(bsp.Texinfo, BSPTexinfo{
			VecS: [3]float32{s[0], s[1], s[2]}, DistS: s[3],
			VecT: [3]float32{t[0], t[1], t[2]}, DistT: t[3],
			MiptexID: miptex, Flags: flags,
		})
		tinfoMap[k] = idx
		return idx
	}

	// Convert polygons to BSP faces.
	type bspFaceInfo struct {
		planeIdx  int
		side      int
		texinfo   int
		verts     [][3]float32
		texName   string
	}
	var faceInfos []bspFaceInfo

	for _, poly := range allPolys {
		// Get or create plane. BSP planes always point in positive direction.
		nx, ny, nz, dist := poly.Normal[0], poly.Normal[1], poly.Normal[2], poly.Dist
		side := 0
		if nx < 0 || (nx == 0 && ny < 0) || (nx == 0 && ny == 0 && nz < 0) {
			nx, ny, nz = -nx, -ny, -nz
			dist = -dist
			side = 1
		}

		planeIdx := getPlane(nx, ny, nz, dist)

		flags := uint32(0)
		if strings.HasPrefix(poly.TexName, "*") {
			flags = 1 // animated/liquid
		}
		tinfoIdx := getTexinfo(poly.TexS, poly.TexT, texNameToID[poly.TexName], flags)

		faceInfos = append(faceInfos, bspFaceInfo{
			planeIdx: planeIdx,
			side:     side,
			texinfo:  tinfoIdx,
			verts:    poly.Verts,
			texName:  poly.TexName,
		})
	}

	// Emit faces with edges.
	for _, fi := range faceInfos {
		firstSE := len(bsp.Surfedges)
		nv := len(fi.verts)

		for j := range nv {
			v0 := getVertex(fi.verts[j][0], fi.verts[j][1], fi.verts[j][2])
			v1 := getVertex(fi.verts[(j+1)%nv][0], fi.verts[(j+1)%nv][1], fi.verts[(j+1)%nv][2])

			edgeIdx := len(bsp.Edges)
			if v0 <= v1 {
				bsp.Edges = append(bsp.Edges, BSPEdge{uint16(v0), uint16(v1)})
				bsp.Surfedges = append(bsp.Surfedges, int32(edgeIdx))
			} else {
				bsp.Edges = append(bsp.Edges, BSPEdge{uint16(v1), uint16(v0)})
				bsp.Surfedges = append(bsp.Surfedges, -int32(edgeIdx))
			}
		}

		face := BSPFace{
			PlaneID:   uint16(fi.planeIdx),
			Side:      uint16(fi.side),
			FirstEdge: int32(firstSE),
			NumEdges:  uint16(nv),
			TexinfoID: uint16(fi.texinfo),
			LightType: 0,
			BaseLight: 0xFF,
			Light1:    0xFF,
			Light2:    0xFF,
			LightOfs:  -1, // No lightmap initially
		}
		bsp.Faces = append(bsp.Faces, face)
	}

	// Build BSP tree.
	faceIndices := make([]int, len(bsp.Faces))
	for i := range faceIndices {
		faceIndices[i] = i
	}
	buildBSPTree(bsp, faceIndices, allPolys)

	// Build clip hulls with proper hull expansion.
	buildClipHulls(bsp, mf)

	// Generate lightmaps.
	generateLightmaps(bsp, mf, allPolys)

	// Generate visibility (simple: all-visible).
	generateVisibility(bsp)

	// Entities string.
	bsp.Entities = buildEntityString(mf)

	// Model 0 = worldspawn.
	bboxMin, bboxMax := computeBounds(bsp.Vertices)
	bsp.Models = append(bsp.Models, BSPModel{
		BBoxMin:   bboxMin,
		BBoxMax:   bboxMax,
		Origin:    [3]float32{0, 0, 0},
		HeadNodes: [4]int32{0, bsp.hull1Head, bsp.hull2Head, 0},
		VisLeafs:  int32(len(bsp.Leaves) - 1),
		FirstFace: 0,
		NumFaces:  int32(len(bsp.Faces)),
	})

	// Brush entity models (trigger_teleport etc).
	for _, ent := range mf.Entities {
		if len(ent.Brushes) == 0 {
			continue
		}
		firstFace := int32(len(bsp.Faces))
		var entPolys []Polygon
		for i := range ent.Brushes {
			polys := BrushToPolygons(&ent.Brushes[i])
			entPolys = append(entPolys, polys...)
		}
		for _, poly := range entPolys {
			nx, ny, nz, dist := poly.Normal[0], poly.Normal[1], poly.Normal[2], poly.Dist
			side := 0
			if nx < 0 || (nx == 0 && ny < 0) || (nx == 0 && ny == 0 && nz < 0) {
				nx, ny, nz = -nx, -ny, -nz
				dist = -dist
				side = 1
			}
			planeIdx := getPlane(nx, ny, nz, dist)
			flags := uint32(0)
			if strings.HasPrefix(poly.TexName, "*") {
				flags = 1
			}
			tinfoIdx := getTexinfo(poly.TexS, poly.TexT, texNameToID[poly.TexName], flags)

			firstSE := len(bsp.Surfedges)
			nv := len(poly.Verts)
			for j := range nv {
				v0 := getVertex(poly.Verts[j][0], poly.Verts[j][1], poly.Verts[j][2])
				v1 := getVertex(poly.Verts[(j+1)%nv][0], poly.Verts[(j+1)%nv][1], poly.Verts[(j+1)%nv][2])
				edgeIdx := len(bsp.Edges)
				if v0 <= v1 {
					bsp.Edges = append(bsp.Edges, BSPEdge{uint16(v0), uint16(v1)})
					bsp.Surfedges = append(bsp.Surfedges, int32(edgeIdx))
				} else {
					bsp.Edges = append(bsp.Edges, BSPEdge{uint16(v1), uint16(v0)})
					bsp.Surfedges = append(bsp.Surfedges, -int32(edgeIdx))
				}
			}
			bsp.Faces = append(bsp.Faces, BSPFace{
				PlaneID:   uint16(planeIdx),
				Side:      uint16(side),
				FirstEdge: int32(firstSE),
				NumEdges:  uint16(nv),
				TexinfoID: uint16(tinfoIdx),
				LightType: 0xFF, // no lightmap for triggers
				BaseLight: 0xFF,
				Light1:    0xFF,
				Light2:    0xFF,
				LightOfs:  -1,
			})
		}
		numFaces := int32(len(bsp.Faces)) - firstFace
		eMin, eMax := computePolyBounds(entPolys)
		bsp.Models = append(bsp.Models, BSPModel{
			BBoxMin:   eMin,
			BBoxMax:   eMax,
			Origin:    [3]float32{0, 0, 0},
			HeadNodes: [4]int32{0, 0, 0, 0},
			VisLeafs:  0,
			FirstFace: firstFace,
			NumFaces:  numFaces,
		})
	}

	return bsp
}

// --- BSP tree construction ---

type aabb3 struct {
	min, max [3]float32
}

func polyBounds(polys []Polygon, indices []int) aabb3 {
	bb := aabb3{
		min: [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32},
		max: [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32},
	}
	for _, idx := range indices {
		for _, v := range polys[idx].Verts {
			for a := range 3 {
				if v[a] < bb.min[a] {
					bb.min[a] = v[a]
				}
				if v[a] > bb.max[a] {
					bb.max[a] = v[a]
				}
			}
		}
	}
	return bb
}

func buildBSPTree(bsp *BSPData, faceIndices []int, polys []Polygon) {
	// We build a simple axis-aligned BSP.
	// Recursive: pick the longest axis, split at the midpoint.

	type buildNode struct {
		faceIndices []int
		bounds      aabb3
	}

	var buildRecursive func(indices []int) int16

	buildRecursive = func(indices []int) int16 {
		if len(indices) == 0 {
			// Empty leaf.
			leafIdx := len(bsp.Leaves)
			bsp.Leaves = append(bsp.Leaves, BSPLeaf{
				Contents: ContentsSolid,
				VisOfs:   -1,
			})
			return ^int16(leafIdx)
		}

		bb := polyBounds(polys, indices)

		// If few faces, make a leaf.
		if len(indices) <= 8 {
			leafIdx := len(bsp.Leaves)
			firstMark := uint16(len(bsp.Marksurfaces))
			for _, fi := range indices {
				bsp.Marksurfaces = append(bsp.Marksurfaces, uint16(fi))
			}
			bbMin := [3]int16{int16(bb.min[0]), int16(bb.min[1]), int16(bb.min[2])}
			bbMax := [3]int16{int16(bb.max[0]), int16(bb.max[1]), int16(bb.max[2])}
			bsp.Leaves = append(bsp.Leaves, BSPLeaf{
				Contents: ContentsEmpty,
				VisOfs:   -1,
				BBoxMin:  bbMin,
				BBoxMax:  bbMax,
				FirstMark: firstMark,
				NumMarks:  uint16(len(indices)),
			})
			return ^int16(leafIdx)
		}

		// Pick split axis (longest extent).
		dx := bb.max[0] - bb.min[0]
		dy := bb.max[1] - bb.min[1]
		dz := bb.max[2] - bb.min[2]
		axis := 0
		if dy > dx && dy > dz {
			axis = 1
		} else if dz > dx && dz > dy {
			axis = 2
		}

		mid := (bb.min[axis] + bb.max[axis]) / 2

		// Create split plane.
		var nx, ny, nz float32
		switch axis {
		case 0:
			nx = 1
		case 1:
			ny = 1
		case 2:
			nz = 1
		}
		planeIdx := bsp.AddPlane(nx, ny, nz, mid)

		// Partition faces.
		var front, back []int
		for _, fi := range indices {
			center := polyCenterAxis(polys[fi], axis)
			if center >= mid {
				front = append(front, fi)
			} else {
				back = append(back, fi)
			}
		}

		// Avoid infinite recursion: if one side got everything, force split.
		if len(front) == 0 || len(back) == 0 {
			half := len(indices) / 2
			front = indices[:half]
			back = indices[half:]
		}

		frontChild := buildRecursive(front)
		backChild := buildRecursive(back)

		// Assign faces that lie on this node's plane.
		nodeFirstFace := uint16(0)
		nodeNumFaces := uint16(0)

		bbMin := [3]int16{int16(bb.min[0]), int16(bb.min[1]), int16(bb.min[2])}
		bbMax := [3]int16{int16(bb.max[0]), int16(bb.max[1]), int16(bb.max[2])}

		nodeIdx := len(bsp.Nodes)
		bsp.Nodes = append(bsp.Nodes, BSPNode{
			PlaneID:   int32(planeIdx),
			Children:  [2]int16{frontChild, backChild},
			BBoxMin:   bbMin,
			BBoxMax:   bbMax,
			FirstFace: nodeFirstFace,
			NumFaces:  nodeNumFaces,
		})
		return int16(nodeIdx)
	}

	buildRecursive(faceIndices)
}

func polyCenterAxis(p Polygon, axis int) float32 {
	sum := float32(0)
	for _, v := range p.Verts {
		sum += v[axis]
	}
	return sum / float32(len(p.Verts))
}

// --- Clip hulls ---
// Quake uses 3 collision hulls:
//   Hull 0: point-sized (uses BSP nodes directly)
//   Hull 1: player (32 wide, 32 deep, 56 tall → half-extents 16,16,28)
//   Hull 2: shambler (64 wide, 64 deep, 88 tall → half-extents 32,32,44)
// For each hull, every solid brush is expanded by the hull's half-extents,
// then a separate clip BSP tree is built from those expanded brushes.

type hullExtents struct {
	hx, hy, hz float32 // half-extents
}

var (
	hull1Extents = hullExtents{16, 16, 28}  // player
	hull2Extents = hullExtents{32, 32, 44}  // shambler
)

// expandedBrushAABB represents a solid volume expanded for collision.
type clipAABB struct {
	minX, minY, minZ float32
	maxX, maxY, maxZ float32
}

// collectSolidAABBs extracts AABBs from all worldspawn brushes, expanded by hull extents.
func collectSolidAABBs(mf *MapFile, he hullExtents) []clipAABB {
	var boxes []clipAABB
	for i := range mf.Worldspawn.Brushes {
		b := &mf.Worldspawn.Brushes[i]
		if len(b.Planes) != 6 {
			continue
		}
		var mnX, mxX, mnY, mxY, mnZ, mxZ float32
		found := 0
		for _, p := range b.Planes {
			nx, ny, nz := planeNormal(p.P1, p.P2, p.P3)
			if near(nx, -1) { mnX = float32(p.P1[0]); found |= 1 }
			if near(nx, 1)  { mxX = float32(p.P1[0]); found |= 2 }
			if near(ny, -1) { mnY = float32(p.P1[1]); found |= 4 }
			if near(ny, 1)  { mxY = float32(p.P1[1]); found |= 8 }
			if near(nz, -1) { mnZ = float32(p.P1[2]); found |= 16 }
			if near(nz, 1)  { mxZ = float32(p.P1[2]); found |= 32 }
		}
		if found != 63 {
			continue
		}
		boxes = append(boxes, clipAABB{
			minX: mnX - he.hx, minY: mnY - he.hy, minZ: mnZ - he.hz,
			maxX: mxX + he.hx, maxY: mxY + he.hy, maxZ: mxZ + he.hz,
		})
	}
	return boxes
}

// buildClipTree builds a BSP clip tree from expanded solid AABBs.
// Returns the head clip node index.
func buildClipTree(bsp *BSPData, boxes []clipAABB) int32 {
	if len(boxes) == 0 {
		// Single node: everything is empty.
		idx := int32(len(bsp.Clipnodes))
		bsp.Clipnodes = append(bsp.Clipnodes, BSPClipnode{
			PlaneID:  0,
			Children: [2]int16{-1, -1}, // both sides empty
		})
		return idx
	}

	type clipBounds struct {
		min, max [3]float32
	}
	allBounds := func(boxes []clipAABB) clipBounds {
		cb := clipBounds{
			min: [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32},
			max: [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32},
		}
		for _, b := range boxes {
			if b.minX < cb.min[0] { cb.min[0] = b.minX }
			if b.minY < cb.min[1] { cb.min[1] = b.minY }
			if b.minZ < cb.min[2] { cb.min[2] = b.minZ }
			if b.maxX > cb.max[0] { cb.max[0] = b.maxX }
			if b.maxY > cb.max[1] { cb.max[1] = b.maxY }
			if b.maxZ > cb.max[2] { cb.max[2] = b.maxZ }
		}
		return cb
	}

	var buildRec func(boxes []clipAABB, depth int) int16
	buildRec = func(boxes []clipAABB, depth int) int16 {
		if len(boxes) == 0 {
			return -1 // CONTENTS_EMPTY
		}
		if depth > 32 || len(boxes) == 1 {
			// Leaf: this region is solid.
			return -2 // CONTENTS_SOLID
		}

		cb := allBounds(boxes)
		dx := cb.max[0] - cb.min[0]
		dy := cb.max[1] - cb.min[1]
		dz := cb.max[2] - cb.min[2]
		axis := 0
		if dy > dx && dy > dz { axis = 1 }
		if dz > dx && dz > dy { axis = 2 }

		mid := (cb.min[axis] + cb.max[axis]) / 2

		var nx, ny, nz float32
		switch axis {
		case 0: nx = 1
		case 1: ny = 1
		case 2: nz = 1
		}
		planeIdx := bsp.AddPlane(nx, ny, nz, mid)

		var front, back []clipAABB
		for _, b := range boxes {
			bmin := [3]float32{b.minX, b.minY, b.minZ}
			bmax := [3]float32{b.maxX, b.maxY, b.maxZ}
			if bmax[axis] > mid {
				front = append(front, b)
			}
			if bmin[axis] < mid {
				back = append(back, b)
			}
		}

		// Prevent infinite recursion.
		if len(front) == len(boxes) && len(back) == len(boxes) {
			return -2 // solid
		}

		nodeIdx := len(bsp.Clipnodes)
		bsp.Clipnodes = append(bsp.Clipnodes, BSPClipnode{}) // placeholder

		frontChild := buildRec(front, depth+1)
		backChild := buildRec(back, depth+1)

		bsp.Clipnodes[nodeIdx] = BSPClipnode{
			PlaneID:  int32(planeIdx),
			Children: [2]int16{frontChild, backChild},
		}
		return int16(nodeIdx)
	}

	headIdx := int32(len(bsp.Clipnodes))
	buildRec(boxes, 0)
	return headIdx
}

func buildClipHulls(bsp *BSPData, mf *MapFile) {
	hull1Boxes := collectSolidAABBs(mf, hull1Extents)
	hull2Boxes := collectSolidAABBs(mf, hull2Extents)

	hull1Head := buildClipTree(bsp, hull1Boxes)
	hull2Head := buildClipTree(bsp, hull2Boxes)

	// Store for model 0 (set after model is created).
	bsp.hull1Head = hull1Head
	bsp.hull2Head = hull2Head
}

// --- Visibility (portal-based flood-fill PVS) ---

// generateVisibility builds a PVS (potentially visible set) for each leaf
// by finding portals between adjacent leaves and flood-filling visibility
// through open connections. Solid leaves block visibility.
func generateVisibility(bsp *BSPData) {
	numLeaves := len(bsp.Leaves)
	if numLeaves <= 1 {
		return
	}

	// Build leaf adjacency by walking the BSP tree.
	// Two leaves are adjacent if they share a parent node (are siblings or
	// separated by a single split plane through a node).
	adj := buildLeafAdjacency(bsp)

	// For each non-solid leaf, flood-fill through adjacent empty leaves.
	numVis := numLeaves - 1 // exclude leaf 0
	byteCount := (numVis + 7) / 8

	for i := 1; i < numLeaves; i++ {
		if bsp.Leaves[i].Contents == ContentsSolid {
			bsp.Leaves[i].VisOfs = -1
			continue
		}

		pvs := make([]byte, byteCount)

		// BFS flood fill.
		visited := make([]bool, numLeaves)
		queue := []int{i}
		visited[i] = true

		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]

			// Mark visible (leaf indices are 1-based in PVS bit vector).
			if cur > 0 && cur <= numVis {
				bit := cur - 1
				pvs[bit/8] |= 1 << (bit % 8)
			}

			for _, nb := range adj[cur] {
				if visited[nb] {
					continue
				}
				visited[nb] = true
				if bsp.Leaves[nb].Contents == ContentsSolid {
					continue // solid blocks visibility
				}
				queue = append(queue, nb)
			}
		}

		// RLE compress the PVS row.
		bsp.Leaves[i].VisOfs = int32(len(bsp.Visibility))
		bsp.Visibility = append(bsp.Visibility, compressVis(pvs)...)
	}
}

// buildLeafAdjacency finds which leaves are connected through BSP node portals.
func buildLeafAdjacency(bsp *BSPData) map[int][]int {
	adj := map[int][]int{}

	// For each node, its two children share a portal through the split plane.
	// Collect all leaves reachable from each child subtree, then connect them.
	var leavesUnder func(child int16) []int
	leavesUnder = func(child int16) []int {
		if child < 0 {
			// It's a leaf: ~child is the leaf index.
			leafIdx := int(^child)
			if leafIdx >= 0 && leafIdx < len(bsp.Leaves) {
				return []int{leafIdx}
			}
			return nil
		}
		if int(child) >= len(bsp.Nodes) {
			return nil
		}
		node := &bsp.Nodes[child]
		left := leavesUnder(node.Children[0])
		right := leavesUnder(node.Children[1])
		return append(left, right...)
	}

	for _, node := range bsp.Nodes {
		frontLeaves := leavesUnder(node.Children[0])
		backLeaves := leavesUnder(node.Children[1])

		// Connect front leaves to back leaves (portal adjacency).
		// Only connect non-solid to non-solid at the boundary.
		for _, fl := range frontLeaves {
			for _, bl := range backLeaves {
				if bsp.Leaves[fl].Contents == ContentsSolid && bsp.Leaves[bl].Contents == ContentsSolid {
					continue
				}
				adj[fl] = append(adj[fl], bl)
				adj[bl] = append(adj[bl], fl)
			}
		}
	}

	// Deduplicate.
	for k, v := range adj {
		seen := map[int]bool{}
		deduped := v[:0]
		for _, nb := range v {
			if !seen[nb] {
				seen[nb] = true
				deduped = append(deduped, nb)
			}
		}
		adj[k] = deduped
	}

	return adj
}

// compressVis RLE-compresses a PVS bit vector using Quake's VIS format:
// zero bytes are encoded as 0x00 followed by a count byte.
func compressVis(pvs []byte) []byte {
	var out []byte
	i := 0
	for i < len(pvs) {
		if pvs[i] != 0 {
			out = append(out, pvs[i])
			i++
		} else {
			// Count consecutive zeros.
			count := 0
			for i < len(pvs) && pvs[i] == 0 && count < 255 {
				count++
				i++
			}
			out = append(out, 0, byte(count))
		}
	}
	return out
}

// --- Lightmaps ---

// rayHitsAABB tests if a ray from (ox,oy,oz) toward (dx,dy,dz) hits an AABB
// within distance maxDist. Uses slab intersection test.
func rayHitsAABB(ox, oy, oz, dx, dy, dz, maxDist float32, b *clipAABB) bool {
	var tmin, tmax float32
	tmin = 0
	tmax = maxDist

	inv := func(d, o, bmin, bmax float32) (float32, float32, bool) {
		if absf(d) < 1e-8 {
			// Parallel to slab — check if origin is inside.
			if o < bmin || o > bmax {
				return 0, 0, false
			}
			return -math.MaxFloat32, math.MaxFloat32, true
		}
		invD := 1.0 / d
		t0 := (bmin - o) * invD
		t1 := (bmax - o) * invD
		if t0 > t1 {
			t0, t1 = t1, t0
		}
		return t0, t1, true
	}

	t0, t1, ok := inv(dx, ox, b.minX, b.maxX)
	if !ok { return false }
	if t0 > tmin { tmin = t0 }
	if t1 < tmax { tmax = t1 }
	if tmin > tmax { return false }

	t0, t1, ok = inv(dy, oy, b.minY, b.maxY)
	if !ok { return false }
	if t0 > tmin { tmin = t0 }
	if t1 < tmax { tmax = t1 }
	if tmin > tmax { return false }

	t0, t1, ok = inv(dz, oz, b.minZ, b.maxZ)
	if !ok { return false }
	if t0 > tmin { tmin = t0 }
	if t1 < tmax { tmax = t1 }
	if tmin > tmax { return false }

	return tmin < maxDist && tmax > 0
}

// shadowTest returns true if the ray from point to light is blocked by any solid.
func shadowTest(px, py, pz, lx, ly, lz float32, solids []clipAABB) bool {
	dx := lx - px
	dy := ly - py
	dz := lz - pz
	dist := float32(math.Sqrt(float64(dx*dx + dy*dy + dz*dz)))
	if dist < 1 {
		return false
	}
	// Nudge origin along normal slightly to avoid self-intersection.
	invD := 1.0 / dist
	dx *= invD
	dy *= invD
	dz *= invD
	eps := float32(1.0)
	ox := px + dx*eps
	oy := py + dy*eps
	oz := pz + dz*eps
	maxDist := dist - 2*eps

	for i := range solids {
		if rayHitsAABB(ox, oy, oz, dx, dy, dz, maxDist, &solids[i]) {
			return true
		}
	}
	return false
}

func generateLightmaps(bsp *BSPData, mf *MapFile, polys []Polygon) {
	// Collect light sources.
	type lightSrc struct {
		x, y, z    float32
		brightness float32
	}
	var lights []lightSrc

	for _, ent := range mf.Entities {
		if ent.Properties["classname"] != "light" {
			continue
		}
		var lx, ly, lz float32
		fmt.Sscanf(ent.Properties["origin"], "%f %f %f", &lx, &ly, &lz)
		bright := float32(200)
		if v, ok := ent.Properties["light"]; ok {
			fmt.Sscanf(v, "%f", &bright)
		}
		lights = append(lights, lightSrc{lx, ly, lz, bright})
	}

	for i := range bsp.Faces {
		face := &bsp.Faces[i]
		if face.LightType == 0xFF {
			face.LightOfs = -1
			continue
		}

		if i >= len(polys) {
			face.LightOfs = -1
			continue
		}

		poly := &polys[i]
		ti := &bsp.Texinfo[face.TexinfoID]

		// Face extents in texture space.
		sMin, sMax := float32(math.MaxFloat32), float32(-math.MaxFloat32)
		tMin, tMax := float32(math.MaxFloat32), float32(-math.MaxFloat32)

		for _, v := range poly.Verts {
			s := ti.VecS[0]*v[0] + ti.VecS[1]*v[1] + ti.VecS[2]*v[2] + ti.DistS
			t := ti.VecT[0]*v[0] + ti.VecT[1]*v[1] + ti.VecT[2]*v[2] + ti.DistT
			if s < sMin { sMin = s }
			if s > sMax { sMax = s }
			if t < tMin { tMin = t }
			if t > tMax { tMax = t }
		}

		ls0 := float32(math.Floor(float64(sMin) / 16))
		lt0 := float32(math.Floor(float64(tMin) / 16))
		ls1 := float32(math.Ceil(float64(sMax) / 16))
		lt1 := float32(math.Ceil(float64(tMax) / 16))

		lw := int(ls1 - ls0) + 1
		lh := int(lt1 - lt0) + 1
		if lw < 1 { lw = 1 }
		if lh < 1 { lh = 1 }
		if lw > 18 { lw = 18 }
		if lh > 18 { lh = 18 }

		face.LightOfs = int32(len(bsp.Lighting))

		for ly := range lh {
			for lx := range lw {
				ls := (ls0 + float32(lx) + 0.5) * 16
				lt := (lt0 + float32(ly) + 0.5) * 16

				wx, wy, wz := luxelToWorld(ls, lt, ti, poly)

				total := float32(0)
				for _, l := range lights {
					dx := wx - l.x
					dy := wy - l.y
					dz := wz - l.z
					distSq := dx*dx + dy*dy + dz*dz
					if distSq < 1 {
						distSq = 1
					}

					dist := float32(math.Sqrt(float64(distSq)))
					dirX := (l.x - wx) / dist
					dirY := (l.y - wy) / dist
					dirZ := (l.z - wz) / dist
					dot := dirX*poly.Normal[0] + dirY*poly.Normal[1] + dirZ*poly.Normal[2]
					if dot <= 0 {
						continue // back-facing
					}

					// Linear falloff with N·L (skip shadow test for box-room geometry).
					contrib := l.brightness * dot / dist
					total += contrib
				}

				val := int(total)
				if val < 0 { val = 0 }
				if val > 255 { val = 255 }
				bsp.Lighting = append(bsp.Lighting, byte(val))
			}
		}
	}
}

// luxelToWorld converts texture-space S/T coordinates to approximate world pos.
// For axis-aligned faces, inverts the texture projection:
//   s = VecS · pos + DistS  →  pos_component = (s - DistS) / VecS_component
func luxelToWorld(s, t float32, ti *BSPTexinfo, poly *Polygon) (float32, float32, float32) {
	// Face center as fallback / for the fixed axis.
	var cx, cy, cz float32
	for _, v := range poly.Verts {
		cx += v[0]
		cy += v[1]
		cz += v[2]
	}
	nv := float32(len(poly.Verts))
	cx /= nv; cy /= nv; cz /= nv

	// Determine which axes S and T map to from the texture vectors.
	wx, wy, wz := cx, cy, cz

	// S axis: find dominant component.
	if absf(ti.VecS[0]) > 0.01 {
		wx = (s - ti.DistS) / ti.VecS[0]
	} else if absf(ti.VecS[1]) > 0.01 {
		wy = (s - ti.DistS) / ti.VecS[1]
	} else if absf(ti.VecS[2]) > 0.01 {
		wz = (s - ti.DistS) / ti.VecS[2]
	}

	// T axis: find dominant component.
	if absf(ti.VecT[0]) > 0.01 {
		wx = (t - ti.DistT) / ti.VecT[0]
	} else if absf(ti.VecT[1]) > 0.01 {
		wy = (t - ti.DistT) / ti.VecT[1]
	} else if absf(ti.VecT[2]) > 0.01 {
		wz = (t - ti.DistT) / ti.VecT[2]
	}

	return wx, wy, wz
}

// --- Helper functions ---

func collectTexNames(polys []Polygon) []string {
	seen := map[string]bool{}
	var names []string
	for _, p := range polys {
		if !seen[p.TexName] {
			seen[p.TexName] = true
			names = append(names, p.TexName)
		}
	}
	return names
}

func buildMiptexLump(names []string) []byte {
	n := len(names)
	headerSize := 4 + 4*n // numtex + offsets

	var blobs [][]byte
	for _, name := range names {
		blobs = append(blobs, LookupMiptex(name))
	}

	var lump []byte
	lump = appendInt32(lump, int32(n))

	offset := headerSize
	for _, blob := range blobs {
		lump = appendInt32(lump, int32(offset))
		offset += len(blob)
	}

	for _, blob := range blobs {
		lump = append(lump, blob...)
	}

	return lump
}

func appendInt32(buf []byte, v int32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(v))
	return append(buf, b...)
}

func buildEntityString(mf *MapFile) string {
	var sb strings.Builder

	writeEnt := func(ent *Entity, modelIdx int) {
		sb.WriteString("{\n")
		for k, v := range ent.Properties {
			fmt.Fprintf(&sb, "\"%s\" \"%s\"\n", k, v)
		}
		if modelIdx > 0 {
			fmt.Fprintf(&sb, "\"model\" \"*%d\"\n", modelIdx)
		}
		sb.WriteString("}\n")
	}

	writeEnt(&mf.Worldspawn, 0)

	modelIdx := 1
	for i := range mf.Entities {
		if len(mf.Entities[i].Brushes) > 0 {
			writeEnt(&mf.Entities[i], modelIdx)
			modelIdx++
		} else {
			writeEnt(&mf.Entities[i], 0)
		}
	}

	return sb.String()
}

func computeBounds(verts []BSPVertex) ([3]float32, [3]float32) {
	mn := [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}
	mx := [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}
	for _, v := range verts {
		if v.X < mn[0] { mn[0] = v.X }
		if v.Y < mn[1] { mn[1] = v.Y }
		if v.Z < mn[2] { mn[2] = v.Z }
		if v.X > mx[0] { mx[0] = v.X }
		if v.Y > mx[1] { mx[1] = v.Y }
		if v.Z > mx[2] { mx[2] = v.Z }
	}
	return mn, mx
}

func computePolyBounds(polys []Polygon) ([3]float32, [3]float32) {
	mn := [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}
	mx := [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}
	for _, p := range polys {
		for _, v := range p.Verts {
			for a := range 3 {
				if v[a] < mn[a] { mn[a] = v[a] }
				if v[a] > mx[a] { mx[a] = v[a] }
			}
		}
	}
	return mn, mx
}
