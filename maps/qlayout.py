"""Layout IR for NexMap: compose a map from Rooms wired by Connections.

This is the planning layer above raw brushes. You describe rooms (sealed box
shells) and how they connect (doors, corridors, stairs, arches, drops,
teleports); the layer punches the wall openings, builds the connector
geometry, and runs a build-time reachability BFS over the room graph so a
room you forgot to connect is a hard error, not a silent dead zone.

It is deliberately NOT a rigid schema: Room.add()/place() take raw qgeo
brushes and qprefab Prefabs, and shell=False is a full escape hatch to hand
-author a room's geometry. The IR organizes space and guarantees sealing +
connectivity; everything else stays open Python.

Convention: a Room's bounds are its INTERIOR playable volume. Floor top sits
at z0, ceiling underside at z1; walls/floor/ceiling slabs are built outward by
`wall` thickness. Faces are named xn/xp/yn/yp (the -x/+x/-y/+y walls).
"""
import os

import qgeo
import qprefab as P
from qtheme import METRICS as M
from qtheme import THEMES

FACE_AXIS = {"xn": ("x", 0), "xp": ("x", 1), "yn": ("y", 0), "yp": ("y", 1)}


def _wall_with_openings(fixed_lo, fixed_hi, span0, span1, z0, z1, openings,
                        tex, along):
    """Build a wall slab as boxes, leaving rectangular openings. `along` is
    "y" if the wall runs along y (a +-x wall) or "x" if along x (a +-y wall).
    fixed_lo/fixed_hi are the wall's thickness extent on the perpendicular
    axis. openings: list of (s_lo, s_hi, z_lo, z_hi) along the span."""
    out = []

    def slab(s_lo, s_hi, zl, zh):
        if s_hi - s_lo <= 0 or zh - zl <= 0:
            return
        if along == "y":
            out.append(qgeo.box(fixed_lo, s_lo, zl, fixed_hi, s_hi, zh, tex))
        else:
            out.append(qgeo.box(s_lo, fixed_lo, zl, s_hi, fixed_hi, zh, tex))

    cur = span0
    for s_lo, s_hi, zl, zh in sorted(openings):
        if s_lo > cur:
            slab(cur, s_lo, z0, z1)          # solid pier before opening
        slab(s_lo, s_hi, z0, zl)             # below opening
        slab(s_lo, s_hi, zh, z1)             # above opening
        cur = max(cur, s_hi)
    if cur < span1:
        slab(cur, span1, z0, z1)
    return out


class Room:
    def __init__(self, name, x0, y0, x1, y1, z0, z1, theme, wall=None,
                 shell=True, chamfer=0, floor_tex=None, ceil_tex=None,
                 sky=False):
        self.name = name
        self.x0, self.y0, self.x1, self.y1 = x0, y0, x1, y1
        self.z0, self.z1 = z0, z1
        self.theme = theme
        self.th = THEMES[theme]
        self.wall = wall if wall is not None else M["wall_thickness"]
        self.shell = shell
        self.chamfer = chamfer
        self.floor_tex = floor_tex or self.th["floor"]
        self.ceil_tex = ceil_tex or self.th["ceiling"]
        self.sky = sky
        self.openings = {f: [] for f in FACE_AXIS}   # face -> [(s_lo,s_hi,zl,zh)]
        self._brushes = []
        self._ents = []

    @property
    def cx(self):
        return (self.x0 + self.x1) / 2

    @property
    def cy(self):
        return (self.y0 + self.y1) / 2

    def contains(self, x, y):
        return self.x0 <= x <= self.x1 and self.y0 <= y <= self.y1

    # ----- authoring surface
    def add(self, b):
        if isinstance(b, list):
            self._brushes.extend(b)
        else:
            self._brushes.append(b)
        return self

    def place(self, prefab):
        self._brushes.extend(prefab.brushes)
        self._ents.extend(prefab.ents)
        return prefab

    def light(self, x, y, z, value=None, **keys):
        v = value if value is not None else self.th["light_value"]
        self._ents.append(("light", {"origin": f"{x:g} {y:g} {z:g}",
                                      "light": v, **keys}, None))

    def ent(self, classname, brush=None, **keys):
        self._ents.append((classname, keys, brush))

    def fixture(self, x, y, z, value=None, wall_normal=None):
        self.place(P.light_fixture(x, y, z, self.theme, value, wall_normal))

    def item(self, classname, x, y, pedestal=True, height=16, spawnflags=None,
             lit=True):
        """Place an item; by default on a pedestal so it never floats and is
        always reachable from the room floor (z0)."""
        if pedestal:
            self.place(P.item_pedestal(x, y, self.z0, classname, self.theme,
                                       height=height, spawnflags=spawnflags,
                                       lit=lit))
        else:
            keys = {"origin": f"{x:g} {y:g} {self.z0 + M['item_z']:g}"}
            if spawnflags is not None:
                keys["spawnflags"] = spawnflags
            self._ents.append((classname, keys, None))

    def spawn(self, x, y, angle=0, dm=True):
        cls = "info_player_deathmatch" if dm else "info_player_start"
        self._ents.append((cls, {"origin": f"{x:g} {y:g} {self.z0 + 24:g}",
                                  "angle": str(angle)}, None))

    def punch(self, face, s_lo, s_hi, z_lo, z_hi):
        self.openings[face].append((s_lo, s_hi, z_lo, z_hi))

    # ----- emission
    def emit(self):
        brushes, ents = list(self._brushes), list(self._ents)
        if self.shell:
            T = self.wall
            x0, y0, x1, y1, z0, z1 = (self.x0, self.y0, self.x1, self.y1,
                                      self.z0, self.z1)
            # floor + ceiling
            brushes.append(qgeo.box(x0 - T, y0 - T, z0 - T, x1 + T, y1 + T, z0,
                                    self.floor_tex))
            brushes.append(qgeo.box(x0 - T, y0 - T, z1, x1 + T, y1 + T, z1 + T,
                                    "sky1" if self.sky else self.ceil_tex))
            w = self.th["wall"]
            brushes += _wall_with_openings(x0 - T, x0, y0, y1, z0, z1,
                                           self.openings["xn"], w, "y")
            brushes += _wall_with_openings(x1, x1 + T, y0, y1, z0, z1,
                                           self.openings["xp"], w, "y")
            brushes += _wall_with_openings(y0 - T, y0, x0, x1, z0, z1,
                                           self.openings["yn"], w, "x")
            brushes += _wall_with_openings(y1, y1 + T, x0, x1, z0, z1,
                                           self.openings["yp"], w, "x")
            if self.chamfer:
                for q, (ox, oy) in (("++", (x0, y0)), ("-+", (x1, y0)),
                                    ("+-", (x0, y1)), ("--", (x1, y1))):
                    brushes += P.chamfered_corner(ox, oy, z0, z1, self.theme,
                                                  size=self.chamfer,
                                                  quadrant=q).brushes
        return brushes, ents


