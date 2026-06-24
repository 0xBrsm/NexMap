#!/usr/bin/env python3
"""Flow / topology analysis for .map files.

Where qcheck grades placement (items, lights, stairs), qflow grades *movement*:
does the walkable space form loops you can run, or a tree of dead ends? It
reuses qcheck's walkable-surface graph (the same nodes/adjacency the
reachability check already builds), so it works on any .map — the id corpus
and our own maps alike.

Metrics (all from the walkable nav graph):
  nodes/edges      walkable surfaces and the steps/walks between them
  loops            cyclomatic number E - N + components: independent circuits.
                   A tree has 0; arenas you can run rings around have many.
  loop_density     loops / nodes — scale-free "how loopy"
  dead_end_ratio   fraction of degree<=1 nodes in the main component
  coverage         largest component / all nodes (fragmentation)
  elevation        distinct 16u floor levels, and total vertical span
  sightline_p50/p90  open horizontal distance sampled from node centers

Usage:
  python3 tools/qflow.py out/foo.map [--json out/foo.flow.json]
  python3 tools/qflow.py corpus [--source id]      # bands across the corpus
"""
import json
import math
import os
import statistics
import sys

_HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, _HERE)
sys.path.insert(0, os.path.join(_HERE, "..", "maps"))

from qcheck import Checker, rects_overlap          # noqa: E402
from qtheme import METRICS as M                     # noqa: E402

CLIMB = M["max_climb"]


def _nodes(chk):
    return [t for t in chk.tops if t[2] - t[0] >= 16 and t[3] - t[1] >= 16]


def _ray_clear(chk, x0, y0, x1, y1, z):
    """True if the horizontal segment at height z doesn't pass through a solid
    brush (cheap AABB-slab test at the eye height z)."""
    for br in chk.solids:
        a = br.aabb
        if a is None or a[2] > z or a[5] < z:
            continue
        # segment vs axis box in 2D (slab clip)
        tmin, tmax = 0.0, 1.0
        dx, dy = x1 - x0, y1 - y0
        for p, d, lo, hi in ((x0, dx, a[0], a[3]), (y0, dy, a[1], a[4])):
            if abs(d) < 1e-9:
                if p < lo or p > hi:
                    break
            else:
                t0, t1 = (lo - p) / d, (hi - p) / d
                if t0 > t1:
                    t0, t1 = t1, t0
                tmin, tmax = max(tmin, t0), min(tmax, t1)
                if tmin > tmax:
                    break
        else:
            if tmin <= tmax:
                return False
    return True


def analyze(path, sightlines=True):
    chk = Checker(path)
    nodes = _nodes(chk)
    n = len(nodes)
    out = {"map": os.path.basename(path), "nodes": n}
    if n == 0:
        return out

    # undirected edges: overlapping footprints within a single step in height
    adj = [set() for _ in range(n)]
    edges = 0
    for i in range(n):
        ai = nodes[i]
        for j in range(i + 1, n):
            bj = nodes[j]
            if abs(bj[4] - ai[4]) <= CLIMB and rects_overlap(ai, bj, grow=18.0):
                adj[i].add(j)
                adj[j].add(i)
                edges += 1

    # connected components + largest
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
    c = len(comps)
    loops = edges - n + c
    main_set = set(main)
    dead = sum(1 for i in main if len(adj[i]) <= 1)

    out.update({
        "edges": edges,
        "components": c,
        "loops": loops,
        "loop_density": round(loops / n, 3),
        "dead_end_ratio": round(dead / len(main), 3),
        "coverage": round(len(main) / n, 3),
    })

    levels = sorted({round(nodes[i][4] / 16) * 16 for i in main})
    out["elevation_levels"] = len(levels)
    out["elevation_span"] = (max(levels) - min(levels)) if levels else 0

    if sightlines and len(main) > 2:
        cen = [( (nodes[i][0] + nodes[i][2]) / 2,
                 (nodes[i][1] + nodes[i][3]) / 2, nodes[i][4] + 40)
               for i in main]
        dists = []
        step = max(1, len(cen) // 60)          # cap the sample for speed
        for a in range(0, len(cen), step):
            best = 0.0
            for b in range(0, len(cen), step):
                if a == b:
                    continue
                if _ray_clear(chk, cen[a][0], cen[a][1], cen[b][0], cen[b][1],
                              cen[a][2]):
                    best = max(best, math.hypot(cen[b][0] - cen[a][0],
                                                cen[b][1] - cen[a][1]))
            dists.append(best)
        if dists:
            dists.sort()
            out["sightline_p50"] = int(statistics.median(dists))
            out["sightline_p90"] = int(dists[min(len(dists) - 1,
                                                  int(len(dists) * 0.9))])
    return out


def _corpus(source="id"):
    import sqlite3
    db = os.path.expanduser("~/.cache/nexmap/corpus.db")
    src = os.path.expanduser("~/.cache/nexmap/quake_map_source")
    names = []
    if os.path.exists(db):
        con = sqlite3.connect(db)
        try:
            q = "select name from maps where source=? and bsp_format!='bsp-only'"
            names = [r[0] for r in con.execute(q, (source,))]
        except sqlite3.OperationalError:
            names = [r[0] for r in con.execute("select name from maps")]
        con.close()
    rows = []
    for nm in sorted(names):
        p = os.path.join(src, nm + ".map")
        if not os.path.exists(p):
            continue
        try:
            rows.append(analyze(p, sightlines=True))
        except Exception as e:
            print(f"  ! {nm}: {e}", file=sys.stderr)
    keys = ["loop_density", "dead_end_ratio", "coverage", "elevation_levels",
            "elevation_span", "sightline_p50", "sightline_p90"]
    print(f"{'map':10} {'nodes':>6} {'loops':>6} {'lpD':>6} {'dead':>6} "
          f"{'cov':>5} {'elv':>4} {'span':>5} {'sl50':>5} {'sl90':>5}")
    for r in rows:
        print(f"{r['map'][:10]:10} {r.get('nodes',0):6} {r.get('loops',0):6} "
              f"{r.get('loop_density',0):6.3f} {r.get('dead_end_ratio',0):6.3f} "
              f"{r.get('coverage',0):5.2f} {r.get('elevation_levels',0):4} "
              f"{r.get('elevation_span',0):5} {r.get('sightline_p50',0):5} "
              f"{r.get('sightline_p90',0):5}")
    print("\n-- bands (p25 / median / p75) --")
    for k in keys:
        vals = sorted(r[k] for r in rows if k in r)
        if not vals:
            continue
        q = lambda f: vals[min(len(vals) - 1, int(len(vals) * f))]  # noqa: E731
        print(f"  {k:16} {q(.25):8.2f} {q(.5):8.2f} {q(.75):8.2f}")


def main():
    args = sys.argv[1:]
    if args and args[0] == "corpus":
        src = "id"
        if "--source" in args:
            src = args[args.index("--source") + 1]
        _corpus(src)
        return
    json_path = None
    if "--json" in args:
        i = args.index("--json")
        json_path = args[i + 1]
        del args[i:i + 2]
    if len(args) != 1:
        print(__doc__)
        sys.exit(2)
    res = analyze(args[0])
    print(json.dumps(res, indent=1))
    if json_path:
        with open(json_path, "w") as f:
            json.dump(res, f, indent=1)


if __name__ == "__main__":
    main()
