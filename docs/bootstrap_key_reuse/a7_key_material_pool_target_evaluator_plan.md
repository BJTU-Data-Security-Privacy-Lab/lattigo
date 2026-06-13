# A7 Key Material Pool Target Evaluator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement A7 as a key-material-only pool strategy: prepare reusable
bootstrap key sets for declared target levels, deduplicate compatible key
material, and bootstrap each target with a dedicated evaluator whose output
level exactly matches the request.

**Architecture:** A7 separates key material ownership from runtime evaluator
ownership. The pool owns only relinearization, Galois, ring/domain switch, and
dense/sparse keys. Each declared target has a key manifest. Runtime evaluator
construction assembles `EvaluationKeys` from the manifest and then calls the
existing `NewEvaluator(targetParams, keys)`, allowing DFT matrices to remain
target-local evaluator preparation. Before any physical key generation, A7 runs
a descriptor-only requirement planner that proves which owner hints contribute
physical key material and that every target manifest uses one coherent bootstrap
secret domain.

**Tech Stack:** Go, Lattigo CKKS bootstrapping,
`bootstrapping.EvaluationKeys`, `rlwe.MemEvaluationKeySet`, existing A0-A6
target-level harness, final target-level material API.

---

## Problem Statement

The previous A7 draft expanded the reuse boundary to executable linear
transformations, matrix schedules, encoded diagonals, and DFT matrix bundle
assembly. Material-size measurements for the normal `N=2^16`, 12-Q-limb
profile show that persistent key sets dominate the cost. The final method
therefore reduces A7 to key-set reuse only.

The intended method is:

1. The caller declares future target levels.
2. Preparation generates all key material needed by those targets.
3. Compatible key material is generated once and shared by manifests.
4. Target-specific key material remains private to the target manifest.
5. `BootstrapAtLevel(ct, r)` uses a target evaluator built for `r`.
6. DFT matrices are generated normally by `NewEvaluator` for that target.

## Non-Goals

- Do not pool or reuse `lintrans.LinearTransformation` objects.
- Do not pool or reuse encoded diagonals.
- Do not pool matrix schedule metadata as A7 runtime material.
- Do not introduce `EvaluatorMaterialBundle`.
- Do not introduce `NewEvaluatorFromMaterialBundle`.
- Do not add no-regeneration constraints for DFT matrices.
- Do not remove A6. A6 remains the explicit superset/drop fallback lane.

## File Map

- Create `circuits/ckks/bootstrapping/target_level_key_material_pool.go`
  - Defines `KeyMaterialKey`, `KeyMaterialPool`,
    `TargetKeyMaterialManifest`, and key-material accounting helpers.
- Create `circuits/ckks/bootstrapping/target_level_key_material_pool_test.go`
  - Tests key identity, incompatibility, target manifests, and size accounting.
- Create `circuits/ckks/bootstrapping/target_level_key_evaluator_assembly.go`
  - Builds target-level `EvaluationKeys` and target `Evaluator` instances from
    key manifests.
- Create `circuits/ckks/bootstrapping/target_level_key_evaluator_assembly_test.go`
  - Tests target-dedicated evaluator construction, missing key rejection, and
    output-level invariants.
- Modify `circuits/ckks/bootstrapping/target_level_material.go`
  - Adds the A7 key-material policy, target owner selection, pool generation,
    and report fields.
- Modify `circuits/ckks/bootstrapping/target_level_material_test.go`
  - Adds default-policy regression tests.
- Modify benchmark and docs files under `docs/bootstrap_key_reuse/`
  - Rename A7 benchmark/report wording from material-pool to key-material-pool.

## Core Types

Implement these types before changing planner behavior:

