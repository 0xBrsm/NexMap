package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// BrushRole classifies a brush's architectural function.
type BrushRole int

const (
	RoleUnknown BrushRole = iota
	RoleFloor
	RoleCeiling
	RoleWall
	RoleStair
	RolePillar
	RoleRamp
	RoleTrim // thin detail brushes
)

func (r BrushRole) String() string {
	switch r {
	case RoleFloor:
		return "floor"
	case RoleCeiling:
		return "ceiling"
	case RoleWall:
		return "wall"
	case RoleStair:
		return "stair"
	case RolePillar:
		return "pillar"
	case RoleRamp:
		return "ramp"
	case RoleTrim:
		return "trim"
	default:
		return "unknown"
	}
}

// BrushStyle is the visual theme derived from the source map's WAD.
type BrushStyle int

const (
	StyleUnknown BrushStyle = iota
	StyleBase               // military tech (e1m1, dm3)
	StyleMetal              // dark industrial (dm4, e1m6)
	StyleMedieval           // brown stone/wood (dm1, dm5)
	StyleWizard             // elder/gothic (e1m2, e2m2)
	StyleTim                // Tim Willits (dm6, e2m7)
)

func (s BrushStyle) String() string {
	switch s {
	case StyleBase:
		return "base"
	case StyleMetal:
		return "metal"
	case StyleMedieval:
		return "medieval"
	case StyleWizard:
		return "wizard"
	case StyleTim:
		return "tim"
	default:
		return "unknown"
	}
}

// wadToStyle maps WAD filenames to styles.
var wadToStyle = map[string]BrushStyle{
	"gfx/base.wad":     StyleBase,
	"gfx/metal.wad":    StyleMetal,
	"gfx/medieval.wad": StyleMedieval,
	"gfx/wizard.wad":   StyleWizard,
	"gfx/tim.wad":      StyleTim,
	"gfx/jr_med.wad":   StyleMedieval,
	"gfx/start.wad":    StyleBase,
}

// BrushEntry is a single classified brush in the library.
type BrushEntry struct {
	Planes   []ParsedPlane
	Bounds   BBox
	Width    float64 // X extent
	Height   float64 // Y extent
	Depth    float64 // Z extent
	Role     BrushRole
	Style    BrushStyle
	Source   string   // map name (e.g. "dm4")
	Textures []string // unique visible textures on this brush
}

// BrushLibrary is a queryable collection of classified brushes.
type BrushLibrary struct {
	Entries []BrushEntry
	// Indices for fast queries.
	ByRole  map[BrushRole][]int
	ByStyle map[BrushStyle][]int
}

