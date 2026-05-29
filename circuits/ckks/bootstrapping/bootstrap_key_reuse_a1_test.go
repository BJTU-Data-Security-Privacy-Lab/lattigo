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

type bkrA1PreparedTarget struct {
	targetLevel         int
	expectedOutputLevel int
	residualParams      ckks.Parameters
	btpParams           Parameters
	sk                  *rlwe.SecretKey
	keys                *EvaluationKeys
	eval                *Evaluator
	baseline            bkrMaterialBaselineDetails
	material            bkrMaterialMetrics
	rotationPoolRecords []bkrRotationKeyPoolRecord
	rotationViewRecords []bkrRotationKeyViewRecord
	keygenElapsed       time.Duration
	keygenPeak          uint64
	constructionElapsed time.Duration
}

func runBootstrapKeyReuseA1(t *testing.T, spec bkrCaseSpec) error {
	t.Helper()

	if spec.LongOnly && !*flagLongTest {
		t.Skip("long A1 bootstrap key reuse case; rerun with -args -long")
	}

	rec, err := newBootstrapKeyReuseRecorder(t, spec, bkrA1RunConfig(spec))
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
		return fmt.Errorf("A1 case %s failed; result_dir=%s: %w", spec.CaseID, summary.ResultDirectory, runErr)
	}

	prepared := make(map[int]*bkrA1PreparedTarget, len(spec.UsedTargetLevels))
	evaluators := make(map[int]*Evaluator, len(spec.UsedTargetLevels))

	for _, targetLevel := range spec.UsedTargetLevels {
		target, err := bkrPrepareA1Target(t, rec, spec, targetLevel)
		if err != nil {
			runErr = errors.Join(runErr, err)
			continue
		}
		prepared[targetLevel] = target
		evaluators[targetLevel] = target.eval
	}

	if len(evaluators) > 0 {
		dispatcher, err := newTargetLevelBootstrapper(evaluators)
		if err != nil {
			_ = rec.writeFailure(-1, "dispatcher_construction", err.Error(), false, true, false)
			runErr = errors.Join(runErr, err)
		} else {
			for _, targetLevel := range spec.UsedTargetLevels {
				target := prepared[targetLevel]
				if target == nil {
					continue
				}
				if err := bkrRunA1BootstrapTarget(t, rec, spec, dispatcher, target); err != nil {
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
		return fmt.Errorf("A1 case %s failed; result_dir=%s: %w", spec.CaseID, summary.ResultDirectory, runErr)
	}

	return nil
}

func bkrValidateA1TargetSelection(spec bkrCaseSpec) error {
	if len(spec.UsedTargetLevels) == 0 {
		return errors.New("UsedTargetLevels must not be empty for A1")
	}

	fullParams, err := ckks.NewParametersFromLiteral(spec.SchemeParams)
	if err != nil {
		return fmt.Errorf("cannot instantiate full profile parameters: %w", err)
	}

	all := make(map[int]struct{}, len(spec.AllTargetLevels))
	for _, targetLevel := range spec.AllTargetLevels {
		if _, ok := all[targetLevel]; ok {
			return fmt.Errorf("AllTargetLevels contains duplicate target level %d", targetLevel)
		}
		if targetLevel < 0 || targetLevel > fullParams.MaxLevel() {
			return fmt.Errorf("AllTargetLevels contains invalid target level %d for profile max level %d", targetLevel, fullParams.MaxLevel())
		}
		all[targetLevel] = struct{}{}
	}

	used := make(map[int]struct{}, len(spec.UsedTargetLevels))
	for _, targetLevel := range spec.UsedTargetLevels {
		if _, ok := used[targetLevel]; ok {
			return fmt.Errorf("UsedTargetLevels contains duplicate target level %d", targetLevel)
		}
		if targetLevel < 0 || targetLevel > fullParams.MaxLevel() {
			return fmt.Errorf("UsedTargetLevels contains invalid target level %d for profile max level %d", targetLevel, fullParams.MaxLevel())
		}
		if _, ok := all[targetLevel]; !ok {
			return fmt.Errorf("UsedTargetLevels contains target level %d that is not present in AllTargetLevels", targetLevel)
		}
		used[targetLevel] = struct{}{}
	}

	return nil
}

func bkrPrepareA1Target(t *testing.T, rec *bkrRecorder, spec bkrCaseSpec, targetLevel int) (*bkrA1PreparedTarget, error) {
	t.Helper()

	expectedOutputLevel := targetLevel
	if spec.ExpectedOutputLevelOverride != nil {
		expectedOutputLevel = *spec.ExpectedOutputLevelOverride
	}

	residualParams, btpParams, err := buildTargetBootstrappingParameters(spec, targetLevel)
	if err != nil {
		_ = rec.writeFailure(targetLevel, "build_parameters", err.Error(), true, false, false)
		return nil, err
	}

	if have := btpParams.ResidualParameters.MaxLevel(); have != expectedOutputLevel {
		err := fmt.Errorf("pre-keygen output level check failed: residual max level=%d expected=%d", have, expectedOutputLevel)
		_ = rec.writeFailure(targetLevel, "output_level_pre_keygen", err.Error(), true, false, false)
		return nil, err
	}

	sk, err := newDeterministicSecretKey(residualParams, spec.Seed+"/secret-key")
	if err != nil {
		_ = rec.writeFailure(targetLevel, "secret_key", err.Error(), true, false, false)
		return nil, err
	}

	var keys *EvaluationKeys
	keygenElapsed, keygenPeak, err := bkrMeasurePhase(func() error {
		var keygenErr error
		keys, _, keygenErr = btpParams.GenEvaluationKeys(sk)
		return keygenErr
	})
	if err != nil {
		_ = rec.writeFailure(targetLevel, "keygen", err.Error(), false, false, false)
		return nil, err
	}

	constructionStart := time.Now()
	eval, err := NewEvaluator(btpParams, keys)
	constructionElapsed := time.Since(constructionStart)
	if err != nil {
		_ = rec.writeFailure(targetLevel, "evaluator_construction", err.Error(), false, true, false)
		return nil, err
	}

	if have := eval.OutputLevel(); have != expectedOutputLevel {
		err := fmt.Errorf("evaluator OutputLevel=%d expected=%d", have, expectedOutputLevel)
		_ = rec.writeFailure(targetLevel, "output_level_after_evaluator", err.Error(), false, true, false)
		return nil, err
	}

	baseline, err := collectBootstrapKeyReuseBaselineDetails(bkrPlanA1, spec, targetLevel, btpParams, keys, eval)
	if err != nil {
		_ = rec.writeFailure(targetLevel, "baseline_details", err.Error(), false, true, false)
		return nil, err
	}

	material := collectBootstrapKeyReuseMetrics(bkrPlanA1, spec, targetLevel, keys, eval, baseline)
	completeBootstrapKeyReuseBaselineIndex(&baseline, material)
	rotationPoolRecords, rotationViewRecords := bkrDefaultRotationKeyRecords(bkrPlanA1, spec, targetLevel, btpParams, keys)

	return &bkrA1PreparedTarget{
		targetLevel:         targetLevel,
		expectedOutputLevel: expectedOutputLevel,
		residualParams:      residualParams,
		btpParams:           btpParams,
		sk:                  sk,
		keys:                keys,
		eval:                eval,
		baseline:            baseline,
		material:            material,
		rotationPoolRecords: rotationPoolRecords,
		rotationViewRecords: rotationViewRecords,
		keygenElapsed:       keygenElapsed,
		keygenPeak:          keygenPeak,
		constructionElapsed: constructionElapsed,
	}, nil
}

func bkrRunA1BootstrapTarget(t *testing.T, rec *bkrRecorder, spec bkrCaseSpec, dispatcher *targetLevelBootstrapper, target *bkrA1PreparedTarget) error {
	t.Helper()

	runtimeSampler := newBKRHeapSampler()
	bootstrapStart := time.Now()
	outputs, wants, bootstrapErr := bkrBootstrapA1Ciphertexts(spec, target.residualParams, dispatcher, target.sk, target.targetLevel)
	bootstrapElapsed := time.Since(bootstrapStart)
	runtimePeak := runtimeSampler.stopAndMax()

	runtimeMetrics := bkrRuntimeMetrics{
		RecordType:                    "RuntimeMetrics",
		SchemaVersion:                 bkrSchemaVersion,
		PlanID:                        bkrPlanA1,
		CaseID:                        spec.CaseID,
		ParamsProfile:                 spec.ProfileID,
		TargetLevel:                   target.targetLevel,
		KeygenTimeMS:                  bkrDurationMS(target.keygenElapsed),
		EvaluatorMatrixConstructionMS: bkrDurationMS(target.constructionElapsed),
		BootstrapLatencyMS:            bkrDurationMS(bootstrapElapsed),
		PeakKeygenHeapBytes:           target.keygenPeak,
		PeakRuntimeHeapBytes:          runtimePeak,
	}
	rec.addRotationKeyPoolRecords(target.rotationPoolRecords)
	rec.addRotationKeyViewRecords(target.rotationViewRecords)

	if bootstrapErr != nil {
		result := bkrEmptyTargetRunResult(bkrPlanA1, spec, target.targetLevel, target.expectedOutputLevel, target.residualParams, bootstrapErr.Error())
		if err := writeBootstrapKeyReuseResult(rec, result, target.material, runtimeMetrics, target.baseline); err != nil {
			return err
		}
		_ = rec.writeFailure(target.targetLevel, "bootstrap", bootstrapErr.Error(), false, true, true)
		return bootstrapErr
	}

	result, checkErr := bkrValidateBootstrapKeyReuseOutputs(t, bkrPlanA1, spec, target.targetLevel, target.expectedOutputLevel, target.residualParams, outputs, wants)
	if err := writeBootstrapKeyReuseResult(rec, result, target.material, runtimeMetrics, target.baseline); err != nil {
		return err
	}
	if checkErr != nil {
		_ = rec.writeFailure(target.targetLevel, "validate_output", checkErr.Error(), false, true, true)
		return checkErr
	}

	return nil
}

func bkrBootstrapA1Ciphertexts(spec bkrCaseSpec, params ckks.Parameters, dispatcher *targetLevelBootstrapper, sk *rlwe.SecretKey, targetLevel int) ([]rlwe.Ciphertext, [][]complex128, error) {
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
	eval, err := dispatcher.evaluatorAtLevel(targetLevel)
	if err != nil {
		return nil, nil, err
	}
	if maxLogSlots := eval.LogMaxSlots(); maxLogSlots < logSlots {
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

func runBootstrapKeyReuseA1ChainedTargetSwitch(t *testing.T, spec bkrCaseSpec, stageTargetLevels []int) error {
	t.Helper()

	rec, err := newBootstrapKeyReuseRecorder(t, spec, bkrA1RunConfig(spec))
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
		return fmt.Errorf("A1 chained case %s failed; result_dir=%s: %w", spec.CaseID, summary.ResultDirectory, runErr)
	}

	prepared := make(map[int]*bkrA1PreparedTarget, len(spec.UsedTargetLevels))
	evaluators := make(map[int]*Evaluator, len(spec.UsedTargetLevels))
	for _, targetLevel := range spec.UsedTargetLevels {
		target, err := bkrPrepareA1Target(t, rec, spec, targetLevel)
		if err != nil {
			runErr = errors.Join(runErr, err)
			continue
		}
		prepared[targetLevel] = target
		evaluators[targetLevel] = target.eval
	}

	dispatcher, err := newTargetLevelBootstrapper(evaluators)
	if err != nil {
		_ = rec.writeFailure(-1, "dispatcher_construction", err.Error(), false, true, false)
		runErr = errors.Join(runErr, err)
	} else {
		successfulTargets, err = bkrRunA1ChainedTargetSwitch(t, rec, spec, dispatcher, prepared, stageTargetLevels)
		if err != nil {
			runErr = errors.Join(runErr, err)
		}
	}

	summary, summaryErr := rec.writeSummary(successfulTargets)
	if summaryErr != nil {
		runErr = errors.Join(runErr, summaryErr)
	}

	if runErr != nil {
		return fmt.Errorf("A1 chained case %s failed; result_dir=%s: %w", spec.CaseID, summary.ResultDirectory, runErr)
	}

	return nil
}

func bkrRunA1ChainedTargetSwitch(t *testing.T, rec *bkrRecorder, spec bkrCaseSpec, dispatcher *targetLevelBootstrapper, prepared map[int]*bkrA1PreparedTarget, stageTargetLevels []int) (int, error) {
	t.Helper()

	if !reflect.DeepEqual(stageTargetLevels, []int{5, 8, 9}) {
		return 0, fmt.Errorf("unexpected chained target sequence %v, want [5 8 9]", stageTargetLevels)
	}

	initialTarget := prepared[9]
	if initialTarget == nil {
		return 0, errors.New("missing target 9 material for chained initial encryption")
	}

	ct, values, err := bkrA1InitialCiphertextAtLevel(spec, initialTarget, 2)
	if err != nil {
		_ = rec.writeFailure(-1, "chain_initial_modswitch_to_level_2", err.Error(), true, false, false)
		return 0, err
	}

	var runErr error
	successfulTargets := 0

	for i, targetLevel := range stageTargetLevels {
		target := prepared[targetLevel]
		if target == nil {
			err := fmt.Errorf("missing prepared material for chained target level %d", targetLevel)
			_ = rec.writeFailure(targetLevel, "chain_missing_target_material", err.Error(), true, false, false)
			runErr = errors.Join(runErr, err)
			continue
		}

		result, runtimeMetrics, ctOut, err := bkrRunA1ChainedBootstrapStage(t, spec, dispatcher, target, ct, values)
		rec.addRotationKeyPoolRecords(target.rotationPoolRecords)
		rec.addRotationKeyViewRecords(target.rotationViewRecords)
		if writeErr := writeBootstrapKeyReuseResult(rec, result, target.material, runtimeMetrics, target.baseline); writeErr != nil {
			runErr = errors.Join(runErr, writeErr)
		}
		if err != nil {
			_ = rec.writeFailure(targetLevel, "chain_bootstrap_to_target", err.Error(), false, true, true)
			runErr = errors.Join(runErr, err)
			continue
		}

		successfulTargets++
		ct = ctOut

		switch {
		case targetLevel == 5:
			if err := bkrDropCiphertextToLevel(ct, 1); err != nil {
				_ = rec.writeFailure(targetLevel, "chain_drop_after_target_5_to_level_1", err.Error(), false, true, true)
				runErr = errors.Join(runErr, err)
			}
		case targetLevel == 8:
			if err := bkrDropCiphertextToLevel(ct, 1); err != nil {
				_ = rec.writeFailure(targetLevel, "chain_drop_after_target_8_to_level_1", err.Error(), false, true, true)
				runErr = errors.Join(runErr, err)
			}
		case i != len(stageTargetLevels)-1:
			err := fmt.Errorf("no drop rule for non-terminal chained target level %d", targetLevel)
			_ = rec.writeFailure(targetLevel, "chain_missing_drop_rule", err.Error(), false, true, true)
			runErr = errors.Join(runErr, err)
		}
	}

	return successfulTargets, runErr
}

func bkrA1InitialCiphertextAtLevel(spec bkrCaseSpec, target *bkrA1PreparedTarget, initialLevel int) (*rlwe.Ciphertext, []complex128, error) {
	params := target.residualParams
	encoder := ckks.NewEncoder(params)
	encryptor := rlwe.NewEncryptor(params, target.sk)
	values := bkrComplexValues(params, fmt.Sprintf("%s/chained/values", spec.Seed))

	plaintext := ckks.NewPlaintext(params, params.MaxLevel())
	if err := encoder.Encode(values, plaintext); err != nil {
		return nil, nil, err
	}

	ct, err := encryptor.EncryptNew(plaintext)
	if err != nil {
		return nil, nil, err
	}
	if ct.Level() != params.MaxLevel() {
		return nil, nil, fmt.Errorf("initial ciphertext level=%d expected=%d", ct.Level(), params.MaxLevel())
	}
	if err := bkrDropCiphertextToLevel(ct, initialLevel); err != nil {
		return nil, nil, err
	}
	if ct.Level() != initialLevel {
		return nil, nil, fmt.Errorf("initial ModSwitch level=%d expected=%d", ct.Level(), initialLevel)
	}

	return ct, values, nil
}

func bkrRunA1ChainedBootstrapStage(t *testing.T, spec bkrCaseSpec, dispatcher *targetLevelBootstrapper, target *bkrA1PreparedTarget, ctIn *rlwe.Ciphertext, values []complex128) (bkrTargetRunResult, bkrRuntimeMetrics, *rlwe.Ciphertext, error) {
	t.Helper()

	inputLevel := ctIn.Level()
	runtimeSampler := newBKRHeapSampler()
	bootstrapStart := time.Now()
	ctOut, bootstrapErr := dispatcher.bootstrapAtLevel(ctIn, target.targetLevel)
	bootstrapElapsed := time.Since(bootstrapStart)
	runtimePeak := runtimeSampler.stopAndMax()

	runtimeMetrics := bkrRuntimeMetrics{
		RecordType:                    "RuntimeMetrics",
		SchemaVersion:                 bkrSchemaVersion,
		PlanID:                        bkrPlanA1,
		CaseID:                        spec.CaseID,
		ParamsProfile:                 spec.ProfileID,
		TargetLevel:                   target.targetLevel,
		KeygenTimeMS:                  bkrDurationMS(target.keygenElapsed),
		EvaluatorMatrixConstructionMS: bkrDurationMS(target.constructionElapsed),
		BootstrapLatencyMS:            bkrDurationMS(bootstrapElapsed),
		PeakKeygenHeapBytes:           target.keygenPeak,
		PeakRuntimeHeapBytes:          runtimePeak,
	}

	if bootstrapErr != nil {
		result := bkrEmptyTargetRunResult(bkrPlanA1, spec, target.targetLevel, target.expectedOutputLevel, target.residualParams, bootstrapErr.Error())
		return result, runtimeMetrics, nil, bootstrapErr
	}
	if ctOut.Level() != target.targetLevel {
		err := fmt.Errorf("chain bootstrap input level=%d target=%d produced level=%d", inputLevel, target.targetLevel, ctOut.Level())
		result := bkrEmptyTargetRunResult(bkrPlanA1, spec, target.targetLevel, target.expectedOutputLevel, target.residualParams, err.Error())
		return result, runtimeMetrics, ctOut, err
	}

	result, checkErr := bkrValidateBootstrapKeyReuseOutputs(t, bkrPlanA1, spec, target.targetLevel, target.expectedOutputLevel, target.residualParams, []rlwe.Ciphertext{*ctOut}, [][]complex128{values})
	if checkErr != nil {
		checkErr = fmt.Errorf("chain bootstrap input level=%d target=%d validation failed: %w", inputLevel, target.targetLevel, checkErr)
	}
	return result, runtimeMetrics, ctOut, checkErr
}

func bkrDropCiphertextToLevel(ct *rlwe.Ciphertext, level int) error {
	if ct == nil {
		return errors.New("cannot drop nil ciphertext")
	}
	if level < 0 {
		return fmt.Errorf("cannot drop ciphertext to negative level %d", level)
	}
	if level > ct.Level() {
		return fmt.Errorf("cannot drop ciphertext from level %d to higher level %d", ct.Level(), level)
	}
	ct.Resize(ct.Degree(), level)
	return nil
}

func bkrRunA1Test(t *testing.T, spec bkrCaseSpec) {
	t.Helper()
	if err := runBootstrapKeyReuseA1(t, spec); err != nil {
		t.Fatal(err)
	}
}

func bkrRunA1ExpectedFailure(t *testing.T, spec bkrCaseSpec) {
	t.Helper()
	if *bkrResultDir == "" {
		bkrSetResultDirForTest(t, t.TempDir())
	}
	if err := runBootstrapKeyReuseA1(t, spec); err == nil {
		t.Fatalf("expected A1 case %s to fail", spec.CaseID)
	}
}

func TestTargetLevelBootstrapperSelectsConfiguredEvaluator(t *testing.T) {
	eval1 := bkrDummyEvaluatorForOutputLevel(t, 1)
	eval3 := bkrDummyEvaluatorForOutputLevel(t, 3)

	dispatcher, err := newTargetLevelBootstrapper(map[int]*Evaluator{3: eval3, 1: eval1})
	if err != nil {
		t.Fatal(err)
	}

	levels := dispatcher.targetLevels()
	if !reflect.DeepEqual(levels, []int{1, 3}) {
		t.Fatalf("targetLevels=%v, want [1 3]", levels)
	}

	levels[0] = 99
	if got := dispatcher.targetLevels(); !reflect.DeepEqual(got, []int{1, 3}) {
		t.Fatalf("targetLevels returned non-copy slice: %v", got)
	}

	outputLevel, err := dispatcher.outputLevel(3)
	if err != nil {
		t.Fatal(err)
	}
	if outputLevel != 3 {
		t.Fatalf("outputLevel=%d, want 3", outputLevel)
	}
}

func TestTargetLevelBootstrapperRejectsInvalidConstruction(t *testing.T) {
	if _, err := newTargetLevelBootstrapper(nil); err == nil {
		t.Fatal("expected empty evaluator map to fail")
	}

	if _, err := newTargetLevelBootstrapper(map[int]*Evaluator{1: nil}); err == nil {
		t.Fatal("expected nil evaluator to fail")
	}

	if _, err := newTargetLevelBootstrapper(map[int]*Evaluator{2: bkrDummyEvaluatorForOutputLevel(t, 1)}); err == nil {
		t.Fatal("expected output-level mismatch to fail")
	}
}

func TestTargetLevelBootstrapperRejectsUnconfiguredTargetLevel(t *testing.T) {
	dispatcher, err := newTargetLevelBootstrapper(map[int]*Evaluator{1: bkrDummyEvaluatorForOutputLevel(t, 1)})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := dispatcher.bootstrapAtLevel(nil, 2); err == nil {
		t.Fatal("expected bootstrapAtLevel to reject unconfigured target level")
	}
	if _, err := dispatcher.bootstrapManyAtLevel(nil, 2); err == nil {
		t.Fatal("expected bootstrapManyAtLevel to reject unconfigured target level")
	}
	if _, err := dispatcher.outputLevel(2); err == nil {
		t.Fatal("expected outputLevel to reject unconfigured target level")
	}
}

func TestBootstrapKeyReuseA1_P2MultiFastUsedSingle(t *testing.T) {
	bkrRunA1Test(t, bkrCaseA1P2MultiFastUsedSingle())
}

func TestBootstrapKeyReuseA1_P2MultiFastUsedSparse(t *testing.T) {
	bkrRunA1Test(t, bkrCaseA1P2MultiFastUsedSparse())
}

func TestBootstrapKeyReuseA1_P2MultiFastUsedAll(t *testing.T) {
	bkrRunA1Test(t, bkrCaseA1P2MultiFastUsedAll())
}

func TestBootstrapKeyReuseA1_P2MultiFastUsedSparsePacked(t *testing.T) {
	bkrRunA1Test(t, bkrCaseA1P2MultiFastUsedSparsePacked())
}

func TestBootstrapKeyReuseA1_P4N15DenseUsedSparse(t *testing.T) {
	bkrRunA1Test(t, bkrCaseA1P4N15DenseUsedSparse())
}

func TestBootstrapKeyReuseA1_TenRNSLimbsTarget8(t *testing.T) {
	bkrRunA1Test(t, bkrCaseA1TenRNSLimbsTarget8())
}

func TestBootstrapKeyReuseA1_TenRNSLimbsChainedTargetSwitch(t *testing.T) {
	if err := runBootstrapKeyReuseA1ChainedTargetSwitch(t, bkrCaseA1TenRNSLimbsChainedTargetSwitch(), []int{5, 8, 9}); err != nil {
		t.Fatal(err)
	}
}

func TestBootstrapKeyReuseA1_UsedTargetNotInAllRejectsBeforeKeygen(t *testing.T) {
	bkrRunA1ExpectedFailure(t, bkrCaseA1UsedTargetNotInAll())
}

func TestBootstrapKeyReuseA1_DuplicateUsedTargetRejectsBeforeKeygen(t *testing.T) {
	bkrRunA1ExpectedFailure(t, bkrCaseA1DuplicateUsedTarget())
}

func TestBootstrapKeyReuseA1_EmptyUsedTargetsRejectBeforeKeygen(t *testing.T) {
	bkrRunA1ExpectedFailure(t, bkrCaseA1EmptyUsedTargets())
}

func TestBootstrapKeyReuseA1_CSVOnlyUsedTargetContract(t *testing.T) {
	runDir := t.TempDir()
	bkrSetResultDirForTest(t, runDir)

	spec := bkrCaseA1P2MultiFastUsedSparse()
	if err := runBootstrapKeyReuseA1(t, spec); err != nil {
		t.Fatal(err)
	}

	bkrAssertCSVOnlyOutput(t, runDir)
	bkrAssertCSVTargetRowsForTest(t, runDir, spec.UsedTargetLevels)

	experimentRows := bkrReadCSVForTest(t, filepath.Join(runDir, "experiment_case.csv"))
	if got, want := bkrCSVValueForTest(t, experimentRows, 1, "all_target_levels"), "1;2;3"; got != want {
		t.Fatalf("all_target_levels=%q, want %q", got, want)
	}
	if got, want := bkrCSVValueForTest(t, experimentRows, 1, "used_target_levels"), "1;3"; got != want {
		t.Fatalf("used_target_levels=%q, want %q", got, want)
	}

	materialRows := bkrReadCSVForTest(t, filepath.Join(runDir, "material_metrics.csv"))
	for row := 1; row < len(materialRows); row++ {
		if got := bkrCSVValueForTest(t, materialRows, row, "shared_rotation_keys"); got != "0" {
			t.Fatalf("shared_rotation_keys=%q, want 0", got)
		}
		if got := bkrCSVValueForTest(t, materialRows, row, "shared_encoded_diagonals"); got != "0" {
			t.Fatalf("shared_encoded_diagonals=%q, want 0", got)
		}
		if got := bkrCSVValueForTest(t, materialRows, row, "rns_slice_success"); got != "not_applicable" {
			t.Fatalf("rns_slice_success=%q, want not_applicable", got)
		}
		if got := bkrCSVValueForTest(t, materialRows, row, "fallback_reason"); got != "none" {
			t.Fatalf("fallback_reason=%q, want none", got)
		}
	}
}

func TestBootstrapKeyReuseA1_MatchesA0UsedTargets(t *testing.T) {
	a0Dir := t.TempDir()
	bkrSetResultDirForTest(t, a0Dir)
	if err := runBootstrapKeyReuseA0(t, bkrCaseP2MultiFastClustered()); err != nil {
		t.Fatal(err)
	}

	a1Dir := t.TempDir()
	bkrSetResultDirForTest(t, a1Dir)
	a1Spec := bkrCaseA1P2MultiFastUsedSparse()
	if err := runBootstrapKeyReuseA1(t, a1Spec); err != nil {
		t.Fatal(err)
	}

	bkrAssertCSVTargetRowsForTest(t, a1Dir, []int{1, 3})

	a0Targets := bkrReadCSVForTest(t, filepath.Join(a0Dir, "target_results.csv"))
	a1Targets := bkrReadCSVForTest(t, filepath.Join(a1Dir, "target_results.csv"))
	a0Materials := bkrReadCSVForTest(t, filepath.Join(a0Dir, "material_metrics.csv"))
	a1Materials := bkrReadCSVForTest(t, filepath.Join(a1Dir, "material_metrics.csv"))

	for _, targetLevel := range []int{1, 3} {
		for _, column := range []string{"output_level", "expected_output_level", "output_level_equality", "output_scale_equality"} {
			got := bkrCSVValueForTargetForTest(t, a1Targets, targetLevel, column)
			want := bkrCSVValueForTargetForTest(t, a0Targets, targetLevel, column)
			if got != want {
				t.Fatalf("target %d %s: A1=%q A0=%q", targetLevel, column, got, want)
			}
		}

		for _, column := range []string{
			"generated_evaluation_key_count",
			"generated_rotation_key_count",
			"generated_encoded_diagonal_count",
			"persistent_key_bytes",
			"persistent_matrix_bytes",
		} {
			got := bkrCSVValueForTargetForTest(t, a1Materials, targetLevel, column)
			want := bkrCSVValueForTargetForTest(t, a0Materials, targetLevel, column)
			if got != want {
				t.Fatalf("target %d %s: A1=%q A0=%q", targetLevel, column, got, want)
			}
		}
	}
}

func bkrDummyEvaluatorForOutputLevel(t *testing.T, outputLevel int) *Evaluator {
	t.Helper()
	spec := bkrCaseP2MultiFastClustered()
	literal, err := buildTargetResidualLiteral(spec, outputLevel)
	if err != nil {
		t.Fatal(err)
	}
	params, err := ckks.NewParametersFromLiteral(literal)
	if err != nil {
		t.Fatal(err)
	}
	return &Evaluator{Parameters: Parameters{ResidualParameters: params}}
}

func bkrAssertCSVTargetRowsForTest(t *testing.T, dir string, want []int) {
	t.Helper()

	wantSet := bkrSortedUniqueInts(want)
	for _, file := range []string{
		"target_results.csv",
		"material_metrics.csv",
		"runtime_metrics.csv",
		"parameter_chain_baseline.csv",
		"galois_key_baseline.csv",
		"linear_transform_schedule_baseline.csv",
		"encoded_diagonal_baseline.csv",
		"material_baseline_index.csv",
		"rotation_key_view.csv",
	} {
		rows := bkrReadCSVForTest(t, filepath.Join(dir, file))
		got := bkrCSVTargetLevelsForTest(t, rows)
		if !reflect.DeepEqual(got, wantSet) {
			t.Fatalf("%s target rows=%v, want %v", file, got, wantSet)
		}
	}

	summaryRows := bkrReadCSVForTest(t, filepath.Join(dir, "summary.csv"))
	if got, want := bkrCSVValueForTest(t, summaryRows, 1, "target_levels"), bkrJoinInts(want); got != want {
		t.Fatalf("summary target_levels=%q, want %q", got, want)
	}
	if got, want := bkrCSVIntForTest(t, summaryRows, 1, "total_targets"), len(want); got != want {
		t.Fatalf("summary total_targets=%d, want %d", got, want)
	}
	if got, want := bkrCSVIntForTest(t, summaryRows, 1, "successful_targets"), len(want); got != want {
		t.Fatalf("summary successful_targets=%d, want %d", got, want)
	}
	if got := bkrCSVValueForTest(t, summaryRows, 1, "lane"); got != bkrLaneA1 {
		t.Fatalf("summary lane=%q, want %q", got, bkrLaneA1)
	}
	if got := bkrCSVValueForTest(t, summaryRows, 1, "plan_id"); got != bkrPlanA1 {
		t.Fatalf("summary plan_id=%q, want %q", got, bkrPlanA1)
	}

	failuresRows := bkrReadCSVForTest(t, filepath.Join(dir, "failures.csv"))
	if len(failuresRows) != 1 {
		t.Fatalf("failures.csv rows=%d, want header only", len(failuresRows))
	}
}

func bkrCSVTargetLevelsForTest(t *testing.T, rows [][]string) []int {
	t.Helper()
	if len(rows) == 0 {
		t.Fatal("CSV has no rows")
	}

	targetColumn := -1
	for i, header := range rows[0] {
		if header == "target_level" {
			targetColumn = i
			break
		}
	}
	if targetColumn < 0 {
		t.Fatal("CSV missing target_level column")
	}

	targets := map[int]struct{}{}
	for row := 1; row < len(rows); row++ {
		value, err := strconv.Atoi(rows[row][targetColumn])
		if err != nil {
			t.Fatalf("cannot parse target_level row %d: %v", row, err)
		}
		targets[value] = struct{}{}
	}

	values := make([]int, 0, len(targets))
	for value := range targets {
		values = append(values, value)
	}
	sort.Ints(values)
	return values
}

func bkrCSVValueForTargetForTest(t *testing.T, rows [][]string, targetLevel int, column string) string {
	t.Helper()
	for row := 1; row < len(rows); row++ {
		if bkrCSVIntForTest(t, rows, row, "target_level") == targetLevel {
			return bkrCSVValueForTest(t, rows, row, column)
		}
	}
	t.Fatalf("missing target_level=%d", targetLevel)
	return ""
}

func bkrSortedUniqueInts(values []int) []int {
	set := make(map[int]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	out := make([]int, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}

func bkrCaseA1P2MultiFastUsedSingle() bkrCaseSpec {
	return bkrP2MultiFastBase("a1_p2_multi_fast_used_single", []int{1, 2, 3}, func(spec *bkrCaseSpec) {
		spec.UsedTargetLevels = []int{3}
	})
}

func bkrCaseA1P2MultiFastUsedSparse() bkrCaseSpec {
	return bkrP2MultiFastBase("a1_p2_multi_fast_used_sparse", []int{1, 2, 3}, func(spec *bkrCaseSpec) {
		spec.UsedTargetLevels = []int{1, 3}
	})
}

func bkrCaseA1P2MultiFastUsedAll() bkrCaseSpec {
	return bkrP2MultiFastBase("a1_p2_multi_fast_used_all", []int{1, 2, 3}, func(spec *bkrCaseSpec) {
		spec.UsedTargetLevels = []int{1, 2, 3}
	})
}

func bkrCaseA1P2MultiFastUsedSparsePacked() bkrCaseSpec {
	return bkrP2MultiFastBase("a1_p2_multi_fast_used_sparse_packed", []int{1, 2, 3}, func(spec *bkrCaseSpec) {
		spec.UsedTargetLevels = []int{1, 3}
		spec.PackedMode = true
	})
}

func bkrCaseA1P4N15DenseUsedSparse() bkrCaseSpec {
	return buildBootstrapKeyReuseCase("a1_p4_n15_dense_used_sparse", "P4_N15_DENSE", N15QP880H16384H32.SchemeParams, N15QP880H16384H32.BootstrappingParams, []int{1, 2, 3, 4}, func(spec *bkrCaseSpec) {
		spec.UsedTargetLevels = []int{1, 4}
	})
}

func bkrCaseA1TenRNSLimbsTarget8() bkrCaseSpec {
	return buildBootstrapKeyReuseCase(
		"a1_ten_rns_limbs_target8",
		"P7_TEN_RNS_LIMBS_TARGET8",
		ckks.ParametersLiteral{LogN: 10, LogQ: []int{60, 40, 40, 40, 40, 40, 40, 40, 40, 40}, LogP: []int{61}, LogDefaultScale: 40},
		ParametersLiteral{LogN: bkrPointyInt(10)},
		[]int{8, 9},
		func(spec *bkrCaseSpec) {
			spec.UsedTargetLevels = []int{8}
			spec.LogSlots = 9
			spec.FastInsecureBootstrapping = true
		},
	)
}

func bkrCaseA1TenRNSLimbsChainedTargetSwitch() bkrCaseSpec {
	return buildBootstrapKeyReuseCase(
		"a1_ten_rns_limbs_chain_2_5_1_8_1_9",
		"P7_TEN_RNS_LIMBS_CHAIN",
		ckks.ParametersLiteral{LogN: 10, LogQ: []int{60, 40, 40, 40, 40, 40, 40, 40, 40, 40}, LogP: []int{61}, LogDefaultScale: 40},
		ParametersLiteral{LogN: bkrPointyInt(10)},
		[]int{5, 8, 9},
		func(spec *bkrCaseSpec) {
			spec.UsedTargetLevels = []int{5, 8, 9}
			spec.LogSlots = 9
			spec.FastInsecureBootstrapping = true
		},
	)
}

func bkrCaseA1UsedTargetNotInAll() bkrCaseSpec {
	return bkrP2MultiFastBase("a1_used_target_not_in_all_rejects_before_keygen", []int{1, 3}, func(spec *bkrCaseSpec) {
		spec.UsedTargetLevels = []int{2}
	})
}

func bkrCaseA1DuplicateUsedTarget() bkrCaseSpec {
	return bkrP2MultiFastBase("a1_duplicate_used_target_rejects_before_keygen", []int{1, 2, 3}, func(spec *bkrCaseSpec) {
		spec.UsedTargetLevels = []int{1, 1}
	})
}

func bkrCaseA1EmptyUsedTargets() bkrCaseSpec {
	return bkrP2MultiFastBase("a1_empty_used_targets_reject_before_keygen", []int{1, 2, 3}, func(spec *bkrCaseSpec) {
		spec.UsedTargetLevels = []int{}
	})
}
