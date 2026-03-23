# NexMap Blueprint Reference (for LLM context)

You are generating Quake 1 map blueprints. This reference describes the
available themes, textures, architectural details, and entities.

## Blueprint format

```json
{
  "version": 1,
  "meta": {
    "name": "Map Name",
    "theme": "tech",
    "style": "dm4",
    "gamemode": "deathmatch",
    "player_count": {"min": 2, "max": 8},
    "arena_size": 3072,
    "design_notes": "Brief description of design intent"
  },
  "rooms": [...],
  "connections": [...],
  "flow": {...}
}
```

## Themes

### `style` field (optional)
Reference a specific Quake map to use its exact texture palette:
`dm1` through `dm6` (deathmatch), `e1m1` through `e4m8` (episodes), `start`.
Example: `"style": "dm4"` uses dm4's textures (dark metal with copper floors).
If omitted, the compiler picks palettes from the theme's pool.

### Theme presets

### `tech` (aliases: `base`)
Industrial/military. Metal walls, tech panels, grated floors.
Liquids: slime. Sky: overcast gray.

### `castle` (aliases: `medieval`, `runic`)
Medieval fortress. Stone/brick walls, ornate ceilings, wooden elements.
Liquids: lava. Sky: cloudy.

## Room fields

| Field | Values | Default | Description |
|-------|--------|---------|-------------|
| `id` | string | required | Unique room identifier |
| `role` | arena, sniper_perch, control_point, supply, transition, spawn | required | Gameplay role |
| `size` | small, medium, large, huge | medium | Room dimensions |
| `shape` | any, square, wide, tall_narrow | any | Aspect ratio hint |
| `elevation` | ground, low, mid, high, varied | ground | Floor height |
| `ceiling` | low, normal, tall, cavernous | normal | Ceiling height |
| `environment` | building, cave, outdoor, hallway | building | Texture environment |
| `details` | array of detail names | [] | Architectural features |

## Architectural details

| Detail | Min room size | Description |
|--------|--------------|-------------|
| `pillars` | large | Four columns near room corners |
| `platform` | large | Raised center platform (32 units) |
| `light_recesses` | medium | Wall alcoves with lights, one per wall |
| `wall_trim` | small | Decorative step at base of all walls |
| `crates` | medium | 2-4 crate stacks near walls |
| `step_down` | large | Raised center creating a sunken perimeter |

Notes:
- `platform` and `step_down` are mutually exclusive
- `pillars` conflict with `platform`/`step_down`
- Don't use details with rooms that have hazard pools (except `wall_trim` and `light_recesses`)

## Connections

| Field | Values | Description |
|-------|--------|-------------|
| `from`, `to` | room IDs | Connected rooms |
| `type` | corridor, stairs, drop, teleporter | Connection type |
| `width` | narrow, normal, wide | Corridor width |
| `bidirectional` | true/false | Default true. Drops are typically one-way |

## Items (classnames)

### Weapons
- `weapon_supershotgun` — Close range, common
- `weapon_nailgun` — Medium range, reliable
- `weapon_supernailgun` — High DPS, eats ammo
- `weapon_grenadelauncher` — Area denial, bouncing
- `weapon_rocketlauncher` — The power weapon, splash damage
- `weapon_lightning` — Hitscan, high damage, uses cells

### Ammo
`item_shells`, `item_spikes`, `item_rockets`, `item_cells`

### Armor
- `item_armor1` — Green armor (100 points)
- `item_armor2` — Yellow armor (150 points)
- `item_armorInv` — Red armor (200 points, rare)

### Health
`item_health` — 25 HP pickup

### Powerups (use sparingly, 0-1 per map)
- `item_artifact_super_damage` — Quad damage
- `item_artifact_invulnerability` — Pentagram of protection
- `item_artifact_invisibility` — Ring of shadows

### Spawns
- `info_player_start` — Required (exactly 1)
- `info_player_deathmatch` — DM spawn (1 per room typical)

### Lights
- `light` — Point light source (place 2-4 per room)

## Item placement hints

| Placement | Description |
|-----------|-------------|
| `center` | Room center — for key weapons/powerups |
| `corner` | Room corners — for spawns, armor |
| `wall` | Against walls — for health, ammo |
| `near_hazard` | Near pool edges — risk/reward items |
| `hidden` | Farthest corner from center — secrets |
| `any` | Random valid position |

## Hazards

```json
"hazard": {"type": "lava", "coverage": "medium"}
```

Types: `lava` (damage), `water` (safe), `slime` (damage)
Coverage: `small` (15%), `medium` (30%), `large` (50%)

## Design guidelines

- 4-8 rooms for a good DM map
- Every room should be reachable (connected graph)
- Place the best weapon (RL) in a high-risk location
- Red armor should be contestable, not hidden
- Elevation variation creates interesting combat
- At least 2 connection paths between major rooms (loops, not dead ends)
- Use `drop` connections for one-way shortcuts (commit to the jump)
- Teleporters for connecting distant rooms
- Lights: 2-4 per room, more for arenas

## Engine constraints

- Room dimensions: 256-1024 units per side
- Total arena: ~3072 units recommended
- Corridors: 96 units wide (default)
- Max elevation difference between connected rooms: ~96 units (stairs limit)
- All coordinates must stay within -4000 to +4000
