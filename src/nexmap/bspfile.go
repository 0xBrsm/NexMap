package main

// BSP29 file format types and binary writer for Quake 1.
// Reference: Quake Specs v3.4, id Software GPL source.

import (
	"encoding/binary"
	"io"
	"math"
)

const BSPVersion = 29

// Lump indices in header order.
const (
	LumpEntities    = 0
	LumpPlanes      = 1
	LumpMiptex      = 2
	LumpVertices    = 3
	LumpVisibility  = 4
	LumpNodes       = 5
	LumpTexinfo     = 6
	LumpFaces       = 7
	LumpLighting    = 8
	LumpClipnodes   = 9
	LumpLeaves      = 10
	LumpMarksurfaces = 11
	LumpEdges       = 12
	LumpSurfedges   = 13
	LumpModels      = 14
	NumLumps        = 15
)

// Leaf content types.
const (
	ContentsEmpty = -1
	ContentsSolid = -2
	ContentsWater = -3
	ContentsSlime = -4
	ContentsLava  = -5
	ContentsSky   = -6
)

// Plane axis types.
const (
	PlaneX    = 0
	PlaneY    = 1
	PlaneZ    = 2
	PlaneAnyX = 3
	PlaneAnyY = 4
	PlaneAnyZ = 5
)

// --- BSP data structures (match binary layout exactly) ---

type BSPPlane struct {
	Normal   [3]float32
	Dist     float32
	PlaneType int32
}

type BSPVertex struct {
	X, Y, Z float32
}

type BSPEdge struct {
	V0, V1 uint16
}

type BSPFace struct {
	PlaneID    uint16
	Side       uint16
	FirstEdge  int32
	NumEdges   uint16
	TexinfoID  uint16
	LightType  uint8
	BaseLight  uint8
	Light1     uint8
	Light2     uint8
	LightOfs   int32
}

type BSPTexinfo struct {
	VecS    [3]float32
	DistS   float32
	VecT    [3]float32
	DistT   float32
	MiptexID uint32
	Flags    uint32
}

type BSPNode struct {
	PlaneID   int32
	Children  [2]int16
	BBoxMin   [3]int16
	BBoxMax   [3]int16
	FirstFace uint16
	NumFaces  uint16
}

type BSPLeaf struct {
	Contents  int32
	VisOfs    int32
	BBoxMin   [3]int16
	BBoxMax   [3]int16
	FirstMark uint16
	NumMarks  uint16
	Ambient   [4]uint8
}

type BSPClipnode struct {
	PlaneID  int32
	Children [2]int16
}

type BSPModel struct {
	BBoxMin    [3]float32
	BBoxMax    [3]float32
	Origin     [3]float32
	HeadNodes  [4]int32
	VisLeafs   int32
	FirstFace  int32
	NumFaces   int32
}

// --- Aggregate BSP file ---

type BSPData struct {
	Entities    string
	Planes      []BSPPlane
	MiptexData  []byte // Pre-built miptex lump (header + textures)
	Vertices    []BSPVertex
	Visibility  []byte
	Nodes       []BSPNode
	Texinfo     []BSPTexinfo
	Faces       []BSPFace
	Lighting    []byte
	Clipnodes   []BSPClipnode
	Leaves      []BSPLeaf
	Marksurfaces []uint16
	Edges       []BSPEdge
	Surfedges   []int32
	Models      []BSPModel

	// Internal: clip hull head node indices (set during buildClipHulls).
	hull1Head int32
	hull2Head int32
}

func (bsp *BSPData) AddVertex(x, y, z float32) int {
	idx := len(bsp.Vertices)
	bsp.Vertices = append(bsp.Vertices, BSPVertex{x, y, z})
	return idx
}

func (bsp *BSPData) AddPlane(nx, ny, nz, dist float32) int {
	pt := int32(PlaneAnyX)
	anx := float32(math.Abs(float64(nx)))
	any := float32(math.Abs(float64(ny)))
	anz := float32(math.Abs(float64(nz)))
	if anx == 1 {
		pt = PlaneX
	} else if any == 1 {
		pt = PlaneY
	} else if anz == 1 {
		pt = PlaneZ
	} else if anx >= any && anx >= anz {
		pt = PlaneAnyX
	} else if any >= anx && any >= anz {
		pt = PlaneAnyY
	} else {
		pt = PlaneAnyZ
	}
	idx := len(bsp.Planes)
	bsp.Planes = append(bsp.Planes, BSPPlane{
		Normal: [3]float32{nx, ny, nz}, Dist: dist, PlaneType: pt,
	})
	return idx
}

