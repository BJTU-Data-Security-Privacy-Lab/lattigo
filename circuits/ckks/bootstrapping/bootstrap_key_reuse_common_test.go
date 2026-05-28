package bootstrapping

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tuneinsight/lattigo/v6/core/rlwe"
	"github.com/tuneinsight/lattigo/v6/ring"
	"github.com/tuneinsight/lattigo/v6/schemes/ckks"
	"github.com/tuneinsight/lattigo/v6/utils/sampling"
)

var bkrResultDir = flag.String("bkr.result-dir", "", "directory for bootstrap key reuse structured test outputs")

const (
	bkrSchemaVersion    = "bootstrap-key-reuse-test/v1"
	bkrLaneA0           = "a0"
	bkrPlanA0           = "A0_FullKeyPerTargetLevel"
	bkrDefaultSeed      = "bootstrap-key-reuse-a0-2026-05-28"
	bkrMinPrecisionBits = 12.0
	bkrPackedCiphertext = 4
)

type bkrCaseSpec struct {
	CaseID                      string
	ProfileID                   string
	SchemeParams                ckks.ParametersLiteral
	BootstrappingParams         ParametersLiteral
	AllTargetLevels             []int
	UsedTargetLevels            []int
	FixedTargetScaleLog         int
	LogSlots                    int
	RingSwitchMode              string
	PackedMode                  bool
	Seed                        string
	RepeatCount                 int
	MinPrecisionBits            float64
	LongOnly                    bool
	FastInsecureBootstrapping   bool
	ExpectedOutputLevelOverride *int
	ExpectedScaleLogOverride    *int
}

type bkrExperimentCase struct {
	RecordType          string `json:"record_type"`
	SchemaVersion       string `json:"schema_version"`
	PlanID              string `json:"plan_id"`
	CaseID              string `json:"case_id"`
	ParamsProfile       string `json:"params_profile"`
	AllTargetLevels     []int  `json:"all_target_levels"`
	UsedTargetLevels    []int  `json:"used_target_levels"`
	FixedTargetScaleLog int    `json:"fixed_target_scale_log"`
	LogSlots            int    `json:"log_slots"`
	RingSwitchMode      string `json:"ring_switch_mode"`
	PackedMode          bool   `json:"packed_mode"`
	Seed                string `json:"seed"`
	RepeatCount         int    `json:"repeat_count"`
}

type bkrTargetRunResult struct {
	RecordType              string  `json:"record_type"`
	SchemaVersion           string  `json:"schema_version"`
	PlanID                  string  `json:"plan_id"`
	CaseID                  string  `json:"case_id"`
	ParamsProfile           string  `json:"params_profile"`
	TargetLevel             int     `json:"target_level"`
	OutputLevel             int     `json:"output_level"`
	ExpectedOutputLevel     int     `json:"expected_output_level"`
	OutputLevelEquality     bool    `json:"output_level_equality"`
	OutputScaleLog2         float64 `json:"output_scale_log2"`
	ExpectedOutputScaleLog2 float64 `json:"expected_output_scale_log2"`
	OutputScaleEquality     bool    `json:"output_scale_equality"`
	AverageLog2PrecReal     float64 `json:"average_log2_precision_real"`
	AverageLog2PrecImag     float64 `json:"average_log2_precision_imag"`
	PrecisionThresholdBits  float64 `json:"precision_threshold_bits"`
	BootstrapErrorStatus    string  `json:"bootstrap_error_status"`
	PackedCiphertexts       int     `json:"packed_ciphertexts"`
}

