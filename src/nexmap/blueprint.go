package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// --- Resolution tables ---

var sizeRanges = map[string][2]int{
	"small":  {256, 384},
	"medium": {384, 640},
	"large":  {640, 896},
	"huge":   {896, 1024},
}

var elevationRanges = map[string][2]int{
	"ground": {0, 0},
	"low":    {-128, -64},
	"mid":    {64, 192},
	"high":   {256, 384},
	"varied": {-128, 384},
}

var ceilingMap = map[string]int{
	"low":       128,
	"normal":    192,
	"tall":      256,
	"cavernous": 320,
}

var connectionWidths = map[string]int{
	"narrow": 64,
	"normal": 96,
	"wide":   160,
}

var hazardCoverage = map[string]float64{
	"small":  0.15,
	"medium": 0.30,
	"large":  0.50,
}

var hazardTextures = map[string]string{
	"lava":  "*lava1",
	"water": "*04water1",
	"slime": "*slime",
}

// --- Raw JSON types ---

type rawBlueprint struct {
	Version     int              `json:"version"`
	Meta        rawMeta          `json:"meta"`
	Rooms       []rawRoom        `json:"rooms"`
	Connections []rawConnection  `json:"connections"`
	Flow        *rawFlow         `json:"flow,omitempty"`
	Textures    map[string]string `json:"textures,omitempty"`
}

type rawMeta struct {
	Name        string          `json:"name"`
	Author      string          `json:"author,omitempty"`
	Description string          `json:"description,omitempty"`
	Theme       string          `json:"theme"`
	Style       string          `json:"style,omitempty"`    // source map style (e.g. "dm4")
	CustomTheme string          `json:"custom_theme,omitempty"`
	Gamemode    string          `json:"gamemode"`
	PlayerCount *rawPlayerCount `json:"player_count,omitempty"`
	ArenaSize   int             `json:"arena_size,omitempty"`
	DesignNotes string          `json:"design_notes,omitempty"`
}

