#!/usr/bin/env python3
"""The Sludgeworks — a DM2-style arena, hand-designed. Emits out/sludge.map."""
import os
from qgeo import MapWriter, box, prism, wedge, cylinder, arch, stairs

WALL = "wizmet1_2"
WALL2 = "wizmet1_6"
FLOOR = "wizmet1_1"
CEIL = "wizmet1_8"
TRIM = "wmet1_1"
BEAM = "wizmet1_4"
PILLAR = "wizmet1_3"
PITWALL = "wizmet1_7"
PITFLOOR = "wizmet1_1"
RIB = "wmet3_1"
SLIME = "*slime0"

m = MapWriter("The Sludgeworks", minlight=8)

# ---- arena shell (interior x -512..512, y -384..384, z 0..320) ----
m.add(box(-528, -400, 320, 528, 400, 336, CEIL))
m.add(box(-528, 384, -16, 528, 400, 320, WALL))            # north wall
m.add(box(-528, -400, -16, 528, -384, 320, WALL))          # south wall
m.add(box(512, -400, -16, 528, 400, 320, WALL))            # east wall
m.add(box(-528, -400, -16, -512, 64, 320, WALL))           # west wall, south of door
m.add(box(-528, 192, -16, -512, 400, 320, WALL))           # west wall, north of door
# arched doorway header (curve springs at z 80, apex 128)
m.add(arch("y", 64, 192, -528, -512, 80, 48, 320, WALL, segments=6))

# ---- walkway floors around the pit ----
m.add(box(-512, -384, -16, -384, 384, 0, FLOOR, zp=FLOOR, xp=TRIM, zn=CEIL))
m.add(box(384, -384, -16, 512, 384, 0, FLOOR, zp=FLOOR, xn=TRIM, zn=CEIL))
m.add(box(-384, -384, -16, 384, -256, 0, FLOOR, zp=FLOOR, yp=TRIM, zn=CEIL))
m.add(box(-384, 256, -16, 384, 384, 0, FLOOR, zp=FLOOR, yn=TRIM, zn=CEIL))

# ---- slime pit (interior x -384..384, y -256..256, floor -160) ----
m.add(box(-400, -272, -176, 400, 272, -160, PITWALL, zp=PITFLOOR))
m.add(box(-400, -272, -160, -384, 272, -16, PITWALL))
m.add(box(384, -272, -160, 400, 272, -16, PITWALL))
m.add(box(-400, -272, -160, 400, -256, -16, PITWALL))
m.add(box(-400, 256, -160, 400, 272, -16, PITWALL))
m.add(box(-384, -256, -160, 384, 256, -96, SLIME))

# ---- bridge with lip rails and cylindrical supports ----
m.add(box(-48, -256, -16, 48, 256, 0, TRIM, zp="plat_top1", zn=PITWALL))
m.add(box(-60, -256, 0, -48, 256, 10, TRIM))
m.add(box(48, -256, 0, 60, 256, 10, TRIM))
m.add(cylinder(0, -128, 24, -160, -16, PITWALL, sides=8))
m.add(cylinder(0, 128, 24, -160, -16, PITWALL, sides=8))

# ---- megahealth island ----
m.add(cylinder(232, 0, 40, -160, -88, "mmetal1_5", sides=12, cap_hi="plat_top2"))

# ---- escape steps out of the slime (south pit wall) ----
for i in range(1, 11):
    m.add(box(-384 + 24 * (i - 1), -256, -160, -384 + 24 * i, -192, -160 + 16 * i,
              TRIM, zp=FLOOR))

# ---- upper ledges (north and south, z 192) ----
m.add(box(-512, 256, 176, 512, 384, 192, TRIM, zp=FLOOR, zn=CEIL))
m.add(box(-512, -384, 176, 512, -256, 192, TRIM, zp=FLOOR, zn=CEIL))

# ---- stairs: east to north ledge, west to south ledge ----
m.add(stairs(384, -32, 512, 256, 0, 192, "y", "+", TRIM, top_tex=FLOOR))
m.add(stairs(-512, -256, -384, 32, 0, 192, "y", "-", TRIM, top_tex=FLOOR))

# ---- arcade under the ledge fronts: cylinder columns + arches ----
COLS = (-384, -128, 128, 384)
for cx in COLS:
    m.add(cylinder(cx, 272, 16, 0, 176, PILLAR, sides=12))
    m.add(cylinder(cx, -272, 16, 0, 176, PILLAR, sides=12))
spans = [(-512, -384), (-384, -128), (-128, 128), (128, 384), (384, 512)]
for a, b in spans:
    a1 = a + 16 if a in COLS else a
    a2 = b - 16 if b in COLS else b
    m.add(arch("x", a1, a2, 256, 288, 120, 56, 176, TRIM, segments=6))
    m.add(arch("x", a1, a2, -288, -256, 120, 56, 176, TRIM, segments=6))

# ---- ceiling beams with hanging light fixtures ----
for x in (-272, -16, 240):
    m.add(box(x, -400, 288, x + 32, 400, 320, BEAM))
for fx, fy in ((-256, -160), (-256, 160), (0, 0), (256, -160), (256, 160)):
    m.add(box(fx - 12, fy - 12, 272, fx + 12, fy + 12, 288, "light1_1"))
m.light(-256, -160, 256, 400, wait=0.8)
m.light(-256, 160, 256, 400, wait=0.8)
m.light(256, -160, 256, 400, wait=0.8)
m.light(256, 160, 256, 400, wait=0.8)
m.light(0, 0, 264, 450, wait=0.8)

# ---- wall ribs above the ledges ----
for x in (-416, -160, 96, 352):
    m.add(box(x, 376, 192, x + 64, 384, 320, RIB))
    m.add(box(x, -384, 192, x + 64, -376, 320, RIB))
