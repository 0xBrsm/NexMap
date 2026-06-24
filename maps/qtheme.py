"""Shared dimensional metrics and theme palettes for NexMap authoring.

Single source of truth for qprefab, qlayout, and tools/qcheck.
Numbers come from the id corpus (tools/qcorpus.py) and the archived
procgen constants; see .claude/skills/quake-mapping/ for derivations.
"""

METRICS = {
    # Player hull
    "player_w": 32,           # hull XY extent
    "player_h": 56,           # hull Z extent
    "player_radius": 16,
    "eye_height": 22,
    "max_climb": 18,          # max single step a player walks up

    # Passages
    "door_w": 64,             # minimum clear doorway width
    "door_h": 88,             # minimum clear doorway height
    "corridor_w": 96,
    "corridor_headroom": 112,

    # Stairs
    "step_h": 16,             # corpus-dominant rise
    "step_run": 24,           # comfortable run for 16h (24-32 ok)
    "max_stair_rise": 96,     # unbroken rise before a landing is required
    "landing_depth": 64,      # min depth of a stair landing

    # Entities
    "item_gap_max": 24,       # item origin may sit 0..24 above floor top
    "item_z": 24,             # canonical item origin height above floor
    "entity_spacing": 64,     # min XY distance between items/spawns
    "wall_margin": 48,        # min distance from item to a wall

    # Lighting (corpus bands)
    "light_p25": 150,
    "light_median": 200,
    "light_p90": 300,
    "lights_per_map": (190, 235),

    # Structure
    "wall_thickness": 16,
    "trim": 8,
    "pillar_w": 32,
    "pillar_base_w": 48,
    "pillar_base_h": 8,
    "recess_w": 48,
    "recess_d": 16,
    "recess_h": 64,
}

# Palette dicts. Keys used by qprefab/qlayout:
#   wall/floor/ceiling/trim  - primary surfaces
#   pier                     - pillars, jambs, structural verticals
#   panel                    - embedded light-panel texture ("" = theme has none)
#   liquid                   - theme liquid
#   torch                    - fixture classname for flame light ("" = none; use plain light)
#   light_value              - default point-light value
THEMES = {
    # Runic metal: dm2, dm4, e1m6-8, e3m2-7. The Quake signature look.
    "metal": {
        "wall": "metal4_4",
        "floor": "metal1_4",
        "ceiling": "met5_1",
        "trim": "cop1_1",
        "pier": "metal1_3",
        "panel": "light3_8",
        "liquid": "*lava1",
        "torch": "",
        "light_value": 200,
    },
    # Military base: e1m1, e2m1, e3m1, e4m1, dm3.
    "tech": {
        "wall": "tech04_3",
        "floor": "sfloor4_2",
        "ceiling": "tech04_7",
        "trim": "ecop1_4",
        "pier": "tech05_2",
        "panel": "tlight08",
        "liquid": "*slime0",
        "torch": "",            # tech uses light_fluoro, see fluoro flag below
        "fluoro": True,
        "light_value": 200,
    },
    # Azure city: dm5, dm6, e1m5, e2m3, e2m7, e4m3. Torch-lit stone.
    "city": {
        "wall": "city6_8",
        "floor": "city4_7",
        "ceiling": "city2_7",
        "trim": "cop2_3",
        "pier": "city2_6",
        "panel": "",
        "liquid": "*04water2",
        "torch": "light_torch_small_walltorch",
        "light_value": 200,
    },
    # Wizard swamp keep: e1m2-4, e2m2, e2m6. Heaviest torch usage.
    "wizard": {
        "wall": "wbrick1_5",
        "floor": "wswamp2_1",
        "ceiling": "wizmet1_2",
        "trim": "wmet1_1",
        "pier": "wiz1_4",
        "panel": "",
        "liquid": "*04water1",
        "torch": "light_torch_small_walltorch",
        "light_value": 220,     # dark families read ~2x darker; more lights, slightly hotter
    },
}

# All textures considered "in palette" per theme, for coherence checks.
# Families (prefix match) from themes.md secondary/accent lists.
THEME_FAMILIES = {
    "metal": ["metal", "met", "mmetal", "nmetal", "cop", "rune", "light3",
              "light1", "m5", "*lava", "*teleport", "sky"],
    "tech": ["tech", "sfloor", "ecop", "uwall", "twall", "comp", "tlight",
             "*slime", "*water", "*teleport", "sky"],
    "city": ["city", "cop", "wwall", "wall", "wood", "window", "*04water",
             "*04awater", "*teleport", "sky"],
    "wizard": ["wbrick", "wizmet", "wswamp", "wiz", "rock", "wizwood", "wmet",
               "wood", "*04water", "*slime", "*teleport", "sky"],
}
