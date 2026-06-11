"""Quake 1 .map authoring kit: brush primitives + map writer.

Plane convention (matches id .map format): for each face, three points
p0, p1, p2 with cross(p1-p0, p2-p0) pointing INTO the brush.
"""
import math

FACES = ["xn", "xp", "yn", "yp", "zn", "zp"]
AXES = {"x": 0, "y": 1, "z": 2}


def _f(v):
    s = f"{v:.6f}".rstrip("0").rstrip(".")
    return "0" if s == "-0" else s


def _pt(p):
    return f"( {_f(p[0])} {_f(p[1])} {_f(p[2])} )"


def _face(p0, p1, p2, tex):
    return f"{_pt(p0)} {_pt(p1)} {_pt(p2)} {tex} 0 0 0 1 1"


def _vadd(a, b):
    return (a[0] + b[0], a[1] + b[1], a[2] + b[2])


def _vscale(a, s):
    return (a[0] * s, a[1] * s, a[2] * s)


def _cross(a, b):
    return (a[1] * b[2] - a[2] * b[1],
            a[2] * b[0] - a[0] * b[2],
            a[0] * b[1] - a[1] * b[0])


def _norm(a):
    l = math.sqrt(a[0] ** 2 + a[1] ** 2 + a[2] ** 2)
    return (a[0] / l, a[1] / l, a[2] / l)


def plane_face(point, normal, tex):
    """Face line for a plane through `point` with OUTWARD `normal`."""
    n = _norm(normal)
    u = (0, 0, 1) if abs(n[2]) < 0.9 else (1, 0, 0)
    u = _norm(_cross(n, u))
    v = _cross(u, n)  # u x v == -n
    return _face(point, _vadd(point, _vscale(u, 128)), _vadd(point, _vscale(v, 128)), tex)


def box(x1, y1, z1, x2, y2, z2, tex, **face_tex):
    t = {f: face_tex.get(f, tex) for f in FACES}
    planes = [
        (f"( {x1} 0 0 ) ( {x1} 1 0 ) ( {x1} 0 1 )", t["xn"]),
        (f"( {x2} 0 0 ) ( {x2} 0 1 ) ( {x2} 1 0 )", t["xp"]),
        (f"( 0 {y1} 0 ) ( 0 {y1} 1 ) ( 1 {y1} 0 )", t["yn"]),
        (f"( 0 {y2} 0 ) ( 1 {y2} 0 ) ( 0 {y2} 1 )", t["yp"]),
        (f"( 0 0 {z1} ) ( 1 0 {z1} ) ( 0 1 {z1} )", t["zn"]),
        (f"( 0 0 {z2} ) ( 0 1 {z2} ) ( 1 0 {z2} )", t["zp"]),
    ]
    return "{\n" + "\n".join(f"{p} {tx} 0 0 0 1 1" for p, tx in planes) + "\n}"


def prism(points, axis, lo, hi, tex, cap_lo=None, cap_hi=None):
    """Convex prism: 2D cross-section `points` (CCW) extruded along `axis`.

    For axis "z", points are (x, y); for "x", (y, z); for "y", (x, z) —
    always the remaining coordinates in natural order, wound CCW in that
    2D coordinate plane's standard orientation.
    """
    ai = AXES[axis]
    o1, o2 = [i for i in range(3) if i != ai]  # the in-plane axes

    def to3(p2, a):
        v = [0.0, 0.0, 0.0]
        v[ai] = a
        v[o1], v[o2] = p2
        return tuple(v)

    faces = []
    n = len(points)
    for i in range(n):
        a, b = points[i], points[(i + 1) % n]
        e = (b[0] - a[0], b[1] - a[1])
        en = (e[1], -e[0])  # outward normal of a CCW edge, axis-independent
        normal = [0.0, 0.0, 0.0]
        normal[o1], normal[o2] = en
        faces.append(plane_face(to3(a, lo), tuple(normal), tex))

    lo_n = [0, 0, 0]
    lo_n[ai] = -1
    hi_n = [0, 0, 0]
    hi_n[ai] = 1
    faces.append(plane_face(to3(points[0], lo), tuple(lo_n), cap_lo or tex))
    faces.append(plane_face(to3(points[0], hi), tuple(hi_n), cap_hi or tex))
    return "{\n" + "\n".join(faces) + "\n}"


def wedge(x1, y1, z1, x2, y2, z2, axis, high_end, tex, **kw):
    """Ramp: box whose top slopes along `axis` ("x" or "y") from z1 at the
    low end to z2 at `high_end` ("+" or "-") end of that axis."""
    if axis == "x":
        lo, hi = (x1, x2) if high_end == "+" else (x2, x1)
        pts = [(lo, z1), (hi, z1), (hi, z2)] if high_end == "+" else [(lo, z2), (lo, z1), (hi, z1)]
        return prism(pts, "y", y1, y2, tex, **kw)
    lo, hi = (y1, y2) if high_end == "+" else (y2, y1)
    pts = [(lo, z1), (hi, z1), (hi, z2)] if high_end == "+" else [(lo, z2), (lo, z1), (hi, z1)]
    return prism([(p[0], p[1]) for p in pts], "x", x1, x2, tex, **kw)


def cylinder(cx, cy, r, z1, z2, tex, sides=8, **kw):
    pts = []
    for i in range(sides):
        a = 2 * math.pi * i / sides
        pts.append((cx + r * math.cos(a), cy + r * math.sin(a)))
    return prism(pts, "z", z1, z2, tex, **kw)


