#!/usr/bin/env python3
"""Design corpus over Quake .map sources (id originals + tagged extras).

ingest:  parse all .map files into a sqlite db (~/.cache/nexmap/corpus.db)
queries: themes, role, pairs, lights, dims, ents, sql
         queries default to --source id; pass --source udob|ad|all to widen
"""

import json
import math
import os
import re
import sqlite3
import sys

CACHE_DIR = os.path.expanduser("~/.cache/nexmap")
CORPUS_DIR = os.path.join(CACHE_DIR, "quake_map_source")
DB_PATH = os.path.join(CACHE_DIR, "corpus.db")

# (source, dir, progs) — id must stay first: its texture set must be known
# before stock_tex_pct can be computed for the other sources.
SOURCES = [
    ("id", CORPUS_DIR, "vanilla"),
    ("udob", os.path.join(CACHE_DIR, "sources/udob"), "copper"),
    ("ad", os.path.join(CACHE_DIR, "sources/ad"), "ad"),
]

# BSP2-format maps per scan of the shipped paks; everything else is format 29.
BSP2 = {"ad/ad_azad", "ad/ad_crucial", "ad/ad_magna",
        "ad/ad_sepulcher", "ad/ad_swampy", "ad/ad_tfuma"}

# Dimension of the Past ships compiled bsps only (no sources), but it is
# vanilla-progs + stock-texture, so it's tagged into maps for the playable set.
DOPA_MAPS = ["start", "e5m1", "e5m2", "e5m3", "e5m4",
             "e5m5", "e5m6", "e5m7", "e5m8", "e5end", "e5dm"]

# ---------------------------------------------------------------- parsing

FACE_RE = re.compile(
    r"\(\s*([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)\s*\)\s*"
    r"\(\s*([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)\s*\)\s*"
    r"\(\s*([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)\s*\)\s*"
    r"(\S+)"
)
KV_RE = re.compile(r'"([^"]*)"\s+"([^"]*)"')


def parse_map(path):
    """Yield entities: (keys_dict, [brush, ...]); brush = [(p0,p1,p2,tex), ...]."""
    ents = []
    ent = None
    brush = None
    for raw in open(path, encoding="latin-1"):
        line = raw.strip()
        if not line or line.startswith("//"):
            continue
        if line == "{":
            if ent is None:
                ent = ({}, [])
            else:
                brush = []
        elif line == "}":
            if brush is not None:
                ent[1].append(brush)
                brush = None
            else:
                ents.append(ent)
                ent = None
        elif brush is not None:
            m = FACE_RE.match(line)
            if not m:
                raise ValueError(f"{path}: bad face line: {line}")
            g = m.groups()
            p = [tuple(float(g[i * 3 + j]) for j in range(3)) for i in range(3)]
            brush.append((p[0], p[1], p[2], g[9].lower()))
        elif ent is not None:
            m = KV_RE.match(line)
            if m:
                ent[0][m.group(1)] = m.group(2)
    return ents


def outward_normal(p0, p1, p2):
    # cross(p1-p0, p2-p0) points INTO the brush in .map convention; negate.
    ax, ay, az = (p1[0] - p0[0], p1[1] - p0[1], p1[2] - p0[2])
    bx, by, bz = (p2[0] - p0[0], p2[1] - p0[1], p2[2] - p0[2])
    n = (-(ay * bz - az * by), -(az * bx - ax * bz), -(ax * by - ay * bx))
    ln = math.sqrt(n[0] ** 2 + n[1] ** 2 + n[2] ** 2)
    return (0.0, 0.0, 0.0) if ln == 0 else (n[0] / ln, n[1] / ln, n[2] / ln)


def orient(nz):
    if nz > 0.7:
        return "floor"
    if nz < -0.7:
        return "ceiling"
    return "wall"


def family(tex):
    """Texture family: leading prefix with digits/suffixes stripped."""
    t = tex.lower()
    star = ""
    if t[0] in "*+":
        if t[0] == "+" and len(t) > 1 and t[1].isdigit():
            t = t[2:]  # animation frame prefix +0name
        elif t[0] == "*":
            star, t = "*", t[1:]
        else:
            t = t[1:]
    t = t.lstrip("0123456789")
    m = re.match(r"[a-z]+", t)
    return star + (m.group(0) if m else t)


