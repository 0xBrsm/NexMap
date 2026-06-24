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
import os
import sys

_HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, _HERE)
sys.path.insert(0, os.path.join(_HERE, "..", "maps"))

from qcheck import Checker                          # noqa: E402


def analyze(path, sightlines=True):
    """Flow metrics for one map. The graph + metric code lives on
    qcheck.Checker so qcheck's build-time flow gate and this reporter never
    diverge."""
    out = {"map": os.path.basename(path)}
    out.update(Checker(path).flow_metrics(sightlines=sightlines))
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
