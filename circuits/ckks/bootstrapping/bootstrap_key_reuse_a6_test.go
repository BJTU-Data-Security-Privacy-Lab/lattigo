package bootstrapping

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/tuneinsight/lattigo/v6/core/rlwe"
	"github.com/tuneinsight/lattigo/v6/ring"
	"github.com/tuneinsight/lattigo/v6/schemes/ckks"
	"github.com/tuneinsight/lattigo/v6/utils"
)

func TestBootstrapKeyReuseA6PlanIDContract(t *testing.T) {
	if bkrLaneA6 != "a6" {
		t.Fatalf("bkrLaneA6=%q, want a6", bkrLaneA6)
	}
	if bkrPlanA6 != "A6_SupersetOutputDropLevelView" {
		t.Fatalf("bkrPlanA6=%q", bkrPlanA6)
	}
}

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

func bkrCaseA6P2MultiFastUsedSparse() bkrCaseSpec {
	return bkrP2MultiFastBase("a6_p2_multi_fast_used_sparse", []int{1, 2, 3}, func(spec *bkrCaseSpec) {
		spec.UsedTargetLevels = []int{1, 3}
	})
}

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
					material, baseline := bkrA6ViewMaterialFromOwner(targetLevel, ownerLevel, owner)
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

func bkrA6ViewMaterialFromOwner(targetLevel, ownerLevel int, owner *bkrA1PreparedTarget) (bkrMaterialMetrics, bkrMaterialBaselineDetails) {
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
	if spec.LogSlots > 0 && spec.LogSlots > ownerParams.LogMaxSlots() {
		return fmt.Errorf("A6 LogSlots=%d exceeds owner LogMaxSlots=%d", spec.LogSlots, ownerParams.LogMaxSlots())
	}
	if ownerParams.DefaultScale().Cmp(targetParams.DefaultScale()) != 0 {
		return fmt.Errorf("A6 owner scale log2=%f target scale log2=%f", ownerParams.DefaultScale().Log2(), targetParams.DefaultScale().Log2())
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

func bkrCSVFloatForTargetForTest(t *testing.T, rows [][]string, targetLevel int, column string) float64 {
	t.Helper()
	row := bkrCSVRowForTargetForTest(t, rows, targetLevel)
	value, err := strconv.ParseFloat(bkrCSVValueForTest(t, rows, row, column), 64)
	if err != nil {
		t.Fatalf("cannot parse %s for target %d: %v", column, targetLevel, err)
	}
	return value
}
