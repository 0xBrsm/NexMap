"""Blueprint compiler: ResolvedBlueprint -> Layout -> .map geometry.

Bridges the LLM-authored blueprint into the mapgen compiler pipeline.
Uses grid-based room placement with automatic corridor routing.
Non-adjacent connections become teleporters.
"""

from __future__ import annotations

import random
from collections import defaultdict
from dataclasses import dataclass, field

from .blueprint import ResolvedBlueprint, ResolvedConnection, ResolvedRoom
from .brush import MapFile, axis_aligned_box
from .layout import (
    CORRIDOR_HEADROOM, CORRIDOR_WIDTH, FLOOR, FLOOR_ELEVATION_STEP,
    POOL_BORDER, POOL_DEPTH, POOL_WALL,
    Corridor, Layout, Pool, Room, Split,
    _build_gap_fills, _constrain_elevations,
    build_corridor_brushes, build_room_brushes,
)
from .textures import MAP_TEXTURES

# Entity placement constants (match entities.py).
ITEM_Z_ABOVE_FLOOR = 32
MARGIN = 48
MIN_ENTITY_SPACING = 64

# Grid directions: (dcol, drow).
_DIRS = [(1, 0), (-1, 0), (0, 1), (0, -1)]


# ---------------------------------------------------------------------------
# Grid info
# ---------------------------------------------------------------------------

@dataclass
class GridInfo:
    """Grid layout metadata produced during blueprint compilation."""

    col_x0: list[int]       # world-X of each column's left edge
    col_widths: list[int]
    row_y0: list[int]       # world-Y of each row's bottom edge
    row_heights: list[int]
    cell_map: dict[tuple[int, int], str]   # (col, row) -> room_id
    n_cols: int
    n_rows: int

    @property
    def global_x0(self) -> int:
        return self.col_x0[0]

    @property
    def global_x1(self) -> int:
        return self.col_x0[-1] + self.col_widths[-1]

    @property
    def global_y0(self) -> int:
        return self.row_y0[0]

    @property
    def global_y1(self) -> int:
        return self.row_y0[-1] + self.row_heights[-1]


# ---------------------------------------------------------------------------
# Phase 1 — Grid assignment
# ---------------------------------------------------------------------------

def _physical_adj(bp: ResolvedBlueprint) -> dict[str, list[str]]:
    """Adjacency list for physical (non-teleporter) connections."""
    adj: dict[str, list[str]] = defaultdict(list)
    for conn in bp.connections:
        if conn.type == "teleporter":
            continue
        adj[conn.from_id].append(conn.to_id)
        if conn.bidirectional:
            adj[conn.to_id].append(conn.from_id)
    return adj


def _assign_grid(bp: ResolvedBlueprint) -> dict[str, tuple[int, int]]:
    """Assign each room a (col, row) grid cell, maximising adjacency."""
    adj = _physical_adj(bp)

    placed: dict[str, tuple[int, int]] = {}
    occupied: set[tuple[int, int]] = set()

    start = bp.rooms[0].id
    placed[start] = (0, 0)
    occupied.add((0, 0))
    queue = [start]

    while queue:
        current = queue.pop(0)
        cc, cr = placed[current]

        for nbr in adj.get(current, []):
            if nbr in placed:
                continue

            # Candidate positions: adjacent to *any* already-placed neighbour.
            candidates: list[tuple[int, int]] = []
            for adj_id in adj.get(nbr, []):
                if adj_id in placed:
                    ac, ar = placed[adj_id]
                    for dc, dr in _DIRS:
                        pos = (ac + dc, ar + dr)
                        if pos not in occupied:
                            candidates.append(pos)
            # Fallback: adjacent to current.
            if not candidates:
                for dc, dr in _DIRS:
                    pos = (cc + dc, cr + dr)
                    if pos not in occupied:
                        candidates.append(pos)

            # Score each candidate by adjacency to connected rooms.
            best_pos: tuple[int, int] | None = None
            best_score = -1
            for pos in candidates:
                score = sum(
                    1
                    for a in adj.get(nbr, [])
                    if a in placed
                    and abs(pos[0] - placed[a][0]) + abs(pos[1] - placed[a][1]) == 1
                )
                if score > best_score:
                    best_score = score
                    best_pos = pos

            if best_pos is None:
                # Expand radius search.
                for radius in range(1, 20):
                    for dc in range(-radius, radius + 1):
                        for dr in range(-radius, radius + 1):
                            p = (cc + dc, cr + dr)
                            if p not in occupied:
                                best_pos = p
                                break
                        if best_pos:
                            break
                    if best_pos:
                        break

            assert best_pos is not None
            placed[nbr] = best_pos
            occupied.add(best_pos)
            queue.append(nbr)

    return placed


