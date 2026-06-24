# Layout and dimension numbers

From `python3 tools/qcorpus.py dims` over the id originals, plus engine
constants.

## Engine constants (fixed)

- Player hull: 32 x 32 x 56; eye height 22; crouch does not exist in Quake 1
- Max step-up: 18 units; max jump: ~43 units onto a ledge
- Doorways must be >= 64 wide x 88 tall to feel comfortable (id commonly used
  64/96/128 widths)
- Grid discipline: keep structural geometry on 8- or 16-unit grid; off-grid
  invites qbsp cracks and ugly texture alignment

## Measured from the corpus

Step heights (walkable brushes, dz 4-24):

| dz | count | use |
|----|-------|-----|
| 16 | 5599  | the standard stair/step |
| 24 | 934   | aggressive steps, ramped feel |
| 8  | 703   | subtle grade changes, trim steps |

Brush thickness (min dimension) — the structural vocabulary:

| thickness | count | typical use |
|-----------|-------|-------------|
| 16  | 11613 | walls, floors, platforms — the default |
| 32  | 8389  | heavy walls, load-bearing look |
| 8   | 3020  | trim, inset panels, rails |
| 24  | 2998  | chunky trim, steps |
| 64  | 2591  | pillars, massive structure |

## Rules of thumb

- Stairs: 16 rise / 24-32 run reads as id-standard.
- Vary ceiling heights between connected spaces — uniform height is the
  flat-map tell. Corridors low (96-128), chambers high (192-320+).
- Rooms are not boxes: inset trim lines at floor/ceiling junctions (8-16
  thick), beams, pillars, and wall panels break up large flat faces.
- Not yet measured (query the corpus if needed): room footprints, corridor
  widths, ceiling-height distributions. A future qcorpus query could derive
  these from floor/ceiling face gaps.

## These numbers live in code: `maps/qtheme.py` `METRICS`

Don't retype the constants above into a map — import them. `METRICS` carries
player hull, `step_h`/`step_run`, `max_stair_rise`/`landing_depth`,
`door_w`/`door_h`, `corridor_w`/`corridor_headroom`, `item_z`/`item_gap_max`,
`entity_spacing`, `wall_margin`, `wall_thickness`, and the lighting bands.
`tools/qcheck.py` enforces them, so keeping authoring and validation on the
same source prevents drift.

## Building layouts with `qlayout`

```python
from qlayout import Layout
L = Layout("My Arena", "metal", worldtype=1, minlight=6)
hub  = L.room("hub", -384,-384,384,384, 0,320, chamfer=48)
vault = L.room("vault", 512,-256,1024,256, 0,256)
L.connect("corridor", hub, vault)          # fills + seals the gap, lights it
L.connect("arch", hub, side, width=128)    # curved opening (adjacent rooms)
L.connect("teleport", vault, hub, a_at=(768,0), b_at=(0,0))
hub.item("item_artifact_invulnerability", 0, 0)   # seated on a pedestal
hub.spawn(-300, -300, angle=45, dm=False)
L.write("out/myarena.map")                  # navcheck runs here; raises if a
                                            # room is unreachable
```

Rules: `door`/`arch` are for **adjacent** rooms (walls ~touching); `corridor`
for **separated** rooms (it builds the connecting tube). Connections punch both
walls automatically. Every room needs a connection path from the spawn room or
the build raises before it ever reaches qbsp.
