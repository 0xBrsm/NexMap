import flowstruct, statistics, os

CACHE = os.path.expanduser("~/.cache/nexmap")
DIRS = [f"{CACHE}/quake_map_source/bsp", f"{CACHE}/corpusbsp"]


def find(name):
    for d in DIRS:
        p = f"{d}/{name}.bsp"
        if os.path.exists(p):
            return p
    return None


names = []
with open(f"{CACHE}/sp_band.txt") as f:
    next(f)
    for line in f:
        if line.strip():
            names.append(line.split()[0])

medA, cvs, dens = [], [], []
missing = []
for n in names:
    p = find(n)
    if not p:
        missing.append(n); continue
    try:
        g = flowstruct.load_graph(p)
        a = [x[3] for x in g["polys"]]
        if len(a) < 5:
            continue
        tot = sum(a); mean = tot / len(a)
        medA.append(statistics.median(a))
        cvs.append(statistics.pstdev(a) / mean)
        dens.append(len(a) / (tot / 1e6))      # polys per million units^2
    except Exception as e:
        missing.append(f"{n}:{str(e)[:30]}")


def band(xs, label):
    xs = sorted(xs)
    n = len(xs)
    p = lambda q: xs[min(n - 1, int(q * n))]
    print(f"  {label:<10} n={n:<3} p10={p(0.1):>8.1f}  median={statistics.median(xs):>8.1f}  "
          f"p90={p(0.9):>8.1f}  min={xs[0]:.1f} max={xs[-1]:.1f}")


print(f"corpus maps measured: {len(medA)}  (missing {len(missing)})")
band(medA, "medPolyA")
band(cvs, "areaCV")
band(dens, "polyDens")
if missing:
    print("missing:", ", ".join(missing[:12]))
