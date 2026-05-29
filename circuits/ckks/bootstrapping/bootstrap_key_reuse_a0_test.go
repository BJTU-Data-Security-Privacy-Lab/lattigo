package bootstrapping

import (
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/tuneinsight/lattigo/v6/core/rlwe"
	"github.com/tuneinsight/lattigo/v6/ring"
	"github.com/tuneinsight/lattigo/v6/schemes/ckks"
	"github.com/tuneinsight/lattigo/v6/utils"
)

func runBootstrapKeyReuseA0(t *testing.T, spec bkrCaseSpec) error {
	t.Helper()

	if spec.LongOnly && !*flagLongTest {
		t.Skip("long A0 bootstrap key reuse case; rerun with -args -long")
	}

	rec, err := newBootstrapKeyReuseRecorder(t, spec, bkrA0RunConfig(spec))
	if err != nil {
		return err
	}
	defer rec.close()

	var runErr error
	successfulTargets := 0

	if err := rec.writeExperimentCase(); err != nil {
		runErr = errors.Join(runErr, err)
	}

	for _, targetLevel := range spec.AllTargetLevels {
		if err := bkrRunA0Target(t, rec, spec, targetLevel); err != nil {
			runErr = errors.Join(runErr, err)
			continue
		}
		successfulTargets++
	}

	summary, summaryErr := rec.writeSummary(successfulTargets)
	if summaryErr != nil {
		runErr = errors.Join(runErr, summaryErr)
	}

	if runErr != nil {
		return fmt.Errorf("A0 case %s failed; result_dir=%s: %w", spec.CaseID, summary.ResultDirectory, runErr)
	}

	return nil
}

func bkrRunA0Target(t *testing.T, rec *bkrRecorder, spec bkrCaseSpec, targetLevel int) error {
	t.Helper()

	expectedOutputLevel := targetLevel
	if spec.ExpectedOutputLevelOverride != nil {
		expectedOutputLevel = *spec.ExpectedOutputLevelOverride
	}

	residualParams, btpParams, err := buildTargetBootstrappingParameters(spec, targetLevel)
	if err != nil {
		_ = rec.writeFailure(targetLevel, "build_parameters", err.Error(), true, false, false)
		return err
	}

	if have := btpParams.ResidualParameters.MaxLevel(); have != expectedOutputLevel {
		err := fmt.Errorf("pre-keygen output level check failed: residual max level=%d expected=%d", have, expectedOutputLevel)
		_ = rec.writeFailure(targetLevel, "output_level_pre_keygen", err.Error(), true, false, false)
		return err
	}

	sk, err := newDeterministicSecretKey(residualParams, spec.Seed+"/secret-key")
	if err != nil {
		_ = rec.writeFailure(targetLevel, "secret_key", err.Error(), true, false, false)
		return err
	}

	var keys *EvaluationKeys
	keygenElapsed, keygenPeak, err := bkrMeasurePhase(func() error {
		var keygenErr error
		keys, _, keygenErr = btpParams.GenEvaluationKeys(sk)
		return keygenErr
	})
	if err != nil {
		_ = rec.writeFailure(targetLevel, "keygen", err.Error(), false, false, false)
		return err
	}

	runtimeSampler := newBKRHeapSampler()
	constructionStart := time.Now()
	eval, err := NewEvaluator(btpParams, keys)
	constructionElapsed := time.Since(constructionStart)
	if err != nil {
		runtimePeak := runtimeSampler.stopAndMax()
		_ = runtimePeak
		_ = rec.writeFailure(targetLevel, "evaluator_construction", err.Error(), false, true, false)
		return err
	}

	if have := eval.OutputLevel(); have != expectedOutputLevel {
		runtimePeak := runtimeSampler.stopAndMax()
		_ = runtimePeak
		err := fmt.Errorf("evaluator OutputLevel=%d expected=%d", have, expectedOutputLevel)
		_ = rec.writeFailure(targetLevel, "output_level_after_evaluator", err.Error(), false, true, false)
		return err
	}

	baseline, err := collectBootstrapKeyReuseBaselineDetails(bkrPlanA0, spec, targetLevel, btpParams, keys, eval)
	if err != nil {
		runtimePeak := runtimeSampler.stopAndMax()
		_ = runtimePeak
		_ = rec.writeFailure(targetLevel, "baseline_details", err.Error(), false, true, false)
		return err
	}

	material := collectBootstrapKeyReuseMetrics(bkrPlanA0, spec, targetLevel, keys, eval, baseline)
	completeBootstrapKeyReuseBaselineIndex(&baseline, material)
	rotationPoolRecords, rotationViewRecords := bkrDefaultRotationKeyRecords(bkrPlanA0, spec, targetLevel, btpParams, keys)
	rec.addRotationKeyPoolRecords(rotationPoolRecords)
	rec.addRotationKeyViewRecords(rotationViewRecords)

	bootstrapStart := time.Now()
	outputs, wants, bootstrapErr := bkrBootstrapA0Ciphertexts(spec, residualParams, eval, sk, targetLevel)
	bootstrapElapsed := time.Since(bootstrapStart)
	runtimePeak := runtimeSampler.stopAndMax()

	runtimeMetrics := bkrRuntimeMetrics{
		RecordType:                    "RuntimeMetrics",
		SchemaVersion:                 bkrSchemaVersion,
		PlanID:                        bkrPlanA0,
		CaseID:                        spec.CaseID,
		ParamsProfile:                 spec.ProfileID,
		TargetLevel:                   targetLevel,
		KeygenTimeMS:                  bkrDurationMS(keygenElapsed),
		EvaluatorMatrixConstructionMS: bkrDurationMS(constructionElapsed),
		BootstrapLatencyMS:            bkrDurationMS(bootstrapElapsed),
		PeakKeygenHeapBytes:           keygenPeak,
		PeakRuntimeHeapBytes:          runtimePeak,
	}

	if bootstrapErr != nil {
		result := bkrEmptyTargetRunResult(bkrPlanA0, spec, targetLevel, expectedOutputLevel, residualParams, bootstrapErr.Error())
		if err := writeBootstrapKeyReuseResult(rec, result, material, runtimeMetrics, baseline); err != nil {
			return err
		}
		_ = rec.writeFailure(targetLevel, "bootstrap", bootstrapErr.Error(), false, true, true)
		return bootstrapErr
	}

	result, checkErr := bkrValidateBootstrapKeyReuseOutputs(t, bkrPlanA0, spec, targetLevel, expectedOutputLevel, residualParams, outputs, wants)
	if err := writeBootstrapKeyReuseResult(rec, result, material, runtimeMetrics, baseline); err != nil {
		return err
	}
	if checkErr != nil {
		_ = rec.writeFailure(targetLevel, "validate_output", checkErr.Error(), false, true, true)
		return checkErr
	}

	return nil
}