m.light(-256, 352, 248, 200)
m.light(256, 352, 248, 200)
m.light(-256, -352, 248, 200)
m.light(256, -352, 248, 200)

# ---- base skirts along the long walls ----
m.add(prism([(368, 0), (384, 0), (384, 16)], "x", -512, 512, TRIM))
m.add(prism([(-384, 0), (-368, 0), (-384, 16)], "x", -512, 512, TRIM))

# ---- sconces: west wall flanking the door ----
m.add(box(-512, 224, 96, -496, 256, 160, "light1_3"))
m.add(box(-512, 0, 96, -496, 32, 160, "light1_3"))
m.light(-480, 240, 128, 300)
m.light(-480, 16, 128, 300)

# ---- sconces under the ledges (north/south walls) ----
for sx in (-240, 144):
    m.add(box(sx, -384, 64, sx + 32, -368, 128, "light1_3"))
    m.add(box(sx, 368, 64, sx + 32, 384, 128, "light1_3"))
m.light(-224, -360, 96, 250)
m.light(160, -360, 96, 250)
m.light(-224, 360, 96, 250)
m.light(160, 360, 96, 250)

# ---- stair lights ----
m.add(box(496, 96, 224, 512, 128, 256, "light1_3"))
m.add(box(-512, -144, 224, -496, -112, 256, "light1_3"))
m.light(480, 112, 240, 280)
m.light(-480, -128, 240, 280)

# ---- slime glow ----
for gx in (-200, 0, 200):
    for gy in (-128, 128):
        m.light(gx, gy, -72, 200, wait=0.7, _color="140 255 90")

# ---- corridor (x -768..-512, y 64..192, z 0..128) ----
m.add(box(-768, 48, -16, -512, 208, 0, FLOOR, zp=FLOOR))
m.add(box(-768, 192, -16, -512, 208, 144, WALL2))
m.add(box(-768, 48, -16, -512, 64, 144, WALL2))
m.add(box(-768, 48, 128, -512, 208, 144, CEIL))
m.light(-640, 128, 104, 220, style=10)

# ---- teleporter chamber (interior x -1024..-768, y 0..256, z 0..192) ----
m.add(box(-1040, -16, -16, -768, 272, 0, FLOOR, zp=FLOOR))
m.add(box(-1040, -16, 192, -768, 272, 208, CEIL))
m.add(box(-1040, -16, -16, -1024, 272, 208, WALL2))
m.add(box(-1040, 256, -16, -768, 272, 208, WALL2))
m.add(box(-1040, -16, -16, -768, 0, 208, WALL2))
m.add(box(-784, -16, -16, -768, 64, 208, WALL2))
m.add(box(-784, 192, -16, -768, 272, 208, WALL2))
m.add(box(-784, 64, 128, -768, 192, 208, WALL2))

# teleporter: arched frame, glowing surface, pad
m.add(box(-1024, 72, 0, -1000, 88, 152, PILLAR))
m.add(box(-1024, 168, 0, -1000, 184, 152, PILLAR))
m.add(arch("y", 88, 168, -1024, -1000, 104, 48, 176, PILLAR, segments=4))
m.add(box(-1016, 88, 8, -1008, 168, 152, "*teleport"))
m.add(box(-1008, 96, 0, -944, 160, 8, TRIM, zp="tele_top"))
m.light(-976, 128, 160, 300)
m.light(-800, 32, 160, 150)
m.ent("trigger_teleport", brush=box(-1008, 88, 8, -944, 168, 136, "black"),
      target="tele1")

# arrival pad on the north ledge
m.add(box(-32, 288, 192, 32, 352, 200, TRIM, zp="tele_top"))
m.ent("info_teleport_destination", origin="0 320 224", angle="270",
      targetname="tele1")

# ---- items ----
m.ent("weapon_rocketlauncher", origin="0 0 24")
m.ent("weapon_supernailgun", origin="-128 320 216")
m.ent("weapon_grenadelauncher", origin="-896 128 24")
m.ent("item_armor2", origin="-400 -320 216")
m.ent("item_health", origin="232 0 -64", spawnflags=2)      # megahealth
m.ent("item_rockets", origin="64 -320 24")
m.ent("item_rockets", origin="128 -320 24")
m.ent("item_spikes", origin="-320 320 216")
m.ent("item_spikes", origin="-256 320 216")
m.ent("item_shells", origin="-640 96 24")
m.ent("item_shells", origin="-640 160 24")
m.ent("item_health", origin="256 336 24")
m.ent("item_health", origin="-448 128 24")
m.ent("item_health", origin="0 -300 24", spawnflags=1)      # rotten
m.ent("item_health", origin="0 300 24", spawnflags=1)

# ---- spawns ----
m.ent("info_player_start", origin="-448 -320 24", angle="0")
m.ent("info_player_deathmatch", origin="-448 320 24", angle="270")
m.ent("info_player_deathmatch", origin="448 -320 24", angle="90")
m.ent("info_player_deathmatch", origin="-880 48 24", angle="0")
m.ent("info_player_deathmatch", origin="256 320 216", angle="270")

# ---- ambience ----
m.ent("ambient_drip", origin="-100 -220 -40")
m.ent("ambient_drip", origin="150 180 -40")
m.ent("ambient_swamp1", origin="250 -100 -60")
m.ent("ambient_swamp2", origin="-250 100 -60")

m.write(os.path.join(os.path.dirname(__file__), "..", "out", "sludge.map"))


# ---- arcade fill lights ----
for cx in (-256, 0, 256, 448, -448):
    m.light(cx, 320, 100, 120, wait=1.2)
    m.light(cx, -320, 100, 120, wait=1.2)
