#!/usr/bin/env python3
"""Deterministic validators for NexMap-emitted .map files.

Runs between map-script execution and qbsp in the build pipeline.
Hard failures exit 1; warnings print but pass. Writes <map>.check.json
with machine-readable results plus suggested render cameras.

Usage: python3 tools/qcheck.py out/foo.map [--json out/foo.check.json]
"""
import itertools
import json
import math
import os
import statistics
import sys

_HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, _HERE)
sys.path.insert(0, os.path.join(_HERE, "..", "maps"))

from qcorpus import parse_map, outward_normal, family  # noqa: E402
from qtheme import METRICS, THEME_FAMILIES, FLOW_BANDS  # noqa: E402

EPS = 0.1

ITEM_PREFIXES = ("item_", "weapon_")
SPAWN_CLASSES = ("info_player_start", "info_player_deathmatch",
                 "info_player_coop")
NONSOLID_TEX = ("trigger", "clip", "skip", "hint")


class Brush:
    """Convex brush: planes with outward normals, enumerated vertices."""

    def __init__(self, faces):
        self.textures = [f[3] for f in faces]
        self.planes = []
        for p0, p1, p2, tex in faces:
            n = outward_normal(p0, p1, p2)
            d = n[0] * p0[0] + n[1] * p0[1] + n[2] * p0[2]
            self.planes.append((n, d, tex))
        self.verts = self._vertices()
        if self.verts:
            self.aabb = (min(v[0] for v in self.verts), min(v[1] for v in self.verts),
                         min(v[2] for v in self.verts), max(v[0] for v in self.verts),
                         max(v[1] for v in self.verts), max(v[2] for v in self.verts))
        else:
            self.aabb = None

    def _vertices(self):
        verts = []
        for (n1, d1, _), (n2, d2, _), (n3, d3, _) in itertools.combinations(self.planes, 3):
            det = (n1[0] * (n2[1] * n3[2] - n2[2] * n3[1])
                   - n1[1] * (n2[0] * n3[2] - n2[2] * n3[0])
                   + n1[2] * (n2[0] * n3[1] - n2[1] * n3[0]))
            if abs(det) < 1e-6:
                continue
            x = (d1 * (n2[1] * n3[2] - n2[2] * n3[1])
                 - n1[1] * (d2 * n3[2] - n2[2] * d3)
                 + n1[2] * (d2 * n3[1] - n2[1] * d3)) / det
            y = (n1[0] * (d2 * n3[2] - n2[2] * d3)
                 - d1 * (n2[0] * n3[2] - n2[2] * n3[0])
                 + n1[2] * (n2[0] * d3 - d2 * n3[0])) / det
            z = (n1[0] * (n2[1] * d3 - d2 * n3[1])
                 - n1[1] * (n2[0] * d3 - d2 * n3[0])
                 + d1 * (n2[0] * n3[1] - n2[1] * n3[0])) / det
            if all(n[0] * x + n[1] * y + n[2] * z <= d + EPS
                   for n, d, _ in self.planes):
                if not any(abs(x - v[0]) < EPS and abs(y - v[1]) < EPS
                           and abs(z - v[2]) < EPS for v in verts):
                    verts.append((x, y, z))
        return verts

    def flat_tops(self):
        """[(x0, y0, x1, y1, z, tex)] for each upward axis-flat face."""
        out = []
        for n, d, tex in self.planes:
            if n[2] < 0.999:
                continue
            on = [v for v in self.verts if abs(v[2] - d) < EPS]
            if len(on) >= 3:
                out.append((min(v[0] for v in on), min(v[1] for v in on),
                            max(v[0] for v in on), max(v[1] for v in on), d, tex))
        return out

    def flat_bottoms(self):
        out = []
        for n, d, tex in self.planes:
            if n[2] > -0.999:
                continue
            z = -d
            on = [v for v in self.verts if abs(v[2] - z) < EPS]
            if len(on) >= 3:
                out.append((min(v[0] for v in on), min(v[1] for v in on),
                            max(v[0] for v in on), max(v[1] for v in on), z, tex))
        return out

    def is_solid(self):
        t = self.textures[0]
        return not (t.startswith("*") or t.startswith("sky")
                    or any(t.startswith(p) for p in NONSOLID_TEX))


def rects_overlap(a, b, grow=0.0):
    return (a[0] - grow < b[2] and b[0] - grow < a[2]
            and a[1] - grow < b[3] and b[1] - grow < a[3])


def pt_rect_dist(x, y, r):
    dx = max(r[0] - x, 0, x - r[2])
    dy = max(r[1] - y, 0, y - r[3])
    return math.hypot(dx, dy)


