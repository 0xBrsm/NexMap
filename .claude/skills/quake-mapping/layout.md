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
