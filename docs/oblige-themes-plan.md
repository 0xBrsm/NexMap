# Oblige-Derived Theme & Prefab System

## Goal

Give an LLM the vocabulary to generate Quake maps that look like real Quake maps.
The blueprint schema is the API, themes/prefabs are the library, the LLM is the designer.

## Data Source

Oblige (v7.70) / Obsidian had unfinished Quake 1 support with:
- Two themes: `q1_tech` (base/industrial) and `q1_castle` (medieval)
- Per-surface texture pools with probability weights (walls/floors/ceilings/naturals)
- Environment types: building, cave, outdoor, hallway
- Skin/prefab system: doors, hallway pieces (I/C/T/P), stairs, lifts, decorations
- Entity catalog: 13 monsters, 7 weapons, all pickups/keys/powerups
- Source: `games/quake/` in obsidian-level-maker/Obsidian repo

## Architecture (Three Layers)

### Layer 1: Theme & Component Data (Go source)

Port Oblige's Lua tables as Go structs in `src/nexmap/themes.go`:

```go
type Theme struct {
    Name     string
    Building RoomTextures  // walls, floors, ceilings with weights
    Cave     RoomTextures  // naturals
    Outdoor  RoomTextures  // floors, naturals
    Hallway  RoomTextures
    Liquids  []string
    Sky      string
}

type WeightedTex struct {
    Name   string
    Weight int
}
```

Also: prefab definitions as parameterized brush generators in `src/nexmap/prefabs.go`.
Components: pillars, raised platforms, light alcoves, crate stacks, angled walls,
door frames, windows, trim bands.

### Layer 2: Expanded Blueprint Schema

New fields in room definitions:
```json
{
  "theme": "tech",
  "environment": "building",
  "wall_texture": "riveted_metal",
  "floor_texture": "dark_grating",
  "details": ["pillars_along_walls", "recessed_lights"],
  "lighting_mood": "dim_flickering"
}
```

Texture names are semantic (resolved to actual Quake textures by the compiler).
The LLM doesn't need to know `tech06_1` — it says `riveted_metal`.

### Layer 3: Markdown Reference for LLM Context

Auto-generated from Go source. Describes:
- Available themes and what they look like
- Every texture with natural-language description and usage tags
- Available architectural components with parameters
- Entity reference with gameplay notes
- Engine constraints (max faces, texture sizes, playable dimensions)

Fed to the LLM alongside the blueprint schema as system context.

## Oblige Texture Pools (from source)

### q1_tech (Base/Industrial)
- **Walls**: TECH06_1, TECH08_2, TECH09_3, TECH08_1, TECH13_2, TECH14_1, TWALL1_4, TECH14_2, TWALL2_3, TECH03_1, TECH05_1
- **Floors**: FLOOR01_5, METAL2_4, METFLOR2_1, MMETAL1_1, SFLOOR4_1/5/6/7, WIZMET1_2, MMETAL1_6/7/8, WMET4_5, WMET1_1
- **Ceilings**: (same pool as floors)
- **Hallway**: WOOD1_5 walls, WOODFLR1_5 floors, WOODFLR1_4 ceilings
- **Cave**: ROCK1_2, ROCK5_2, ROCK3_8, WALL11_2, GROUND1_6/7, GRAVE01_3, WSWAMP1_2
- **Outdoor floors**: CITY4_6, CITY6_7, CITY4_5/8, CITY6_8, WALL14_6, CITY4_1/2/7
- **Outdoor naturals**: GROUND1_2/5/6/7/8, ROCK3_7/8, ROCK4_2, VINE1_2
- **Sky**: sky4 (80%), sky1 (20%)
- **Liquids**: slime

### q1_castle (Medieval)
- **Walls**: ~40 textures (CITY/BRICKA/WSWAMP/ALTAR/COLUMN/METAL families)
- **Sky**: sky1 (80%), sky4 (20%)
- **Liquids**: lava1

## Prefab Components (from Oblige SKINS)

- Doors: plain, silver_key, gold_key (with decorative panels)
- Hallways: I-shape, C-shape (corner), T-shape (junction), P-shape (4-way)
- Stairs: multi-step with configurable delta
- Lifts: platform with trigger
- Decorations: pedestals, tech_lamps, pictures/reliefs, cages, windows
- Teleporters: pad with destination
- Exits: slipgate platform

## Implementation Order

1. `themes.go` — Port texture pools as Go data
2. Theme-aware texture selection in compiler (replace hardcoded `Textures.Floor` etc.)
3. `prefabs.go` — Parameterized detail brush generators (start with pillars + platforms)
4. Schema expansion — Add theme/environment/details fields to blueprint JSON
5. Reference doc generation — Markdown from Go source for LLM context
6. Rich example blueprints using full vocabulary
