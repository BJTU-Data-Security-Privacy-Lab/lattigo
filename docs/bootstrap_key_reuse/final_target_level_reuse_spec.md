# CKKS Arbitrary Target-Level Bootstrapping and Reusable Key Material Spec

This document defines the final implementation target for CKKS bootstrapping in
this repository: callers can request any legal bootstrap target level, and the
implementation reuses overlapping bootstrap key material across target levels
when compatibility is proven.

This document is a requirements source, not a status claim. A0-A6 remain the
evidence chain and regression oracle. The final goal is complete only when the
API, planner, reusable key pool, compatibility checks, and acceptance tests in
this spec are implemented and passing.

## 1. Goal

The final implementation must satisfy all of the following:

1. A caller can request any legal bootstrap output target level for a supported
   CKKS bootstrapping parameter set.
2. A successful call returns a ciphertext whose `Level()` is exactly the
   requested target level.
3. The returned scale matches the semantics of direct bootstrapping to the same
   target level.
4. The output precision is equivalent to the A0 direct-BK(target) oracle under
   the repository's existing bootstrap precision thresholds.
5. Overlapping bootstrap key material is generated once as physical material
   and reused through logical views whenever compatibility is proven.
6. Incompatible material is never reused. The planner or runtime must return an
   explicit error instead of silently falling back to unsafe reuse.
7. Existing `Evaluator.Bootstrap` and `Evaluator.BootstrapMany` behavior stays
   compatible unless the caller opts into the new target-level API.

## 2. Non-Goals

1. Do not weaken CKKS security, secret-key domain separation, modulus-chain
   semantics, scale semantics, or precision requirements.
2. Do not decide material compatibility by target-level integers, file names,
   string labels, or plan row names alone.
3. Do not allow an unconfigured target level to trigger key generation at
   runtime.
4. Do not enable RNS-slice key views by default until each material kind has
   positive and negative compatibility tests.
5. Do not treat A0-A6 harness helpers as the public product API.

## 3. Terms

- **Target level**: the requested bootstrap output level, written as `r`.
- **Direct BK(r)**: a bootstrapping evaluator and key set generated directly for
  target level `r`.
- **Owner level**: the target level that owns physical reusable material.
- **Consumer target**: a target level served by an owner material or owner view.
- **Physical material**: generated key, encoded diagonal, linear-transform
  schedule, or evaluator state that occupies memory.
- **Logical view**: a read-only view of physical material with exact, prefix, or
  superset-output semantics.
- **Material identity**: the structured identifier used to prove that a piece
  of material is equivalent or view-compatible.
- **Closed-world material plan**: a prepared material plan that can serve only
  the caller-declared `FutureTargetLevels`.
- **Superset output**: bootstrapping at a higher owner level and applying
  `DropLevel` to reach the requested lower target level.

### 3.1 Legal Target Level

A legal target level is not just an integer. For a full CKKS profile and
bootstrapping literal, target level `r` is legal only when all checks below pass:

1. `0 <= r <= fullParams.MaxLevel()`.
2. The target residual parameter literal is built from the full profile by
   taking `Q[:r+1]`, retaining the full `P` chain, clearing `LogQ` and `LogP`,
   and setting the fixed target scale.
3. `ckks.NewParametersFromLiteral(targetResidualLiteral)` succeeds.
4. `NewParametersFromLiteral(targetResidualParams, bootstrappingLiteral)`
   succeeds.
5. The resulting bootstrapping parameters have
   `ResidualParameters.MaxLevel() == r`.
6. An evaluator built for this target reports `OutputLevel() == r`.

Planner target validation must use this construction or a production equivalent
with the same semantics. A target that fails any check is invalid, even if the
integer is inside the full-profile level range.

## 4. Current Baseline

The repository currently has an A0-A6 harness evidence chain:

1. A0 defines the direct per-target baseline.
2. A1 restricts generation to configured target levels and verifies that
   unconfigured targets fail.
3. A2-A4 verify CSV accounting, owner/view relationships, function-level
   observations, and latency or precision reports.