class Connection:
    def __init__(self, kind, a, b, **kw):
        self.kind = kind
        self.a, self.b = a, b
        self.kw = kw

    def _facing(self):
        """Return (face_a, face_b, axis, sign) for the walls of a and b that
        face each other, based on room-center geometry."""
        a, b = self.a, self.b
        dx, dy = b.cx - a.cx, b.cy - a.cy
        if abs(dx) >= abs(dy):
            return ("xp", "xn", "x", 1) if dx >= 0 else ("xn", "xp", "x", -1)
        return ("yp", "yn", "y", 1) if dy >= 0 else ("yn", "yp", "y", -1)

    def emit(self):
        return getattr(self, f"_emit_{self.kind}")()

    def _span_center(self, room, axis):
        return room.cy if axis == "x" else room.cx

    def _emit_door(self):
        a, b = self.a, self.b
        fa, fb, axis, sign = self._facing()
        w = self.kw.get("width", M["door_w"])
        h = self.kw.get("height", M["door_h"])
        sill = self.kw.get("sill", 0)
        c = (self._span_center(a, axis) + self._span_center(b, axis)) / 2
        zl = a.z0 + sill
        a.punch(fa, c - w / 2, c + w / 2, zl, zl + h)
        b.punch(fb, c - w / 2, c + w / 2, zl, zl + h)
        return [], []

    def _emit_arch(self):
        a, b = self.a, self.b
        fa, fb, axis, sign = self._facing()
        w = self.kw.get("width", M["door_w"] + 32)
        h = self.kw.get("height", M["door_h"] + 32)
        c = (self._span_center(a, axis) + self._span_center(b, axis)) / 2
        a.punch(fa, c - w / 2, c + w / 2, a.z0, a.z0 + h)
        b.punch(fb, c - w / 2, c + w / 2, b.z0, b.z0 + h)
        # curved header spanning the wall gap between the two shells
        if axis == "x":
            wall_x = a.x1 if sign > 0 else a.x0
            d1, d2 = wall_x - a.wall, (b.x0 if sign > 0 else b.x1) + b.wall
            lo, hi = sorted((d1, d2))
            pf = P.archway("y", c - w / 2, c + w / 2, lo, hi, a.z0, a.z0 + h,
                           a.theme)
        else:
            wall_y = a.y1 if sign > 0 else a.y0
            d1, d2 = wall_y - a.wall, (b.y0 if sign > 0 else b.y1) + b.wall
            lo, hi = sorted((d1, d2))
            pf = P.archway("x", c - w / 2, c + w / 2, lo, hi, a.z0, a.z0 + h,
                           a.theme)
        return pf.brushes, pf.ents

    def _emit_corridor(self):
        a, b = self.a, self.b
        fa, fb, axis, sign = self._facing()
        w = self.kw.get("width", M["corridor_w"])
        h = self.kw.get("height", M["corridor_headroom"])
        th = a.th
        c = (self._span_center(a, axis) + self._span_center(b, axis)) / 2
        z0 = a.z0
        a.punch(fa, c - w / 2, c + w / 2, z0, z0 + h)
        b.punch(fb, c - w / 2, c + w / 2, z0, z0 + h)
        T = a.wall
        brushes = []
        if axis == "x":
            g0 = (a.x1 if sign > 0 else b.x1)
            g1 = (b.x0 if sign > 0 else a.x0)
            g0, g1 = sorted((g0, g1))
            brushes.append(qgeo.box(g0, c - w / 2 - T, z0 - T, g1, c + w / 2 + T,
                                    z0, th["floor"]))
            brushes.append(qgeo.box(g0, c - w / 2 - T, z0 + h, g1, c + w / 2 + T,
                                    z0 + h + T, th["ceiling"]))
            brushes.append(qgeo.box(g0, c - w / 2 - T, z0, g1, c - w / 2, z0 + h,
                                    th["wall"]))
            brushes.append(qgeo.box(g0, c + w / 2, z0, g1, c + w / 2 + T, z0 + h,
                                    th["wall"]))
        else:
            g0 = (a.y1 if sign > 0 else b.y1)
            g1 = (b.y0 if sign > 0 else a.y0)
            g0, g1 = sorted((g0, g1))
            brushes.append(qgeo.box(c - w / 2 - T, g0, z0 - T, c + w / 2 + T, g1,
                                    z0, th["floor"]))
            brushes.append(qgeo.box(c - w / 2 - T, g0, z0 + h, c + w / 2 + T, g1,
                                    z0 + h + T, th["ceiling"]))
            brushes.append(qgeo.box(c - w / 2 - T, g0, z0, c - w / 2, g1, z0 + h,
                                    th["wall"]))
            brushes.append(qgeo.box(c + w / 2, g0, z0, c + w / 2 + T, g1, z0 + h,
                                    th["wall"]))
        ents = []
        mid = ((g0 + g1) / 2)
        if axis == "x":
            self._light_at(ents, mid, c, z0 + h - 16, a)
        else:
            self._light_at(ents, c, mid, z0 + h - 16, a)
        return brushes, ents

    def _light_at(self, ents, x, y, z, room):
        pf = P.light_fixture(x, y, z, room.theme)
        ents.extend((cn, k, br) for cn, k, br in pf.ents)

    def _emit_teleport(self):
        a, b = self.a, self.b
        name = self.kw.get("name", f"tp_{a.name}_{b.name}")
        ax = self.kw.get("a_at", (a.cx, a.cy))
        bx = self.kw.get("b_at", (b.cx, b.cy))
        pf = P.teleporter_pad(ax[0], ax[1], a.z0, a.theme, target=name,
                              dest_name=name, dest_origin=(bx[0], bx[1],
                                                           b.z0 + 24))
        return pf.brushes, pf.ents


