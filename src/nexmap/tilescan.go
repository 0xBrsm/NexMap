package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"strings"
)

// MapTile is a spatial cell extracted from a real Quake BSP.
// Represents a 2D column of space with floor/ceiling and wall info.
type MapTile struct {
	Source string // source map name
	// Grid position in source (for reference only)
	GridX, GridY int
	// Geometry
	FloorZ float32
	CeilZ  float32
	Empty  bool // true if this cell is open (playable space)
	// Which sides are open (have openings to adjacent cells)
	OpenN, OpenS, OpenE, OpenW bool
	// Textures observed in this cell
	WallTex  string
	FloorTex string
	CeilTex  string
}

const tileSize = 256 // Quake units per tile cell

// ScanMapTiles reads a BSP and divides it into a grid of tiles.
func ScanMapTiles(bsp []byte, mapName string) []MapTile {
	if len(bsp) < 124 || binary.LittleEndian.Uint32(bsp[:4]) != 29 {
		return nil
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

	// Parse texture names.
	texNames := map[int]string{}
	if mipOff+4 <= len(bsp) && mipSz >= 4 {
		ml := bsp[mipOff : mipOff+mipSz]
		nt := int(binary.LittleEndian.Uint32(ml[:4]))
		for j := range nt {
			if 4+j*4+4 > len(ml) { break }
			off := int(binary.LittleEndian.Uint32(ml[4+j*4:]))
			if off >= 0 && off+16 <= len(ml) {
				texNames[j] = strings.ToLower(cString(ml[off : off+16]))
			}
		}
	}

	// Parse all faces with their vertices, normals, and textures.
	type faceInfo struct {
		verts   [][3]float32
		nx, ny, nz float32
		texture string
	}
	var faces []faceInfo

	for fi := range faceSz / 20 {
		fo := faceOff + fi*20
		if fo+20 > len(bsp) { break }
		planeID := int(binary.LittleEndian.Uint16(bsp[fo:]))
		side := int(binary.LittleEndian.Uint16(bsp[fo+2:]))
		firstEdge := int(binary.LittleEndian.Uint32(bsp[fo+4:]))
		numEdges := int(binary.LittleEndian.Uint16(bsp[fo+8:]))
		texinfoID := int(binary.LittleEndian.Uint16(bsp[fo+10:]))

		if texinfoID >= tiSz/40 { continue }
		to := tiOff + texinfoID*40
		if to+40 > len(bsp) { continue }
		mid := int(binary.LittleEndian.Uint32(bsp[to+32:]))
		tn := texNames[mid]
		if tn == "" || tn == "trigger" || tn == "clip" || tn == "skip" { continue }

		po := plOff + planeID*20
		if po+20 > len(bsp) { continue }
		nx := math.Float32frombits(binary.LittleEndian.Uint32(bsp[po:]))
		ny := math.Float32frombits(binary.LittleEndian.Uint32(bsp[po+4:]))
		nz := math.Float32frombits(binary.LittleEndian.Uint32(bsp[po+8:]))
		if side != 0 { nx, ny, nz = -nx, -ny, -nz }

		verts := make([][3]float32, 0, numEdges)
		for ei := 0; ei < numEdges; ei++ {
			seo := seOff + (firstEdge+ei)*4
			if seo+4 > len(bsp) { break }
			se := int(int32(binary.LittleEndian.Uint32(bsp[seo:])))
			var vidx int
			if se >= 0 {
				eo := edgeOff + se*4
				if eo+4 <= len(bsp) { vidx = int(binary.LittleEndian.Uint16(bsp[eo:])) }
			} else {
				eo := edgeOff + (-se)*4
				if eo+4 <= len(bsp) { vidx = int(binary.LittleEndian.Uint16(bsp[eo+2:])) }
			}
			vo := vtxOff + vidx*12
			if vo+12 > len(bsp) { continue }
			vx := math.Float32frombits(binary.LittleEndian.Uint32(bsp[vo:]))
			vy := math.Float32frombits(binary.LittleEndian.Uint32(bsp[vo+4:]))
			vz := math.Float32frombits(binary.LittleEndian.Uint32(bsp[vo+8:]))
			verts = append(verts, [3]float32{vx, vy, vz})
		}
		if len(verts) < 3 { continue }

		faces = append(faces, faceInfo{verts: verts, nx: nx, ny: ny, nz: nz, texture: tn})
	}

	// Find world bounds from all face vertices.
	var minX, minY, maxX, maxY float32
	minX, minY = math.MaxFloat32, math.MaxFloat32
	maxX, maxY = -math.MaxFloat32, -math.MaxFloat32
	for _, f := range faces {
		for _, v := range f.verts {
			if v[0] < minX { minX = v[0] }
			if v[1] < minY { minY = v[1] }
			if v[0] > maxX { maxX = v[0] }
			if v[1] > maxY { maxY = v[1] }
		}
	}

	// Snap to tile grid.
	gridMinX := int(math.Floor(float64(minX) / tileSize))
	gridMinY := int(math.Floor(float64(minY) / tileSize))
	gridMaxX := int(math.Ceil(float64(maxX) / tileSize))
	gridMaxY := int(math.Ceil(float64(maxY) / tileSize))

	// For each cell, collect faces that overlap it.
	type cellKey struct{ gx, gy int }
	cellFaces := map[cellKey][]faceInfo{}

	for _, f := range faces {
		// Find which cells this face overlaps.
		var fMinX, fMinY, fMaxX, fMaxY float32
		fMinX, fMinY = math.MaxFloat32, math.MaxFloat32
		fMaxX, fMaxY = -math.MaxFloat32, -math.MaxFloat32
		for _, v := range f.verts {
			if v[0] < fMinX { fMinX = v[0] }
			if v[1] < fMinY { fMinY = v[1] }
			if v[0] > fMaxX { fMaxX = v[0] }
			if v[1] > fMaxY { fMaxY = v[1] }
		}
		gx0 := int(math.Floor(float64(fMinX) / tileSize))
		gy0 := int(math.Floor(float64(fMinY) / tileSize))
		gx1 := int(math.Ceil(float64(fMaxX) / tileSize))
		gy1 := int(math.Ceil(float64(fMaxY) / tileSize))
		for gx := gx0; gx < gx1; gx++ {
			for gy := gy0; gy < gy1; gy++ {
				k := cellKey{gx, gy}
				cellFaces[k] = append(cellFaces[k], f)
			}
		}
	}

	// Build tiles from cell data.
	var tiles []MapTile
	for gx := gridMinX; gx < gridMaxX; gx++ {
		for gy := gridMinY; gy < gridMaxY; gy++ {
			k := cellKey{gx, gy}
			cFaces := cellFaces[k]

			// No faces = solid cell.
			if len(cFaces) == 0 {
				continue
			}

			tile := MapTile{
				Source: mapName,
				GridX:  gx, GridY: gy,
				Empty:  true,
			}

			// Compute floor/ceiling from horizontal faces.
			var floorZ, ceilZ float32
			floorZ = math.MaxFloat32
			ceilZ = -math.MaxFloat32
			var wallTex, floorTex, ceilTex string
			wallCount, floorCount, ceilCount := map[string]int{}, map[string]int{}, map[string]int{}

			for _, f := range cFaces {
				anz := float32(math.Abs(float64(f.nz)))
				if anz > 0.7 {
					// Horizontal face.
					for _, v := range f.verts {
						if f.nz > 0 && v[2] < floorZ { floorZ = v[2] }
						if f.nz < 0 && v[2] > ceilZ { ceilZ = v[2] }
					}
					if f.nz > 0 && !strings.HasPrefix(f.texture, "*") {
						floorCount[f.texture]++
					} else if f.nz < 0 {
						ceilCount[f.texture]++
					}
				} else {
					// Vertical face (wall).
					if !strings.HasPrefix(f.texture, "*") {
						wallCount[f.texture]++
					}

					// Check if wall is on a cell boundary → that side is walled.
					cellX0 := float32(gx * tileSize)
					cellY0 := float32(gy * tileSize)
					cellX1 := cellX0 + tileSize
					cellY1 := cellY0 + tileSize

					anx := float32(math.Abs(float64(f.nx)))
					any := float32(math.Abs(float64(f.ny)))

					if anx > 0.7 {
						// X-facing wall: check if it's on the E or W boundary.
						for _, v := range f.verts {
							if math.Abs(float64(v[0]-cellX0)) < 4 { tile.OpenW = false }
							if math.Abs(float64(v[0]-cellX1)) < 4 { tile.OpenE = false }
						}
					}
					if any > 0.7 {
						for _, v := range f.verts {
							if math.Abs(float64(v[1]-cellY0)) < 4 { tile.OpenS = false }
							if math.Abs(float64(v[1]-cellY1)) < 4 { tile.OpenN = false }
						}
					}
				}
			}

			// Default: sides are open unless a wall was found on that boundary.
			// (We initialized Open* to false, walls on boundaries keep them false,
			// absence of walls means the side is open.)
			// Actually reverse: assume open, walls close them.
			tile.OpenN = true; tile.OpenS = true; tile.OpenE = true; tile.OpenW = true
			for _, f := range cFaces {
				anx := float32(math.Abs(float64(f.nx)))
				any := float32(math.Abs(float64(f.ny)))
				anz := float32(math.Abs(float64(f.nz)))
				if anz > 0.7 { continue } // skip floors/ceilings

				cellX0 := float32(gx * tileSize)
				cellY0 := float32(gy * tileSize)
				cellX1 := cellX0 + tileSize
				cellY1 := cellY0 + tileSize

				if anx > 0.7 {
					for _, v := range f.verts {
						if math.Abs(float64(v[0]-cellX0)) < 8 { tile.OpenW = false }
						if math.Abs(float64(v[0]-cellX1)) < 8 { tile.OpenE = false }
					}
				}
				if any > 0.7 {
					for _, v := range f.verts {
						if math.Abs(float64(v[1]-cellY0)) < 8 { tile.OpenS = false }
						if math.Abs(float64(v[1]-cellY1)) < 8 { tile.OpenN = false }
					}
				}
			}

			if floorZ == math.MaxFloat32 { floorZ = 0 }
			if ceilZ == -math.MaxFloat32 { ceilZ = 256 }
			tile.FloorZ = floorZ
			tile.CeilZ = ceilZ

			// Pick most common texture per surface.
			topOf := func(m map[string]int) string {
				best, bestN := "", 0
				for k, v := range m { if v > bestN { best = k; bestN = v } }
				return best
			}
			wallTex = topOf(wallCount)
			floorTex = topOf(floorCount)
			ceilTex = topOf(ceilCount)
			if wallTex == "" { wallTex = "mmetal1_3" }
			if floorTex == "" { floorTex = wallTex }
			if ceilTex == "" { ceilTex = wallTex }
			tile.WallTex = wallTex
			tile.FloorTex = floorTex
			tile.CeilTex = ceilTex

			tiles = append(tiles, tile)
		}
	}

	return tiles
}

// --- Tile-based map assembly ---

// TileMap is a grid of tiles arranged for a new map.
type TileMap struct {
	Width, Height int
	Tiles         [][]MapTile // [x][y]
}

// AssembleTileMap picks tiles from a source map and arranges them
// in a new configuration, respecting open/closed side constraints.
func AssembleTileMap(rng *rand.Rand, sourceTiles []MapTile, width, height int) *TileMap {
	tm := &TileMap{
		Width: width, Height: height,
		Tiles: make([][]MapTile, width),
	}
	for x := range width {
		tm.Tiles[x] = make([]MapTile, height)
	}

	// Separate tiles into open (playable) and edge sets.
	var openTiles []MapTile
	for _, t := range sourceTiles {
		if t.Empty {
			openTiles = append(openTiles, t)
		}
	}
	if len(openTiles) == 0 {
		return tm
	}

	// Simple placement: for each grid cell, pick a compatible tile.
	for x := range width {
		for y := range height {
			// Find tiles that are compatible with already-placed neighbors.
			var candidates []MapTile
			for _, t := range openTiles {
				if isCompatible(tm, x, y, t) {
					candidates = append(candidates, t)
				}
			}
			if len(candidates) == 0 {
				// Fallback: any open tile.
				candidates = openTiles
			}
			tm.Tiles[x][y] = candidates[rng.IntN(len(candidates))]
		}
	}

	return tm
}

func isCompatible(tm *TileMap, x, y int, tile MapTile) bool {
	// Check compatibility with placed neighbors.
	// West neighbor: our W must match their E.
	if x > 0 {
		west := tm.Tiles[x-1][y]
		if west.Empty {
			if tile.OpenW != west.OpenE { return false }
		}
	}
	// South neighbor.
	if y > 0 {
		south := tm.Tiles[x][y-1]
		if south.Empty {
			if tile.OpenS != south.OpenN { return false }
		}
	}
	return true
}

// EmitTileMapBrushes converts a TileMap to .map brushes.
func EmitTileMapBrushes(m *MapFile, tm *TileMap) {
	originX := -(tm.Width * tileSize) / 2
	originY := -(tm.Height * tileSize) / 2

	for gx := range tm.Width {
		for gy := range tm.Height {
			tile := &tm.Tiles[gx][gy]
			if !tile.Empty { continue }

			x0 := originX + gx*tileSize
			y0 := originY + gy*tileSize
			x1 := x0 + tileSize
			y1 := y0 + tileSize
			z0 := int(tile.FloorZ)
			z1 := int(tile.CeilZ)
			if z1-z0 < 64 { z1 = z0 + 192 }

			// Floor.
			m.AddBrush(AxisAlignedBox(x0, y0, z0-16, x1, y1, z0, tile.FloorTex))
			// Ceiling.
			m.AddBrush(AxisAlignedBox(x0, y0, z1, x1, y1, z1+16, tile.CeilTex))

			// Walls on closed sides.
			wallD := 16
			if !tile.OpenW { m.AddBrush(AxisAlignedBox(x0-wallD, y0, z0, x0, y1, z1, tile.WallTex)) }
			if !tile.OpenE { m.AddBrush(AxisAlignedBox(x1, y0, z0, x1+wallD, y1, z1, tile.WallTex)) }
			if !tile.OpenS { m.AddBrush(AxisAlignedBox(x0, y0-wallD, z0, x1, y0, z1, tile.WallTex)) }
			if !tile.OpenN { m.AddBrush(AxisAlignedBox(x0, y1, z0, x1, y1+wallD, z1, tile.WallTex)) }
		}
	}

	// Outer shell.
	shellX0 := originX - 32
	shellY0 := originY - 32
	shellX1 := originX + tm.Width*tileSize + 32
	shellY1 := originY + tm.Height*tileSize + 32
	zLo := -256
	zHi := 512
	s := 32
	tex := "metal1_1"
	m.AddBrush(AxisAlignedBox(shellX0-s, shellY0-s, zLo-s, shellX1+s, shellY1+s, zLo, tex))
	m.AddBrush(AxisAlignedBox(shellX0-s, shellY0-s, zHi, shellX1+s, shellY1+s, zHi+s, tex))
	m.AddBrush(AxisAlignedBox(shellX0-s, shellY0-s, zLo, shellX0, shellY1+s, zHi, tex))
	m.AddBrush(AxisAlignedBox(shellX1, shellY0-s, zLo, shellX1+s, shellY1+s, zHi, tex))
	m.AddBrush(AxisAlignedBox(shellX0, shellY0-s, zLo, shellX1, shellY0, zHi, tex))
	m.AddBrush(AxisAlignedBox(shellX0, shellY1, zLo, shellX1, shellY1+s, zHi, tex))
}

// ScanMapFromPAK extracts tiles from a named map in a PAK file.
func ScanMapFromPAK(pakPath, mapName string) ([]MapTile, error) {
	pakData, err := os.ReadFile(pakPath)
	if err != nil { return nil, err }
	entries, err := parsePAK(pakData)
	if err != nil { return nil, err }

	target := "maps/" + strings.ToLower(mapName) + ".bsp"
	for _, ent := range entries {
		if strings.ToLower(ent.name) == target {
			if ent.offset+ent.size > len(pakData) { continue }
			bsp := pakData[ent.offset : ent.offset+ent.size]
			tiles := ScanMapTiles(bsp, mapName)
			return tiles, nil
		}
	}
	return nil, fmt.Errorf("map %s not found in %s", mapName, pakPath)
}