func bkrBootstrapA0Ciphertexts(spec bkrCaseSpec, params ckks.Parameters, eval *Evaluator, sk *rlwe.SecretKey, targetLevel int) ([]rlwe.Ciphertext, [][]complex128, error) {
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

		out, err := eval.Bootstrap(ct)
		if err != nil {
			return nil, nil, err
		}

		return []rlwe.Ciphertext{*out}, [][]complex128{values}, nil
	}

	logSlots := params.LogMaxSlots()
	if eval.LogMaxSlots() < logSlots {
		logSlots = eval.LogMaxSlots()
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

	outputs, err := eval.BootstrapMany(inputs)
	if err != nil {
		return nil, nil, err
	}

	return outputs, wants, nil
}

func bkrValidateBootstrapKeyReuseOutputs(t *testing.T, planID string, spec bkrCaseSpec, targetLevel, expectedOutputLevel int, params ckks.Parameters, outputs []rlwe.Ciphertext, wants [][]complex128) (bkrTargetRunResult, error) {
	t.Helper()

	expectedScale := params.DefaultScale()
	if spec.ExpectedScaleLogOverride != nil {
		expectedScale = rlwe.NewScale(math.Exp2(float64(*spec.ExpectedScaleLogOverride)))
	}

	outputLevel := -1
	outputScaleLog2 := 0.0
	outputLevelEquality := true
	outputScaleEquality := true
	avgReal := math.Inf(1)
	avgImag := math.Inf(1)

	encoder := ckks.NewEncoder(params)
	decryptorSeed, err := newDeterministicSecretKey(params, spec.Seed+"/secret-key")
	if err != nil {
		return bkrEmptyTargetRunResult(planID, spec, targetLevel, expectedOutputLevel, params, err.Error()), err
	}
	decryptor := rlwe.NewDecryptor(params, decryptorSeed)

	for i := range outputs {
		ct := &outputs[i]
		if outputLevel == -1 {
			outputLevel = ct.Level()
			outputScaleLog2 = ct.Scale.Log2()
		}
		if ct.Level() != expectedOutputLevel {
			outputLevelEquality = false
		}
		if !ct.Scale.Equal(expectedScale) {
			outputScaleEquality = false
		}

		precStats := ckks.GetPrecisionStats(params, encoder, decryptor, wants[i], ct, 0, false)
		if *printPrecisionStats {
			t.Log(precStats.String())
		}

		avgReal = math.Min(avgReal, precStats.AVGLog2Prec.Real)
		avgImag = math.Min(avgImag, precStats.AVGLog2Prec.Imag)
	}

	if math.IsInf(avgReal, 1) {
		avgReal = 0
	}
	if math.IsInf(avgImag, 1) {
		avgImag = 0
	}

	result := bkrTargetRunResult{
		RecordType:              "TargetRunResult",
		SchemaVersion:           bkrSchemaVersion,
		PlanID:                  planID,
		CaseID:                  spec.CaseID,
		ParamsProfile:           spec.ProfileID,
		TargetLevel:             targetLevel,
		OutputLevel:             outputLevel,
		ExpectedOutputLevel:     expectedOutputLevel,
		OutputLevelEquality:     outputLevelEquality,
		OutputScaleLog2:         outputScaleLog2,
		ExpectedOutputScaleLog2: expectedScale.Log2(),
		OutputScaleEquality:     outputScaleEquality,
		AverageLog2PrecReal:     avgReal,
		AverageLog2PrecImag:     avgImag,
		PrecisionThresholdBits:  spec.MinPrecisionBits,
		BootstrapErrorStatus:    "ok",
		PackedCiphertexts:       len(outputs),
	}

	var validationErr error
	if !outputLevelEquality {
		validationErr = errors.Join(validationErr, fmt.Errorf("output level mismatch: got at least one level different from %d", expectedOutputLevel))
	}
	if !outputScaleEquality {
		validationErr = errors.Join(validationErr, fmt.Errorf("output scale mismatch: expected log2 %.6f", expectedScale.Log2()))
	}
	if avgReal < spec.MinPrecisionBits || avgImag < spec.MinPrecisionBits {
		validationErr = errors.Join(validationErr, fmt.Errorf("precision below threshold %.2f: real=%.4f imag=%.4f", spec.MinPrecisionBits, avgReal, avgImag))
	}

	return result, validationErr
}