4. A5 verifies exact, prefix, and superset RNS-slice sharing contracts in the
   harness.
5. A6 verifies that a higher target-level owner can bootstrap and then
   `DropLevel` to a lower target while matching A0 direct-target behavior.

The production API is still centered on one `Parameters` value and one
`EvaluationKeys` bundle:

```go
func NewEvaluator(btpParams Parameters, evk *EvaluationKeys) (*Evaluator, error)
func (eval *Evaluator) Bootstrap(ct *rlwe.Ciphertext) (*rlwe.Ciphertext, error)
func (eval *Evaluator) BootstrapMany(cts []rlwe.Ciphertext) ([]rlwe.Ciphertext, error)
func (b Parameters) GenEvaluationKeys(skN1 *rlwe.SecretKey) (*EvaluationKeys, error)
```

Those APIs do not accept a requested target level. Therefore the final target is
not productized yet. This spec defines the product API, planner, reusable key
pool, compatibility rules, and completion gates that close that gap.

## 5. Public API

Existing APIs remain stable. New behavior is exposed through a separate
material-preparation API and a target-level runtime API.

The material-preparation API is closed-world: the caller declares the complete
set of target levels it expects to bootstrap to in the future. The planner then
decides which physical owner levels must be generated and which declared targets
can be served by reusable views.

```go
type ReusePolicy int

const (
	ReuseExactOnly ReusePolicy = iota
	ReuseExactAndPrefix
	ReuseExactPrefixAndSupersetDrop
)

type TargetLevelMaterialRequest struct {
	FullResidualParameters ckks.Parameters
	BootstrappingLiteral   ParametersLiteral
	FutureTargetLevels     []int
	OwnerTargetLevels      []int
	ReusePolicy            ReusePolicy
	EnableRNSSliceViews    bool
	AllowSupersetDrop      bool
}

type TargetLevelMaterialPlan struct {
	// Validated future targets, owner selections, per-target Parameters,
	// material identities, and logical views.
}

type ReusableEvaluationKeys struct {
	// Immutable physical material pool plus target-level logical views.
}

type TargetLevelBootstrapper struct {
	// Dispatcher over exact evaluators and verified reusable owner views.
}

func NewTargetLevelMaterialPlan(
	req TargetLevelMaterialRequest,
) (*TargetLevelMaterialPlan, error)

func (plan *TargetLevelMaterialPlan) GenReusableEvaluationKeys(
	sk *rlwe.SecretKey,
) (*ReusableEvaluationKeys, error)

func NewTargetLevelBootstrapper(
	plan *TargetLevelMaterialPlan,
	keys *ReusableEvaluationKeys,
) (*TargetLevelBootstrapper, error)

func (b *TargetLevelBootstrapper) BootstrapAtLevel(
	ct *rlwe.Ciphertext,
	targetLevel int,
) (*rlwe.Ciphertext, error)

func (b *TargetLevelBootstrapper) BootstrapManyAtLevel(
	cts []rlwe.Ciphertext,
	targetLevel int,
) ([]rlwe.Ciphertext, error)

func (b *TargetLevelBootstrapper) PlanReport() TargetLevelPlanReport
```

API invariants:

1. `FutureTargetLevels` is required, sorted, deduplicated, and validated against
   the legal-target construction in section 3.1.
2. `FutureTargetLevels` is the complete runtime serve set. `BootstrapAtLevel`
   must reject any target that was not declared during material preparation.
3. `OwnerTargetLevels` is optional. If empty, the planner chooses physical
   owners from legal target levels according to `ReusePolicy` and
   `AllowSupersetDrop`. If non-empty, every owner must be legal and must cover
   at least one future target.
4. Plan construction fails before key generation if the declared future targets
   cannot all be served by exact owners or compatible reusable owner views.
5. `AllowSupersetDrop=false` forbids serving a lower target from a higher owner
   through `DropLevel`.
6. `EnableRNSSliceViews=false` allows exact material reuse and superset-output
   `DropLevel`, but forbids slicing key limbs or encoded limbs.
7. `BootstrapAtLevel(ct, r)` succeeds only if the returned ciphertext has
   `Level() == r`.
