package main

import "math/rand/v2"

// WeightedTex is a texture name with a probability weight.
type WeightedTex struct {
	Name   string
	Weight int
}

// RoomTextures holds texture pools for different surface types within an environment.
type RoomTextures struct {
	Walls    []WeightedTex
	Floors   []WeightedTex
	Ceilings []WeightedTex
	Naturals []WeightedTex // cave/outdoor rock textures
}

// Theme defines a complete visual theme for map generation.
type Theme struct {
	Name     string
	Building RoomTextures
	Cave     RoomTextures
	Outdoor  RoomTextures
	Hallway  RoomTextures
	Liquids  []string
	Sky      []WeightedTex
}

// PickWeighted selects a random texture from a weighted pool.
func PickWeighted(rng *rand.Rand, pool []WeightedTex) string {
	if len(pool) == 0 {
		return "tech01_1" // fallback
	}
	total := 0
	for _, t := range pool {
		total += t.Weight
	}
	r := rng.IntN(total)
	for _, t := range pool {
		r -= t.Weight
		if r < 0 {
			return t.Name
		}
	}
	return pool[0].Name
}

// PickLiquid returns a random liquid texture for the theme.
func (th *Theme) PickLiquid(rng *rand.Rand) string {
	if len(th.Liquids) == 0 {
		return "*water0"
	}
	return th.Liquids[rng.IntN(len(th.Liquids))]
}

// PickSky returns a weighted-random sky texture.
func (th *Theme) PickSky(rng *rand.Rand) string {
	return PickWeighted(rng, th.Sky)
}

// RoomTexturesFor returns the appropriate texture set for an environment type.
func (th *Theme) RoomTexturesFor(env string) *RoomTextures {
	switch env {
	case "cave":
		return &th.Cave
	case "outdoor":
		return &th.Outdoor
	case "hallway":
		return &th.Hallway
	default:
		return &th.Building
	}
}

// --- Theme definitions (ported from Oblige games/quake/themes.lua) ---

var ThemeTech = Theme{
	Name: "tech",
	Building: RoomTextures{
		Walls: []WeightedTex{
			{"tech06_1", 50}, {"tech08_2", 50}, {"tech09_3", 50}, {"tech08_1", 50},
			{"tech13_2", 50}, {"tech14_1", 50}, {"twall1_4", 50}, {"tech14_2", 50},
			{"twall2_3", 50}, {"tech03_1", 50}, {"tech05_1", 50},
		},
		Floors: []WeightedTex{
			{"floor01_5", 50}, {"metal2_4", 50}, {"metflor2_1", 50}, {"mmetal1_1", 50},
			{"sfloor4_1", 50}, {"sfloor4_5", 50}, {"sfloor4_6", 50}, {"sfloor4_7", 50},
			{"wizmet1_2", 50}, {"mmetal1_6", 50}, {"mmetal1_7", 50}, {"mmetal1_8", 50},
			{"wmet4_5", 50}, {"wmet1_1", 50},
		},
		Ceilings: []WeightedTex{
			{"floor01_5", 50}, {"metal2_4", 50}, {"metflor2_1", 50}, {"mmetal1_1", 50},
			{"sfloor4_1", 50}, {"sfloor4_5", 50}, {"sfloor4_6", 50}, {"sfloor4_7", 50},
			{"wizmet1_2", 50}, {"mmetal1_6", 50}, {"mmetal1_7", 50}, {"mmetal1_8", 50},
			{"wmet4_5", 50}, {"wmet1_1", 50},
		},
	},
	Hallway: RoomTextures{
		Walls:    []WeightedTex{{"wood1_5", 30}},
		Floors:   []WeightedTex{{"woodflr1_5", 50}},
		Ceilings: []WeightedTex{{"woodflr1_4", 50}},
	},
	Cave: RoomTextures{
		Naturals: []WeightedTex{
			{"rock1_2", 10}, {"rock5_2", 40}, {"rock3_8", 20}, {"wall11_2", 10},
			{"ground1_6", 10}, {"ground1_7", 10}, {"grave01_3", 10}, {"wswamp1_2", 20},
		},
	},
	Outdoor: RoomTextures{
		Floors: []WeightedTex{
			{"city4_6", 30}, {"city6_7", 30}, {"city4_5", 30}, {"city4_8", 30},
			{"city6_8", 30}, {"wall14_6", 20}, {"city4_1", 30}, {"city4_2", 30}, {"city4_7", 30},
		},
		Naturals: []WeightedTex{
			{"ground1_2", 50}, {"ground1_5", 50}, {"ground1_6", 20}, {"ground1_7", 30},
			{"ground1_8", 20}, {"rock3_7", 50}, {"rock3_8", 50}, {"rock4_2", 50}, {"vine1_2", 50},
		},
	},
	Liquids: []string{"*slime0", "*slime"},
	Sky:     []WeightedTex{{"sky4", 80}, {"sky1", 20}},
}

