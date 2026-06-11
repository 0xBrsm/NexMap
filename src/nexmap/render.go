package main

// Headless software renderer for BSP29 files: lightmapped, textured,
// z-buffered, no GPU or display required. Output is PNG screenshots.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const texSpecial = 1 // texinfo flag: no lightmap (liquids, sky)

type RenderTex struct {
	Name   string
	Width  int
	Height int
	Pixels []byte // mip0, palette indices
}

type RenderBSP struct {
	Entities   string
	Planes     []BSPPlane
	Vertices   []BSPVertex
	Texinfo    []BSPTexinfo
	Faces      []BSPFace
	Lighting   []byte
	Edges      []BSPEdge
	Surfedges  []int32
	Models     []BSPModel
	Textures   []RenderTex
}

func LoadRenderBSP(path string) (*RenderBSP, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 4+NumLumps*8 {
		return nil, fmt.Errorf("truncated BSP")
	}
	version := int32(binary.LittleEndian.Uint32(data[0:4]))
	if version != BSPVersion {
		return nil, fmt.Errorf("unsupported BSP version %d", version)
	}
	lump := func(i int) []byte {
		off := binary.LittleEndian.Uint32(data[4+i*8:])
		size := binary.LittleEndian.Uint32(data[8+i*8:])
		return data[off : off+size]
	}
	readSlice := func(i int, elemSize int, out any) error {
		b := lump(i)
		n := len(b) / elemSize
		return binary.Read(bytes.NewReader(b[:n*elemSize]), binary.LittleEndian, out)
	}

	bsp := &RenderBSP{Lighting: lump(LumpLighting)}
	bsp.Entities = string(bytes.TrimRight(lump(LumpEntities), "\x00"))

	planes := lump(LumpPlanes)
	bsp.Planes = make([]BSPPlane, len(planes)/20)
	if err := readSlice(LumpPlanes, 20, bsp.Planes); err != nil {
		return nil, err
	}
	verts := lump(LumpVertices)
	bsp.Vertices = make([]BSPVertex, len(verts)/12)
	if err := readSlice(LumpVertices, 12, bsp.Vertices); err != nil {
		return nil, err
	}
	ti := lump(LumpTexinfo)
	bsp.Texinfo = make([]BSPTexinfo, len(ti)/40)
	if err := readSlice(LumpTexinfo, 40, bsp.Texinfo); err != nil {
		return nil, err
	}
	faces := lump(LumpFaces)
	bsp.Faces = make([]BSPFace, len(faces)/20)
	if err := readSlice(LumpFaces, 20, bsp.Faces); err != nil {
		return nil, err
	}
	edges := lump(LumpEdges)
	bsp.Edges = make([]BSPEdge, len(edges)/4)
	if err := readSlice(LumpEdges, 4, bsp.Edges); err != nil {
		return nil, err
	}
	se := lump(LumpSurfedges)
	bsp.Surfedges = make([]int32, len(se)/4)
	if err := readSlice(LumpSurfedges, 4, bsp.Surfedges); err != nil {
		return nil, err
	}
	models := lump(LumpModels)
	bsp.Models = make([]BSPModel, len(models)/64)
	if err := readSlice(LumpModels, 64, bsp.Models); err != nil {
		return nil, err
	}
	// Miptex lump: nummiptex, offsets, then miptex headers + pixel data.
	mt := lump(LumpMiptex)
	if len(mt) >= 4 {
		num := int(int32(binary.LittleEndian.Uint32(mt)))
		bsp.Textures = make([]RenderTex, num)
		for i := 0; i < num; i++ {
			off := int(int32(binary.LittleEndian.Uint32(mt[4+i*4:])))
			if off < 0 || off+40 > len(mt) {
				continue
			}
			h := mt[off:]
			name := cString(h[:16])
			w := int(binary.LittleEndian.Uint32(h[16:]))
			ht := int(binary.LittleEndian.Uint32(h[20:]))
			mip0 := int(binary.LittleEndian.Uint32(h[24:]))
			if w <= 0 || ht <= 0 || off+mip0+w*ht > len(mt) {
				continue
			}
			bsp.Textures[i] = RenderTex{name, w, ht, h[mip0 : mip0+w*ht]}
		}
	}
	return bsp, nil
}

