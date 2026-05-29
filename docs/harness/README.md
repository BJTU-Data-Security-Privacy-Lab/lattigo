# Harness Workflow

This repository follows a map-first harness style inspired by OpenAI's harness
engineering write-up: keep the agent entry point small, make repository
knowledge discoverable, and encode durable constraints where future agents can
find them.

Source: https://openai.com/index/harness-engineering/

## Principles

- `AGENTS.md` is a table of contents, not a manual.
- `docs/` is the system of record for durable repository knowledge.
- `docs/golden-rules/` stores constraints that should shape future work.
- Mechanical checks are preferred when a rule can be verified.
- Human review lessons should become docs or checks when they are likely to
  recur.

## Agent Operating Loop

1. Read [AGENTS.md](../../AGENTS.md).
2. Open [docs/README.md](../README.md) and the package README nearest to the
   files involved.
3. Check [golden rules](../golden-rules/README.md) for durable constraints that
   apply to the task.
4. Make the smallest scoped change that satisfies the request.
5. Run the narrowest meaningful verification first, then broaden when risk or
   blast radius justifies it.
6. If a new durable constraint is discovered, add it under `docs/golden-rules/`
   and link it from the golden-rules index.

## Toolchain Expectations

- `go.mod` is the source of truth for Go language and toolchain selection.
- [Toolchain golden rule](../golden-rules/toolchain.md) is the source of truth
  for build and check tool dependencies.
- Use a writable build cache in restricted sandboxes:

```bash
GOCACHE=/tmp/lattigo-gocache XDG_CACHE_HOME=/tmp/lattigo-xdg-cache go test ./...
```

- `make get_tools` installs pinned check tools into `$(go env GOPATH)/bin`.
- `make checks` is the CI-style local quality gate. The Makefile prepends the
  Go bin directory to `PATH` so freshly installed tools are discoverable.

## When To Add Harness Constraints

Add a golden rule when the lesson is:

- durable across multiple tasks,
- specific enough to guide an agent,
- enforceable by review or automation, and
- tied to repository behavior rather than personal preference.

Do not add transient task notes, speculative ideas, or duplicate package README
content as golden rules.