func bkrEmptyTargetRunResult(planID string, spec bkrCaseSpec, targetLevel, expectedOutputLevel int, params ckks.Parameters, status string) bkrTargetRunResult {
	expectedScale := params.DefaultScale()
	if spec.ExpectedScaleLogOverride != nil {
		expectedScale = rlwe.NewScale(math.Exp2(float64(*spec.ExpectedScaleLogOverride)))
	}

	return bkrTargetRunResult{
		RecordType:              "TargetRunResult",
		SchemaVersion:           bkrSchemaVersion,
		PlanID:                  planID,
		CaseID:                  spec.CaseID,
		ParamsProfile:           spec.ProfileID,
		TargetLevel:             targetLevel,
		OutputLevel:             -1,
		ExpectedOutputLevel:     expectedOutputLevel,
		OutputLevelEquality:     false,
		OutputScaleLog2:         0,
		ExpectedOutputScaleLog2: expectedScale.Log2(),
		OutputScaleEquality:     false,
		AverageLog2PrecReal:     0,
		AverageLog2PrecImag:     0,
		PrecisionThresholdBits:  spec.MinPrecisionBits,
		BootstrapErrorStatus:    status,
		PackedCiphertexts:       0,
	}
}

func bkrRunA0Test(t *testing.T, spec bkrCaseSpec) {
	t.Helper()
	if err := runBootstrapKeyReuseA0(t, spec); err != nil {
		t.Fatal(err)
	}
}

func bkrRunA0ExpectedFailure(t *testing.T, spec bkrCaseSpec) {
	t.Helper()
	if err := runBootstrapKeyReuseA0(t, spec); err == nil {
		t.Fatalf("expected A0 case %s to fail", spec.CaseID)
	}
}

func TestBootstrapKeyReuseA0_P0TinyNativeSingle(t *testing.T) {
	bkrRunA0Test(t, bkrCaseP0TinyNativeSingle())
}

func TestBootstrapKeyReuseA0_P1TinyRingSwitchSingle(t *testing.T) {
	bkrRunA0Test(t, bkrCaseP1TinyRingSwitchSingle())
}

