# A6 Superset-Output DropLevel View Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement A6 as `A5 + superset-output bootstrap then DropLevel view, if allowed`, and prove it against A0 direct `BK(r)` for output level, scale, decoded precision, and latency recording.

**Architecture:** Keep A0-A5 behavior untouched. Add an A6-only exploratory dispatcher that maps each requested target level to a compatible owner evaluator at the highest configured output level, bootstraps with that owner, and drops the ciphertext level to the requested target level without rescaling. A6 records owner/view material rows in the existing CSV schema and uses dedicated tests to compare A6 with A0.

**Tech Stack:** Go tests in `circuits/ckks/bootstrapping`, existing Lattigo CKKS/RLWE APIs, existing bootstrap key reuse CSV harness, `ckks.Evaluator.DropLevel`/`Ciphertext.Resize` semantics already used by the repository.

---

## A6 Definition

Authoritative source:

```text
A6: A5 + superset-output bootstrap then DropLevel view, if allowed
```

A6 is exploratory by default. It must not change A1-A5 dispatch semantics, and it must not teach the existing `targetLevelBootstrapper` to silently serve an unconfigured target from a higher level. The A6 behavior belongs in A6-specific helpers and tests.

## Completion Requirements

A6 is factually complete only when all requirements below are proven by current-state evidence:

- A6 has its own lane and plan ID:
  - lane: `a6`
  - plan ID: `A6_SupersetOutputDropLevelView`
- A6 reuses the A5 preparation path for the owner target:
  - A2 rotation-key interning remains active.
  - A3 schedule interning remains active.
  - A4 exact encoded-diagonal sharing remains active.
  - A5 RNS slicing remains disabled by default and recorded as `disabled`.
- A6 has an explicit compatibility predicate:
  - owner output level is greater than or equal to the requested target level;
  - owner and consumer are in the same `bkrCaseSpec`;
  - owner and consumer share `LogN`, ring type, log slots, fixed target scale, full profile, bootstrapping profile, and secret-key domain;
  - owner Q chain is a prefix-compatible superset of the consumer target after the final `DropLevel`;
  - no rescale, key switch, re-encryption, or ad hoc parameter change is allowed after the owner bootstrap;
  - unsupported cases fail before runtime or are recorded as an explicit A6 fallback, never as a silent success.
- A6 output validation compares A6 to A0 direct `BK(r)`:
  - `output_level == r`;
  - `expected_output_level == r`;
  - `output_scale_equality == true`;
  - `bootstrap_error_status == ok`;
  - decoded real and imaginary precision remain at or above `bkrMinPrecisionBits`;
  - A0 and A6 runtime rows both contain positive bootstrap latency values for each compared target.
- A6 CSV output is CSV-only and preserves the existing A0-A6 schema:
  - no `.json`, `.jsonl`, or `.log` artifacts;
  - no schema column changes;
  - A6 owner/view state is represented through lane, plan ID, material metrics, rotation view rows, and fallback reason strings.
- A6 does not claim production default optimization:
  - tests and docs describe it as exploratory;
  - A1-A5 tests still prove lower lanes do not use high-level bootstrap plus `DropLevel`.

## File Structure

- Modify `circuits/ckks/bootstrapping/bootstrap_key_reuse_common_test.go`
  - Add A6 lane and plan constants.
  - Add `bkrA6RunConfig`.
- Create `circuits/ckks/bootstrapping/bootstrap_key_reuse_a6_test.go`
  - A6 owner selection.
  - A6 compatibility validation.
  - A6 superset bootstrap and level-drop dispatcher.
  - A6 runner.
  - A6 contract and A0 comparison tests.
- Reuse `circuits/ckks/bootstrapping/bootstrap_key_reuse_a5_test.go`
  - Keep A5 as the owner preparation model; do not move A5 behavior.
- Reuse `circuits/ckks/bootstrapping/bootstrap_key_reuse_a1_test.go`
  - Reuse `bkrBootstrapA1Ciphertexts` only for direct target dispatch.
  - Reuse `bkrDropCiphertextToLevel` for A6 output drop validation.
- Reuse `circuits/ckks/bootstrapping/bootstrap_key_reuse_a2_test.go`
  - Reuse `bkrPrepareA2TargetForPlan` and `bkrRunA2BootstrapTargetForPlan` patterns.
- Modify `docs/README.md`
  - Link this A6 plan.

## Task 1: Add A6 Lane Wiring

**Files:**

- Modify: `circuits/ckks/bootstrapping/bootstrap_key_reuse_common_test.go`
- Test: `circuits/ckks/bootstrapping/bootstrap_key_reuse_a6_test.go`

- [ ] **Step 1: Add failing compile reference**

Create `bootstrap_key_reuse_a6_test.go` with a tiny compile-only test that references the A6 constants before they exist:

```go
package bootstrapping

import "testing"

func TestBootstrapKeyReuseA6PlanIDContract(t *testing.T) {
	if bkrLaneA6 != "a6" {
		t.Fatalf("bkrLaneA6=%q, want a6", bkrLaneA6)
	}
	if bkrPlanA6 != "A6_SupersetOutputDropLevelView" {
		t.Fatalf("bkrPlanA6=%q", bkrPlanA6)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run '^TestBootstrapKeyReuseA6PlanIDContract$' -count=1 -timeout=30m
```

Expected:

```text
undefined: bkrLaneA6
undefined: bkrPlanA6
```

- [ ] **Step 3: Add constants and run config**

In `bootstrap_key_reuse_common_test.go`, extend the existing constant block:

```go
	bkrLaneA6           = "a6"
	bkrPlanA6           = "A6_SupersetOutputDropLevelView"
```

