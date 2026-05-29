package bootstrapping

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

func runBootstrapKeyReuseA4(t *testing.T, spec bkrCaseSpec) error {
	t.Helper()

	if spec.LongOnly && !*flagLongTest {
		t.Skip("long A4 bootstrap key reuse case; rerun with -args -long")
	}

	rec, err := newBootstrapKeyReuseRecorder(t, spec, bkrA4RunConfig(spec))
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
		return fmt.Errorf("A4 case %s failed; result_dir=%s: %w", spec.CaseID, summary.ResultDirectory, runErr)
	}

	rotationPool := newBKRRotationKeyPool(bkrPlanA4, spec)
	schedulePool := newBKRLinearTransformSchedulePool(bkrPlanA4, spec)
	diagonalPool := newBKREncodedDiagonalPool(bkrPlanA4, spec)
	prepared := make(map[int]*bkrA1PreparedTarget, len(spec.UsedTargetLevels))
	evaluators := make(map[int]*Evaluator, len(spec.UsedTargetLevels))

	for _, targetLevel := range spec.UsedTargetLevels {
		target, err := bkrPrepareA2TargetForPlan(t, rec, spec, targetLevel, rotationPool, bkrPlanA4)
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
				if err := bkrRunA2BootstrapTargetForPlan(t, rec, spec, dispatcher, target, bkrPlanA4); err != nil {
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
		return fmt.Errorf("A4 case %s failed; result_dir=%s: %w", spec.CaseID, summary.ResultDirectory, runErr)
	}

	return nil
}

func TestBootstrapKeyReuseA4_P2MultiFastEncodedDiagonalSharingContract(t *testing.T) {
	runDir := t.TempDir()
	bkrSetResultDirForTest(t, runDir)

	spec := bkrCaseA4P2MultiFastUsedSparse()
	if err := runBootstrapKeyReuseA4(t, spec); err != nil {
		t.Fatal(err)
	}

	bkrAssertCSVOnlyOutput(t, runDir)
	bkrAssertCSVTargetRowsForPlanForTest(t, runDir, spec.UsedTargetLevels, bkrLaneA4, bkrPlanA4)

	summaryRows := bkrReadCSVForTest(t, filepath.Join(runDir, "summary.csv"))
	if got := bkrCSVValueForTest(t, summaryRows, 1, "passed"); got != "true" {
		t.Fatalf("summary passed=%q, want true", got)
	}
	if got := bkrCSVValueForTest(t, summaryRows, 1, "rns_slice_success"); got != "not_applicable" {
		t.Fatalf("summary rns_slice_success=%q, want not_applicable for A4", got)
	}

	encodedRows := bkrReadCSVForTest(t, filepath.Join(runDir, "encoded_diagonal_baseline.csv"))
	if len(encodedRows) <= 1 {
		t.Fatal("encoded_diagonal_baseline.csv has no diagonal rows")
	}
	for row := 1; row < len(encodedRows); row++ {
		if got := bkrCSVValueForTest(t, encodedRows, row, "plan_id"); got != bkrPlanA4 {
			t.Fatalf("encoded diagonal row %d plan_id=%q, want %q", row, got, bkrPlanA4)
		}
		if got := bkrCSVValueForTest(t, encodedRows, row, "encoded_diagonal_id"); got == "" {
			t.Fatalf("encoded diagonal row %d has empty encoded_diagonal_id", row)
		}
	}

	materialRows := bkrReadCSVForTest(t, filepath.Join(runDir, "material_metrics.csv"))
	indexRows := bkrReadCSVForTest(t, filepath.Join(runDir, "material_baseline_index.csv"))
	for row := 1; row < len(materialRows); row++ {
		targetLevel := bkrCSVIntForTest(t, materialRows, row, "target_level")
		generated := bkrCSVIntForTest(t, materialRows, row, "generated_encoded_diagonal_count")
		shared := bkrCSVIntForTest(t, materialRows, row, "shared_encoded_diagonals")
		baseline := bkrCSVRowCountForTargetForTest(t, encodedRows, targetLevel)
		if generated+shared != baseline {
			t.Fatalf("target %d generated+shared encoded diagonals=%d, want baseline count=%d", targetLevel, generated+shared, baseline)
		}
		if got := bkrCSVValueForTest(t, materialRows, row, "rns_slice_success"); got != "not_applicable" {
			t.Fatalf("target %d rns_slice_success=%q, want not_applicable for A4", targetLevel, got)
		}
		if got := bkrCSVValueForTargetForTest(t, indexRows, targetLevel, "encoded_diagonal_count_matches_metrics"); got != "true" {
			t.Fatalf("target %d encoded_diagonal_count_matches_metrics=%q, want true", targetLevel, got)
		}
	}
}

