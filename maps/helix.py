#!/usr/bin/env python3
"""The Ashen Spire — a gothic keep that ASCENDS, on the qlayout `stairs`
connector.

Nine brick halls on a 3x3 footprint spiralling upward in 112-unit steps
(0 -> 784), wired nose-to-tail by torchlit stairs; a teleporter drops from the
summit back to the flooded base to close the big circuit, a central hub and a
cross-teleporter add more. The climb is lit by fire — warm torch pools with
deep shadow between — and it ends at a cold, fog-lit sky summit you can see
from the dark below. Atmosphere over evenness: moody, not flat, not cave-black.
Emits out/helix.map.
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import qprefab as P
from qlayout import Layout

TH = "wizard"          # elder brick + torches: the gothic AD register
H = 480                # interior half-extent (960 wide rooms)
P3 = 1088              # grid pitch (leaves a 128u void for stair shafts)
RH = 256               # room height
WARM = "255 196 130"   # firelight
COOL = "150 180 235"   # moonlight bleeding down the shaft

L = Layout("The Ashen Spire", TH, worldtype=0, minlight=6,
           fog="0.02 0.05 0.055 0.075",
           _sunlight="180", _sun_mangle="25 -60 0", _sunlight_penumbra="8",
           _sunlight2="55", _sunlight_color="160 185 235")


def room(name, cx, cy, z0):
    return L.room(name, cx - H, cy - H, cx + H, cy + H, z0, z0 + RH)


# perimeter ring (clockwise from SW), each 112 higher than the last
SW = room("sw", -P3, -P3,   0)
W  = room("w",  -P3,   0, 112)
NW = room("nw", -P3,  P3, 224)
N  = room("n",    0,  P3, 336)
NE = room("ne",  P3,  P3, 448)
E  = room("e",   P3,   0, 560)
SE = room("se",  P3, -P3, 672)
S  = room("s",    0, -P3, 784)
HUB = room("hub", 0,   0, 448)

# the summit opens to the sky: taller, capped with sky for raking moonlight
S.z1 = S.z0 + 416
S.ceil_tex = "sky4"

# ascending spiral of stairs
for a, b in ((SW, W), (W, NW), (NW, N), (N, NE), (NE, E), (E, SE), (SE, S)):
    L.connect("stairs", a, b, width=128)
# hub spokes: stair climbs to N and E (both a tier away)
L.connect("stairs", HUB, N, width=128)
L.connect("stairs", HUB, E, width=128)
# teleporters close the loops: summit -> flooded base, and a cross-link
L.connect("teleport", S, SW, name="helix_drop",
          a_at=(0, -P3 - 240), b_at=(-P3, -P3))
L.connect("teleport", NW, SE, name="helix_cross",
          a_at=(-P3 - 240, P3), b_at=(P3, -P3))

ITEMS = {
    "sw": "weapon_supershotgun", "w": "item_shells", "nw": "weapon_nailgun",
    "n": "item_spikes", "ne": "weapon_grenadelauncher", "e": "item_cells",
    "se": "weapon_supernailgun", "s": "weapon_rocketlauncher",
    "hub": "item_armorInv",
}
VAULTED = (HUB, NW, NE, W, E)     # feature rooms get a barrel-vaulted lid
DRIPS = (SW, W, HUB)              # wet lower halls
START = SW

for r in (SW, W, NW, N, NE, E, SE, S, HUB):
    big = r is HUB
    r.place(P.pillar(r.cx, r.cy, r.z0, r.z1, TH, w=160 if big else None,
                     round_=not big))
    if r in VAULTED:
        r.add(P.ceiling_vault(r.x0, r.y0, r.x1, r.y1, r.z1 - 80, TH,
                              rise=80, axis="x").brushes)

    r.item(ITEMS[r.name], r.cx - 200, r.cy - 200, height=8)
    if r is START:
        r.ent("info_player_start",
              origin=f"{r.cx+160:g} {r.cy+160:g} {r.z0+24:g}", angle="45")
    else:
        r.spawn(r.cx + 160, r.cy + 160, 225)
    r.spawn(r.cx + 160, r.cy - 200, 315)

    # firelight: a wall torch mid each wall, each with a tight warm pool so the
    # light reads as cast BY the flame and falls off into shadow between torches
    for fx, fy, nrm in ((r.x0 + 12, r.cy, (1, 0)), (r.x1 - 12, r.cy, (-1, 0)),
                        (r.cx, r.y0 + 12, (0, 1)), (r.cx, r.y1 - 12, (0, -1))):
        r.fixture(fx, fy, r.z0 + 150, wall_normal=nrm)
        r.light(fx, fy, r.z0 + 140, 195, _color=WARM, wait="1.1")
    # corner standing flames throw low floor pools (more pools, deeper gaps)
    for sx in (-1, 1):
        for sy in (-1, 1):
            px, py = r.cx + sx * (H - 96), r.cy + sy * (H - 96)
            r.ent("light_flame_small_yellow",
                  origin=f"{px:g} {py:g} {r.z0 + 20:g}")
            r.light(px, py, r.z0 + 64, 150, _color=WARM, wait="1.25")
    r.light(r.cx - 200, r.cy - 200, r.z0 + 70, 150, _color=WARM)  # item key

# summit: let the cold sky win — moonlit fills high in the shaft + a cool floor
for cz in (S.z0 + 120, S.z0 + 300):
    S.light(S.cx, S.cy, cz, 150, _color=COOL, wait="0.9")
for sx in (-1, 1):
    S.light(S.cx + sx * 260, S.cy, S.z0 + 360, 130, _color=COOL, wait="0.9")

# ambience: dripping water below, wind keening at the open summit
for r in DRIPS:
    r.ent("ambient_drip", origin=f"{r.cx + 120:g} {r.cy - 120:g} {r.z0 + 40:g}")
S.ent("ambient_wind", origin=f"{S.cx:g} {S.cy:g} {S.z0 + 300:g}")

L.write(os.path.join(os.path.dirname(__file__), "..", "out", "helix.map"))
