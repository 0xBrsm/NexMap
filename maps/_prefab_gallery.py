"""Prefab gallery: one of each qprefab component in a lit hall, so every
prefab can be built through qbsp/vis/light and eyeballed in a render.

Writes out/_prefab_gallery.map.
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import qgeo
import qprefab as P
from qtheme import THEMES

THEME = "metal"
th = THEMES[THEME]

m = qgeo.MapWriter("Prefab Gallery", minlight=8)

# Hall shell: 1024 (x) x 512 (y) x 320 high, walls 16 thick.
X0, X1 = -512, 512
Y0, Y1 = -256, 256
Z0, Z1 = 0, 320
T = 16
m.add(qgeo.box(X0 - T, Y0 - T, Z0 - T, X1 + T, Y1 + T, Z0, th["floor"]))   # floor
m.add(qgeo.box(X0 - T, Y0 - T, Z1, X1 + T, Y1 + T, Z1 + T, th["ceiling"]))  # ceiling
m.add(qgeo.box(X0 - T, Y0 - T, Z0, X0, Y1 + T, Z1, th["wall"]))            # -x
m.add(qgeo.box(X1, Y0 - T, Z0, X1 + T, Y1 + T, Z1, th["wall"]))           # +x
m.add(qgeo.box(X0 - T, Y0 - T, Z0, X1 + T, Y0, Z1, th["wall"]))           # -y
m.add(qgeo.box(X0 - T, Y1, Z0, X1 + T, Y1 + T, Z1, th["wall"]))          # +y

# A row of prefabs along x, each in its own bay.
m.place(P.pillar(-400, 160, Z0, Z1, THEME, round_=True))
m.place(P.pillar(-400, -160, Z0, Z1, THEME))
m.place(P.stairs_landed(-320, -64, -64, 64, Z0, 160, "x", "+", THEME))
m.place(P.item_pedestal(-180, 160, Z0, "item_health", THEME, spawnflags=2))
m.place(P.item_pedestal(-180, -160, Z0, "weapon_supernailgun", THEME))
m.place(P.light_fixture(0, Y1 - 8, 200, THEME, wall_normal=(0, 1)))
m.place(P.light_fixture(0, Y0 + 8, 200, THEME, wall_normal=(0, -1)))
m.place(P.archway("y", -96, 96, 100, 160, Z0, 224, THEME))
m.place(P.chamfered_corner(X0, Y0, Z0, Z1, THEME, size=48, quadrant="++"))
m.place(P.chamfered_corner(X1, Y1, Z0, Z1, THEME, size=48, quadrant="--"))
m.place(P.curved_wall(256, -160, 80, Z0, Z1, THEME, a0=0, a1=90))
m.place(P.wainscot(120, 100, 360, 240, Z0, THEME, height=72))
m.place(P.walkway(180, -240, 460, -120, 120, THEME))
m.place(P.window_embrasure("x", 380, 460, -8, 8, 120, 220, THEME,
                           liquid_glow=True))
m.place(P.teleporter_pad(400, 0, Z0, THEME, target="gallery_exit",
                         dest_origin=(-440, 0, Z0 + 24), dest_name="gallery_exit"))
m.place(P.ceiling_vault(-96, -240, 96, 240, Z1 - 64, THEME, axis="y"))

# General fill light so the render isn't black between fixtures.
# Dark metal eats light (~2x vs brown base), and this is a 1024x512 hall, so
# the gallery is lit hot purely to verify prefab geometry reads clearly.
for gx in range(-420, 421, 140):
    for gy in (-160, 0, 160):
        m.light(gx, gy, 250, 400)

m.ent("info_player_start", origin="-440 0 24", angle="0")

out = os.path.join(os.path.dirname(__file__), "..", "out", "_prefab_gallery.map")
m.write(os.path.abspath(out))
