# Final Target-Level Reuse Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use subagent-driven-development
> or executing-plans to implement this plan task-by-task. Steps use checkbox
> (`- [ ]`) syntax for tracking.

**Goal:** Implement `final_target_level_reuse_spec.md` so CKKS bootstrapping can
prepare reusable material for declared future target levels and bootstrap to
those levels through a production API.

**Architecture:** Add a closed-world material planning layer over full residual
parameters plus a bootstrapping literal. The plan validates future targets,
selects physical owner levels, generates immutable reusable key material, and
dispatches runtime bootstrap requests only for declared targets.

**Tech Stack:** Go, Lattigo CKKS bootstrapping, existing A0-A6 bootstrap key
reuse harness tests.

---

## File Map

- `circuits/ckks/bootstrapping/target_level_material.go`: public request,
  planner, report, reusable key bundle, and production target-level
  bootstrapper.
- `circuits/ckks/bootstrapping/target_level_material_test.go`: API, planner,
  closed-world, exact owner, and superset DropLevel tests.
- `circuits/ckks/bootstrapping/bootstrap_key_reuse_*_test.go`: oracle replay
  tests that compare final API behavior against A0-A6.
- `docs/bootstrap_key_reuse/final_target_level_reuse_spec.md`: source spec.
- `docs/bootstrap_key_reuse/final_target_level_reuse_implementation_plan.md`:
  this implementation checklist.

## Task 1: Material Plan API and Closed-World Validation

- [x] **Step 1: Write failing tests**

Add tests in `circuits/ckks/bootstrapping/target_level_material_test.go`:

```go
func TestTargetLevelMaterialPlanRejectsEmptyFutureTargets(t *testing.T)
func TestTargetLevelMaterialPlanRejectsInvalidFutureTargetBeforeKeygen(t *testing.T)
func TestTargetLevelMaterialPlanRejectsOwnerThatServesNoFutureTarget(t *testing.T)
func TestTargetLevelMaterialPlanCanonicalizesFutureTargets(t *testing.T)
```

Run:

```bash
go test ./circuits/ckks/bootstrapping -run 'TestTargetLevelMaterialPlan' -count=1 -timeout=30m
```

Expected: FAIL because the public API does not exist.

- [x] **Step 2: Implement minimal planner types**

Create `target_level_material.go` with:

```go
type ReusePolicy int
type TargetLevelMaterialRequest struct { ... }
type TargetLevelMaterialPlan struct { ... }
type TargetLevelPlanReport struct { ... }
func NewTargetLevelMaterialPlan(req TargetLevelMaterialRequest) (*TargetLevelMaterialPlan, error)
```

The planner must validate legal targets using the same Q-prefix construction as
A0/A1 and must not generate keys.

- [x] **Step 3: Verify Task 1**

Run:

```bash
go test ./circuits/ckks/bootstrapping -run 'TestTargetLevelMaterialPlan' -count=1 -timeout=30m
git diff --check
```

Expected: PASS.

## Task 2: Exact-Owner Reusable Key Generation and Runtime API

- [x] **Step 1: Write failing tests**

Add tests:

```go
func TestTargetLevelBootstrapperRejectsUndeclaredTarget(t *testing.T)
func TestTargetLevelBootstrapperExactOwnerBootstrapsDeclaredTarget(t *testing.T)
func TestTargetLevelBootstrapperManyMatchesSingleCalls(t *testing.T)
func TestTargetLevelBootstrapperPreservesExistingEvaluatorBootstrap(t *testing.T)
```

Run:

```bash
go test ./circuits/ckks/bootstrapping -run 'TestTargetLevelBootstrapper' -count=1 -timeout=30m
```

Expected: FAIL until runtime API exists.

- [x] **Step 2: Implement exact-owner runtime**

Add:

```go
type ReusableEvaluationKeys struct { ... }
func (plan *TargetLevelMaterialPlan) GenReusableEvaluationKeys(sk *rlwe.SecretKey) (*ReusableEvaluationKeys, error)
type TargetLevelBootstrapper struct { ... }
func NewTargetLevelBootstrapper(plan *TargetLevelMaterialPlan, keys *ReusableEvaluationKeys) (*TargetLevelBootstrapper, error)
func (b *TargetLevelBootstrapper) BootstrapAtLevel(ct *rlwe.Ciphertext, targetLevel int) (*rlwe.Ciphertext, error)
func (b *TargetLevelBootstrapper) BootstrapManyAtLevel(cts []rlwe.Ciphertext, targetLevel int) ([]rlwe.Ciphertext, error)
```

Generate physical keys only during `GenReusableEvaluationKeys`.

- [x] **Step 3: Verify Task 2**

Run:

```bash
go test ./circuits/ckks/bootstrapping -run 'TestTargetLevelBootstrapper' -count=1 -timeout=30m
git diff --check
```

Expected: PASS.

## Task 3: Superset-Output Owner Selection and DropLevel Dispatch

- [x] **Step 1: Write failing tests**

Add tests:

```go
func TestTargetLevelMaterialPlanSelectsSupersetOwnerWhenAllowed(t *testing.T)
func TestTargetLevelBootstrapperSupersetDropMatchesA0DirectTarget(t *testing.T)
func TestTargetLevelMaterialPlanRejectsSupersetOwnerWhenPolicyDisallowsIt(t *testing.T)
```

Run:

```bash
go test ./circuits/ckks/bootstrapping -run 'TestTargetLevel.*Superset|TestTargetLevel.*Drop' -count=1 -timeout=30m
```

Expected: FAIL until superset dispatch is implemented.

- [x] **Step 2: Implement owner+DropLevel dispatch**

When owner level `o > r`, bootstrap with owner evaluator and DropLevel without
rescale until `ctOut.Level() == r`. Record owner, target, and drop count in
`TargetLevelPlanReport`.

- [x] **Step 3: Verify Task 3**

Run:

```bash
go test ./circuits/ckks/bootstrapping -run 'TestTargetLevel.*Superset|TestTargetLevel.*Drop|TestBootstrapKeyReuseA6' -count=1 -timeout=30m
git diff --check
```

Expected: PASS.

## Task 4: MaterialID, SharedKeyPool, and Report Accounting

- [x] **Step 1: Write failing tests**

Add tests for positive and negative compatibility of relin, rotation,
ring-switch, dense/sparse, schedule, encoded diagonal, RNS prefix, and
compressed-key rejection.

Progress:

- [x] Material ID coverage test for relin, rotation, ring-switch, dense/sparse,
  matrix schedule, and encoded diagonal.
- [x] Positive exact compatibility matrix for relin, rotation, ring-switch,
  dense/sparse, matrix schedule, and encoded diagonal material IDs.
- [x] Compatibility predicate tests for exact relin, rotation `galEl` mismatch,
  and secret-domain mismatch.
- [x] Missing `galEl` rejection on production RNS-prefix key view construction.
- [x] RNS prefix positive/out-of-range tests on production view helpers.
- [x] Compressed-key slice rejection on the final API.
- [x] Target-level RNS prefix incompatibility test for mismatched
  bootstrapping/residual parameter prefixes.
- [x] Planner/report rejected-candidate reason tests for incompatible prefix
  owner candidates.
- [x] Supported-profile RNS prefix incompatibility matrix test covering current
  fast and long cross-target owner candidates.

- [x] **Step 2: Implement material identities and immutable pool**

Add structured material IDs, physical owner counts, logical view counts, shared
counts, and rejected reason codes. Keep RNS slice views disabled by default.

Progress:

- [x] `SharedKeyPool` exposes immutable owner lists and material IDs.
- [x] Physical owner material IDs include relin, rotation, ring-switch,
  dense/sparse, matrix schedule, and encoded diagonal kinds.
- [x] Physical key material IDs separate input-secret, ephemeral-secret, and
  sparse encapsulation domains by material kind and direction.
- [x] `TargetLevelPlanReport` summarizes generated/shared/logical-view counts
  from `SharedKeyPool`.
- [x] Superset consumer rows include reason codes such as
  `superset_output_owner_level=3;drop_levels=2`.
- [x] Planner records selected strategy per target and exposes rejected
  candidate reason counts in both global and per-target reports.
- [x] RNS prefix views remain disabled by default and are registered as
  `rns_prefix` logical views only when `EnableRNSSliceViews` is true and the
  owner/target bootstrapping and residual parameter chains are prefix
  compatible.
- [x] Incompatible target-level RNS prefix candidates are not registered and do
  not dispatch to prefix evaluators; allowed plans fall back to superset-output
  DropLevel.
- [x] RNS prefix runtime dispatch branch is covered by an internal dispatcher
  test; current supported profiles have no cross-target pair whose instantiated
  bootstrapping/residual `Q`/`P` chains are prefix-compatible, so final API
  replay correctly exercises explicit rejection and superset fallback instead
  of an unsafe positive prefix dispatch.

- [x] **Step 3: Verify Task 4**

Run:

```bash
go test ./circuits/ckks/bootstrapping -run 'TestReusableEvaluationKeys|TestSharedKeyPool|TestBootstrapKeyReuseA5' -count=1 -timeout=30m
go test -race ./circuits/ckks/bootstrapping -run 'TestTargetLevelBootstrapper|TestReusableEvaluationKeys|TestSharedKeyPool' -count=1 -timeout=30m
git diff --check
```

Expected: PASS.

Progress:

- [x] `go test ./circuits/ckks/bootstrapping -run 'TestReusableEvaluationKeys|TestSharedKeyPool|TestBootstrapKeyReuseA5' -count=1 -timeout=30m`
- [x] `go test -race ./circuits/ckks/bootstrapping -run 'TestTargetLevelBootstrapper|TestReusableEvaluationKeys|TestSharedKeyPool' -count=1 -timeout=30m`
- [x] `go test ./circuits/ckks/bootstrapping -run 'TestTargetLevel|TestReusableEvaluationKeys|TestSharedKeyPool|TestMaterialIDCompatibility|TestRNSPrefix' -count=1 -timeout=30m`
- [x] `git diff --check`

## Task 5: A0-A6 Oracle Replay Through Final API

- [x] **Step 1: Write failing replay tests**

