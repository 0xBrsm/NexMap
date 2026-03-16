package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type pointEnt struct {
	classname string
	props     map[string]string
	x, y, z   float64
}

// BSP2Blueprint converts a BSP29 file into a blueprint JSON.
func BSP2Blueprint(bspData []byte, mapName string) (*rawBlueprint, error) {
	ents, err := parseEntityLump(bspData)
	if err != nil {
		return nil, err
	}

	var points []pointEnt
	var worldProps map[string]string

	for _, e := range ents {
		cn := e["classname"]
		if cn == "worldspawn" {
			worldProps = e
			continue
		}
		orig, ok := e["origin"]
		if !ok {
			continue
		}
		var x, y, z float64
		fmt.Sscanf(orig, "%f %f %f", &x, &y, &z)
		points = append(points, pointEnt{
			classname: cn, props: e,
			x: x, y: y, z: z,
		})
	}

	// Cluster entities into rooms using spatial proximity.
	rooms := clusterEntities(points)

	// Assign room metadata.
	var rawRooms []rawRoom
	for i, room := range rooms {
		rawRooms = append(rawRooms, buildRawRoom(room, i))
	}

	// Infer connections from teleporters and spatial adjacency.
	conns := inferConnections(rooms)

	// Build flow info.
	flow := inferFlow(rooms, conns)

	title := prettifyMapName(mapName)
	message := ""
	if worldProps != nil {
		if m, ok := worldProps["message"]; ok {
			title = m
		}
	}
	_ = message

	bp := &rawBlueprint{
		Version: 1,
		Meta: rawMeta{
			Name:        title,
			Author:      "id Software",
			Description: fmt.Sprintf("Converted from Quake shareware %s", mapName),
			Theme:       "base",
			Gamemode:    inferGamemode(points),
			PlayerCount: &rawPlayerCount{Min: 1, Max: 4},
			DesignNotes: fmt.Sprintf("Auto-extracted from %s entity data. %d rooms, %d entities.", mapName, len(rawRooms), len(points)),
		},
		Rooms:       rawRooms,
		Connections: conns,
		Flow:        flow,
	}

	return bp, nil
}

// --- Entity lump parser ---

func parseEntityLump(bspData []byte) ([]map[string]string, error) {
	if len(bspData) < 4+15*8 {
		return nil, fmt.Errorf("BSP too small")
	}
	ver := binary.LittleEndian.Uint32(bspData[:4])
	if ver != 29 {
		return nil, fmt.Errorf("not BSP29 (version %d)", ver)
	}

	// Lump 0 = entities.
	entOff := int(binary.LittleEndian.Uint32(bspData[4:8]))
	entSize := int(binary.LittleEndian.Uint32(bspData[8:12]))
	if entOff+entSize > len(bspData) {
		return nil, fmt.Errorf("entity lump out of bounds")
	}

	entStr := string(bspData[entOff : entOff+entSize])
	// Trim null terminator.
	entStr = strings.TrimRight(entStr, "\x00")

	return parseEntities(entStr), nil
}

func parseEntities(s string) []map[string]string {
	var result []map[string]string
	var current map[string]string

	lines := strings.Split(s, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "{" {
			current = map[string]string{}
		} else if line == "}" {
			if current != nil {
				result = append(result, current)
				current = nil
			}
		} else if current != nil {
			// Parse "key" "value"
			parts := strings.SplitN(line, "\" \"", 2)
			if len(parts) == 2 {
				key := strings.Trim(parts[0], "\" ")
				val := strings.Trim(parts[1], "\" ")
				current[key] = val
			}
		}
	}
	return result
}

// --- Spatial clustering ---

type entityCluster struct {
	id       string
	entities []pointEnt
	cx, cy, cz       float64 // centroid
	minX, minY, minZ float64
	maxX, maxY, maxZ float64
}

const gridCell = 512.0 // Grid cell size for spatial grouping

