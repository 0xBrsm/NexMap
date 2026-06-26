---
name: Playtest verdict on the Ashen Spire (helix) terraced tower
description: Why inflating room size to pass the areas gate produced monotonous boxes; what a "winding" tower actually needs
type: feedback
---

Playtested helix/"The Ashen Spire" v1 in NexQuake on the Pi (2026-06-25).
Cool concept, major problems:

1. **Rooms too large + monotonous** — the biggest issue. They were all
   identical sparse boxes (one central pillar each). ROOT CAUSE: I hit the
   gate's `areas` target by ENLARGING rooms (480 half-extent / 960 wide). That
   is the wrong lever. The id corpus hits areas ~245 with MANY SMALLER,
   CONNECTED, DIFFERENTIATED spaces — not big boxes.
2. **Stairs too steep, doorway too low** — the qlayout stairs connector ran a
   ~112 rise over the ~128 grid void = too steep, and the 128x112 opening read
   as a mouse-hole against oversized rooms.
3. **No winding feeling** — a 3x3 orthogonal grid of terraced rooms climbs but
   doesn't read as a spiral/tower. Big identical rooms kill any sense of turn.

**Why it matters:** this is the concrete proof that gate-pass != good. Five
green targets, bad map.
**How to apply:**
- Raise `areas` with MORE, SMALLER, VARIED spaces + internal structure
  (balconies, alcoves, multi-level), NEVER by scaling rooms up.
- Differentiate rooms individually — never template every room identically
  (same size, same single pillar). Vary size, shape, focus, prefab mix.
- For a "tower/winding" fantasy: a central open shaft with a spiral ascent and
  vertical sightlines (see up to the top / down to the base) reads as winding;
  a grid of stacked boxes does not.
- Give stairs longer runs + taller/wider openings; size openings relative to
  the room so they don't look like mouse-holes.

**Verified 2026-06-25 — room-monotony is NOT a geometric-entropy signal.**
Probed three ways: navmesh per-room entropy (label-prop segmentation INVERTED —
helix scored most-diverse), .map brush-signature entropy (helix scored HIGH
distinctness 0.82), .map translation-offset duplication (helix only a weak 2x
peak). Root cause: identical-SHAPED rooms have different brushwork because each
connects differently, so geometry decomposition is noisy; monotony is a
room-level semantic property. CONCLUSION: don't try to infer rooms from compiled
geometry. Measure room diversity at the qlayout AUTHORING layer where Room
objects are explicit (Layout._room_diversity_warnings) — reliable for gating our
own generation; threshold is design-reasoned, not corpus-auto-grounded.