Add tests that run final API results against A0 direct-target outputs and reuse
the A1-A6 contracts.

Progress:

- [x] Target-level API tests compare exact-owner and superset-output results
  against A0 direct-target validation helpers.
- [x] Final API exact-owner oracle covers every constructible legal target level
  for the current fast supported profiles P0-P4, including target level 0.
- [x] Final API exact-owner oracle covers every constructible legal target level
  for the current long-only profiles P5/P6 under `-args -long`.
- [x] A0-A6 representative oracle replay passes:
  `go test ./circuits/ckks/bootstrapping -run 'TestBootstrapKeyReuse(A0_CSVOnlyOutputContract|A1_CSVOnlyUsedTargetContract|A1_MatchesA0UsedTargets|A2|A3|A4|A5|A6)' -count=1 -timeout=30m`.
- [x] Full `TestBootstrapKeyReuse` structured harness passes with
  `-args -bkr.result-dir=/tmp/lattigo-bkr-results-final-target` in 686.216s.

- [x] **Step 2: Replace harness-only final behavior**

Move reusable behavior into production API and keep A0-A6 harness as oracle and
reporting.

- [x] **Step 3: Verify Task 5**

Run:

```bash
go test ./circuits/ckks/bootstrapping -run 'TestBootstrapKeyReuse|TestTargetLevelBootstrapper|TestReusableEvaluationKeys|TestBootstrapping' -count=1 -timeout=30m
git diff --check
```

Expected: PASS.

Progress:

- [x] The long gate below re-ran `TestBootstrapKeyReuse`,
  `TestTargetLevelBootstrapper`, `TestReusableEvaluationKeys`, and
  `TestBootstrapping` together with `TestCircuit` and `TestAllParameters`.
- [x] `git diff --check` passed after the final edits.

## Task 6: Final Gates

- [x] **Step 1: Minimum gate**

Run:

```bash
go test ./circuits/ckks/bootstrapping -run 'TestBootstrapKeyReuse' -count=1 -timeout=30m
go test ./circuits/ckks/bootstrapping -run 'TestTargetLevelBootstrapper|TestReusableEvaluationKeys|TestBootstrapping' -count=1 -timeout=30m
go test -race ./circuits/ckks/bootstrapping -run 'TestTargetLevelBootstrapper|TestReusableEvaluationKeys|TestSharedKeyPool' -count=1 -timeout=30m
go test ./circuits/ckks/bootstrapping -count=1 -timeout=30m
git diff --check
```

Expected: PASS.

Progress:

- [x] Final audit run, 2026-05-30 UTC: `TestBootstrapKeyReuse` passed with
  explicit `-bkr.result-dir=/tmp/lattigo-bkr-results-final-goal-min-bkr` in
  688.761s.
- [x] Final audit run, 2026-05-30 UTC:
  `TestTargetLevelBootstrapper|TestReusableEvaluationKeys|TestBootstrapping`
  passed in 377.522s.
- [x] Final audit run, 2026-05-30 UTC: race target-level/shared-pool run passed
  in 46.749s.
- [x] Race target-level/shared-pool run keeps the all-legal target replay in
  non-race gates and skips those two exhaustive replay tests under the race
  build tag so the spec's 30m race timeout remains meaningful.
- [x] Final audit run, 2026-05-30 UTC: full package
  `go test ./circuits/ckks/bootstrapping -count=1 -timeout=30m` passed with
  explicit `-bkr.result-dir=/tmp/lattigo-bkr-results-final-goal-min-full` in
  1068.042s.
- [x] Final audit run, 2026-05-30 UTC: `git diff --check` passed.

- [x] **Step 2: Long and performance gates**

Run:

```bash
go test ./circuits/ckks/bootstrapping -run 'TestBootstrapKeyReuse|TestTargetLevelBootstrapper|TestReusableEvaluationKeys|TestBootstrapping|TestCircuit|TestAllParameters' -count=1 -timeout=60m
GOMAXPROCS=1 go test ./circuits/ckks/bootstrapping -bench 'Bootstrap|BootstrapKeyReuse|TargetLevel' -run '^$' -count=3 -timeout=60m
```

Expected: PASS with benchmark output recorded in the review summary.

Progress:

- [x] Final audit run, 2026-05-30 UTC: long gate passed with
  `-args -bkr.result-dir=/tmp/lattigo-bkr-results-final-goal-long` in
  1062.687s.
- [x] Final audit run, 2026-05-30 UTC: long-only P5/P6 final API all-legal
  target oracle passed with `-args -long` in 1797.697s.
- [x] Final audit run, 2026-05-30 UTC: performance gate passed in 479.679s with
  representative benchmark lines:
  `BenchmarkBootstrapKeyReuseA0_P2MultiFastClustered` around 310-314ms/op,
  `BenchmarkBootstrapKeyReuseA0_P4N15DenseSparse` around 12.3-13.6s/op, and
  `BenchmarkConcurrentBootstrap/...` around 26.4-27.9s/op.

## Remaining Completion Audit Items

- [x] Literal complete legal target set coverage now includes the fast supported
  profiles P0-P4 and the long-only P5/P6 profiles through the final
  target-level API.