Add the run config near the existing `bkrA5RunConfig` helper:

```go
func bkrA6RunConfig(spec bkrCaseSpec) bkrRunConfig {
	return bkrRunConfig{
		Lane:         bkrLaneA6,
		PlanID:       bkrPlanA6,
		TargetLevels: append([]int(nil), spec.UsedTargetLevels...),
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run:

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run '^TestBootstrapKeyReuseA6PlanIDContract$' -count=1 -timeout=30m
```

Expected:

```text
ok  	github.com/tuneinsight/lattigo/v6/circuits/ckks/bootstrapping
```

- [ ] **Step 5: Commit**

```bash
git add circuits/ckks/bootstrapping/bootstrap_key_reuse_common_test.go circuits/ckks/bootstrapping/bootstrap_key_reuse_a6_test.go
git commit -m "Add A6 bootstrap key reuse lane wiring"
```

## Task 2: Define A6 Owner Selection and Compatibility

**Files:**

- Modify: `circuits/ckks/bootstrapping/bootstrap_key_reuse_a6_test.go`

- [ ] **Step 1: Write failing owner-selection tests**

Add these tests:

```go
func TestBootstrapKeyReuseA6SelectsMaxUsedOwner(t *testing.T) {
	spec := bkrCaseA6P2MultiFastUsedSparse()
	owner, err := bkrA6OwnerLevel(spec)
	if err != nil {
		t.Fatal(err)
	}
	if owner != 3 {
		t.Fatalf("owner=%d, want 3", owner)
	}
}

func TestBootstrapKeyReuseA6RejectsNoHigherOrEqualOwner(t *testing.T) {
	spec := bkrCaseA6P2MultiFastUsedSparse()
	spec.UsedTargetLevels = nil
	if _, err := bkrA6OwnerLevel(spec); err == nil {
		t.Fatal("expected missing owner to fail")
	}
}

func TestBootstrapKeyReuseA6CompatibilityPredicate(t *testing.T) {
	spec := bkrCaseA6P2MultiFastUsedSparse()
	if err := bkrValidateA6SupersetDrop(spec, 3, 1); err != nil {
		t.Fatalf("expected owner 3 to serve target 1: %v", err)
	}
	if err := bkrValidateA6SupersetDrop(spec, 1, 3); err == nil {
		t.Fatal("expected owner 1 serving target 3 to fail")
	}
}

func TestBootstrapKeyReuseA6SecretKeyDomainCompatibility(t *testing.T) {
	spec := bkrCaseA6P2MultiFastUsedSparse()
	ownerParams, err := bkrA6ResidualParamsForLevel(spec, 3)
	if err != nil {
		t.Fatal(err)
	}
	targetParams, err := bkrA6ResidualParamsForLevel(spec, 1)
	if err != nil {
		t.Fatal(err)
	}
	ownerSK, err := newDeterministicSecretKey(ownerParams, spec.Seed+"/secret-key")
	if err != nil {
		t.Fatal(err)
	}
	targetSK, err := newDeterministicSecretKey(targetParams, spec.Seed+"/secret-key")
	if err != nil {
		t.Fatal(err)
	}
	if !bkrA6SecretKeyPrefixCompatible(ownerSK, targetSK) {
		t.Fatal("expected deterministic owner secret key to be prefix-compatible with target secret key")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run:

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run 'TestBootstrapKeyReuseA6(SelectsMaxUsedOwner|RejectsNoHigherOrEqualOwner|CompatibilityPredicate|SecretKeyDomainCompatibility)' -count=1 -timeout=30m
```

Expected: compile failures for missing A6 case and helper functions.

- [ ] **Step 3: Implement owner selection and compatibility**

Add:

```go
func bkrCaseA6P2MultiFastUsedSparse() bkrCaseSpec {
	return bkrP2MultiFastBase("a6_p2_multi_fast_used_sparse", []int{1, 2, 3}, func(spec *bkrCaseSpec) {
		spec.UsedTargetLevels = []int{1, 3}
	})
}

func bkrA6OwnerLevel(spec bkrCaseSpec) (int, error) {
	if len(spec.UsedTargetLevels) == 0 {
		return 0, errors.New("A6 requires at least one used target level")
	}
	levels := append([]int(nil), spec.UsedTargetLevels...)
	sort.Ints(levels)
	return levels[len(levels)-1], nil
}

func bkrValidateA6SupersetDrop(spec bkrCaseSpec, ownerLevel, targetLevel int) error {
	if err := bkrValidateA1TargetSelection(spec); err != nil {
		return err
	}
	if ownerLevel < targetLevel {
		return fmt.Errorf("A6 owner level %d cannot serve higher target level %d", ownerLevel, targetLevel)
	}
	if !bkrIntInSlice(spec.UsedTargetLevels, ownerLevel) {
		return fmt.Errorf("A6 owner level %d is not in UsedTargetLevels", ownerLevel)
	}
	if !bkrIntInSlice(spec.UsedTargetLevels, targetLevel) {
		return fmt.Errorf("A6 target level %d is not in UsedTargetLevels", targetLevel)
	}

	ownerParams, err := bkrA6ResidualParamsForLevel(spec, ownerLevel)
	if err != nil {
		return fmt.Errorf("cannot instantiate owner residual params: %w", err)
	}
	targetParams, err := bkrA6ResidualParamsForLevel(spec, targetLevel)
	if err != nil {
		return fmt.Errorf("cannot instantiate target residual params: %w", err)
	}

	if ownerParams.LogN() != targetParams.LogN() {
		return fmt.Errorf("A6 owner LogN=%d target LogN=%d", ownerParams.LogN(), targetParams.LogN())
	}
	if ownerParams.RingType() != targetParams.RingType() {
		return fmt.Errorf("A6 owner ring type=%v target ring type=%v", ownerParams.RingType(), targetParams.RingType())
	}
	if ownerParams.DefaultScale().Cmp(targetParams.DefaultScale()) != 0 {
		return fmt.Errorf("A6 owner scale=%s target scale=%s", ownerParams.DefaultScale().Value.Text('g', -1), targetParams.DefaultScale().Value.Text('g', -1))
	}
	if !bkrUint64PrefixEqual(ownerParams.Q(), targetParams.Q()) {
		return fmt.Errorf("A6 owner Q chain is not a prefix-compatible superset of target Q chain")
	}
	if !reflect.DeepEqual(ownerParams.P(), targetParams.P()) {
		return fmt.Errorf("A6 owner P chain differs from target P chain")
	}

	ownerSK, err := newDeterministicSecretKey(ownerParams, spec.Seed+"/secret-key")
	if err != nil {
		return fmt.Errorf("cannot instantiate owner secret key: %w", err)
	}
	targetSK, err := newDeterministicSecretKey(targetParams, spec.Seed+"/secret-key")
	if err != nil {
		return fmt.Errorf("cannot instantiate target secret key: %w", err)
	}
	if !bkrA6SecretKeyPrefixCompatible(ownerSK, targetSK) {
		return errors.New("A6 owner and target secret-key domains are not prefix-compatible")
	}
	return nil
}

func bkrA6ResidualParamsForLevel(spec bkrCaseSpec, targetLevel int) (ckks.Parameters, error) {
	literal, err := buildTargetResidualLiteral(spec, targetLevel)
	if err != nil {
		return ckks.Parameters{}, err
	}
	return ckks.NewParametersFromLiteral(literal)
}

func bkrIntInSlice(values []int, want int) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func bkrUint64PrefixEqual(owner, target []uint64) bool {
	if len(owner) < len(target) {
		return false
	}
	for i := range target {
		if owner[i] != target[i] {
			return false
		}
	}
	return true
}

func bkrA6SecretKeyPrefixCompatible(owner, target *rlwe.SecretKey) bool {
	if owner == nil || target == nil {
		return false
	}
	return bkrA6RingPolyPrefixEqual(owner.Value.Q, target.Value.Q) &&
		bkrA6RingPolyPrefixEqual(owner.Value.P, target.Value.P)
}

func bkrA6RingPolyPrefixEqual(owner, target ring.Poly) bool {
	if len(owner.Coeffs) < len(target.Coeffs) {
		return false
	}
	for i := range target.Coeffs {
		if !reflect.DeepEqual(owner.Coeffs[i], target.Coeffs[i]) {
			return false
		}
	}
	return true
}
```

The imports for `bootstrap_key_reuse_a6_test.go` must include:

```go
import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/tuneinsight/lattigo/v6/core/rlwe"
	"github.com/tuneinsight/lattigo/v6/ring"
	"github.com/tuneinsight/lattigo/v6/schemes/ckks"
)
```

- [ ] **Step 4: Run the tests to verify they pass**

Run:

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run 'TestBootstrapKeyReuseA6(SelectsMaxUsedOwner|RejectsNoHigherOrEqualOwner|CompatibilityPredicate|SecretKeyDomainCompatibility)' -count=1 -timeout=30m
```

Expected:

```text
ok  	github.com/tuneinsight/lattigo/v6/circuits/ckks/bootstrapping
```

- [ ] **Step 5: Commit**

```bash
git add circuits/ckks/bootstrapping/bootstrap_key_reuse_a6_test.go
git commit -m "Add A6 superset compatibility predicate"
```

## Task 3: Implement A6 Superset Drop Dispatcher

**Files:**

- Modify: `circuits/ckks/bootstrapping/bootstrap_key_reuse_a6_test.go`

- [ ] **Step 1: Write failing dispatcher tests**

Add:

```go
func TestBootstrapKeyReuseA6DropCiphertextAfterOwnerBootstrap(t *testing.T) {
	eval3 := bkrDummyEvaluatorForOutputLevel(t, 3)
	dispatcher, err := newBKRA6SupersetDropDispatcher(3, eval3)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := dispatcher.ownerLevelForTarget(1); err != nil || got != 3 {
		t.Fatalf("ownerLevelForTarget(1)=(%d,%v), want (3,nil)", got, err)
	}
	if _, err := newBKRA6SupersetDropDispatcher(1, bkrDummyEvaluatorForOutputLevel(t, 3)); err == nil {
		t.Fatal("expected owner/output mismatch to fail")
	}
}
```

This test verifies construction and routing before wiring real bootstrapping.

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run '^TestBootstrapKeyReuseA6DropCiphertextAfterOwnerBootstrap$' -count=1 -timeout=30m
```

Expected: compile failures for `newBKRA6SupersetDropDispatcher`.

- [ ] **Step 3: Implement dispatcher construction and routing**

Add:

```go
type bkrA6SupersetDropDispatcher struct {
	ownerLevel int
	ownerEval  *Evaluator
}

func newBKRA6SupersetDropDispatcher(ownerLevel int, ownerEval *Evaluator) (*bkrA6SupersetDropDispatcher, error) {
	if ownerEval == nil {
		return nil, errors.New("A6 owner evaluator is nil")
	}
	if ownerEval.OutputLevel() != ownerLevel {
		return nil, fmt.Errorf("A6 owner evaluator OutputLevel=%d expected=%d", ownerEval.OutputLevel(), ownerLevel)
	}
	return &bkrA6SupersetDropDispatcher{ownerLevel: ownerLevel, ownerEval: ownerEval}, nil
}

func (d *bkrA6SupersetDropDispatcher) ownerLevelForTarget(targetLevel int) (int, error) {
	if d == nil {
		return 0, errors.New("A6 dispatcher is nil")
	}
	if targetLevel > d.ownerLevel {
		return 0, fmt.Errorf("A6 target level %d exceeds owner level %d", targetLevel, d.ownerLevel)
	}
	return d.ownerLevel, nil
}

func (d *bkrA6SupersetDropDispatcher) bootstrapAtLevel(ct *rlwe.Ciphertext, targetLevel int) (*rlwe.Ciphertext, error) {
	if _, err := d.ownerLevelForTarget(targetLevel); err != nil {
		return nil, err
	}
	ctOut, err := d.ownerEval.Bootstrap(ct)
	if err != nil {
		return nil, err
	}
	if ctOut.Level() < targetLevel {
		return nil, fmt.Errorf("A6 owner bootstrap produced level %d below target %d", ctOut.Level(), targetLevel)
	}
	if err := bkrDropCiphertextToLevel(ctOut, targetLevel); err != nil {
		return nil, err
	}
	return ctOut, nil
}

func (d *bkrA6SupersetDropDispatcher) bootstrapManyAtLevel(cts []rlwe.Ciphertext, targetLevel int) ([]rlwe.Ciphertext, error) {
	if _, err := d.ownerLevelForTarget(targetLevel); err != nil {
		return nil, err
	}
	outs, err := d.ownerEval.BootstrapMany(cts)
	if err != nil {
		return nil, err
	}
	for i := range outs {
		if outs[i].Level() < targetLevel {
			return nil, fmt.Errorf("A6 owner bootstrap output[%d] produced level %d below target %d", i, outs[i].Level(), targetLevel)
		}
		if err := bkrDropCiphertextToLevel(&outs[i], targetLevel); err != nil {
			return nil, err
		}
	}
	return outs, nil
}
```

Add `github.com/tuneinsight/lattigo/v6/core/rlwe` to the imports.

- [ ] **Step 4: Run the dispatcher test to verify it passes**