def _adjacency_axis(a: tuple[int, int], b: tuple[int, int]) -> str | None:
    """Return 'x' if cells are column-adjacent, 'y' if row-adjacent."""
    dc = b[0] - a[0]
    dr = b[1] - a[1]
    if abs(dc) == 1 and dr == 0:
        return "x"
    if dc == 0 and abs(dr) == 1:
        return "y"
    return None


# ---------------------------------------------------------------------------
# Phase 2 — Room placement & Layout construction
# ---------------------------------------------------------------------------

def compile_blueprint(
    bp: ResolvedBlueprint,
    rng: random.Random,
) -> tuple[Layout, dict[str, int], list[ResolvedConnection], GridInfo]:
    """Convert a ResolvedBlueprint into a Layout.

    Returns (layout, id_to_idx, teleporter_connections, grid_info).
    """
    grid = _assign_grid(bp)

    # Normalise to (0, 0) origin.
    min_c = min(c for c, _ in grid.values())
    min_r = min(r for _, r in grid.values())
    grid = {rid: (c - min_c, r - min_r) for rid, (c, r) in grid.items()}

    n_cols = max(c for c, _ in grid.values()) + 1
    n_rows = max(r for _, r in grid.values()) + 1

    bp_room_map = {r.id: r for r in bp.rooms}

    # ------ cell sizes ------
    col_widths = [256] * n_cols
    row_heights = [256] * n_rows

    for rid, (c, r) in grid.items():
        br = bp_room_map[rid]
        room_w = rng.randint(br.size_min, br.size_max)
        room_h = rng.randint(br.size_min, br.size_max)
        if br.shape == "wide":
            room_w = min(br.size_max, max(room_w, int(room_h * 1.5)))
        elif br.shape == "tall_narrow":
            room_h = min(br.size_max, max(room_h, int(room_w * 1.5)))
        col_widths[c] = max(col_widths[c], room_w)
        row_heights[r] = max(row_heights[r], room_h)

    # ------ world coordinates ------
    col_x0: list[int] = []
    x = 0
    for c in range(n_cols):
        col_x0.append(x)
        x += col_widths[c]
        if c < n_cols - 1:
            x += CORRIDOR_WIDTH

    row_y0: list[int] = []
    y = 0
    for r in range(n_rows):
        row_y0.append(y)
        y += row_heights[r]
        if r < n_rows - 1:
            y += CORRIDOR_WIDTH

    # Centre around origin.
    x_off = -x // 2
    y_off = -y // 2
    col_x0 = [v + x_off for v in col_x0]
    row_y0 = [v + y_off for v in row_y0]

    cell_map = {(c, r): rid for rid, (c, r) in grid.items()}

    gi = GridInfo(
        col_x0=col_x0, col_widths=col_widths,
        row_y0=row_y0, row_heights=row_heights,
        cell_map=cell_map, n_cols=n_cols, n_rows=n_rows,
    )

    # ------ rooms ------
    rooms: list[Room] = []
    id_to_idx: dict[str, int] = {}

    for rid, (c, r) in grid.items():
        br = bp_room_map[rid]
        rx0 = col_x0[c]
        ry0 = row_y0[r]
        rx1 = rx0 + col_widths[c]
        ry1 = ry0 + row_heights[r]

        elev_steps = list(range(
            br.elevation_min, br.elevation_max + 1, FLOOR_ELEVATION_STEP,
        ))
        z0 = rng.choice(elev_steps) if elev_steps else br.elevation_min
        z1 = z0 + br.ceiling_height

        id_to_idx[rid] = len(rooms)
        rooms.append(Room(rx0, ry0, rx1, ry1, z0, z1))

    # ------ splits (localised per cell-pair) ------
    splits: list[Split] = []

    # Column boundaries.
    for c in range(n_cols - 1):
        sp = col_x0[c] + col_widths[c]
        for r in range(n_rows):
            left = cell_map.get((c, r))
            right = cell_map.get((c + 1, r))
            if left is not None or right is not None:
                splits.append(Split(
                    axis="x", pos=sp,
                    perp_lo=row_y0[r],
                    perp_hi=row_y0[r] + row_heights[r],
                ))

    # Row boundaries.
    for r in range(n_rows - 1):
        sp = row_y0[r] + row_heights[r]
        for c in range(n_cols):
            bot = cell_map.get((c, r))
            top = cell_map.get((c, r + 1))
            if bot is not None or top is not None:
                splits.append(Split(
                    axis="y", pos=sp,
                    perp_lo=col_x0[c],
                    perp_hi=col_x0[c] + col_widths[c],
                ))

    # ------ corridors ------
    corridors: list[Corridor] = []
    teleporter_conns: list[ResolvedConnection] = []

    for conn in bp.connections:
        if conn.from_id not in grid or conn.to_id not in grid:
            continue

        pos_a = grid[conn.from_id]
        pos_b = grid[conn.to_id]
        axis = _adjacency_axis(pos_a, pos_b)

        if axis is None or conn.type == "teleporter":
            teleporter_conns.append(conn)
            continue

        idx_a = id_to_idx[conn.from_id]
        idx_b = id_to_idx[conn.to_id]
        ra, rb = rooms[idx_a], rooms[idx_b]

        # Ensure a is the "lower-coordinate" room.
        if axis == "x" and ra.x1 > rb.x0:
            ra, rb = rb, ra
            idx_a, idx_b = idx_b, idx_a
        elif axis == "y" and ra.y1 > rb.y0:
            ra, rb = rb, ra
            idx_a, idx_b = idx_b, idx_a

        corr_w = min(conn.width, CORRIDOR_WIDTH)
        low_z = min(ra.z0, rb.z0)
        high_z = max(ra.z0, rb.z0)
        ceil_z = min(high_z + CORRIDOR_HEADROOM, ra.z1, rb.z1)

        if axis == "x":
            ov0 = max(ra.y0, rb.y0)
            ov1 = min(ra.y1, rb.y1)
            span = ov1 - ov0
            if span < corr_w:
                cy = (ov0 + ov1) // 2
            else:
                cy = rng.randint(ov0 + corr_w // 2, ov1 - corr_w // 2)
            corridors.append(Corridor(
                room_a=idx_a, room_b=idx_b,
                x0=ra.x1, y0=cy - corr_w // 2,
                x1=rb.x0, y1=cy + corr_w // 2,
                z0=low_z, z1=ceil_z,
                z0_a=rooms[idx_a].z0, z0_b=rooms[idx_b].z0,
                axis="x",
            ))
        else:
            ov0 = max(ra.x0, rb.x0)
            ov1 = min(ra.x1, rb.x1)
            span = ov1 - ov0
            if span < corr_w:
                cx = (ov0 + ov1) // 2
            else:
                cx = rng.randint(ov0 + corr_w // 2, ov1 - corr_w // 2)
            corridors.append(Corridor(
                room_a=idx_a, room_b=idx_b,
                x0=cx - corr_w // 2, y0=ra.y1,
                x1=cx + corr_w // 2, y1=rb.y0,
                z0=low_z, z1=ceil_z,
                z0_a=rooms[idx_a].z0, z0_b=rooms[idx_b].z0,
                axis="y",
            ))

    # ------ constrain elevations ------
    _constrain_elevations(rooms, corridors)

    # ------ pools from blueprint ------
    pools: dict[int, Pool] = {}
    for br in bp.rooms:
        if br.hazard is None:
            continue
        idx = id_to_idx.get(br.id)
        if idx is None:
            continue
        room = rooms[idx]
        if room.width < 2 * POOL_BORDER + 96 or room.height < 2 * POOL_BORDER + 96:
            continue

        max_pw = min(256, room.width - 2 * POOL_BORDER)
        max_ph = min(256, room.height - 2 * POOL_BORDER)
        pw = max(96, int(max_pw * br.hazard.coverage))
        ph = max(96, int(max_ph * br.hazard.coverage))
        px = room.x0 + (room.width - pw) // 2
        py = room.y0 + (room.height - ph) // 2
        pools[idx] = Pool(px, py, px + pw, py + ph, br.hazard.texture)

    layout = Layout(rooms=rooms, corridors=corridors, splits=splits, pools=pools)
    return layout, id_to_idx, teleporter_conns, gi


# ---------------------------------------------------------------------------
# Geometry building
# ---------------------------------------------------------------------------

def _compute_z_range(layout: Layout) -> tuple[int, int]:
    """Global z extent including pool pits."""
    z_lo = min(r.z0 for r in layout.rooms) - FLOOR
    z_hi = max(r.z1 for r in layout.rooms) + FLOOR
    for c in layout.corridors:
        z_lo = min(z_lo, c.z0 - FLOOR)
        z_hi = max(z_hi, c.z1 + FLOOR)
    for idx, pool in layout.pools.items():
        pit_bot = layout.rooms[idx].z0 - POOL_DEPTH - FLOOR
        z_lo = min(z_lo, pit_bot)
    return z_lo, z_hi


def _build_shell(m: MapFile, gi: GridInfo, z_lo: int, z_hi: int) -> None:
    """Outer shell covering the full grid extent."""
    x0 = gi.global_x0
    x1 = gi.global_x1
    y0 = gi.global_y0
    y1 = gi.global_y1
    s = 32
    tex = MAP_TEXTURES.shell

    m.add_brush(axis_aligned_box(x0 - s, y0 - s, z_lo - s, x1 + s, y1 + s, z_lo, tex))
    m.add_brush(axis_aligned_box(x0 - s, y0 - s, z_hi, x1 + s, y1 + s, z_hi + s, tex))
    m.add_brush(axis_aligned_box(x0 - s, y0 - s, z_lo, x0, y1 + s, z_hi, tex))
    m.add_brush(axis_aligned_box(x1, y0 - s, z_lo, x1 + s, y1 + s, z_hi, tex))
    m.add_brush(axis_aligned_box(x0, y0 - s, z_lo, x1, y0, z_hi, tex))
    m.add_brush(axis_aligned_box(x0, y1, z_lo, x1, y1 + s, z_hi, tex))


def _build_empty_cell_fills(
    m: MapFile, gi: GridInfo, z_lo: int, z_hi: int,
) -> None:
    """Fill empty grid cells with solid to prevent void leaks."""
    tex = MAP_TEXTURES.fill
    for c in range(gi.n_cols):
        for r in range(gi.n_rows):
            if (c, r) in gi.cell_map:
                continue
            x0 = gi.col_x0[c]
            y0 = gi.row_y0[r]
            x1 = x0 + gi.col_widths[c]
            y1 = y0 + gi.row_heights[r]
            m.add_brush(axis_aligned_box(x0, y0, z_lo, x1, y1, z_hi, tex))


def _build_intersection_fills(
    m: MapFile, gi: GridInfo, z_lo: int, z_hi: int,
) -> None:
    """Fill the CW x CW squares at grid line intersections."""
    tex = MAP_TEXTURES.fill
    for c in range(gi.n_cols - 1):
        for r in range(gi.n_rows - 1):
            ix0 = gi.col_x0[c] + gi.col_widths[c]
            iy0 = gi.row_y0[r] + gi.row_heights[r]
            ix1 = ix0 + CORRIDOR_WIDTH
            iy1 = iy0 + CORRIDOR_WIDTH
            m.add_brush(axis_aligned_box(ix0, iy0, z_lo, ix1, iy1, z_hi, tex))


def build_blueprint_geometry(
    m: MapFile, layout: Layout, gi: GridInfo,
) -> None:
    """Emit all world geometry for a blueprint-compiled layout."""
    z_lo, z_hi = _compute_z_range(layout)

    _build_shell(m, gi, z_lo, z_hi)
    _build_gap_fills(m, layout, z_lo, z_hi)
    _build_intersection_fills(m, gi, z_lo, z_hi)
    _build_empty_cell_fills(m, gi, z_lo, z_hi)

    for i, room in enumerate(layout.rooms):
        build_room_brushes(m, room, pool=layout.pools.get(i))

    for corridor in layout.corridors:
        build_corridor_brushes(m, corridor, layout.rooms)


# ---------------------------------------------------------------------------
# Entity placement from blueprint
# ---------------------------------------------------------------------------

def _item_z(room: Room) -> int:
    return room.z0 + ITEM_Z_ABOVE_FLOOR


def _pick_xy(
    rng: random.Random,
    room: Room,
    pool: Pool | None,
    placement: str,
    placed: list[tuple[int, int]],
) -> tuple[int, int]:
    """Pick XY for an entity respecting the placement hint."""
    m = MARGIN

    def _valid(x: int, y: int) -> bool:
        if pool and pool.x0 <= x <= pool.x1 and pool.y0 <= y <= pool.y1:
            return False
        for px, py in placed:
            if abs(x - px) < MIN_ENTITY_SPACING and abs(y - py) < MIN_ENTITY_SPACING:
                return False
        return True

    if placement == "center":
        if _valid(room.cx, room.cy):
            return room.cx, room.cy

    elif placement == "corner":
        corners = [
            (room.x0 + m, room.y0 + m),
            (room.x0 + m, room.y1 - m),
            (room.x1 - m, room.y0 + m),
            (room.x1 - m, room.y1 - m),
        ]
        rng.shuffle(corners)
        for x, y in corners:
            if _valid(x, y):
                return x, y

    elif placement == "wall":
        sides = [
            (rng.randint(room.x0 + m, room.x1 - m), room.y0 + m),
            (rng.randint(room.x0 + m, room.x1 - m), room.y1 - m),
            (room.x0 + m, rng.randint(room.y0 + m, room.y1 - m)),
            (room.x1 - m, rng.randint(room.y0 + m, room.y1 - m)),
        ]
        rng.shuffle(sides)
        for x, y in sides:
            if _valid(x, y):
                return x, y

    elif placement == "near_hazard" and pool:
        for _ in range(20):
            edge = rng.choice(["n", "s", "e", "w"])
            if edge == "n":
                x = rng.randint(pool.x0, pool.x1)
                y = pool.y1 + 32
            elif edge == "s":
                x = rng.randint(pool.x0, pool.x1)
                y = pool.y0 - 32
            elif edge == "e":
                x = pool.x1 + 32
                y = rng.randint(pool.y0, pool.y1)
            else:
                x = pool.x0 - 32
                y = rng.randint(pool.y0, pool.y1)
            x = max(room.x0 + m, min(room.x1 - m, x))
            y = max(room.y0 + m, min(room.y1 - m, y))
            if _valid(x, y):
                return x, y

    elif placement == "hidden":
        # Tuck into the least-accessible corner.
        corners = [
            (room.x0 + m, room.y0 + m),
            (room.x0 + m, room.y1 - m),
            (room.x1 - m, room.y0 + m),
            (room.x1 - m, room.y1 - m),
        ]
        # Pick corner farthest from centre.
        corners.sort(
            key=lambda p: abs(p[0] - room.cx) + abs(p[1] - room.cy),
            reverse=True,
        )
        for x, y in corners:
            if _valid(x, y):
                return x, y

    # Fallback: random.
    for _ in range(30):
        x = rng.randint(room.x0 + m, room.x1 - m)
        y = rng.randint(room.y0 + m, room.y1 - m)
        if _valid(x, y):
            return x, y

    return room.cx, room.cy


def _place_teleporters(
    m: MapFile,
    layout: Layout,
    id_to_idx: dict[str, int],
    teleporter_conns: list[ResolvedConnection],
    rng: random.Random,
    placed: list[tuple[int, int]],
) -> None:
    """Place trigger_teleport + info_teleport_destination pairs."""
    for i, conn in enumerate(teleporter_conns):
        src_idx = id_to_idx.get(conn.from_id)
        dst_idx = id_to_idx.get(conn.to_id)
        if src_idx is None or dst_idx is None:
            continue

        src_room = layout.rooms[src_idx]
        dst_room = layout.rooms[dst_idx]
        src_pool = layout.pools.get(src_idx)
        dst_pool = layout.pools.get(dst_idx)

        # Destination entity.
        dx, dy = _pick_xy(rng, dst_room, dst_pool, "any", placed)
        target_name = f"tele_dest_{i}"
        m.add_entity(
            "info_teleport_destination", (dx, dy, _item_z(dst_room)),
            targetname=target_name, angle=str(rng.choice([0, 90, 180, 270])),
        )
        placed.append((dx, dy))

        # Teleporter trigger (brush entity).
        sx, sy = _pick_xy(rng, src_room, src_pool, "any", placed)
        # Build a small trigger volume.
        tw = 64
        trigger_brush = axis_aligned_box(
            sx - tw // 2, sy - tw // 2, src_room.z0,
            sx + tw // 2, sy + tw // 2, src_room.z0 + 64,
            "*teleport",
        )
        from .brush import Entity
        trigger = Entity(properties={
            "classname": "trigger_teleport",
            "target": target_name,
        }, brushes=[trigger_brush])
        m.entities.append(trigger)
        placed.append((sx, sy))

        # Reverse direction if bidirectional.
        if conn.bidirectional:
            dx2, dy2 = _pick_xy(rng, src_room, src_pool, "any", placed)
            rev_name = f"tele_dest_{i}_rev"
            m.add_entity(
                "info_teleport_destination", (dx2, dy2, _item_z(src_room)),
                targetname=rev_name, angle=str(rng.choice([0, 90, 180, 270])),
            )
            placed.append((dx2, dy2))

            sx2, sy2 = _pick_xy(rng, dst_room, dst_pool, "any", placed)
            trigger2 = Entity(properties={
                "classname": "trigger_teleport",
                "target": rev_name,
            }, brushes=[axis_aligned_box(
                sx2 - tw // 2, sy2 - tw // 2, dst_room.z0,
                sx2 + tw // 2, sy2 + tw // 2, dst_room.z0 + 64,
                "*teleport",
            )])
            m.entities.append(trigger2)
            placed.append((sx2, sy2))


def populate_from_blueprint(
    m: MapFile,
    layout: Layout,
    bp: ResolvedBlueprint,
    id_to_idx: dict[str, int],
    teleporter_conns: list[ResolvedConnection],
    rng: random.Random,
) -> None:
    """Place all entities according to blueprint item specs."""
    placed: list[tuple[int, int]] = []
    has_player_start = False

    for br in bp.rooms:
        idx = id_to_idx.get(br.id)
        if idx is None:
            continue
        room = layout.rooms[idx]
        pool = layout.pools.get(idx)

        for item in br.items:
            for _ in range(item.count):
                if item.classname == "light":
                    # Lights don't need floor collision.
                    m.add_light(
                        (room.cx + rng.randint(-room.width // 4, room.width // 4),
                         room.cy + rng.randint(-room.height // 4, room.height // 4),
                         room.z1 - 16),
                        brightness=300,
                    )
                    continue

                x, y = _pick_xy(rng, room, pool, item.placement, placed)
                z = _item_z(room)

                if item.classname == "info_player_start":
                    has_player_start = True
                    m.add_entity(item.classname, (x, y, z), angle="0")
                elif item.classname == "info_player_deathmatch":
                    angle = str(rng.choice([0, 90, 180, 270]))
                    m.add_entity(item.classname, (x, y, z), angle=angle)
                else:
                    m.add_entity(item.classname, (x, y, z))
                placed.append((x, y))

    # Ensure info_player_start exists (required for map to load).
    if not has_player_start and layout.rooms:
        room = layout.rooms[0]
        m.add_entity("info_player_start", (room.cx, room.cy, _item_z(room)), angle="0")

    # Corridor lights.
    for c in layout.corridors:
        cx = (c.x0 + c.x1) // 2
        cy = (c.y0 + c.y1) // 2
        m.add_light((cx, cy, c.z1 - 8), brightness=200)

    # Teleporters for non-adjacent connections.
    _place_teleporters(m, layout, id_to_idx, teleporter_conns, rng, placed)


# ---------------------------------------------------------------------------
# High-level orchestrator
# ---------------------------------------------------------------------------

def generate_from_blueprint(
    bp: ResolvedBlueprint,
    rng: random.Random,
) -> MapFile:
    """Full pipeline: blueprint -> compiled .map."""
    layout, id_to_idx, tele_conns, gi = compile_blueprint(bp, rng)

    m = MapFile()
    m.worldspawn.properties["message"] = bp.name

    build_blueprint_geometry(m, layout, gi)
    populate_from_blueprint(m, layout, bp, id_to_idx, tele_conns, rng)

    return m