8. `BootstrapManyAtLevel(cts, r)` has the same semantics as calling
   `BootstrapAtLevel` for each input.
9. `PlanReport` explains each target's owner, reuse strategy, physical material
   counts, logical view counts, shared counts, and rejected candidate reasons.

## 6. Internal Architecture

### 6.1 TargetLevelPlanner

The planner takes `TargetLevelMaterialRequest` and secret-key domain metadata.
It outputs an immutable `TargetLevelMaterialPlan`.

Planner responsibilities:

1. Build the target-level capability table.
2. Select exact owners or superset owners for each target.
3. Generate a `MaterialID` for every reusable material candidate.
4. Decide physical owners and logical views.
5. Record a reason code for every rejected reuse candidate.
6. Produce the structured `PlanReport`.

The planner must not generate keys. Its output must be serializable for tests
and stable enough for golden report comparisons.

### 6.2 SharedKeyPool

`SharedKeyPool` owns all physical reusable material:

1. Ring-switch evaluation keys.
2. Dense and sparse encapsulation keys.
3. Relinearization keys.
4. Rotation keys.
5. Bootstrapping linear-transform schedules.
6. Encoded diagonals.
7. Per-owner evaluator state.

Physical material is immutable after plan construction. Logical views are
read-only and must not mutate owner material.

### 6.3 MaterialID

Every reusable material must have a structured identifier. The identifier must
include at least:

1. Lattigo parameter hash: ring degree, Q/P chains, default scale, `LogSlots`,
   and ring type.
2. Secret-key domain hash: input secret, ephemeral secret, dense secret, and
   sparse secret separated by material kind.
3. Material kind: relin, rotation, ring-switch, dense-to-sparse,
   sparse-to-dense, matrix schedule, or encoded diagonal.
4. Level descriptor: LevelQ, LevelP, RNS prefix length, and target output
   level.
5. Decomposition descriptor: BaseRNS, BaseTwo, gadget decomposition, and
   compressed or uncompressed format.
6. Automorphism descriptor: `galEl`, conjugation flag, and NthRoot.
7. Matrix descriptor: transform name, schedule index, diagonal index, matrix
   hash, and scale metadata.
8. View descriptor: exact, prefix, or superset-output-drop.

Compatibility cannot be inferred from target-level integers alone.

### 6.4 Runtime Dispatcher

The runtime dispatches according to the planner result:

1. Exact owner: call the owner evaluator directly.
2. Prefix view: build a read-only view and call the corresponding evaluator.
3. Superset output: call the owner evaluator, then `DropLevel` to the requested
   target level.

The runtime must not generate keys inside `BootstrapAtLevel` or
`BootstrapManyAtLevel`. All material must come from `ReusableEvaluationKeys`.

## 7. Compatibility Rules

### 7.1 Global Rules

Material reuse is allowed only when all relevant checks pass:

1. Same `Parameters` profile.
2. Same secret-key domain, or an explicitly modeled deterministic relation for
   that material kind.
3. Same target scale semantics.
4. Owner material covers every level, automorphism, decomposition, and schedule
   required by the consumer.
5. The view evaluator cannot read limbs, diagonals, rotation keys, or schedules
   outside the view range.
6. Output equivalence is validated against the A0 direct-BK(target) oracle.
7. Target-level prefix reuse must compare the instantiated owner and target
   residual and bootstrapping `Q`/`P` chains by value. `ownerLevel > targetLevel`
   is not proof of modulus-prefix compatibility because target-specific
   bootstrapping parameters may resample circuit primes.

### 7.2 Relinearization Key

Relinearization keys can be reused exactly. Prefix reuse is allowed only when the
implementation proves that the evaluator reads only the prefix limbs.

Reject reuse when:

1. Secret-key domain hashes differ.
2. Q/P prefix descriptors differ.
3. Decomposition descriptors differ.
4. The key is compressed and slice-safe access has not been implemented.

### 7.3 Rotation Keys

Rotation key reuse requires:

1. Identical `galEl`.
2. Identical NthRoot and ring type.
3. Identical secret-key domain.
4. Compatible decomposition descriptor.
5. Exact Q/P descriptor, or a prefix view proven for rotation keys.

A missing `galEl` returns a planning or runtime error. It must not fall back to a
different rotation key.

### 7.4 Ring-Switch Keys

Ring-switch key reuse requires:

1. Identical source secret domain.
2. Identical destination secret domain.
3. Identical ring degree, NthRoot, and ring type.
4. Exact modulus descriptor, or a prefix view proven for ring-switch keys.

If a profile uses a different ephemeral secret, the secret-domain hash must
reject reuse.

### 7.5 Dense and Sparse Encapsulation Keys

Dense/sparse key reuse requires:

1. Identical dense secret domain.
2. Identical sparse secret domain.
3. Identical encapsulation direction.
4. Compatible level, P-chain, and decomposition descriptors.

Conjugate-invariant profiles must have dedicated coverage.

### 7.6 Matrix Schedules

Matrix schedules are metadata. They can be reused when the schedule ID is
identical. The schedule ID must include:

1. Transform kind.
2. Matrix dimensions.
3. Baby-step and giant-step parameters.
4. Diagonal set.
5. Rotation set.
6. Scale metadata.

Schedule metadata reuse does not imply encoded diagonal reuse.

### 7.7 Encoded Diagonals

Encoded diagonal exact reuse requires:

1. Identical matrix descriptor.
2. Identical diagonal index.
3. Identical encoded polynomial hash.
4. Identical modulus descriptor.

Encoded diagonal prefix reuse requires:

1. Owner prefix limbs match the consumer direct encoding.
2. The evaluator reads only the prefix limbs.
3. Tests cover binary or semantic equivalence against direct encoding.

### 7.8 Superset Output DropLevel

Superset-output reuse requires:

1. Owner output level `o` is greater than target level `r`.
2. Owner bootstrap succeeds before `DropLevel`.
3. `DropLevel` does not rescale.
4. The dropped ciphertext has `Level() == r`.
5. The dropped ciphertext scale matches direct BK(r) output scale.
6. Precision matches direct BK(r) under A0-A6 thresholds.
7. The plan records `owner_level=o`, `target_level=r`, and `drop_levels=o-r`.

## 8. Error Semantics

All failures are explicit errors:

1. Invalid target level: `ErrInvalidTargetLevel`.
2. Target unavailable in the prepared material plan:
   `ErrTargetLevelUnavailable`.
3. Missing reusable material: `ErrMissingReusableMaterial`.
4. Incompatible reusable material: `ErrIncompatibleReusableMaterial`.
5. View range read violation: `ErrReusableViewOutOfRange`.

Error messages must include target level, candidate owner level, material kind,
and reason code when those values exist.

## 9. Plan Report and Accounting

`PlanReport` must include:

1. `plan_id`.
2. Future target level list.
3. Owner level list.
4. Dispatch strategy for every target.
5. Generated physical count per material kind.
6. Logical view count per material kind.
7. Shared count per material kind.
8. Rejected candidate reason codes.
9. `EnableRNSSliceViews` state.
10. `AllowSupersetDrop` state.

Accounting rules:

1. Physical material is counted as generated only for its owner.
2. Logical views are not counted as generated.
3. Each shared count traces back to a physical owner.
4. Counting semantics must remain aligned with the A0-A6 CSV contracts.

## 10. Acceptance Gates

The final goal is complete only when every gate in this section passes.

### 10.1 API Unit Tests

Required coverage:

1. `NewTargetLevelMaterialPlan` rejects an empty `FutureTargetLevels` set.
2. `NewTargetLevelMaterialPlan` rejects invalid future targets before key
   generation.
3. `NewTargetLevelMaterialPlan` rejects owner targets that cannot serve any
   declared future target.
4. `BootstrapAtLevel` succeeds for every declared future target.
5. `BootstrapAtLevel` rejects undeclared target levels, even if the target is
   legal for the full profile.