func clusterEntities(points []pointEnt) []entityCluster {
	// Grid-based clustering: divide 2D space into cells, then merge
	// adjacent occupied cells into rooms.
	type cellKey struct{ cx, cy int }

	// Filter out path_corners, ambient sounds, info_intermission, coop spawns.
	var usable []pointEnt
	for _, p := range points {
		switch {
		case strings.HasPrefix(p.classname, "path_corner"):
			continue
		case strings.HasPrefix(p.classname, "ambient_"):
			continue
		case p.classname == "info_intermission":
			continue
		case p.classname == "info_player_coop":
			continue
		}
		usable = append(usable, p)
	}

	// Assign each entity to a grid cell.
	cellEnts := map[cellKey][]pointEnt{}
	for _, p := range usable {
		k := cellKey{int(math.Floor(p.x / gridCell)), int(math.Floor(p.y / gridCell))}
		cellEnts[k] = append(cellEnts[k], p)
	}

	// Flood-fill adjacent cells into clusters. Two cells merge if they
	// share an edge (4-connected). Only merge cells on the same Z-band
	// (within 256 units vertically) to keep different floors separate.
	cellCluster := map[cellKey]int{}
	clusterID := 0

	avgZ := func(ents []pointEnt) float64 {
		s := 0.0
		for _, e := range ents {
			s += e.z
		}
		return s / float64(len(ents))
	}

	for k := range cellEnts {
		if _, ok := cellCluster[k]; ok {
			continue
		}
		cellCluster[k] = clusterID
		queue := []cellKey{k}
		baseZ := avgZ(cellEnts[k])
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nb := cellKey{cur.cx + d[0], cur.cy + d[1]}
				if _, exists := cellEnts[nb]; !exists {
					continue
				}
				if _, done := cellCluster[nb]; done {
					continue
				}
				// Only merge if Z is similar.
				nbZ := avgZ(cellEnts[nb])
				if math.Abs(nbZ-baseZ) > 256 {
					continue
				}
				cellCluster[nb] = clusterID
				queue = append(queue, nb)
			}
		}
		clusterID++
	}

	// Build clusters from cell assignments.
	clusterMap := map[int]*entityCluster{}
	for k, cid := range cellCluster {
		if _, ok := clusterMap[cid]; !ok {
			clusterMap[cid] = &entityCluster{
				minX: math.MaxFloat64, minY: math.MaxFloat64, minZ: math.MaxFloat64,
				maxX: -math.MaxFloat64, maxY: -math.MaxFloat64, maxZ: -math.MaxFloat64,
			}
		}
		c := clusterMap[cid]
		for _, p := range cellEnts[k] {
			c.entities = append(c.entities, p)
			c.cx += p.x
			c.cy += p.y
			c.cz += p.z
			if p.x < c.minX { c.minX = p.x }
			if p.y < c.minY { c.minY = p.y }
			if p.z < c.minZ { c.minZ = p.z }
			if p.x > c.maxX { c.maxX = p.x }
			if p.y > c.maxY { c.maxY = p.y }
			if p.z > c.maxZ { c.maxZ = p.z }
		}
	}

	for _, c := range clusterMap {
		n := float64(len(c.entities))
		c.cx /= n
		c.cy /= n
		c.cz /= n
	}

	// Split clusters that are too large (> 1500 units across) by
	// subdividing along the longest axis.
	var final []entityCluster
	for _, c := range clusterMap {
		subs := splitLargeClusters(*c, 1500)
		final = append(final, subs...)
	}

	// Drop clusters with only lights and no gameplay entities.
	var filtered []entityCluster
	for _, c := range final {
		hasGameplay := false
		for _, e := range c.entities {
			if e.classname != "light" && !strings.HasPrefix(e.classname, "light_") {
				hasGameplay = true
				break
			}
		}
		if hasGameplay {
			filtered = append(filtered, c)
		}
	}

	// Sort by centroid Y then X for stable ordering.
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].cy != filtered[j].cy {
			return filtered[i].cy < filtered[j].cy
		}
		return filtered[i].cx < filtered[j].cx
	})

	for i := range filtered {
		filtered[i].id = fmt.Sprintf("room_%d", i)
	}

	return filtered
}

func splitLargeClusters(c entityCluster, maxSpan float64) []entityCluster {
	dx := c.maxX - c.minX
	dy := c.maxY - c.minY

	if dx <= maxSpan && dy <= maxSpan {
		return []entityCluster{c}
	}

	// Split along longest axis at centroid.
	splitHoriz := dy > dx
	var left, right entityCluster
	left.minX = math.MaxFloat64; left.minY = math.MaxFloat64; left.minZ = math.MaxFloat64
	left.maxX = -math.MaxFloat64; left.maxY = -math.MaxFloat64; left.maxZ = -math.MaxFloat64
	right.minX = math.MaxFloat64; right.minY = math.MaxFloat64; right.minZ = math.MaxFloat64
	right.maxX = -math.MaxFloat64; right.maxY = -math.MaxFloat64; right.maxZ = -math.MaxFloat64

	for _, e := range c.entities {
		var target *entityCluster
		if splitHoriz {
			if e.y < c.cy {
				target = &left
			} else {
				target = &right
			}
		} else {
			if e.x < c.cx {
				target = &left
			} else {
				target = &right
			}
		}
		target.entities = append(target.entities, e)
		target.cx += e.x; target.cy += e.y; target.cz += e.z
		if e.x < target.minX { target.minX = e.x }
		if e.y < target.minY { target.minY = e.y }
		if e.z < target.minZ { target.minZ = e.z }
		if e.x > target.maxX { target.maxX = e.x }
		if e.y > target.maxY { target.maxY = e.y }
		if e.z > target.maxZ { target.maxZ = e.z }
	}

	var result []entityCluster
	for _, sub := range []*entityCluster{&left, &right} {
		if len(sub.entities) == 0 {
			continue
		}
		n := float64(len(sub.entities))
		sub.cx /= n; sub.cy /= n; sub.cz /= n
		result = append(result, splitLargeClusters(*sub, maxSpan)...)
	}
	return result
}