```go
type BootstrapSecretDomain struct {
	OwnerLevel     int
	ParametersHash string
	SecretDomain   string
}

type KeyMaterialKind string

const (
	KeyMaterialKindRelinearization KeyMaterialKind = "relinearization_key"
	KeyMaterialKindRotation        KeyMaterialKind = "rotation_key"
	KeyMaterialKindRingSwitch      KeyMaterialKind = "ring_switch_key"
	KeyMaterialKindDenseSparse     KeyMaterialKind = "dense_sparse_key"
)

type KeyMaterialKey struct {
	Kind                  KeyMaterialKind
	LevelQ                int
	LevelP                int
	ParametersHash        string
	BootstrapSecretDomain string
	SecretDomain          string
	GaloisElement         uint64
	Direction             string
	DescriptorHash        string
}

type KeyMaterialView string

const (
	KeyMaterialViewPhysical  KeyMaterialView = "physical"
	KeyMaterialViewShared    KeyMaterialView = "shared"
	KeyMaterialViewPrivate   KeyMaterialView = "private"
	KeyMaterialViewRNSPrefix KeyMaterialView = "rns_prefix"
)

type KeyMaterialObject struct {
	Key       KeyMaterialKey
	View      KeyMaterialView
	Owner     int
	Targets   []int
	Value     any
	BinaryLen int
}

type TargetKeyMaterialManifest struct {
	TargetLevel           int
	KeyMaterialOwnerLevel int
	BootstrapSecretDomain string
	Shared                []KeyMaterialKey
	Private               []KeyMaterialKey
}

type KeyMaterialPool struct {
	objects   map[KeyMaterialKey]KeyMaterialObject
	manifests map[int]TargetKeyMaterialManifest
}

type TargetKeyMaterialRequirement struct {
	TargetLevel           int
	CandidateOwnerLevel   int
	BootstrapSecretDomain string
	Key                   KeyMaterialKey
	Shareable             bool
}
```

Allowed `Value` types:

- `*rlwe.RelinearizationKey`
- `*rlwe.GaloisKey`
- `*rlwe.EvaluationKey` for ring/domain switch keys
- `*rlwe.EvaluationKey` for dense/sparse keys

Do not add key material kinds for linear transformations, matrix schedules,
encoded diagonals, or DFT matrices.

## Task 1: Reclassify A7 as Key-Material-Only Policy

**Files:**

- Modify `circuits/ckks/bootstrapping/target_level_material.go`
- Modify `circuits/ckks/bootstrapping/target_level_material_test.go`
- Modify `docs/bootstrap_key_reuse/final_target_level_reuse_spec.md`

- [ ] **Step 1: Write failing policy tests**

Add tests that set `ReusePolicy = ReuseKeyMaterialPoolTargetEvaluator` and
assert:

- `FutureTargetLevels` stays equal to declared targets.
- Runtime owner for every target equals that target.
- `OwnerTargetLevels` are reported only as key-material owner hints.
- every non-empty `OwnerTargetLevels` hint must contribute at least one
  physical key material object to a declared target manifest.
- unused owner hints are rejected with an explicit
  `unused_key_material_owner_hint` reason before key generation.
- unused owner hints are detected by a descriptor-only requirement plan, not by
  generating keys and inspecting the result after the fact.
- `AllowSupersetDrop` does not affect A7 policy.

- [ ] **Step 2: Run RED**

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run 'TestKeyMaterialPoolPolicyDoesNotCollapseToSingleHighestOwner|TestA7OwnerHintsDoNotRemoveRuntimeTargets|TestA7RejectsUnusedKeyMaterialOwnerHint' -count=1 -timeout=30m
```

Expected: fails because the A7 key-material policy does not exist.

- [ ] **Step 3: Implement policy**

Add:

```go
const (
	ReuseExactOnly ReusePolicy = iota
	ReuseExactAndPrefix
	ReuseExactPrefixAndSupersetDrop
	ReuseKeyMaterialPoolTargetEvaluator
)
```

For `ReuseKeyMaterialPoolTargetEvaluator`:

- canonical runtime owners are exactly `FutureTargetLevels`;
- non-empty `OwnerTargetLevels` are key-material owner hints only;
- owner hints must be legal and must contribute at least one physical key
  material object to at least one target manifest;
- owner hints that contribute nothing must fail planning with an explicit
  `unused_key_material_owner_hint` rejection reason;
- contribution is computed from descriptor-only
  `TargetKeyMaterialRequirement` rows before physical key generation;
- `AllowSupersetDrop` is ignored;
- report strategy string is `key_material_pool_target_evaluator`.

- [ ] **Step 4: Run GREEN**

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run 'TestKeyMaterialPoolPolicyDoesNotCollapseToSingleHighestOwner|TestA7OwnerHintsDoNotRemoveRuntimeTargets|TestA7RejectsUnusedKeyMaterialOwnerHint|TestTargetLevelMaterialPlanSelectsSupersetOwnerWhenAllowed' -count=1 -timeout=30m
```

