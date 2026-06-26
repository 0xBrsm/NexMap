#!/usr/bin/env python3
"""Validation map for the qlayout `stairs` connector.

Four metal halls in a 2x2 gapped grid, each floor at a different z0, wired
into a ring entirely by stairs connectors (A->B->D->C->A, climbing then
descending). Tests that the connector seals (no leak) and that terracing
alone lifts the gate's verticality / areas / chokepoint scores. Minimal
interior detail on purpose — the structure should come from the layout.
Emits out/_stairs_test.map.
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import qprefab as P
from qlayout import Layout

TH = "metal"
L = Layout("Stairs Test", TH, worldtype=0, minlight=10)

A = L.room("low",  -704, -704, -64, -64,   0, 384)
B = L.room("east",   64, -704, 704, -64, 112, 448)
D = L.room("high",   64,   64, 704, 704, 224, 544)
C = L.room("west", -704,   64, -64, 704, 112, 448)

L.connect("stairs", A, B, width=112)
L.connect("stairs", B, D, width=112)
L.connect("stairs", D, C, width=112)
L.connect("stairs", C, A, width=112)

A.ent("info_player_start", origin="-384 -384 24", angle="45")
A.place(P.pillar(-384, -384, A.z0, A.z1, TH, round_=True))
A.item("weapon_supershotgun", -560, -200, height=8)
B.place(P.pillar(384, -384, B.z0, B.z1, TH, round_=True))
B.item("item_shells", 560, -200, height=8)
C.place(P.pillar(-384, 384, C.z0, C.z1, TH, round_=True))
C.item("item_cells", -560, 200, height=8)
D.place(P.pillar(384, 384, D.z0, D.z1, TH, w=160))
D.ent("weapon_rocketlauncher", origin="384 384 248")
D.item("item_armor2", 560, 200, height=8)
for r, ang in ((A, 45), (B, 135), (C, 315), (D, 225)):
    r.spawn(r.cx + 120, r.cy + 120, ang)

# fixtures + ambient fill (metal reads dark; corpus pacing wants many lights)
for r in (A, B, C, D):
    for fx, fy, nrm in (((r.x0, r.cy, (1, 0)), (r.cx, r.y0, (0, 1)),
                         (r.x1, r.cy, (-1, 0)))):
        r.fixture(fx, fy, r.z0 + 220, wall_normal=nrm)
    xs = list(range(int(r.x0) + 96, int(r.x1), 176))
    ys = list(range(int(r.y0) + 96, int(r.y1), 176))
    for x in xs:
        for y in ys:
            r.light(x, y, r.z0 + 130, 160)
    for x in xs[::2]:
        for y in ys[::2]:
            r.light(x, y, r.z1 - 64, 150)

L.write(os.path.join(os.path.dirname(__file__), "..", "out", "_stairs_test.map"))
