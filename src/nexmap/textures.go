package main

import (
	"archive/zip"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/0xBrsm/NexQuake/nexus/quake106"
)

const DefaultTextureWAD = "mapgen_textures.wad"

// TextureSet holds the texture names used by the map generator.
var Textures = struct {
	Floor, Ceiling, Shell, Fill string
	Lava, Water, Slime          string
}{
	Floor:   "tech01_1",
	Ceiling: "tech07_2",
	Shell:   "tech04_1",
	Fill:    "metal1_1",
	Lava:    "*lava1",
	Water:   "*04water1",
	Slime:   "*slime",
}

// quakeTexCache is the lazily-loaded cache of real Quake miptex blobs (name → raw miptex).
var quakeTexCache map[string][]byte

const (
	quakeZipURL  = "https://raw.githubusercontent.com/0xBrsm/QuakeAssets/main/q1/quake106.zip"
	cacheSubdir  = "nexmap"
	pak0Name     = "pak0.pak"
)

func cacheDir() string {
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		return filepath.Join(d, cacheSubdir)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", cacheSubdir)
}

// EnsureQuakeTextures downloads quake106.zip (if needed), extracts pak0.pak,
// and loads all miptex entries from PAK contents into quakeTexCache.
func EnsureQuakeTextures() (map[string][]byte, error) {
	if quakeTexCache != nil {
		return quakeTexCache, nil
	}

	dir := cacheDir()
	pak0Path := filepath.Join(dir, pak0Name)

	// Check if pak0.pak is already cached.
	if _, err := os.Stat(pak0Path); err != nil {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create cache dir: %w", err)
		}

		// Download quake106.zip.
		zipPath := filepath.Join(dir, "quake106.zip")
		if _, err := os.Stat(zipPath); err != nil {
			fmt.Println("downloading quake106.zip ...")
			if err := downloadFile(zipPath, quakeZipURL); err != nil {
				return nil, fmt.Errorf("download quake106.zip: %w", err)
			}
		}

		// Extract pak0.pak using the quake106 package.
		fmt.Println("extracting pak0.pak ...")
		zr, err := zip.OpenReader(zipPath)
		if err != nil {
			return nil, fmt.Errorf("open quake106.zip: %w", err)
		}
		defer zr.Close()

		if err := quake106.ExtractPak0(&zr.Reader, dir); err != nil {
			return nil, fmt.Errorf("extract pak0: %w", err)
		}
	}

	// Read pak0.pak and extract all textures.
	pakData, err := os.ReadFile(pak0Path)
	if err != nil {
		return nil, fmt.Errorf("read pak0.pak: %w", err)
	}

	texMap, err := extractTexturesFromPAK(pakData)
	if err != nil {
		return nil, fmt.Errorf("parse pak0.pak: %w", err)
	}

	quakeTexCache = texMap
	fmt.Printf("loaded %d Quake textures from pak0.pak\n", len(texMap))
	return quakeTexCache, nil
}

