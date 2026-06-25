#!/usr/bin/env python3
"""Extract files from a Quake PAK archive.

PAK format: 12-byte header ("PACK", int dirofs, int dirlen), then dirlen/64
directory entries of {char name[56]; int filepos; int filelen}.

Usage: pakextract.py <file.pak> <dest_dir> [substr]
  Extracts entries whose name contains <substr> (default ".bsp"), flattened to
  basename in dest_dir.
"""
import os, struct, sys

def extract(pak_path, dest, substr=".bsp"):
    os.makedirs(dest, exist_ok=True)
    with open(pak_path, "rb") as f:
        data = f.read()
    if data[:4] != b"PACK":
        raise ValueError(f"{pak_path}: not a PAK (magic {data[:4]!r})")
    dirofs, dirlen = struct.unpack("<ii", data[4:12])
    n = dirlen // 64
    out = []
    for i in range(n):
        e = data[dirofs + i*64: dirofs + i*64 + 64]
        name = e[:56].split(b"\0")[0].decode("latin1")
        pos, length = struct.unpack("<ii", e[56:64])
        if substr in name:
            base = os.path.basename(name)
            with open(os.path.join(dest, base), "wb") as w:
                w.write(data[pos:pos+length])
            out.append(base)
    return out

if __name__ == "__main__":
    if len(sys.argv) < 3:
        print(__doc__); sys.exit(2)
    sub = sys.argv[3] if len(sys.argv) > 3 else ".bsp"
    got = extract(sys.argv[1], sys.argv[2], sub)
    print(f"extracted {len(got)} files matching {sub!r}: {', '.join(sorted(got))}")
