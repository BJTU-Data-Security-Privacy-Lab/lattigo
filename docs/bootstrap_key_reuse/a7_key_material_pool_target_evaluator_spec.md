# A7 Key Material Pool Target Evaluator Spec

This document defines the reduced A7 final method for CKKS arbitrary
target-level bootstrapping. A7 reuses only bootstrap key-set material across
declared target levels. It deliberately does not reuse DFT matrices, linear
transformations, matrix schedules, or encoded diagonals.

The reduction is based on the material-size evidence for the normal
`N=2^16`, 12-Q-limb profile: key sets dominate persistent material cost, while
DFT encoded diagonal material is smaller and is not worth expanding the reuse
boundary for the final method.

## 1. Goal

A7 must satisfy all requirements below:

1. The caller declares the complete set of future bootstrap target levels.
2. Preparation generates the key material required by those target levels.
3. Physically identical key material across target levels is generated once.
4. Target-specific key material is generated separately and attached only to
   that target manifest.
5. Runtime `BootstrapAtLevel(ct, r)` uses an evaluator built for target level
   `r`.
6. The target evaluator uses `targetLevelParameters(r)`.
7. The returned ciphertext has `Level() == r`.
8. Runtime bootstrap never generates keys or mutates physical key material.
9. Superset-output `DropLevel` remains available only as an explicit fallback
   policy and is not reported as the A7 final method.

## 2. Non-Goals

1. A7 does not remove A0-A6. They remain oracle, accounting, compatibility, and
   fallback evidence.
2. A7 does not change `Evaluator.Bootstrap` or `Evaluator.BootstrapMany`.
3. A7 does not infer sharing from target-level integers alone.
4. A7 does not reuse or pool `lintrans.LinearTransformation`,
   `dft.Matrix`, matrix schedule metadata, or encoded diagonal material.
5. A7 does not add `EvaluatorMaterialBundle` or
   `NewEvaluatorFromMaterialBundle`.
6. A7 does not forbid `NewEvaluator` from generating target-local DFT matrices.
   DFT material is evaluator-local runtime preparation, not reusable A7 key
   material.
7. A7 does not silently reuse RNS prefix views unless the relevant key kind has
   positive and negative compatibility tests.
8. A7 does not treat benchmark improvements as proof of correctness.

## 3. Required Public API Changes

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

`ReuseKeyMaterialPoolTargetEvaluator` is the default final-method policy for
A7 tests and benchmarks. If implementation keeps the older experimental name
`ReuseMaterialPoolTargetEvaluator` for compatibility, that name must be
documented as a key-material-only policy.

Required semantics:

1. `FutureTargetLevels` is the complete runtime serve set.
2. Under `ReuseKeyMaterialPoolTargetEvaluator`, runtime execution targets are
   exactly `FutureTargetLevels`.
3. The planner must not collapse multiple targets into the highest target.
4. `OwnerTargetLevels`, when provided, are key-material owner hints only. They
   cannot remove any declared runtime target evaluator.
5. Every non-empty `OwnerTargetLevels` entry must be legal and must contribute
   at least one physical key material object to at least one declared target
   manifest. If an owner hint contributes nothing, planning must reject it with
   an explicit reason such as `unused_key_material_owner_hint`.
6. Runtime evaluator ownership is always exact:
   `runtimeOwner(targetLevel) == targetLevel`.
7. Reports must expose key-material owner hints separately from runtime target
   evaluators. Existing report fields may remain for backward compatibility,
   but their meaning must be explicit.
8. `AllowSupersetDrop` affects only `ReuseExactPrefixAndSupersetDrop`.
9. `BootstrapAtLevel(ct, r)` must reject target `r` if `r` is not declared in
   `FutureTargetLevels`.
10. `BootstrapAtLevel(ct, r)` must call an evaluator whose
   `OutputLevel() == r`.

## 4. Key Material Model

### 4.1 KeyMaterialKey

A7 uses structured key material identity. Two pieces of key material can be
physically shared only when their identity is compatible for that key kind.

```go
type KeyMaterialKind string

const (
	KeyMaterialKindRelinearization KeyMaterialKind = "relinearization_key"
	KeyMaterialKindRotation        KeyMaterialKind = "rotation_key"
	KeyMaterialKindRingSwitch      KeyMaterialKind = "ring_switch_key"
	KeyMaterialKindDenseSparse     KeyMaterialKind = "dense_sparse_key"
)

type KeyMaterialKey struct {
	Kind           KeyMaterialKind
	LevelQ         int
	LevelP         int
	ParametersHash string
	SecretDomain   string
	GaloisElement  uint64
	Direction      string
	DescriptorHash string
}
```

