---
name: quake-mapping
description: Author Quake 1 maps in the style of the id originals. Use when writing or revising a map script in maps/, choosing textures/themes, placing lights, or critiquing renders. Covers workflow, scale, theme palettes, and lighting grammar derived from the 38 original id .map sources.
---

# Quake Mapping

Author maps as Python scripts (`maps/<name>.py`) using `maps/qgeo.py` primitives.
Build and iterate with:

    ./src/nexmap/nexmap build maps/<name>.py [-cams "x y z yaw pitch;..."] [-deploy]

The pipeline fails loudly on invalid brushes, leaks, unsealed maps, and zero
lightdata. Never pass `-bounce` to light (vendored build silently produces a
fullbright map).

## The loop

1. Write/edit the map script. 2. Build. 3. **Read the rendered PNGs and
critique them** — composition, texture coherence, light pools, silhouettes.
4. Iterate. Never ship a map you haven't looked at from multiple cameras.

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
