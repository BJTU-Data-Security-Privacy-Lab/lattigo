package bootstrapping

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

func runBootstrapKeyReuseA3(t *testing.T, spec bkrCaseSpec) error {
	t.Helper()

	if spec.LongOnly && !*flagLongTest {
		t.Skip("long A3 bootstrap key reuse case; rerun with -args -long")
	}

	rec, err := newBootstrapKeyReuseRecorder(t, spec, bkrA3RunConfig(spec))
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
		return fmt.Errorf("A3 case %s failed; result_dir=%s: %w", spec.CaseID, summary.ResultDirectory, runErr)
	}

	rotationPool := newBKRRotationKeyPool(bkrPlanA3, spec)
	schedulePool := newBKRLinearTransformSchedulePool(bkrPlanA3, spec)
	prepared := make(map[int]*bkrA1PreparedTarget, len(spec.UsedTargetLevels))
	evaluators := make(map[int]*Evaluator, len(spec.UsedTargetLevels))

	for _, targetLevel := range spec.UsedTargetLevels {
		target, err := bkrPrepareA2TargetForPlan(t, rec, spec, targetLevel, rotationPool, bkrPlanA3)
		if err != nil {
			runErr = errors.Join(runErr, err)
			continue
		}
		if _, err := bkrInternA3TargetSchedules(schedulePool, target); err != nil {
			_ = rec.writeFailure(targetLevel, "schedule_interning", err.Error(), false, true, false)
			runErr = errors.Join(runErr, err)
			continue
		}
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
				if err := bkrRunA2BootstrapTargetForPlan(t, rec, spec, dispatcher, target, bkrPlanA3); err != nil {
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
		return fmt.Errorf("A3 case %s failed; result_dir=%s: %w", spec.CaseID, summary.ResultDirectory, runErr)
	}

	return nil
}

