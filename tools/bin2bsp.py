#!/usr/bin/env python3
# Extract Quake paks from a MODE1/2352 raw CD data track, then the .bsp maps.
# Strips 2352-byte sector overhead -> 2048 ISO, carves paks by their header.
import sys, struct, os

def iso_from_raw(path):
    with open(path, "rb") as f:
        raw = f.read()
    out = bytearray()
    for off in range(0, len(raw) - 2351, 2352):
        out += raw[off+16: off+16+2048]   # MODE1: skip 16 sync/header, take 2048
    return bytes(out)

def carve_paks(iso):
    paks = []
    i = 0
    while True:
        i = iso.find(b"PACK", i)
        if i < 0:
            break
        dirofs, dirlen = struct.unpack("<ii", iso[i+4:i+12])
        if dirlen > 0 and dirlen % 64 == 0 and 0 < dirofs and dirofs + dirlen <= len(iso) - i:
            size = dirofs + dirlen
            blob = iso[i:i+size]
            # validate: first dir entry has a sane name + in-range file span
            name = blob[dirofs:dirofs+56].split(b"\0")[0]
            fp, fl = struct.unpack("<ii", blob[dirofs+56:dirofs+64])
            if name and all(32 <= c < 127 for c in name) and 0 <= fp and fp+fl <= size:
                paks.append(blob)
                i += size
                continue
        i += 4
    return paks

if __name__ == "__main__":
    raw, dest = sys.argv[1], sys.argv[2]
    os.makedirs(dest, exist_ok=True)
    iso = iso_from_raw(raw)
    paks = carve_paks(iso)
    print(f"{os.path.basename(raw)}: ISO {len(iso)} bytes, {len(paks)} pak(s)")
    for n, blob in enumerate(paks):
        p = os.path.join(dest, f"carved{n}.pak")
        with open(p, "wb") as w:
            w.write(blob)
        print(f"  carved{n}.pak {len(blob)} bytes")
