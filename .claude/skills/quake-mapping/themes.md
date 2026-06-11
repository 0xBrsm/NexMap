# The four id themes

Face-count data from the original .map sources. Texture names are lowercase
(our WAD normalizes); counts are corpus-wide within the theme's maps.
Verify any texture exists before use: `python3 tools/qcorpus.py role <tex>`.

## Runic metal — dm2, dm4, e1m6-e1m8, e3m2-e3m7

The Quake signature look: rusted iron, runes, lava.

- Primary: `metal4_4` (dominant by far), `metal1_4`, `met5_1`, `metal1_3`
- Secondary: `metal2_2`, `metal5_4`, `mmetal1_3`, `nmetal2_1`, `metal5_1`
- Trim/accent: `cop*` (copper), `rune*` panels, demon faces (`m5_8`, `metal5_8`-style)
- Light panels: `light3_8`, `light1_4`, `light3_7`, `light3_6` (embedded in walls/ceilings)
- Liquid: `*lava1`; teleporters `*teleport`
- Fixtures: plain `light` entities dominate; occasional walltorch

## Base / tech — e1m1, e2m1, e3m1, e4m1, dm3

Military installation: computers, slipgates, slime.

- Primary: `tech04_3`, `tech04_1`, `tech04_7`, `tech05_2`, `tech06_1`
- Floors: `sfloor4_2` (named floor family — actually used as floors)
- Secondary: `ecop1_4`, `uwall1_2`, `twall*`, `comp*` (computer screens)
- Light panels: `tlight08`, `tlight01`, `tlight11`, `tlight02` (ceiling-recessed)
- Liquid: `*slime0`, `*water1`
- Fixtures: plain `light` + `light_fluoro` (hum) — NO torches in base maps

## Azure city / medieval — dm5, dm6, e1m5, e2m3, e2m7, e4m3

Stone city: blue-grey masonry, torchlight, open sky.

- Primary: `city6_8`, `city2_6`, `city5_4`, `city2_7`, `city4_7`
- Secondary: `cop1_1`, `cop2_3` (copper banding), `wwall1_1`, `wall11_6`
- Light panels: almost none — light comes from torches and sky
- Liquid: `*04water2`, `*04awater1`
- Fixtures: torches everywhere — `light_torch_small_walltorch`,
  `light_flame_small_yellow`, `light_flame_large_yellow`

## Wizard — e1m2-e1m4, e2m2, e2m6

Elder-world swamp keep: green-brown brick, wood, water. (This is the
Sludgeworks family — note it is NOT the DM2 look; DM2 is runic metal.)

- Primary: `wbrick1_5` (dominant), `wizmet1_2`, `wswamp2_1`, `wiz1_4`
- Secondary: `wswamp1_4`, `wswamp2_2`, `rock4_2`, `rock1_2`, `wizwood1_5`,
  `wmet*` trim
- Light panels: none — torch-lit theme
- Liquid: `*04water1`; slime rare (`*slime`)
- Fixtures: heaviest torch usage of all themes — `light_torch_small_walltorch`
  (125 across 5 maps), `light_flame_small_yellow`, `light_flame_large_yellow`

## Choosing and committing

- One theme per map. Borrowing a single accent family (e.g., copper trim in
  metal maps) is authentic; mixing primaries is not.
- Theme implies liquid, fixture type, AND light-panel usage — see lighting.md.
- For palette inspection, render a contact sheet of candidate textures before
  committing (textures live in the 574-entry WAD; `out/mapgen_textures.wad`).