func (bsp *BSPData) AddEdge(v0, v1 int) int {
	idx := len(bsp.Edges)
	bsp.Edges = append(bsp.Edges, BSPEdge{uint16(v0), uint16(v1)})
	return idx
}

func (bsp *BSPData) AddSurfedge(edgeIdx int, reversed bool) int {
	idx := len(bsp.Surfedges)
	val := int32(edgeIdx)
	if reversed {
		val = -val
	}
	bsp.Surfedges = append(bsp.Surfedges, val)
	return idx
}

// WriteBSP writes the complete BSP29 binary file.
func WriteBSP(w io.Writer, bsp *BSPData) error {
	// Serialize each lump to bytes.
	lumps := make([][]byte, NumLumps)
	lumps[LumpEntities] = append([]byte(bsp.Entities), 0) // null-terminated
	lumps[LumpPlanes] = marshalSlice(bsp.Planes)
	lumps[LumpMiptex] = bsp.MiptexData
	lumps[LumpVertices] = marshalSlice(bsp.Vertices)
	lumps[LumpVisibility] = bsp.Visibility
	lumps[LumpNodes] = marshalSlice(bsp.Nodes)
	lumps[LumpTexinfo] = marshalSlice(bsp.Texinfo)
	lumps[LumpFaces] = marshalSlice(bsp.Faces)
	lumps[LumpLighting] = bsp.Lighting
	lumps[LumpClipnodes] = marshalSlice(bsp.Clipnodes)
	lumps[LumpLeaves] = marshalSlice(bsp.Leaves)
	lumps[LumpMarksurfaces] = marshalSlice(bsp.Marksurfaces)
	lumps[LumpEdges] = marshalSlice(bsp.Edges)
	lumps[LumpSurfedges] = marshalSlice(bsp.Surfedges)
	lumps[LumpModels] = marshalSlice(bsp.Models)

	// Header: version(4) + 15 * (offset(4) + size(4)) = 124 bytes.
	headerSize := 4 + NumLumps*8
	offset := headerSize

	type lumpDir struct {
		Offset int32
		Size   int32
	}
	dirs := make([]lumpDir, NumLumps)
	for i := range NumLumps {
		sz := 0
		if lumps[i] != nil {
			sz = len(lumps[i])
		}
		dirs[i] = lumpDir{int32(offset), int32(sz)}
		offset += sz
		// Align to 4 bytes.
		if offset%4 != 0 {
			offset += 4 - offset%4
		}
	}

	// Write header.
	if err := binary.Write(w, binary.LittleEndian, int32(BSPVersion)); err != nil {
		return err
	}
	for _, d := range dirs {
		if err := binary.Write(w, binary.LittleEndian, d); err != nil {
			return err
		}
	}

	// Write lumps.
	written := headerSize
	for i := range NumLumps {
		if lumps[i] != nil {
			if _, err := w.Write(lumps[i]); err != nil {
				return err
			}
			written += len(lumps[i])
		}
		// Pad to 4-byte alignment.
		pad := 0
		if written%4 != 0 {
			pad = 4 - written%4
		}
		if pad > 0 {
			if _, err := w.Write(make([]byte, pad)); err != nil {
				return err
			}
			written += pad
		}
	}

	return nil
}

// marshalSlice serializes a slice of fixed-size structs to little-endian bytes.
func marshalSlice[T any](s []T) []byte {
	if len(s) == 0 {
		return nil
	}
	var zero T
	elemSize := binary.Size(zero)
	buf := make([]byte, 0, len(s)*elemSize)
	for i := range s {
		tmp := make([]byte, elemSize)
		w := &bytesWriter{buf: tmp}
		binary.Write(w, binary.LittleEndian, s[i])
		buf = append(buf, tmp...)
	}
	return buf
}

type bytesWriter struct {
	buf []byte
	pos int
}

func (bw *bytesWriter) Write(p []byte) (int, error) {
	n := copy(bw.buf[bw.pos:], p)
	bw.pos += n
	return n, nil
}