type bkrMaterialMetrics struct {
	RecordType                    string `json:"record_type"`
	SchemaVersion                 string `json:"schema_version"`
	PlanID                        string `json:"plan_id"`
	CaseID                        string `json:"case_id"`
	ParamsProfile                 string `json:"params_profile"`
	TargetLevel                   int    `json:"target_level"`
	GeneratedEvaluationKeyCount   int    `json:"generated_evaluation_key_count"`
	GeneratedRotationKeyCount     int    `json:"generated_rotation_key_count"`
	GeneratedEncodedDiagonalCount int    `json:"generated_encoded_diagonal_count"`
	PersistentKeyBytes            int64  `json:"persistent_key_bytes"`
	PersistentMatrixBytes         int64  `json:"persistent_matrix_bytes"`
	SharedRotationKeys            int    `json:"shared_rotation_keys"`
	SharedEncodedDiagonals        int    `json:"shared_encoded_diagonals"`
	RNSSliceSuccess               string `json:"rns_slice_success"`
	FallbackReason                string `json:"fallback_reason"`
}

type bkrRuntimeMetrics struct {
	RecordType                    string  `json:"record_type"`
	SchemaVersion                 string  `json:"schema_version"`
	PlanID                        string  `json:"plan_id"`
	CaseID                        string  `json:"case_id"`
	ParamsProfile                 string  `json:"params_profile"`
	TargetLevel                   int     `json:"target_level"`
	KeygenTimeMS                  float64 `json:"keygen_time_ms"`
	EvaluatorMatrixConstructionMS float64 `json:"evaluator_matrix_construction_time_ms"`
	BootstrapLatencyMS            float64 `json:"bootstrap_latency_ms"`
	PeakKeygenHeapBytes           uint64  `json:"peak_keygen_heap_bytes"`
	PeakRuntimeHeapBytes          uint64  `json:"peak_runtime_heap_bytes"`
}

type bkrAblationComparison struct {
	RecordType              string  `json:"record_type"`
	SchemaVersion           string  `json:"schema_version"`
	PlanID                  string  `json:"plan_id"`
	CaseID                  string  `json:"case_id"`
	ComparedPlanID          string  `json:"compared_plan_id"`
	TargetLevel             int     `json:"target_level"`
	KeyBytesRatioToA0       float64 `json:"key_bytes_ratio_to_a0"`
	MatrixBytesRatioToA0    float64 `json:"matrix_bytes_ratio_to_a0"`
	BootstrapLatencyRatioA0 float64 `json:"bootstrap_latency_ratio_to_a0"`
}

type bkrFailure struct {
	RecordType       string `json:"record_type"`
	SchemaVersion    string `json:"schema_version"`
	PlanID           string `json:"plan_id"`
	CaseID           string `json:"case_id"`
	ParamsProfile    string `json:"params_profile"`
	TargetLevel      int    `json:"target_level"`
	Stage            string `json:"stage"`
	Message          string `json:"message"`
	BeforeKeygen     bool   `json:"before_keygen"`
	GeneratedKeygen  bool   `json:"generated_keygen"`
	GeneratedRuntime bool   `json:"generated_runtime"`
}

type bkrRunSummary struct {
	SchemaVersion      string       `json:"schema_version"`
	Lane               string       `json:"lane"`
	PlanID             string       `json:"plan_id"`
	CaseID             string       `json:"case_id"`
	ParamsProfile      string       `json:"params_profile"`
	Status             string       `json:"status"`
	TargetLevels       []int        `json:"target_levels"`
	ResultDirectory    string       `json:"result_directory"`
	StartedAt          string       `json:"started_at"`
	FinishedAt         string       `json:"finished_at"`
	ElapsedMS          float64      `json:"elapsed_ms"`
	TotalTargets       int          `json:"total_targets"`
	SuccessfulTargets  int          `json:"successful_targets"`
	FailedTargets      int          `json:"failed_targets"`
	Passed             bool         `json:"passed"`
	FailureCount       int          `json:"failure_count"`
	Failures           []bkrFailure `json:"failures,omitempty"`
	SharedRotationKeys int          `json:"shared_rotation_keys"`
	SharedDiagonals    int          `json:"shared_encoded_diagonals"`
	RNSSliceSuccess    string       `json:"rns_slice_success"`
	FallbackReason     string       `json:"fallback_reason"`
}

