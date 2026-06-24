"""Layout IR smoke test: three rooms wired by a corridor, an arch, and a
teleport. Verifies sealing, the build-time navcheck, and that connections
render. Writes out/_layout_test.map.
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from qlayout import Layout

L = Layout("Layout Test", "metal", worldtype=1, minlight=6)

# A (spawn) --corridor--> B --arch--> C ;  A --teleport--> C (alt route)
A = L.room("A", -512, -192, -128, 192, 0, 256)
B = L.room("B", 128, -192, 512, 192, 0, 256)
C = L.room("C", 128, 224, 512, 608, 0, 256, chamfer=48)

L.connect("corridor", A, B)
L.connect("arch", B, C)
L.connect("teleport", A, C, a_at=(-480, 160), b_at=(180, 560))

for r in (A, B, C):
    # ceiling fixtures + hot fill (dark metal eats light)
    for fx in (r.x0 + 96, r.cx, r.x1 - 96):
        r.fixture(fx, r.cy, 200)
    for gx in (r.x0 + 100, r.cx, r.x1 - 100):
        for gy in (r.y0 + 100, r.y1 - 100):
            r.light(gx, gy, 220, 300)

A.spawn(-440, 0, angle=0, dm=False)
A.item("weapon_supernailgun", -300, -120)
B.item("item_health", 320, 0, spawnflags=2)        # megahealth, seated
B.spawn(440, 120, angle=180)
C.item("item_armor2", 320, 416)
C.spawn(180, 560, angle=270)

out = os.path.join(os.path.dirname(__file__), "..", "out", "_layout_test.map")
L.write(out)
print("navcheck passed; wrote", os.path.abspath(out))