Required discriminators:

| Kind | Required discriminators |
| --- | --- |
| `relinearization_key` | `LevelQ`, `LevelP`, `ParametersHash`, `SecretDomain`, decomposition descriptor in `DescriptorHash` |
| `rotation_key` | all relinearization fields plus `GaloisElement` |
| `ring_switch_key` | all relinearization fields plus `Direction` and source/destination secret domains encoded in `SecretDomain` |
| `dense_sparse_key` | all relinearization fields plus `Direction` and dense/sparse secret domains encoded in `SecretDomain` |

The following are out of scope for A7 key material identity:

1. Matrix name.
2. Transform index.
3. Diagonal index.
4. DFT matrix descriptor.
5. Encoded polynomial descriptor.

### 4.2 KeyMaterialPool

`KeyMaterialPool` owns physical key material and target key manifests:

```go
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
	TargetLevel int
	Shared      []KeyMaterialKey
	Private     []KeyMaterialKey
}

type KeyMaterialPool struct {
	objects   map[KeyMaterialKey]KeyMaterialObject
	manifests map[int]TargetKeyMaterialManifest
}
```

Accepted runtime object values:

1. `*rlwe.RelinearizationKey`.
2. `*rlwe.GaloisKey`.
3. `*rlwe.EvaluationKey` for ring/domain switch keys.
4. `*rlwe.EvaluationKey` for dense/sparse encapsulation keys.

The pool must not store or require executable DFT material. Target evaluators
may generate `C2SDFTMatrix` and `S2CDFTMatrix` through the existing
`NewEvaluator` path.

### 4.3 Target Manifest

Every declared target level has exactly one key manifest.

The manifest must contain every key needed to build an `EvaluationKeys` object
for that target:

1. Relinearization key.
2. Required Galois keys.
3. Required ring/domain switch keys when the target parameters require them.
4. Dense/sparse encapsulation keys when `EphemeralSecretWeight != 0`.

If a target manifest is missing any required key, evaluator construction must
fail before runtime bootstrap.

## 5. Target Evaluator Assembly

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

`BuildTargetEvaluationKeys` must assemble:

1. `EvkN1ToN2` when required.
2. `EvkN2ToN1` when required.
3. `EvkRealToCmplx` when required.
4. `EvkCmplxToReal` when required.
5. `EvkDenseToSparse` when required.
6. `EvkSparseToDense` when required.
7. `MemEvaluationKeySet` containing the target relinearization key and all
   required Galois keys.

`BuildTargetEvaluator` must:

1. Look up `targetLevelParameters(targetLevel)`.
2. Build target `EvaluationKeys` from the pool.
3. Call `NewEvaluator(targetParams, targetEvaluationKeys)`.
4. Verify `eval.OutputLevel() == targetLevel`.
5. Return `ErrMissingReusableMaterial` when required key material is absent.

`BuildTargetEvaluator` is allowed to let `NewEvaluator` generate DFT matrices.
That work is not key material reuse and is outside A7's sharing boundary.

## 6. Compatibility Predicates

### 6.1 Relinearization Keys

A7 may share relinearization keys only when all fields below match:

1. `LevelQ`.
2. `LevelP`.
3. `ParametersHash`.
4. `SecretDomain`.
5. Decomposition descriptor.

### 6.2 Rotation Keys

A7 may share rotation keys only when all relinearization-key fields match and
`GaloisElement` matches.

### 6.3 Ring/Domain Switch Keys

A7 may share ring/domain switch keys only when:

1. `Direction` matches.
2. Source and destination secret domains match.
3. Key levels and decomposition descriptors match.
4. Parameter hashes match.

`N1ToN2`, `N2ToN1`, `RealToCmplx`, and `CmplxToReal` are distinct directions.

### 6.4 Dense/Sparse Keys

A7 may share dense/sparse encapsulation keys only when:

1. `Direction` matches.
2. Dense and sparse secret domains match.
3. Key levels and decomposition descriptors match.
4. Parameter hashes match.

`DenseToSparse` and `SparseToDense` are distinct directions.

