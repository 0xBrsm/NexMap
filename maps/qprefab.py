"""Validated, parametric prefab library for NexMap authoring.

Each builder returns a Prefab bundling world brushes AND the entities that
belong with them (lights paired with fixtures, items seated on pedestals).
The point is to fix each recurring failure mode once, in the component:

  - flat lighting        -> light_fixture always pairs a visible source + light
  - boxy geometry        -> archway, ceiling_vault, chamfered_corner add curves
  - missing landings     -> stairs_landed guarantees a bottom landing + splits
                            long flights at max_stair_rise
  - floating items       -> item_pedestal seats the item at floor+gap exactly
  - harsh lines          -> chamfered_corner / curved_wall break 90-degree edges

Dimensions come from qtheme.METRICS, textures/lights from qtheme.THEMES.
Build a gallery with maps/_prefab_gallery.py to eyeball every component.
"""
from dataclasses import dataclass, field

import qgeo
from qtheme import METRICS as M
from qtheme import THEMES


@dataclass
class Prefab:
    brushes: list = field(default_factory=list)
    ents: list = field(default_factory=list)   # (classname, keys_dict, brush|None)
    anchors: dict = field(default_factory=dict)  # named readback points, e.g. item origin

    def add(self, b):
        if isinstance(b, list):
            self.brushes.extend(b)
        else:
            self.brushes.append(b)

    def ent(self, classname, brush=None, **keys):
        self.ents.append((classname, keys, brush))

    def merge(self, other, into=None):
        self.brushes.extend(other.brushes)
        self.ents.extend(other.ents)
        return self


def _light_value(theme, override=None):
    return override if override is not None else THEMES[theme]["light_value"]


# --------------------------------------------------------------- fixtures

def light_fixture(x, y, z, theme, value=None, wall_normal=None):
    """A VISIBLE light source paired with its light entity, so no surface is
    lit by an invisible point light. Themes with a torch classname emit the
    torch (it carries its own flame light); panel themes emit a recessed
    fullbright panel brush plus a point light just in front of it.

    wall_normal: unit (nx, ny) the fixture faces away from, for panel recess
    depth; defaults to a free-standing panel."""
    th = THEMES[theme]
    pf = Prefab()
    val = _light_value(theme, value)
    if th["torch"]:
        pf.ent(th["torch"], origin=f"{x:g} {y:g} {z:g}")
        return pf
    if th.get("fluoro"):
        pf.ent("light_fluoro", origin=f"{x:g} {y:g} {z:g}", light=val,
               style="0")
        # keep the visible tube as a thin emissive box
        pf.add(qgeo.box(x - 32, y - 4, z - 4, x + 32, y + 4, z + 4,
                        th["panel"]))
        return pf
    panel = th["panel"] or th["trim"]
    nx, ny = wall_normal or (0.0, 1.0)
    cx, cy = x - nx * 2, y - ny * 2
    if abs(nx) > abs(ny):
        pf.add(qgeo.box(cx - 4, y - 16, z - 16, cx + 4, y + 16, z + 16, panel))
    else:
        pf.add(qgeo.box(x - 16, cy - 4, z - 16, x + 16, cy + 4, z + 16, panel))
    pf.ent("light", origin=f"{x - nx * 12:g} {y - ny * 12:g} {z:g}",
           light=val)
    return pf


def pillar(cx, cy, z_base, z_top, theme, w=None, round_=False):
    """Structural column with a wider base and cap so it never reads as a bare
    stick. round_=True uses an octagonal shaft (curves)."""
    th = THEMES[theme]
    w = w or M["pillar_w"]
    bw = M["pillar_base_w"]
    bh = M["pillar_base_h"]
    pf = Prefab()
    pf.add(qgeo.box(cx - bw / 2, cy - bw / 2, z_base,
                    cx + bw / 2, cy + bw / 2, z_base + bh, th["trim"]))
    if round_:
        pf.add(qgeo.cylinder(cx, cy, w / 2, z_base + bh, z_top - bh,
                             th["pier"], sides=8))
    else:
        pf.add(qgeo.box(cx - w / 2, cy - w / 2, z_base + bh,
                        cx + w / 2, cy + w / 2, z_top - bh, th["pier"]))
    pf.add(qgeo.box(cx - bw / 2, cy - bw / 2, z_top - bh,
                    cx + bw / 2, cy + bw / 2, z_top, th["trim"]))
    return pf


