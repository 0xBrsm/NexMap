package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// WallChunk is a reusable wall section extracted from a real Quake map.
// Stored as a 2D cross-section (width x height) with texture info.
type WallChunk struct {
	Source  string  // source map (e.g. "dm4")
	Texture string  // wall texture name
	Width   float32 // horizontal extent (Quake units)
	Height  float32 // vertical extent
	Depth   float32 // protrusion/recess depth (0 = flat)
	HasRecess bool  // wall has an indentation
	HasPillar bool  // wall has a protrusion
}

// FloorChunk is a reusable floor or ceiling section.
type FloorChunk struct {
	Source   string
	Texture  string
	Width    float32
	Height   float32
	Elevated bool    // has a step/platform
	StepH    float32 // step height if elevated
}

// MapChunkLibrary holds all extracted chunks from a set of source maps.
type MapChunkLibrary struct {
	Walls  []WallChunk
	Floors []FloorChunk
	// Per-map texture usage for palette extraction.
	MapPalettes map[string]*ExtractedPalette
}

// ExtractedPalette records which textures a map actually uses per surface.
type ExtractedPalette struct {
	MapName  string
	Walls    map[string]int // texture -> face count
	Floors   map[string]int
	Ceilings map[string]int
}

// bspFaceData is the parsed data for a single BSP face.
type bspFaceData struct {
	texture  string
	normal   [3]float32
	verts    [][3]float32
	area     float32
}

// ExtractChunksFromPAK reads all BSP maps from a PAK file and extracts chunks.
func ExtractChunksFromPAK(pakPath string, lib *MapChunkLibrary) error {
	pakData, err := os.ReadFile(pakPath)
	if err != nil {
		return err
	}
	if len(pakData) < 12 || string(pakData[:4]) != "PACK" {
		return fmt.Errorf("not a PAK file")
	}

	entries, err := parsePAK(pakData)
	if err != nil {
		return err
	}

	for _, ent := range entries {
		lower := strings.ToLower(ent.name)
		if !strings.HasPrefix(lower, "maps/") || !strings.HasSuffix(lower, ".bsp") {
			continue
		}
		base := strings.TrimSuffix(filepath.Base(lower), ".bsp")
		if strings.HasPrefix(base, "b_") {
			continue
		}
		if ent.offset+ent.size > len(pakData) {
			continue
		}

		bspData := pakData[ent.offset : ent.offset+ent.size]
		extractChunksFromBSP(bspData, base, lib)
	}

	return nil
}

