# CKKS Arbitrary Target-Level Bootstrapping and Reusable Key Material Spec

This document defines the final implementation target for CKKS bootstrapping in
this repository: callers can request any legal bootstrap target level, and the
implementation reuses compatible bootstrap key-set material across target
levels.

This document is a requirements source, not a status claim. A0-A6 remain the
evidence chain and regression oracle. A7 is the reduced final method: key-set
reuse only.

## 1. Goal

The final implementation must satisfy all requirements below:

1. A caller can request any legal bootstrap output target level for a supported
   CKKS bootstrapping parameter set.
2. A successful call returns a ciphertext whose `Level()` is exactly the
   requested target level.
3. The returned scale matches direct bootstrapping to the same target level.
4. Output precision is equivalent to the A0 direct-BK(target) oracle under the
   repository's existing bootstrap precision thresholds.
5. Compatible bootstrap key material is generated once as physical material and
   reused through target manifests.
6. Incompatible key material is never reused.
7. Owner-hint contribution is proven by descriptor-only planning before key
   generation.
8. Every target manifest uses one coherent bootstrap secret domain.
9. Existing `Evaluator.Bootstrap` and `Evaluator.BootstrapMany` behavior stays
   compatible unless the caller opts into the target-level API.

## 2. Non-Goals

1. Do not weaken CKKS security, secret-key domain separation, modulus-chain
   semantics, scale semantics, or precision requirements.
2. Do not decide key compatibility by target-level integers, file names, string
   labels, or plan row names alone.
3. Do not allow an unconfigured target level to trigger key generation at
   runtime.
4. Do not enable RNS-slice key views by default until each key kind has
   positive and negative compatibility tests.
5. Do not treat A0-A6 harness helpers as the public product API.
6. Do not pool or reuse DFT matrices, linear transformations, matrix schedules,
   or encoded diagonals in A7.
7. Do not introduce `EvaluatorMaterialBundle` or
   `NewEvaluatorFromMaterialBundle` for A7.

## 3. Terms

- **Target level**: the requested bootstrap output level, written as `r`.
- **Direct BK(r)**: a bootstrapping evaluator and key set generated directly for
  target level `r`.
- **Key owner level**: the target level that owns physical reusable key
  material.
- **Consumer target**: a target level whose manifest references owner key
  material.
- **Physical key material**: generated relinearization, rotation, ring/domain
  switch, or dense/sparse key material that occupies memory.
- **Logical key view**: a read-only view of physical key material with exact,
  prefix, or explicit fallback semantics.
- **Key identity**: the structured identifier used to prove that a key is
  equivalent or view-compatible.
- **Closed-world key plan**: a prepared key plan that can serve only the
  caller-declared `FutureTargetLevels`.
- **Superset output**: bootstrapping at a higher owner level and applying
  `DropLevel` to reach the requested lower target level.

## 4. Legal Target Level

A legal target level is not just an integer. For a full CKKS profile and
bootstrapping literal, target level `r` is legal only when all checks below
pass:

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
with the same semantics.

## 5. Final Policy

Extend the reuse policy enum:

```go
type ReusePolicy int

const (
	ReuseExactOnly ReusePolicy = iota
	ReuseExactAndPrefix
	ReuseExactPrefixAndSupersetDrop
	ReuseKeyMaterialPoolTargetEvaluator
)
```

`ReuseKeyMaterialPoolTargetEvaluator` is the final default method.

Policy semantics:

1. `FutureTargetLevels` is the complete runtime serve set.
2. Runtime evaluator ownership is exact:
   `runtimeOwner(targetLevel) == targetLevel`.
3. `OwnerTargetLevels` are key-material owner hints only.
4. `OwnerTargetLevels` cannot remove declared runtime targets.
5. Every non-empty owner hint must be legal and must contribute at least one
   physical key material object to at least one declared target manifest.
   Contribution must be computed from descriptor-only key requirements before
   physical key generation. An unused hint must be rejected before key generation
   with an explicit reason such as `unused_key_material_owner_hint`.
