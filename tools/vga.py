#!/usr/bin/env python3
"""Space-syntax Visibility Graph Analysis of a map's walkable space.

Runs `navcheck <bsp> -vga` (area visibility graph via the hull-0 line-of-sight
tracer) and computes the validated legibility metrics from Turner et al. /
space syntax: visibility density, mean connectivity, integration (1/RA), and
intelligibility (corr of local connectivity vs global integration). These
distinguish a legible structured map from an over-connected blob — the failure
mode raw loop-density misses.

Usage: vga.py <map.bsp> [map2.bsp ...]
"""
import json, os, subprocess, sys, math
from collections import deque, defaultdict

HERE = os.path.dirname(os.path.abspath(__file__))
NAVCHECK = os.path.join(HERE, "navcheck", "build", "navcheck")

def vga(bsp):
    out = subprocess.run([NAVCHECK, bsp, "-vga"], capture_output=True, text=True)
    if out.returncode != 0:
        raise RuntimeError(out.stderr.strip()[:200])
    g = json.loads(out.stdout)
    areas, edges = g["areas"], g["vis_edges"]
    N = len(areas)
    adj = defaultdict(set)
    for a, b in edges:
        adj[a].add(b); adj[b].add(a)

    conn = [len(adj[i]) for i in range(N)]
    # Mean depth per node (BFS on visibility graph), then RA + integration.
    integ = [0.0] * N
    for s in range(N):
        dist = {s: 0}; q = deque([s])
        while q:
            u = q.popleft()
            for v in adj[u]:
                if v not in dist:
                    dist[v] = dist[u] + 1; q.append(v)
        reach = [d for d in dist.values() if d > 0]
        if len(reach) < 2 or N < 3:
            integ[s] = 0.0; continue
        md = sum(reach) / len(reach)
        ra = 2 * (md - 1) / (N - 2)
        integ[s] = 1.0 / ra if ra > 0 else 0.0

    def pearson(x, y):
        n = len(x)
        if n < 3: return 0.0
        mx, my = sum(x)/n, sum(y)/n
        sx = sum((a-mx)**2 for a in x); sy = sum((b-my)**2 for b in y)
        if sx == 0 or sy == 0: return 0.0
        cov = sum((a-mx)*(b-my) for a, b in zip(x, y))
        return cov / math.sqrt(sx*sy)

    # Sightline lengths: euclidean length of each line-of-sight edge.
    slen = []
    for a, b in edges:
        dx = areas[a][0]-areas[b][0]; dy = areas[a][1]-areas[b][1]; dz = areas[a][2]-areas[b][2]
        slen.append(math.sqrt(dx*dx + dy*dy + dz*dz))
    slen.sort()
    def pct(p):
        if not slen: return 0.0
        return slen[min(len(slen)-1, int(p*len(slen)))]
    med = pct(0.5)
    # variety = coefficient of variation (spread relative to median); good maps
    # mix short rooms with occasional long vistas, not one uniform scale.
    mean_s = sum(slen)/len(slen) if slen else 0
    var = sum((s-mean_s)**2 for s in slen)/len(slen) if slen else 0
    cv = round(math.sqrt(var)/mean_s, 2) if mean_s else 0

    possible = N*(N-1)/2 if N > 1 else 1
    return dict(
        areas=N,
        vis_density=round(len(edges)/possible, 3),       # blob detector (high = blob)
        mean_conn=round(sum(conn)/N, 1) if N else 0,
        mean_integ=round(sum(integ)/N, 2) if N else 0,
        intelligibility=round(pearson(conn, integ), 2),   # high = legible (but blobs score high too)
        sight_med=round(med, 0),                          # typical sightline length
        sight_max=round(pct(0.99), 0),                    # longest vista
        sight_cv=cv,                                       # sightline variety (spread/mean)
    )

def main():
    print(f"{'map':<11} {'areas':>5} {'visDens':>7} {'intellig':>8} "
          f"{'sightMed':>8} {'sightMax':>8} {'sightCV':>7}")
    for bsp in sys.argv[1:]:
        name = os.path.basename(bsp).replace(".bsp", "")
        try:
            r = vga(bsp)
        except Exception as e:
            print(f"{name:<11} ERROR {e}"); continue
        print(f"{name:<11} {r['areas']:>5} {r['vis_density']:>7} {r['intelligibility']:>8} "
              f"{r['sight_med']:>8} {r['sight_max']:>8} {r['sight_cv']:>7}")

if __name__ == "__main__":
    main()