func downloadFile(dest, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// --- PAK format reader ---
// PAK header: 4-byte magic "PACK", 4-byte dir offset, 4-byte dir size.
// Each dir entry: 56-byte name + 4-byte offset + 4-byte size.

type pakEntry struct {
	name   string
	offset int
	size   int
}

func parsePAK(data []byte) ([]pakEntry, error) {
	if len(data) < 12 || string(data[:4]) != "PACK" {
		return nil, fmt.Errorf("not a PAK file")
	}
	dirOff := int(binary.LittleEndian.Uint32(data[4:8]))
	dirSize := int(binary.LittleEndian.Uint32(data[8:12]))
	numEntries := dirSize / 64

	var entries []pakEntry
	for i := range numEntries {
		off := dirOff + i*64
		if off+64 > len(data) {
			break
		}
		name := cString(data[off : off+56])
		foff := int(binary.LittleEndian.Uint32(data[off+56 : off+60]))
		fsz := int(binary.LittleEndian.Uint32(data[off+60 : off+64]))
		entries = append(entries, pakEntry{name: name, offset: foff, size: fsz})
	}
	return entries, nil
}

func cString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// extractTexturesFromPAK reads all miptex entries from BSP files and WAD2 files in the PAK.
func extractTexturesFromPAK(pakData []byte) (map[string][]byte, error) {
	entries, err := parsePAK(pakData)
	if err != nil {
		return nil, err
	}

	texMap := make(map[string][]byte)

	for _, ent := range entries {
		if ent.offset+ent.size > len(pakData) {
			continue
		}
		data := pakData[ent.offset : ent.offset+ent.size]
		lower := strings.ToLower(ent.name)

		if strings.HasSuffix(lower, ".bsp") {
			extractMiptexFromBSP(data, texMap)
		} else if strings.HasSuffix(lower, ".wad") {
			extractMiptexFromWAD2(data, texMap)
		}
	}

	return texMap, nil
}

// --- BSP miptex lump reader ---
// BSP29 header: version(4) + 15 lumps * (offset(4) + size(4)).
// Lump 2 is the miptex lump.
// Miptex lump: numtex(4) + offsets(4*numtex) + miptex entries.
// Each miptex: name(16) + width(4) + height(4) + offsets[4](16) + pixel data.

func extractMiptexFromBSP(bspData []byte, out map[string][]byte) {
	if len(bspData) < 4+15*8 {
		return
	}
	ver := binary.LittleEndian.Uint32(bspData[:4])
	if ver != 29 {
		return
	}

	// Lump 2 (miptex): offset at 4 + 2*8 = 20, size at 24.
	mipOff := int(binary.LittleEndian.Uint32(bspData[20:24]))
	mipSize := int(binary.LittleEndian.Uint32(bspData[24:28]))
	if mipOff+mipSize > len(bspData) || mipSize < 4 {
		return
	}

	mipLump := bspData[mipOff : mipOff+mipSize]
	parseMiptexLump(mipLump, out)
}

// parseMiptexLump reads individual miptex entries from a miptex lump.
func parseMiptexLump(lump []byte, out map[string][]byte) {
	if len(lump) < 4 {
		return
	}
	numTex := int(binary.LittleEndian.Uint32(lump[:4]))
	if 4+numTex*4 > len(lump) {
		return
	}

	for i := range numTex {
		off := int(binary.LittleEndian.Uint32(lump[4+i*4 : 4+i*4+4]))
		if off < 0 || off+40 > len(lump) {
			continue // -1 means unused slot
		}

		name := strings.ToLower(cString(lump[off : off+16]))
		if name == "" {
			continue
		}

		w := int(binary.LittleEndian.Uint32(lump[off+16 : off+20]))
		h := int(binary.LittleEndian.Uint32(lump[off+20 : off+24]))
		if w <= 0 || h <= 0 || w > 1024 || h > 1024 {
			continue
		}

		// Total size: 40-byte header + mip0 + mip1 + mip2 + mip3.
		totalPixels := miptexPixelCount(w, h)
		entrySize := 40 + totalPixels
		if off+entrySize > len(lump) {
			continue
		}

		// Only store the first occurrence (highest-res version).
		if _, exists := out[name]; !exists {
			blob := make([]byte, entrySize)
			copy(blob, lump[off:off+entrySize])
			out[name] = blob
		}
	}
}

// --- WAD2 reader ---
// Header: magic(4) + numEntries(4) + dirOffset(4).
// Dir entry: offset(4) + diskSize(4) + fullSize(4) + type(1) + compression(1) + pad(2) + name(16).

func extractMiptexFromWAD2(wadData []byte, out map[string][]byte) {
	if len(wadData) < 12 {
		return
	}
	magic := string(wadData[:4])
	if magic != "WAD2" && magic != "WAD3" {
		return
	}
	numEntries := int(binary.LittleEndian.Uint32(wadData[4:8]))
	dirOff := int(binary.LittleEndian.Uint32(wadData[8:12]))

	for i := range numEntries {
		doff := dirOff + i*32
		if doff+32 > len(wadData) {
			break
		}

		eoff := int(binary.LittleEndian.Uint32(wadData[doff : doff+4]))
		esize := int(binary.LittleEndian.Uint32(wadData[doff+4 : doff+8]))
		etype := wadData[doff+12]
		name := strings.ToLower(cString(wadData[doff+16 : doff+32]))

		if etype != wadMiptexType {
			continue
		}
		if eoff+esize > len(wadData) || esize < 40 {
			continue
		}

		if _, exists := out[name]; !exists {
			blob := make([]byte, esize)
			copy(blob, wadData[eoff:eoff+esize])
			out[name] = blob
		}
	}
}

// --- Public API for BSP builder and WAD writer ---

// LookupMiptex returns a complete miptex blob for the given texture name.
// Tries real Quake textures first, falls back to a synthetic texture.
func LookupMiptex(name string) []byte {
	lower := strings.ToLower(name)

	// Try real textures (may not be loaded yet).
	if quakeTexCache != nil {
		if blob, ok := quakeTexCache[lower]; ok {
			// Rename to match requested name exactly.
			return renameMiptex(blob, name)
		}
	}

	// Fallback: synthetic.
	return synthMiptex(name)
}

func synthMiptex(name string) []byte {
	w, h := 64, 64
	var px []byte
	lower := strings.ToLower(name)

	switch {
	case strings.HasPrefix(lower, "*lava"):
		px = waves(w, h, [3]byte{224, 232, 240}, 5)
	case strings.HasPrefix(lower, "*water"), strings.HasPrefix(lower, "*04water"):
		px = waves(w, h, [3]byte{144, 152, 160}, 7)
	case strings.HasPrefix(lower, "*slime"):
		px = waves(w, h, [3]byte{184, 192, 200}, 9)
	case strings.HasPrefix(lower, "*tele"):
		px = waves(w, h, [3]byte{200, 208, 216}, 6)
	default:
		px = checker(w, h, 86, 96, 8)
	}
	return encodeMiptex(name, w, h, px)
}

// MaterializeTextureWAD writes a WAD2 file with real Quake textures
// when available, falling back to synthetics.
func MaterializeTextureWAD(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, DefaultTextureWAD)

	// Try to load real textures (non-fatal if it fails).
	if _, err := EnsureQuakeTextures(); err != nil {
		fmt.Fprintf(os.Stderr, "note: real textures unavailable, using synthetic: %v\n", err)
	}

	// Collect all texture names we need.
	needed := []string{
		Textures.Floor, Textures.Ceiling, Textures.Shell, Textures.Fill,
		Textures.Lava, Textures.Water, Textures.Slime,
		"*water0", "*slime0", "*teleport",
	}

	var entries []struct {
		name string
		blob []byte
	}
	seen := map[string]bool{}

	for _, name := range needed {
		if seen[name] {
			continue
		}
		seen[name] = true
		entries = append(entries, struct {
			name string
			blob []byte
		}{name, LookupMiptex(name)})
	}

	return path, writeWAD2(path, entries)
}