func extractChunksFromBSP(bsp []byte, mapName string, lib *MapChunkLibrary) {
	if len(bsp) < 124 || binary.LittleEndian.Uint32(bsp[:4]) != 29 {
		return
	}

	lump := func(idx int) (int, int) {
		o := 4 + idx*8
		return int(binary.LittleEndian.Uint32(bsp[o:])), int(binary.LittleEndian.Uint32(bsp[o+4:]))
	}

	plOff, _ := lump(1)
	mipOff, mipSz := lump(2)
	vtxOff, _ := lump(3)
	tiOff, tiSz := lump(6)
	faceOff, faceSz := lump(7)
	edgeOff, _ := lump(12)
	seOff, _ := lump(13)

	// Parse texture names from miptex lump.
	texNames := map[int]string{}
	if mipOff+4 <= len(bsp) && mipSz >= 4 {
		ml := bsp[mipOff : mipOff+mipSz]
		nt := int(binary.LittleEndian.Uint32(ml[:4]))
		for i := range nt {
			if 4+i*4+4 > len(ml) {
				break
			}
			off := int(binary.LittleEndian.Uint32(ml[4+i*4:]))
			if off >= 0 && off+16 <= len(ml) {
				texNames[i] = strings.ToLower(cString(ml[off : off+16]))
			}
		}
	}

	numFaces := faceSz / 20
	numTI := tiSz / 40

	palette := &ExtractedPalette{
		MapName:  mapName,
		Walls:    map[string]int{},
		Floors:   map[string]int{},
		Ceilings: map[string]int{},
	}

	var wallFaces []bspFaceData
	var floorFaces []bspFaceData

	for fi := range numFaces {
		fo := faceOff + fi*20
		if fo+20 > len(bsp) {
			break
		}
		planeID := int(binary.LittleEndian.Uint16(bsp[fo:]))
		side := int(binary.LittleEndian.Uint16(bsp[fo+2:]))
		firstEdge := int(binary.LittleEndian.Uint32(bsp[fo+4:]))
		numEdges := int(binary.LittleEndian.Uint16(bsp[fo+8:]))
		texinfoID := int(binary.LittleEndian.Uint16(bsp[fo+10:]))

		if texinfoID >= numTI {
			continue
		}

		// Get texture name.
		to := tiOff + texinfoID*40
		if to+40 > len(bsp) {
			continue
		}
		miptexID := int(binary.LittleEndian.Uint32(bsp[to+32:]))
		texName := texNames[miptexID]
		if texName == "" || strings.HasPrefix(texName, "*") || texName == "trigger" || texName == "skip" || texName == "clip" {
			continue
		}

		// Get plane normal.
		po := plOff + planeID*20
		if po+20 > len(bsp) {
			continue
		}
		nx := math.Float32frombits(binary.LittleEndian.Uint32(bsp[po:]))
		ny := math.Float32frombits(binary.LittleEndian.Uint32(bsp[po+4:]))
		nz := math.Float32frombits(binary.LittleEndian.Uint32(bsp[po+8:]))
		if side != 0 {
			nx, ny, nz = -nx, -ny, -nz
		}

		// Get face vertices via surfedges → edges → vertices.
		verts := make([][3]float32, 0, numEdges)
		for ei := 0; ei < numEdges; ei++ {
			seIdx := firstEdge + ei
			seo := seOff + seIdx*4
			if seo+4 > len(bsp) {
				break
			}
			se := int(int32(binary.LittleEndian.Uint32(bsp[seo:])))
			var vidx int
			if se >= 0 {
				eo := edgeOff + se*4
				if eo+4 > len(bsp) {
					continue
				}
				vidx = int(binary.LittleEndian.Uint16(bsp[eo:]))
			} else {
				eo := edgeOff + (-se)*4
				if eo+4 > len(bsp) {
					continue
				}
				vidx = int(binary.LittleEndian.Uint16(bsp[eo+2:]))
			}
			vo := vtxOff + vidx*12
			if vo+12 > len(bsp) {
				continue
			}
			vx := math.Float32frombits(binary.LittleEndian.Uint32(bsp[vo:]))
			vy := math.Float32frombits(binary.LittleEndian.Uint32(bsp[vo+4:]))
			vz := math.Float32frombits(binary.LittleEndian.Uint32(bsp[vo+8:]))
			verts = append(verts, [3]float32{vx, vy, vz})
		}

		if len(verts) < 3 {
			continue
		}

		// Compute face bounding box.
		fd := bspFaceData{
			texture: texName,
			normal:  [3]float32{nx, ny, nz},
			verts:   verts,
		}

		// Classify by normal.
		anz := float32(math.Abs(float64(nz)))
		if anz > 0.7 {
			if nz > 0 {
				palette.Floors[texName]++
				floorFaces = append(floorFaces, fd)
			} else {
				palette.Ceilings[texName]++
			}
		} else {
			palette.Walls[texName]++
			wallFaces = append(wallFaces, fd)
		}
	}

	// Extract wall chunks: group by texture, compute dimensions.
	wallsByTex := map[string][]bspFaceData{}
	for _, f := range wallFaces {
		wallsByTex[f.texture] = append(wallsByTex[f.texture], f)
	}

	for tex, faces := range wallsByTex {
		// Aggregate stats for this texture's usage in this map.
		var totalW, totalH float32
		var minDepth, maxDepth float32
		count := 0

		for _, f := range faces {
			bb := faceBounds(f.verts)
			w := bb.maxX - bb.minX
			h := bb.maxZ - bb.minZ
			d := bb.maxY - bb.minY

			// Use the two larger dimensions as width/height.
			dims := []float32{w, h, d}
			sort.Slice(dims, func(i, j int) bool { return dims[i] > dims[j] })

			totalW += dims[0]
			totalH += dims[1]
			if dims[2] > maxDepth {
				maxDepth = dims[2]
			}
			if dims[2] < minDepth || count == 0 {
				minDepth = dims[2]
			}
			count++
		}

		if count == 0 {
			continue
		}

		avgW := totalW / float32(count)
		avgH := totalH / float32(count)

		lib.Walls = append(lib.Walls, WallChunk{
			Source:    mapName,
			Texture:   tex,
			Width:     avgW,
			Height:    avgH,
			Depth:     maxDepth - minDepth,
			HasRecess: maxDepth-minDepth > 8,
			HasPillar: maxDepth > 16,
		})
	}

	// Extract floor chunks similarly.
	floorsByTex := map[string][]bspFaceData{}
	for _, f := range floorFaces {
		floorsByTex[f.texture] = append(floorsByTex[f.texture], f)
	}

	for tex, faces := range floorsByTex {
		var totalW, totalH float32
		count := 0
		for _, f := range faces {
			bb := faceBounds(f.verts)
			w := bb.maxX - bb.minX
			h := bb.maxY - bb.minY
			totalW += w
			totalH += h
			count++
		}
		if count == 0 {
			continue
		}
		lib.Floors = append(lib.Floors, FloorChunk{
			Source:  mapName,
			Texture: tex,
			Width:   totalW / float32(count),
			Height:  totalH / float32(count),
		})
	}

	if lib.MapPalettes == nil {
		lib.MapPalettes = map[string]*ExtractedPalette{}
	}
	lib.MapPalettes[mapName] = palette
}