## Task 2: Add Key Material Pool and Manifests

**Files:**

- Create `circuits/ckks/bootstrapping/target_level_key_material_pool.go`
- Create `circuits/ckks/bootstrapping/target_level_key_material_pool_test.go`

- [ ] **Step 1: Write failing pool tests**

Add tests for:

- descriptor-only requirements for relinearization, rotation, ring-switch, and
  dense/sparse key kinds;
- one bootstrap secret domain per target manifest;
- rejecting a manifest that mixes key material from multiple bootstrap secret
  domains;
- owner-hint contribution counts based only on physical key descriptors;
- deduplicating identical rotation material;
- separating incompatible secret domains;
- building shared/private manifests;
- rejecting conflicting objects with the same key;
- counting physical, shared, private, and binary-size totals;
- rejecting non-key material kinds.

- [ ] **Step 2: Run RED**

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run '^TestKeyMaterialPool' -count=1 -timeout=30m
```

Expected: fails because `KeyMaterialPool` is missing.

- [ ] **Step 3: Implement pool**

Implement:

```go
func NewKeyMaterialPool() *KeyMaterialPool
func NewBootstrapSecretDomainDescriptor(ownerLevel int, params Parameters) BootstrapSecretDomain
func (p *TargetLevelMaterialPlan) KeyMaterialRequirements() []TargetKeyMaterialRequirement
func (p *TargetLevelMaterialPlan) KeyMaterialManifestForTarget(targetLevel int) (TargetKeyMaterialManifest, bool)
func (p *KeyMaterialPool) Add(owner int, targets []int, key KeyMaterialKey, view KeyMaterialView, value any, binaryLen int) error
func (p *KeyMaterialPool) Object(key KeyMaterialKey) (KeyMaterialObject, bool)
func (p *KeyMaterialPool) ObjectsByKind(kind KeyMaterialKind) []KeyMaterialObject
func (p *KeyMaterialPool) ManifestForTarget(targetLevel int) (TargetKeyMaterialManifest, bool)
func (p *KeyMaterialPool) CountByKind(kind KeyMaterialKind) int
func (p *KeyMaterialPool) PhysicalMaterialCount() int
func (p *KeyMaterialPool) MaterialBinarySize() int
func (p *KeyMaterialPool) SharedMaterialCount() int
func (p *KeyMaterialPool) PrivateMaterialCount() int
```

`Add` must reject material kinds outside the four allowed key kinds.
`KeyMaterialRequirements` must be descriptor-only: it must not call
`GenEvaluationKeys`, `GenRelinearizationKeyNew`, `GenGaloisKeyNew`,
`GenEvaluationKeyNew`, or `NewEvaluator`.
`NewBootstrapSecretDomainDescriptor` must not hash generated secret bytes. It
creates the stable logical domain used by descriptor planning; physical keygen
later binds the actual owner bootstrap secret to that descriptor.

- [ ] **Step 4: Run GREEN**

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run '^TestKeyMaterialPool' -count=1 -timeout=30m
```

## Task 3: Generate Key Material into the Pool

**Files:**

- Modify `circuits/ckks/bootstrapping/target_level_material.go`
- Modify `circuits/ckks/bootstrapping/target_level_material_test.go`

- [ ] **Step 1: Write failing generation tests**

Add tests that:

