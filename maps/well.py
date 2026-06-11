#!/usr/bin/env python3
"""The Drowning Well — azure city courtyard DM map. Emits out/well.map."""
import os
from qgeo import MapWriter, box, cylinder, arch, stairs

# Azure city palette per the corpus: dm6's primary pair is cop1_1 + city2_6;
# city5_4 is dm5's floor tile. wmet4_8 is dm6's own trim metal.
WALL = "city2_6"
PIER = "city4_2"
TRIM = "cop1_1"
RAIL = "wmet4_8"
FLOOR = "city5_4"
GROUND = "city2_7"
BASIN = "afloor3_1"
WATER = "*04water2"
SKY = "sky4"
WINDOW = "window02_1"

m = MapWriter("The Drowning Well", worldtype=0, minlight=8)
m.props["_sunlight"] = "180"
m.props["_sun_mangle"] = "135 -70 0"
m.props["_sunlight_penumbra"] = "6"
m.props["_sunlight2"] = "55"

# ---- shell: outer walls, ground, sky lid (interior +-320, z 0..480) ----
m.add(box(-352, -352, -16, -320, 352, 496, WALL))
m.add(box(320, -352, -16, 352, 352, 496, WALL))
m.add(box(-352, -352, -16, 352, -320, 496, WALL))
m.add(box(-352, 320, -16, 352, 352, 496, WALL))
m.add(box(-352, -352, 480, 352, 352, 496, SKY))

# ---- ground floor ring around the basin ----
m.add(box(-352, 144, -16, 352, 352, 0, TRIM, zp=GROUND))
m.add(box(-352, -352, -16, 352, -144, 0, TRIM, zp=GROUND))
m.add(box(144, -144, -16, 352, 144, 0, TRIM, zp=GROUND))
m.add(box(-352, -144, -16, -144, 144, 0, TRIM, zp=GROUND))

# ---- water basin (pool -128..128, floor -48, surface -8) ----
m.add(box(-144, -144, -64, 144, 144, -48, BASIN))
m.add(box(-144, -144, -48, -128, 144, 0, TRIM))
m.add(box(128, -144, -48, 144, 144, 0, TRIM))
m.add(box(-128, -144, -48, 128, -128, 0, TRIM))
m.add(box(-128, 128, -48, 128, 144, 0, TRIM))
m.add(box(-128, -128, -48, 128, 128, -8, WATER))
# megahealth pedestal rising flush with the floor
m.add(cylinder(0, 0, 28, -48, 0, RAIL, sides=8, cap_hi=BASIN))

# ---- bottom arcade at the shaft edge (+-192..224 band, piers + arches) ----
def piers(z1, z2):
    for cx in (-192, 192):
        for cy in (-192, 192):
            m.add(box(cx - 32, cy - 32, z1, cx + 32, cy + 32, z2, PIER))
    for c in (-208, 208):
        m.add(box(-16, c - 16, z1, 16, c + 16, z2, PIER))
        m.add(box(c - 16, -16, z1, c + 16, 16, z2, PIER))

piers(0, 160)
for s in (-1, 1):
    d1, d2 = (192, 224) if s > 0 else (-224, -192)
    for a1, a2 in ((-176, -16), (16, 176)):
        m.add(arch("x", a1, a2, d1, d2, 104, 40, 160, WALL, segments=4))
        m.add(arch("y", a1, a2, d1, d2, 104, 40, 160, WALL, segments=4))

# ---- mid gallery floor ring (z 160..176, inner edge at +-192) ----
m.add(box(-320, 192, 160, 320, 320, 176, TRIM, zp=FLOOR, zn=WALL))
m.add(box(-320, -320, 160, 192, -192, 176, TRIM, zp=FLOOR, zn=WALL))
m.add(box(-320, -192, 160, -192, 192, 176, TRIM, zp=FLOOR, zn=WALL))
m.add(box(192, -56, 160, 320, 320, 176, TRIM, zp=FLOOR, zn=WALL))
m.add(box(192, -320, 160, 256, -56, 176, TRIM, zp=FLOOR, zn=WALL))  # catwalk by stairwell

# ---- mid arcade: piers, jump-over parapets, arch headers ----
piers(176, 320)
for s in (-1, 1):
    d1, d2 = (192, 224) if s > 0 else (-224, -192)
    r1, r2 = (192, 208) if s > 0 else (-208, -192)
    for a1, a2 in ((-176, -16), (16, 176)):
        m.add(arch("x", a1, a2, d1, d2, 256, 48, 320, WALL, segments=4))
        m.add(arch("y", a1, a2, d1, d2, 256, 48, 320, WALL, segments=4))
        m.add(box(a1, r1, 176, a2, r2, 216, RAIL))
        m.add(box(r1, a1, 176, r2, a2, 216, RAIL))

# ---- top rampart floor ring (z 320..336) ----
m.add(box(-256, 192, 320, 320, 320, 336, TRIM, zp=FLOOR, zn=WALL))
m.add(box(-320, -320, 320, 320, -192, 336, TRIM, zp=FLOOR, zn=WALL))
m.add(box(192, -192, 320, 320, 192, 336, TRIM, zp=FLOOR, zn=WALL))
m.add(box(-320, -192, 320, -192, 80, 336, TRIM, zp=FLOOR, zn=WALL))
m.add(box(-256, 80, 320, -192, 320, 336, TRIM, zp=FLOOR, zn=WALL))  # catwalk by stairwell