Run:

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run '^TestBootstrapKeyReuseA6DropCiphertextAfterOwnerBootstrap$' -count=1 -timeout=30m
```

Expected:

```text
ok  	github.com/tuneinsight/lattigo/v6/circuits/ckks/bootstrapping
```

- [ ] **Step 5: Commit**

```bash
git add circuits/ckks/bootstrapping/bootstrap_key_reuse_a6_test.go
git commit -m "Add A6 superset DropLevel dispatcher"
```

## Task 4: Add A6 Runner and CSV Material View Rows

**Files:**

- Modify: `circuits/ckks/bootstrapping/bootstrap_key_reuse_a6_test.go`

- [ ] **Step 1: Write failing A6 CSV contract test**

Add:

```go
func TestBootstrapKeyReuseA6_P2MultiFastSupersetDropLevelContract(t *testing.T) {
	runDir := t.TempDir()
	bkrSetResultDirForTest(t, runDir)

	spec := bkrCaseA6P2MultiFastUsedSparse()
	if err := runBootstrapKeyReuseA6(t, spec); err != nil {
		t.Fatal(err)
	}

	bkrAssertCSVOnlyOutput(t, runDir)
	bkrAssertCSVTargetRowsForPlanForTest(t, runDir, spec.UsedTargetLevels, bkrLaneA6, bkrPlanA6)

	summaryRows := bkrReadCSVForTest(t, filepath.Join(runDir, "summary.csv"))
	if got := bkrCSVValueForTest(t, summaryRows, 1, "passed"); got != "true" {
		t.Fatalf("summary passed=%q, want true", got)
	}
	if got := bkrCSVValueForTest(t, summaryRows, 1, "rns_slice_success"); got != "disabled" {
		t.Fatalf("summary rns_slice_success=%q, want disabled", got)
	}

	targetRows := bkrReadCSVForTest(t, filepath.Join(runDir, "target_results.csv"))
	if got := bkrCSVIntForTest(t, targetRows, bkrCSVRowForTargetForTest(t, targetRows, 1), "output_level"); got != 1 {
		t.Fatalf("target 1 output_level=%d, want 1", got)
	}
	if got := bkrCSVIntForTest(t, targetRows, bkrCSVRowForTargetForTest(t, targetRows, 3), "output_level"); got != 3 {
		t.Fatalf("target 3 output_level=%d, want 3", got)
	}

	materialRows := bkrReadCSVForTest(t, filepath.Join(runDir, "material_metrics.csv"))
	consumerRow := bkrCSVRowForTargetForTest(t, materialRows, 1)
	if got := bkrCSVIntForTest(t, materialRows, consumerRow, "generated_rotation_key_count"); got != 0 {
		t.Fatalf("target 1 generated_rotation_key_count=%d, want 0 for A6 view", got)
	}
	if got := bkrCSVIntForTest(t, materialRows, consumerRow, "generated_encoded_diagonal_count"); got != 0 {
		t.Fatalf("target 1 generated_encoded_diagonal_count=%d, want 0 for A6 view", got)
	}
	if got := bkrCSVValueForTest(t, materialRows, consumerRow, "fallback_reason"); got != "superset_output_owner_level=3;drop_levels=2" {
		t.Fatalf("target 1 fallback_reason=%q", got)
	}
}
```

Add helper `bkrCSVRowForTargetForTest` if it does not already exist:

```go
func bkrCSVRowForTargetForTest(t *testing.T, rows [][]string, targetLevel int) int {
	t.Helper()
	for row := 1; row < len(rows); row++ {
		if bkrCSVIntForTest(t, rows, row, "target_level") == targetLevel {
			return row
		}
	}
	t.Fatalf("missing CSV row for target_level=%d", targetLevel)
	return -1
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run '^TestBootstrapKeyReuseA6_P2MultiFastSupersetDropLevelContract$' -count=1 -timeout=30m
```

Expected: compile failure for missing `runBootstrapKeyReuseA6`.

- [ ] **Step 3: Implement A6 owner material view**

Add A6 view material cloning:

```go
func bkrA6ViewMaterialFromOwner(spec bkrCaseSpec, targetLevel, ownerLevel int, owner *bkrA1PreparedTarget) (bkrMaterialMetrics, bkrMaterialBaselineDetails) {
	material := owner.material
	material.PlanID = bkrPlanA6
	material.TargetLevel = targetLevel
	material.GeneratedEvaluationKeyCount = 0
	material.GeneratedRotationKeyCount = 0
	material.GeneratedEncodedDiagonalCount = 0
	material.PersistentKeyBytes = 0
	material.PersistentMatrixBytes = 0
	material.SharedRotationKeys = owner.baseline.Index.GeneratedRotationKeyCount
	material.SharedEncodedDiagonals = owner.baseline.Index.EncodedDiagonalRecordCount
	material.RNSSliceSuccess = bkrA5DefaultRNSSliceStatus()
	material.FallbackReason = fmt.Sprintf("superset_output_owner_level=%d;drop_levels=%d", ownerLevel, ownerLevel-targetLevel)

	baseline := bkrA6CloneBaselineForView(owner.baseline, targetLevel)
	completeBootstrapKeyReuseBaselineIndex(&baseline, material)
	return material, baseline
}

func bkrA6CloneBaselineForView(owner bkrMaterialBaselineDetails, targetLevel int) bkrMaterialBaselineDetails {
	baseline := owner
	baseline.ParameterChain.PlanID = bkrPlanA6
	baseline.ParameterChain.TargetLevel = targetLevel
	baseline.GaloisKeys.PlanID = bkrPlanA6
	baseline.GaloisKeys.TargetLevel = targetLevel
	baseline.Index.PlanID = bkrPlanA6
	baseline.Index.TargetLevel = targetLevel
	baseline.Schedules = append([]bkrLinearTransformScheduleBaseline(nil), owner.Schedules...)
	for i := range baseline.Schedules {
		baseline.Schedules[i].PlanID = bkrPlanA6
		baseline.Schedules[i].TargetLevel = targetLevel
	}
	baseline.EncodedDiagonals = append([]bkrEncodedDiagonalBaseline(nil), owner.EncodedDiagonals...)
	for i := range baseline.EncodedDiagonals {
		baseline.EncodedDiagonals[i].PlanID = bkrPlanA6
		baseline.EncodedDiagonals[i].TargetLevel = targetLevel
	}
	return baseline
}
```

- [ ] **Step 4: Implement A6 runner**

Use the A5 preparation stack for the owner, and material view rows for lower targets:

```go
func runBootstrapKeyReuseA6(t *testing.T, spec bkrCaseSpec) error {
	t.Helper()

	if spec.LongOnly && !*flagLongTest {
		t.Skip("long A6 bootstrap key reuse case; rerun with -args -long")
	}

	rec, err := newBootstrapKeyReuseRecorder(t, spec, bkrA6RunConfig(spec))
	if err != nil {
		return err
	}
	defer rec.close()

	var runErr error
	successfulTargets := 0

	if err := rec.writeExperimentCase(); err != nil {
		runErr = errors.Join(runErr, err)
	}
	if err := bkrValidateA1TargetSelection(spec); err != nil {
		_ = rec.writeFailure(-1, "target_selection", err.Error(), true, false, false)
		runErr = errors.Join(runErr, err)
		summary, summaryErr := rec.writeSummary(successfulTargets)
		if summaryErr != nil {
			runErr = errors.Join(runErr, summaryErr)
		}
		return fmt.Errorf("A6 case %s failed; result_dir=%s: %w", spec.CaseID, summary.ResultDirectory, runErr)
	}

	ownerLevel, err := bkrA6OwnerLevel(spec)
	if err != nil {
		_ = rec.writeFailure(-1, "a6_owner_selection", err.Error(), true, false, false)
		runErr = errors.Join(runErr, err)
		summary, summaryErr := rec.writeSummary(successfulTargets)
		if summaryErr != nil {
			runErr = errors.Join(runErr, summaryErr)
		}
		return fmt.Errorf("A6 case %s failed; result_dir=%s: %w", spec.CaseID, summary.ResultDirectory, runErr)
	}

	rotationPool := newBKRRotationKeyPool(bkrPlanA6, spec)
	schedulePool := newBKRLinearTransformSchedulePool(bkrPlanA6, spec)
	diagonalPool := newBKREncodedDiagonalPool(bkrPlanA6, spec)

	owner, err := bkrPrepareA2TargetForPlan(t, rec, spec, ownerLevel, rotationPool, bkrPlanA6)
	if err != nil {
		runErr = errors.Join(runErr, err)
	} else {
		if _, err := bkrInternA3TargetSchedules(schedulePool, owner); err != nil {
			_ = rec.writeFailure(ownerLevel, "schedule_interning", err.Error(), false, true, false)
			runErr = errors.Join(runErr, err)
		}
		if err := bkrInternA4TargetEncodedDiagonals(diagonalPool, owner); err != nil {
			_ = rec.writeFailure(ownerLevel, "encoded_diagonal_interning", err.Error(), false, true, false)
			runErr = errors.Join(runErr, err)
		}
		owner.material.RNSSliceSuccess = bkrA5DefaultRNSSliceStatus()
	}

	rec.addRotationKeyPoolRecords(rotationPool.records())

	if owner != nil && runErr == nil {
		dispatcher, err := newBKRA6SupersetDropDispatcher(ownerLevel, owner.eval)
		if err != nil {
			_ = rec.writeFailure(-1, "a6_dispatcher_construction", err.Error(), false, true, false)
			runErr = errors.Join(runErr, err)
		} else {
			for _, targetLevel := range spec.UsedTargetLevels {
				if err := bkrValidateA6SupersetDrop(spec, ownerLevel, targetLevel); err != nil {
					_ = rec.writeFailure(targetLevel, "a6_superset_validation", err.Error(), true, false, false)
					runErr = errors.Join(runErr, err)
					continue
				}
				target := owner
				if targetLevel != ownerLevel {
					material, baseline := bkrA6ViewMaterialFromOwner(spec, targetLevel, ownerLevel, owner)
					target = &bkrA1PreparedTarget{
						targetLevel:         targetLevel,
						expectedOutputLevel: targetLevel,
						residualParams:      owner.residualParams,
						btpParams:           owner.btpParams,
						sk:                  owner.sk,
						keys:                owner.keys,
						eval:                owner.eval,
						baseline:            baseline,
						material:            material,
						rotationViewRecords: bkrA6RotationViewRecordsForTarget(owner, targetLevel, ownerLevel),
						keygenElapsed:       0,
						keygenPeak:          0,
						constructionElapsed: 0,
					}
				}
				if err := bkrRunA6BootstrapTarget(t, rec, spec, dispatcher, target); err != nil {
					runErr = errors.Join(runErr, err)
					continue
				}
				successfulTargets++
			}
		}
	}

	summary, summaryErr := rec.writeSummary(successfulTargets)
	if summaryErr != nil {
		runErr = errors.Join(runErr, summaryErr)
	}
	if runErr != nil {
		return fmt.Errorf("A6 case %s failed; result_dir=%s: %w", spec.CaseID, summary.ResultDirectory, runErr)
	}
	return nil
}
```

Implement `bkrRunA6BootstrapTarget` like `bkrRunA2BootstrapTargetForPlan`, but call the A6 dispatcher and validate against `target.expectedOutputLevel`:

```go
func bkrRunA6BootstrapTarget(t *testing.T, rec *bkrRecorder, spec bkrCaseSpec, dispatcher *bkrA6SupersetDropDispatcher, target *bkrA1PreparedTarget) error {
	t.Helper()

	runtimeSampler := newBKRHeapSampler()
	bootstrapStart := time.Now()
	outputs, wants, bootstrapErr := bkrBootstrapA6Ciphertexts(spec, target.residualParams, dispatcher, target.sk, target.targetLevel)
	bootstrapElapsed := time.Since(bootstrapStart)
	runtimePeak := runtimeSampler.stopAndMax()

	runtimeMetrics := bkrRuntimeMetrics{
		RecordType:                    "RuntimeMetrics",
		SchemaVersion:                 bkrSchemaVersion,
		PlanID:                        bkrPlanA6,
		CaseID:                        spec.CaseID,
		ParamsProfile:                 spec.ProfileID,
		TargetLevel:                   target.targetLevel,
		KeygenTimeMS:                  bkrDurationMS(target.keygenElapsed),
		EvaluatorMatrixConstructionMS: bkrDurationMS(target.constructionElapsed),
		BootstrapLatencyMS:            bkrDurationMS(bootstrapElapsed),
		PeakKeygenHeapBytes:           target.keygenPeak,
		PeakRuntimeHeapBytes:          runtimePeak,
	}
	rec.addRotationKeyViewRecords(target.rotationViewRecords)

	if bootstrapErr != nil {
		result := bkrEmptyTargetRunResult(bkrPlanA6, spec, target.targetLevel, target.expectedOutputLevel, target.residualParams, bootstrapErr.Error())
		if err := writeBootstrapKeyReuseResult(rec, result, target.material, runtimeMetrics, target.baseline); err != nil {
			return err
		}
		_ = rec.writeFailure(target.targetLevel, "bootstrap", bootstrapErr.Error(), false, true, true)
		return bootstrapErr
	}

	result, checkErr := bkrValidateBootstrapKeyReuseOutputs(t, bkrPlanA6, spec, target.targetLevel, target.expectedOutputLevel, target.residualParams, outputs, wants)
	if err := writeBootstrapKeyReuseResult(rec, result, target.material, runtimeMetrics, target.baseline); err != nil {
		return err
	}
	if checkErr != nil {
		_ = rec.writeFailure(target.targetLevel, "validate_output", checkErr.Error(), false, true, true)
		return checkErr
	}
	return nil
}
```

- [ ] **Step 5: Implement A6 ciphertext bootstrap helper**

Add:

```go
func bkrBootstrapA6Ciphertexts(spec bkrCaseSpec, params ckks.Parameters, dispatcher *bkrA6SupersetDropDispatcher, sk *rlwe.SecretKey, targetLevel int) ([]rlwe.Ciphertext, [][]complex128, error) {
	encoder := ckks.NewEncoder(params)
	encryptor := rlwe.NewEncryptor(params, sk)
	values := bkrComplexValues(params, fmt.Sprintf("%s/target/%d/values", spec.Seed, targetLevel))

	if !spec.PackedMode {
		plaintext := ckks.NewPlaintext(params, 0)
		if err := encoder.Encode(values, plaintext); err != nil {
			return nil, nil, err
		}
		ct, err := encryptor.EncryptNew(plaintext)
		if err != nil {
			return nil, nil, err
		}
		if ct.Level() != 0 {
			return nil, nil, fmt.Errorf("input ciphertext level=%d expected=0", ct.Level())
		}
		out, err := dispatcher.bootstrapAtLevel(ct, targetLevel)
		if err != nil {
			return nil, nil, err
		}
		return []rlwe.Ciphertext{*out}, [][]complex128{values}, nil
	}

	logSlots := params.LogMaxSlots()
	if maxLogSlots := dispatcher.ownerEval.LogMaxSlots(); maxLogSlots < logSlots {
		logSlots = maxLogSlots
	}
	values = values[:1<<logSlots]

	plaintext := ckks.NewPlaintext(params, 0)
	plaintext.LogDimensions = ring.Dimensions{Rows: 0, Cols: logSlots}

	inputs := make([]rlwe.Ciphertext, bkrPackedCiphertext)
	wants := make([][]complex128, bkrPackedCiphertext)
	for i := range inputs {
		rotated := utils.RotateSlice(values, i)
		if err := encoder.Encode(rotated, plaintext); err != nil {
			return nil, nil, err
		}
		ct, err := encryptor.EncryptNew(plaintext)
		if err != nil {
			return nil, nil, err
		}
		if ct.Level() != 0 {
			return nil, nil, fmt.Errorf("packed input ciphertext[%d] level=%d expected=0", i, ct.Level())
		}
		inputs[i] = *ct
		wants[i] = rotated
	}
	outputs, err := dispatcher.bootstrapManyAtLevel(inputs, targetLevel)
	if err != nil {
		return nil, nil, err
	}
	return outputs, wants, nil
}
```

This helper requires imports:

```go
	"path/filepath"
	"time"

	"github.com/tuneinsight/lattigo/v6/ring"
	"github.com/tuneinsight/lattigo/v6/utils"
```

- [ ] **Step 6: Implement A6 rotation view records**

Add:

```go
func bkrA6RotationViewRecordsForTarget(owner *bkrA1PreparedTarget, targetLevel, ownerLevel int) []bkrRotationKeyViewRecord {
	records := make([]bkrRotationKeyViewRecord, 0, len(owner.rotationViewRecords))
	for _, record := range owner.rotationViewRecords {
		view := record
		view.PlanID = bkrPlanA6
		view.TargetLevel = targetLevel
		view.OwnerTargetLevel = ownerLevel
		view.IsShared = targetLevel != ownerLevel
		view.DomainCompatible = true
		view.FallbackReason = "none"
		records = append(records, view)
	}
	return records
}
```

- [ ] **Step 7: Run the A6 CSV contract test**

Run:

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run '^TestBootstrapKeyReuseA6_P2MultiFastSupersetDropLevelContract$' -count=1 -timeout=30m
```

Expected:

```text
ok  	github.com/tuneinsight/lattigo/v6/circuits/ckks/bootstrapping
```

- [ ] **Step 8: Commit**

```bash
git add circuits/ckks/bootstrapping/bootstrap_key_reuse_a6_test.go
git commit -m "Add A6 superset output DropLevel runner"
```

## Task 5: Compare A6 With A0 Direct BK(r)

**Files:**

- Modify: `circuits/ckks/bootstrapping/bootstrap_key_reuse_a6_test.go`

- [ ] **Step 1: Write failing A0 comparison test**

Add:

```go
func TestBootstrapKeyReuseA6_MatchesA0DirectTargets(t *testing.T) {
	a0Dir := t.TempDir()
	bkrSetResultDirForTest(t, a0Dir)
	if err := runBootstrapKeyReuseA0(t, bkrCaseP2MultiFastClustered()); err != nil {
		t.Fatal(err)
	}

	a6Dir := t.TempDir()
	bkrSetResultDirForTest(t, a6Dir)
	a6Spec := bkrCaseA6P2MultiFastUsedSparse()
	if err := runBootstrapKeyReuseA6(t, a6Spec); err != nil {
		t.Fatal(err)
	}

	a0Targets := bkrReadCSVForTest(t, filepath.Join(a0Dir, "target_results.csv"))
	a6Targets := bkrReadCSVForTest(t, filepath.Join(a6Dir, "target_results.csv"))
	a0Runtime := bkrReadCSVForTest(t, filepath.Join(a0Dir, "runtime_metrics.csv"))
	a6Runtime := bkrReadCSVForTest(t, filepath.Join(a6Dir, "runtime_metrics.csv"))

	for _, targetLevel := range []int{1, 3} {
		for _, column := range []string{
			"output_level",
			"expected_output_level",
			"output_level_equality",
			"output_scale_equality",
			"bootstrap_error_status",
			"packed_ciphertexts",
		} {
			got := bkrCSVValueForTargetForTest(t, a6Targets, targetLevel, column)
			want := bkrCSVValueForTargetForTest(t, a0Targets, targetLevel, column)
			if got != want {
				t.Fatalf("target %d %s: A6=%q A0=%q", targetLevel, column, got, want)
			}
		}

		if got := bkrCSVFloatForTargetForTest(t, a6Targets, targetLevel, "average_log2_precision_real"); got < bkrMinPrecisionBits {
			t.Fatalf("target %d A6 real precision=%f below %f", targetLevel, got, bkrMinPrecisionBits)
		}
		if got := bkrCSVFloatForTargetForTest(t, a6Targets, targetLevel, "average_log2_precision_imag"); got < bkrMinPrecisionBits {
			t.Fatalf("target %d A6 imag precision=%f below %f", targetLevel, got, bkrMinPrecisionBits)
		}
		if got := bkrCSVFloatForTargetForTest(t, a0Runtime, targetLevel, "bootstrap_latency_ms"); got <= 0 {
			t.Fatalf("target %d A0 bootstrap_latency_ms=%f, want > 0", targetLevel, got)
		}
		if got := bkrCSVFloatForTargetForTest(t, a6Runtime, targetLevel, "bootstrap_latency_ms"); got <= 0 {
			t.Fatalf("target %d A6 bootstrap_latency_ms=%f, want > 0", targetLevel, got)
		}
	}
}
```

Add helper if missing:

```go
func bkrCSVFloatForTargetForTest(t *testing.T, rows [][]string, targetLevel int, column string) float64 {
	t.Helper()
	row := bkrCSVRowForTargetForTest(t, rows, targetLevel)
	value, err := strconv.ParseFloat(bkrCSVValueForTest(t, rows, row, column), 64)
	if err != nil {
		t.Fatalf("cannot parse %s for target %d: %v", column, targetLevel, err)
	}
	return value
}
```

Add `strconv` to the imports if it is not already present.

- [ ] **Step 2: Run the test to verify it fails or exposes implementation gaps**

Run:

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run '^TestBootstrapKeyReuseA6_MatchesA0DirectTargets$' -count=1 -timeout=30m
```

Expected before Task 4 is complete: compile or runtime failure. Expected after Task 4: pass.

- [ ] **Step 3: Fix any A6 output mismatch without weakening validation**

Allowed fixes:

- use the owner residual params consistently for encryption and decryption when using owner material;
- set `expectedOutputLevel` to the requested target level for consumer view rows;
- call `bkrDropCiphertextToLevel` after owner bootstrap and before validation;
- keep scale unchanged across `DropLevel`;
- preserve `bkrMinPrecisionBits`.

Forbidden fixes:

- lowering `bkrMinPrecisionBits`;
- changing A0 output expectations;
- comparing only target 3 and skipping target 1;
- changing `targetLevelBootstrapper` so A1-A5 silently accept unconfigured target levels;
- rescaling after DropLevel to mask scale mismatch.

- [ ] **Step 4: Run the A0 comparison test**

Run:

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run '^TestBootstrapKeyReuseA6_MatchesA0DirectTargets$' -count=1 -timeout=30m
```

Expected:

```text
ok  	github.com/tuneinsight/lattigo/v6/circuits/ckks/bootstrapping
```

- [ ] **Step 5: Commit**

```bash
git add circuits/ckks/bootstrapping/bootstrap_key_reuse_a6_test.go
git commit -m "Compare A6 DropLevel output with A0"
```

## Task 6: Prove A1-A5 Did Not Drift

**Files:**

- Test only.

- [ ] **Step 1: Run A6 focused tests**

Run:

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run 'TestBootstrapKeyReuseA6' -count=1 -timeout=30m
```

Expected:

```text
ok  	github.com/tuneinsight/lattigo/v6/circuits/ckks/bootstrapping
```

- [ ] **Step 2: Run A0-A6 regression**

Run:

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run 'TestBootstrapKeyReuse(A0_CSVOnlyOutputContract|A1_CSVOnlyUsedTargetContract|A1_MatchesA0UsedTargets|A2|A3|A4|A5|A6)' -count=1 -timeout=30m
```

Expected:

```text
ok  	github.com/tuneinsight/lattigo/v6/circuits/ckks/bootstrapping
```

- [ ] **Step 3: Run CSV/toolchain artifact check**

Run:

```bash
git diff --check
```

Expected: no output and exit code 0.

- [ ] **Step 4: Commit final verification note if docs changed during execution**

If implementation required updates to this plan or docs:

```bash
git add docs/bootstrap_key_reuse/a6_superset_output_drop_level_plan.md docs/README.md
git commit -m "Document A6 DropLevel verification"
```

## Completion Audit Checklist

Before claiming A6 is factually 100% complete, inspect current evidence for each item:

- `bootstrap_key_reuse_common_test.go` contains `bkrLaneA6`, `bkrPlanA6`, and `bkrA6RunConfig`.
- `bootstrap_key_reuse_a6_test.go` exists and contains:
  - `runBootstrapKeyReuseA6`;
  - `bkrCaseA6P2MultiFastUsedSparse`;
  - owner selection tests;
  - compatibility predicate tests;
  - dispatcher tests;
  - CSV contract test;
  - A0 direct comparison test.
- `TestBootstrapKeyReuseA6_P2MultiFastSupersetDropLevelContract` proves:
  - lane and plan ID are A6;
  - `summary.csv` passes;
  - `rns_slice_success=disabled`;
  - target 1 is served by owner level 3 and dropped to output level 1;
  - target 3 remains output level 3;
  - target 1 material row is a view row, not newly generated owner material.
- `TestBootstrapKeyReuseA6_MatchesA0DirectTargets` proves:
  - A6 target 1 and 3 level/scale/status match A0;
  - A6 decoded precision is at or above the threshold;
  - A0 and A6 latency rows are both recorded.
- A0-A5 tests still pass under the A0-A6 regression command.
- `git diff --check` exits 0.
- The worktree contains no unintended unrelated edits.

Only after every checklist item is backed by current file contents and fresh command output may A6 be called factually 100% complete.

## Non-Goals

- Do not enable A6 as a production default path.
- Do not change public Lattigo APIs.
- Do not change the A0-A6 CSV schema.
- Do not use `go test -json`, shell log redirects, Python scripts, or third-party benchmark tools as standard evidence.
- Do not weaken precision, scale, or output-level validation to make A6 pass.
- Do not alter A1-A5 dispatcher behavior.