- generate A7 key material for multiple target levels;
- generate shared core keys under the selected owner bootstrap secret domain;
- generate target-private ring/domain switch keys to or from the selected owner
  bootstrap secret domain;
- reject any target manifest whose keys reference more than one bootstrap secret
  domain;
- assert the pool contains only relinearization, rotation, ring-switch, and
  dense/sparse key kinds;
- assert no DFT, matrix schedule, linear transformation, or encoded diagonal
  material is present;
- assert the target manifest contains all required key kinds for that target.

- [ ] **Step 2: Run RED**

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run 'TestKeyMaterialPoolAddsOnlyKeyMaterialKinds|TestKeyMaterialPoolBuildsTargetManifest' -count=1 -timeout=30m
```

- [ ] **Step 3: Implement generation**

Extend `ReusableEvaluationKeys` with:

```go
keyMaterialPool *KeyMaterialPool

func (k *ReusableEvaluationKeys) KeyMaterialPool() *KeyMaterialPool
```

For A7 policy, `GenReusableEvaluationKeys` must:

- consume the descriptor-only requirement plan produced during planning;
- generate or derive exactly one bootstrap secret domain per selected
  key-material owner descriptor;
- keep a private map from `BootstrapSecretDomain` to the actual generated owner
  bootstrap secret;
- generate shared core keys once under the selected owner bootstrap secret
  domain;
- generate target-private switch keys against the same selected owner bootstrap
  secret domain;
- insert each key into `KeyMaterialPool`;
- build target key manifests;
- compute binary size from key `BinarySize()` only;
- skip evaluator creation and DFT material extraction.

- [ ] **Step 4: Run GREEN**

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run 'TestKeyMaterialPoolAddsOnlyKeyMaterialKinds|TestKeyMaterialPoolBuildsTargetManifest|TestSharedKeyPoolMaterialIDsCoverBootstrappingKinds' -count=1 -timeout=30m
```

## Task 4: Assemble Target Evaluation Keys and Evaluators

**Files:**

- Create `circuits/ckks/bootstrapping/target_level_key_evaluator_assembly.go`
- Create `circuits/ckks/bootstrapping/target_level_key_evaluator_assembly_test.go`

- [ ] **Step 1: Write failing assembly tests**

Add tests for:

- `BuildTargetEvaluationKeys` returns a complete `EvaluationKeys`;
- missing relinearization, rotation, ring-switch, or dense/sparse key material
  returns `ErrMissingReusableMaterial`;
- mixed bootstrap secret domains in one manifest return
  `ErrIncompatibleReusableMaterial`;
- RNS-prefix key views are rejected for A7 v1 target evaluator assembly;
- `BuildTargetEvaluator` returns an evaluator with `OutputLevel() == target`;
- removing DFT material does not affect construction because DFT material is not
  in the pool;
- `BuildTargetEvaluator` uses `NewEvaluator(targetParams, keys)`.

- [ ] **Step 2: Run RED**

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run 'TestBuildTargetEvaluationKeys|TestBuildTargetEvaluator' -count=1 -timeout=30m
```

- [ ] **Step 3: Implement assembly**

Implement:

```go
func (p *TargetLevelMaterialPlan) BuildTargetEvaluationKeys(pool *KeyMaterialPool, targetLevel int) (*EvaluationKeys, error)
func (p *TargetLevelMaterialPlan) BuildTargetEvaluator(pool *KeyMaterialPool, targetLevel int) (*Evaluator, error)
```

`BuildTargetEvaluator` must call:

```go
eval, err := NewEvaluator(targetParams, targetEvaluationKeys)
```

It must not call `NewEvaluatorFromMaterialBundle`, and it must not inject
prebuilt DFT matrices.

`BuildTargetEvaluationKeys` must verify that every manifest entry points to
exact physical key material. It must reject `KeyMaterialViewRNSPrefix` for A7
v1. It must reject any key whose `BootstrapSecretDomain` is incompatible with
the manifest's `BootstrapSecretDomain`; for relinearization and rotation keys,
`SecretDomain` must also equal the manifest bootstrap domain.

- [ ] **Step 4: Run GREEN**

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run 'TestBuildTargetEvaluationKeys|TestBuildTargetEvaluator' -count=1 -timeout=30m
```