SCHEMA = """
CREATE TABLE maps(id INTEGER PRIMARY KEY, name TEXT, message TEXT, worldtype TEXT,
    source TEXT, progs TEXT, bsp_format TEXT, stock_tex_pct REAL, engine_needs TEXT);
CREATE TABLE entities(id INTEGER PRIMARY KEY, map_id INT, classname TEXT,
    keys TEXT, x REAL, y REAL, z REAL);
CREATE TABLE brushes(id INTEGER PRIMARY KEY, map_id INT, entity_id INT,
    minx REAL, miny REAL, minz REAL, maxx REAL, maxy REAL, maxz REAL,
    dx REAL, dy REAL, dz REAL);
CREATE TABLE faces(id INTEGER PRIMARY KEY, brush_id INT, map_id INT,
    texture TEXT, family TEXT, nx REAL, ny REAL, nz REAL, orient TEXT);
CREATE INDEX idx_faces_map ON faces(map_id);
CREATE INDEX idx_faces_tex ON faces(texture);
CREATE INDEX idx_faces_fam ON faces(family);
CREATE INDEX idx_ents_class ON entities(classname);
"""


def ingest():
    if os.path.exists(DB_PATH):
        os.remove(DB_PATH)
    db = sqlite3.connect(DB_PATH)
    db.executescript(SCHEMA)
    id_textures = set()
    for source, dirpath, progs in SOURCES:
        maps = sorted(
            f for f in os.listdir(dirpath)
            if f.endswith(".map")
        ) if os.path.isdir(dirpath) else []
        if not maps:
            sys.exit(f"no .map files in {dirpath}")
        for fname in maps:
            ingest_map(db, source, dirpath, fname, progs, id_textures)
    for base in DOPA_MAPS:
        db.execute(
            "INSERT INTO maps(name, message, worldtype, source, progs,"
            " bsp_format, stock_tex_pct, engine_needs)"
            " VALUES(?,?,?,?,?,?,?,?)",
            (f"dopa/{base}", "", "", "dopa", "vanilla", "29", 100.0, ""),
        )
    db.commit()
    print(f"-> {DB_PATH}")


def ingest_map(db, source, dirpath, fname, progs, id_textures):
    base = fname[:-4]
    name = base if source == "id" else f"{source}/{base}"
    ents = parse_map(os.path.join(dirpath, fname))
    world = ents[0][0] if ents else {}
    cur = db.execute(
        "INSERT INTO maps(name, message, worldtype, source, progs, bsp_format)"
        " VALUES(?,?,?,?,?,?)",
        (name, world.get("message", ""), world.get("worldtype", ""),
         source, progs, "bsp2" if name in BSP2 else "29"),
    )
    map_id = cur.lastrowid
    needs = set()
    if name in BSP2:
        needs.add("bsp2")
    if any(k.startswith("fog") or k == "_skyfog" for k in world):
        needs.add("fog")
    seen_tex, stock_faces = set(), 0
    nb = nf = 0
    for keys, brushes in ents:
        org = keys.get("origin", "").split()
        x, y, z = (float(v) for v in org) if len(org) == 3 else (None,) * 3
        cur = db.execute(
            "INSERT INTO entities(map_id, classname, keys, x, y, z)"
            " VALUES(?,?,?,?,?,?)",
            (map_id, keys.get("classname", ""), json.dumps(keys), x, y, z),
        )
        ent_id = cur.lastrowid
        for brush in brushes:
            pts = [p for f in brush for p in f[:3]]
            mn = [min(p[i] for p in pts) for i in range(3)]
            mx = [max(p[i] for p in pts) for i in range(3)]
            cur = db.execute(
                "INSERT INTO brushes(map_id, entity_id, minx, miny, minz,"
                " maxx, maxy, maxz, dx, dy, dz) VALUES(?,?,?,?,?,?,?,?,?,?,?)",
                (map_id, ent_id, *mn, *mx,
                 mx[0] - mn[0], mx[1] - mn[1], mx[2] - mn[2]),
            )
            brush_id = cur.lastrowid
            for p0, p1, p2, tex in brush:
                n = outward_normal(p0, p1, p2)
                db.execute(
                    "INSERT INTO faces(brush_id, map_id, texture, family,"
                    " nx, ny, nz, orient) VALUES(?,?,?,?,?,?,?,?)",
                    (brush_id, map_id, tex, family(tex), *n, orient(n[2])),
                )
                seen_tex.add(tex)
                if tex.startswith("{"):
                    needs.add("fence")
                if tex in id_textures:
                    stock_faces += 1
                nf += 1
            nb += 1
    if source == "id":
        id_textures |= seen_tex
        pct = 100.0
    else:
        pct = round(100.0 * stock_faces / nf, 1) if nf else 0.0
    db.execute(
        "UPDATE maps SET stock_tex_pct=?, engine_needs=? WHERE id=?",
        (pct, ",".join(sorted(needs)), map_id),
    )
    print(f"{name}: {nb} brushes, {nf} faces, {len(ents)} entities,"
          f" stock={pct:.0f}% needs=[{','.join(sorted(needs))}]")