class Layout:
    def __init__(self, message, theme, **props):
        self.message = message
        self.theme = theme
        self.props = props
        self.rooms = []
        self.conns = []

    def room(self, name, x0, y0, x1, y1, z0, z1, theme=None, **kw):
        r = Room(name, x0, y0, x1, y1, z0, z1, theme or self.theme, **kw)
        self.rooms.append(r)
        return r

    def connect(self, kind, a, b, **kw):
        c = Connection(kind, a, b, **kw)
        self.conns.append(c)
        return c

    def _navcheck(self):
        adj = {r.name: set() for r in self.rooms}
        for c in self.conns:
            adj[c.a.name].add(c.b.name)
            adj[c.b.name].add(c.a.name)
        # start from the room holding the first spawn, else room 0
        start = None
        for r in self.rooms:
            if any(cn[0].startswith("info_player") for cn in r._ents):
                start = r.name
                break
        if start is None and self.rooms:
            start = self.rooms[0].name
        seen, stack = {start}, [start]
        while stack:
            for n in adj[stack.pop()]:
                if n not in seen:
                    seen.add(n)
                    stack.append(n)
        missing = [r.name for r in self.rooms if r.name not in seen]
        if missing:
            raise ValueError(
                f"qlayout navcheck: rooms unreachable from '{start}': "
                f"{', '.join(missing)} — every room needs a connection path")

    def build(self):
        self._navcheck()
        m = qgeo.MapWriter(self.message, **{k: v for k, v in self.props.items()
                                            if k in ("wad", "worldtype",
                                                     "minlight", "sounds")})
        for k, v in self.props.items():
            if k not in ("wad", "worldtype", "minlight", "sounds"):
                m.props[k] = str(v)
        for c in self.conns:           # connections punch BEFORE rooms emit
            cb, ce = c.emit()
            m.brushes.extend(cb)
            for cn, keys, br in ce:
                m.ent(cn, brush=br, **keys)
        for r in self.rooms:
            rb, re = r.emit()
            m.brushes.extend(rb)
            for cn, keys, br in re:
                m.ent(cn, brush=br, **keys)
        return m

    def write(self, path):
        self.build().write(os.path.abspath(path))
