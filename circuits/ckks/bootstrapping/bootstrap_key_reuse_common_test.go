package bootstrapping

import (
	"crypto/sha256"
	"encoding/csv"
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
	"sort"
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
	RotationSetHash               string `json:"rotation_set_hash"`
	C2SScheduleHash               string `json:"c2s_schedule_hash"`
	S2CScheduleHash               string `json:"s2c_schedule_hash"`
	EncodedDiagonalSetHash        string `json:"encoded_diagonal_set_hash"`
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

type bkrParameterChainBaseline struct {
	RecordType                    string
	SchemaVersion                 string
	PlanID                        string
	CaseID                        string
	ParamsProfile                 string
	TargetLevel                   int
	FullLogN                      int
	FullMaxLevel                  int
	FullQCount                    int
	FullPCount                    int
	FullQHash                     string
	FullPHash                     string
	ResidualLogN                  int
	ResidualMaxLevel              int
	ResidualQCount                int
	ResidualPCount                int
	ResidualQPrefixHash           string
	ResidualPHash                 string
	BootstrappingLogN             int
	BootstrappingMaxLevel         int
	BootstrappingQCount           int
	BootstrappingPCount           int
	BootstrappingQHash            string
	BootstrappingPHash            string
	FixedTargetScaleLog           int
	ResidualDefaultScaleLog2      float64
	BootstrappingDefaultScaleLog2 float64
}

type bkrGaloisKeyBaseline struct {
	RecordType                          string
	SchemaVersion                       string
	PlanID                              string
	CaseID                              string
	ParamsProfile                       string
	TargetLevel                         int
	GeneratedGaloisElementsSorted       []uint64
	RequiredBootstrappingGaloisElements []uint64
	CanonicalRotationIDs                []string
	DiscreteLogK                        []int
	ContainsComplexConjugation          bool
	GeneratedCount                      int
	RequiredCount                       int
	MissingRequiredGaloisElements       []uint64
	ExtraGeneratedGaloisElements        []uint64
	RotationSetHash                     string
}

type bkrLinearTransformScheduleBaseline struct {
	RecordType          string
	SchemaVersion       string
	PlanID              string
	CaseID              string
	ParamsProfile       string
	TargetLevel         int
	MatrixName          string
	TransformIndex      int
	DFTType             string
	DFTTypeValue        int
	DFTFormat           string
	DFTFormatValue      int
	DFTLogSlots         int
	DFTLevelQ           int
	DFTLevelP           int
	DFTLevels           []int
	DFTBitReversed      bool
	DFTLogBSGSRatio     int
	LTLevelQ            int
	LTLevelP            int
	LTN1                int
	LTLogRows           int
	LTLogCols           int
	LTScale             string
	LTScaleLog2         float64
	DiagonalIndices     []int
	GaloisElements      []uint64
	TransformScheduleID string
	MatrixScheduleID    string
}

type bkrEncodedDiagonalBaseline struct {
	RecordType        string
	SchemaVersion     string
	PlanID            string
	CaseID            string
	ParamsProfile     string
	TargetLevel       int
	MatrixName        string
	TransformIndex    int
	DiagonalIndex     int
	LevelQ            int
	LevelP            int
	N                 int
	QPrefixHash       string
	PPrefixHash       string
	PolyBinarySize    int64
	PolySHA256        string
	EncodedDiagonalID string
}

type bkrMaterialBaselineIndex struct {
	RecordType                           string
	SchemaVersion                        string
	PlanID                               string
	CaseID                               string
	ParamsProfile                        string
	TargetLevel                          int
	RotationSetHash                      string
	C2SScheduleHash                      string
	S2CScheduleHash                      string
	EncodedDiagonalSetHash               string
	GeneratedRotationKeyCount            int
	ScheduleRecordCount                  int
	EncodedDiagonalRecordCount           int
	DistinctEncodedDiagonalCount         int
	MetricsGeneratedRotationKeyCount     int
	MetricsGeneratedEncodedDiagonalCount int
	RotationCountMatchesMetrics          bool
	EncodedDiagonalCountMatchesMetrics   bool
}

type bkrMaterialBaselineDetails struct {
	ParameterChain   bkrParameterChainBaseline
	GaloisKeys       bkrGaloisKeyBaseline
	Schedules        []bkrLinearTransformScheduleBaseline
	EncodedDiagonals []bkrEncodedDiagonalBaseline
	Index            bkrMaterialBaselineIndex
}

type bkrRecorder struct {
	dir              string
	spec             bkrCaseSpec
	start            time.Time
	metadata         bkrMetadata
	experimentCase   *bkrExperimentCase
	targetResults    []bkrTargetRunResult
	materialMetrics  []bkrMaterialMetrics
	runtimeMetrics   []bkrRuntimeMetrics
	parameterChains  []bkrParameterChainBaseline
	galoisBaselines  []bkrGaloisKeyBaseline
	schedules        []bkrLinearTransformScheduleBaseline
	encodedDiagonals []bkrEncodedDiagonalBaseline
	materialIndices  []bkrMaterialBaselineIndex
	failures         []bkrFailure
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

	if err := bkrRejectForbiddenAuditArtifacts(dir); err != nil {
		return nil, err
	}

	rec := &bkrRecorder{
		dir:   dir,
		spec:  spec,
		start: time.Now(),
	}

	if err := rec.writeMetadata(); err != nil {
		return nil, err
	}

	return rec, nil
}

func (r *bkrRecorder) close() error {
	return nil
}

func writeBootstrapKeyReuseResult(rec *bkrRecorder, result bkrTargetRunResult, material bkrMaterialMetrics, runtimeMetrics bkrRuntimeMetrics, baseline bkrMaterialBaselineDetails) error {
	rec.targetResults = append(rec.targetResults, result)
	rec.materialMetrics = append(rec.materialMetrics, material)
	rec.runtimeMetrics = append(rec.runtimeMetrics, runtimeMetrics)
	rec.parameterChains = append(rec.parameterChains, baseline.ParameterChain)
	rec.galoisBaselines = append(rec.galoisBaselines, baseline.GaloisKeys)
	rec.schedules = append(rec.schedules, baseline.Schedules...)
	rec.encodedDiagonals = append(rec.encodedDiagonals, baseline.EncodedDiagonals...)
	rec.materialIndices = append(rec.materialIndices, baseline.Index)
	return nil
}

func (r *bkrRecorder) writeExperimentCase() error {
	spec := r.spec
	experimentCase := bkrExperimentCase{
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
	}
	r.experimentCase = &experimentCase
	return nil
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
	return nil
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

	if err := r.writeAllCSV(summary); err != nil {
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

	r.metadata = metadata
	return nil
}

var (
	bkrRequiredCSVFiles = []string{
		"metadata.csv",
		"experiment_case.csv",
		"target_results.csv",
		"material_metrics.csv",
		"runtime_metrics.csv",
		"parameter_chain_baseline.csv",
		"galois_key_baseline.csv",
		"linear_transform_schedule_baseline.csv",
		"encoded_diagonal_baseline.csv",
		"material_baseline_index.csv",
		"failures.csv",
		"summary.csv",
	}

	bkrMetadataCSVHeader = []string{
		"schema_version",
		"lane",
		"plan_id",
		"case_id",
		"params_profile",
		"command",
		"go_version",
		"go_env",
		"goos",
		"goarch",
		"num_cpu",
		"gomaxprocs",
		"vcs_revision",
		"vcs_time",
		"vcs_modified",
		"branch",
		"commit",
		"dirty_state",
		"started_at",
	}

	bkrExperimentCaseCSVHeader = []string{
		"record_type",
		"schema_version",
		"plan_id",
		"case_id",
		"params_profile",
		"all_target_levels",
		"used_target_levels",
		"fixed_target_scale_log",
		"log_slots",
		"ring_switch_mode",
		"packed_mode",
		"seed",
		"repeat_count",
	}

	bkrTargetResultsCSVHeader = []string{
		"record_type",
		"schema_version",
		"plan_id",
		"case_id",
		"params_profile",
		"target_level",
		"status",
		"output_level",
		"expected_output_level",
		"output_level_equality",
		"output_scale_log2",
		"expected_output_scale_log2",
		"output_scale_equality",
		"average_log2_precision_real",
		"average_log2_precision_imag",
		"precision_threshold_bits",
		"bootstrap_error_status",
		"packed_ciphertexts",
	}

	bkrMaterialMetricsCSVHeader = []string{
		"record_type",
		"schema_version",
		"plan_id",
		"case_id",
		"params_profile",
		"target_level",
		"generated_evaluation_key_count",
		"generated_rotation_key_count",
		"generated_encoded_diagonal_count",
		"persistent_key_bytes",
		"persistent_matrix_bytes",
		"shared_rotation_keys",
		"shared_encoded_diagonals",
		"rns_slice_success",
		"fallback_reason",
		"rotation_set_hash",
		"c2s_schedule_hash",
		"s2c_schedule_hash",
		"encoded_diagonal_set_hash",
	}

	bkrRuntimeMetricsCSVHeader = []string{
		"record_type",
		"schema_version",
		"plan_id",
		"case_id",
		"params_profile",
		"target_level",
		"keygen_time_ms",
		"evaluator_matrix_construction_time_ms",
		"bootstrap_latency_ms",
		"peak_keygen_heap_bytes",
		"peak_runtime_heap_bytes",
	}

	bkrParameterChainBaselineCSVHeader = []string{
		"record_type",
		"schema_version",
		"plan_id",
		"case_id",
		"params_profile",
		"target_level",
		"full_log_n",
		"full_max_level",
		"full_q_count",
		"full_p_count",
		"full_q_hash",
		"full_p_hash",
		"residual_log_n",
		"residual_max_level",
		"residual_q_count",
		"residual_p_count",
		"residual_q_prefix_hash",
		"residual_p_hash",
		"bootstrapping_log_n",
		"bootstrapping_max_level",
		"bootstrapping_q_count",
		"bootstrapping_p_count",
		"bootstrapping_q_hash",
		"bootstrapping_p_hash",
		"fixed_target_scale_log",
		"residual_default_scale_log2",
		"bootstrapping_default_scale_log2",
	}

	bkrGaloisKeyBaselineCSVHeader = []string{
		"record_type",
		"schema_version",
		"plan_id",
		"case_id",
		"params_profile",
		"target_level",
		"generated_galois_elements_sorted",
		"required_bootstrapping_galois_elements_sorted",
		"canonical_rotation_ids",
		"discrete_log_k",
		"contains_complex_conjugation",
		"generated_count",
		"required_count",
		"missing_required_galois_elements",
		"extra_generated_galois_elements",
		"rotation_set_hash",
	}

	bkrLinearTransformScheduleBaselineCSVHeader = []string{
		"record_type",
		"schema_version",
		"plan_id",
		"case_id",
		"params_profile",
		"target_level",
		"matrix_name",
		"transform_index",
		"dft_type",
		"dft_type_value",
		"dft_format",
		"dft_format_value",
		"dft_log_slots",
		"dft_level_q",
		"dft_level_p",
		"dft_levels",
		"dft_bit_reversed",
		"dft_log_bsgs_ratio",
		"lt_level_q",
		"lt_level_p",
		"lt_n1",
		"lt_log_rows",
		"lt_log_cols",
		"lt_scale",
		"lt_scale_log2",
		"diagonal_indices",
		"galois_elements",
		"transform_schedule_id",
		"matrix_schedule_id",
	}

	bkrEncodedDiagonalBaselineCSVHeader = []string{
		"record_type",
		"schema_version",
		"plan_id",
		"case_id",
		"params_profile",
		"target_level",
		"matrix_name",
		"transform_index",
		"diagonal_index",
		"level_q",
		"level_p",
		"n",
		"q_prefix_hash",
		"p_prefix_hash",
		"poly_binary_size",
		"poly_sha256",
		"encoded_diagonal_id",
	}

	bkrMaterialBaselineIndexCSVHeader = []string{
		"record_type",
		"schema_version",
		"plan_id",
		"case_id",
		"params_profile",
		"target_level",
		"rotation_set_hash",
		"c2s_schedule_hash",
		"s2c_schedule_hash",
		"encoded_diagonal_set_hash",
		"generated_rotation_key_count",
		"schedule_record_count",
		"encoded_diagonal_record_count",
		"distinct_encoded_diagonal_count",
		"metrics_generated_rotation_key_count",
		"metrics_generated_encoded_diagonal_count",
		"rotation_count_matches_metrics",
		"encoded_diagonal_count_matches_metrics",
	}

	bkrFailuresCSVHeader = []string{
		"record_type",
		"schema_version",
		"plan_id",
		"case_id",
		"params_profile",
		"target_level",
		"stage",
		"message",
		"before_keygen",
		"generated_keygen",
		"generated_runtime",
	}

	bkrSummaryCSVHeader = []string{
		"schema_version",
		"lane",
		"plan_id",
		"case_id",
		"params_profile",
		"status",
		"target_levels",
		"result_directory",
		"started_at",
		"finished_at",
		"elapsed_ms",
		"total_targets",
		"successful_targets",
		"failed_targets",
		"passed",
		"failure_count",
		"shared_rotation_keys",
		"shared_encoded_diagonals",
		"rns_slice_success",
		"fallback_reason",
	}
)

func (r *bkrRecorder) writeAllCSV(summary bkrRunSummary) error {
	var err error
	err = errors.Join(err, r.writeMetadataCSV())
	err = errors.Join(err, r.writeExperimentCaseCSV())
	err = errors.Join(err, r.writeTargetResultsCSV())
	err = errors.Join(err, r.writeMaterialMetricsCSV())
	err = errors.Join(err, r.writeRuntimeMetricsCSV())
	err = errors.Join(err, r.writeParameterChainBaselineCSV())
	err = errors.Join(err, r.writeGaloisKeyBaselineCSV())
	err = errors.Join(err, r.writeLinearTransformScheduleBaselineCSV())
	err = errors.Join(err, r.writeEncodedDiagonalBaselineCSV())
	err = errors.Join(err, r.writeMaterialBaselineIndexCSV())
	err = errors.Join(err, r.writeFailuresCSV())
	err = errors.Join(err, r.writeSummaryCSV(summary))
	return err
}

func (r *bkrRecorder) writeMetadataCSV() error {
	m := r.metadata
	return bkrWriteCSVFile(filepath.Join(r.dir, "metadata.csv"), bkrMetadataCSVHeader, [][]string{{
		m.SchemaVersion,
		m.Lane,
		m.PlanID,
		m.CaseID,
		m.ParamsProfile,
		m.Command,
		m.GoVersion,
		bkrJoinKeyValues(m.GoEnv),
		m.GOOS,
		m.GOARCH,
		strconv.Itoa(m.NumCPU),
		strconv.Itoa(m.GOMAXPROCS),
		m.VCSRevision,
		m.VCSTime,
		m.VCSModified,
		m.Branch,
		m.Commit,
		m.DirtyState,
		m.StartedAt,
	}})
}

func (r *bkrRecorder) writeExperimentCaseCSV() error {
	rows := [][]string{}
	if r.experimentCase != nil {
		c := r.experimentCase
		rows = append(rows, []string{
			c.RecordType,
			c.SchemaVersion,
			c.PlanID,
			c.CaseID,
			c.ParamsProfile,
			bkrJoinInts(c.AllTargetLevels),
			bkrJoinInts(c.UsedTargetLevels),
			strconv.Itoa(c.FixedTargetScaleLog),
			strconv.Itoa(c.LogSlots),
			c.RingSwitchMode,
			strconv.FormatBool(c.PackedMode),
			c.Seed,
			strconv.Itoa(c.RepeatCount),
		})
	}
	return bkrWriteCSVFile(filepath.Join(r.dir, "experiment_case.csv"), bkrExperimentCaseCSVHeader, rows)
}

func (r *bkrRecorder) writeTargetResultsCSV() error {
	rows := make([][]string, 0, len(r.targetResults))
	for _, row := range r.targetResults {
		rows = append(rows, []string{
			row.RecordType,
			row.SchemaVersion,
			row.PlanID,
			row.CaseID,
			row.ParamsProfile,
			strconv.Itoa(row.TargetLevel),
			bkrTargetStatus(row),
			strconv.Itoa(row.OutputLevel),
			strconv.Itoa(row.ExpectedOutputLevel),
			strconv.FormatBool(row.OutputLevelEquality),
			bkrFormatFloat(row.OutputScaleLog2),
			bkrFormatFloat(row.ExpectedOutputScaleLog2),
			strconv.FormatBool(row.OutputScaleEquality),
			bkrFormatFloat(row.AverageLog2PrecReal),
			bkrFormatFloat(row.AverageLog2PrecImag),
			bkrFormatFloat(row.PrecisionThresholdBits),
			row.BootstrapErrorStatus,
			strconv.Itoa(row.PackedCiphertexts),
		})
	}
	return bkrWriteCSVFile(filepath.Join(r.dir, "target_results.csv"), bkrTargetResultsCSVHeader, rows)
}

func (r *bkrRecorder) writeMaterialMetricsCSV() error {
	rows := make([][]string, 0, len(r.materialMetrics))
	for _, row := range r.materialMetrics {
		rows = append(rows, []string{
			row.RecordType,
			row.SchemaVersion,
			row.PlanID,
			row.CaseID,
			row.ParamsProfile,
			strconv.Itoa(row.TargetLevel),
			strconv.Itoa(row.GeneratedEvaluationKeyCount),
			strconv.Itoa(row.GeneratedRotationKeyCount),
			strconv.Itoa(row.GeneratedEncodedDiagonalCount),
			strconv.FormatInt(row.PersistentKeyBytes, 10),
			strconv.FormatInt(row.PersistentMatrixBytes, 10),
			strconv.Itoa(row.SharedRotationKeys),
			strconv.Itoa(row.SharedEncodedDiagonals),
			row.RNSSliceSuccess,
			row.FallbackReason,
			row.RotationSetHash,
			row.C2SScheduleHash,
			row.S2CScheduleHash,
			row.EncodedDiagonalSetHash,
		})
	}
	return bkrWriteCSVFile(filepath.Join(r.dir, "material_metrics.csv"), bkrMaterialMetricsCSVHeader, rows)
}

func (r *bkrRecorder) writeRuntimeMetricsCSV() error {
	rows := make([][]string, 0, len(r.runtimeMetrics))
	for _, row := range r.runtimeMetrics {
		rows = append(rows, []string{
			row.RecordType,
			row.SchemaVersion,
			row.PlanID,
			row.CaseID,
			row.ParamsProfile,
			strconv.Itoa(row.TargetLevel),
			bkrFormatFloat(row.KeygenTimeMS),
			bkrFormatFloat(row.EvaluatorMatrixConstructionMS),
			bkrFormatFloat(row.BootstrapLatencyMS),
			strconv.FormatUint(row.PeakKeygenHeapBytes, 10),
			strconv.FormatUint(row.PeakRuntimeHeapBytes, 10),
		})
	}
	return bkrWriteCSVFile(filepath.Join(r.dir, "runtime_metrics.csv"), bkrRuntimeMetricsCSVHeader, rows)
}

func (r *bkrRecorder) writeParameterChainBaselineCSV() error {
	rows := make([][]string, 0, len(r.parameterChains))
	for _, row := range r.parameterChains {
		rows = append(rows, []string{
			row.RecordType,
			row.SchemaVersion,
			row.PlanID,
			row.CaseID,
			row.ParamsProfile,
			strconv.Itoa(row.TargetLevel),
			strconv.Itoa(row.FullLogN),
			strconv.Itoa(row.FullMaxLevel),
			strconv.Itoa(row.FullQCount),
			strconv.Itoa(row.FullPCount),
			row.FullQHash,
			row.FullPHash,
			strconv.Itoa(row.ResidualLogN),
			strconv.Itoa(row.ResidualMaxLevel),
			strconv.Itoa(row.ResidualQCount),
			strconv.Itoa(row.ResidualPCount),
			row.ResidualQPrefixHash,
			row.ResidualPHash,
			strconv.Itoa(row.BootstrappingLogN),
			strconv.Itoa(row.BootstrappingMaxLevel),
			strconv.Itoa(row.BootstrappingQCount),
			strconv.Itoa(row.BootstrappingPCount),
			row.BootstrappingQHash,
			row.BootstrappingPHash,
			strconv.Itoa(row.FixedTargetScaleLog),
			bkrFormatFloat(row.ResidualDefaultScaleLog2),
			bkrFormatFloat(row.BootstrappingDefaultScaleLog2),
		})
	}
	return bkrWriteCSVFile(filepath.Join(r.dir, "parameter_chain_baseline.csv"), bkrParameterChainBaselineCSVHeader, rows)
}

func (r *bkrRecorder) writeGaloisKeyBaselineCSV() error {
	rows := make([][]string, 0, len(r.galoisBaselines))
	for _, row := range r.galoisBaselines {
		rows = append(rows, []string{
			row.RecordType,
			row.SchemaVersion,
			row.PlanID,
			row.CaseID,
			row.ParamsProfile,
			strconv.Itoa(row.TargetLevel),
			bkrJoinUint64s(row.GeneratedGaloisElementsSorted),
			bkrJoinUint64s(row.RequiredBootstrappingGaloisElements),
			bkrJoinStrings(row.CanonicalRotationIDs),
			bkrJoinInts(row.DiscreteLogK),
			strconv.FormatBool(row.ContainsComplexConjugation),
			strconv.Itoa(row.GeneratedCount),
			strconv.Itoa(row.RequiredCount),
			bkrJoinUint64s(row.MissingRequiredGaloisElements),
			bkrJoinUint64s(row.ExtraGeneratedGaloisElements),
			row.RotationSetHash,
		})
	}
	return bkrWriteCSVFile(filepath.Join(r.dir, "galois_key_baseline.csv"), bkrGaloisKeyBaselineCSVHeader, rows)
}

func (r *bkrRecorder) writeLinearTransformScheduleBaselineCSV() error {
	rows := make([][]string, 0, len(r.schedules))
	for _, row := range r.schedules {
		rows = append(rows, []string{
			row.RecordType,
			row.SchemaVersion,
			row.PlanID,
			row.CaseID,
			row.ParamsProfile,
			strconv.Itoa(row.TargetLevel),
			row.MatrixName,
			strconv.Itoa(row.TransformIndex),
			row.DFTType,
			strconv.Itoa(row.DFTTypeValue),
			row.DFTFormat,
			strconv.Itoa(row.DFTFormatValue),
			strconv.Itoa(row.DFTLogSlots),
			strconv.Itoa(row.DFTLevelQ),
			strconv.Itoa(row.DFTLevelP),
			bkrJoinInts(row.DFTLevels),
			strconv.FormatBool(row.DFTBitReversed),
			strconv.Itoa(row.DFTLogBSGSRatio),
			strconv.Itoa(row.LTLevelQ),
			strconv.Itoa(row.LTLevelP),
			strconv.Itoa(row.LTN1),
			strconv.Itoa(row.LTLogRows),
			strconv.Itoa(row.LTLogCols),
			row.LTScale,
			bkrFormatFloat(row.LTScaleLog2),
			bkrJoinInts(row.DiagonalIndices),
			bkrJoinUint64s(row.GaloisElements),
			row.TransformScheduleID,
			row.MatrixScheduleID,
		})
	}
	return bkrWriteCSVFile(filepath.Join(r.dir, "linear_transform_schedule_baseline.csv"), bkrLinearTransformScheduleBaselineCSVHeader, rows)
}

func (r *bkrRecorder) writeEncodedDiagonalBaselineCSV() error {
	rows := make([][]string, 0, len(r.encodedDiagonals))
	for _, row := range r.encodedDiagonals {
		rows = append(rows, []string{
			row.RecordType,
			row.SchemaVersion,
			row.PlanID,
			row.CaseID,
			row.ParamsProfile,
			strconv.Itoa(row.TargetLevel),
			row.MatrixName,
			strconv.Itoa(row.TransformIndex),
			strconv.Itoa(row.DiagonalIndex),
			strconv.Itoa(row.LevelQ),
			strconv.Itoa(row.LevelP),
			strconv.Itoa(row.N),
			row.QPrefixHash,
			row.PPrefixHash,
			strconv.FormatInt(row.PolyBinarySize, 10),
			row.PolySHA256,
			row.EncodedDiagonalID,
		})
	}
	return bkrWriteCSVFile(filepath.Join(r.dir, "encoded_diagonal_baseline.csv"), bkrEncodedDiagonalBaselineCSVHeader, rows)
}

func (r *bkrRecorder) writeMaterialBaselineIndexCSV() error {
	rows := make([][]string, 0, len(r.materialIndices))
	for _, row := range r.materialIndices {
		rows = append(rows, []string{
			row.RecordType,
			row.SchemaVersion,
			row.PlanID,
			row.CaseID,
			row.ParamsProfile,
			strconv.Itoa(row.TargetLevel),
			row.RotationSetHash,
			row.C2SScheduleHash,
			row.S2CScheduleHash,
			row.EncodedDiagonalSetHash,
			strconv.Itoa(row.GeneratedRotationKeyCount),
			strconv.Itoa(row.ScheduleRecordCount),
			strconv.Itoa(row.EncodedDiagonalRecordCount),
			strconv.Itoa(row.DistinctEncodedDiagonalCount),
			strconv.Itoa(row.MetricsGeneratedRotationKeyCount),
			strconv.Itoa(row.MetricsGeneratedEncodedDiagonalCount),
			strconv.FormatBool(row.RotationCountMatchesMetrics),
			strconv.FormatBool(row.EncodedDiagonalCountMatchesMetrics),
		})
	}
	return bkrWriteCSVFile(filepath.Join(r.dir, "material_baseline_index.csv"), bkrMaterialBaselineIndexCSVHeader, rows)
}

func (r *bkrRecorder) writeFailuresCSV() error {
	rows := make([][]string, 0, len(r.failures))
	for _, row := range r.failures {
		rows = append(rows, []string{
			row.RecordType,
			row.SchemaVersion,
			row.PlanID,
			row.CaseID,
			row.ParamsProfile,
			strconv.Itoa(row.TargetLevel),
			row.Stage,
			row.Message,
			strconv.FormatBool(row.BeforeKeygen),
			strconv.FormatBool(row.GeneratedKeygen),
			strconv.FormatBool(row.GeneratedRuntime),
		})
	}
	return bkrWriteCSVFile(filepath.Join(r.dir, "failures.csv"), bkrFailuresCSVHeader, rows)
}

func (r *bkrRecorder) writeSummaryCSV(summary bkrRunSummary) error {
	return bkrWriteCSVFile(filepath.Join(r.dir, "summary.csv"), bkrSummaryCSVHeader, [][]string{{
		summary.SchemaVersion,
		summary.Lane,
		summary.PlanID,
		summary.CaseID,
		summary.ParamsProfile,
		summary.Status,
		bkrJoinInts(summary.TargetLevels),
		summary.ResultDirectory,
		summary.StartedAt,
		summary.FinishedAt,
		bkrFormatFloat(summary.ElapsedMS),
		strconv.Itoa(summary.TotalTargets),
		strconv.Itoa(summary.SuccessfulTargets),
		strconv.Itoa(summary.FailedTargets),
		strconv.FormatBool(summary.Passed),
		strconv.Itoa(summary.FailureCount),
		strconv.Itoa(summary.SharedRotationKeys),
		strconv.Itoa(summary.SharedDiagonals),
		summary.RNSSliceSuccess,
		summary.FallbackReason,
	}})
}

func bkrWriteCSVFile(path string, header []string, rows [][]string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", filepath.Base(path), err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write(header); err != nil {
		return err
	}

	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return writer.Error()
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

func bkrRejectForbiddenAuditArtifacts(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("cannot inspect result directory %q: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		switch ext {
		case ".json", ".jsonl", ".log":
			return fmt.Errorf("result directory %q contains forbidden audit artifact %q; use a new CSV-only result directory", dir, entry.Name())
		}
	}

	return nil
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

func bkrJoinInts(values []int) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = strconv.Itoa(value)
	}
	return strings.Join(parts, ";")
}

func bkrJoinUint64s(values []uint64) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = strconv.FormatUint(value, 10)
	}
	return strings.Join(parts, ";")
}

func bkrJoinStrings(values []string) string {
	return strings.Join(values, ";")
}

func bkrJoinKeyValues(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values[key])
	}
	return strings.Join(parts, ";")
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