class Checker:
    def __init__(self, path):
        self.path = path
        self.failures = []
        self.warnings = []
        ents = parse_map(path)
        self.world = ents[0][0]
        self.waived = set(self.world.get("_qcheck_waive", "").split())
        self.solids = []
        for keys, brushes in ents:
            cls = keys.get("classname", "")
            if brushes and (cls == "worldspawn" or cls.startswith("func_")):
                for b in brushes:
                    br = Brush(b)
                    if br.aabb and br.is_solid():
                        self.solids.append(br)
        self.tops = []          # walkable flat tops
        for br in self.solids:
            for t in br.flat_tops():
                if t[2] - t[0] >= 8 and t[3] - t[1] >= 8:
                    self.tops.append(t)
        self.points = []        # (classname, x, y, z, keys)
        for keys, brushes in ents:
            org = keys.get("origin", "").split()
            if len(org) == 3:
                self.points.append((keys.get("classname", ""),
                                    float(org[0]), float(org[1]), float(org[2]), keys))
        self.items = [p for p in self.points
                      if p[0].startswith(ITEM_PREFIXES)]
        self.spawns = [p for p in self.points if p[0] in SPAWN_CLASSES]
        self.ents = ents

    def fail(self, check, detail, pos=None):
        rec = {"check": check, "detail": detail, "pos": pos}
        if check in self.waived:
            rec["detail"] += " [waived]"
            self.warnings.append(rec)
        else:
            self.failures.append(rec)

    def warn(self, check, detail, pos=None):
        self.warnings.append({"check": check, "detail": detail, "pos": pos})

    def floor_under(self, x, y, z, radius=8):
        foot = (x - radius, y - radius, x + radius, y + radius)
        best = None
        for t in self.tops:
            if t[4] <= z + EPS and rects_overlap(foot, t):
                if best is None or t[4] > best[4]:
                    best = t
        return best

    # ---------------------------------------------------------- checks

    # Thresholds below are calibrated against id originals (dm4, dm6, e1m1):
    # id clusters ammo 24-48 apart, floats powerups up to 32 (droptofloor),
    # and tucks items 8-45 from walls. Stricter values flood false positives.

    def check_item_floor_gap(self):
        for cls, x, y, z, _ in self.items + self.spawns:
            floor = self.floor_under(x, y, z)
            limit = 64 if cls in SPAWN_CLASSES else 48
            pos = f"({x:g} {y:g} {z:g})"
            if floor is None:
                self.fail("item_floor_gap", f"{cls} at {pos} has no floor below", pos)
                continue
            gap = z - floor[4]
            if gap < -EPS:
                self.fail("item_floor_gap",
                          f"{cls} at {pos} embedded {-gap:.0f} into floor", pos)
            elif gap > limit:
                self.fail("item_floor_gap",
                          f"{cls} at {pos} floats {gap:.0f} above floor (max {limit})", pos)

    def check_entity_spacing(self):
        SKILL_BITS = 256 | 512 | 1024
        pts = self.items + self.spawns
        for a, b in itertools.combinations(pts, 2):
            if abs(a[3] - b[3]) > 64:
                continue
            fa = int(a[4].get("spawnflags", 0)) & SKILL_BITS
            fb = int(b[4].get("spawnflags", 0)) & SKILL_BITS
            if fa != fb:
                continue  # co-located difficulty variants are id practice
            d = math.hypot(a[1] - b[1], a[2] - b[2])
            where = f"{a[0]} and {b[0]} only {d:.0f} apart at " \
                    f"({a[1]:g} {a[2]:g} {a[3]:g})"
            if d < 16:
                self.fail("entity_spacing", where)
            elif d < 24:
                self.warn("entity_spacing", where)

    def check_wall_margin(self):
        for cls, x, y, z, _ in self.items + self.spawns:
            for br in self.solids:
                a = br.aabb
                if a[5] < z + 8 or a[2] > z + METRICS["player_h"]:
                    continue  # not wall-like at item height
                d = pt_rect_dist(x, y, (a[0], a[1], a[3], a[4]))
                if 0 < d < 16:
                    self.warn("wall_margin",
                              f"{cls} at ({x:g} {y:g} {z:g}) is {d:.0f} from a wall")
                    break

    def check_headroom(self):
        for cls, x, y, z, _ in self.items + self.spawns:
            floor = self.floor_under(x, y, z)
            if floor is None:
                continue
            foot = (x - 16, y - 16, x + 16, y + 16)
            ceil = None
            for br in self.solids:
                for b in br.flat_bottoms():
                    if b[4] > floor[4] + EPS and rects_overlap(foot, b):
                        if ceil is None or b[4] < ceil:
                            ceil = b[4]
            if ceil is not None and ceil - floor[4] < 64:
                self.warn("headroom",
                          f"{cls} at ({x:g} {y:g} {z:g}) has only "
                          f"{ceil - floor[4]:.0f} headroom")

    def _step_edges(self):
        edges = {}
        for i, a in enumerate(self.tops):
            for j, b in enumerate(self.tops):
                if i == j:
                    continue
                dz = b[4] - a[4]
                if 8 - EPS <= dz <= 24 + EPS and rects_overlap(a, b, grow=2.0):
                    edges.setdefault(i, []).append(j)
        return edges

    def check_stair_landing(self):
        edges = self._step_edges()
        has_in = {j for outs in edges.values() for j in outs}
        for i, outs in edges.items():
            if i in has_in:
                continue
            chain = [i]
            cur = i
            while cur in edges:
                cur = edges[cur][0]
                chain.append(cur)
                if len(chain) > 64:
                    break
            if len(chain) < 4:
                continue
            bot, nxt = self.tops[chain[0]], self.tops[chain[1]]
            axis = 0 if abs((nxt[0] + nxt[2]) - (bot[0] + bot[2])) > \
                abs((nxt[1] + nxt[3]) - (bot[1] + bot[3])) else 1
            depth = (bot[2] - bot[0]) if axis == 0 else (bot[3] - bot[1])
            if depth >= 48:
                continue
            ok = False
            for t in self.tops:
                if abs(t[4] - bot[4]) <= METRICS["max_climb"] + 16 and \
                        rects_overlap(bot, t, grow=2.0) and t is not bot:
                    td = (t[2] - t[0]) if axis == 0 else (t[3] - t[1])
                    if td >= 48:
                        ok = True
                        break
            if not ok:
                self.warn("stair_landing",
                          f"stair run of {len(chain)} steps starting at "
                          f"({bot[0]:g} {bot[1]:g} {bot[4]:g}) lacks a bottom landing")

    def check_reachability(self):
        nodes = [t for t in self.tops
                 if t[2] - t[0] >= 16 and t[3] - t[1] >= 16]
        if not nodes or not self.spawns:
            return
        adj = {i: set() for i in range(len(nodes))}
        for i, a in enumerate(nodes):
            for j, b in enumerate(nodes):
                if i == j or not rects_overlap(a, b, grow=18.0):
                    continue
                dz = b[4] - a[4]
                if dz <= METRICS["max_climb"]:   # up to +18, or any drop
                    adj[i].add(j)
        # teleporters
        dests = {k.get("targetname"): (cls, x, y, z) for cls, x, y, z, k in self.points
                 if cls == "info_teleport_destination"}
        for keys, brushes in self.ents:
            if keys.get("classname") == "trigger_teleport" and brushes:
                dst = dests.get(keys.get("target"))
                if not dst:
                    continue
                tb = Brush(brushes[0]).aabb
                dn = self._node_at(nodes, dst[1], dst[2], dst[3])
                for i, n in enumerate(nodes):
                    if tb and rects_overlap(n, (tb[0], tb[1], tb[3], tb[4])) and \
                            abs(n[4] - tb[2]) < 64 and dn is not None:
                        adj[i].add(dn)

        def node_for(x, y, z):
            return self._node_at(nodes, x, y, z)

        start = node_for(*self.spawns[0][1:4])
        if start is None:
            self.warn("reachability", "spawn has no walkable node under it")
            return
        seen = {start}
        queue = [start]
        while queue:
            cur = queue.pop()
            for j in adj[cur]:
                if j not in seen:
                    seen.add(j)
                    queue.append(j)
        missing = []
        for cls, x, y, z, _ in self.items + self.spawns:
            n = node_for(x, y, z)
            if n is not None and n not in seen:
                missing.append(f"{cls} ({x:g} {y:g} {z:g})")
        if missing:
            head = ", ".join(missing[:4])
            more = f" +{len(missing) - 4} more" if len(missing) > 4 else ""
            self.warn("reachability",
                      f"{len(missing)} entities unreachable from spawn 0 "
                      f"(doors/lifts/water not modeled): {head}{more}")

    def _node_at(self, nodes, x, y, z):
        best = None
        for i, t in enumerate(nodes):
            if t[4] <= z + EPS and rects_overlap((x - 8, y - 8, x + 8, y + 8), t):
                if best is None or t[4] > nodes[best][4]:
                    best = i
        return best

    def check_lights(self):
        vals = []
        for cls, _, _, _, keys in self.points:
            if cls == "light" or cls.startswith("light_"):
                vals.append(float(keys.get("light", 300)))
        area = sum((t[2] - t[0]) * (t[3] - t[1]) for t in self.tops)
        want = max(30, int(area / 60000))
        if len(vals) < want:
            self.warn("light_stats",
                      f"{len(vals)} light entities for {area / 1e6:.1f}M units^2 "
                      f"walkable area; corpus pacing suggests >= {want}")
        if vals:
            med = statistics.median(vals)
            if not 140 <= med <= 320:
                self.warn("light_stats",
                          f"median light value {med:.0f} outside corpus band 140-320")
        minlight = float(self.world.get("_minlight", 0))
        if minlight > 30:
            self.warn("light_stats",
                      f"_minlight {minlight:.0f} flattens shadows (keep <= 30)")

    # ---------------------------------------------------------- flow

    def _nav_nodes(self):
        return [t for t in self.tops
                if t[2] - t[0] >= 16 and t[3] - t[1] >= 16]

    def _seg_clear(self, x0, y0, x1, y1, z):
        """True if the horizontal segment at height z misses every solid brush
        (AABB slab clip at eye height) — a cheap sightline test."""
        dx, dy = x1 - x0, y1 - y0
        for br in self.solids:
            a = br.aabb
            if a is None or a[2] > z or a[5] < z:
                continue
            tmin, tmax = 0.0, 1.0
            hit = True
            for p, d, lo, hi in ((x0, dx, a[0], a[3]), (y0, dy, a[1], a[4])):
                if abs(d) < 1e-9:
                    if p < lo or p > hi:
                        hit = False
                        break
                else:
                    t0, t1 = (lo - p) / d, (hi - p) / d
                    if t0 > t1:
                        t0, t1 = t1, t0
                    tmin, tmax = max(tmin, t0), min(tmax, t1)
                    if tmin > tmax:
                        hit = False
                        break
            if hit and tmin <= tmax:
                return False
        return True

    def flow_metrics(self, sightlines=False):
        """Topology of the walkable nav graph: loops, dead ends, elevation,
        sightlines. The same node/adjacency model the reachability check uses."""
        nodes = self._nav_nodes()
        n = len(nodes)
        out = {"nodes": n}
        if n == 0:
            return out
        adj = [set() for _ in range(n)]
        edges = 0
        for i in range(n):
            ai = nodes[i]
            for j in range(i + 1, n):
                bj = nodes[j]
                if abs(bj[4] - ai[4]) <= METRICS["max_climb"] and \
                        rects_overlap(ai, bj, grow=18.0):
                    adj[i].add(j)
                    adj[j].add(i)
                    edges += 1
        seen = [False] * n
        comps = []
        for s in range(n):
            if seen[s]:
                continue
            stack, comp = [s], []
            seen[s] = True
            while stack:
                u = stack.pop()
                comp.append(u)
                for v in adj[u]:
                    if not seen[v]:
                        seen[v] = True
                        stack.append(v)
            comps.append(comp)
        comps.sort(key=len, reverse=True)
        main = comps[0]
        out.update({
            "edges": edges,
            "components": len(comps),
            "loops": edges - n + len(comps),
            "loop_density": round((edges - n + len(comps)) / n, 3),
            "dead_end_ratio": round(sum(1 for i in main if len(adj[i]) <= 1)
                                    / len(main), 3),
            "coverage": round(len(main) / n, 3),
        })
        levels = sorted({round(nodes[i][4] / 16) * 16 for i in main})
        out["elevation_levels"] = len(levels)
        out["elevation_span"] = (max(levels) - min(levels)) if levels else 0
        if sightlines and len(main) > 2:
            cen = [((nodes[i][0] + nodes[i][2]) / 2,
                    (nodes[i][1] + nodes[i][3]) / 2, nodes[i][4] + 40)
                   for i in main]
            step = max(1, len(cen) // 60)
            dists = []
            for a in range(0, len(cen), step):
                best = 0.0
                for b in range(0, len(cen), step):
                    if a != b and self._seg_clear(cen[a][0], cen[a][1],
                                                  cen[b][0], cen[b][1], cen[a][2]):
                        best = max(best, math.hypot(cen[b][0] - cen[a][0],
                                                    cen[b][1] - cen[a][1]))
                dists.append(best)
            if dists:
                dists.sort()
                out["sightline_p50"] = int(statistics.median(dists))
                out["sightline_p90"] = int(dists[min(len(dists) - 1,
                                                     int(len(dists) * 0.9))])
        return out

    def check_flow(self):
        """Warn on dead-end-heavy (spur-tree) layouts — the one early-feedback
        signal not covered by the post-compile navmesh feature gate.

        NOTE: loop_density was removed as a gate — it's a misleading descriptor
        that rates over-connected blobs HIGHEST (the unplayable Well topped it),
        contradicting the navmesh occlusion/chokepoint gate. "Loops, not trees"
        stays a design principle, but flow/structure quality is owned by
        `mapfeatures.py --gate` (occlusion, scale, chokepoints, verticality,
        sightline variety), not by a brush-top loop count."""
        fm = self.flow_metrics()
        if fm.get("nodes", 0) < 6:
            return
        de = fm["dead_end_ratio"]
        if de > FLOW_BANDS["dead_end_ratio"]["warn_above"]:
            self.warn("flow", f"dead-end-heavy: {de*100:.0f}% of the main path "
                      f"is dead ends (id <= 19%; rooms need >=2 exits)")
        if fm.get("elevation_levels", 0) < FLOW_BANDS["elevation_levels"]["warn_below"] \
                and fm.get("elevation_span", 0) < 64:
            self.warn("flow", f"flat: {fm.get('elevation_levels')} floor levels "
                      f"(id median {FLOW_BANDS['elevation_levels']['median']}; "
                      f"vary heights for interest)")

    def check_theme(self):
        counts = {}
        total = 0
        for keys, brushes in self.ents:
            for b in brushes:
                for _, _, _, tex in b:
                    total += 1
                    counts[tex] = counts.get(tex, 0) + 1
        if not total:
            return
        best, cover = None, 0.0
        for theme, fams in THEME_FAMILIES.items():
            n = sum(c for tex, c in counts.items()
                    if any(tex.startswith(f) or family(tex) == f for f in fams))
            if n / total > cover:
                best, cover = theme, n / total
        if cover < 0.9:
            off = sorted((c, t) for t, c in counts.items()
                         if not any(t.startswith(f) or family(t) == f
                                    for f in THEME_FAMILIES[best]))[-3:]
            self.warn("theme_coherence",
                      f"best theme '{best}' covers only {cover * 100:.0f}% of faces; "
                      f"top off-palette: {[t for _, t in off]}")

    # ---------------------------------------------------------- cams

    def suggest_cams(self):
        cams = []
        if not self.tops:
            return ""
        cx = sum(t[0] + t[2] for t in self.tops) / (2 * len(self.tops))
        cy = sum(t[1] + t[3] for t in self.tops) / (2 * len(self.tops))
        targets = list(self.items)
        for keys, brushes in self.ents:
            if keys.get("classname") == "trigger_teleport" and brushes:
                a = Brush(brushes[0]).aabb
                if a:
                    targets.append(("teleporter", (a[0] + a[3]) / 2,
                                    (a[1] + a[4]) / 2, a[2], {}))
        for cls, x, y, z, _ in targets:
            dx, dy = cx - x, cy - y
            dl = math.hypot(dx, dy) or 1.0
            px, py = x + dx / dl * 200, y + dy / dl * 200
            pz = z + 56
            yaw = math.degrees(math.atan2(y - py, x - px)) % 360
            pitch = math.degrees(math.atan2(z - pz, 200))
            cams.append(f"{px:.0f} {py:.0f} {pz:.0f} {yaw:.0f} {pitch:.0f}")
        return ";".join(cams[:8])

    # ---------------------------------------------------------- driver

    def run(self):
        self.check_item_floor_gap()
        self.check_entity_spacing()
        self.check_wall_margin()
        self.check_headroom()
        self.check_stair_landing()
        self.check_reachability()
        self.check_lights()
        self.check_theme()
        self.check_flow()
        return {"failures": self.failures, "warnings": self.warnings,
                "cams": self.suggest_cams()}


def main():
    args = sys.argv[1:]
    json_path = None
    if "--json" in args:
        i = args.index("--json")
        json_path = args[i + 1]
        del args[i:i + 2]
    if len(args) != 1:
        print(__doc__)
        sys.exit(2)
    path = args[0]
    c = Checker(path)
    result = c.run()
    if json_path is None:
        json_path = os.path.splitext(path)[0] + ".check.json"
    with open(json_path, "w") as f:
        json.dump(result, f, indent=1)
    name = os.path.basename(path)
    for w in result["warnings"]:
        print(f"qcheck WARN  [{w['check']}] {w['detail']}")
    for fl in result["failures"]:
        print(f"qcheck FAIL  [{fl['check']}] {fl['detail']}")
    n_f, n_w = len(result["failures"]), len(result["warnings"])
    print(f"qcheck: {name}: {n_f} failures, {n_w} warnings -> {json_path}")
    sys.exit(1 if result["failures"] else 0)


if __name__ == "__main__":
    main()