type bkrMetadata struct {
	SchemaVersion string            `json:"schema_version"`
	Lane          string            `json:"lane"`
	PlanID        string            `json:"plan_id"`
	CaseID        string            `json:"case_id"`
	ParamsProfile string            `json:"params_profile"`
	Command       string            `json:"command"`
	GoVersion     string            `json:"go_version"`
	GoEnv         map[string]string `json:"go_env"`
	GOOS          string            `json:"goos"`
	GOARCH        string            `json:"goarch"`
	NumCPU        int               `json:"num_cpu"`
	GOMAXPROCS    int               `json:"gomaxprocs"`
	VCSRevision   string            `json:"vcs_revision"`
	VCSTime       string            `json:"vcs_time"`
	VCSModified   string            `json:"vcs_modified"`
	Branch        string            `json:"branch"`
	Commit        string            `json:"commit"`
	DirtyState    string            `json:"dirty_state"`
	StartedAt     string            `json:"started_at"`
}

type bkrCSVRow struct {
	Status                      string
	CaseID                      string
	ParamsProfile               string
	TargetLevel                 int
	OutputLevel                 int
	OutputScaleEquality         bool
	AverageLog2PrecisionReal    float64
	AverageLog2PrecisionImag    float64
	GeneratedEvaluationKeyCount int
	GeneratedRotationKeyCount   int
	GeneratedEncodedDiagonals   int
	PersistentKeyBytes          int64
	PersistentMatrixBytes       int64
	KeygenTimeMS                float64
	ConstructionTimeMS          float64
	BootstrapLatencyMS          float64
	PeakKeygenHeapBytes         uint64
	PeakRuntimeHeapBytes        uint64
	SharedRotationKeys          int
	SharedEncodedDiagonals      int
	RNSSliceSuccess             string
	FallbackReason              string
}

type bkrRecorder struct {
	dir      string
	spec     bkrCaseSpec
	start    time.Time
	results  *os.File
	csvRows  []bkrCSVRow
	failures []bkrFailure
}

func buildBootstrapKeyReuseCase(caseID, profileID string, schemeParams ckks.ParametersLiteral, btpParams ParametersLiteral, allTargetLevels []int, opts ...func(*bkrCaseSpec)) bkrCaseSpec {
	spec := bkrCaseSpec{
		CaseID:              caseID,
		ProfileID:           profileID,
		SchemeParams:        bkrCloneCKKSParametersLiteral(schemeParams),
		BootstrappingParams: bkrCloneBootstrappingParametersLiteral(btpParams),
		AllTargetLevels:     append([]int(nil), allTargetLevels...),
		UsedTargetLevels:    append([]int(nil), allTargetLevels...),
		FixedTargetScaleLog: schemeParams.LogDefaultScale,
		LogSlots:            -1,
		RingSwitchMode:      "none",
		Seed:                bkrDefaultSeed + "/" + caseID,
		RepeatCount:         1,
		MinPrecisionBits:    bkrMinPrecisionBits,
	}

	for _, opt := range opts {
		opt(&spec)
	}

	if spec.FixedTargetScaleLog == 0 {
		spec.FixedTargetScaleLog = spec.SchemeParams.LogDefaultScale
	}
	if spec.MinPrecisionBits == 0 {
		spec.MinPrecisionBits = bkrMinPrecisionBits
	}
	if spec.Seed == "" {
		spec.Seed = bkrDefaultSeed + "/" + caseID
	}
	if spec.UsedTargetLevels == nil {
		spec.UsedTargetLevels = append([]int(nil), spec.AllTargetLevels...)
	}

	return spec
}