// BuildBrushLibrary parses all .map files in a directory and builds
// a classified brush library.
func BuildBrushLibrary(mapDir string) (*BrushLibrary, error) {
	files, err := filepath.Glob(filepath.Join(mapDir, "*.map"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no .map files in %s", mapDir)
	}

	lib := &BrushLibrary{
		ByRole:  map[BrushRole][]int{},
		ByStyle: map[BrushStyle][]int{},
	}

	for _, f := range files {
		mapName := strings.TrimSuffix(filepath.Base(f), ".map")
		entities, err := ParseMapFile(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s: parse error: %v\n", mapName, err)
			continue
		}

		// Determine style from worldspawn WAD key.
		style := StyleUnknown
		var worldspawn *ParsedEntity
		for i := range entities {
			if entities[i].Properties["classname"] == "worldspawn" {
				worldspawn = &entities[i]
				wad := entities[i].Properties["wad"]
				if s, ok := wadToStyle[wad]; ok {
					style = s
				}
				break
			}
		}
		if worldspawn == nil {
			continue
		}

		count := 0
		for i := range worldspawn.Brushes {
			b := &worldspawn.Brushes[i]
			bb := BrushBounds(b)

			w, h, d := bb.Width(), bb.Height(), bb.MaxZ-bb.MinZ
			// Skip degenerate brushes.
			if w < 1 || h < 1 || d < 1 {
				continue
			}

			// Collect visible textures.
			texSet := map[string]bool{}
			for _, p := range b.Planes {
				if !isSpecialTexture(p.Texture) {
					texSet[p.Texture] = true
				}
			}
			// Skip brushes with only special textures (triggers, clips).
			if len(texSet) == 0 {
				continue
			}
			textures := make([]string, 0, len(texSet))
			for t := range texSet {
				textures = append(textures, t)
			}

			role := classifyBrush(b, bb)

			idx := len(lib.Entries)
			lib.Entries = append(lib.Entries, BrushEntry{
				Planes:   b.Planes,
				Bounds:   bb,
				Width:    w,
				Height:   h,
				Depth:    d,
				Role:     role,
				Style:    style,
				Source:   mapName,
				Textures: textures,
			})
			lib.ByRole[role] = append(lib.ByRole[role], idx)
			lib.ByStyle[style] = append(lib.ByStyle[style], idx)
			count++
		}
		fmt.Printf("  %s: %d brushes (%s)\n", mapName, count, style)
	}

	fmt.Printf("brush library: %d total", len(lib.Entries))
	for role, indices := range lib.ByRole {
		fmt.Printf("  %s=%d", role, len(indices))
	}
	fmt.Println()

	return lib, nil
}

// classifyBrush determines a brush's architectural role from its shape.
func classifyBrush(b *ParsedBrush, bb BBox) BrushRole {
	w, h, d := bb.Width(), bb.Height(), bb.MaxZ-bb.MinZ

	// Analyze plane normals to understand orientation.
	var verticalFaces, horizontalFaces, angledFaces int
	for _, p := range b.Planes {
		n := planeNormalf(p.P1, p.P2, p.P3)
		mag := math.Sqrt(n[0]*n[0] + n[1]*n[1] + n[2]*n[2])
		if mag < 1e-10 {
			continue
		}
		nz := math.Abs(n[2] / mag)
		if nz > 0.9 {
			horizontalFaces++
		} else if nz < 0.1 {
			verticalFaces++
		} else {
			angledFaces++
		}
	}

	// Thin horizontal slab = floor or ceiling.
	if d <= 32 && w > d*2 && h > d*2 {
		// Can't distinguish floor from ceiling without context.
		// Default to floor; ceiling is just a floor placed high.
		return RoleFloor
	}

	// Thin vertical slab = wall.
	if (w <= 32 && h > w*2 && d > w*2) ||
		(h <= 32 && w > h*2 && d > h*2) {
		return RoleWall
	}

	// Tall and narrow on both axes = pillar.
	if d > w*2 && d > h*2 && w <= 128 && h <= 128 {
		return RolePillar
	}

	// Has angled faces and step-like proportions = stair/ramp.
	if angledFaces > 0 && d > 16 && d < 256 {
		return RoleRamp
	}

	// Stair-like: moderate Z, small horizontal extent.
	if d >= 8 && d <= 32 && (w <= 64 || h <= 64) {
		return RoleStair
	}

	// Very thin in any dimension = trim/detail.
	if w <= 8 || h <= 8 || d <= 8 {
		return RoleTrim
	}

	// Thick horizontal slab (thicker than 32 but still flat-ish).
	if d < w/2 && d < h/2 {
		return RoleFloor
	}

	// Default: wall for tall things, floor for flat things.
	if d > w || d > h {
		return RoleWall
	}
	return RoleFloor
}

// Query returns brush entries matching the given criteria.
// Any zero-value filter is ignored.
func (lib *BrushLibrary) Query(role BrushRole, style BrushStyle, minW, maxW, minH, maxH float64) []int {
	// Start with role index if specified, otherwise all entries.
	var candidates []int
	if role != RoleUnknown {
		candidates = lib.ByRole[role]
	} else {
		candidates = make([]int, len(lib.Entries))
		for i := range candidates {
			candidates[i] = i
		}
	}

	var results []int
	for _, idx := range candidates {
		e := &lib.Entries[idx]

		if style != StyleUnknown && e.Style != style {
			continue
		}

		// Width filter (the primary dimension along the wall/floor).
		// For walls, "width" is the long horizontal extent.
		// Allow scaling 0.5x-2x, so the effective range is wider.
		brushW := max(e.Width, e.Height) // use longer XY dimension
		if minW > 0 && brushW*2 < minW {
			continue // even at 2x scale, too small
		}
		if maxW > 0 && brushW*0.5 > maxW {
			continue // even at 0.5x scale, too big
		}

		// Height filter (Z extent for walls, thickness for floors).
		if minH > 0 && e.Depth*2 < minH {
			continue
		}
		if maxH > 0 && e.Depth*0.5 > maxH {
			continue
		}

		results = append(results, idx)
	}
	return results
}

// ScaleBrush uniformly scales a brush's planes by the given factor,
// centered on the brush's bounding box center.
func ScaleBrush(planes []ParsedPlane, bb BBox, scale float64) []ParsedPlane {
	cx, cy, cz := bb.CenterX(), bb.CenterY(), (bb.MinZ+bb.MaxZ)/2

	out := make([]ParsedPlane, len(planes))
	for i, p := range planes {
		out[i] = ParsedPlane{
			P1: [3]float64{
				cx + (p.P1[0]-cx)*scale,
				cy + (p.P1[1]-cy)*scale,
				cz + (p.P1[2]-cz)*scale,
			},
			P2: [3]float64{
				cx + (p.P2[0]-cx)*scale,
				cy + (p.P2[1]-cy)*scale,
				cz + (p.P2[2]-cz)*scale,
			},
			P3: [3]float64{
				cx + (p.P3[0]-cx)*scale,
				cy + (p.P3[1]-cy)*scale,
				cz + (p.P3[2]-cz)*scale,
			},
			Texture:  p.Texture,
			XOff:     p.XOff,
			YOff:     p.YOff,
			Rotation: p.Rotation,
			XScale:   p.XScale * scale,
			YScale:   p.YScale * scale,
		}
	}
	return out
}
