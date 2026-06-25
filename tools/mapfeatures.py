#!/usr/bin/env python3
"""Full per-map structural feature vector, grounded in level-design research.

Combines the walk-graph features (flowstruct: scale, loops, betweenness /
chokepoint structure, verticality) with the visibility features (vga:
occlusion density, sightline length + variety) into one vector per map, and
summarizes the "good region" (median + p10..p90) over a map set — the
Expressive Range Analysis the corpus is meant to define.

Each feature's grounding:
  areas      scale/differentiation                         (descriptive)
  loopDens   cyclomatic loops per area  [KNOWN MISLEADING]  (network sci)
  bcGini     betweenness concentration = chokepoint structure (network sci; high=structured)
  bcMax      strongest single chokepoint                    (Hullett choke point)
  visDens    fraction of areas in mutual line-of-sight = occlusion (space syntax VGA; low=occluded/good)
  sightMed   typical sightline length                       (Hullett sightlines)
  sightCV    sightline-length variety                       (Hullett "varied sightlines")
  zband      elevation bands = vertical layering            (Hullett split-level/gallery)

Usage: mapfeatures.py [--summary] <map.bsp> ...
"""
import os, sys, statistics

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import flowstruct, vga as vgamod

COLS = ["areas", "loopDens", "bcGini", "bcMax", "visDens", "sightMed", "sightCV", "zband"]

def features(bsp):
    f = flowstruct.analyze(flowstruct.load_graph(bsp), 256.0, 128.0)
    v = vgamod.vga(bsp)
    return {
        "areas": f["areas"], "loopDens": f["loop_density"],
        "bcGini": f["bc_gini"], "bcMax": f["bc_max"],
        "visDens": v["vis_density"], "sightMed": v["sight_med"], "sightCV": v["sight_cv"],
        "zband": f["zbands"],
    }

# SP good-region gate, derived from the 97-map exemplar corpus (base Quake +
# DOPA + SoA + DoE + udob + AD, median-scale). Each entry: (bad_direction,
# threshold, median_target). A map is flagged when it crosses the threshold in
# the "bad" direction. These are AUTHORING TARGETS, not hard limits — aim for
# the median; a WARN means out of the range every shipped id-quality map sits in.
SP_GATE = {
    "areas":    ("low",  125,  245),   # below = too small / under-differentiated
    "visDens":  ("high", 0.23, 0.05),  # above = over-exposed blob (no occlusion)
    "bcGini":   ("low",  0.68, 0.74),  # below = no real chokepoints (flat betweenness)
    "zband":    ("low",  7,    11),    # below = too flat (no vertical layering)
    "sightCV":  ("low",  0.47, 0.54),  # below = uniform sightlines (no intimate/vista mix)
}

def gate(bsp):
    r = features(bsp)
    name = os.path.basename(bsp).replace(".bsp", "")
    print(f"feature gate: {name}")
    passes = 0
    for k, (bad, thr, tgt) in SP_GATE.items():
        v = r[k]
        ok = (v >= thr) if bad == "low" else (v <= thr)
        passes += ok
        arrow = "" if ok else (" TOO LOW"  if bad == "low" else " TOO HIGH")
        print(f"  {'OK ' if ok else 'WARN'} {k:<9} {v:>8}  (target ~{tgt}, band {'>=' if bad=='low' else '<='}{thr}){arrow}")
    verdict = "IN BAND" if passes == len(SP_GATE) else f"{passes}/{len(SP_GATE)} in band"
    print(f"  => {verdict}")
    return passes == len(SP_GATE)

def main():
    args = sys.argv[1:]
    if "--gate" in args:
        bsps = [a for a in args if not a.startswith("--")]
        allok = all(gate(b) for b in bsps)
        sys.exit(0 if allok else 1)
    summary = "--summary" in args
    bsps = [a for a in args if a != "--summary"]
    rows = {}
    hdr = f"{'map':<12}" + "".join(f"{c:>9}" for c in COLS)
    print(hdr)
    for bsp in bsps:
        name = os.path.basename(bsp).replace(".bsp", "")
        try:
            r = features(bsp); rows[name] = r
        except Exception as e:
            print(f"{name:<12} ERROR {str(e)[:60]}"); continue
        print(f"{name:<12}" + "".join(f"{r[c]:>9}" for c in COLS))
    if summary and rows:
        def band(c):
            xs = sorted(rows[n][c] for n in rows)
            p = lambda q: xs[min(len(xs)-1, int(q*len(xs)))]
            return f"{statistics.median(xs):>9.2f}", f"{p(0.1):.2f}-{p(0.9):.2f}"
        print("-" * len(hdr))
        print(f"{'MEDIAN':<12}" + "".join(band(c)[0] for c in COLS))
        print(f"{'p10..p90':<12}" + "".join(f"{band(c)[1]:>9}" for c in COLS))

if __name__ == "__main__":
    main()