func buildTargetResidualLiteral(spec bkrCaseSpec, targetLevel int) (ckks.ParametersLiteral, error) {
	fullParams, err := ckks.NewParametersFromLiteral(spec.SchemeParams)
	if err != nil {
		return ckks.ParametersLiteral{}, fmt.Errorf("cannot instantiate full profile parameters: %w", err)
	}

	if targetLevel < 0 || targetLevel > fullParams.MaxLevel() {
		return ckks.ParametersLiteral{}, fmt.Errorf("invalid target level %d for profile max level %d", targetLevel, fullParams.MaxLevel())
	}

	lit := fullParams.ParametersLiteral()
	q := fullParams.Q()
	p := fullParams.P()

	lit.Q = append([]uint64(nil), q[:targetLevel+1]...)
	lit.P = append([]uint64(nil), p...)
	lit.LogQ = nil
	lit.LogP = nil
	lit.LogDefaultScale = spec.FixedTargetScaleLog

	return lit, nil
}

func buildTargetBootstrappingParameters(spec bkrCaseSpec, targetLevel int) (ckks.Parameters, Parameters, error) {
	residualLiteral, err := buildTargetResidualLiteral(spec, targetLevel)
	if err != nil {
		return ckks.Parameters{}, Parameters{}, err
	}

	residualParams, err := ckks.NewParametersFromLiteral(residualLiteral)
	if err != nil {
		return ckks.Parameters{}, Parameters{}, fmt.Errorf("cannot instantiate target residual parameters: %w", err)
	}

	btpLiteral := bkrCloneBootstrappingParametersLiteral(spec.BootstrappingParams)
	if spec.LogSlots >= 0 {
		btpLiteral.LogSlots = bkrPointyInt(spec.LogSlots)
	}

	btpParams, err := NewParametersFromLiteral(residualParams, btpLiteral)
	if err != nil {
		return ckks.Parameters{}, Parameters{}, fmt.Errorf("cannot instantiate target bootstrapping parameters: %w", err)
	}

	if spec.FastInsecureBootstrapping {
		logSlots := btpParams.BootstrappingParameters.LogN() - 1
		btpParams.SlotsToCoeffsParameters.LogSlots = logSlots
		btpParams.CoeffsToSlotsParameters.LogSlots = logSlots
		if delta := 16 - residualParams.LogN(); delta > 0 {
			btpParams.Mod1ParametersLiteral.LogMessageRatio += delta
		}
	}

	return residualParams, btpParams, nil
}

func newDeterministicSecretKey(params ckks.Parameters, seed string) (*rlwe.SecretKey, error) {
	key := sha256.Sum256([]byte(seed))
	prng, err := sampling.NewKeyedPRNG(key[:])
	if err != nil {
		return nil, err
	}

	sampler, err := ring.NewSampler(prng, params.RingQ(), params.Xs(), false)
	if err != nil {
		return nil, err
	}

	sk := rlwe.NewSecretKey(params)
	ringQP := params.RingQP().AtLevel(sk.LevelQ(), sk.LevelP())

	sampler.AtLevel(sk.LevelQ()).Read(sk.Value.Q)
	if levelP := sk.LevelP(); levelP > -1 {
		ringQP.ExtendBasisSmallNormAndCenter(sk.Value.Q, levelP, sk.Value.Q, sk.Value.P)
	}

	ringQP.NTT(sk.Value, sk.Value)
	ringQP.MForm(sk.Value, sk.Value)

	return sk, nil
}

func newBootstrapKeyReuseRecorder(t testing.TB, spec bkrCaseSpec) (*bkrRecorder, error) {
	t.Helper()

	if strings.TrimSpace(*bkrResultDir) == "" {
		return nil, errors.New("missing -bkr.result-dir; A0 structured tests require an explicit result directory")
	}

	dir, err := bkrResolveResultDir(*bkrResultDir)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create result directory %q: %w", dir, err)
	}

	results, err := os.Create(filepath.Join(dir, "results.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("cannot create results.jsonl: %w", err)
	}

	rec := &bkrRecorder{
		dir:     dir,
		spec:    spec,
		start:   time.Now(),
		results: results,
	}

	if err := rec.writeMetadata(); err != nil {
		results.Close()
		return nil, err
	}

	return rec, nil
}

