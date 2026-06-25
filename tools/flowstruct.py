#!/usr/bin/env python3
"""Read the macro flow structure of a map from navcheck's walk graph.

Runs `navcheck <bsp> -flow`, clusters the navmesh polys into room-scale areas,
and reports the area graph's structure: independent loops, hubs, vertical links,
elevation spread. The point is to SEE what good (id) flow looks like
structurally, as input for loop-first map authoring — not to grade a map.

Usage: flowstruct.py [-c CELL] [-z ZCELL] <map.bsp> [map2.bsp ...]
"""
import json, os, subprocess, sys, argparse
from collections import defaultdict

HERE = os.path.dirname(os.path.abspath(__file__))
NAVCHECK = os.path.join(HERE, "navcheck", "build", "navcheck")

def load_graph(bsp):
    out = subprocess.run([NAVCHECK, bsp, "-flow"], capture_output=True, text=True)
    if out.returncode != 0:
        raise RuntimeError(f"navcheck failed on {bsp}: {out.stderr.strip()[:200]}")
    return json.loads(out.stdout)

def analyze(g, cell, zcell):
    polys = g["polys"]                      # [x,y,z,area]
    def cid(i):
        x, y, z, _ = polys[i]
        return (round(x / cell), round(y / cell), round(z / zcell))

    # areas (clusters) and their representative elevation
    cells = {}
    cell_area = defaultdict(float)
    cell_z = defaultdict(list)
    for i, p in enumerate(polys):
        c = cid(i)
        cells.setdefault(c, len(cells))
        cell_area[c] += p[3]
        cell_z[c].append(p[2])

    # edges between areas (base mesh adjacency + off-mesh links)
    edges = set()
    vlinks = 0  # off-mesh edges crossing a real elevation gap
    for (a, b) in g["base_edges"]:
        ca, cb = cid(a), cid(b)
        if ca != cb:
            edges.add((min(cells[ca], cells[cb]), max(cells[ca], cells[cb])))
    for (a, b) in g["offmesh_edges"]:
        ca, cb = cid(a), cid(b)
        if ca != cb:
            edges.add((min(cells[ca], cells[cb]), max(cells[ca], cells[cb])))
        if abs(polys[a][2] - polys[b][2]) > 48:
            vlinks += 1

    V = len(cells)
    E = len(edges)
    # connected components
    adj = defaultdict(set)
    for u, v in edges:
        adj[u].add(v); adj[v].add(u)
    seen = set(); C = 0
    for n in range(V):
        if n in seen: continue
        C += 1; stack = [n]
        while stack:
            x = stack.pop()
            if x in seen: continue
            seen.add(x); stack.extend(adj[x] - seen)
    L = E - V + C                       # independent loops (cyclomatic)
    deg = [len(adj[n]) for n in range(V)]
    hubs = sum(1 for d in deg if d >= 4)
    zbands = len({round(sum(zs)/len(zs) / 96) for zs in cell_z.values()})
    bc = betweenness(adj, V)            # chokepoint structure
    return dict(areas=V, edges=E, comps=C, loops=L,
                loop_density=round(L / V, 2) if V else 0,
                hubs=hubs, max_deg=max(deg) if deg else 0,
                vlinks=vlinks, zbands=zbands,
                bc_max=round(max(bc), 3) if bc else 0,   # strongest chokepoint
                bc_gini=round(gini(bc), 2))              # concentration: high=few real chokes, low=blob

def betweenness(adj, V):
    """Brandes betweenness centrality on the area walk graph, normalized."""
    from collections import deque
    bc = [0.0] * V
    for s in range(V):
        S = []; P = [[] for _ in range(V)]; sigma = [0]*V; sigma[s] = 1
        d = [-1]*V; d[s] = 0; Q = deque([s])
        while Q:
            v = Q.popleft(); S.append(v)
            for w in adj[v]:
                if d[w] < 0: d[w] = d[v] + 1; Q.append(w)
                if d[w] == d[v] + 1: sigma[w] += sigma[v]; P[w].append(v)
        delta = [0.0]*V
        while S:
            w = S.pop()
            for v in P[w]:
                delta[v] += (sigma[v]/sigma[w]) * (1 + delta[w])
            if w != s: bc[w] += delta[w]
    norm = (V-1)*(V-2)/2 if V > 2 else 1   # undirected normalization
    return [b/2/norm for b in bc]

def gini(x):
    xs = sorted(x); n = len(xs); s = sum(xs)
    if n == 0 or s == 0: return 0.0
    cum = sum((i+1)*v for i, v in enumerate(xs))
    return round(2*cum/(n*s) - (n+1)/n, 4)

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("-c", "--cell", type=float, default=256.0)
    ap.add_argument("-z", "--zcell", type=float, default=128.0)
    ap.add_argument("bsps", nargs="+")
    a = ap.parse_args()
    print(f"{'map':<11} {'areas':>5} {'loops':>5} {'L/area':>6} {'bcMax':>6} {'bcGini':>6} "
          f"{'vlink':>5} {'zband':>5}  (cell={a.cell:.0f} z={a.zcell:.0f})")
    for bsp in a.bsps:
        name = os.path.basename(bsp).replace(".bsp", "")
        try:
            r = analyze(load_graph(bsp), a.cell, a.zcell)
        except Exception as e:
            print(f"{name:<10} ERROR {e}"); continue
        print(f"{name:<11} {r['areas']:>5} {r['loops']:>5} {r['loop_density']:>6} "
              f"{r['bc_max']:>6} {r['bc_gini']:>6} {r['vlinks']:>5} {r['zbands']:>5}")

if __name__ == "__main__":
    main()