6. `AllowSupersetDrop` affects only `ReuseExactPrefixAndSupersetDrop`.
7. `BootstrapAtLevel(ct, r)` rejects undeclared targets.
8. `BootstrapAtLevel(ct, r)` calls an evaluator whose `OutputLevel() == r`.

## 6. Key Material Model

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
```

Required discriminators:

| Kind | Required discriminators |
| --- | --- |
| `relinearization_key` | `LevelQ`, `LevelP`, `ParametersHash`, `BootstrapSecretDomain`, `SecretDomain`, decomposition descriptor |
| `rotation_key` | all relinearization fields plus `GaloisElement` |
| `ring_switch_key` | `LevelQ`, `LevelP`, `ParametersHash`, `BootstrapSecretDomain`, `Direction`, and source/destination secret domains |
| `dense_sparse_key` | `LevelQ`, `LevelP`, `ParametersHash`, `BootstrapSecretDomain`, `Direction`, and dense/sparse secret domains |

A7 key identity does not include matrix name, transform index, diagonal index,
DFT matrix descriptor, or encoded polynomial descriptor.

## 7. Key Material Pool

```go
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
```

Accepted object values:

1. `*rlwe.RelinearizationKey`.
2. `*rlwe.GaloisKey`.
3. `*rlwe.EvaluationKey` for ring/domain switch keys.
4. `*rlwe.EvaluationKey` for dense/sparse keys.

Each declared target has one manifest containing all keys needed to assemble an
`EvaluationKeys` object for that target.

Each manifest must select exactly one `BootstrapSecretDomain`. This value is a
stable logical descriptor available during descriptor-only planning; it must not
depend on generated secret-key bytes. Physical key generation later binds the
actual owner bootstrap secret to the descriptor. Shared relinearization and
rotation keys, target-private ring/domain switch keys, and dense/sparse keys in
that manifest must all be compatible with the selected domain through
`KeyMaterialKey.BootstrapSecretDomain`. Private switch keys are private because
their target residual or sparse secret is target-specific; they must not
introduce a second bootstrap secret domain into the same manifest.

Before physical key generation, the planner must compute descriptor-only
requirements:

```go
type TargetKeyMaterialRequirement struct {
	TargetLevel           int
	CandidateOwnerLevel   int
	BootstrapSecretDomain string
	Key                   KeyMaterialKey
	Shareable             bool
}
```

This requirement plan is the source of truth for owner-hint contribution checks.
It must not call physical key generation or `NewEvaluator`.

## 8. Target Evaluator Assembly

A7 has one assembly boundary: `EvaluationKeys`.

```go
func (p *TargetLevelMaterialPlan) BuildTargetEvaluationKeys(
	pool *KeyMaterialPool,
	targetLevel int,
) (*EvaluationKeys, error)