func (r *bkrRecorder) close() error {
	if r.results != nil {
		return r.results.Close()
	}
	return nil
}

func (r *bkrRecorder) writeRecord(record interface{}) error {
	enc := json.NewEncoder(r.results)
	return enc.Encode(record)
}

func writeBootstrapKeyReuseResult(rec *bkrRecorder, result bkrTargetRunResult, material bkrMaterialMetrics, runtimeMetrics bkrRuntimeMetrics) error {
	if err := rec.writeRecord(result); err != nil {
		return err
	}
	if err := rec.writeRecord(material); err != nil {
		return err
	}
	if err := rec.writeRecord(runtimeMetrics); err != nil {
		return err
	}

	rec.csvRows = append(rec.csvRows, bkrCSVRow{
		Status:                      bkrTargetStatus(result),
		CaseID:                      result.CaseID,
		ParamsProfile:               result.ParamsProfile,
		TargetLevel:                 result.TargetLevel,
		OutputLevel:                 result.OutputLevel,
		OutputScaleEquality:         result.OutputScaleEquality,
		AverageLog2PrecisionReal:    result.AverageLog2PrecReal,
		AverageLog2PrecisionImag:    result.AverageLog2PrecImag,
		GeneratedEvaluationKeyCount: material.GeneratedEvaluationKeyCount,
		GeneratedRotationKeyCount:   material.GeneratedRotationKeyCount,
		GeneratedEncodedDiagonals:   material.GeneratedEncodedDiagonalCount,
		PersistentKeyBytes:          material.PersistentKeyBytes,
		PersistentMatrixBytes:       material.PersistentMatrixBytes,
		KeygenTimeMS:                runtimeMetrics.KeygenTimeMS,
		ConstructionTimeMS:          runtimeMetrics.EvaluatorMatrixConstructionMS,
		BootstrapLatencyMS:          runtimeMetrics.BootstrapLatencyMS,
		PeakKeygenHeapBytes:         runtimeMetrics.PeakKeygenHeapBytes,
		PeakRuntimeHeapBytes:        runtimeMetrics.PeakRuntimeHeapBytes,
		SharedRotationKeys:          material.SharedRotationKeys,
		SharedEncodedDiagonals:      material.SharedEncodedDiagonals,
		RNSSliceSuccess:             material.RNSSliceSuccess,
		FallbackReason:              material.FallbackReason,
	})

	return nil
}

func (r *bkrRecorder) writeExperimentCase() error {
	spec := r.spec
	return r.writeRecord(bkrExperimentCase{
		RecordType:          "ExperimentCase",
		SchemaVersion:       bkrSchemaVersion,
		PlanID:              bkrPlanA0,
		CaseID:              spec.CaseID,
		ParamsProfile:       spec.ProfileID,
		AllTargetLevels:     append([]int(nil), spec.AllTargetLevels...),
		UsedTargetLevels:    append([]int(nil), spec.UsedTargetLevels...),
		FixedTargetScaleLog: spec.FixedTargetScaleLog,
		LogSlots:            spec.LogSlots,
		RingSwitchMode:      spec.RingSwitchMode,
		PackedMode:          spec.PackedMode,
		Seed:                spec.Seed,
		RepeatCount:         spec.RepeatCount,
	})
}

func (r *bkrRecorder) writeFailure(targetLevel int, stage, message string, beforeKeygen, generatedKeygen, generatedRuntime bool) error {
	failure := bkrFailure{
		RecordType:       "Failure",
		SchemaVersion:    bkrSchemaVersion,
		PlanID:           bkrPlanA0,
		CaseID:           r.spec.CaseID,
		ParamsProfile:    r.spec.ProfileID,
		TargetLevel:      targetLevel,
		Stage:            stage,
		Message:          message,
		BeforeKeygen:     beforeKeygen,
		GeneratedKeygen:  generatedKeygen,
		GeneratedRuntime: generatedRuntime,
	}
	r.failures = append(r.failures, failure)
	return r.writeRecord(failure)
}

