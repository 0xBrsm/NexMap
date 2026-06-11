"""CLI entry point: python -m mapgen [options]"""

from __future__ import annotations

import argparse
import random
import sys
from pathlib import Path

from .brush import MapFile
from .compile import compile_map, find_tool
from .entities import populate
from .layout import build_layout, generate_layout
from .navcheck import validate_layout_graph
from .textures import materialize_texture_wad

MAX_ATTEMPTS = 10


def _run_procedural(args: argparse.Namespace) -> None:
    """Original procedural BSP-subdivision pipeline."""
    seed = args.seed if args.seed is not None else random.randint(0, 2**31 - 1)
    rng = random.Random(seed)

    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)

    map_path = Path(args.output) if args.output else output_dir / f"gen_{seed}.map"

    # Generate layout with connectivity validation and retry.
    best_layout = None
    best_unreachable = float("inf")

    for attempt in range(MAX_ATTEMPTS):
        layout = generate_layout(rng, arena_size=args.arena_size, max_depth=args.rooms)
        result = validate_layout_graph(layout)

        if result.connected:
            best_layout = layout
            break

        n_unreachable = len(result.unreachable_rooms)
        if n_unreachable < best_unreachable:
            best_unreachable = n_unreachable
            best_layout = layout

        if attempt < MAX_ATTEMPTS - 1:
            rng = random.Random(seed + attempt + 1)

    layout = best_layout  # type: ignore[assignment]
    nav = validate_layout_graph(layout)
    n_rooms = len(layout.rooms)
    n_corrs = len(layout.corridors)
    status = "connected" if nav.connected else f"{len(nav.unreachable_rooms)} unreachable"
    print(f"seed={seed}  rooms={n_rooms}  corridors={n_corrs}  {status}")

    m = MapFile()
    m.worldspawn.properties["message"] = f"gen_{seed}"
    build_layout(m, layout)
    populate(m, layout, rng)

    materialize_texture_wad(map_path.parent)

    with open(map_path, "w") as f:
        m.write(f)
    print(f"wrote {map_path}")

    if args.compile:
        if find_tool("qbsp") is None:
            print("error: qbsp not found on PATH. Install ericw-tools.", file=sys.stderr)
            sys.exit(1)
        bsp_path = compile_map(map_path, output_dir=output_dir)
        print(f"compiled {bsp_path}")


def _run_blueprint(args: argparse.Namespace) -> None:
    """Blueprint-driven pipeline: LLM-authored JSON -> .map."""
    from .blueprint import load_blueprint
    from .blueprint_compiler import generate_from_blueprint

    bp = load_blueprint(args.blueprint)
    seed = args.seed if args.seed is not None else random.randint(0, 2**31 - 1)
    rng = random.Random(seed)

    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)

    # Derive filename from blueprint name.
    slug = bp.name.lower().replace(" ", "_").replace("'", "")
    map_path = Path(args.output) if args.output else output_dir / f"{slug}.map"

    print(f"blueprint: {bp.name}  rooms={len(bp.rooms)}  "
          f"connections={len(bp.connections)}  theme={bp.theme}")

    m = generate_from_blueprint(bp, rng)

    materialize_texture_wad(map_path.parent)

    with open(map_path, "w") as f:
        m.write(f)
    print(f"wrote {map_path}  (seed={seed})")

    if args.compile:
        if find_tool("qbsp") is None:
            print("error: qbsp not found on PATH. Install ericw-tools.", file=sys.stderr)
            sys.exit(1)
        bsp_path = compile_map(map_path, output_dir=output_dir)
        print(f"compiled {bsp_path}")


def main(argv: list[str] | None = None) -> None:
    p = argparse.ArgumentParser(
        prog="mapgen",
        description="Quake 1 .map generator — procedural or blueprint-driven.",
    )
    p.add_argument("--seed", type=int, default=None, help="RNG seed (random if omitted)")
    p.add_argument("--output", "-o", type=str, default=None,
                   help="Output .map path")
    p.add_argument("--compile", action="store_true",
                   help="Compile to .bsp using ericw-tools (qbsp/vis/light)")
    p.add_argument("--output-dir", type=str, default=".",
                   help="Directory for output files (default: cwd)")

    # Blueprint mode.
    p.add_argument("--blueprint", "-b", type=str, default=None,
                   help="Path to a blueprint JSON file (enables blueprint mode)")

    # Procedural mode options.
    p.add_argument("--rooms", type=int, default=3,
                   help="Max BSP subdivision depth (procedural mode, default: 3)")
    p.add_argument("--arena-size", type=int, default=3072,
                   help="Arena side length in Quake units (procedural mode, default: 3072)")

    args = p.parse_args(argv)

    if args.blueprint:
        _run_blueprint(args)
    else:
        _run_procedural(args)


if __name__ == "__main__":
    main()
