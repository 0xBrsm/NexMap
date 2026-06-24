#!/usr/bin/env python3
"""The Drowning Well — azure-city flooded courtyard DM map, rebuilt on the
qlayout/qprefab stack. Emits out/well.map.

Spec:
  theme   city (azure stone, torch-lit)
  rooms   hub (tall sky-open courtyard, flooded floor + central wellhead),
          armory (east, weapons), reliquary (north, armor + secret tease)
  graph   hub --arch--> armory ; hub --arch--> reliquary ; teleporter from
          the hub floor up to the perimeter gallery
  verticality  floor (z0) -> two landed stairs -> perimeter gallery (z224)
  items   seated on pedestals on both tiers; nothing floats
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import qgeo
import qprefab as P
from qlayout import Layout

THEME = "city"
WATER = "*04water2"
GAL = 224                       # perimeter gallery height
HX = 416                        # hub half-extent
HZ = 448                        # hub height (open to sky)

L = Layout("The Drowning Well", THEME, worldtype=0, minlight=10)
L.props.update({"_sunlight": "160", "_sun_mangle": "20 -65 0",
                "_sunlight_penumbra": "8", "_sunlight2": "60"})

hub = L.room("hub", -HX, -HX, HX, HX, 0, HZ, chamfer=72, sky=True)
armory = L.room("armory", 448, -192, 832, 192, 0, 256)
reliquary = L.room("reliquary", -192, 448, 192, 832, 0, 288)

L.connect("arch", hub, armory, width=128, height=144)
L.connect("arch", hub, reliquary, width=128, height=144)

# ---- hub: flooded floor + central wellhead ----------------------------------
TRIM = "cop2_3"
PIER = "city2_6"
# shallow flood across the whole floor, ringed lip so it reads as water
hub.add(qgeo.box(-HX, -HX, 0, HX, HX, 16, WATER))
for lip in ((-HX, -HX, HX, -HX + 16), (-HX, HX - 16, HX, HX),
            (-HX, -HX, -HX + 16, HX), (HX - 16, -HX, HX, HX)):
    hub.add(qgeo.box(lip[0], lip[1], 0, lip[2], lip[3], 28, TRIM))
# central octagonal wellhead rising from the water, megahealth on top
hub.add(qgeo.cylinder(0, 0, 96, 0, 48, PIER, sides=8, cap_hi=TRIM))
hub.place(P.item_pedestal(0, 0, 48, "item_health", THEME, height=8,
                          spawnflags=2))      # megahealth, seated & reachable
hub.add(qgeo.cylinder(0, 0, 64, 48, 56, WATER, sides=8))  # well water surface

# corner shrine pillars
for sx in (-1, 1):
    for sy in (-1, 1):
        hub.place(P.pillar(sx * 300, sy * 300, 0, HZ, THEME, round_=True))

# ---- perimeter gallery at z=224 (walkway ring) + landed stairs --------------
for x1, y1, x2, y2 in ((-HX, 320, HX, HX), (-HX, -HX, HX, -320),
                       (320, -320, HX, 320), (-HX, -320, -320, 320)):
    hub.add(P.walkway(x1, y1, x2, y2, GAL, THEME, railing=True).brushes)

hub.place(P.stairs_landed(-HX, 96, -160, 360, 0, GAL, "x", "-", THEME))
hub.place(P.stairs_landed(160, -360, HX, -96, 0, GAL, "x", "+", THEME))

# teleporter: hub floor SW -> NE gallery
hub.place(P.teleporter_pad(-320, -320, 28, THEME, target="well_gallery",
                           dest_name="well_gallery",
                           dest_origin=(340, 340, GAL + 24)))

# ---- lighting: torch grammar + sun fill (city reads brighter than metal) ----
for (x, y), face in ((( -HX + 12, 0), (1, 0)), ((HX - 12, 0), (-1, 0)),
                     ((0, -HX + 12), (0, 1)), ((0, HX - 12), (0, -1))):
    for z in (130, 320):
        hub.fixture(x, y, z, wall_normal=face)
for sx in (-1, 1):
    for sy in (-1, 1):
        hub.ent("light_flame_large_yellow",
                origin=f"{sx*300:g} {sy*300:g} 360")
for gx in (-240, 0, 240):
    for gy in (-240, 0, 240):
        hub.light(gx, gy, 300, 150, wait="1.1")        # soft pooled sky fill
for gx in (-300, 300):                                  # gallery wash
    for gy in (-300, 300):
        hub.light(gx, gy, GAL + 80, 170)

# ---- armory (east): weapons on pedestals, lit alcoves -----------------------
for fx in (560, 720):
    armory.fixture(fx, -180, 150, wall_normal=(0, 1))
    armory.fixture(fx, 180, 150, wall_normal=(0, -1))
armory.light(640, 0, 200, 180)
armory.item("weapon_rocketlauncher", 560, 0)
armory.item("weapon_supernailgun", 760, -120)
armory.item("item_armorInv", 760, 120)
armory.place(P.wainscot(470, -180, 810, 180, 0, THEME, height=64))
armory.spawn(640, 0, angle=180)

# ---- reliquary (north): armor, a teased upper niche --------------------------
for fy in (560, 720):
    reliquary.fixture(-180, fy, 180, wall_normal=(1, 0))
    reliquary.fixture(180, fy, 180, wall_normal=(-1, 0))
reliquary.light(0, 640, 220, 180)
reliquary.item("item_armor2", 0, 560)
# secret tease: a raised niche with a quad, reachable only by the side ledges
reliquary.add(qgeo.box(-64, 760, 0, 64, 832, 96, PIER, zp="city4_7"))
reliquary.place(P.item_pedestal(0, 796, 96, "item_artifact_super_damage",
                                THEME, height=8))
reliquary.place(P.walkway(-192, 700, -120, 832, 96, THEME, railing=False))
reliquary.place(P.walkway(120, 700, 192, 832, 96, THEME, railing=False))
reliquary.spawn(0, 520, angle=90)

# ---- hub spawns (floor + gallery) -------------------------------------------
hub.spawn(-340, 300, angle=315, dm=False)        # info_player_start
hub.spawn(300, -340, angle=135)
hub.ent("info_player_deathmatch", origin=f"-360 360 {GAL+24:g}", angle="315")
hub.ent("info_player_deathmatch", origin=f"360 -360 {GAL+24:g}", angle="135")

# gallery items (seated on the gallery deck, opposite the gallery spawns)
hub.place(P.item_pedestal(360, 360, GAL, "item_rockets", THEME, height=8))
hub.place(P.item_pedestal(-360, -360, GAL, "item_cells", THEME, height=8))

hub.ent("ambient_drip", origin="120 -120 20")
hub.ent("ambient_drip", origin="-120 120 20")

out = os.path.join(os.path.dirname(__file__), "..", "out", "well.map")
L.write(out)
print("wrote", os.path.abspath(out))