# ---------------------------------------------------------------- queries

def connect():
    if not os.path.exists(DB_PATH):
        sys.exit(f"{DB_PATH} missing — run: qcorpus.py ingest")
    return sqlite3.connect(DB_PATH)


def map_filter(args):
    """(sql_clause, params) for optional --map <name> / --source <src|all>.

    Defaults to --source id so the stats the quake-mapping skill was distilled
    from stay uncontaminated by the tagged extras (udob/ad).
    """
    clause, params, src = "", [], "id"
    for i, a in enumerate(args):
        if a == "--map":
            clause = " AND map_id=(SELECT id FROM maps WHERE name=?)"
            params = [args[i + 1]]
            src = "all"  # an explicit map already pins the source
        elif a == "--source":
            src = args[i + 1]
    if src != "all":
        clause += " AND map_id IN (SELECT id FROM maps WHERE source=?)"
        params.append(src)
    return clause, params


def cmd_themes(args):
    db = connect()
    mf, mp = map_filter(args)
    rows = db.execute(f"""
        SELECT m.name, f.family, COUNT(*) c
        FROM faces f JOIN maps m ON m.id = f.map_id
        WHERE 1=1 {mf.replace('map_id', 'f.map_id')}
        GROUP BY m.name, f.family ORDER BY m.name, c DESC""", mp).fetchall()
    bymap = {}
    for name, fam, c in rows:
        bymap.setdefault(name, []).append((fam, c))
    only = args[0] if args and not args[0].startswith("--") else None
    for name, fams in bymap.items():
        if only and name != only:
            continue
        total = sum(c for _, c in fams)
        top = "  ".join(f"{f}:{100*c//total}%" for f, c in fams[:6])
        print(f"{name:8} {top}")


def cmd_role(args):
    if not args:
        sys.exit("usage: role <texture-or-family>")
    db = connect()
    t = args[0].lower()
    mf, mp = map_filter(args)
    rows = db.execute(f"""
        SELECT texture, orient, COUNT(*) FROM faces
        WHERE (texture = ? OR family = ?) {mf}
        GROUP BY texture, orient ORDER BY texture""", [t, t] + mp).fetchall()
    agg = {}
    for tex, o, c in rows:
        agg.setdefault(tex, {})[o] = c
    for tex, d in sorted(agg.items()):
        total = sum(d.values())
        parts = "  ".join(f"{o}:{100*c//total}%({c})" for o, c in
                          sorted(d.items(), key=lambda kv: -kv[1]))
        print(f"{tex:16} {parts}")


def cmd_pairs(args):
    if not args:
        sys.exit("usage: pairs <texture-or-family> [--map name]")
    db = connect()
    t = args[0].lower()
    mf, mp = map_filter(args)
    # Co-occurrence: textures sharing a brush with t, by orientation of the partner.
    rows = db.execute(f"""
        SELECT f2.texture, f2.orient, COUNT(*) c FROM faces f1
        JOIN faces f2 ON f1.brush_id = f2.brush_id
        WHERE (f1.texture = ? OR f1.family = ?) AND f2.texture != f1.texture
          AND 1=1 {mf.replace('map_id', 'f1.map_id')}
        GROUP BY f2.texture, f2.orient ORDER BY c DESC LIMIT 25""",
        [t, t] + mp).fetchall()
    print(f"same-brush partners of {t}:")
    for tex, o, c in rows:
        print(f"  {tex:16} {o:8} {c}")
    # Same-map context: top textures per orientation in maps where t is prominent.
    rows = db.execute(f"""
        SELECT f.orient, f.texture, COUNT(*) c FROM faces f
        WHERE f.map_id IN (
            SELECT map_id FROM faces WHERE texture = ? OR family = ?
            GROUP BY map_id HAVING COUNT(*) >= 50)
          AND 1=1 {mf.replace('map_id', 'f.map_id')}
        GROUP BY f.orient, f.texture ORDER BY f.orient, c DESC""",
        [t, t] + mp).fetchall()
    byorient = {}
    for o, tex, c in rows:
        byorient.setdefault(o, []).append(f"{tex}({c})")
    print(f"top textures in maps using {t}:")
    for o, texs in byorient.items():
        print(f"  {o:8} " + "  ".join(texs[:8]))