def arch(axis, a1, a2, depth1, depth2, z_base, rise, z_top, tex, segments=6):
    """Arched opening header: brushes filling from a curved soffit up to
    z_top. Opening spans a1..a2 along `axis` ("x" or "y"); the wall runs
    depth1..depth2 across the other horizontal axis. The curve springs from
    z_base at the jambs to z_base+rise at the apex (half-ellipse).
    Returns a list of brushes; place jamb walls/columns separately."""
    w = (a2 - a1) / 2
    cx = (a1 + a2) / 2
    brushes = []
    ext_axis = "y" if axis == "x" else "x"
    for i in range(segments):
        t0 = math.pi * i / segments
        t1 = math.pi * (i + 1) / segments
        u0, h0 = cx - w * math.cos(t0), z_base + rise * math.sin(t0)
        u1, h1 = cx - w * math.cos(t1), z_base + rise * math.sin(t1)
        # quad: curve chord up to flat top (CCW in (u, z))
        pts = [(u0, h0), (u1, h1), (u1, z_top), (u0, z_top)]
        if pts[0][1] >= z_top - 0.01 and pts[1][1] >= z_top - 0.01:
            continue
        pts = [(u, min(h, z_top)) for u, h in pts]
        # drop degenerate duplicate corners at the apex
        dedup = []
        for p in pts:
            if not dedup or (abs(p[0] - dedup[-1][0]) > 0.01 or abs(p[1] - dedup[-1][1]) > 0.01):
                dedup.append(p)
        while len(dedup) > 1 and abs(dedup[0][0] - dedup[-1][0]) < 0.01 and abs(dedup[0][1] - dedup[-1][1]) < 0.01:
            dedup.pop()
        if len(dedup) < 3:
            continue
        brushes.append(prism(dedup, ext_axis, depth1, depth2, tex))
    return brushes


def stairs(x1, y1, x2, y2, z_base, z_top, axis, direction, tex,
           step_h=16, top_tex=None, solid_base=True):
    """Straight stairs from z_base to z_top ascending along `axis` toward
    `direction` ("+" or "-"). Returns a list of box brushes."""
    n = max(1, round((z_top - z_base) / step_h))
    run = ((x2 - x1) if axis == "x" else (y2 - y1)) / n
    out = []
    for i in range(1, n + 1):
        top = z_base + step_h * i
        bot = z_base if solid_base else top - step_h
        if axis == "x":
            sx1 = x1 + run * (i - 1) if direction == "+" else x2 - run * i
            sx2 = sx1 + run
            out.append(box(sx1, y1, bot, sx2, y2, top, tex, zp=top_tex or tex))
        else:
            sy1 = y1 + run * (i - 1) if direction == "+" else y2 - run * i
            sy2 = sy1 + run
            out.append(box(x1, sy1, bot, x2, sy2, top, tex, zp=top_tex or tex))
    return out


def spiral_stairs(cx, cy, r_inner, r_outer, z_base, step_h, n, tex,
                  start_angle=0.0, step_angle=30.0, top_tex=None, arc_segs=2):
    """Spiral stairs winding CCW around (cx, cy). Each step is a sector
    prism from its tread height down `step_h*2` (floating spiral)."""
    out = []
    for i in range(n):
        a0 = math.radians(start_angle + step_angle * i)
        a1 = math.radians(start_angle + step_angle * (i + 1))
        pts = []
        for k in range(arc_segs + 1):
            a = a0 + (a1 - a0) * k / arc_segs
            pts.append((cx + r_outer * math.cos(a), cy + r_outer * math.sin(a)))
        for k in range(arc_segs + 1):
            a = a1 + (a0 - a1) * k / arc_segs
            pts.append((cx + r_inner * math.cos(a), cy + r_inner * math.sin(a)))
        top = z_base + step_h * (i + 1)
        out.append(prism(pts, "z", top - step_h * 2, top, tex, cap_hi=top_tex or tex))
    return out


class MapWriter:
    def __init__(self, message, wad="mapgen_textures.wad", worldtype=1,
                 minlight=5, sounds=6):
        self.props = {"wad": wad, "worldtype": str(worldtype),
                      "message": message, "sounds": str(sounds),
                      "_minlight": str(minlight)}
        self.brushes = []
        self.entities = []

    def add(self, b):
        if isinstance(b, list):
            self.brushes.extend(b)
        else:
            self.brushes.append(b)

    def ent(self, classname, brush=None, **keys):
        lines = ["{", f'"classname" "{classname}"']
        for k, v in keys.items():
            lines.append(f'"{k}" "{v}"')
        if brush:
            lines.append(brush if isinstance(brush, str) else "\n".join(brush))
        lines.append("}")
        self.entities.append("\n".join(lines))

    def light(self, x, y, z, value, **keys):
        self.ent("light", origin=f"{x} {y} {z}", light=value, **keys)

    def write(self, path):
        ws = ["{"] + [f'"{k}" "{v}"' for k, v in self.props.items()]
        ws += ['"classname" "worldspawn"'] + self.brushes + ["}"]
        with open(path, "w") as f:
            f.write("\n".join(ws) + "\n" + "\n".join(self.entities) + "\n")
        print(f"{len(self.brushes)} brushes, {len(self.entities)} entities -> {path}")