// --- Room building ---

// safePointEntities lists classnames that are safe to place as point entities
// in a generated BSP. Excludes monsters (need progs/), brush entities
// (func_*, trigger_*, misc_explobox), and engine internals.
var safePointEntities = map[string]bool{
	// Weapons
	"weapon_shotgun": true, "weapon_supershotgun": true,
	"weapon_nailgun": true, "weapon_supernailgun": true,
	"weapon_grenadelauncher": true, "weapon_rocketlauncher": true,
	"weapon_lightning": true,
	// Ammo
	"item_shells": true, "item_spikes": true,
	"item_rockets": true, "item_cells": true,
	// Armor
	"item_armor1": true, "item_armor2": true, "item_armorInv": true,
	// Health
	"item_health": true,
	// Powerups
	"item_artifact_super_damage": true, "item_artifact_invulnerability": true,
	"item_artifact_invisibility": true, "item_artifact_envirosuit": true,
	// Spawns
	"info_player_start": true, "info_player_deathmatch": true,
	// Light (handled separately but listed for completeness)
	"light": true,
}

func buildRawRoom(c entityCluster, idx int) rawRoom {
	role := inferRole(c)
	size := inferSize(c)
	elev := inferElevation(c)
	ceil := inferCeiling(c)

	var items []rawItem
	var hazard *rawHazard

	classCounts := map[string]int{}
	for _, e := range c.entities {
		cn := e.classname
		if cn == "light" || strings.HasPrefix(cn, "light_") {
			continue // handled separately
		}
		if !safePointEntities[cn] {
			continue // skip unsafe entities
		}
		classCounts[cn]++
	}

	for cn, count := range classCounts {
		placement := inferPlacement(cn)
		items = append(items, rawItem{
			Classname: cn,
			Placement: placement,
			Count:     count,
		})
	}

	// Count lights.
	lightCount := 0
	for _, e := range c.entities {
		if e.classname == "light" || strings.HasPrefix(e.classname, "light_") {
			lightCount++
		}
	}
	if lightCount > 0 {
		items = append(items, rawItem{
			Classname: "light",
			Count:     lightCount,
		})
	}

	// Sort items for deterministic output.
	sort.Slice(items, func(i, j int) bool {
		return items[i].Classname < items[j].Classname
	})

	// Check for hazard textures in trigger_hurt entities.
	for _, e := range c.entities {
		if e.classname == "trigger_hurt" {
			hazard = &rawHazard{Type: "lava", Coverage: "medium"}
			break
		}
	}

	label := inferLabel(c, idx)

	return rawRoom{
		ID:        c.id,
		Role:      role,
		Label:     label,
		Size:      size,
		Elevation: elev,
		Ceiling:   ceil,
		Hazard:    hazard,
		Items:     items,
		Tags:      inferTags(c),
	}
}

func inferRole(c entityCluster) string {
	has := func(prefix string) bool {
		for _, e := range c.entities {
			if strings.HasPrefix(e.classname, prefix) {
				return true
			}
		}
		return false
	}
	exact := func(name string) bool {
		for _, e := range c.entities {
			if e.classname == name {
				return true
			}
		}
		return false
	}

	// Powerups → control point.
	if exact("item_artifact_super_damage") || exact("item_artifact_invulnerability") ||
		exact("item_artifact_invisibility") || exact("item_armorInv") {
		return "control_point"
	}
	// Major weapons → arena.
	if exact("weapon_rocketlauncher") || exact("weapon_lightning") {
		return "arena"
	}
	// Player start.
	if exact("info_player_start") && !has("weapon_") && !has("monster_") {
		return "spawn"
	}
	// Multiple weapons → arena.
	weaponCount := 0
	for _, e := range c.entities {
		if strings.HasPrefix(e.classname, "weapon_") {
			weaponCount++
		}
	}
	if weaponCount >= 2 {
		return "arena"
	}
	// Items only.
	if has("weapon_") || has("item_armor") {
		return "supply"
	}
	// Teleporter.
	if exact("trigger_teleport") || exact("info_teleport_destination") {
		return "transition"
	}

	return "transition"
}

