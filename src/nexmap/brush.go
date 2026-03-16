package main

import (
	"fmt"
	"io"
	"math"
)

// Quake engine lightmap limit.
const (
	maxLuxels = 16
	luxelSize = 16
)

func safeScale(extent int) float64 {
	if extent <= maxLuxels*luxelSize {
		return 1.0
	}
	return math.Ceil(float64(extent) / float64(maxLuxels*luxelSize))
}

// Plane defines one face of a brush via three clockwise points + texture.
type Plane struct {
	P1, P2, P3         [3]int
	Texture            string
	XOff, YOff         int
	Rotation           float64
	XScale, YScale     float64
}

func (p *Plane) Write(w io.Writer) {
	pt := func(v [3]int) string { return fmt.Sprintf("( %d %d %d )", v[0], v[1], v[2]) }
	fmt.Fprintf(w, "%s %s %s %s %d %d %g %g %g\n",
		pt(p.P1), pt(p.P2), pt(p.P3),
		p.Texture, p.XOff, p.YOff,
		p.Rotation, p.XScale, p.YScale)
}

// Brush is a convex solid defined by the intersection of half-spaces.
type Brush struct {
	Planes []Plane
}

func (b *Brush) Write(w io.Writer) {
	fmt.Fprintln(w, "{")
	for i := range b.Planes {
		b.Planes[i].Write(w)
	}
	fmt.Fprintln(w, "}")
}

// AxisAlignedBox creates a 6-plane brush with safe texture scales.
func AxisAlignedBox(minX, minY, minZ, maxX, maxY, maxZ int, texture string) Brush {
	dx := maxX - minX
	dy := maxY - minY
	dz := maxZ - minZ

	sxYZ := [2]float64{safeScale(dy), safeScale(dz)}
	syXZ := [2]float64{safeScale(dx), safeScale(dz)}
	szXY := [2]float64{safeScale(dx), safeScale(dy)}

	return Brush{Planes: []Plane{
		{P1: [3]int{minX, 0, 0}, P2: [3]int{minX, 1, 0}, P3: [3]int{minX, 0, 1}, Texture: texture, XScale: sxYZ[0], YScale: sxYZ[1]},
		{P1: [3]int{maxX, 0, 0}, P2: [3]int{maxX, 0, 1}, P3: [3]int{maxX, 1, 0}, Texture: texture, XScale: sxYZ[0], YScale: sxYZ[1]},
		{P1: [3]int{0, minY, 0}, P2: [3]int{0, minY, 1}, P3: [3]int{1, minY, 0}, Texture: texture, XScale: syXZ[0], YScale: syXZ[1]},
		{P1: [3]int{0, maxY, 0}, P2: [3]int{1, maxY, 0}, P3: [3]int{0, maxY, 1}, Texture: texture, XScale: syXZ[0], YScale: syXZ[1]},
		{P1: [3]int{0, 0, minZ}, P2: [3]int{1, 0, minZ}, P3: [3]int{0, 1, minZ}, Texture: texture, XScale: szXY[0], YScale: szXY[1]},
		{P1: [3]int{0, 0, maxZ}, P2: [3]int{0, 1, maxZ}, P3: [3]int{1, 0, maxZ}, Texture: texture, XScale: szXY[0], YScale: szXY[1]},
	}}
}

// Entity is a Quake entity with key-value properties and optional brushes.
type Entity struct {
	Properties map[string]string
	Brushes    []Brush
}

func (e *Entity) Write(w io.Writer) {
	fmt.Fprintln(w, "{")
	for k, v := range e.Properties {
		fmt.Fprintf(w, "\"%s\" \"%s\"\n", k, v)
	}
	for i := range e.Brushes {
		e.Brushes[i].Write(w)
	}
	fmt.Fprintln(w, "}")
}

// MapFile is a complete .map: worldspawn + point entities.
type MapFile struct {
	Worldspawn Entity
	Entities   []Entity
}

func NewMapFile() *MapFile {
	return &MapFile{
		Worldspawn: Entity{
			Properties: map[string]string{
				"classname": "worldspawn",
				"wad":       DefaultTextureWAD,
				"worldtype": "2",
			},
		},
	}
}

func (m *MapFile) AddBrush(b Brush) {
	m.Worldspawn.Brushes = append(m.Worldspawn.Brushes, b)
}

func (m *MapFile) AddEntity(classname string, x, y, z int, props map[string]string) {
	ent := Entity{Properties: map[string]string{
		"classname": classname,
		"origin":    fmt.Sprintf("%d %d %d", x, y, z),
	}}
	for k, v := range props {
		ent.Properties[k] = v
	}
	m.Entities = append(m.Entities, ent)
}

func (m *MapFile) AddLight(x, y, z, brightness int) {
	m.AddEntity("light", x, y, z, map[string]string{"light": fmt.Sprintf("%d", brightness)})
}

func (m *MapFile) Write(w io.Writer) {
	m.Worldspawn.Write(w)
	for i := range m.Entities {
		m.Entities[i].Write(w)
	}
}