# --------------------------------------------------------------- items

def item_pedestal(x, y, z_floor, classname, theme, height=16, spawnflags=None,
                  lit=True):
    """Seat an item on a short pedestal with its origin at the correct height
    above the pedestal top (METRICS item_z), killing the floating-item bug.
    Optionally drops a soft light so the pickup reads in a dark room."""
    th = THEMES[theme]
    pf = Prefab()
    pf.add(qgeo.box(x - 16, y - 16, z_floor, x + 16, y + 16, z_floor + height,
                    th["trim"], zp=th["pier"]))
    iz = z_floor + height + M["item_z"]
    keys = {"origin": f"{x:g} {y:g} {iz:g}"}
    if spawnflags is not None:
        keys["spawnflags"] = spawnflags
    pf.ent(classname, **keys)
    pf.anchors["item"] = (x, y, iz)
    if lit:
        pf.ent("light", origin=f"{x:g} {y:g} {iz + 48:g}",
               light=int(_light_value(theme) * 0.7))
    return pf


def teleporter_pad(x, y, z_floor, theme, target, dest_name=None,
                   dest_origin=None, dest_angle="0", size=64):
    """A teleport trigger volume on a marked pad. Pairs the *teleport-textured
    trigger with an info_teleport_destination so qcheck reachability links the
    two. Provide dest_origin/dest_name to also emit the destination."""
    th = THEMES[theme]
    h = size / 2
    pf = Prefab()
    pf.add(qgeo.box(x - h, y - h, z_floor - 8, x + h, y + h, z_floor,
                    th["trim"], zp=th["pier"]))
    trig = qgeo.box(x - h, y - h, z_floor, x + h, y + h, z_floor + 80,
                    "*teleport")
    pf.ent("trigger_teleport", brush=trig, target=target)
    pf.ent("light", origin=f"{x:g} {y:g} {z_floor + 48:g}",
           light=int(_light_value(theme) * 0.8), _color="0.4 0.5 1.0")
    if dest_origin is not None:
        ox, oy, oz = dest_origin
        pf.ent("info_teleport_destination",
               origin=f"{ox:g} {oy:g} {oz:g}", angle=dest_angle,
               targetname=dest_name or target)
    return pf


# --------------------------------------------------------------- stairs

