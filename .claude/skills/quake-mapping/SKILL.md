---
name: quake-mapping
description: Author Quake 1 maps in the style of the id originals. Use when writing or revising a map script in maps/, choosing textures/themes, placing lights, or critiquing renders. Covers workflow, scale, theme palettes, and lighting grammar derived from the 38 original id .map sources.
---

# Quake Mapping

Author maps as Python scripts (`maps/<name>.py`). Build and iterate with:

    ./src/nexmap/nexmap build maps/<name>.py [-cams "x y z yaw pitch;..."] [-deploy]

The pipeline fails loudly on invalid brushes, leaks, unsealed maps, and zero
lightdata. Never pass `-bounce` to light (vendored build silently produces a
fullbright map).

## The authoring stack (use it — don't freehand brushes)

Hand-placed raw coordinates are the source of the boxy/floating/dead-zone
failures. The stack exists so each failure mode is fixed once, in code:

- `maps/qtheme.py` — `METRICS` (hull, steps, doorways, lighting bands) and
  `THEMES` palettes. Single source of truth for dimensions and textures.
- `maps/qlayout.py` — the layout IR. Describe `Room`s and `connect()` them
  (door/corridor/arch/stairs/drop/teleport). Rooms seal themselves, portals
  punch automatically, and a **build-time navcheck** makes an unconnected
  room a hard error. `Room.item()` seats items on pedestals so they never
  float. `shell=False` is the escape hatch for hand-built geometry.
- `maps/qprefab.py` — parametric prefabs (landed stairs, archways, pillars,
  curved/chamfered walls, item pedestals, light fixtures, catwalks, vaults,
  teleporter pads). Catalog in [prefabs.md](prefabs.md).
- `maps/qgeo.py` — low-level brush primitives. Reach for these only when a
  prefab/room doesn't cover the shape.
- `tools/qcheck.py` — deterministic validator wired into `nexmap build`. It
  runs before qbsp and **hard-fails the build** on floating/embedded/colliding
  entities; it warns on missing landings, unreachable items, thin wall margins,
  low headroom, sparse/flat lighting, off-palette textures, and **poor flow**.
- `tools/qflow.py` — topology analysis of the walkable nav graph (loops,
  dead ends, coverage, elevation, sightlines). The same metrics qcheck's flow
  warnings use; run `python3 tools/qflow.py corpus` to see the id bands or
  `qflow out/foo.map` to grade one map. Flow is *movement* quality — the thing
  renders can't show.

## The mandatory loop

1. **Spec** — name the theme (one only), the rooms, the connection graph, the
   item/spawn plan. A few lines of intent before any geometry.
2. **Layout** — build it with `qlayout.Room` + `connect()`. Let the navcheck
   and self-sealing do their job; drop to `qprefab`/`qgeo` for detail.
3. **Build** — `nexmap build`. The qcheck gate must pass (fix every FAIL).
4. **Render-critique** — Read the PNGs from every camera and grade against the
   rubric below. Iterate until it passes. Never deploy a map you haven't
   looked at, and never ship while any rubric point is failing.

## Ship rubric (all seven must pass before deploy)

0. **Flow & structure** — the `nexmap build` feature gate is IN BAND on all
   five targets (see "Structural targets" below): occlusion, scale,
   chokepoints, verticality, sightline variety. These supersede raw loop
   density (a misleading descriptor — it rates over-connected blobs highest).
   Continuous ring walkways, bridges, and multiple stairs between tiers make
   real flow; spurs and single-entrance rooms make dead ends. A beautiful map
   that plays as a tree — or as an over-exposed blob — still fails.

1. **Mood lighting** — light *pools and falls off*; visible sources motivate
   every bright patch; no flat wash, no `_minlight` > 30. Dark families
   (wizard/metal) need ~2x the light count/value of brown base.
2. **Curves** — at least one arch, vault, chamfer, or swept wall. No room is a
   bare box; no run of unbroken 90-degree corners.
3. **Scale** — rooms read generous, not cramped; headroom > a jump; sightlines
   that span more than one room.
4. **Stair landings** — every flight has a bottom landing; runs over 96 of
   rise are broken by a mid-landing (use `qprefab.stairs_landed`).
5. **Seated items/spawns** — nothing floats or embeds; pickups sit on the
   floor or a pedestal; qcheck item/spacing checks pass clean.
6. **Mystery/atmosphere** — at least one concealed, teased, or vertically
   revealed space; not every area visible from spawn.

## Structural targets (the feature gate)

`nexmap build` runs a feature gate after compile (`tools/mapfeatures.py --gate`)
that scores the map's navmesh-derived structure against the SP good-region
measured from 97 exemplar maps (base Quake + DOPA + Scourge of Armagon +
Dissolution of Eternity + udob + AD). Aim for the median; a WARN means you're
outside the range *every* shipped id-quality map sits in. The five that matter
(each a distinct failure mode our early maps hit):

| target | band (p10–p90) | median | failure if outside |
|---|---|---|---|
| **occlusion** (vis_density) | 0.03–0.23 | 0.05 | too high = over-exposed blob, no mystery |
| **scale** (areas) | 125–395 | 245 | too low = small / under-differentiated |
| **chokepoints** (bc_gini) | 0.68–0.80 | 0.74 | too low = flat, no real funnels |
| **verticality** (zband) | 7–17 | 11 | too low = flat, no layering |
| **sightline variety** (sight_cv) | 0.47–0.64 | 0.54 | too low = uniform, no intimate/vista mix |

**Author toward these, don't debug into them.** Occlusion is the dominant one:
break sightlines, layer space vertically, hide areas. NOTE: raw *loop density*
is NOT a quality target — it's a descriptor that mis-rates blobs as good (our
worst map topped it). Trust the gate's five features over loop counts.

## Core invariants (from the id corpus)

All numbers below were measured from the 38 original id .map sources.
Refresh or dig deeper with `python3 tools/qcorpus.py {themes|role|pairs|lights|dims|ents|sql}`
(corpus db: `~/.cache/nexmap/corpus.db`; sources: `~/.cache/nexmap/quake_map_source`).

**Scale** (player hull is 32x32x56, eye height 22, max step 18):
- Step height: 16 (dominant), 8 for subtle grade changes, 24 only with momentum
- Structural thickness: 16 and 32 are the workhorses; 8 for trim/inset panels
- Walls/floors between rooms: 16+ thick; thinner reads flimsy and risks leaks

**Theme discipline**: pick ONE theme and stay inside its texture families.
The four id themes and their palettes are in [themes.md](themes.md).
Mixing families across themes is the #1 "AI map" tell.

**Lighting grammar**: many moderate lights, not few bright ones. id used
~200 light entities per map, median value 200, p90 250-300. Details and
per-theme fixtures in [lighting.md](lighting.md).

**Layout numbers**: step/stair, thickness, and dimension data in
[layout.md](layout.md).

## Authoring rules of thumb

- Texture whole brushes with one texture (id did); use distinct face textures
  only for deliberate accents (trim strips, demon faces, light panels).
- Every functional element must be visually announced: teleporters get
  `*teleport` faces and a frame, buttons get button textures, secrets get a
  texture misalignment — never bare invisible triggers.
- Liquids: `*lava1` (metal), `*slime0` (tech), `*water1`/`*04water1`
  (wizard/city). Liquid theme-match matters as much as wall textures.
- Sky: sealed sky boxes textured `sky4` (or theme sky) on all faces.