## Task 5: Wire Runtime Bootstrap to Target Evaluators

**Files:**

- Modify `circuits/ckks/bootstrapping/target_level_material.go`
- Modify `circuits/ckks/bootstrapping/target_level_material_test.go`

- [ ] **Step 1: Write failing runtime test**

Add `TestTargetLevelBootstrapperKeyMaterialPoolUsesDedicatedTargetEvaluator`.
It must prepare at least two target levels and assert:

- `BootstrapAtLevel(ct, r)` uses target evaluator `r`;
- output level equals `r`;
- unconfigured targets fail;
- runtime does not generate additional key material.

Add `TestKeyMaterialPoolTargetEvaluatorCoversAllLegalFastTargets`. It must build
the fast profile's complete constructible target-level set, prepare A7 key
material once, and verify every target returns `Level() == target`.

- [ ] **Step 2: Run RED**

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run '^TestTargetLevelBootstrapperKeyMaterialPoolUsesDedicatedTargetEvaluator$' -count=1 -timeout=30m
```

- [ ] **Step 3: Implement runtime wiring**

For `ReuseKeyMaterialPoolTargetEvaluator`, `NewTargetLevelBootstrapper` must
prebuild a target evaluator for every declared future target from
`keys.KeyMaterialPool()`.

Runtime calls must select by exact target level:

```go
eval := evaluators[targetLevel]
```

No `DropLevel` is used in A7 runtime selection.

- [ ] **Step 4: Run GREEN**

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run 'TestTargetLevelBootstrapperKeyMaterialPoolUsesDedicatedTargetEvaluator|TestTargetLevelBootstrapperSupersetDropMatchesDirectTarget|TestTargetLevelBootstrapperExactOwnersCoverAllLegalFastTargets' -count=1 -timeout=30m
```

Add the A7 all-legal fast target test to this command before marking the task
green:

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run 'TestTargetLevelBootstrapperKeyMaterialPoolUsesDedicatedTargetEvaluator|TestKeyMaterialPoolTargetEvaluatorCoversAllLegalFastTargets|TestTargetLevelBootstrapperSupersetDropMatchesDirectTarget|TestTargetLevelBootstrapperExactOwnersCoverAllLegalFastTargets' -count=1 -timeout=30m
```

## Task 6: Update Reports and Benchmarks

**Files:**

- Modify `circuits/ckks/bootstrapping/bootstrap_key_reuse_research_benchmark_suite_test.go`
- Modify `circuits/ckks/bootstrapping/bootstrap_key_reuse_research_benchmark_test.go`
- Modify `docs/bootstrap_key_reuse/research_benchmark_suite_spec.md`
- Modify `docs/bootstrap_key_reuse/target_count_sweep_experiment_plan.md`

- [ ] **Step 1: Write failing report tests**

Add tests that assert A7 report rows include:

- `key_material_total_mb`;
- physical key count;
- shared key count;
- private key count;
- target evaluator count.
- key-material owner hint count.
- target manifest bootstrap secret-domain count.

The tests must assert that DFT encoded diagonal bytes are not included in
`key_material_total_mb`. They must also assert the exact accounting formula:

```text
key_material_total_bytes = sum(BinarySize()) over unique physical key objects
key_material_total_mb = key_material_total_bytes / 1024 / 1024
```

The unique physical key objects are only relinearization keys, individual
Galois keys, non-nil ring/domain switch evaluation keys, and non-nil
dense/sparse evaluation keys. Wrapper overhead from `EvaluationKeys` and
`MemEvaluationKeySet`, manifests, logical/RNS-prefix views, DFT matrices,
linear transformations, schedules, and encoded diagonals are excluded.

Original Lattigo rows must use the same object-level decomposition over each
independently generated per-target `EvaluationKeys`; repeated key objects from
different independent evaluators are counted once per evaluator instance.

A7 rows must expose enough diagnostics to audit owner-hint contribution:

- `key_material_owner_hint_count`;
- `contributing_key_material_owner_count`;
- `target_manifest_count`;
- `bootstrap_secret_domain_count`.

- [ ] **Step 2: Run RED**

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run 'TestA7PlanReportExplainsSharedAndPrivateKeyMaterial|TestTargetCountSweepIncludesKeyMaterialPoolScheme' -count=1 -timeout=30m
```