// LoadQuakePalette reads gfx/palette.lmp from the cached pak0.pak.
func LoadQuakePalette() ([256][3]uint8, error) {
	var pal [256][3]uint8
	pakData, err := os.ReadFile(filepath.Join(cacheDir(), pak0Name))
	if err != nil {
		return pal, err
	}
	entries, err := parsePAK(pakData)
	if err != nil {
		return pal, err
	}
	for _, e := range entries {
		if e.name == "gfx/palette.lmp" && e.size >= 768 {
			for i := 0; i < 256; i++ {
				pal[i] = [3]uint8{
					pakData[e.offset+i*3],
					pakData[e.offset+i*3+1],
					pakData[e.offset+i*3+2],
				}
			}
			return pal, nil
		}
	}
	return pal, fmt.Errorf("gfx/palette.lmp not found in pak0")
}

// parseEntities parses the BSP entity lump into key/value maps.
func parseEntities(s string) []map[string]string {
	var result []map[string]string
	var current map[string]string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "{" {
			current = map[string]string{}
		} else if line == "}" {
			if current != nil {
				result = append(result, current)
				current = nil
			}
		} else if current != nil {
			parts := strings.SplitN(line, "\" \"", 2)
			if len(parts) == 2 {
				current[strings.Trim(parts[0], "\" ")] = strings.Trim(parts[1], "\" ")
			}
		}
	}
	return result
}

type Vec3 struct{ X, Y, Z float64 }

func (a Vec3) Sub(b Vec3) Vec3   { return Vec3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a Vec3) Dot(b Vec3) float64 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func (a Vec3) Cross(b Vec3) Vec3 {
	return Vec3{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X}
}

type Camera struct {
	Pos   Vec3
	Yaw   float64 // degrees, 0 = +x
	Pitch float64 // degrees, positive = up
	FovX  float64 // degrees
}

type rvert struct {
	view Vec3    // view space (x right, y up, z depth)
	s, t float64 // texture coords (texels)
}

type renderer struct {
	bsp    *RenderBSP
	pal    [256][3]uint8
	w, h   int
	gamma  float64
	img    *image.RGBA
	zbuf   []float64
}