// --- Synthetic texture generators ---

func checker(w, h, a, b int, size int) []byte {
	px := make([]byte, w*h)
	for y := range h {
		for x := range w {
			if ((x/size)+(y/size))%2 == 0 {
				px[y*w+x] = byte(a)
			} else {
				px[y*w+x] = byte(b)
			}
		}
	}
	return px
}

func trimBand(w, h, a, b, accent int) []byte {
	px := make([]byte, w*h)
	for y := range h {
		rowColor := a
		if y < h/2 {
			rowColor = a
		} else {
			rowColor = b
		}
		if y <= 1 || y >= h-2 {
			rowColor = accent
		}
		for x := range w {
			c := rowColor
			if (x/8)%2 != 0 {
				c += 2
			}
			px[y*w+x] = byte(c)
		}
	}
	return px
}

func rivets(w, h, base, alt, rivet int) []byte {
	px := make([]byte, w*h)
	for y := range h {
		for x := range w {
			c := base
			if ((x/8)+(y/8))%2 != 0 {
				c = alt
			}
			if x%16 == 2 || x%16 == 13 {
				if y%16 == 2 || y%16 == 13 {
					c = rivet
				}
			}
			px[y*w+x] = byte(c)
		}
	}
	return px
}

func waves(w, h int, colors [3]byte, period int) []byte {
	px := make([]byte, w*h)
	for y := range h {
		for x := range w {
			idx := (x/period + y/period + (x+y)/(period*2)) % 3
			px[y*w+x] = colors[idx]
		}
	}
	return px
}