- [ ] **Step 3: Implement reports**

Rename A7 scheme labels to key-material terminology:

- `ours_key_material_pool_target_evaluator`;
- `key_material_total_mb`;
- `key_material_total_bytes`;
- `shared_key_material_count`;
- `private_key_material_count`.

Optional DFT diagnostic columns must use names such as
`dft_encoded_diagonal_diagnostic_mb` and must not be summed into key material.

- [ ] **Step 4: Run GREEN**

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run 'TestA7PlanReportExplainsSharedAndPrivateKeyMaterial|TestTargetCountSweepIncludesKeyMaterialPoolScheme' -count=1 -timeout=30m
```

## Task 7: Final Validation Gates

**Files:**

- Modify docs under `docs/bootstrap_key_reuse/`
- Modify `docs/README.md` only if links need renaming

- [ ] **Step 1: Run fast gate**

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run 'TestTargetLevelMaterial|TestKeyMaterialPool|TestBuildTargetEvaluationKeys|TestBuildTargetEvaluator|TestTargetLevelBootstrapper|TestSharedKeyPool|TestRNSPrefix|TestBootstrapKeyReuseA[0-6]' -count=1 -timeout=30m -args -bkr.result-dir=/tmp/lattigo-bkr-a7-fast
```

- [ ] **Step 2: Run all-legal A7 gates**

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run 'TestKeyMaterialPoolTargetEvaluatorCoversAllLegalFastTargets|TestKeyMaterialPoolTargetEvaluatorCoversAllLegalLongTargets' -count=1 -timeout=0 -args -long -bkr.result-dir=/tmp/lattigo-bkr-a7-all-legal
```

- [ ] **Step 3: Run example material-size check**

```bash
GOCACHE=/tmp/lattigo-gocache go run ./examples/singleparty/ckks_bootstrapping/material_size
```

Expected: normal `N=2^16`, 12-Q-limb output shows key sets as the dominant
persistent material cost. This is only evidence for A7 scope selection; it is
not a correctness gate.

- [ ] **Step 4: Verify docs**

```bash
rg -n 'EvaluatorMaterialBundle|NewEvaluatorFromMaterialBundle|MaterialKindLinearTransformation|linear_transformation|encoded_diagonal|matrix_schedule' docs/bootstrap_key_reuse/a7_key_material_pool_target_evaluator_spec.md docs/bootstrap_key_reuse/a7_key_material_pool_target_evaluator_plan.md
```

Expected: matches are limited to explicit non-goal, excluded-material,
negative-test, or diagnostic wording. Any match that defines those terms as A7
runtime material, key material kind, or acceptance requirement is a failure.

- [ ] **Step 5: Run whitespace check**

```bash
git diff --check
```

## Completion Criteria

A7 is complete when:

1. Runtime targets equal declared `FutureTargetLevels`.
2. Each target evaluator has `OutputLevel() == targetLevel`.
3. Physical reuse is limited to key material.
4. `BuildTargetEvaluationKeys` assembles target keys from the pool.
5. `BuildTargetEvaluator` calls `NewEvaluator(targetParams, keys)`.
6. DFT matrices, linear transformations, schedules, and encoded diagonals are
   outside A7 physical reuse.
7. Benchmarks report key material size separately from optional DFT diagnostics.
8. Descriptor-only planning rejects unused owner hints before key generation.
9. Each target manifest uses exactly one bootstrap secret domain.
10. Fast and long all-legal-target A7 gates pass.
