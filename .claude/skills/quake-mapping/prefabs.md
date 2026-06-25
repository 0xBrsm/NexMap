# Prefab catalog (`maps/qprefab.py`)

Each builder returns a `Prefab` (world brushes + the entities that belong with
them). Drop one into a map with `MapWriter.place(prefab)` or `Room.place()`.
Textures come from the room's theme; dimensions from `qtheme.METRICS`. Every
prefab exists to fix one recurring failure mode once — reach for these instead
of freehanding the geometry.

| prefab | signature (core args) | fixes |
|--------|----------------------|-------|
| `stairs_landed` | `(x1,y1,x2,y2,z_base,z_top,axis,direction,theme)` | missing landings: always a bottom landing, splits flights at `max_stair_rise` |
| `item_pedestal` | `(x,y,z_floor,classname,theme,height=16,spawnflags=None)` | floating/embedded items: seats the item at floor+`item_z`, adds a soft light |
| `light_fixture` | `(x,y,z,theme,value=None,wall_normal=None)` | flat lighting: pairs a *visible* source (torch/panel/tube) with its light entity |
| `pillar` | `(cx,cy,z_base,z_top,theme,w=None,round_=False)` | bare-stick columns: adds base + cap; `round_` = octagonal shaft |
| `archway` | `(axis,a1,a2,depth1,depth2,z_base,z_top,theme)` | boxy doorways: curved soffit header + jamb columns |
| `chamfered_corner` | `(cx,cy,z1,z2,theme,size=32,quadrant)` | harsh 90-degree corners: 45-degree fillet |
| `curved_wall` | `(cx,cy,r,z1,z2,theme,a0,a1,thick=16,segs=6)` | hard wall joins: swept arc |
| `ceiling_vault` | `(x1,y1,x2,y2,z_spring,theme,rise=None,axis)` | box-with-a-lid: barrel-vaulted ceiling |
| `wainscot` | `(x1,y1,x2,y2,z_base,theme,height=None)` | flat walls: a proud trim band around the base |
| `window_embrasure` | `(axis,a1,a2,wall_lo,wall_hi,z_sill,z_head,theme,liquid_glow=False)` | dead walls: recessed window, optional backlight |
| `walkway` | `(x1,y1,x2,y2,z,theme,railing=True)` | flat play: elevated catwalk with curb rails |
| `teleporter_pad` | `(x,y,z_floor,theme,target,dest_name,dest_origin)` | bare triggers: marked `*teleport` pad + linked destination |
| `sightline_baffle` | `(x1,y1,x2,y2,z_base,theme,height=96)` | over-exposed space: free-standing partial wall that breaks LOS + bends the route |

## Selecting by feature target

Parts are tagged with the gate features they advance (`qprefab.CATALOG`). When a
target is short, query the catalog instead of guessing:
`qprefab.catalog("occlusion")`, `catalog("verticality")`, `catalog("chokepoint")`,
`catalog("sightline_variety")`. The catalog is a vocabulary to compose freely —
parametric parts you place, not a fixed kit — and `qgeo` is always there for
shapes it lacks. Outcomes (the five gate features) are the only hard constraint;
the forms are yours.

## Notes

- `light_fixture` adapts to the theme: torch themes emit the wall-torch (it
  carries its own flame light), `fluoro` themes emit a `light_fluoro` + tube,
  panel themes emit a recessed emissive panel + a point light in front of it.
  Pass `wall_normal=(nx,ny)` so the recess faces into the room.
- `stairs_landed` plans flights from `METRICS["max_stair_rise"]`; if the given
  footprint is too short for steps + landings it compresses the landings rather
  than overrun the bounds.
- Verify any new or changed prefab with `maps/_prefab_gallery.py` — it builds
  one of every prefab in a lit hall for a render check.
- Winding: prism-based prefabs (chamfer, curved_wall, vault) rely on
  `qgeo.prism` auto-correcting clockwise polygons to CCW; if you write a raw
  prism, you no longer need to hand-wind it CCW.