func TestBootstrapKeyReuseA0_P2MultiFastSingle(t *testing.T) {
	bkrRunA0Test(t, bkrCaseP2MultiFastSingle())
}

func TestBootstrapKeyReuseA0_P2MultiFastClustered(t *testing.T) {
	bkrRunA0Test(t, bkrCaseP2MultiFastClustered())
}

func TestBootstrapKeyReuseA0_P2MultiFastSparse(t *testing.T) {
	bkrRunA0Test(t, bkrCaseP2MultiFastSparse())
}

func TestBootstrapKeyReuseA0_P2MultiFastRandomFixed(t *testing.T) {
	bkrRunA0Test(t, bkrCaseP2MultiFastRandomFixed())
}

func TestBootstrapKeyReuseA0_P2MultiFastClusteredPacked(t *testing.T) {
	bkrRunA0Test(t, bkrCaseP2MultiFastClusteredPacked())
}

func TestBootstrapKeyReuseA0_P3N15SparseSingle(t *testing.T) {
	bkrRunA0Test(t, bkrCaseP3N15SparseSingle())
}

func TestBootstrapKeyReuseA0_P3N15SparseClustered(t *testing.T) {
	bkrRunA0Test(t, bkrCaseP3N15SparseClustered())
}

func TestBootstrapKeyReuseA0_P4N15DenseSingle(t *testing.T) {
	bkrRunA0Test(t, bkrCaseP4N15DenseSingle())
}

func TestBootstrapKeyReuseA0_P4N15DenseClustered(t *testing.T) {
	bkrRunA0Test(t, bkrCaseP4N15DenseClustered())
}

func TestBootstrapKeyReuseA0_P4N15DenseSparse(t *testing.T) {
	bkrRunA0Test(t, bkrCaseP4N15DenseSparse())
}

func TestBootstrapKeyReuseA0_P4N15DenseRandom(t *testing.T) {
	bkrRunA0Test(t, bkrCaseP4N15DenseRandom())
}

func TestBootstrapKeyReuseA0_P5N16SparseLongSingle(t *testing.T) {
	bkrRunA0Test(t, bkrCaseP5N16SparseLongSingle())
}

func TestBootstrapKeyReuseA0_P5N16SparseLongClustered(t *testing.T) {
	bkrRunA0Test(t, bkrCaseP5N16SparseLongClustered())
}

func TestBootstrapKeyReuseA0_P5N16SparseLongSparse(t *testing.T) {
	bkrRunA0Test(t, bkrCaseP5N16SparseLongSparse())
}

func TestBootstrapKeyReuseA0_P5N16SparseLongRandom(t *testing.T) {
	bkrRunA0Test(t, bkrCaseP5N16SparseLongRandom())
}

func TestBootstrapKeyReuseA0_P6N16DenseLongSingle(t *testing.T) {
	bkrRunA0Test(t, bkrCaseP6N16DenseLongSingle())
}

func TestBootstrapKeyReuseA0_P6N16DenseLongClustered(t *testing.T) {
	bkrRunA0Test(t, bkrCaseP6N16DenseLongClustered())
}

func TestBootstrapKeyReuseA0_P6N16DenseLongSparse(t *testing.T) {
	bkrRunA0Test(t, bkrCaseP6N16DenseLongSparse())
}

func TestBootstrapKeyReuseA0_P6N16DenseLongRandom(t *testing.T) {
	bkrRunA0Test(t, bkrCaseP6N16DenseLongRandom())
}

func TestBootstrapKeyReuseA0_InvalidTargetRejectsBeforeKeygen(t *testing.T) {
	bkrRunA0ExpectedFailure(t, bkrCaseInvalidTarget())
}

func TestBootstrapKeyReuseA0_OutputLevelMismatchFails(t *testing.T) {
	bkrRunA0ExpectedFailure(t, bkrCaseOutputLevelMismatch())
}

func TestBootstrapKeyReuseA0_OutputScaleMismatchFails(t *testing.T) {
	bkrRunA0ExpectedFailure(t, bkrCaseOutputScaleMismatch())
}