type faceBB struct {
	minX, minY, minZ float32
	maxX, maxY, maxZ float32
}

func faceBounds(verts [][3]float32) faceBB {
	bb := faceBB{
		minX: math.MaxFloat32, minY: math.MaxFloat32, minZ: math.MaxFloat32,
		maxX: -math.MaxFloat32, maxY: -math.MaxFloat32, maxZ: -math.MaxFloat32,
	}
	for _, v := range verts {
		if v[0] < bb.minX { bb.minX = v[0] }
		if v[1] < bb.minY { bb.minY = v[1] }
		if v[2] < bb.minZ { bb.minZ = v[2] }
		if v[0] > bb.maxX { bb.maxX = v[0] }
		if v[1] > bb.maxY { bb.maxY = v[1] }
		if v[2] > bb.maxZ { bb.maxZ = v[2] }
	}
	return bb
}

// --- Chunk selection for room building ---

// PickWallChunks selects wall chunks that fit a given wall length and height.
func PickWallChunks(lib *MapChunkLibrary, wallLen, wallH float32, mapFilter string) []WallChunk {
	var candidates []WallChunk
	for _, wc := range lib.Walls {
		if mapFilter != "" && wc.Source != mapFilter {
			continue
		}
		if wc.Width > 0 && wc.Height > 0 {
			candidates = append(candidates, wc)
		}
	}
	return candidates
}

// --- Library loading ---

func LoadChunkLibrary() (*MapChunkLibrary, error) {
	lib := &MapChunkLibrary{
		MapPalettes: map[string]*ExtractedPalette{},
	}

	// Load from shareware pak0.
	pak0Path := filepath.Join(cacheDir(), pak0Name)
	if _, err := os.Stat(pak0Path); err == nil {
		if err := ExtractChunksFromPAK(pak0Path, lib); err != nil {
			return nil, fmt.Errorf("pak0: %w", err)
		}
	}

	// Load from registered pak1 if available.
	pak1Paths := []string{
		filepath.Join(filepath.Dir(filepath.Dir(cacheDir())), "quake", "id1", "pak1.pak"),
		"/data/data/com.termux/files/home/quake/id1/pak1.pak",
	}
	for _, p := range pak1Paths {
		if _, err := os.Stat(p); err == nil {
			if err := ExtractChunksFromPAK(p, lib); err != nil {
				fmt.Fprintf(os.Stderr, "warning: pak1 %s: %v\n", p, err)
			}
			break
		}
	}

	fmt.Printf("chunk library: %d wall chunks, %d floor chunks from %d maps\n",
		len(lib.Walls), len(lib.Floors), len(lib.MapPalettes))

	return lib, nil
}

// PrintLibrarySummary shows what was extracted.
func (lib *MapChunkLibrary) PrintLibrarySummary() {
	fmt.Println("\n--- Chunk Library Summary ---")
	for mapName, pal := range lib.MapPalettes {
		topWall, topFloor, topCeil := "", "", ""
		topWN, topFN, topCN := 0, 0, 0
		for t, n := range pal.Walls { if n > topWN { topWall = t; topWN = n } }
		for t, n := range pal.Floors { if n > topFN { topFloor = t; topFN = n } }
		for t, n := range pal.Ceilings { if n > topCN { topCeil = t; topCN = n } }
		fmt.Printf("  %-8s walls=%-4d(%-16s) floors=%-4d(%-16s) ceils=%-4d(%s)\n",
			mapName, topWN, topWall, topFN, topFloor, topCN, topCeil)
	}

	// Wall chunks with recesses/pillars (the interesting ones).
	interesting := 0
	for _, wc := range lib.Walls {
		if wc.HasRecess || wc.HasPillar {
			interesting++
		}
	}
	fmt.Printf("\n  %d wall chunks with architectural features (recesses/pillars)\n", interesting)
}
