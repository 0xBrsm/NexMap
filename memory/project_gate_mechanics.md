---
name: NexMap feature-gate mechanics + qlayout stairs connector
description: How tools/flowstruct.py actually computes the gate's areas/zband/chokepoints, and the new qlayout vertical connector
type: project
---

The `nexmap build` feature gate (tools/mapfeatures.py --gate) derives its SEVEN
structural scores from a navmesh built by `navcheck <bsp> -flow`, analyzed in
tools/flowstruct.py with cell=256, zcell=128.

Key levers (non-obvious, dug out of flowstruct.analyze):
- **areas** = count of distinct occupied nav cells, each `(round(x/256),
  round(y/256), round(z/128))`. To raise it: more walkable XY footprint AND/OR
  more distinct heights. Big single-tier boxes score low.
- **zband** = `len(distinct round(mean_cell_z / 96))`. So it counts ~96-unit
  elevation layers that hold walkable area. zband 3 means only 3 floor heights
  existed — NOT a navmesh failure. Spread floors across ~7+ heights to pass.
- **bcGini** (chokepoints) rises with real funnels — stair shafts / single
  doorways between rooms register strongly.
- **emptyA** (median raw nav-poly area) + **monoCV** (poly-area CV) — added
  2026-06-25 to catch the "sparse identical boxes" failure the other 5 missed.
  navcheck merges open floor into BIG polys and chops detail into small ones,
  so big-poly-median = emptiness, low-CV = monotony. Corpus bands (100 SP maps):
  emptyA p10/med/p90 = 128/256/1456 (warn HIGH >1500); monoCV = 2.3/3.1/4.9
  (warn LOW <2.3). These are the anti-inflation guard: you CANNOT pass `areas`
  by enlarging rooms (that spikes emptyA, flattens monoCV). Re-derive bands with
  tools/_ground_empty.py.

**Why:** our early maps failed areas/zband/chokepoints because qlayout rooms
were single-tier and vertical links had to be hand-built in qgeo.
**How to apply:** author height spread + footprint scale deliberately; terrace
rooms with the `connect("stairs", a, b)` connector (added 2026-06-25) — it
seals a stair shaft between side-by-side rooms at different z0 and the navmesh
climbs it, so verticality/areas/chokepoints come from the layout. The connector
auto-extends the flight INTO the lower room (STAIR_RUN_PER_RISE from METRICS) so
stairs aren't ladder-steep, gives full-height openings, and warns on stderr if
still steep. qcheck's `reachability` warning is a false positive over stairs
(its heuristic doesn't model stair traversal; the gate's real navmesh does).