func TestBootstrapKeyReuseA0_CSVOnlyOutputContract(t *testing.T) {
	runDir := t.TempDir()
	bkrSetResultDirForTest(t, runDir)

	if err := runBootstrapKeyReuseA0(t, bkrCaseP0TinyNativeSingle()); err != nil {
		t.Fatal(err)
	}

	bkrAssertCSVOnlyOutput(t, runDir)

	materialRows := bkrReadCSVForTest(t, filepath.Join(runDir, "material_metrics.csv"))
	galoisRows := bkrReadCSVForTest(t, filepath.Join(runDir, "galois_key_baseline.csv"))
	encodedRows := bkrReadCSVForTest(t, filepath.Join(runDir, "encoded_diagonal_baseline.csv"))
	indexRows := bkrReadCSVForTest(t, filepath.Join(runDir, "material_baseline_index.csv"))

	if len(materialRows) != 2 {
		t.Fatalf("material_metrics.csv rows=%d, want header + one target row", len(materialRows))
	}
	if len(galoisRows) != 2 {
		t.Fatalf("galois_key_baseline.csv rows=%d, want header + one target row", len(galoisRows))
	}
	if len(indexRows) != 2 {
		t.Fatalf("material_baseline_index.csv rows=%d, want header + one target row", len(indexRows))
	}

	rotationMetric := bkrCSVIntForTest(t, materialRows, 1, "generated_rotation_key_count")
	diagonalMetric := bkrCSVIntForTest(t, materialRows, 1, "generated_encoded_diagonal_count")
	galoisGenerated := bkrCSVIntForTest(t, galoisRows, 1, "generated_count")

	if rotationMetric != galoisGenerated {
		t.Fatalf("rotation count mismatch: material_metrics=%d galois_baseline=%d", rotationMetric, galoisGenerated)
	}
	if diagonalMetric != len(encodedRows)-1 {
		t.Fatalf("encoded diagonal count mismatch: material_metrics=%d encoded_diagonal_baseline=%d", diagonalMetric, len(encodedRows)-1)
	}
	if got := bkrCSVValueForTest(t, indexRows, 1, "rotation_count_matches_metrics"); got != "true" {
		t.Fatalf("rotation_count_matches_metrics=%q, want true", got)
	}
	if got := bkrCSVValueForTest(t, indexRows, 1, "encoded_diagonal_count_matches_metrics"); got != "true" {
		t.Fatalf("encoded_diagonal_count_matches_metrics=%q, want true", got)
	}
}

func TestBootstrapKeyReuseA0_CSVOnlyFailureSummary(t *testing.T) {
	runDir := t.TempDir()
	bkrSetResultDirForTest(t, runDir)

	if err := runBootstrapKeyReuseA0(t, bkrCaseInvalidTarget()); err == nil {
		t.Fatal("expected invalid target case to fail")
	}

	bkrAssertCSVOnlyOutput(t, runDir)

	summaryRows := bkrReadCSVForTest(t, filepath.Join(runDir, "summary.csv"))
	failuresRows := bkrReadCSVForTest(t, filepath.Join(runDir, "failures.csv"))

	if got := bkrCSVValueForTest(t, summaryRows, 1, "status"); got != "fail" {
		t.Fatalf("summary status=%q, want fail", got)
	}
	if len(failuresRows) < 2 {
		t.Fatalf("failures.csv rows=%d, want at least one failure row", len(failuresRows))
	}
}

func TestBootstrapKeyReuseCSVHelpers(t *testing.T) {
	if got, want := bkrJoinInts([]int{1, 2, 3}), "1;2;3"; got != want {
		t.Fatalf("bkrJoinInts=%q, want %q", got, want)
	}
	if got, want := bkrJoinUint64s([]uint64{5, 7, 11}), "5;7;11"; got != want {
		t.Fatalf("bkrJoinUint64s=%q, want %q", got, want)
	}
	if bkrHashStringList([]string{"b", "a"}) != bkrHashStringList([]string{"a", "b"}) {
		t.Fatal("bkrHashStringList must be order-stable")
	}

	path := filepath.Join(t.TempDir(), "escaping.csv")
	if err := bkrWriteCSVFile(path, []string{"a", "b"}, [][]string{{"x;y", "line\nbreak"}}); err != nil {
		t.Fatal(err)
	}

	rows := bkrReadCSVForTest(t, path)
	if got, want := rows[1], []string{"x;y", "line\nbreak"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("CSV escaping row=%q, want %q", got, want)
	}
}

func bkrSetResultDirForTest(t *testing.T, dir string) {
	t.Helper()
	previous := *bkrResultDir
	*bkrResultDir = dir
	t.Cleanup(func() {
		*bkrResultDir = previous
	})
}

