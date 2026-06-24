#!/usr/bin/env python3
"""The Drowning Well v2 — azure-city shaft arena, rebuilt for FLOW.

The v1 layout scored dead-end ratio 0.77 / loop density 0.24 (qflow) — a tree
of spurs. This version is two continuous ring walkways (ground + gallery)
around a central water shaft, cross-linked by two bridges and four GENEROUS
radial stairs, plus a teleporter to a top perch. Continuous rings + bridges +
four vertical links = many circuits to run, like the id arenas. Emits
out/well.map.
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import qprefab as P
from qgeo import MapWriter, box, cylinder

TH = "city"
WALL = "city6_8"
FLOOR = "city4_7"
LEDGE = "city2_6"
TRIM = "cop2_3"
PIER = "city2_6"
PITW = "city5_6"
PITF = "afloor3_1"
WATER = "*04water2"
SKY = "sky4"
WARM = "255 186 120"
COOL = "170 198 255"
WATERC = "120 200 220"
TELEC = "150 175 255"

OUT = 672          # arena half-extent (interior wall at +-OUT)
SHAFT = 176        # central shaft half-width
GAL = 160          # gallery height
GIN = OUT - 128    # gallery ledge inner edge (544): stairs land here
TOP = 360          # open-air height
WB = -160          # water basin floor

m = MapWriter("The Drowning Well", worldtype=0, minlight=12)
m.props.update({"_sunlight": "210", "_sun_mangle": "30 -55 0",
                "_sunlight_penumbra": "10", "_sunlight2": "90"})


def ring(z0, z1, inner, outer, tex, **kw):
    """Square annulus (a continuous walkable frame) inner..outer, z0..z1."""
    return [
        box(-outer, -outer, z0, outer, -inner, z1, tex, **kw),
        box(-outer, inner, z0, outer, outer, z1, tex, **kw),
        box(-outer, -inner, z0, -inner, inner, z1, tex, **kw),
        box(inner, -inner, z0, outer, inner, z1, tex, **kw),
    ]


# ---- shell: outer walls (z -176..TOP) + sky lid -----------------------------
for w in ring(-176, TOP, OUT, OUT + 16, WALL):
    m.add(w)
m.add(box(-OUT - 16, -OUT - 16, TOP, OUT + 16, OUT + 16, TOP + 16, SKY))

# ---- water basin at the bottom of the shaft ---------------------------------
m.add(box(-SHAFT - 16, -SHAFT - 16, WB - 16, SHAFT + 16, SHAFT + 16, WB, PITF))
for w in ring(WB, 0, SHAFT, SHAFT + 16, PITW):
    m.add(w)
m.add(box(-SHAFT, -SHAFT, WB, SHAFT, SHAFT, -64, WATER))

# ---- tier 0: ground ring (z0) around the shaft ------------------------------
for w in ring(-16, 0, SHAFT, OUT, FLOOR, zp=FLOOR, zn=WALL):
    m.add(w)

# two bridges crossing the shaft at z0 (a + over the water) -> cross-links the
# ground ring on all four sides and carries the contested megahealth at center
m.add(box(-40, -SHAFT, -16, 40, SHAFT, 0, TRIM, zp=FLOOR))
m.add(box(-SHAFT, -40, -16, SHAFT, 40, 0, TRIM, zp=FLOOR))
m.place(P.item_pedestal(0, 0, 0, "item_health", TH, height=8, spawnflags=2))
for sx in (-1, 1):                       # shaft-corner support pillars
    for sy in (-1, 1):
        m.add(cylinder(sx * 132, sy * 132, 20, WB, 0, PIER, sides=8))

# ---- tier 1: gallery ring ledge (z160) along the walls ----------------------
for w in ring(GAL - 16, GAL, GIN, OUT, LEDGE, zp=FLOOR, zn=WALL):
    m.add(w)
for w in ring(GAL, GAL + 28, GIN, GIN + 10, TRIM):   # inner parapet curb
    m.add(w)

# two gallery-level catwalks across the shaft (a high + over the water) — they
# close the gallery ring across the middle, adding circuits and vertigo
m.add(P.walkway(-44, -GIN, 44, GIN, GAL, TH, railing=True).brushes)
m.add(P.walkway(-GIN, -44, GIN, 44, GAL, TH, railing=True).brushes)

# ---- four generous radial stairs, ground ring -> gallery ledge --------------
m.place(P.stairs_landed(SHAFT, -104, GIN, 104, 0, GAL, "x", "+", TH))   # east
m.place(P.stairs_landed(-GIN, -104, -SHAFT, 104, 0, GAL, "x", "-", TH))  # west
m.place(P.stairs_landed(-104, SHAFT, 104, GIN, 0, GAL, "y", "+", TH))   # north
m.place(P.stairs_landed(-104, -GIN, 104, -SHAFT, 0, GAL, "y", "-", TH))  # south

# ---- top perch (teleport destination) + teleporter on the ground ring -------
PERCH = 300
m.add(box(OUT - 160, OUT - 160, PERCH - 16, OUT, OUT, PERCH, LEDGE, zp=FLOOR))
for w in (box(OUT - 160, OUT - 160, PERCH, OUT - 150, OUT, PERCH + 28, TRIM),
          box(OUT - 160, OUT - 160, PERCH, OUT, OUT - 150, PERCH + 28, TRIM)):
    m.add(w)
m.place(P.teleporter_pad(-(OUT - 90), -(OUT - 90), 0, TH, target="well_top",
                         dest_name="well_top",
                         dest_origin=(OUT - 80, OUT - 80, PERCH + 24)))

# ---- corner pillars tying the tiers together (visual verticals) -------------
for sx in (-1, 1):
    for sy in (-1, 1):
        m.add(cylinder(sx * (OUT - 40), sy * (OUT - 40), 22, 0, TOP, PIER,
                       sides=8))

# ---- lighting: torch grammar per tier + sky/water glow ----------------------
def torch(x, y, z):
    m.ent("light_torch_small_walltorch", origin=f"{x:g} {y:g} {z:g}")


for s in (-480, -160, 160, 480):
    torch(s, -OUT + 20, 92)
    torch(s, OUT - 20, 92)
    torch(-OUT + 20, s, 92)
    torch(OUT - 20, s, 92)
    torch(s, -OUT + 20, 248)
    torch(OUT - 20, s, 248)
    m.light(s, -OUT + 40, 60, 165, _color=WARM)   # warm floor pools
    m.light(s, OUT - 40, 60, 165, _color=WARM)
    m.light(-OUT + 40, s, 60, 165, _color=WARM)
    m.light(OUT - 40, s, 60, 165, _color=WARM)
for sx in (-1, 1):                         # corner brazier flames up high
    for sy in (-1, 1):
        m.ent("light_flame_large_yellow",
              origin=f"{sx*(OUT-40):g} {sy*(OUT-40):g} {TOP-40:g}")
# pooled sky fill (cool), water glow (teal), warm gallery wash
for gx in (-440, 0, 440):
    for gy in (-440, 0, 440):
        m.light(gx, gy, 300, 160, wait="1.15", _color=COOL)
for gx in (-96, 96):
    for gy in (-96, 96):
        m.light(gx, gy, WB + 40, 210, wait="0.7", _color=WATERC)
for gx in (-420, 420):
    for gy in (-420, 420):
        m.light(gx, gy, GAL + 60, 175, _color=WARM)
m.light(0, 0, 120, 230, _color=WARM)       # key light on the megahealth
m.light(-(OUT - 90), -(OUT - 90), 60, 200, _color=TELEC)
m.light(OUT - 80, OUT - 80, PERCH + 50, 200, _color=TELEC)

# ---- items on CARDINALS, spawns on DIAGONALS (never coincide) ---------------
GR = OUT - 72       # ground item ring radius (600)
LR = OUT - 64       # gallery ledge item radius (608)
# ground weapons + items, cardinal directions (clear of diagonal spawns)
m.ent("weapon_rocketlauncher", origin=f"{GR:g} 0 24")
m.ent("weapon_supernailgun", origin=f"-{GR:g} 0 24")
m.ent("item_armorInv", origin=f"0 {GR:g} 24")
m.ent("item_rockets", origin=f"0 -{GR:g} 24")
m.ent("item_health", origin=f"{GR:g} {GR:g} 24", spawnflags=1)     # corners
m.ent("item_health", origin=f"{GR:g} -{GR:g} 24", spawnflags=1)
# gallery weapons + items, cardinal directions on the ledge
m.ent("weapon_grenadelauncher", origin=f"0 {LR:g} {GAL+24:g}")
m.ent("weapon_lightning", origin=f"0 -{LR:g} {GAL+24:g}")
m.ent("item_shells", origin=f"{LR:g} 0 {GAL+24:g}")
m.ent("item_cells", origin=f"-{LR:g} 0 {GAL+24:g}")
m.ent("item_health", origin=f"{GR:g} {GR:g} {GAL+24:g}")           # NE corner
# shaft + perch prizes
m.ent("item_armor2", origin=f"{OUT-80:g} {OUT-80:g} {PERCH+24:g}")

# ground spawns on diagonals (clear of the cardinal stair strips)
for sx, sy, ang in ((1, 1, 225), (-1, -1, 45), (1, -1, 135), (-1, 1, 315)):
    m.ent("info_player_deathmatch" if (sx, sy) != (-1, -1)
          else "info_player_start",
          origin=f"{sx*440:g} {sy*440:g} 24", angle=str(ang))
# gallery spawns on diagonal ledge corners (inset to clear the corner pillars)
for sx, sy, ang in ((-1, -1, 45), (1, -1, 135)):
    m.ent("info_player_deathmatch",
          origin=f"{sx*560:g} {sy*560:g} {GAL+24:g}", angle=str(ang))

# a few more fill lights for corpus light pacing
for gx in (-560, 560):
    for gy in (-560, 560):
        m.light(gx, gy, 40, 140, _color=WARM)

# ---- ambience ---------------------------------------------------------------
m.ent("ambient_drip", origin="120 -120 -40")
m.ent("ambient_drip", origin="-120 120 -40")
m.ent("ambient_swamp1", origin="0 0 -40")

m.write(os.path.join(os.path.dirname(__file__), "..", "out", "well.map"))
