# Lighting grammar

Measured from all light entities in the 38 id originals
(`python3 tools/qcorpus.py lights [--map m] [--near]`).

## The core lesson: many moderate lights

- Density: ~190-235 light entities per map (every theme).
- Values: p25=150, median=200, p75=200, p90=250-300. Hot maximums (500-850)
  are rare single accents, not the norm.
- id built mood from MANY overlapping 150-250 lights creating pools and
  shadow gaps — not from a few 400+ floods. A flat, evenly-bright map and a
  cave-dark map are both wrong; aim for pooled light with readable dark zones.

## Per-theme fixture style

| Theme  | Source of light                                        |
|--------|--------------------------------------------------------|
| metal  | plain `light` near embedded light panels (`light3_8`, `light1_4`) + lava glow |
| tech   | `light_fluoro` + ceiling `tlight*` panels               |
| city   | torches/flames; open sky; almost no light textures      |
| wizard | torches everywhere; windows; no light textures          |

**Anchor every light entity to visible geometry**: a light panel texture, a
torch fixture, lava, a window. Unanchored lights read as arbitrary — this was
explicit playtest feedback on our maps.

## Entity reference

- `light` — generic point light; key `light` (value, default 200), `wait`
  (falloff scale: >1 falls off faster, <1 reaches farther), `_color "R G B"`
  (0-255, ericw-tools extension), `style` (1=flicker, 10=fluoro flicker...)
- `light_torch_small_walltorch` — wall torch model + flame, self-animating
- `light_flame_small_yellow` / `large_yellow` / `small_white` — standing flames
- `light_fluoro` — humming fluorescent (tech theme)

## Local gotchas

- NEVER `light -bounce` with the vendored tools — silently writes zero
  lightdata (fullbright). The build pipeline hard-fails on this.
- Compile uses `light -surflight_subdivide 64`.
- Dark texture families (wizmet etc.) read ~2x darker than brown metal at the
  same light value. Compensate with MORE lights (id's way), not hotter ones.
- Colored light: subtle tints only (e.g., slime glow `_color "140 255 90"`
  at low value with tight `wait`); id-era maps were effectively white-lit.