// RenderShot renders one camera view to an RGBA image.
func RenderShot(bsp *RenderBSP, pal [256][3]uint8, cam Camera, w, h int, gamma float64) *image.RGBA {
	r := &renderer{bsp: bsp, pal: pal, w: w, h: h, gamma: gamma}
	r.img = image.NewRGBA(image.Rect(0, 0, w, h))
	r.zbuf = make([]float64, w*h)
	for i := range r.zbuf {
		r.zbuf[i] = math.Inf(1)
	}

	yaw := cam.Yaw * math.Pi / 180
	pitch := cam.Pitch * math.Pi / 180
	forward := Vec3{math.Cos(yaw) * math.Cos(pitch), math.Sin(yaw) * math.Cos(pitch), math.Sin(pitch)}
	right := Vec3{math.Sin(yaw), -math.Cos(yaw), 0}
	up := right.Cross(forward)

	fpix := float64(w) / 2 / math.Tan(cam.FovX*math.Pi/360)

	model := bsp.Models[0]
	for fi := model.FirstFace; fi < model.FirstFace+model.NumFaces; fi++ {
		face := bsp.Faces[fi]
		plane := bsp.Planes[face.PlaneID]
		n := Vec3{float64(plane.Normal[0]), float64(plane.Normal[1]), float64(plane.Normal[2])}
		d := float64(plane.Dist)
		if face.Side != 0 {
			n = Vec3{-n.X, -n.Y, -n.Z}
			d = -d
		}
		// Backface culling: camera must be on the front side.
		if cam.Pos.Dot(n)-d <= 0.01 {
			continue
		}

		ti := bsp.Texinfo[face.TexinfoID]
		tex := bsp.Textures[ti.MiptexID]
		if tex.Pixels == nil {
			continue
		}

		// Gather polygon vertices with texture coords, transform to view space.
		poly := make([]rvert, 0, face.NumEdges)
		for e := face.FirstEdge; e < face.FirstEdge+int32(face.NumEdges); e++ {
			se := bsp.Surfedges[e]
			var v BSPVertex
			if se >= 0 {
				v = bsp.Vertices[bsp.Edges[se].V0]
			} else {
				v = bsp.Vertices[bsp.Edges[-se].V1]
			}
			wp := Vec3{float64(v.X), float64(v.Y), float64(v.Z)}
			s := wp.Dot(Vec3{float64(ti.VecS[0]), float64(ti.VecS[1]), float64(ti.VecS[2])}) + float64(ti.DistS)
			t := wp.Dot(Vec3{float64(ti.VecT[0]), float64(ti.VecT[1]), float64(ti.VecT[2])}) + float64(ti.DistT)
			rel := wp.Sub(cam.Pos)
			poly = append(poly, rvert{Vec3{rel.Dot(right), rel.Dot(up), rel.Dot(forward)}, s, t})
		}

		poly = clipNear(poly, 1.0)
		if len(poly) < 3 {
			continue
		}

		// Lightmap extents (engine convention: 16-unit luxels).
		var lm lightmapRef
		if ti.Flags&texSpecial == 0 && face.LightOfs >= 0 {
			lm = r.lightmapFor(face, fi)
		}

		liquid := strings.HasPrefix(tex.Name, "*")

		// Triangulate as a fan and rasterize.
		for i := 1; i+1 < len(poly); i++ {
			r.rasterize(poly[0], poly[i], poly[i+1], tex, lm, liquid, fpix)
		}
	}
	return r.img
}

type lightmapRef struct {
	data           []byte
	sw, sh         int // luxel dimensions
	smin, tmin     float64
	valid          bool
}

func (r *renderer) lightmapFor(face BSPFace, fi int32) lightmapRef {
	ti := r.bsp.Texinfo[face.TexinfoID]
	smin, tmin := math.Inf(1), math.Inf(1)
	smax, tmax := math.Inf(-1), math.Inf(-1)
	for e := face.FirstEdge; e < face.FirstEdge+int32(face.NumEdges); e++ {
		se := r.bsp.Surfedges[e]
		var v BSPVertex
		if se >= 0 {
			v = r.bsp.Vertices[r.bsp.Edges[se].V0]
		} else {
			v = r.bsp.Vertices[r.bsp.Edges[-se].V1]
		}
		wp := Vec3{float64(v.X), float64(v.Y), float64(v.Z)}
		s := wp.Dot(Vec3{float64(ti.VecS[0]), float64(ti.VecS[1]), float64(ti.VecS[2])}) + float64(ti.DistS)
		t := wp.Dot(Vec3{float64(ti.VecT[0]), float64(ti.VecT[1]), float64(ti.VecT[2])}) + float64(ti.DistT)
		smin, smax = math.Min(smin, s), math.Max(smax, s)
		tmin, tmax = math.Min(tmin, t), math.Max(tmax, t)
	}
	bsmin := math.Floor(smin / 16)
	btmin := math.Floor(tmin / 16)
	sw := int(math.Ceil(smax/16)-bsmin) + 1
	sh := int(math.Ceil(tmax/16)-btmin) + 1
	ofs := int(face.LightOfs)
	if ofs+sw*sh > len(r.bsp.Lighting) || sw <= 0 || sh <= 0 {
		return lightmapRef{}
	}
	return lightmapRef{r.bsp.Lighting[ofs : ofs+sw*sh], sw, sh, bsmin * 16, btmin * 16, true}
}