var ThemeCastle = Theme{
	Name: "castle",
	Building: RoomTextures{
		Walls: []WeightedTex{
			{"bricka2_4", 30}, {"city5_4", 30}, {"wall14_5", 30}, {"city1_4", 30},
			{"metal4_4", 20}, {"metalt1_1", 15}, {"city5_8", 40}, {"city5_7", 50},
			{"city6_3", 50}, {"city6_4", 50}, {"metal4_3", 20}, {"metalt2_2", 5},
			{"city2_1", 30}, {"city2_2", 30}, {"city2_3", 30}, {"city2_5", 30},
			{"metal4_2", 15}, {"metalt2_3", 20}, {"city2_6", 30}, {"city2_7", 30},
			{"city2_8", 30}, {"city6_7", 20}, {"metal4_7", 20}, {"metalt2_6", 5},
			{"city8_2", 30}, {"wall3_4", 30}, {"wall5_4", 30}, {"wbrick1_5", 30},
			{"metal4_8", 20}, {"metalt2_7", 20}, {"wmet4_8", 15}, {"wiz1_4", 30},
			{"wswamp1_4", 30}, {"wswamp2_1", 30}, {"wswamp2_2", 30}, {"altarc_1", 20},
			{"wmet4_3", 15}, {"wmet4_7", 15}, {"wwall1_1", 30}, {"altar1_3", 20},
			{"altar1_6", 5}, {"altar1_7", 20}, {"wmet4_4", 15}, {"wmet4_6", 15},
		},
		Floors: []WeightedTex{
			{"afloor1_4", 50}, {"afloor3_1", 25}, {"azfloor1_1", 20}, {"rock3_8", 20},
			{"metal5_4", 30}, {"floor01_5", 30}, {"city4_1", 15}, {"city4_2", 25},
			{"city4_5", 15}, {"city4_6", 20}, {"rock3_7", 20}, {"metal5_2", 30},
			{"mmetal1_2", 15}, {"city4_7", 15}, {"city4_8", 15}, {"city5_1", 30},
			{"city5_2", 30}, {"wall3_4", 30}, {"city6_8", 20}, {"city8_2", 20},
			{"ground1_8", 20}, {"afloor1_3", 20}, {"bricka2_4", 30}, {"wall9_8", 30},
			{"afloor1_8", 20}, {"woodflr1_5", 25}, {"bricka2_1", 30}, {"bricka2_2", 30},
			{"city6_7", 20},
		},
		Ceilings: []WeightedTex{
			{"dung01_4", 50}, {"dung01_5", 50}, {"ecop1_8", 50}, {"ecop1_4", 50},
			{"ecop1_6", 50}, {"wswamp1_4", 30}, {"wizmet1_1", 50}, {"wizmet1_4", 50},
			{"wizmet1_6", 50}, {"wizmet1_7", 50}, {"wiz1_1", 50}, {"wswamp2_1", 30},
			{"grave01_1", 50}, {"grave01_3", 50}, {"grave03_2", 50}, {"wall3_4", 30},
			{"wall5_4", 30}, {"wall11_2", 20}, {"wswamp2_2", 30}, {"wbrick1_5", 30},
			{"wiz1_4", 20}, {"cop1_1", 30}, {"cop1_2", 30}, {"cop1_8", 30}, {"cop2_2", 30},
			{"met5_1", 20}, {"metal1_1", 20}, {"metal1_2", 20}, {"metal1_3", 20}, {"wmet1_1", 15},
		},
	},
	Hallway: RoomTextures{
		Walls:    []WeightedTex{{"wood1_5", 30}},
		Floors:   []WeightedTex{{"woodflr1_5", 50}},
		Ceilings: []WeightedTex{{"woodflr1_4", 50}},
	},
	Cave: RoomTextures{
		Naturals: []WeightedTex{
			{"rock1_2", 10}, {"rock5_2", 40}, {"rock3_8", 20}, {"wall11_2", 10},
			{"ground1_6", 10}, {"ground1_7", 10}, {"grave01_3", 10}, {"wswamp1_2", 20},
		},
	},
	Outdoor: RoomTextures{
		Floors: []WeightedTex{
			{"city4_6", 30}, {"city6_7", 30}, {"city4_5", 30}, {"city4_8", 30},
			{"city6_8", 30}, {"wall14_6", 20}, {"city4_1", 30}, {"city4_2", 30}, {"city4_7", 30},
		},
		Naturals: []WeightedTex{
			{"ground1_2", 50}, {"ground1_5", 50}, {"ground1_6", 20}, {"ground1_7", 30},
			{"ground1_8", 20}, {"rock3_7", 50}, {"rock3_8", 50}, {"rock4_2", 50}, {"vine1_2", 50},
		},
	},
	Liquids: []string{"*lava1"},
	Sky:     []WeightedTex{{"sky1", 80}, {"sky4", 20}},
}

// ThemeByName looks up a theme. Falls back to tech.
var ThemesByName = map[string]*Theme{
	"tech":   &ThemeTech,
	"base":   &ThemeTech,
	"castle": &ThemeCastle,
	"medieval": &ThemeCastle,
	"runic":  &ThemeCastle,
}

func GetTheme(name string) *Theme {
	if th, ok := ThemesByName[name]; ok {
		return th
	}
	return &ThemeTech
}

// RoomMaterials holds the resolved textures for a specific room.
// Once picked, these are used consistently across the room.
type RoomMaterials struct {
	Wall    string
	Floor   string
	Ceiling string
}

// PickRoomMaterials selects textures for a room from a theme's pools.
func PickRoomMaterials(rng *rand.Rand, th *Theme, env string) RoomMaterials {
	rt := th.RoomTexturesFor(env)

	wall := PickWeighted(rng, rt.Walls)
	floor := PickWeighted(rng, rt.Floors)
	ceiling := PickWeighted(rng, rt.Ceilings)

	// Cave/outdoor: if no walls/floors/ceilings, use naturals.
	if len(rt.Walls) == 0 && len(rt.Naturals) > 0 {
		wall = PickWeighted(rng, rt.Naturals)
	}
	if len(rt.Floors) == 0 && len(rt.Naturals) > 0 {
		floor = PickWeighted(rng, rt.Naturals)
	}
	if len(rt.Ceilings) == 0 {
		if len(rt.Naturals) > 0 {
			ceiling = PickWeighted(rng, rt.Naturals)
		} else {
			ceiling = wall
		}
	}

	return RoomMaterials{Wall: wall, Floor: floor, Ceiling: ceiling}
}
