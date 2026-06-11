"""Load, validate, and resolve NexMap blueprints.

A blueprint is the LLM-authored JSON that describes *what* a map should be.
This module validates it against the schema, checks graph connectivity,
and resolves semantic fields (e.g. size='large') into concrete Quake-unit
ranges that the layout compiler can consume.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from pathlib import Path

# ---------------------------------------------------------------------------
# Semantic -> concrete resolution tables
# ---------------------------------------------------------------------------

# Room footprint ranges (min_side, max_side) in Quake units.
SIZE_RANGES: dict[str, tuple[int, int]] = {
    "small":  (256, 384),
    "medium": (384, 640),
    "large":  (640, 896),
    "huge":   (896, 1024),
}

# Floor elevation ranges (z0_min, z0_max).
ELEVATION_RANGES: dict[str, tuple[int, int]] = {
    "ground": (0, 0),
    "low":    (-128, -64),
    "mid":    (64, 192),
    "high":   (256, 384),
    "varied": (-128, 384),
}

# Floor-to-ceiling heights.
CEILING_HEIGHTS: dict[str, int] = {
    "low":       128,
    "normal":    192,
    "tall":      256,
    "cavernous": 320,
}

# Connection widths in Quake units.
CONNECTION_WIDTHS: dict[str, int] = {
    "narrow": 64,
    "normal": 96,
    "wide":   160,
}

# Hazard pool coverage as fraction of room floor area.
HAZARD_COVERAGE: dict[str, float] = {
    "small":  0.15,
    "medium": 0.30,
    "large":  0.50,
}

# Hazard type -> texture.
HAZARD_TEXTURES: dict[str, str] = {
    "lava":  "*lava1",
    "water": "*04water1",
    "slime": "*slime",
}

SCHEMA_PATH = Path(__file__).parent / "blueprint_schema.json"


# ---------------------------------------------------------------------------
# Resolved dataclasses — concrete values ready for the compiler
# ---------------------------------------------------------------------------

@dataclass
class ResolvedItem:
    classname: str
    placement: str  # center, corner, wall, elevated, hidden, near_hazard, any
    intent: str
    count: int


@dataclass
class ResolvedHazard:
    type: str          # lava, water, slime
    texture: str       # *lava1, etc.
    coverage: float    # 0.0-1.0 fraction of floor area


@dataclass
class ResolvedRoom:
    id: str
    role: str
    label: str
    size_min: int      # minimum side length (Quake units)
    size_max: int      # maximum side length
    shape: str         # square, wide, tall_narrow, l_shape, any
    elevation_min: int # z0 range low
    elevation_max: int # z0 range high
    ceiling_height: int
    hazard: ResolvedHazard | None
    items: list[ResolvedItem]
    tags: list[str]


@dataclass
class ResolvedConnection:
    from_id: str
    to_id: str
    type: str           # corridor, wide_opening, stairs, drop, teleporter, door
    bidirectional: bool
    width: int          # Quake units
    intent: str


@dataclass
class ResolvedFlow:
    primary_loop: list[str]
    secondary_loops: list[list[str]]
    choke_points: list[str]
    sightlines: list[dict[str, str]]


@dataclass
class ResolvedBlueprint:
    """Fully resolved blueprint ready for the layout compiler."""
    version: int
    name: str
    author: str
    description: str
    theme: str
    gamemode: str
    player_count_min: int
    player_count_max: int
    arena_size: int
    design_notes: str
    rooms: list[ResolvedRoom]
    connections: list[ResolvedConnection]
    flow: ResolvedFlow | None
    textures: dict[str, str]

    def room_by_id(self, room_id: str) -> ResolvedRoom | None:
        for r in self.rooms:
            if r.id == room_id:
                return r
        return None

    def room_ids(self) -> set[str]:
        return {r.id for r in self.rooms}

    def adjacency(self) -> dict[str, list[str]]:
        """Build adjacency list from connections."""
        adj: dict[str, list[str]] = {r.id: [] for r in self.rooms}
        for c in self.connections:
            adj[c.from_id].append(c.to_id)
            if c.bidirectional:
                adj[c.to_id].append(c.from_id)
        return adj


# ---------------------------------------------------------------------------
# Validation
# ---------------------------------------------------------------------------

class BlueprintError(Exception):
    """Raised when a blueprint fails validation."""


def _validate_structure(data: dict) -> list[str]:
    """Basic structural checks (no jsonschema dependency required)."""
    errors = []

    if data.get("version") != 1:
        errors.append("version must be 1")

    meta = data.get("meta")
    if not isinstance(meta, dict):
        errors.append("missing 'meta' object")
    else:
        for key in ("name", "theme", "gamemode"):
            if not meta.get(key):
                errors.append(f"meta.{key} is required")

    rooms = data.get("rooms")
    if not isinstance(rooms, list) or len(rooms) < 2:
        errors.append("need at least 2 rooms")
    else:
        ids = set()
        for i, room in enumerate(rooms):
            rid = room.get("id", "")
            if not rid:
                errors.append(f"rooms[{i}] missing id")
            elif rid in ids:
                errors.append(f"duplicate room id: {rid}")
            ids.add(rid)
            if not room.get("role"):
                errors.append(f"rooms[{i}] missing role")

    conns = data.get("connections")
    if not isinstance(conns, list) or len(conns) < 1:
        errors.append("need at least 1 connection")

    return errors


def _validate_graph(data: dict) -> list[str]:
    """Check that connections reference valid rooms and the graph is connected."""
    errors = []
    room_ids = {r["id"] for r in data.get("rooms", [])}

    for i, conn in enumerate(data.get("connections", [])):
        for end in ("from", "to"):
            rid = conn.get(end, "")
            if rid not in room_ids:
                errors.append(
                    f"connections[{i}].{end} references unknown room '{rid}'"
                )

    # BFS connectivity check (bidirectional edges by default).
    adj: dict[str, list[str]] = {rid: [] for rid in room_ids}
    for conn in data.get("connections", []):
        f, t = conn.get("from", ""), conn.get("to", "")
        if f in room_ids and t in room_ids:
            adj[f].append(t)
            if conn.get("bidirectional", True):
                adj[t].append(f)

    if room_ids:
        start = next(iter(room_ids))
        visited: set[str] = set()
        queue = [start]
        while queue:
            node = queue.pop(0)
            if node in visited:
                continue
            visited.add(node)
            queue.extend(adj.get(node, []))

        unreachable = room_ids - visited
        if unreachable:
            errors.append(
                f"unreachable rooms (disconnected graph): {sorted(unreachable)}"
            )

    # Validate flow references if present.
    flow = data.get("flow")
    if flow:
        for loop_name in ("primary_loop", "secondary_loops"):
            loops = flow.get(loop_name, [])
            if loop_name == "primary_loop":
                loops = [loops] if loops else []
            for loop in loops:
                for rid in loop:
                    if rid not in room_ids:
                        errors.append(
                            f"flow.{loop_name} references unknown room '{rid}'"
                        )

    return errors


def validate(data: dict) -> list[str]:
    """Validate a raw blueprint dict. Returns list of error strings (empty = valid)."""
    errors = _validate_structure(data)
    if not errors:
        errors.extend(_validate_graph(data))
    return errors


# ---------------------------------------------------------------------------
# Resolution — semantic values -> concrete numbers
# ---------------------------------------------------------------------------

def _resolve_room(raw: dict) -> ResolvedRoom:
    size = raw.get("size", "medium")
    size_min, size_max = SIZE_RANGES.get(size, SIZE_RANGES["medium"])

    elev = raw.get("elevation", "ground")
    elev_min, elev_max = ELEVATION_RANGES.get(elev, ELEVATION_RANGES["ground"])

    ceiling = raw.get("ceiling", "normal")
    ceiling_h = CEILING_HEIGHTS.get(ceiling, CEILING_HEIGHTS["normal"])

    hazard = None
    raw_hazard = raw.get("hazard")
    if raw_hazard:
        htype = raw_hazard["type"]
        coverage = HAZARD_COVERAGE.get(
            raw_hazard.get("coverage", "medium"), 0.30
        )
        hazard = ResolvedHazard(
            type=htype,
            texture=HAZARD_TEXTURES[htype],
            coverage=coverage,
        )

    items = []
    for raw_item in raw.get("items", []):
        items.append(ResolvedItem(
            classname=raw_item["classname"],
            placement=raw_item.get("placement", "any"),
            intent=raw_item.get("intent", ""),
            count=raw_item.get("count", 1),
        ))

    return ResolvedRoom(
        id=raw["id"],
        role=raw["role"],
        label=raw.get("label", raw["id"]),
        size_min=size_min,
        size_max=size_max,
        shape=raw.get("shape", "any"),
        elevation_min=elev_min,
        elevation_max=elev_max,
        ceiling_height=ceiling_h,
        hazard=hazard,
        items=items,
        tags=raw.get("tags", []),
    )


def _resolve_connection(raw: dict) -> ResolvedConnection:
    width_key = raw.get("width", "normal")
    return ResolvedConnection(
        from_id=raw["from"],
        to_id=raw["to"],
        type=raw["type"],
        bidirectional=raw.get("bidirectional", True),
        width=CONNECTION_WIDTHS.get(width_key, 96),
        intent=raw.get("intent", ""),
    )


def _resolve_flow(raw: dict | None) -> ResolvedFlow | None:
    if not raw:
        return None
    return ResolvedFlow(
        primary_loop=raw.get("primary_loop", []),
        secondary_loops=raw.get("secondary_loops", []),
        choke_points=raw.get("choke_points", []),
        sightlines=raw.get("sightlines", []),
    )


def resolve(data: dict) -> ResolvedBlueprint:
    """Resolve a validated blueprint dict into concrete compiler-ready values."""
    meta = data["meta"]
    pc = meta.get("player_count", {})

    return ResolvedBlueprint(
        version=data["version"],
        name=meta["name"],
        author=meta.get("author", "NexMap LLM"),
        description=meta.get("description", ""),
        theme=meta.get("custom_theme", meta["theme"])
            if meta["theme"] == "custom" else meta["theme"],
        gamemode=meta["gamemode"],
        player_count_min=pc.get("min", 2),
        player_count_max=pc.get("max", 8),
        arena_size=meta.get("arena_size", 3072),
        design_notes=meta.get("design_notes", ""),
        rooms=[_resolve_room(r) for r in data["rooms"]],
        connections=[_resolve_connection(c) for c in data["connections"]],
        flow=_resolve_flow(data.get("flow")),
        textures=data.get("textures", {}),
    )


# ---------------------------------------------------------------------------
# Public API
# ---------------------------------------------------------------------------

def load_blueprint(path: str | Path) -> ResolvedBlueprint:
    """Load a blueprint JSON file, validate, and resolve to concrete values.

    Raises BlueprintError on validation failure.
    """
    path = Path(path)
    with open(path) as f:
        data = json.load(f)

    errors = validate(data)
    if errors:
        raise BlueprintError(
            f"Blueprint validation failed ({path}):\n"
            + "\n".join(f"  - {e}" for e in errors)
        )

    return resolve(data)


def load_blueprint_str(json_str: str) -> ResolvedBlueprint:
    """Load a blueprint from a JSON string (e.g. direct LLM output)."""
    data = json.loads(json_str)
    errors = validate(data)
    if errors:
        raise BlueprintError(
            "Blueprint validation failed:\n"
            + "\n".join(f"  - {e}" for e in errors)
        )
    return resolve(data)