func (lm lightmapRef) sample(s, t float64) float64 {
	if !lm.valid {
		return 200 // no lightmap: render at a fixed reasonable level
	}
	ls := (s - lm.smin) / 16
	lt := (t - lm.tmin) / 16
	ls = math.Max(0, math.Min(float64(lm.sw-1), ls))
	lt = math.Max(0, math.Min(float64(lm.sh-1), lt))
	x0, y0 := int(ls), int(lt)
	x1, y1 := min(x0+1, lm.sw-1), min(y0+1, lm.sh-1)
	fx, fy := ls-float64(x0), lt-float64(y0)
	a := float64(lm.data[y0*lm.sw+x0])
	b := float64(lm.data[y0*lm.sw+x1])
	c := float64(lm.data[y1*lm.sw+x0])
	d := float64(lm.data[y1*lm.sw+x1])
	return (a*(1-fx)+b*fx)*(1-fy) + (c*(1-fx)+d*fx)*fy
}

// clipNear clips a view-space polygon against the z=near plane.
func clipNear(poly []rvert, near float64) []rvert {
	out := make([]rvert, 0, len(poly)+2)
	for i := range poly {
		cur, next := poly[i], poly[(i+1)%len(poly)]
		curIn, nextIn := cur.view.Z >= near, next.view.Z >= near
		if curIn {
			out = append(out, cur)
		}
		if curIn != nextIn {
			f := (near - cur.view.Z) / (next.view.Z - cur.view.Z)
			out = append(out, rvert{
				Vec3{
					cur.view.X + f*(next.view.X-cur.view.X),
					cur.view.Y + f*(next.view.Y-cur.view.Y),
					near,
				},
				cur.s + f*(next.s-cur.s),
				cur.t + f*(next.t-cur.t),
			})
		}
	}
	return out
}

func (r *renderer) rasterize(v0, v1, v2 rvert, tex RenderTex, lm lightmapRef, liquid bool, fpix float64) {
	type pv struct {
		x, y, invz, soz, toz float64
	}
	project := func(v rvert) pv {
		invz := 1 / v.view.Z
		return pv{
			float64(r.w)/2 + v.view.X*fpix*invz,
			float64(r.h)/2 - v.view.Y*fpix*invz,
			invz, v.s * invz, v.t * invz,
		}
	}
	p0, p1, p2 := project(v0), project(v1), project(v2)

	area := (p1.x-p0.x)*(p2.y-p0.y) - (p1.y-p0.y)*(p2.x-p0.x)
	if math.Abs(area) < 1e-9 {
		return
	}

	xmin := int(math.Max(0, math.Floor(math.Min(p0.x, math.Min(p1.x, p2.x)))))
	xmax := int(math.Min(float64(r.w-1), math.Ceil(math.Max(p0.x, math.Max(p1.x, p2.x)))))
	ymin := int(math.Max(0, math.Floor(math.Min(p0.y, math.Min(p1.y, p2.y)))))
	ymax := int(math.Min(float64(r.h-1), math.Ceil(math.Max(p0.y, math.Max(p1.y, p2.y)))))

	for y := ymin; y <= ymax; y++ {
		for x := xmin; x <= xmax; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			w0 := ((p1.x-px)*(p2.y-py) - (p1.y-py)*(p2.x-px)) / area
			w1 := ((p2.x-px)*(p0.y-py) - (p2.y-py)*(p0.x-px)) / area
			w2 := 1 - w0 - w1
			if w0 < 0 || w1 < 0 || w2 < 0 {
				continue
			}
			invz := w0*p0.invz + w1*p1.invz + w2*p2.invz
			z := 1 / invz
			idx := y*r.w + x
			if z >= r.zbuf[idx] {
				continue
			}
			s := (w0*p0.soz + w1*p1.soz + w2*p2.soz) * z
			t := (w0*p0.toz + w1*p1.toz + w2*p2.toz) * z

			tx := ((int(math.Floor(s)) % tex.Width) + tex.Width) % tex.Width
			ty := ((int(math.Floor(t)) % tex.Height) + tex.Height) % tex.Height
			c := r.pal[tex.Pixels[ty*tex.Width+tx]]

			var light float64
			if liquid {
				light = 160 // liquids are unlit; render at fixed brightness
			} else {
				light = lm.sample(s, t)
			}
			scale := light / 128

			r.zbuf[idx] = z
			r.img.SetRGBA(x, y, color.RGBA{
				r.shade(c[0], scale), r.shade(c[1], scale), r.shade(c[2], scale), 255,
			})
		}
	}
}