// --- Miptex encoding ---

func encodeMiptex(name string, w, h int, pixels []byte) []byte {
	mip0 := pixels
	mip1 := downsample(mip0, w, h)
	mip2 := downsample(mip1, w/2, h/2)
	mip3 := downsample(mip2, w/4, h/4)

	headerSize := 40
	mip0Off := headerSize
	mip1Off := mip0Off + len(mip0)
	mip2Off := mip1Off + len(mip1)
	mip3Off := mip2Off + len(mip2)

	header := make([]byte, headerSize)
	copy(header[0:16], padName(name))
	binary.LittleEndian.PutUint32(header[16:], uint32(w))
	binary.LittleEndian.PutUint32(header[20:], uint32(h))
	binary.LittleEndian.PutUint32(header[24:], uint32(mip0Off))
	binary.LittleEndian.PutUint32(header[28:], uint32(mip1Off))
	binary.LittleEndian.PutUint32(header[32:], uint32(mip2Off))
	binary.LittleEndian.PutUint32(header[36:], uint32(mip3Off))

	var out []byte
	out = append(out, header...)
	out = append(out, mip0...)
	out = append(out, mip1...)
	out = append(out, mip2...)
	out = append(out, mip3...)
	return out
}

func renameMiptex(blob []byte, name string) []byte {
	out := make([]byte, len(blob))
	copy(out, blob)
	copy(out[0:16], padName(name))
	return out
}

func downsample(pixels []byte, w, h int) []byte {
	nw := max(1, w/2)
	nh := max(1, h/2)
	out := make([]byte, nw*nh)
	for y := range nh {
		for x := range nw {
			tl := int(pixels[min(2*y, h-1)*w+min(2*x, w-1)])
			tr := int(pixels[min(2*y, h-1)*w+min(2*x+1, w-1)])
			bl := int(pixels[min(2*y+1, h-1)*w+min(2*x, w-1)])
			br := int(pixels[min(2*y+1, h-1)*w+min(2*x+1, w-1)])
			out[y*nw+x] = byte((tl + tr + bl + br) / 4)
		}
	}
	return out
}

func padName(name string) []byte {
	b := make([]byte, 16)
	copy(b, []byte(name))
	return b
}

func miptexPixelCount(w, h int) int {
	total := 0
	lw, lh := w, h
	for range 4 {
		total += lw * lh
		lw = max(1, lw/2)
		lh = max(1, lh/2)
	}
	return total
}

// --- WAD2 writer ---

const wadMiptexType = 68

func writeWAD2(path string, entries []struct {
	name string
	blob []byte
}) error {
	headerSize := 12
	filepos := headerSize

	var payload []byte
	type dirEntry struct {
		offset, diskSize, fullSize int
		entryType                  byte
		name                       string
	}
	var directory []dirEntry

	for _, e := range entries {
		directory = append(directory, dirEntry{
			offset: filepos, diskSize: len(e.blob), fullSize: len(e.blob),
			entryType: wadMiptexType, name: e.name,
		})
		payload = append(payload, e.blob...)
		filepos += len(e.blob)
	}

	infoOffset := headerSize + len(payload)

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Header: "WAD2" + numEntries + infoOffset.
	header := make([]byte, 12)
	copy(header[0:4], []byte("WAD2"))
	binary.LittleEndian.PutUint32(header[4:], uint32(len(directory)))
	binary.LittleEndian.PutUint32(header[8:], uint32(infoOffset))
	if _, err := f.Write(header); err != nil {
		return err
	}
	if _, err := f.Write(payload); err != nil {
		return err
	}

	// Directory entries: 32 bytes each.
	for _, d := range directory {
		entry := make([]byte, 32)
		binary.LittleEndian.PutUint32(entry[0:], uint32(d.offset))
		binary.LittleEndian.PutUint32(entry[4:], uint32(d.diskSize))
		binary.LittleEndian.PutUint32(entry[8:], uint32(d.fullSize))
		entry[12] = d.entryType
		entry[13] = 0 // compression
		binary.LittleEndian.PutUint16(entry[14:], 0)
		copy(entry[16:32], padName(d.name))
		if _, err := f.Write(entry); err != nil {
			return err
		}
	}

	return nil
}