func (r *bkrRecorder) writeSummary(successfulTargets int) (bkrRunSummary, error) {
	finished := time.Now()
	summary := bkrRunSummary{
		SchemaVersion:      bkrSchemaVersion,
		Lane:               bkrLaneA0,
		PlanID:             bkrPlanA0,
		CaseID:             r.spec.CaseID,
		ParamsProfile:      r.spec.ProfileID,
		Status:             "fail",
		TargetLevels:       append([]int(nil), r.spec.AllTargetLevels...),
		ResultDirectory:    r.dir,
		StartedAt:          r.start.Format(time.RFC3339Nano),
		FinishedAt:         finished.Format(time.RFC3339Nano),
		ElapsedMS:          bkrDurationMS(finished.Sub(r.start)),
		TotalTargets:       len(r.spec.AllTargetLevels),
		SuccessfulTargets:  successfulTargets,
		FailedTargets:      len(r.spec.AllTargetLevels) - successfulTargets,
		Passed:             len(r.failures) == 0 && successfulTargets == len(r.spec.AllTargetLevels),
		FailureCount:       len(r.failures),
		Failures:           append([]bkrFailure(nil), r.failures...),
		SharedRotationKeys: 0,
		SharedDiagonals:    0,
		RNSSliceSuccess:    "not_applicable",
		FallbackReason:     "none",
	}
	if summary.Passed {
		summary.Status = "pass"
	}

	if err := bkrWriteIndentedJSON(filepath.Join(r.dir, "summary.json"), summary); err != nil {
		return summary, err
	}
	if err := r.writeMetricsCSV(); err != nil {
		return summary, err
	}

	return summary, nil
}

func (r *bkrRecorder) writeMetadata() error {
	metadata := bkrMetadata{
		SchemaVersion: bkrSchemaVersion,
		Lane:          bkrLaneA0,
		PlanID:        bkrPlanA0,
		CaseID:        r.spec.CaseID,
		ParamsProfile: r.spec.ProfileID,
		Command:       strings.Join(os.Args, " "),
		GoVersion:     runtime.Version(),
		GoEnv:         bkrGoEnv(),
		GOOS:          runtime.GOOS,
		GOARCH:        runtime.GOARCH,
		NumCPU:        runtime.NumCPU(),
		GOMAXPROCS:    runtime.GOMAXPROCS(0),
		Branch:        os.Getenv("BKR_BRANCH"),
		Commit:        os.Getenv("BKR_COMMIT"),
		DirtyState:    os.Getenv("BKR_DIRTY"),
		StartedAt:     r.start.Format(time.RFC3339Nano),
	}

	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range bi.Settings {
			switch setting.Key {
			case "vcs.revision":
				metadata.VCSRevision = setting.Value
				if metadata.Commit == "" {
					metadata.Commit = setting.Value
				}
			case "vcs.time":
				metadata.VCSTime = setting.Value
			case "vcs.modified":
				metadata.VCSModified = setting.Value
				if metadata.DirtyState == "" {
					metadata.DirtyState = setting.Value
				}
			}
		}
	}

	branch, commit, dirty := bkrGitMetadata()
	if metadata.Branch == "" {
		metadata.Branch = branch
	}
	if metadata.Commit == "" {
		metadata.Commit = commit
	}
	if metadata.DirtyState == "" {
		metadata.DirtyState = dirty
	}

	return bkrWriteIndentedJSON(filepath.Join(r.dir, "metadata.json"), metadata)
}