### 6.5 RNS Prefix Views

RNS prefix views are disabled by default. When enabled, they apply only to key
material kinds with explicit positive and negative tests. Prefix views must not
be inferred for DFT matrices, linear transformations, schedules, or encoded
diagonals.

## 7. Reporting and Benchmarks

A7 reporting must distinguish:

1. Declared runtime targets.
2. Key-material owner hints.
3. Physical key material count and bytes.
4. Shared key material count and bytes.
5. Private key material count and bytes.
6. Runtime bootstrap latency.

`key_material_total_bytes` is defined as the sum of `BinarySize()` over unique
physical key objects in the `KeyMaterialPool`, counted exactly once per
physical object:

1. one `*rlwe.RelinearizationKey` when present;
2. each required `*rlwe.GaloisKey`;
3. each non-nil ring/domain switch `*rlwe.EvaluationKey`;
4. each non-nil dense/sparse `*rlwe.EvaluationKey`.

`key_material_total_mb = key_material_total_bytes / 1024 / 1024`.

This total excludes `EvaluationKeys` and `MemEvaluationKeySet` wrapper
overhead, target manifests, logical/RNS-prefix views, DFT matrices,
linear transformations, matrix schedules, and encoded diagonals. Original
Lattigo comparison rows must use the same object-level decomposition on each
independently generated per-target `EvaluationKeys`; repeated keys from
different independent target evaluators are counted once per evaluator
instance because they are physically distinct objects.

Shared and private MB are derived from the same physical-object set:

1. shared key material is physical key material referenced by more than one
   target manifest;
2. private key material is physical key material referenced by exactly one
   target manifest;
3. shared MB plus private MB must equal `key_material_total_mb`.

DFT encoded diagonal bytes may be reported in a separate diagnostic column, but
they must not be included in A7 key material reuse savings.

## 8. Required Acceptance Tests

Fast acceptance tests:

1. `TestKeyMaterialPoolPolicyDoesNotCollapseToSingleHighestOwner`.
2. `TestA7OwnerHintsDoNotRemoveRuntimeTargets`.
3. `TestA7RejectsUnusedKeyMaterialOwnerHint`.
4. `TestKeyMaterialPoolDeduplicatesSharedRotationMaterial`.
5. `TestKeyMaterialPoolSeparatesIncompatibleSecretDomains`.
6. `TestKeyMaterialPoolBuildsTargetManifest`.
7. `TestKeyMaterialPoolRejectsConflictingObjectsWithSameKey`.
8. `TestKeyMaterialPoolAddsOnlyKeyMaterialKinds`.
9. `TestBuildTargetEvaluationKeysUsesManifestKeys`.
10. `TestBuildTargetEvaluationKeysRejectsMissingKeyMaterial`.
11. `TestBuildTargetEvaluatorUsesDedicatedTargetEvaluator`.
12. `TestBuildTargetEvaluatorAllowsNewEvaluatorDFTGeneration`.
13. `TestTargetLevelBootstrapperKeyMaterialPoolUsesDedicatedTargetEvaluator`.
14. `TestA7PlanReportExplainsSharedAndPrivateKeyMaterial`.
15. `TestTargetCountSweepIncludesKeyMaterialPoolScheme`.

Negative acceptance tests:

1. No `linear_transformation` material kind is required by A7.
2. No `matrix_schedule` material kind is required by A7.
3. No `encoded_diagonal` material kind is required by A7.
4. Missing DFT material must not prevent target evaluator construction.
5. `NewEvaluatorFromMaterialBundle` must not be introduced for A7.

## 9. Completion Criteria

A7 is complete only when:

1. The final-method policy is key-material-only.
2. Runtime targets equal declared `FutureTargetLevels`.
3. Each runtime evaluator has `OutputLevel() == targetLevel`.
4. Key material is deduplicated by structured key identity.
5. Target-specific keys remain private to their target manifests.
6. Every non-empty key-material owner hint contributes physical key material or
   is rejected before key generation.
7. `BuildTargetEvaluationKeys` assembles target `EvaluationKeys` from the pool.
8. `BuildTargetEvaluator` calls `NewEvaluator(targetParams, keys)`.
9. DFT matrices, linear transformations, schedules, and encoded diagonals are
   not part of A7 physical reuse.
10. Reports and benchmarks measure key material size separately from optional
   DFT diagnostics.