type rawPlayerCount struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type rawRoom struct {
	ID          string          `json:"id"`
	Role        string          `json:"role"`
	Label       string          `json:"label,omitempty"`
	Size        string          `json:"size,omitempty"`
	Shape       string          `json:"shape,omitempty"`
	Elevation   string          `json:"elevation,omitempty"`
	Ceiling     string          `json:"ceiling,omitempty"`
	Environment string          `json:"environment,omitempty"` // building, cave, outdoor, hallway
	Details     []string        `json:"details,omitempty"`     // pillars, platform, light_recesses, wall_trim, crates, step_down
	Hazard      *rawHazard      `json:"hazard,omitempty"`
	Items       []rawItem       `json:"items,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
}

type rawHazard struct {
	Type     string `json:"type"`
	Coverage string `json:"coverage,omitempty"`
}

type rawItem struct {
	Classname string `json:"classname"`
	Placement string `json:"placement,omitempty"`
	Intent    string `json:"intent,omitempty"`
	Count     int    `json:"count,omitempty"`
}

type rawConnection struct {
	From          string `json:"from"`
	To            string `json:"to"`
	Type          string `json:"type"`
	Bidirectional *bool  `json:"bidirectional,omitempty"`
	Width         string `json:"width,omitempty"`
	Intent        string `json:"intent,omitempty"`
}

type rawFlow struct {
	PrimaryLoop    []string     `json:"primary_loop,omitempty"`
	SecondaryLoops [][]string   `json:"secondary_loops,omitempty"`
	ChokePoints    []string     `json:"choke_points,omitempty"`
	Sightlines     []rawSight   `json:"sightlines,omitempty"`
}

type rawSight struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Description string `json:"description,omitempty"`
}

// --- Resolved types ---

type ResolvedItem struct {
	Classname string
	Placement string
	Intent    string
	Count     int
}

type ResolvedHazard struct {
	Type     string
	Texture  string
	Coverage float64
}

type ResolvedRoom struct {
	ID           string
	Role         string
	Label        string
	SizeMin      int
	SizeMax      int
	Shape        string
	ElevationMin int
	ElevationMax int
	CeilingHeight int
	Environment  string   // building, cave, outdoor, hallway
	Details      []Detail // architectural details to place
	Hazard       *ResolvedHazard
	Items        []ResolvedItem
	Tags         []string
}

type ResolvedConnection struct {
	FromID        string
	ToID          string
	Type          string
	Bidirectional bool
	Width         int
	Intent        string
}

type ResolvedBlueprint struct {
	Version       int
	Name          string
	Author        string
	Description   string
	Theme         string
	Style         string // source map style (e.g. "dm4")
	Gamemode      string
	PlayerMin     int
	PlayerMax     int
	ArenaSize     int
	DesignNotes   string
	Rooms         []ResolvedRoom
	Connections   []ResolvedConnection
	TextureOverrides map[string]string
}

func (bp *ResolvedBlueprint) RoomByID(id string) *ResolvedRoom {
	for i := range bp.Rooms {
		if bp.Rooms[i].ID == id {
			return &bp.Rooms[i]
		}
	}
	return nil
}

// --- Validation ---

func validateBlueprint(raw *rawBlueprint) []string {
	var errs []string

	if raw.Version != 1 {
		errs = append(errs, "version must be 1")
	}
	if raw.Meta.Name == "" {
		errs = append(errs, "meta.name is required")
	}
	if raw.Meta.Theme == "" {
		errs = append(errs, "meta.theme is required")
	}
	if raw.Meta.Gamemode == "" {
		errs = append(errs, "meta.gamemode is required")
	}
	if len(raw.Rooms) < 2 {
		errs = append(errs, "need at least 2 rooms")
	}

	ids := map[string]bool{}
	for i, r := range raw.Rooms {
		if r.ID == "" {
			errs = append(errs, fmt.Sprintf("rooms[%d] missing id", i))
		} else if ids[r.ID] {
			errs = append(errs, fmt.Sprintf("duplicate room id: %s", r.ID))
		}
		ids[r.ID] = true
		if r.Role == "" {
			errs = append(errs, fmt.Sprintf("rooms[%d] missing role", i))
		}
	}

	if len(raw.Connections) < 1 {
		errs = append(errs, "need at least 1 connection")
	}

	// Connection reference check.
	for i, c := range raw.Connections {
		if !ids[c.From] {
			errs = append(errs, fmt.Sprintf("connections[%d].from references unknown room '%s'", i, c.From))
		}
		if !ids[c.To] {
			errs = append(errs, fmt.Sprintf("connections[%d].to references unknown room '%s'", i, c.To))
		}
	}

	// BFS connectivity.
	if len(errs) == 0 {
		adj := map[string][]string{}
		for _, id := range raw.Rooms {
			adj[id.ID] = nil
		}
		for _, c := range raw.Connections {
			adj[c.From] = append(adj[c.From], c.To)
			bidir := c.Bidirectional == nil || *c.Bidirectional
			if bidir {
				adj[c.To] = append(adj[c.To], c.From)
			}
		}
		visited := map[string]bool{}
		queue := []string{raw.Rooms[0].ID}
		for len(queue) > 0 {
			node := queue[0]
			queue = queue[1:]
			if visited[node] {
				continue
			}
			visited[node] = true
			queue = append(queue, adj[node]...)
		}
		var unreachable []string
		for _, r := range raw.Rooms {
			if !visited[r.ID] {
				unreachable = append(unreachable, r.ID)
			}
		}
		if len(unreachable) > 0 {
			errs = append(errs, fmt.Sprintf("unreachable rooms (disconnected graph): %v", unreachable))
		}
	}

	return errs
}

// --- Resolution ---

func resolveBlueprint(raw *rawBlueprint) *ResolvedBlueprint {
	meta := raw.Meta

	playerMin, playerMax := 2, 8
	if meta.PlayerCount != nil {
		if meta.PlayerCount.Min > 0 {
			playerMin = meta.PlayerCount.Min
		}
		if meta.PlayerCount.Max > 0 {
			playerMax = meta.PlayerCount.Max
		}
	}

	arenaSize := 3072
	if meta.ArenaSize > 0 {
		arenaSize = meta.ArenaSize
	}

	theme := meta.Theme
	if theme == "custom" && meta.CustomTheme != "" {
		theme = meta.CustomTheme
	}

	bp := &ResolvedBlueprint{
		Version:     raw.Version,
		Name:        meta.Name,
		Author:      orDefault(meta.Author, "NexMap LLM"),
		Description: meta.Description,
		Theme:       theme,
		Style:       meta.Style,
		Gamemode:    meta.Gamemode,
		PlayerMin:   playerMin,
		PlayerMax:   playerMax,
		ArenaSize:   arenaSize,
		DesignNotes: meta.DesignNotes,
		TextureOverrides: raw.Textures,
	}

	for _, rr := range raw.Rooms {
		bp.Rooms = append(bp.Rooms, resolveRoom(rr))
	}
	for _, rc := range raw.Connections {
		bp.Connections = append(bp.Connections, resolveConnection(rc))
	}

	return bp
}

func resolveRoom(r rawRoom) ResolvedRoom {
	sz := orDefault(r.Size, "medium")
	rng, ok := sizeRanges[sz]
	if !ok {
		rng = sizeRanges["medium"]
	}

	elev := orDefault(r.Elevation, "ground")
	er, ok := elevationRanges[elev]
	if !ok {
		er = elevationRanges["ground"]
	}

	ceil := orDefault(r.Ceiling, "normal")
	ch, ok := ceilingMap[ceil]
	if !ok {
		ch = ceilingMap["normal"]
	}

	var hazard *ResolvedHazard
	if r.Hazard != nil {
		cov := 0.30
		if v, ok := hazardCoverage[orDefault(r.Hazard.Coverage, "medium")]; ok {
			cov = v
		}
		hazard = &ResolvedHazard{
			Type:     r.Hazard.Type,
			Texture:  hazardTextures[r.Hazard.Type],
			Coverage: cov,
		}
	}

	var items []ResolvedItem
	for _, ri := range r.Items {
		count := ri.Count
		if count < 1 {
			count = 1
		}
		items = append(items, ResolvedItem{
			Classname: ri.Classname,
			Placement: orDefault(ri.Placement, "any"),
			Intent:    ri.Intent,
			Count:     count,
		})
	}

	var details []Detail
	for _, d := range r.Details {
		details = append(details, Detail(d))
	}

	return ResolvedRoom{
		ID:           r.ID,
		Role:         r.Role,
		Label:        orDefault(r.Label, r.ID),
		SizeMin:      rng[0],
		SizeMax:      rng[1],
		Shape:        orDefault(r.Shape, "any"),
		ElevationMin: er[0],
		ElevationMax: er[1],
		CeilingHeight: ch,
		Environment:  orDefault(r.Environment, "building"),
		Details:      details,
		Hazard:       hazard,
		Items:        items,
		Tags:         r.Tags,
	}
}

func resolveConnection(c rawConnection) ResolvedConnection {
	bidir := true
	if c.Bidirectional != nil {
		bidir = *c.Bidirectional
	}
	w := 96
	if v, ok := connectionWidths[orDefault(c.Width, "normal")]; ok {
		w = v
	}
	return ResolvedConnection{
		FromID:        c.From,
		ToID:          c.To,
		Type:          c.Type,
		Bidirectional: bidir,
		Width:         w,
		Intent:        c.Intent,
	}
}

// --- Public API ---

func LoadBlueprint(path string) (*ResolvedBlueprint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading blueprint: %w", err)
	}

	var raw rawBlueprint
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing blueprint JSON: %w", err)
	}

	if errs := validateBlueprint(&raw); len(errs) > 0 {
		return nil, fmt.Errorf("blueprint validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return resolveBlueprint(&raw), nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
