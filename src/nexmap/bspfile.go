package main

// BSP29 file format types (read-only; compilation is done by ericw-tools).
// Reference: Quake Specs v3.4, id Software GPL source.

const BSPVersion = 29

// Lump indices in header order.
const (
	LumpEntities     = 0
	LumpPlanes       = 1
	LumpMiptex       = 2
	LumpVertices     = 3
	LumpVisibility   = 4
	LumpNodes        = 5
	LumpTexinfo      = 6
	LumpFaces        = 7
	LumpLighting     = 8
	LumpClipnodes    = 9
	LumpLeaves       = 10
	LumpMarksurfaces = 11
	LumpEdges        = 12
	LumpSurfedges    = 13
	LumpModels       = 14
	NumLumps         = 15
)

// --- BSP data structures (match binary layout exactly) ---

type BSPPlane struct {
	Normal    [3]float32
	Dist      float32
	PlaneType int32
}

type BSPVertex struct {
	X, Y, Z float32
}

type BSPEdge struct {
	V0, V1 uint16
}

type BSPFace struct {
	PlaneID   uint16
	Side      uint16
	FirstEdge int32
	NumEdges  uint16
	TexinfoID uint16
	LightType uint8
	BaseLight uint8
	Light1    uint8
	Light2    uint8
	LightOfs  int32
}

type BSPTexinfo struct {
	VecS     [3]float32
	DistS    float32
	VecT     [3]float32
	DistT    float32
	MiptexID uint32
	Flags    uint32
}

type BSPModel struct {
	BBoxMin   [3]float32
	BBoxMax   [3]float32
	Origin    [3]float32
	HeadNodes [4]int32
	VisLeafs  int32
	FirstFace int32
	NumFaces  int32
}