def stairs_landed(x1, y1, x2, y2, z_base, z_top, axis, direction, theme,
                  step_h=None, bottom_landing=True):
    """Straight stairs that ALWAYS get a bottom landing and split into flights
    separated by mid-landings whenever the unbroken rise exceeds
    METRICS max_stair_rise. Footprint runs x1..x2 / y1..y2; the run axis is
    `axis` ascending toward `direction` ("+"/"-")."""
    th = THEMES[theme]
    step_h = step_h or M["step_h"]
    land_d = M["landing_depth"]
    pf = Prefab()
    perp = (y1, y2) if axis == "x" else (x1, x2)
    a_lo, a_hi = (x1, x2) if axis == "x" else (y1, y2)

    total = z_top - z_base
    n_steps = max(1, round(total / step_h))
    per_flight = max(1, M["max_stair_rise"] // step_h)   # steps before landing

    # plan flights: list of step counts
    flights = []
    rem = n_steps
    while rem > 0:
        k = min(per_flight, rem)
        flights.append(k)
        rem -= k
    n_landings = (1 if bottom_landing else 0) + (len(flights) - 1)
    run_span = (a_hi - a_lo) - n_landings * land_d
    if run_span <= 0:
        run_span = (a_hi - a_lo) * 0.6
        land_d = ((a_hi - a_lo) - run_span) / max(1, n_landings)
    run = run_span / n_steps

    sign = 1 if direction == "+" else -1
    cur = a_lo if direction == "+" else a_hi
    z = z_base

    def seg(a_start, a_end, zb, zt, tex):
        lo, hi = min(a_start, a_end), max(a_start, a_end)
        if axis == "x":
            return qgeo.box(lo, perp[0], zb, hi, perp[1], zt, tex,
                            zp=th["floor"])
        return qgeo.box(perp[0], lo, zb, perp[1], hi, zt, tex, zp=th["floor"])

    if bottom_landing:
        nxt = cur + sign * land_d
        pf.add(seg(cur, nxt, z_base, z_base + 4, th["floor"]))  # flush pad lip
        cur = nxt

    step_no = 0
    for fi, k in enumerate(flights):
        if fi > 0:  # mid landing at current height
            nxt = cur + sign * land_d
            pf.add(seg(cur, nxt, z_base, z, th["wall"]))
            cur = nxt
        for _ in range(k):
            step_no += 1
            ztop = z_base + step_h * step_no
            nxt = cur + sign * run
            pf.add(seg(cur, nxt, z_base, ztop, th["wall"]))
            cur = nxt
            z = ztop
    return pf


# --------------------------------------------------------------- curves / walls

def archway(axis, a1, a2, depth1, depth2, z_base, z_top, theme, rise=None,
            jamb_w=16):
    """A curved-soffit opening in a wall: arch header (qgeo.arch) plus jamb
    columns framing the sides. Breaks the boxy-doorway look with a real curve.
    Opening spans a1..a2 along `axis`; wall runs depth1..depth2 across."""
    th = THEMES[theme]
    rise = rise if rise is not None else min((a2 - a1) / 2, (z_top - z_base) * 0.5)
    pf = Prefab()
    spring = z_top - rise
    pf.add(qgeo.arch(axis, a1, a2, depth1, depth2, spring, rise, z_top,
                     th["wall"], segments=6))
    # jambs
    if axis == "x":
        pf.add(qgeo.box(a1 - jamb_w, depth1, z_base, a1, depth2, z_top,
                        th["pier"]))
        pf.add(qgeo.box(a2, depth1, z_base, a2 + jamb_w, depth2, z_top,
                        th["pier"]))
    else:
        pf.add(qgeo.box(depth1, a1 - jamb_w, z_base, depth2, a1, z_top,
                        th["pier"]))
        pf.add(qgeo.box(depth1, a2, z_base, depth2, a2 + jamb_w, z_top,
                        th["pier"]))
    return pf


def chamfered_corner(cx, cy, z1, z2, theme, size=32, quadrant="++"):
    """A 45-degree fillet that softens an inside corner. quadrant picks which
    way the diagonal faces ("++","+-","-+","--" = +x+y, +x-y, ...)."""
    th = THEMES[theme]
    sx = 1 if quadrant[0] == "+" else -1
    sy = 1 if quadrant[1] == "+" else -1
    p = [(cx, cy), (cx + sx * size, cy), (cx, cy + sy * size)]
    if sx * sy < 0:
        p = [(cx, cy), (cx, cy + sy * size), (cx + sx * size, cy)]
    pf = Prefab()
    pf.add(qgeo.prism(p, "z", z1, z2, th["wall"]))
    return pf


def curved_wall(cx, cy, r, z1, z2, theme, a0=0.0, a1=90.0, thick=16, segs=6):
    """A swept curved wall (arc) built from short box segments along an arc of
    radius r. Replaces a hard right-angle wall join with a sweep."""
    import math
    th = THEMES[theme]
    pf = Prefab()
    n = segs
    for i in range(n):
        t0 = math.radians(a0 + (a1 - a0) * i / n)
        t1 = math.radians(a0 + (a1 - a0) * (i + 1) / n)
        p = [
            (cx + r * math.cos(t0), cy + r * math.sin(t0)),
            (cx + r * math.cos(t1), cy + r * math.sin(t1)),
            (cx + (r + thick) * math.cos(t1), cy + (r + thick) * math.sin(t1)),
            (cx + (r + thick) * math.cos(t0), cy + (r + thick) * math.sin(t0)),
        ]
        pf.add(qgeo.prism(p, "z", z1, z2, th["wall"]))
    return pf


def wainscot(x1, y1, x2, y2, z_base, theme, height=None, depth=4):
    """A trim band proud of a wall base, running the rectangle perimeter
    footprint edge x1..x2 / y1..y2. Adds horizontal relief to flat walls."""
    th = THEMES[theme]
    height = height or M["recess_h"]
    pf = Prefab()
    pf.add(qgeo.box(x1, y1, z_base, x2, y1 + depth, z_base + height, th["trim"]))
    pf.add(qgeo.box(x1, y2 - depth, z_base, x2, y2, z_base + height, th["trim"]))
    pf.add(qgeo.box(x1, y1, z_base, x1 + depth, y2, z_base + height, th["trim"]))
    pf.add(qgeo.box(x2 - depth, y1, z_base, x2, y2, z_base + height, th["trim"]))
    return pf


def window_embrasure(axis, a1, a2, wall_lo, wall_hi, z_sill, z_head, theme,
                     liquid_glow=False):
    """A recessed window: two jamb blocks and a sill/head, leaving a clear
    opening a1..a2 between z_sill..z_head through a wall spanning
    wall_lo..wall_hi on the perpendicular axis. Optional backlight for mood."""
    th = THEMES[theme]
    pf = Prefab()
    if axis == "x":
        pf.add(qgeo.box(a1 - 8, wall_lo, z_sill, a1, wall_hi, z_head, th["pier"]))
        pf.add(qgeo.box(a2, wall_lo, z_sill, a2 + 8, wall_hi, z_head, th["pier"]))
    else:
        pf.add(qgeo.box(wall_lo, a1 - 8, z_sill, wall_hi, a1, z_head, th["pier"]))
        pf.add(qgeo.box(wall_lo, a2, z_sill, wall_hi, a2 + 8, z_head, th["pier"]))
    if liquid_glow:
        mid = (a1 + a2) / 2
        wm = (wall_lo + wall_hi) / 2
        pf.ent("light", origin=f"{mid if axis=='x' else wm:g} "
               f"{wm if axis=='x' else mid:g} {(z_sill + z_head) / 2:g}",
               light=int(_light_value(theme) * 0.6), _color="0.5 0.7 1.0")
    return pf


def walkway(x1, y1, x2, y2, z, theme, thick=16, railing=True, rail_h=32):
    """An elevated catwalk slab with optional low railing curbs on its long
    edges. Gives vertical play and sightline variety."""
    th = THEMES[theme]
    pf = Prefab()
    pf.add(qgeo.box(x1, y1, z - thick, x2, y2, z, th["floor"], zp=th["floor"]))
    if railing:
        run_x = (x2 - x1) >= (y2 - y1)
        if run_x:
            pf.add(qgeo.box(x1, y1, z, x2, y1 + 8, z + rail_h, th["trim"]))
            pf.add(qgeo.box(x1, y2 - 8, z, x2, y2, z + rail_h, th["trim"]))
        else:
            pf.add(qgeo.box(x1, y1, z, x1 + 8, y2, z + rail_h, th["trim"]))
            pf.add(qgeo.box(x2 - 8, y1, z, x2, y2, z + rail_h, th["trim"]))
    return pf


def ceiling_vault(x1, y1, x2, y2, z_spring, theme, rise=None, axis="x",
                  segments=6):
    """A barrel-vaulted ceiling over a room: arch brushes mirrored so the room
    is capped by a curve instead of a flat lid. Springs from z_spring at the
    walls up to z_spring+rise at the crown. Kills the box-with-a-lid look."""
    th = THEMES[theme]
    span = (x2 - x1) if axis == "x" else (y2 - y1)
    rise = rise if rise is not None else span / 4
    pf = Prefab()
    if axis == "x":
        # arch spans x; wall depth across y; cap upward
        pf.add(qgeo.arch("x", x1, x2, y1, y2, z_spring, rise,
                         z_spring + rise + 8, th["ceiling"], segments=segments))
    else:
        pf.add(qgeo.arch("y", y1, y2, x1, x2, z_spring, rise,
                         z_spring + rise + 8, th["ceiling"], segments=segments))
    return pf
