# Golden Rules

This directory stores durable harness constraints for future agent runs. Use it
for rules that keep the repository legible, correct, and mechanically
verifiable over time.

## Current Rules

1. Keep `AGENTS.md` short. Add details to `docs/` and link them from the map.
2. Preserve the repository dependency direction:
   `ring -> core -> schemes -> circuits/multiparty/examples`.
3. Treat [toolchain.md](toolchain.md) as the source of truth for Go, build, and
   check tool dependencies.
4. Run targeted verification before claiming a change is complete.
5. Prefer existing Lattigo abstractions over ad hoc helpers, especially in
   cryptographic code paths.

## Adding A Rule

Create a focused Markdown file in this directory when a rule needs more detail.
Use this template:

```markdown
# Rule Name

## Rule

State the invariant in one or two sentences.

## Why

Explain the failure mode this prevents.

## How To Verify

List commands, tests, static checks, or review cues that confirm compliance.

## Applies To

List packages, workflows, or file patterns covered by the rule.
```

Then link the file from the "Current Rules" section above.

## Rule Quality Bar

- A rule should constrain future work, not narrate past work.
- A rule should be easier to follow than to rediscover.
- A rule should name the verification path when one exists.
- A rule should stay small enough that agents can load it only when relevant.
