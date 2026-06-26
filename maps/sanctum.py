#!/usr/bin/env python3
"""Iron Sanctum — a runic-metal fortress in a 2x2 ring.

Spec (one theme: metal):
  Four large halls tiled in a 2x2 grid, wired as a closed ring (A-B-D-C-A)
  with a teleporter chord (A<->D) so there are two circuits, not a tree.
  Verticality lives INSIDE the rooms: landed stairs to gallery walkways,
  a catwalk bridge, vaulted ceilings. Occlusion comes from sealed shells
  that don't see each other plus pillar clusters and a sightline baffle.

  A (SW)  spawn atrium: pillar cluster + south gallery ledge (stairs up).
  B (SE)  bridge hall : catwalk across at mid height, baffle splits the floor.
  C (NW)  stair tower : long landed climb to a high firing ledge.
  D (NE)  the vault   : tallest, barrel-vaulted, central tower platform.

Emits out/sanctum.map.
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import qprefab as P
from qlayout import Layout

TH = "metal"
L = Layout("Iron Sanctum", TH, worldtype=0, minlight=10,
           _sunlight="0")

# ---- rooms: 2x2 grid, all floors at z0=0 so doors/corridors align ----------
A = L.room("atrium", -704, -704, -64, -64, 0, 384)
B = L.room("bridge",   64, -704, 704, -64, 0, 352)
C = L.room("tower", -704,   64, -64, 704, 0, 352)
D = L.room("vault",    64,   64, 704, 704, 0, 448)

# ---- ring + chord: two circuits, no dead ends ------------------------------
L.connect("corridor", A, B, width=112)
L.connect("corridor", B, D, width=112)
L.connect("corridor", D, C, width=112)
L.connect("corridor", C, A, width=112)
L.connect("teleport", A, D, name="san_cross",
          a_at=(-640, -640), b_at=(640, 640))

# ============================================================ A: spawn atrium
# central pillar cluster breaks LOS across the hall (occlusion)
for px, py in ((-460, -460), (-308, -460), (-460, -308), (-308, -308)):
    A.place(P.pillar(px, py, 0, 384, TH, round_=True))
# south gallery ledge at z160, reached by a landed stair on the west run
A.place(P.stairs_landed(-680, -680, -240, -560, 0, 160, "x", "+", TH))
A.add(P.walkway(-704, -704, -64, -544, 160, TH, railing=True).brushes)
A.item("item_armorInv", -384, -624, height=8)        # prize on the ledge (z160)
# floor items + spawns
A.item("weapon_supershotgun", -600, -120, height=8)
A.ent("info_player_start", origin="-200 -200 24", angle="45")
for sx, sy, ang in ((-600, -200, 0), (-200, -600, 90), (-520, -520, 45)):
    A.spawn(sx, sy, ang)
# lighting (metal is dark -> many moderate panels + fill)
for fx, fy, nrm in ((-704, -384, (1, 0)), (-384, -704, (0, 1)),
                    (-64, -384, (-1, 0))):
    A.fixture(fx, fy, 220, wall_normal=nrm)
for lx in (-560, -200):
    for ly in (-560, -200):
        A.light(lx, ly, 220, 200)
A.light(-384, -624, 230, 170)                          # ledge prize key light
A.add(P.ceiling_vault(-704, -704, -64, -64, 384, TH, axis="x").brushes)

# ============================================================ B: bridge hall
# a baffle + a high catwalk crossing y, so the floor route bends and the
# bridge gives a second elevation
B.add(P.sightline_baffle(120, -420, 360, -404, 0, TH, height=128).brushes)
B.add(P.walkway(330, -704, 438, -64, 176, TH, railing=True).brushes)
# stairs up onto the catwalk from the south floor
B.place(P.stairs_landed(180, -680, 330, -300, 0, 176, "x", "+", TH))
B.item("weapon_nailgun", 600, -600, height=8)
B.item("item_shells", 150, -150, height=8)
B.item("item_health", 600, -150, pedestal=False, spawnflags=2)  # floor, clear of catwalk
for sx, sy, ang in ((600, -200, 180), (150, -600, 90)):
    B.spawn(sx, sy, ang)
for fx, fy, nrm in ((704, -384, (-1, 0)), (384, -704, (0, 1))):
    B.fixture(fx, fy, 220, wall_normal=nrm)
for lx in (200, 560):
    for ly in (-560, -200):
        B.light(lx, ly, 230, 200)
B.light(438, -384, 230, 170)

# ============================================================ C: stair tower
# a long landed climb to a high firing ledge overlooking the corridor mouths
C.place(P.stairs_landed(-680, 120, -240, 280, 0, 192, "x", "+", TH))
C.add(P.walkway(-704, 96, -64, 300, 192, TH, railing=True).brushes)
C.place(P.pillar(-384, 520, 0, 352, TH, round_=True))
C.ent("weapon_grenadelauncher", origin="-400 200 216")   # on the high ledge (z192)
C.item("item_cells", -600, 600, height=8)
for sx, sy, ang in ((-200, 600, 270), (-600, 450, 0)):
    C.spawn(sx, sy, ang)
for fx, fy, nrm in ((-704, 384, (1, 0)), (-384, 704, (0, -1))):
    C.fixture(fx, fy, 220, wall_normal=nrm)
for lx in (-560, -200):
    for ly in (200, 560):
        C.light(lx, ly, 220, 200)
C.light(-384, 200, 250, 180)
C.add(P.ceiling_vault(-704, 64, -64, 704, 352, TH, axis="y").brushes)

# ============================================================ D: the vault
# tallest hall: central tower platform reached by stairs, barrel vault lid
D.place(P.pillar(384, 384, 0, 224, TH, w=160))           # squat central tower
D.add(P.walkway(304, 304, 464, 464, 224, TH, railing=False).brushes)
D.place(P.stairs_landed(160, 304, 304, 464, 0, 224, "x", "+", TH))
D.ent("weapon_rocketlauncher", origin="384 384 248")       # atop the central tower
D.item("item_armor2", 600, 600, height=8)
D.item("item_rockets", 150, 600, height=8)
for sx, sy, ang in ((600, 200, 135), (200, 600, 225), (440, 620, 200)):
    D.spawn(sx, sy, ang)
for fx, fy, nrm in ((704, 384, (-1, 0)), (384, 704, (0, -1)),
                    (64, 384, (1, 0))):
    D.fixture(fx, fy, 240, wall_normal=nrm)
for lx in (200, 560):
    for ly in (200, 560):
        D.light(lx, ly, 240, 210)
D.light(384, 384, 300, 200)                              # tower-top weapon key
D.add(P.ceiling_vault(64, 64, 704, 704, 384, TH, axis="x").brushes)

# ---- ambient fill: many moderate point lights (metal reads dark; id pacing) -
for r in (A, B, C, D):
    xs = list(range(int(r.x0) + 96, int(r.x1), 176))
    ys = list(range(int(r.y0) + 96, int(r.y1), 176))
    for x in xs:
        for y in ys:
            r.light(x, y, r.z0 + 130, 160)        # warm floor pools
    for x in xs[::2]:
        for y in ys[::2]:
            r.light(x, y, r.z1 - 64, 150)         # sparse upper fill

L.write(os.path.join(os.path.dirname(__file__), "..", "out", "sanctum.map"))