func bkrAssertCSVOnlyOutput(t *testing.T, dir string) {
	t.Helper()

	expected := bkrExpectedCSVHeadersForTest()
	for _, file := range bkrRequiredCSVFiles {
		path := filepath.Join(dir, file)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing required CSV %s: %v", file, err)
		}

		rows := bkrReadCSVForTest(t, path)
		if len(rows) == 0 {
			t.Fatalf("%s has no header", file)
		}
		if !reflect.DeepEqual(rows[0], expected[file]) {
			t.Fatalf("%s header mismatch:\n got %q\nwant %q", file, rows[0], expected[file])
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("unexpected directory in result dir: %s", entry.Name())
		}
		if filepath.Ext(entry.Name()) != ".csv" {
			t.Fatalf("unexpected non-CSV output file: %s", entry.Name())
		}
		if _, ok := expected[entry.Name()]; !ok {
			t.Fatalf("unexpected CSV output file: %s", entry.Name())
		}
	}
}

func bkrExpectedCSVHeadersForTest() map[string][]string {
	return map[string][]string{
		"metadata.csv":                           bkrMetadataCSVHeader,
		"experiment_case.csv":                    bkrExperimentCaseCSVHeader,
		"target_results.csv":                     bkrTargetResultsCSVHeader,
		"material_metrics.csv":                   bkrMaterialMetricsCSVHeader,
		"runtime_metrics.csv":                    bkrRuntimeMetricsCSVHeader,
		"parameter_chain_baseline.csv":           bkrParameterChainBaselineCSVHeader,
		"galois_key_baseline.csv":                bkrGaloisKeyBaselineCSVHeader,
		"linear_transform_schedule_baseline.csv": bkrLinearTransformScheduleBaselineCSVHeader,
		"encoded_diagonal_baseline.csv":          bkrEncodedDiagonalBaselineCSVHeader,
		"material_baseline_index.csv":            bkrMaterialBaselineIndexCSVHeader,
		"rotation_key_pool.csv":                  bkrRotationKeyPoolCSVHeader,
		"rotation_key_view.csv":                  bkrRotationKeyViewCSVHeader,
		"failures.csv":                           bkrFailuresCSVHeader,
		"summary.csv":                            bkrSummaryCSVHeader,
	}
}

