package bootstrapping

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func runBootstrapKeyReuseA5(t *testing.T, spec bkrCaseSpec) error {
	t.Helper()

	if spec.LongOnly && !*flagLongTest {
		t.Skip("long A5 bootstrap key reuse case; rerun with -args -long")
	}

	rec, err := newBootstrapKeyReuseRecorder(t, spec, bkrA5RunConfig(spec))
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
		return fmt.Errorf("A5 case %s failed; result_dir=%s: %w", spec.CaseID, summary.ResultDirectory, runErr)
	}

	rotationPool := newBKRRotationKeyPool(bkrPlanA5, spec)
	schedulePool := newBKRLinearTransformSchedulePool(bkrPlanA5, spec)
	diagonalPool := newBKREncodedDiagonalPool(bkrPlanA5, spec)
	prepared := make(map[int]*bkrA1PreparedTarget, len(spec.UsedTargetLevels))
	evaluators := make(map[int]*Evaluator, len(spec.UsedTargetLevels))

	for _, targetLevel := range spec.UsedTargetLevels {
		target, err := bkrPrepareA2TargetForPlan(t, rec, spec, targetLevel, rotationPool, bkrPlanA5)
		if err != nil {
			runErr = errors.Join(runErr, err)
			continue
		}
		if _, err := bkrInternA3TargetSchedules(schedulePool, target); err != nil {
			_ = rec.writeFailure(targetLevel, "schedule_interning", err.Error(), false, true, false)
			runErr = errors.Join(runErr, err)
			continue
		}
		if err := bkrInternA4TargetEncodedDiagonals(diagonalPool, target); err != nil {
			_ = rec.writeFailure(targetLevel, "encoded_diagonal_interning", err.Error(), false, true, false)
			runErr = errors.Join(runErr, err)
			continue
		}
		target.material.RNSSliceSuccess = bkrA5DefaultRNSSliceStatus()
		prepared[targetLevel] = target
		evaluators[targetLevel] = target.eval
	}

	rec.addRotationKeyPoolRecords(rotationPool.records())

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
				if err := bkrRunA2BootstrapTargetForPlan(t, rec, spec, dispatcher, target, bkrPlanA5); err != nil {
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
		return fmt.Errorf("A5 case %s failed; result_dir=%s: %w", spec.CaseID, summary.ResultDirectory, runErr)
	}

	return nil
}

func TestBootstrapKeyReuseA5_DefaultOffContract(t *testing.T) {
	runDir := t.TempDir()
	bkrSetResultDirForTest(t, runDir)

	spec := bkrCaseA5P2MultiFastUsedSparse()
	if err := runBootstrapKeyReuseA5(t, spec); err != nil {
		t.Fatal(err)
	}

	bkrAssertCSVOnlyOutput(t, runDir)
	bkrAssertCSVTargetRowsForPlanForTest(t, runDir, spec.UsedTargetLevels, bkrLaneA5, bkrPlanA5)

	summaryRows := bkrReadCSVForTest(t, filepath.Join(runDir, "summary.csv"))
	if got := bkrCSVValueForTest(t, summaryRows, 1, "passed"); got != "true" {
		t.Fatalf("summary passed=%q, want true", got)
	}
	if got := bkrCSVValueForTest(t, summaryRows, 1, "rns_slice_success"); got != "disabled" {
		t.Fatalf("summary rns_slice_success=%q, want disabled", got)
	}

	materialRows := bkrReadCSVForTest(t, filepath.Join(runDir, "material_metrics.csv"))
	encodedRows := bkrReadCSVForTest(t, filepath.Join(runDir, "encoded_diagonal_baseline.csv"))
	indexRows := bkrReadCSVForTest(t, filepath.Join(runDir, "material_baseline_index.csv"))
	for row := 1; row < len(materialRows); row++ {
		targetLevel := bkrCSVIntForTest(t, materialRows, row, "target_level")
		if got := bkrCSVValueForTest(t, materialRows, row, "rns_slice_success"); got != "disabled" {
			t.Fatalf("target %d rns_slice_success=%q, want disabled", targetLevel, got)
		}
		generated := bkrCSVIntForTest(t, materialRows, row, "generated_encoded_diagonal_count")
		shared := bkrCSVIntForTest(t, materialRows, row, "shared_encoded_diagonals")
		baseline := bkrCSVRowCountForTargetForTest(t, encodedRows, targetLevel)
		if generated+shared != baseline {
			t.Fatalf("target %d generated+shared encoded diagonals=%d, want baseline count=%d", targetLevel, generated+shared, baseline)
		}
		if got := bkrCSVValueForTargetForTest(t, indexRows, targetLevel, "encoded_diagonal_count_matches_metrics"); got != "true" {
			t.Fatalf("target %d encoded_diagonal_count_matches_metrics=%q, want true", targetLevel, got)
		}
	}
}

func TestBootstrapKeyReuseA5_MatchesA0UsedTargets(t *testing.T) {
	a0Dir := t.TempDir()
	bkrSetResultDirForTest(t, a0Dir)
	if err := runBootstrapKeyReuseA0(t, bkrCaseP2MultiFastClustered()); err != nil {
		t.Fatal(err)
	}

	a5Dir := t.TempDir()
	bkrSetResultDirForTest(t, a5Dir)
	a5Spec := bkrCaseA5P2MultiFastUsedSparse()
	if err := runBootstrapKeyReuseA5(t, a5Spec); err != nil {
		t.Fatal(err)
	}

	a0Targets := bkrReadCSVForTest(t, filepath.Join(a0Dir, "target_results.csv"))
	a5Targets := bkrReadCSVForTest(t, filepath.Join(a5Dir, "target_results.csv"))

	for _, targetLevel := range []int{1, 3} {
		for _, column := range []string{
			"output_level",
			"expected_output_level",
			"output_level_equality",
			"output_scale_equality",
			"bootstrap_error_status",
			"packed_ciphertexts",
		} {
			got := bkrCSVValueForTargetForTest(t, a5Targets, targetLevel, column)
			want := bkrCSVValueForTargetForTest(t, a0Targets, targetLevel, column)
			if got != want {
				t.Fatalf("target %d %s: A5=%q A0=%q", targetLevel, column, got, want)
			}
		}
	}
}

func bkrCaseA5P2MultiFastUsedSparse() bkrCaseSpec {
	return bkrP2MultiFastBase("a5_p2_multi_fast_used_sparse", []int{1, 2, 3}, func(spec *bkrCaseSpec) {
		spec.UsedTargetLevels = []int{1, 3}
	})
}
