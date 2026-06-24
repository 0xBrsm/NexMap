package main

import "math/rand/v2"

// TexturePalette is a curated set of textures that look good together.
// Derived from analyzing which textures co-occur in original Quake maps.
type TexturePalette struct {
	Name    string // descriptive name for LLM reference
	Wall    string // primary wall texture
	Floor   string // floor texture (should contrast with wall)
	Ceiling string // ceiling texture
	Trim    string // accent/trim texture for details
}

// Palettes per theme, mined from shareware BSP co-occurrence data.
// Each palette represents a cohesive look observed in real id1 maps.

var TechPalettes = []TexturePalette{
	{
		Name: "slipgate_complex", // e1m1/e1m2 style
		Wall: "slipside", Floor: "sfloor4_2", Ceiling: "sliplite", Trim: "slipbotsd",
	},
	{
		Name: "dark_metal", // e1m6/e1m7 style
		Wall: "met5_1", Floor: "metal4_4", Ceiling: "mmetal1_1", Trim: "metal4_5",
	},
	{
		Name: "tech_base", // clean industrial
		Wall: "tech08_1", Floor: "sfloor4_1", Ceiling: "tech07_2", Trim: "tech09_3",
	},
	{
		Name: "heavy_metal", // e1m8 style
		Wall: "metal4_4", Floor: "metal5_1", Ceiling: "cop1_3", Trim: "metal4_5",
	},
	{
		Name: "copper_works",
		Wall: "cop1_1", Floor: "sfloor4_5", Ceiling: "cop1_8", Trim: "cop1_2",
	},
	{
		Name: "tech_panel",
		Wall: "tech06_1", Floor: "metal2_4", Ceiling: "tech01_1", Trim: "tech04_1",
	},
	{
		Name: "grime",
		Wall: "tech14_2", Floor: "metflor2_1", Ceiling: "mmetal1_6", Trim: "tech13_2",
	},
}

var CastlePalettes = []TexturePalette{
	{
		Name: "wizard_metal", // e1m3 style
		Wall: "wmet4_7", Floor: "wizmet1_2", Ceiling: "wbrick1_5", Trim: "wmet4_5",
	},
	{
		Name: "swamp_brick", // e1m4 style
		Wall: "wbrick1_5", Floor: "rock1_2", Ceiling: "wswamp1_4", Trim: "wswamp2_1",
	},
	{
		Name: "city_stone", // start map style
		Wall: "city4_6", Floor: "city5_1", Ceiling: "wiz1_4", Trim: "city4_7",
	},
	{
		Name: "dark_castle", // e1m5 style
		Wall: "cop1_1", Floor: "citya1_1", Ceiling: "city2_7", Trim: "cop1_8",
	},
	{
		Name: "altar",
		Wall: "altarc_1", Floor: "afloor1_4", Ceiling: "dung01_4", Trim: "altar1_7",
	},
	{
		Name: "dungeon",
		Wall: "city6_3", Floor: "azfloor1_1", Ceiling: "dung01_5", Trim: "city6_4",
	},
	{
		Name: "ornate",
		Wall: "city5_7", Floor: "afloor3_1", Ceiling: "ecop1_4", Trim: "metal4_3",
	},
}

// PalettesForTheme returns the palette set for a theme.
func PalettesForTheme(th *Theme) []TexturePalette {
	if th.Name == "castle" {
		return CastlePalettes
	}
	return TechPalettes
}

// PickPalette selects a random palette for a theme.
func PickPalette(rng *rand.Rand, th *Theme) TexturePalette {
	pals := PalettesForTheme(th)
	return pals[rng.IntN(len(pals))]
}

// ToRoomMaterials converts a palette to RoomMaterials.
func (p TexturePalette) ToRoomMaterials() RoomMaterials {
	return RoomMaterials{
		Wall:    p.Wall,
		Floor:   p.Floor,
		Ceiling: p.Ceiling,
	}
}