func (p *TargetLevelMaterialPlan) BuildTargetEvaluator(
	pool *KeyMaterialPool,
	targetLevel int,
) (*Evaluator, error)
```

`BuildTargetEvaluationKeys` assembles:

1. Ring/domain switch keys required by the target parameters.
2. Dense/sparse encapsulation keys when required.
3. Relinearization key.
4. Required Galois keys.
5. `MemEvaluationKeySet`.

`BuildTargetEvaluator` must:

1. Select target parameters for `targetLevel`.
2. Assemble target `EvaluationKeys`.
3. Call `NewEvaluator(targetParams, targetEvaluationKeys)`.
4. Verify `eval.OutputLevel() == targetLevel`.

`NewEvaluator` may generate target-local DFT matrices. DFT matrices are outside
A7 physical reuse.

## 9. Compatibility Rules

Relinearization key reuse requires matching:

1. `LevelQ`.
2. `LevelP`.
3. `ParametersHash`.
4. `SecretDomain`.
5. Decomposition descriptor.

Rotation key reuse requires all relinearization-key fields plus identical
`GaloisElement`.

Ring/domain switch key reuse requires identical direction, source/destination
secret domains, levels, decomposition descriptor, and parameter hash.

Dense/sparse key reuse requires identical direction, dense/sparse secret
domains, levels, decomposition descriptor, and parameter hash.

RNS prefix views are outside A7 v1 and must be rejected by
`ReuseKeyMaterialPoolTargetEvaluator` assembly. They may be promoted into a
later explicit policy only for key material kinds with positive and negative
compatibility tests, and only when the view preserves the target manifest's
selected bootstrap secret domain.

## 10. Reporting and Benchmarks

A7 reports must include:

1. Declared target levels.
2. Runtime target evaluator count.
3. Key-material owner hints.
4. Physical key material count and bytes.
5. Shared key material count and bytes.
6. Private key material count and bytes.
7. Bootstrap latency.

`key_material_total_bytes` is the sum of `BinarySize()` over unique physical
key objects, counted once per physical object:

1. one `*rlwe.RelinearizationKey` when present;
2. each required `*rlwe.GaloisKey`;
3. each non-nil ring/domain switch `*rlwe.EvaluationKey`;
4. each non-nil dense/sparse `*rlwe.EvaluationKey`.

`key_material_total_mb = key_material_total_bytes / 1024 / 1024`.

The total excludes `EvaluationKeys` and `MemEvaluationKeySet` wrapper
overhead, manifests, logical/RNS-prefix views, DFT matrices, linear
transformations, matrix schedules, and encoded diagonals.

Original Lattigo comparison rows must use the same object-level decomposition
on each independently generated per-target `EvaluationKeys`; repeated keys
from different independent target evaluators are counted once per evaluator
instance because they are physically distinct objects.

Shared key material is physical key material referenced by more than one target
manifest. Private key material is physical key material referenced by exactly
one target manifest. Shared MB plus private MB must equal
`key_material_total_mb`.

Optional DFT diagnostic columns must not be included in key material totals.

## 11. Acceptance Tests

Required fast tests:

1. Legal target validation rejects invalid target levels.
2. A0 direct BK(target) oracle still passes.
3. A7 does not collapse future targets to the highest owner.
4. A7 owner hints do not remove runtime targets.
5. A7 rejects owner hints that contribute no physical key material.
6. Key material pool deduplicates shared rotation keys.
7. Key material pool separates incompatible secret domains.
8. Target manifests include all required key material.
9. Target evaluation-key assembly rejects missing required keys.
10. Target evaluator assembly calls `NewEvaluator(targetParams, keys)`.
11. Target evaluator output level equals requested target.
12. Runtime rejects undeclared target levels.
13. A7 reports shared/private key material counts.
14. Benchmarks include A7 key-material-pool scheme.
15. DFT matrices, linear transformations, schedules, and encoded diagonals are
    not required A7 material kinds.
16. Descriptor-only planning rejects unused owner hints before physical key
    generation.
17. A7 target manifests reject mixed bootstrap secret domains.
18. A7 target evaluator assembly rejects `KeyMaterialViewRNSPrefix`.
19. A7 includes all-legal-target gates for at least one fast profile and one
    long profile.

## 12. Completion Criteria

The final goal is complete only when:

1. All legal target levels can be bootstrapped through the target-level API.
2. Every output level equals the requested target level.
3. Key material is deduplicated only when structured compatibility is proven.
4. Runtime bootstrap never generates key material.
5. A7 physical reuse is limited to key sets.
6. Non-empty key-material owner hints contribute physical key material or are
   rejected before key generation.
7. Superset/drop remains explicit fallback behavior.
8. Reports and benchmarks measure key material reuse without folding in DFT
   encoded diagonal material.
9. Every target manifest uses exactly one bootstrap secret domain.
10. All-legal-target A7 gates pass for fast and long profiles.