func bkrReadCSVForTest(t *testing.T, path string) [][]string {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func bkrCSVValueForTest(t *testing.T, rows [][]string, row int, column string) string {
	t.Helper()

	if len(rows) <= row {
		t.Fatalf("CSV has %d rows, cannot read row %d", len(rows), row)
	}
	for i, header := range rows[0] {
		if header == column {
			if len(rows[row]) <= i {
				t.Fatalf("CSV row %d has %d columns, cannot read %q at index %d", row, len(rows[row]), column, i)
			}
			return rows[row][i]
		}
	}
	t.Fatalf("missing CSV column %q", column)
	return ""
}

func bkrCSVIntForTest(t *testing.T, rows [][]string, row int, column string) int {
	t.Helper()

	value, err := strconv.Atoi(bkrCSVValueForTest(t, rows, row, column))
	if err != nil {
		t.Fatalf("cannot parse CSV column %q as int: %v", column, err)
	}
	return value
}

func BenchmarkBootstrapKeyReuseA0_P2MultiFastClustered(b *testing.B) {
	bkrBenchmarkA0(b, bkrCaseP2MultiFastClustered())
}

func BenchmarkBootstrapKeyReuseA0_P4N15DenseSparse(b *testing.B) {
	bkrBenchmarkA0(b, bkrCaseP4N15DenseSparse())
}

func BenchmarkBootstrapKeyReuseA0_P5N16SparseLongRandom(b *testing.B) {
	if !*flagLongTest {
		b.Skip("long A0 benchmark; rerun with -args -long")
	}
	bkrBenchmarkA0(b, bkrCaseP5N16SparseLongRandom())
}

func bkrBenchmarkA0(b *testing.B, spec bkrCaseSpec) {
	b.Helper()
	b.ReportAllocs()

	prepared := make([]struct {
		eval *Evaluator
		ct   *rlwe.Ciphertext
	}, 0, len(spec.AllTargetLevels))

	for _, targetLevel := range spec.AllTargetLevels {
		params, btpParams, err := buildTargetBootstrappingParameters(spec, targetLevel)
		if err != nil {
			b.Fatal(err)
		}

		sk, err := newDeterministicSecretKey(params, spec.Seed+"/secret-key")
		if err != nil {
			b.Fatal(err)
		}

		keys, _, err := btpParams.GenEvaluationKeys(sk)
		if err != nil {
			b.Fatal(err)
		}

		eval, err := NewEvaluator(btpParams, keys)
		if err != nil {
			b.Fatal(err)
		}

		prepared = append(prepared, struct {
			eval *Evaluator
			ct   *rlwe.Ciphertext
		}{
			eval: eval,
			ct:   ckks.NewCiphertext(params, 1, 0),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, item := range prepared {
			b.StopTimer()
			ct := item.ct.CopyNew()
			b.StartTimer()
			if _, err := item.eval.Bootstrap(ct); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func bkrCaseP0TinyNativeSingle() bkrCaseSpec {
	return buildBootstrapKeyReuseCase(
		"a0_p0_tiny_native_single",
		"P0_TINY_NATIVE",
		ckks.ParametersLiteral{LogN: 10, LogQ: []int{60, 40}, LogP: []int{61}, LogDefaultScale: 40},
		ParametersLiteral{LogN: bkrPointyInt(10)},
		[]int{1},
		func(spec *bkrCaseSpec) {
			spec.LogSlots = 9
			spec.FastInsecureBootstrapping = true
		},
	)
}

func bkrCaseP1TinyRingSwitchSingle() bkrCaseSpec {
	return buildBootstrapKeyReuseCase(
		"a0_p1_tiny_ring_switch_single",
		"P1_TINY_RING_SWITCH",
		ckks.ParametersLiteral{LogN: 9, LogNthRoot: 11, LogQ: []int{60, 40}, LogP: []int{61}, LogDefaultScale: 40},
		ParametersLiteral{LogN: bkrPointyInt(10)},
		[]int{1},
		func(spec *bkrCaseSpec) {
			spec.LogSlots = 9
			spec.RingSwitchMode = "ring_degree_switch"
			spec.FastInsecureBootstrapping = true
		},
	)
}

func bkrP2MultiFastBase(caseID string, targets []int, opts ...func(*bkrCaseSpec)) bkrCaseSpec {
	baseOpts := []func(*bkrCaseSpec){
		func(spec *bkrCaseSpec) {
			spec.LogSlots = 9
			spec.FastInsecureBootstrapping = true
		},
	}
	baseOpts = append(baseOpts, opts...)
	return buildBootstrapKeyReuseCase(
		caseID,
		"P2_MULTI_FAST",
		ckks.ParametersLiteral{LogN: 10, LogQ: []int{60, 40, 40, 40}, LogP: []int{61}, LogDefaultScale: 40},
		ParametersLiteral{LogN: bkrPointyInt(10)},
		targets,
		baseOpts...,
	)
}

func bkrCaseP2MultiFastSingle() bkrCaseSpec {
	return bkrP2MultiFastBase("a0_p2_multi_fast_single", []int{3})
}

func bkrCaseP2MultiFastClustered() bkrCaseSpec {
	return bkrP2MultiFastBase("a0_p2_multi_fast_clustered", []int{1, 2, 3})
}

func bkrCaseP2MultiFastSparse() bkrCaseSpec {
	return bkrP2MultiFastBase("a0_p2_multi_fast_sparse", []int{1, 3})
}

func bkrCaseP2MultiFastRandomFixed() bkrCaseSpec {
	return bkrP2MultiFastBase("a0_p2_multi_fast_random_fixed", []int{1, 2, 3})
}

func bkrCaseP2MultiFastClusteredPacked() bkrCaseSpec {
	return bkrP2MultiFastBase("a0_p2_multi_fast_clustered_packed", []int{1, 2, 3}, func(spec *bkrCaseSpec) {
		spec.PackedMode = true
	})
}

func bkrCaseP3N15SparseSingle() bkrCaseSpec {
	return buildBootstrapKeyReuseCase("a0_p3_n15_sparse_single", "P3_N15_SPARSE", N15QP768H192H32.SchemeParams, bkrN15SparseBootstrappingParams(), []int{2})
}

func bkrCaseP3N15SparseClustered() bkrCaseSpec {
	return buildBootstrapKeyReuseCase("a0_p3_n15_sparse_clustered", "P3_N15_SPARSE", N15QP768H192H32.SchemeParams, bkrN15SparseBootstrappingParams(), []int{1, 2})
}

func bkrN15SparseBootstrappingParams() ParametersLiteral {
	params := bkrCloneBootstrappingParametersLiteral(N15QP768H192H32.BootstrappingParams)
	params.LogN = bkrPointyInt(15)
	return params
}

func bkrCaseP4N15DenseSingle() bkrCaseSpec {
	return buildBootstrapKeyReuseCase("a0_p4_n15_dense_single", "P4_N15_DENSE", N15QP880H16384H32.SchemeParams, N15QP880H16384H32.BootstrappingParams, []int{4})
}

func bkrCaseP4N15DenseClustered() bkrCaseSpec {
	return buildBootstrapKeyReuseCase("a0_p4_n15_dense_clustered", "P4_N15_DENSE", N15QP880H16384H32.SchemeParams, N15QP880H16384H32.BootstrappingParams, []int{2, 3, 4})
}

func bkrCaseP4N15DenseSparse() bkrCaseSpec {
	return buildBootstrapKeyReuseCase("a0_p4_n15_dense_sparse", "P4_N15_DENSE", N15QP880H16384H32.SchemeParams, N15QP880H16384H32.BootstrappingParams, []int{1, 4})
}

func bkrCaseP4N15DenseRandom() bkrCaseSpec {
	return buildBootstrapKeyReuseCase("a0_p4_n15_dense_random", "P4_N15_DENSE", N15QP880H16384H32.SchemeParams, N15QP880H16384H32.BootstrappingParams, []int{1, 3, 4})
}

func bkrCaseP5N16SparseLongSingle() bkrCaseSpec {
	return bkrLongCase("a0_p5_n16_sparse_long_single", "P5_N16_SPARSE_LONG", N16QP1546H192H32, []int{9})
}

func bkrCaseP5N16SparseLongClustered() bkrCaseSpec {
	return bkrLongCase("a0_p5_n16_sparse_long_clustered", "P5_N16_SPARSE_LONG", N16QP1546H192H32, []int{7, 8, 9})
}

func bkrCaseP5N16SparseLongSparse() bkrCaseSpec {
	return bkrLongCase("a0_p5_n16_sparse_long_sparse", "P5_N16_SPARSE_LONG", N16QP1546H192H32, []int{1, 5, 9})
}

func bkrCaseP5N16SparseLongRandom() bkrCaseSpec {
	return bkrLongCase("a0_p5_n16_sparse_long_random", "P5_N16_SPARSE_LONG", N16QP1546H192H32, []int{2, 4, 6, 8})
}

func bkrCaseP6N16DenseLongSingle() bkrCaseSpec {
	return bkrLongCase("a0_p6_n16_dense_long_single", "P6_N16_DENSE_LONG", N16QP1767H32768H32, []int{13})
}

func bkrCaseP6N16DenseLongClustered() bkrCaseSpec {
	return bkrLongCase("a0_p6_n16_dense_long_clustered", "P6_N16_DENSE_LONG", N16QP1767H32768H32, []int{11, 12, 13})
}

func bkrCaseP6N16DenseLongSparse() bkrCaseSpec {
	return bkrLongCase("a0_p6_n16_dense_long_sparse", "P6_N16_DENSE_LONG", N16QP1767H32768H32, []int{1, 7, 13})
}

func bkrCaseP6N16DenseLongRandom() bkrCaseSpec {
	return bkrLongCase("a0_p6_n16_dense_long_random", "P6_N16_DENSE_LONG", N16QP1767H32768H32, []int{2, 5, 9, 12})
}

func bkrLongCase(caseID, profileID string, params defaultParametersLiteral, targets []int) bkrCaseSpec {
	return buildBootstrapKeyReuseCase(caseID, profileID, params.SchemeParams, params.BootstrappingParams, targets, func(spec *bkrCaseSpec) {
		spec.LongOnly = true
	})
}

func bkrCaseInvalidTarget() bkrCaseSpec {
	return bkrP2MultiFastBase("a0_invalid_target_rejects_before_keygen", []int{4})
}

func bkrCaseOutputLevelMismatch() bkrCaseSpec {
	expected := 0
	return bkrP2MultiFastBase("a0_output_level_mismatch_fails", []int{1}, func(spec *bkrCaseSpec) {
		spec.ExpectedOutputLevelOverride = &expected
	})
}

func bkrCaseOutputScaleMismatch() bkrCaseSpec {
	expectedScaleLog := 41
	return bkrP2MultiFastBase("a0_output_scale_mismatch_fails", []int{1}, func(spec *bkrCaseSpec) {
		spec.ExpectedScaleLogOverride = &expectedScaleLog
	})
}