func (r *bkrRecorder) writeMetricsCSV() error {
	file, err := os.Create(filepath.Join(r.dir, "metrics.csv"))
	if err != nil {
		return fmt.Errorf("cannot create metrics.csv: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{
		"schema_version",
		"lane",
		"case_id",
		"params_profile",
		"target_level",
		"status",
		"output_level",
		"scale_ok",
		"average_log2_precision_real",
		"average_log2_precision_imag",
		"generated_evaluation_key_count",
		"generated_rotation_key_count",
		"generated_encoded_diagonal_count",
		"persistent_key_bytes",
		"persistent_matrix_bytes",
		"keygen_time_ms",
		"evaluator_matrix_construction_time_ms",
		"bootstrap_latency_ms",
		"peak_keygen_heap_bytes",
		"peak_runtime_heap_bytes",
		"shared_rotation_keys",
		"shared_encoded_diagonals",
		"rns_slice_success",
		"fallback_reason",
	}

	if err := writer.Write(header); err != nil {
		return err
	}

	for _, row := range r.csvRows {
		if err := writer.Write([]string{
			bkrSchemaVersion,
			bkrLaneA0,
			row.CaseID,
			row.ParamsProfile,
			strconv.Itoa(row.TargetLevel),
			row.Status,
			strconv.Itoa(row.OutputLevel),
			strconv.FormatBool(row.OutputScaleEquality),
			bkrFormatFloat(row.AverageLog2PrecisionReal),
			bkrFormatFloat(row.AverageLog2PrecisionImag),
			strconv.Itoa(row.GeneratedEvaluationKeyCount),
			strconv.Itoa(row.GeneratedRotationKeyCount),
			strconv.Itoa(row.GeneratedEncodedDiagonals),
			strconv.FormatInt(row.PersistentKeyBytes, 10),
			strconv.FormatInt(row.PersistentMatrixBytes, 10),
			bkrFormatFloat(row.KeygenTimeMS),
			bkrFormatFloat(row.ConstructionTimeMS),
			bkrFormatFloat(row.BootstrapLatencyMS),
			strconv.FormatUint(row.PeakKeygenHeapBytes, 10),
			strconv.FormatUint(row.PeakRuntimeHeapBytes, 10),
			strconv.Itoa(row.SharedRotationKeys),
			strconv.Itoa(row.SharedEncodedDiagonals),
			row.RNSSliceSuccess,
			row.FallbackReason,
		}); err != nil {
			return err
		}
	}

	return writer.Error()
}

func bkrWriteIndentedJSON(path string, value interface{}) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func bkrResolveResultDir(path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}

	root, err := bkrRepoRoot()
	if err != nil {
		return "", err
	}

	return filepath.Clean(filepath.Join(root, path)), nil
}

func bkrRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("cannot find repo root from %q", wd)
		}
		dir = parent
	}
}

func bkrPointyInt(v int) *int {
	x := v
	return &x
}

func bkrDurationMS(d time.Duration) float64 {
	return float64(d.Nanoseconds()) / 1e6
}

func bkrFormatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 6, 64)
}

func bkrGitMetadata() (branch, commit, dirty string) {
	root, err := bkrRepoRoot()
	if err != nil {
		return "", "", "unknown"
	}

	branch = bkrGitOutput(root, "branch", "--show-current")
	commit = bkrGitOutput(root, "rev-parse", "HEAD")
	status := bkrGitOutput(root, "status", "--short")
	if status == "" {
		dirty = "false"
	} else {
		dirty = "true"
	}

	return branch, commit, dirty
}