6. `BootstrapAtLevel` returns an error for invalid target levels.
7. `BootstrapManyAtLevel` matches repeated `BootstrapAtLevel` calls.
8. A declared future target fails plan construction when no compatible owner
   exists.
9. A declared future target succeeds through superset-output `DropLevel` only when
   a compatible owner exists and the policy allows it.
10. Existing `Evaluator.Bootstrap` behavior remains unchanged.

### 10.2 Planner Tests

Required coverage:

1. Future target canonicalization.
2. Explicit owner target canonicalization.
3. Automatic exact owner selection.
4. Automatic superset owner selection.
5. No-owner rejection.
6. Positive compatibility for every material kind.
7. Negative compatibility for every material kind.
8. Generated/shared/view counts without double-counting.
9. Golden `PlanReport` output.

### 10.3 Material Reuse Tests

Required coverage:

1. Relinearization key exact reuse.
2. Rotation key exact reuse.
3. Missing `galEl` rejection.
4. Ring-switch secret-domain mismatch rejection.
5. Dense/sparse domain mismatch rejection.
6. Matrix schedule exact reuse.
7. Encoded diagonal exact reuse.
8. RNS prefix view positive case.
9. RNS prefix view out-of-range negative case.
10. Compressed-key slice rejection until slice-safe access exists.
11. Concurrent bootstrap over shared read-only material.

### 10.4 A0-A6 Oracle Replay

The final API must replay the A0-A6 semantics:

1. A0 direct target baseline.
2. A1 used-target-only behavior.
3. A2 material accounting.
4. A3 owner/view plan.
5. A4 latency and precision report.
6. A5 exact/prefix/superset RNS-slice contract.
7. A6 superset-output DropLevel contract.

Fast tests may use representative levels. Long tests must cover the complete
legal target set for every supported profile.

### 10.5 Command Gates

Minimum gate:

```bash
go test ./circuits/ckks/bootstrapping -run 'TestBootstrapKeyReuse' -count=1 -timeout=30m
go test ./circuits/ckks/bootstrapping -run 'TestTargetLevelBootstrapper|TestReusableEvaluationKeys|TestBootstrapping' -count=1 -timeout=30m
go test -race ./circuits/ckks/bootstrapping -run 'TestTargetLevelBootstrapper|TestReusableEvaluationKeys|TestSharedKeyPool' -count=1 -timeout=30m
go test ./circuits/ckks/bootstrapping -count=1 -timeout=30m
git diff --check
```

Long gate:

```bash
go test ./circuits/ckks/bootstrapping -run 'TestBootstrapKeyReuse|TestTargetLevelBootstrapper|TestReusableEvaluationKeys|TestBootstrapping|TestCircuit|TestAllParameters' -count=1 -timeout=60m
```

Performance gate:

```bash
GOMAXPROCS=1 go test ./circuits/ckks/bootstrapping -bench 'Bootstrap|BootstrapKeyReuse|TargetLevel' -run '^$' -count=3 -timeout=60m
```

Correctness cannot be waived by benchmark results.

## 11. Drift Prevention Rules

1. Each implementation PR must cite the relevant section numbers from this
   spec.
2. Each new reuse path must add a negative compatibility test before it is
   considered complete.
3. Each planner output change must update a golden plan report.
4. Harness-only helpers cannot be promoted to public API unless they satisfy the
   API invariants in this spec.
5. Runtime key generation inside `BootstrapAtLevel` and
   `BootstrapManyAtLevel` is forbidden.
6. Metrics written only as free-form logs are insufficient. A structured report
   is required.
7. Benchmark data cannot replace the A0 direct-target correctness oracle.
8. `EnableRNSSliceViews` remains default false until every material kind has
   proof and tests.

## 12. Loophole Audit Loop

This section records the self-audit requested for the spec. Every identified
loophole has been converted into a hard requirement or acceptance gate.