func inferSize(c entityCluster) string {
	dx := c.maxX - c.minX
	dy := c.maxY - c.minY
	spread := math.Max(dx, dy)
	switch {
	case spread > 800:
		return "huge"
	case spread > 500:
		return "large"
	case spread > 250:
		return "medium"
	default:
		return "small"
	}
}

func inferElevation(c entityCluster) string {
	avgZ := c.cz
	switch {
	case avgZ < -64:
		return "low"
	case avgZ > 256:
		return "high"
	case avgZ > 64:
		return "mid"
	default:
		return "ground"
	}
}

func inferCeiling(c entityCluster) string {
	dz := c.maxZ - c.minZ
	switch {
	case dz > 256:
		return "cavernous"
	case dz > 160:
		return "tall"
	case dz < 80:
		return "low"
	default:
		return "normal"
	}
}

func inferPlacement(classname string) string {
	switch {
	case strings.HasPrefix(classname, "info_player"):
		return "corner"
	case strings.HasPrefix(classname, "weapon_"):
		return "center"
	case classname == "item_armorInv" || classname == "item_artifact_super_damage":
		return "center"
	case strings.HasPrefix(classname, "item_armor"):
		return "wall"
	case strings.HasPrefix(classname, "item_health"):
		return "wall"
	case strings.HasPrefix(classname, "monster_"):
		return "any"
	default:
		return "any"
	}
}

func inferLabel(c entityCluster, idx int) string {
	// Try to derive a label from nearby entities or position.
	has := func(prefix string) bool {
		for _, e := range c.entities {
			if strings.HasPrefix(e.classname, prefix) {
				return true
			}
		}
		return false
	}
	exact := func(name string) bool {
		for _, e := range c.entities {
			if e.classname == name {
				return true
			}
		}
		return false
	}

	switch {
	case exact("info_player_start"):
		return "Start Area"
	case exact("item_artifact_super_damage"):
		return "Quad Room"
	case exact("item_artifact_invulnerability"):
		return "Pentagram Room"
	case exact("weapon_rocketlauncher"):
		return "Rocket Room"
	case exact("weapon_lightning"):
		return "Lightning Room"
	case has("trigger_teleport"):
		return "Teleporter"
	case exact("trigger_changelevel"):
		return "Exit"
	}

	// Position-based.
	if c.cz > 200 {
		return fmt.Sprintf("Upper Chamber %d", idx)
	}
	if c.cz < -64 {
		return fmt.Sprintf("Lower Chamber %d", idx)
	}
	return fmt.Sprintf("Room %d", idx)
}

func inferTags(c entityCluster) []string {
	var tags []string
	has := func(prefix string) bool {
		for _, e := range c.entities {
			if strings.HasPrefix(e.classname, prefix) {
				return true
			}
		}
		return false
	}

	if has("weapon_") {
		tags = append(tags, "loot")
	}
	dz := c.maxZ - c.minZ
	if dz > 200 {
		tags = append(tags, "vertical")
	}
	if has("info_player_start") || has("info_player_deathmatch") {
		tags = append(tags, "spawn")
	}
	if has("trigger_teleport") {
		tags = append(tags, "teleporter")
	}
	if has("trigger_changelevel") {
		tags = append(tags, "exit")
	}
	if has("func_door") || has("trigger_once") {
		tags = append(tags, "interactive")
	}
	if has("item_artifact") {
		tags = append(tags, "high_value")
	}

	return tags
}

// --- Connection inference ---