func bkrInternA3TargetSchedules(pool *bkrLinearTransformSchedulePool, target *bkrA1PreparedTarget) ([]bkrLinearTransformScheduleViewRecord, error) {
	if target == nil {
		return nil, errors.New("cannot intern schedules for nil target")
	}

	views := make([]bkrLinearTransformScheduleViewRecord, 0, len(target.baseline.Schedules))
	for _, schedule := range target.baseline.Schedules {
		view, _, err := pool.acquire(schedule)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func TestBootstrapKeyReuseA3_P2MultiFastScheduleInterningContract(t *testing.T) {
	runDir := t.TempDir()
	bkrSetResultDirForTest(t, runDir)

	spec := bkrCaseA3P2MultiFastUsedSparse()
	if err := runBootstrapKeyReuseA3(t, spec); err != nil {
		t.Fatal(err)
	}

	bkrAssertCSVOnlyOutput(t, runDir)
	bkrAssertCSVTargetRowsForPlanForTest(t, runDir, spec.UsedTargetLevels, bkrLaneA3, bkrPlanA3)

	summaryRows := bkrReadCSVForTest(t, filepath.Join(runDir, "summary.csv"))
	if got := bkrCSVValueForTest(t, summaryRows, 1, "passed"); got != "true" {
		t.Fatalf("summary passed=%q, want true", got)
	}
	if got := bkrCSVValueForTest(t, summaryRows, 1, "shared_encoded_diagonals"); got != "0" {
		t.Fatalf("summary shared_encoded_diagonals=%q, want 0 for A3", got)
	}

	scheduleRows := bkrReadCSVForTest(t, filepath.Join(runDir, "linear_transform_schedule_baseline.csv"))
	if len(scheduleRows) <= 1 {
		t.Fatal("linear_transform_schedule_baseline.csv has no schedule rows")
	}
	for row := 1; row < len(scheduleRows); row++ {
		if got := bkrCSVValueForTest(t, scheduleRows, row, "plan_id"); got != bkrPlanA3 {
			t.Fatalf("schedule row %d plan_id=%q, want %q", row, got, bkrPlanA3)
		}
		if got := bkrCSVValueForTest(t, scheduleRows, row, "transform_schedule_id"); got == "" {
			t.Fatalf("schedule row %d has empty transform_schedule_id", row)
		}
		if got := bkrCSVValueForTest(t, scheduleRows, row, "matrix_schedule_id"); got == "" {
			t.Fatalf("schedule row %d has empty matrix_schedule_id", row)
		}
	}

	materialRows := bkrReadCSVForTest(t, filepath.Join(runDir, "material_metrics.csv"))
	encodedRows := bkrReadCSVForTest(t, filepath.Join(runDir, "encoded_diagonal_baseline.csv"))
	indexRows := bkrReadCSVForTest(t, filepath.Join(runDir, "material_baseline_index.csv"))

	for row := 1; row < len(materialRows); row++ {
		targetLevel := bkrCSVIntForTest(t, materialRows, row, "target_level")
		if got := bkrCSVValueForTest(t, materialRows, row, "shared_encoded_diagonals"); got != "0" {
			t.Fatalf("target %d shared_encoded_diagonals=%q, want 0 for A3", targetLevel, got)
		}
		generated := bkrCSVIntForTest(t, materialRows, row, "generated_encoded_diagonal_count")
		baseline := bkrCSVRowCountForTargetForTest(t, encodedRows, targetLevel)
		if generated != baseline {
			t.Fatalf("target %d generated encoded diagonals=%d, want baseline count=%d", targetLevel, generated, baseline)
		}
		if got := bkrCSVValueForTargetForTest(t, indexRows, targetLevel, "encoded_diagonal_count_matches_metrics"); got != "true" {
			t.Fatalf("target %d encoded_diagonal_count_matches_metrics=%q, want true", targetLevel, got)
		}
	}
}

func TestBootstrapKeyReuseA3SchedulePoolSharesOnlyExactScheduleID(t *testing.T) {
	pool := newBKRLinearTransformSchedulePool(bkrPlanA3, bkrCaseA3P2MultiFastUsedSparse())

	owner := bkrA3ScheduleRecordForTest(1, "schedule:same", "matrix:same")
	ownerView, generated, err := pool.acquire(owner)
	if err != nil {
		t.Fatal(err)
	}
	if !generated {
		t.Fatal("first schedule acquire did not create an owner")
	}
	if ownerView.IsShared {
		t.Fatal("owner schedule view marked shared")
	}

	consumer := bkrA3ScheduleRecordForTest(3, "schedule:same", "matrix:same")
	sharedView, generated, err := pool.acquire(consumer)
	if err != nil {
		t.Fatal(err)
	}
	if generated {
		t.Fatal("exact schedule ID match generated a duplicate owner")
	}
	if !sharedView.IsShared || sharedView.OwnerTargetLevel != 1 || sharedView.FallbackReason != "none" {
		t.Fatalf("shared schedule view=%+v, want shared owner target 1 without fallback", sharedView)
	}

	different := bkrA3ScheduleRecordForTest(3, "schedule:different", "matrix:different")
	differentView, generated, err := pool.acquire(different)
	if err != nil {
		t.Fatal(err)
	}
	if !generated {
		t.Fatal("different schedule ID did not create a distinct owner")
	}
	if differentView.IsShared {
		t.Fatal("different schedule ID was incorrectly shared")
	}

	colliding := bkrA3ScheduleRecordForTest(4, "schedule:same", "matrix:same")
	colliding.DFTLevelQ++
	if _, _, err := pool.acquire(colliding); err == nil {
		t.Fatal("expected same schedule ID with different schedule fields to be rejected")
	}

	records := pool.records()
	if len(records) != 2 {
		t.Fatalf("schedule pool records=%d, want two distinct owners", len(records))
	}
	if got, want := records[0].ConsumerTargetLevels, []int{1, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shared schedule consumers=%v, want %v", got, want)
	}
}

func TestBootstrapKeyReuseA3_MatchesA0UsedTargets(t *testing.T) {
	a0Dir := t.TempDir()
	bkrSetResultDirForTest(t, a0Dir)
	if err := runBootstrapKeyReuseA0(t, bkrCaseP2MultiFastClustered()); err != nil {
		t.Fatal(err)
	}

	a3Dir := t.TempDir()
	bkrSetResultDirForTest(t, a3Dir)
	a3Spec := bkrCaseA3P2MultiFastUsedSparse()
	if err := runBootstrapKeyReuseA3(t, a3Spec); err != nil {
		t.Fatal(err)
	}

	a0Targets := bkrReadCSVForTest(t, filepath.Join(a0Dir, "target_results.csv"))
	a3Targets := bkrReadCSVForTest(t, filepath.Join(a3Dir, "target_results.csv"))

	for _, targetLevel := range []int{1, 3} {
		for _, column := range []string{
			"output_level",
			"expected_output_level",
			"output_level_equality",
			"output_scale_equality",
			"bootstrap_error_status",
			"packed_ciphertexts",
		} {
			got := bkrCSVValueForTargetForTest(t, a3Targets, targetLevel, column)
			want := bkrCSVValueForTargetForTest(t, a0Targets, targetLevel, column)
			if got != want {
				t.Fatalf("target %d %s: A3=%q A0=%q", targetLevel, column, got, want)
			}
		}
	}
}

func bkrA3ScheduleRecordForTest(targetLevel int, transformScheduleID, matrixScheduleID string) bkrLinearTransformScheduleBaseline {
	return bkrLinearTransformScheduleBaseline{
		RecordType:          "LinearTransformScheduleBaseline",
		SchemaVersion:       bkrSchemaVersion,
		PlanID:              bkrPlanA3,
		CaseID:              "a3_schedule_pool_unit",
		ParamsProfile:       "unit",
		TargetLevel:         targetLevel,
		MatrixName:          "coeffs_to_slots",
		TransformIndex:      0,
		DFTType:             "HomomorphicEncode",
		DFTTypeValue:        0,
		DFTFormat:           "RepackImagAsReal",
		DFTFormatValue:      2,
		DFTLogSlots:         9,
		DFTLevelQ:           12,
		DFTLevelP:           0,
		DFTLevels:           []int{1, 1},
		DFTBitReversed:      false,
		DFTLogBSGSRatio:     1,
		LTLevelQ:            12,
		LTLevelP:            0,
		LTN1:                4,
		LTLogRows:           0,
		LTLogCols:           9,
		LTScale:             "1",
		LTScaleLog2:         0,
		DiagonalIndices:     []int{0, 1, 2},
		GaloisElements:      []uint64{1, 3, 5},
		TransformScheduleID: transformScheduleID,
		MatrixScheduleID:    matrixScheduleID,
	}
}

func bkrCaseA3P2MultiFastUsedSparse() bkrCaseSpec {
	return bkrP2MultiFastBase("a3_p2_multi_fast_used_sparse", []int{1, 2, 3}, func(spec *bkrCaseSpec) {
		spec.UsedTargetLevels = []int{1, 3}
	})
}

func bkrCSVRowCountForTargetForTest(t *testing.T, rows [][]string, targetLevel int) int {
	t.Helper()

	count := 0
	for row := 1; row < len(rows); row++ {
		if bkrCSVIntForTest(t, rows, row, "target_level") == targetLevel {
			count++
		}
	}
	return count
}
