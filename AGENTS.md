# Agent Map

This file is the repository map for agentic work. Keep it short. Durable
knowledge belongs under `docs/`; long-lived constraints belong under
`docs/golden-rules/`.

## Start Here

- Project overview: [README.md](README.md)
- Documentation index: [docs/README.md](docs/README.md)
- Harness workflow: [docs/harness/README.md](docs/harness/README.md)
- Golden rules index: [docs/golden-rules/README.md](docs/golden-rules/README.md)

## Repository Shape

Lattigo is a Go library for RLWE-based homomorphic encryption. The dependency
direction is intentionally hierarchical:

1. `ring`: modular polynomial arithmetic, RNS, NTT, samplers.
2. `core`: generic cryptographic primitives (`rlwe`, `rgsw`).
3. `schemes`: concrete HE schemes (`ckks`, `bgv`, `bfv`).
4. `circuits`: higher-level homomorphic circuits.
5. `multiparty`: threshold and distributed protocols.
6. `examples`: runnable examples and usage templates.
7. `utils`: generic helpers used by the above layers.

Do not introduce dependencies that point back up this chain.

## Working Rules

- Read the package README nearest to the files you touch before editing.
- Prefer existing abstractions over new helpers, especially in cryptographic
  paths.
- Keep changes scoped. Do not reformat or refactor unrelated code.
- Use structured parsers and typed APIs where available.
- Preserve serialization, parameter validation, and security checks unless the
  task explicitly changes them.
- When a repeated instruction becomes durable, add it to
  [docs/golden-rules/README.md](docs/golden-rules/README.md) or a focused file
  in that directory instead of expanding this map.

## Verification Map

- Fast root/package check: `GOCACHE=/tmp/lattigo-gocache go test .`
- Full test suite: `GOCACHE=/tmp/lattigo-gocache go test ./...`
- CI-style checks: `make checks` after installing tools with `make get_tools`.
- Toolchain source of truth: `go.mod`.

If a command fails because the Go build or tool cache is read-only in a
sandbox, retry with writable cache locations under `/tmp`, for example
`GOCACHE=/tmp/lattigo-gocache XDG_CACHE_HOME=/tmp/lattigo-xdg-cache`.

## Documentation Updates

Use repository-local Markdown as the system of record. If a decision, harness
constraint, or review lesson matters for future agents, encode it in `docs/`
and link it from the appropriate index.