func inferConnections(rooms []entityCluster) []rawConnection {
	var conns []rawConnection
	seen := map[[2]int]bool{}

	// Teleporter connections: match trigger_teleport targets to info_teleport_destination targetnames.
	teleTargets := map[string]int{} // targetname → room index
	for i, room := range rooms {
		for _, e := range room.entities {
			if e.classname == "info_teleport_destination" {
				if tn, ok := e.props["targetname"]; ok {
					teleTargets[tn] = i
				}
			}
		}
	}
	for i, room := range rooms {
		for _, e := range room.entities {
			if e.classname == "trigger_teleport" {
				target := e.props["target"]
				if j, ok := teleTargets[target]; ok && i != j {
					key := [2]int{i, j}
					if !seen[key] {
						seen[key] = true
						bidir := false
						conns = append(conns, rawConnection{
							From:          rooms[i].id,
							To:            rooms[j].id,
							Type:          "teleporter",
							Bidirectional: &bidir,
							Width:         "normal",
							Intent:        "Teleporter link",
						})
					}
				}
			}
		}
	}

	// Spatial adjacency: rooms within corridor distance.
	for i := 0; i < len(rooms); i++ {
		for j := i + 1; j < len(rooms); j++ {
			key := [2]int{i, j}
			if seen[key] {
				continue
			}

			dx := rooms[i].cx - rooms[j].cx
			dy := rooms[i].cy - rooms[j].cy
			dz := rooms[i].cz - rooms[j].cz
			dist2D := math.Sqrt(dx*dx + dy*dy)

			if dist2D > 1200 {
				continue
			}

			connType := "corridor"
			width := "normal"
			dzAbs := math.Abs(dz)

			if dzAbs > 128 && dist2D < 600 {
				connType = "stairs"
			} else if dzAbs > 200 {
				connType = "drop"
			}

			if dist2D > 800 {
				width = "narrow"
			} else if dist2D < 400 {
				width = "wide"
			}

			seen[key] = true
			conns = append(conns, rawConnection{
				From:  rooms[i].id,
				To:    rooms[j].id,
				Type:  connType,
				Width: width,
			})
		}
	}

	return conns
}

func inferFlow(rooms []entityCluster, conns []rawConnection) *rawFlow {
	if len(rooms) < 3 {
		return nil
	}

	// Primary loop: just list room IDs in order.
	var loop []string
	for _, r := range rooms {
		loop = append(loop, r.id)
		if len(loop) >= 6 {
			break
		}
	}
	if len(loop) > 0 {
		loop = append(loop, loop[0])
	}

	// Choke points: small rooms with many connections.
	connCount := map[string]int{}
	for _, c := range conns {
		connCount[c.From]++
		connCount[c.To]++
	}
	var chokes []string
	for _, r := range rooms {
		if connCount[r.id] >= 3 && inferSize(r) == "small" {
			chokes = append(chokes, r.id)
		}
	}

	return &rawFlow{
		PrimaryLoop: loop,
		ChokePoints: chokes,
	}
}

func inferGamemode(points []pointEnt) string {
	dmSpawns := 0
	monsters := 0
	for _, p := range points {
		if p.classname == "info_player_deathmatch" {
			dmSpawns++
		}
		if strings.HasPrefix(p.classname, "monster_") {
			monsters++
		}
	}
	if dmSpawns > 2 {
		return "deathmatch"
	}
	if monsters > 0 {
		return "singleplayer"
	}
	return "singleplayer"
}

func prettifyMapName(name string) string {
	name = strings.TrimSuffix(name, ".bsp")
	name = strings.TrimPrefix(name, "maps/")
	return strings.ToUpper(name)
}

// --- Extract all blueprints from pak0.pak ---

func ExtractSharewareBlueprints(outDir string) error {
	pakPath := filepath.Join(cacheDir(), pak0Name)
	pakData, err := os.ReadFile(pakPath)
	if err != nil {
		return fmt.Errorf("read pak0.pak: %w (run with -bsp first to download)", err)
	}

	entries, err := parsePAK(pakData)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	count := 0
	for _, ent := range entries {
		lower := strings.ToLower(ent.name)
		if !strings.HasPrefix(lower, "maps/") || !strings.HasSuffix(lower, ".bsp") {
			continue
		}
		// Skip item model BSPs (b_*.bsp).
		baseName := filepath.Base(lower)
		if strings.HasPrefix(baseName, "b_") {
			continue
		}
		if ent.offset+ent.size > len(pakData) {
			continue
		}

		bspData := pakData[ent.offset : ent.offset+ent.size]
		mapName := strings.TrimPrefix(lower, "maps/")
		mapBase := strings.TrimSuffix(mapName, ".bsp")

		bp, err := BSP2Blueprint(bspData, mapName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", mapName, err)
			continue
		}

		jsonData, err := json.MarshalIndent(bp, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: marshal: %v\n", mapName, err)
			continue
		}

		outPath := filepath.Join(outDir, mapBase+".json")
		if err := os.WriteFile(outPath, jsonData, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", outPath, err)
		}
		fmt.Printf("  %s → %s (%d rooms, %d connections)\n",
			mapName, outPath, len(bp.Rooms), len(bp.Connections))
		count++
	}

	fmt.Printf("extracted %d blueprints\n", count)
	return nil
}