# ---- crenellated inner parapet at the well mouth ----
for s in (-1, 1):
    r1, r2 = (192, 208) if s > 0 else (-208, -192)
    m.add(box(-208, r1, 336, 208, r2, 376, RAIL))
    m.add(box(r1, -208, 336, r2, 208, 376, RAIL))
    for c in (-192, -144, -48, 48, 144, 192):
        w = 16 if abs(c) == 192 else 24
        m.add(box(c - w, r1, 376, c + w, r2, 416, WALL))
        m.add(box(r1, c - w, 376, r2, c + w, 416, WALL))

# ---- stairs: ground -> gallery (east wall), gallery -> top (west wall) ----
m.add(stairs(256, -320, 320, -56, 0, 176, "y", "+", WALL, top_tex=FLOOR))
m.add(stairs(-320, 80, -256, 320, 176, 336, "y", "+", WALL, top_tex=FLOOR))

# ---- teleporter: ground NE corner -> top SW rampart ----
m.add(box(216, 296, 0, 232, 320, 168, PIER))
m.add(box(296, 296, 0, 312, 320, 168, PIER))
m.add(arch("x", 232, 296, 296, 320, 168, 32, 208, PIER, segments=4))
m.add(box(232, 308, 8, 296, 320, 160, "*teleport"))
m.add(box(216, 240, 0, 312, 320, 8, TRIM, zp="tele_top"))
m.ent("trigger_teleport", brush=box(232, 296, 8, 296, 316, 152, "black"),
      target="welltop")
m.ent("info_teleport_destination", origin="-264 -264 360", angle="45",
      targetname="welltop")
m.light(264, 280, 176, 300)
m.light(264, 232, 64, 200, wait=0.9)
m.ent("light_torch_small_walltorch", origin="200 312 112")
m.ent("light_torch_small_walltorch", origin="304 264 112")

# ---- windows on the top-tier outer walls, one per side ----
m.add(box(-32, 312, 384, 32, 320, 448, TRIM, yn=WINDOW))
m.add(box(-32, -320, 384, 32, -312, 448, TRIM, yp=WINDOW))
m.add(box(312, -32, 384, 320, 32, 448, TRIM, xn=WINDOW))
m.add(box(-320, -32, 384, -312, 32, 448, TRIM, xp=WINDOW))
for x, y in ((0, 288), (0, -288), (288, 0), (-288, 0)):
    m.light(x, y, 416, 140, wait=1.1)

# ---- torches: ground ring ----
for x, y in ((-160, 312), (160, 312), (-160, -312), (160, -312),
             (-312, 160), (-312, -160)):
    m.ent("light_torch_small_walltorch", origin=f"{x} {y} 96")
m.ent("light_torch_small_walltorch", origin="312 -240 120")  # over the stairs
m.ent("light_torch_small_walltorch", origin="312 -104 200")
# shaft piers, facing the water
for x, y in ((0, -180), (0, 180), (-180, 0), (180, 0)):
    m.ent("light_torch_small_walltorch", origin=f"{x} {y} 112")

# ---- torches: gallery ring ----
for x, y in ((-160, 312), (160, 312), (-160, -312), (160, -312),
             (-312, -160), (312, 160), (312, -160), (-312, 160)):
    m.ent("light_torch_small_walltorch", origin=f"{x} {y} 256")

# ---- top tier: corner pedestal flames ----
for sx in (-1, 1):
    for sy in (-1, 1):
        px, py = 280 * sx, 280 * sy
        m.add(box(px - 16, py - 16, 336, px + 16, py + 16, 376, PIER, zp=RAIL))
        m.ent("light_flame_large_yellow", origin=f"{px} {py} 392")

# ---- fill lights: pool glow, gallery soffits, stair tops ----
for x, y in ((-96, -96), (-96, 96), (96, -96), (96, 96)):
    m.light(x, y, 48, 170, wait=0.9)
for x, y in ((0, -240), (0, 240), (-240, 0), (240, 0)):
    m.light(x, y, 280, 140, wait=1.2)
m.light(288, -88, 200, 175)
m.light(-288, 288, 360, 175)
m.ent("light_torch_small_walltorch", origin="-312 160 280")  # west stairwell
m.light(288, -288, 80, 150)   # east stair base
m.light(-288, 112, 260, 140)  # west stair base

# ---- items ----
m.ent("weapon_rocketlauncher", origin="0 256 200")
m.ent("weapon_grenadelauncher", origin="0 -256 24")
m.ent("weapon_supernailgun", origin="256 0 360")
m.ent("item_armor2", origin="-256 0 200")
m.ent("item_health", origin="0 0 24", spawnflags=2)        # megahealth
m.ent("item_rockets", origin="64 -256 24")
m.ent("item_rockets", origin="128 -256 24")
m.ent("item_spikes", origin="256 64 360")
m.ent("item_spikes", origin="256 -64 360")
m.ent("item_shells", origin="-256 -96 200")
m.ent("item_shells", origin="-256 -160 200")
m.ent("item_health", origin="288 -96 24")
m.ent("item_health", origin="96 288 24")
m.ent("item_health", origin="224 -224 200", spawnflags=1)  # rotten, catwalk
m.ent("item_health", origin="-224 224 360", spawnflags=1)

# ---- spawns ----
m.ent("info_player_start", origin="-256 256 24", angle="315")
m.ent("info_player_deathmatch", origin="96 -256 24", angle="90")
m.ent("info_player_deathmatch", origin="-256 96 200", angle="270")
m.ent("info_player_deathmatch", origin="256 256 200", angle="180")
m.ent("info_player_deathmatch", origin="0 -256 360", angle="90")

# ---- ambience ----
m.ent("ambient_drip", origin="80 80 16")
m.ent("ambient_drip", origin="-80 -80 16")

m.write(os.path.join(os.path.dirname(__file__), "..", "out", "well.map"))