func TestBootstrapKeyReuseA4EncodedDiagonalPoolSharesOnlyExactID(t *testing.T) {
	pool := newBKREncodedDiagonalPool(bkrPlanA4, bkrCaseA4P2MultiFastUsedSparse())

	owner := bkrA4DiagonalRecordForTest(1, "diagonal:same")
	ownerView, generated, err := pool.acquire(owner, bkrZeroEncodedDiagonalForTest())
	if err != nil {
		t.Fatal(err)
	}
	if !generated {
		t.Fatal("first encoded diagonal acquire did not create an owner")
	}
	if ownerView.IsShared {
		t.Fatal("owner encoded diagonal view marked shared")
	}

	consumer := bkrA4DiagonalRecordForTest(3, "diagonal:same")
	sharedView, generated, err := pool.acquire(consumer, bkrZeroEncodedDiagonalForTest())
	if err != nil {
		t.Fatal(err)
	}
	if generated {
		t.Fatal("exact encoded_diagonal_id match generated a duplicate owner")
	}
	if !sharedView.IsShared || sharedView.OwnerTargetLevel != 1 || sharedView.FallbackReason != "none" {
		t.Fatalf("shared encoded diagonal view=%+v, want shared owner target 1 without fallback", sharedView)
	}

	differentID := bkrA4DiagonalRecordForTest(3, "diagonal:different-id")
	differentID.PolySHA256 = owner.PolySHA256
	differentIDView, generated, err := pool.acquire(differentID, bkrZeroEncodedDiagonalForTest())
	if err != nil {
		t.Fatal(err)
	}
	if !generated {
		t.Fatal("different encoded_diagonal_id did not create a distinct owner")
	}
	if differentIDView.IsShared {
		t.Fatal("different encoded_diagonal_id was incorrectly shared")
	}

	colliding := bkrA4DiagonalRecordForTest(4, "diagonal:same")
	colliding.LevelQ++
	if _, _, err := pool.acquire(colliding, bkrZeroEncodedDiagonalForTest()); err == nil {
		t.Fatal("expected same encoded_diagonal_id with different fields to be rejected")
	}

	records := pool.records()
	if len(records) != 2 {
		t.Fatalf("encoded diagonal pool records=%d, want two distinct owners", len(records))
	}
	if got, want := records[0].ConsumerTargetLevels, []int{1, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shared encoded diagonal consumers=%v, want %v", got, want)
	}
}

func TestBootstrapKeyReuseA4_MatchesA0UsedTargets(t *testing.T) {
	a0Dir := t.TempDir()
	bkrSetResultDirForTest(t, a0Dir)
	if err := runBootstrapKeyReuseA0(t, bkrCaseP2MultiFastClustered()); err != nil {
		t.Fatal(err)
	}

	a4Dir := t.TempDir()
	bkrSetResultDirForTest(t, a4Dir)
	a4Spec := bkrCaseA4P2MultiFastUsedSparse()
	if err := runBootstrapKeyReuseA4(t, a4Spec); err != nil {
		t.Fatal(err)
	}

	a0Targets := bkrReadCSVForTest(t, filepath.Join(a0Dir, "target_results.csv"))
	a4Targets := bkrReadCSVForTest(t, filepath.Join(a4Dir, "target_results.csv"))

	for _, targetLevel := range []int{1, 3} {
		for _, column := range []string{
			"output_level",
			"expected_output_level",
			"output_level_equality",
			"output_scale_equality",
			"bootstrap_error_status",
			"packed_ciphertexts",
		} {
			got := bkrCSVValueForTargetForTest(t, a4Targets, targetLevel, column)
			want := bkrCSVValueForTargetForTest(t, a0Targets, targetLevel, column)
			if got != want {
				t.Fatalf("target %d %s: A4=%q A0=%q", targetLevel, column, got, want)
			}
		}
	}
}

func bkrA4DiagonalRecordForTest(targetLevel int, encodedDiagonalID string) bkrEncodedDiagonalBaseline {
	return bkrEncodedDiagonalBaseline{
		RecordType:        "EncodedDiagonalBaseline",
		SchemaVersion:     bkrSchemaVersion,
		PlanID:            bkrPlanA4,
		CaseID:            "a4_encoded_diagonal_pool_unit",
		ParamsProfile:     "unit",
		TargetLevel:       targetLevel,
		MatrixName:        "coeffs_to_slots",
		TransformIndex:    0,
		DiagonalIndex:     1,
		LevelQ:            12,
		LevelP:            0,
		N:                 1024,
		QPrefixHash:       "sha256:q-prefix",
		PPrefixHash:       "sha256:p-prefix",
		PolyBinarySize:    4096,
		PolySHA256:        "sha256:poly",
		EncodedDiagonalID: encodedDiagonalID,
	}
}

func bkrCaseA4P2MultiFastUsedSparse() bkrCaseSpec {
	return bkrP2MultiFastBase("a4_p2_multi_fast_used_sparse", []int{1, 2, 3}, func(spec *bkrCaseSpec) {
		spec.UsedTargetLevels = []int{1, 3}
	})
}