func bkrGitOutput(root string, args ...string) string {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func bkrGoEnv() map[string]string {
	root, _ := bkrRepoRoot()
	return map[string]string{
		"GOVERSION":   runtime.Version(),
		"GOOS":        runtime.GOOS,
		"GOARCH":      runtime.GOARCH,
		"GOMOD":       filepath.Join(root, "go.mod"),
		"GOROOT":      runtime.GOROOT(),
		"GOPATH":      os.Getenv("GOPATH"),
		"GOCACHE":     os.Getenv("GOCACHE"),
		"GOPROXY":     os.Getenv("GOPROXY"),
		"GOSUMDB":     os.Getenv("GOSUMDB"),
		"GOTOOLCHAIN": os.Getenv("GOTOOLCHAIN"),
		"GOMAXPROCS":  strconv.Itoa(runtime.GOMAXPROCS(0)),
	}
}

func bkrTargetStatus(result bkrTargetRunResult) string {
	if result.BootstrapErrorStatus != "ok" || !result.OutputLevelEquality || !result.OutputScaleEquality ||
		result.AverageLog2PrecReal < result.PrecisionThresholdBits || result.AverageLog2PrecImag < result.PrecisionThresholdBits {
		return "fail"
	}
	return "pass"
}

func bkrCloneCKKSParametersLiteral(in ckks.ParametersLiteral) ckks.ParametersLiteral {
	out := in
	out.Q = append([]uint64(nil), in.Q...)
	out.P = append([]uint64(nil), in.P...)
	out.LogQ = append([]int(nil), in.LogQ...)
	out.LogP = append([]int(nil), in.LogP...)
	return out
}

func bkrCloneBootstrappingParametersLiteral(in ParametersLiteral) ParametersLiteral {
	out := in
	out.LogN = bkrCloneIntPtr(in.LogN)
	out.LogSlots = bkrCloneIntPtr(in.LogSlots)
	out.EvalModLogScale = bkrCloneIntPtr(in.EvalModLogScale)
	out.EphemeralSecretWeight = bkrCloneIntPtr(in.EphemeralSecretWeight)
	out.LogMessageRatio = bkrCloneIntPtr(in.LogMessageRatio)
	out.K = bkrCloneIntPtr(in.K)
	out.Mod1Degree = bkrCloneIntPtr(in.Mod1Degree)
	out.DoubleAngle = bkrCloneIntPtr(in.DoubleAngle)
	out.Mod1InvDegree = bkrCloneIntPtr(in.Mod1InvDegree)
	out.LogP = append([]int(nil), in.LogP...)
	out.CoeffsToSlotsFactorizationDepthAndLogScales = bkrClone2DInt(in.CoeffsToSlotsFactorizationDepthAndLogScales)
	out.SlotsToCoeffsFactorizationDepthAndLogScales = bkrClone2DInt(in.SlotsToCoeffsFactorizationDepthAndLogScales)
	if in.IterationsParameters != nil {
		out.IterationsParameters = &IterationsParameters{
			BootstrappingPrecision: append([]float64(nil), in.IterationsParameters.BootstrappingPrecision...),
			ReservedPrimeBitSize:   in.IterationsParameters.ReservedPrimeBitSize,
		}
	}
	return out
}

func bkrCloneIntPtr(in *int) *int {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func bkrClone2DInt(in [][]int) [][]int {
	if in == nil {
		return nil
	}
	out := make([][]int, len(in))
	for i := range in {
		out[i] = append([]int(nil), in[i]...)
	}
	return out
}

func bkrComplexValues(params ckks.Parameters, seed string) []complex128 {
	values := make([]complex128, params.MaxSlots())
	rng := rand.New(rand.NewSource(bkrSeedInt64(seed)))

	for i := range values {
		values[i] = complex(2*rng.Float64()-1, 2*rng.Float64()-1)
	}

	if len(values) > 0 {
		values[0] = complex(0.9238795325112867, 0.3826834323650898)
	}
	if len(values) > 1 {
		values[1] = complex(0.9238795325112867, 0.3826834323650898)
	}
	if len(values) > 2 {
		values[2] = complex(0.9238795325112867, 0.3826834323650898)
	}
	if len(values) > 3 {
		values[3] = complex(0.9238795325112867, 0.3826834323650898)
	}

	return values
}

func bkrSeedInt64(seed string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(seed))
	return int64(h.Sum64() & (1<<63 - 1))
}