func (r *renderer) shade(c uint8, scale float64) uint8 {
	v := float64(c) / 255 * scale
	v = math.Pow(math.Min(v, 1), 1/r.gamma)
	return uint8(v * 255)
}

// RenderCLI renders screenshots of a BSP. camSpec is a semicolon-separated
// list of "x y z yaw pitch" cameras; empty means auto (overview + spawns).
func RenderCLI(bspPath, camSpec string, width, height int, gamma float64) error {
	bsp, err := LoadRenderBSP(bspPath)
	if err != nil {
		return err
	}
	pal, err := LoadQuakePalette()
	if err != nil {
		return err
	}

	var cams []Camera
	var names []string
	if camSpec != "" {
		for i, spec := range strings.Split(camSpec, ";") {
			f := strings.Fields(spec)
			if len(f) != 5 {
				return fmt.Errorf("camera %q: want 'x y z yaw pitch'", spec)
			}
			var v [5]float64
			for j, s := range f {
				v[j], err = strconv.ParseFloat(s, 64)
				if err != nil {
					return fmt.Errorf("camera %q: %v", spec, err)
				}
			}
			cams = append(cams, Camera{Vec3{v[0], v[1], v[2]}, v[3], v[4], 90})
			names = append(names, fmt.Sprintf("cam%d", i))
		}
	} else {
		cams, names = autoCameras(bsp, width, height)
	}

	base := strings.TrimSuffix(bspPath, ".bsp")
	for i, cam := range cams {
		img := RenderShot(bsp, pal, cam, width, height, gamma)
		out := fmt.Sprintf("%s_%s.png", base, names[i])
		f, err := os.Create(out)
		if err != nil {
			return err
		}
		if err := png.Encode(f, img); err != nil {
			f.Close()
			return err
		}
		f.Close()
		fmt.Printf("rendered %s  (%g %g %g yaw=%g pitch=%g)\n",
			out, cam.Pos.X, cam.Pos.Y, cam.Pos.Z, cam.Yaw, cam.Pitch)
	}
	return nil
}

// autoCameras: top-down overview plus a view from each player spawn.
func autoCameras(bsp *RenderBSP, w, h int) ([]Camera, []string) {
	var cams []Camera
	var names []string

	m := bsp.Models[0]
	cx := float64(m.BBoxMin[0]+m.BBoxMax[0]) / 2
	cy := float64(m.BBoxMin[1]+m.BBoxMax[1]) / 2
	ex := float64(m.BBoxMax[0] - m.BBoxMin[0])
	ey := float64(m.BBoxMax[1] - m.BBoxMin[1])
	tanX := math.Tan(45 * math.Pi / 180)
	tanY := tanX * float64(h) / float64(w)
	depth := math.Max(ex/(2*tanX), ey/(2*tanY)) * 1.05
	cams = append(cams, Camera{Vec3{cx, cy, float64(m.BBoxMax[2]) + depth}, 90, -90, 90})
	names = append(names, "overview")

	n := 0
	for _, ent := range parseEntities(bsp.Entities) {
		cls := ent["classname"]
		if cls != "info_player_start" && cls != "info_player_deathmatch" {
			continue
		}
		f := strings.Fields(ent["origin"])
		if len(f) != 3 {
			continue
		}
		var o [3]float64
		for i, s := range f {
			o[i], _ = strconv.ParseFloat(s, 64)
		}
		yaw := 0.0
		if a, err := strconv.ParseFloat(ent["angle"], 64); err == nil {
			yaw = a
		}
		cams = append(cams, Camera{Vec3{o[0], o[1], o[2] + 26}, yaw, 0, 90})
		names = append(names, fmt.Sprintf("spawn%d", n))
		n++
	}
	return cams, names
}
