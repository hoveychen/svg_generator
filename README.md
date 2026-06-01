# generate_svg

An LLM-driven SVG generator that ports the prompt-engineering philosophy of
[MineBench](https://github.com/) (the voxel-build arena) to 2D vector art.

MineBench never asks the model for raw voxel coordinates blind — it wraps the
request in a heavy system prompt (judging criteria, failure modes, build-order
discipline, "think before you draw") and gives the model a constrained way to
emit geometry. `generate_svg` applies the same idea to SVG: instead of a thin
"draw an SVG of X" one-liner, it hands the model a rigorous art-director brief
and a strict output contract, then validates and repairs the result.

It is a single Go binary that shells out to the `claude` CLI (`claude -p`), so
there is no API key handling and no SDK — it reuses whatever authentication
your `claude` command already has.

## Usage

```sh
generate_svg -p "Generate a cute boat floating on a riverside" -o sample.svg
```

Flags:

| flag | default | meaning |
|------|---------|---------|
| `-p` | (required) | the build request / what to draw |
| `-o` | (required) | output `.svg` file path |
| `-m` | claude default | model alias passed to `claude --model` (e.g. `opus`, `sonnet`) |
| `--retries` | `3` | max repair attempts when output is invalid |
| `--min-elements` | `8` | reject lazy builds with fewer drawable elements |
| `--canvas` | `1024` | square viewBox size hinted to the model |
| `--png` | `false` | also render a PNG preview next to the SVG (needs `rsvg-convert` or macOS `qlmanage`) |
| `--png-size` | `0` | PNG preview pixel size; `0` = use `--canvas` |
| `-v` | `false` | verbose: print the prompt and each attempt to stderr |

With `--png`, `boat.svg` also produces `boat.png` (renderer chosen automatically:
`rsvg-convert` if present, otherwise macOS `qlmanage`). A preview failure is only
a warning — the SVG is still written.

## Requirements

- Go 1.22+ to build
- the `claude` CLI on `PATH`, already authenticated
- optional: `rsvg-convert` or macOS `qlmanage` for `--png` previews

## Install

```sh
go install github.com/hoveychen/svg_generator/cmd/generate_svg@latest
```

## How it works

1. **Brief** — builds a MineBench-style system prompt: an art-director rubric
   (recognizability, composition, depth via layering, proportion, color
   harmony, abundant intentional detail), explicit failure modes to avoid
   (generic AI clipart, flat shapes, no scene, uniform detail), a build order
   (background → subject silhouette → secondary forms → details/atmosphere),
   and a strict "output ONLY raw SVG" contract.
2. **Generate** — invokes `claude -p` with that system prompt and the request.
3. **Extract & validate** — pulls the `<svg>…</svg>` out of the response,
   checks it parses as XML, has a `viewBox`, and clears the element-count floor.
4. **Repair** — on failure, re-prompts with the validation error and the
   previous output (mirroring MineBench's repair loop), up to `--retries` times.

## License

MIT
