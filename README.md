# NexMap

Quake 1 map generator. Produces `.map` and `.bsp` files from LLM-authored
blueprint JSON or via procedural BSP subdivision.

## Install

```bash
cd src/nexmap
go build -o nexmap .
```

Optional: place [ericw-tools](https://github.com/ericwa/ericw-tools) binaries
(`qbsp`, `vis`, `light`) in `vendor/ericw-tools/bin/` or on your `PATH` for
the `--compile` flag.

## Usage

### Procgen mode (no blueprint needed)

```bash
# Generate a random DM map (.map file)
nexmap -seed 42

# Control room count and arena size
nexmap -seed 42 -rooms 4 -arena-size 4096

# Compile to .bsp with ericw-tools
nexmap -seed 42 -compile
```

### Blueprint mode (LLM-authored JSON)

```bash
# Generate from a blueprint
nexmap -b examples/blueprints/the_cistern.json

# Compile to .bsp
nexmap -b examples/blueprints/the_cistern.json -compile
```

### Direct BSP output (experimental, no external tools)

```bash
nexmap -b examples/blueprints/the_cistern.json -bsp
```

The built-in BSP compiler is functional but not yet production-quality.
Use `--compile` with ericw-tools for best results.

### Extract shareware maps as blueprints

```bash
nexmap -extract-blueprints examples/shareware/
```

Downloads the Quake shareware (`quake106.zip`), extracts `pak0.pak`, and
converts the Episode 1 maps to blueprint JSON files. Requires network
access on first run; cached to `~/.cache/nexmap/`.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-b`, `--blueprint` | | Blueprint JSON file (omit for procgen) |
| `-seed` | random | RNG seed |
| `-rooms` | 3 | BSP subdivision depth (procgen only) |
| `-arena-size` | 3072 | Arena side length in Quake units (procgen only) |
| `-o` | auto | Output file path |
| `--output-dir` | `.` | Output directory |
| `--bsp` | false | Direct BSP output (no external tools) |
| `--compile` | false | Compile via ericw-tools (qbsp/vis/light) |
| `--extract-blueprints` | | Extract shareware maps to blueprint dir |

## Blueprint schema

See `docs/blueprint_schema.json` and `examples/blueprints/the_cistern.json`
for the LLM-facing blueprint format. Blueprints describe rooms, connections,
items, hazards, and flow — the compiler handles geometry.

## Textures

On first run with `--bsp` or `--compile`, NexMap downloads the Quake 1.06
shareware archive and extracts 329 real miptex entries from `pak0.pak`.
These are cached at `~/.cache/nexmap/` and embedded in generated maps.
Falls back to synthetic textures if offline.

## Project structure

```
src/nexmap/       Go source (the entire application)
examples/
  blueprints/     LLM-authored blueprint examples
  shareware/      Extracted shareware map blueprints
docs/
  blueprint_schema.json
  oblige-themes-plan.md
vendor/
  ericw-tools/bin/   (gitignored, user-provided)
```