| ID | Loophole | Required fix | Spec location |
| --- | --- | --- | --- |
| L1 | "Any target level" could be reduced to a few representative tests | Fast tests use representatives, long tests cover the complete legal target set per profile | 10.4 |
| L2 | Material from a different secret-key domain could be reused | `MaterialID` includes per-kind secret-domain hashes plus negative tests | 6.3, 7, 10.3 |
| L3 | RNS prefix views could read omitted limbs | RNS slice views default false; each material kind needs proof and out-of-range tests | 5, 7, 10.3 |
| L4 | Superset `DropLevel` could hide scale or precision drift | DropLevel cannot rescale and must compare level, scale, and precision to A0 direct BK(target) | 7.8, 10.4 |
| L5 | New behavior could break existing `Evaluator.Bootstrap` callers | Use a separate target-level API and require old-API regression tests | 5, 10.1 |
| L6 | Accounting could double-count shared physical material | Separate physical material from logical views and count generated material only for owners | 6.2, 9 |
| L7 | Compressed keys or serialization could bypass slice checks | Reject compressed-key slicing until slice-safe access and tests exist | 7.2, 10.3 |
| L8 | Shared material could be mutated during concurrent bootstrap | Make `SharedKeyPool` immutable after planning and test concurrent use | 6.2, 10.3 |
| L9 | Conjugate-invariant or dense/sparse profiles could be missed | Require dedicated conjugate-invariant and dense/sparse mismatch coverage | 7.5, 10.3, 10.4 |
| L10 | Owner coverage could compare only integer levels | Legal-target validation and compatibility must compare Q/P descriptors, decomposition, schedules, and material kind | 3.1, 6.3, 7 |
| L11 | Benchmark noise could be mistaken for correctness | Performance gates use `GOMAXPROCS=1` and `-count=3`; correctness remains oracle-based | 10.5, 11 |
| L12 | Implementation could drift from this spec | PRs cite spec sections; reuse paths require negative tests and golden reports | 11 |
| L13 | A single target-specific `Parameters` value could be mistaken for a full-profile source of truth | Material preparation now accepts full residual parameters plus bootstrapping literal and constructs every declared target before keygen | 5, 6.1, 10.1 |
| L14 | Declared future targets could be confused with physical owner targets | `FutureTargetLevels` and `OwnerTargetLevels` are separate, with closed-world runtime dispatch | 5, 10.1, 10.2 |

After this audit loop, there are no known open loopholes in the spec. The
confidence claim is scoped to the spec: if implementation satisfies the API
invariants, compatibility rules, and acceptance gates above, it is sufficient to
achieve the final target without drift. This is not a claim that the current
code already implements the final target.

## 13. Implementation Work Packages

Implementation should proceed in work packages that preserve a working tree after
each package:

1. Productize `TargetLevelMaterialRequest`, legal-target validation, and planner
   data structures without changing runtime behavior.
2. Add `ReusableEvaluationKeys`, `SharedKeyPool`, `MaterialID`, and
   `PlanReport` with exact-only reuse.
3. Add `TargetLevelBootstrapper` and the target-level runtime API using exact
   owners only.
4. Add superset-output owner selection and `DropLevel` dispatch.
5. Add prefix view support per material kind, keeping each kind disabled until
   its positive and negative tests pass.
6. Replay A0-A6 oracles through the final API.
7. Run the minimum, long, and performance gates before declaring completion.

Each package must leave existing bootstrapping tests passing. Any package that
adds a reuse path must add a negative compatibility test in the same change.

## 14. Factual Completion Definition

The final target can be called 100% complete only when all items below are true:

1. The material-preparation API and target-level public API are implemented and
   covered by API tests.
2. `ReusableEvaluationKeys` and `SharedKeyPool` are implemented.
3. The planner can build plans for declared future target levels from full
   residual parameters plus bootstrapping literals.
4. Every material kind has positive and negative compatibility tests.
5. `BootstrapAtLevel` matches the A0 direct-BK(target) oracle for every declared
   future target level.
6. Reuse paths never generate keys at runtime.
7. `PlanReport` explains every generated, shared, and view count.
8. A0-A6 oracle semantics are replayed through the final API.
9. Minimum, long, and performance gates pass.
10. `git diff --check` passes and no test artifacts are left in the repository.

If any item is incomplete, the correct status is: stage evidence exists, but the
final target is not 100% complete.