def cmd_lights(args):
    db = connect()
    mf, mp = map_filter(args)
    rows = db.execute(f"""
        SELECT classname, keys FROM entities
        WHERE classname LIKE 'light%' {mf}""", mp).fetchall()
    byclass = {}
    for cls, keys in rows:
        v = int(float(json.loads(keys).get("light", 200)))
        byclass.setdefault(cls, []).append(v)
    for cls, vals in sorted(byclass.items()):
        vals.sort()
        n = len(vals)
        print(f"{cls:28} n={n:4}  min={vals[0]:4}  med={vals[n//2]:4}"
              f"  p90={vals[int(n*.9)]:4}  max={vals[-1]:4}")
    # Light value vs nearest brush texture family.
    if "--near" in args:
        cmd_lights_near(db, mp, mf)


def cmd_lights_near(db, mp, mf):
    lights = db.execute(f"""
        SELECT map_id, x, y, z, keys FROM entities
        WHERE classname LIKE 'light%' AND x IS NOT NULL {mf}""", mp).fetchall()
    brushes = {}
    for mid, bx0, by0, bz0, bx1, by1, bz1, bid in db.execute(
            "SELECT map_id, minx, miny, minz, maxx, maxy, maxz, id FROM brushes"):
        brushes.setdefault(mid, []).append((bx0, by0, bz0, bx1, by1, bz1, bid))
    fams = dict(db.execute(
        "SELECT brush_id, family FROM faces GROUP BY brush_id"))
    byfam = {}
    for mid, x, y, z, keys in lights:
        v = int(float(json.loads(keys).get("light", 200)))
        best, bid = 1e18, None
        for b in brushes.get(mid, []):
            dx = max(b[0] - x, 0, x - b[3])
            dy = max(b[1] - y, 0, y - b[4])
            dz = max(b[2] - z, 0, z - b[5])
            d = dx * dx + dy * dy + dz * dz
            if d < best:
                best, bid = d, b[6]
        if bid is not None:
            byfam.setdefault(fams.get(bid, "?"), []).append(v)
    print("\nlight values by nearest brush family:")
    for fam, vals in sorted(byfam.items(), key=lambda kv: -len(kv[1])):
        vals.sort()
        n = len(vals)
        print(f"  {fam:12} n={n:4}  med={vals[n//2]:4}  p90={vals[int(n*.9)]:4}")


def cmd_dims(args):
    db = connect()
    mf, mp = map_filter(args)
    print("step-height candidates (walkable tops, dz 4..24):")
    rows = db.execute(f"""
        SELECT b.dz, COUNT(*) c FROM brushes b
        WHERE b.dz BETWEEN 4 AND 24 AND EXISTS(
            SELECT 1 FROM faces f WHERE f.brush_id = b.id AND f.orient='floor')
        AND 1=1 {mf.replace('map_id', 'b.map_id')}
        GROUP BY b.dz ORDER BY c DESC LIMIT 10""", mp).fetchall()
    for dz, c in rows:
        print(f"  dz={dz:5.0f}  n={c}")
    print("common brush thicknesses (min dimension):")
    rows = db.execute(f"""
        SELECT MIN(dx, dy, dz) t, COUNT(*) c FROM brushes b
        WHERE 1=1 {mf.replace('map_id', 'b.map_id')}
        GROUP BY t ORDER BY c DESC LIMIT 12""", mp).fetchall()
    for t, c in rows:
        print(f"  {t:6.0f}  n={c}")


def cmd_ents(args):
    db = connect()
    pat = (args[0] if args and not args[0].startswith("--") else "") + "%"
    mf, mp = map_filter(args)
    rows = db.execute(f"""
        SELECT e.classname, COUNT(*) total, COUNT(DISTINCT e.map_id) nmaps
        FROM entities e WHERE e.classname LIKE ? {mf.replace('map_id', 'e.map_id')}
        GROUP BY e.classname ORDER BY total DESC""", [pat] + mp).fetchall()
    for cls, total, nmaps in rows:
        print(f"{cls:32} total={total:4}  maps={nmaps}")


def cmd_sql(args):
    db = connect()
    for row in db.execute(args[0]):
        print("\t".join(str(v) for v in row))


def main():
    cmds = {
        "ingest": lambda a: ingest(),
        "themes": cmd_themes,
        "role": cmd_role,
        "pairs": cmd_pairs,
        "lights": cmd_lights,
        "dims": cmd_dims,
        "ents": cmd_ents,
        "sql": cmd_sql,
    }
    if len(sys.argv) < 2 or sys.argv[1] not in cmds:
        sys.exit("usage: qcorpus.py {ingest|themes|role|pairs|lights|dims|ents|sql} ...")
    cmds[sys.argv[1]](sys.argv[2:])


if __name__ == "__main__":
    main()
