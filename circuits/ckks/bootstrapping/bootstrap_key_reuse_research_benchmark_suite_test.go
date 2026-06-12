package bootstrapping

import (
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tuneinsight/lattigo/v6/core/rlwe"
	"github.com/tuneinsight/lattigo/v6/schemes/ckks"
)

var (
	bkrResearchBenchmarkMode      = flag.String("bkr.research-mode", "standard", "research benchmark mode: smoke, standard, or exhaustive")
	bkrResearchBenchmarkResultDir = flag.String("bkr.research-result-dir", "", "optional directory for research benchmark CSV outputs")
	bkrTargetCountSweepResultDir  = flag.String("bkr.target-count-sweep-result-dir", "", "optional directory for target-count sweep CSV outputs")
)

const bkrResearchBenchmarkSchemaVersion = "bootstrap-key-reuse-research-benchmark/v1"

const bkrTargetCountSweepSchemaVersion = "bootstrap-key-reuse-target-count-sweep/v1"

type bkrResearchBenchmarkCase struct {
	Name                string
	Lane                string
	CaseID              string
	ProfileID           string
	TargetShape         string
	Spec                bkrCaseSpec
	FutureTargetLevels  []int
	OwnerTargetLevels   []int
	Policy              string
	EnableRNSSliceViews bool
	AllowSupersetDrop   bool
	LongOnly            bool
	GOMAXPROCS          int
	Count               int
	Phases              []string
	CorrectnessGate     string
	ProfileGate         string
	Tier                string
}

type bkrResearchBenchmarkAuditItem struct {
	ID         string
	Loophole   string
	Mitigation string
	Evidence   string
	Mitigated  bool
}

type bkrResearchBenchmarkResult struct {
	SchemaVersion            string
	Name                     string
	Lane                     string
	CaseID                   string
	ProfileID                string
	TargetShape              string
	TargetLevels             []int
	OwnerTargetLevels        []int
	Policy                   string
	PlanMS                   float64
	KeygenMS                 float64
	EvaluatorConstructionMS  float64
	BootstrapMS              float64
	BootstrapManyMS          float64
	ReportGenerationMS       float64
	PersistentKeyBytes       int64
	PhysicalMaterialCount    int
	LogicalViewCount         int
	SharedCount              int
	OutputLevelEquality      bool
	OutputScaleEquality      bool
	AverageLog2PrecisionReal float64
	AverageLog2PrecisionImag float64
	PrecisionThresholdBits   float64
	GoVersion                string
	GOMAXPROCS               int
	Commit                   string
	DirtyState               string
}

type bkrResearchPrefixComparisonCase struct {
	Name                string
	Scheme              string
	Workload            string
	Spec                bkrCaseSpec
	FutureTargetLevels  []int
	OwnerTargetLevels   []int
	Policy              string
	EnableRNSSliceViews bool
	AllowSupersetDrop   bool
	ExpectedStatus      string
	ExpectedEvidence    []string
	PerformanceMetrics  []string
}

type bkrTargetCountSweepCase struct {
	Name             string
	ProfileID        string
	CaseID           string
	Spec             bkrCaseSpec
	TargetCount      int
	TargetCountLabel string
	TargetLevels     []int
	LongOnly         bool
}

type bkrTargetCountSweepResult struct {
	SchemaVersion          string
	ProfileID              string
	CaseID                 string
	TargetCount            int
	TargetCountLabel       string
	TargetLevels           []int
	Scheme                 string
	MaterialTotalMB        float64
	KeyPreparationTimeS    float64
	TotalBootstrapTimeS    float64
	MeanBootstrapLatencyMS float64
	OwnerTargetLevels      []int
	PhysicalMaterialCount  int
	SharedMaterialCount    int
	CorrectnessPass        bool
	GoVersion              string
	GOMAXPROCS             int
	Commit                 string
	DirtyState             string
}

func bkrResearchBenchmarkSuiteManifest() []bkrResearchBenchmarkCase {
	p0 := bkrCaseP0TinyNativeSingle()
	p1 := bkrCaseP1TinyRingSwitchSingle()
	p2Single := bkrCaseP2MultiFastSingle()
	p2Clustered := bkrCaseP2MultiFastClustered()
	p2Sparse := bkrCaseP2MultiFastSparse()
	p2Random := bkrCaseP2MultiFastRandomFixed()
	p3Clustered := bkrCaseP3N15SparseClustered()
	p4Sparse := bkrCaseP4N15DenseSparse()
	p5All := bkrCaseP5N16SparseLongSingle()
	p6Sparse := bkrCaseP6N16DenseLongSparse()

	return []bkrResearchBenchmarkCase{
		bkrResearchCase("a0_p2_clustered_oracle", "a0_oracle", p2Clustered, "clustered", p2Clustered.AllTargetLevels, nil, "exact_only", false, false, "standard", "TestBootstrapKeyReuseA0", "BenchmarkBootstrapKeyReuseA0"),
		bkrResearchCase("a1_p2_sparse_used_only", "a1_used_only", bkrCaseA1P2MultiFastUsedSparse(), "sparse", []int{1, 3}, nil, "exact_only", false, false, "standard", "TestBootstrapKeyReuseA1", "runtime_metrics.csv"),
		bkrResearchCase("a2_p2_sparse_rotation_pool", "a2_rotation_pool", bkrCaseA2P2MultiFastUsedSparse(), "sparse", []int{1, 3}, nil, "exact_only", false, false, "standard", "TestBootstrapKeyReuseA2", "rotation_key_pool.csv"),
		bkrResearchCase("a3_p2_sparse_schedule_pool", "a3_schedule_pool", bkrCaseA3P2MultiFastUsedSparse(), "sparse", []int{1, 3}, nil, "exact_only", false, false, "standard", "TestBootstrapKeyReuseA3", "linear_transform_schedule_baseline.csv"),
		bkrResearchCase("a4_p2_sparse_diagonal_pool", "a4_diagonal_pool", bkrCaseA4P2MultiFastUsedSparse(), "sparse", []int{1, 3}, nil, "exact_only", false, false, "standard", "TestBootstrapKeyReuseA4", "encoded_diagonal_baseline.csv"),
		bkrResearchCase("a5_p2_sparse_rns_slice", "a5_rns_slice", bkrCaseA5P2MultiFastUsedSparse(), "sparse", []int{1, 3}, nil, "exact_prefix", true, false, "standard", "TestBootstrapKeyReuseA5", "rns_slice_success"),
		bkrResearchCase("a6_p2_sparse_superset_drop", "a6_superset_drop", bkrCaseA6P2MultiFastUsedSparse(), "sparse", []int{1, 3}, nil, "exact_prefix_superset_drop", true, true, "standard", "TestBootstrapKeyReuseA6", "superset_output_drop"),
		bkrResearchCase("final_p0_exact_single", "final_api", p0, "single", p0.AllTargetLevels, nil, "exact_only", false, false, "smoke", "TestTargetLevelBootstrapperExactOwnerBootstrapsDeclaredTarget", "BenchmarkResearchTargetLevelFinalAPI"),
		bkrResearchCase("final_p1_exact_ring_switch", "final_api", p1, "single", p1.AllTargetLevels, nil, "exact_only", false, false, "standard", "TestSharedKeyPoolMaterialIDsCoverBootstrappingKinds", "BenchmarkResearchTargetLevelFinalAPI"),
		bkrResearchCase("final_p2_exact_all_legal", "final_api", p2Clustered, "all_legal", p2Clustered.AllTargetLevels, nil, "exact_only", false, false, "standard", "TestTargetLevelBootstrapperExactOwnersCoverAllLegalFastTargets", "BenchmarkResearchTargetLevelFinalAPI"),
		bkrResearchCase("final_p2_prefix_single", "final_api", p2Single, "single", p2Single.AllTargetLevels, nil, "exact_prefix", true, false, "standard", "TestRNSPrefixViewAllowsEvaluationKeyPrefix", "BenchmarkResearchTargetLevelFinalAPI"),
		bkrResearchCase("final_p2_superset_sparse", "final_api", p2Sparse, "sparse", []int{1, 3}, nil, "exact_prefix_superset_drop", true, true, "standard", "TestTargetLevelBootstrapperSupersetDropMatchesDirectTarget", "BenchmarkResearchTargetLevelFinalAPI"),
		bkrResearchCase("final_p2_exact_random", "final_api", p2Random, "random", p2Random.UsedTargetLevels, nil, "exact_only", false, false, "standard", "TestTargetLevelBootstrapperExactOwnersCoverAllLegalFastTargets", "BenchmarkResearchTargetLevelFinalAPI"),
		bkrResearchCase("final_p3_exact_clustered", "final_api", p3Clustered, "clustered", p3Clustered.UsedTargetLevels, nil, "exact_only", false, false, "standard", "TestTargetLevelBootstrapperExactOwnersCoverAllLegalFastTargets", "BenchmarkResearchTargetLevelFinalAPI"),
		bkrResearchCase("final_p4_superset_sparse", "final_api", p4Sparse, "sparse", p4Sparse.UsedTargetLevels, nil, "exact_prefix_superset_drop", true, true, "standard", "TestTargetLevelBootstrapperExactOwnersCoverAllLegalFastTargets", "BenchmarkResearchTargetLevelFinalAPI"),
		bkrResearchCase("final_p5_exact_all_legal_long", "final_api", p5All, "all_legal", p5All.AllTargetLevels, nil, "exact_only", false, false, "exhaustive", "TestTargetLevelBootstrapperExactOwnersCoverAllLegalLongTargets", "BenchmarkResearchTargetLevelFinalAPI"),
		bkrResearchCase("final_p6_superset_sparse_long", "final_api", p6Sparse, "sparse", p6Sparse.UsedTargetLevels, nil, "exact_prefix_superset_drop", true, true, "exhaustive", "TestTargetLevelBootstrapperExactOwnersCoverAllLegalLongTargets", "BenchmarkResearchTargetLevelFinalAPI"),
	}
}

func bkrResearchPrefixComparisonCases() []bkrResearchPrefixComparisonCase {
	spec := bkrCaseA6P2MultiFastUsedSparse()
	metrics := []string{"plan_ns_per_op", "executable_targets", "prefix_rejections", "owner_target_count", "shared_material_count"}
	return []bkrResearchPrefixComparisonCase{
		{
			Name:                "prefix_only_p2_sparse_rejected",
			Scheme:              "rns_prefix_only",
			Workload:            "P2_MULTI_FAST sparse target levels [1,3] with owner [3]",
			Spec:                spec,
			FutureTargetLevels:  []int{1, 3},
			OwnerTargetLevels:   []int{3},
			Policy:              "exact_prefix",
			EnableRNSSliceViews: true,
			AllowSupersetDrop:   false,
			ExpectedStatus:      "rejected",
			ExpectedEvidence:    []string{"ErrTargetLevelUnavailable", "rns_prefix_bootstrapping_q_prefix_mismatch_2"},
			PerformanceMetrics:  append([]string(nil), metrics...),
		},
		{
			Name:                "planner_p2_sparse_superset_success",
			Scheme:              "target_level_planner",
			Workload:            "P2_MULTI_FAST sparse target levels [1,3] with owner [3]",
			Spec:                spec,
			FutureTargetLevels:  []int{1, 3},
			OwnerTargetLevels:   []int{3},
			Policy:              "exact_prefix_superset_drop",
			EnableRNSSliceViews: true,
			AllowSupersetDrop:   true,
			ExpectedStatus:      "executable",
			ExpectedEvidence:    []string{"superset_output_drop", "shared evaluation key owner 3"},
			PerformanceMetrics:  append([]string(nil), metrics...),
		},
	}
}

func bkrTargetCountSweepCases() []bkrTargetCountSweepCase {
	p4Base := bkrCaseP4N15DenseSingle()
	p4All := bkrLegalTargetLevelsForSweep(p4Base)
	p6Base := bkrCaseP6N16DenseLongSingle()
	p6All := bkrLegalTargetLevelsForSweep(p6Base)

	return []bkrTargetCountSweepCase{
		bkrTargetCountSweepCaseFromLevels("P4", "1", p4Base, []int{4}),
		bkrTargetCountSweepCaseFromLevels("P4", "2", p4Base, []int{1, 4}),
		bkrTargetCountSweepCaseFromLevels("P4", "3", p4Base, []int{1, 3, 4}),
		bkrTargetCountSweepCaseFromLevels("P4", "all", p4Base, p4All),
		bkrTargetCountSweepCaseFromLevels("P6", "1", p6Base, []int{13}),
		bkrTargetCountSweepCaseFromLevels("P6", "2", p6Base, []int{1, 13}),
		bkrTargetCountSweepCaseFromLevels("P6", "4", p6Base, []int{1, 5, 9, 13}),
		bkrTargetCountSweepCaseFromLevels("P6", "8", p6Base, []int{1, 3, 5, 7, 9, 11, 12, 13}),
		bkrTargetCountSweepCaseFromLevels("P6", "all", p6Base, p6All),
	}
}

func bkrTargetCountSweepSmokeCase() bkrTargetCountSweepCase {
	return bkrTargetCountSweepCaseFromLevels("P0", "1", bkrCaseP0TinyNativeSingle(), []int{1})
}

func bkrTargetCountSweepCaseFromLevels(prefix, label string, base bkrCaseSpec, levels []int) bkrTargetCountSweepCase {
	targets := append([]int(nil), levels...)
	spec := base
	spec.CaseID = fmt.Sprintf("target_count_sweep_%s_%s", strings.ToLower(prefix), label)
	spec.AllTargetLevels = append([]int(nil), targets...)
	spec.UsedTargetLevels = append([]int(nil), targets...)
	return bkrTargetCountSweepCase{
		Name:             fmt.Sprintf("%s_target_count_%s", prefix, label),
		ProfileID:        spec.ProfileID,
		CaseID:           spec.CaseID,
		Spec:             spec,
		TargetCount:      len(targets),
		TargetCountLabel: label,
		TargetLevels:     targets,
		LongOnly:         spec.LongOnly,
	}
}

func bkrLegalTargetLevelsForSweep(spec bkrCaseSpec) []int {
	fullParams, err := ckks.NewParametersFromLiteral(spec.SchemeParams)
	if err != nil {
		panic(err)
	}
	levels := make([]int, 0, fullParams.MaxLevel()+1)
	for targetLevel := 0; targetLevel <= fullParams.MaxLevel(); targetLevel++ {
		if _, _, err := buildTargetBootstrappingParameters(spec, targetLevel); err == nil {
			levels = append(levels, targetLevel)
		}
	}
	if len(levels) == 0 {
		panic(fmt.Sprintf("no legal target levels for %s", spec.ProfileID))
	}
	return levels
}

func bkrResearchCase(name, lane string, spec bkrCaseSpec, shape string, futureTargets, ownerTargets []int, policy string, enableRNSSliceViews, allowSupersetDrop bool, tier string, correctnessGate, profileGate string) bkrResearchBenchmarkCase {
	phases := []string{"plan", "keygen", "evaluator_construction", "bootstrap", "report_generation", "oracle_validation"}
	if shape == "clustered" || shape == "sparse" || shape == "all_legal" {
		phases = append(phases, "bootstrap_many", "parallel_bootstrap")
	}
	return bkrResearchBenchmarkCase{
		Name:                name,
		Lane:                lane,
		CaseID:              spec.CaseID,
		ProfileID:           spec.ProfileID,
		TargetShape:         shape,
		Spec:                spec,
		FutureTargetLevels:  append([]int(nil), futureTargets...),
		OwnerTargetLevels:   append([]int(nil), ownerTargets...),
		Policy:              policy,
		EnableRNSSliceViews: enableRNSSliceViews,
		AllowSupersetDrop:   allowSupersetDrop,
		LongOnly:            spec.LongOnly || tier == "exhaustive",
		GOMAXPROCS:          1,
		Count:               5,
		Phases:              phases,
		CorrectnessGate:     correctnessGate,
		ProfileGate:         profileGate,
		Tier:                tier,
	}
}

func bkrResearchBenchmarkConfidenceAudit() []bkrResearchBenchmarkAuditItem {
	return []bkrResearchBenchmarkAuditItem{
		{ID: "RB1", Loophole: "Benchmark numbers can be cherry-picked from a subset of lanes.", Mitigation: "The manifest enumerates A0-A6 plus final_api lanes and the report writes every selected case.", Evidence: "research_benchmark_manifest.csv", Mitigated: true},
		{ID: "RB2", Loophole: "Performance could hide correctness regressions.", Mitigation: "Each benchmark case names a correctness gate and smoke results run A0 precision, scale, and output-level validation.", Evidence: "correctness_gate plus research_benchmark_results.csv", Mitigated: true},
		{ID: "RB3", Loophole: "Scalar latency and throughput could be mixed.", Mitigation: "The manifest records GOMAXPROCS=1 for scalar cases and parallel bootstrap as a separate phase.", Evidence: "gomaxprocs and phases columns", Mitigated: true},
		{ID: "RB4", Loophole: "Long profiles could be silently skipped.", Mitigation: "P5/P6 cases are present with long_only=true and exhaustive tier.", Evidence: "manifest P5_N16_SPARSE_LONG and P6_N16_DENSE_LONG rows", Mitigated: true},
		{ID: "RB5", Loophole: "RNS prefix compatibility could be inferred from integer levels.", Mitigation: "The final API cases include exact_prefix policy and the correctness gate names the RNS prefix view tests.", Evidence: "policy and correctness_gate columns", Mitigated: true},
		{ID: "RB6", Loophole: "Material reuse savings could be reported without material accounting.", Mitigation: "Results include persistent key bytes, physical material count, logical view count, and shared count.", Evidence: "research_benchmark_results.csv material columns", Mitigated: true},
		{ID: "RB7", Loophole: "Environment drift could invalidate comparisons.", Mitigation: "Results record Go version, GOMAXPROCS, commit, and dirty state.", Evidence: "research_benchmark_results.csv environment columns", Mitigated: true},
		{ID: "RB8", Loophole: "Profile-guided or pprof runs could be compared against normal latency.", Mitigation: "Profile commands are documented as a separate lane and are not the scalar baseline.", Evidence: "research_benchmark_suite_cl_test_plan.md profile section", Mitigated: true},
		{ID: "RB9", Loophole: "Go benchmark cache could return stale data.", Mitigation: "CL commands require -count and explicit modes; report records commands and result directories.", Evidence: "research_benchmark_suite_cl_test_plan.md command gates", Mitigated: true},
		{ID: "RB10", Loophole: "A single smoke run could be mistaken for research completion.", Mitigation: "Smoke is only the smallest executable proof; standard and exhaustive lanes are separate required gates.", Evidence: "research_benchmark_suite_spec.md completion definition", Mitigated: true},
	}
}

func bkrResearchBenchmarkManifestCSVHeader() []string {
	return []string{
		"schema_version",
		"name",
		"lane",
		"case_id",
		"profile_id",
		"target_shape",
		"future_target_levels",
		"owner_target_levels",
		"policy",
		"enable_rns_slice_views",
		"allow_superset_drop",
		"long_only",
		"gomaxprocs",
		"count",
		"phases",
		"correctness_gate",
		"profile_gate",
	}
}

func bkrResearchBenchmarkResultCSVHeader() []string {
	return []string{
		"schema_version",
		"name",
		"lane",
		"case_id",
		"profile_id",
		"target_shape",
		"target_levels",
		"owner_target_levels",
		"policy",
		"plan_ms",
		"keygen_ms",
		"evaluator_construction_ms",
		"bootstrap_ms",
		"bootstrap_many_ms",
		"report_generation_ms",
		"persistent_key_bytes",
		"physical_material_count",
		"logical_view_count",
		"shared_count",
		"output_level_equality",
		"output_scale_equality",
		"average_log2_precision_real",
		"average_log2_precision_imag",
		"precision_threshold_bits",
		"go_version",
		"gomaxprocs",
		"commit",
		"dirty_state",
	}
}

func bkrTargetCountSweepCSVHeader() []string {
	return []string{
		"schema_version",
		"profile_id",
		"case_id",
		"target_count",
		"target_count_label",
		"target_levels",
		"scheme",
		"material_total_mb",
		"key_preparation_time_s",
		"total_bootstrap_time_s",
		"mean_bootstrap_latency_ms",
		"owner_target_levels",
		"physical_material_count",
		"shared_material_count",
		"correctness_pass",
		"go_version",
		"gomaxprocs",
		"commit",
		"dirty_state",
	}
}

func bkrWriteResearchBenchmarkSmokeReport(t *testing.T, dir string) (bkrResearchBenchmarkResult, error) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return bkrResearchBenchmarkResult{}, err
	}

	manifest := bkrResearchBenchmarkSuiteManifest()
	manifestRows := make([][]string, 0, len(manifest))
	for _, tc := range manifest {
		manifestRows = append(manifestRows, tc.csvRow())
	}
	if err := bkrWriteCSVFile(filepath.Join(dir, "research_benchmark_manifest.csv"), bkrResearchBenchmarkManifestCSVHeader(), manifestRows); err != nil {
		return bkrResearchBenchmarkResult{}, err
	}

	auditRows := make([][]string, 0, len(bkrResearchBenchmarkConfidenceAudit()))
	for _, item := range bkrResearchBenchmarkConfidenceAudit() {
		auditRows = append(auditRows, []string{item.ID, item.Loophole, item.Mitigation, item.Evidence, strconv.FormatBool(item.Mitigated)})
	}
	if err := bkrWriteCSVFile(filepath.Join(dir, "research_benchmark_confidence_audit.csv"), []string{"id", "loophole", "mitigation", "evidence", "mitigated"}, auditRows); err != nil {
		return bkrResearchBenchmarkResult{}, err
	}

	smoke, ok := bkrResearchBenchmarkCaseByName(manifest, "final_p0_exact_single")
	if !ok {
		return bkrResearchBenchmarkResult{}, fmt.Errorf("missing final_p0_exact_single benchmark case")
	}
	result, err := bkrRunResearchBenchmarkCaseOnce(t, smoke)
	if err != nil {
		return bkrResearchBenchmarkResult{}, err
	}
	if err := bkrWriteCSVFile(filepath.Join(dir, "research_benchmark_results.csv"), bkrResearchBenchmarkResultCSVHeader(), [][]string{result.csvRow()}); err != nil {
		return bkrResearchBenchmarkResult{}, err
	}
	return result, nil
}

func bkrWriteTargetCountSweepSmokeReport(t *testing.T, dir string) ([]bkrTargetCountSweepResult, error) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	tc := bkrTargetCountSweepSmokeCase()
	rows := make([]bkrTargetCountSweepResult, 0, 2)
	for _, scheme := range []string{"original_lattigo", "ours_reusable_planner"} {
		row, err := bkrRunTargetCountSweepScheme(t, tc, scheme)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	if err := bkrWriteTargetCountSweepCSV(filepath.Join(dir, "target_count_sweep_results.csv"), rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func bkrWriteTargetCountSweepCSV(path string, rows []bkrTargetCountSweepResult) error {
	csvRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		csvRows = append(csvRows, row.csvRow())
	}
	return bkrWriteCSVFile(path, bkrTargetCountSweepCSVHeader(), csvRows)
}

func bkrAppendTargetCountSweepCSV(path string, row bkrTargetCountSweepResult) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	writeHeader := false
	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		writeHeader = true
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if writeHeader {
		if err := w.Write(bkrTargetCountSweepCSVHeader()); err != nil {
			return err
		}
	}
	if err := w.Write(row.csvRow()); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func bkrRunResearchBenchmarkCaseOnce(t *testing.T, tc bkrResearchBenchmarkCase) (bkrResearchBenchmarkResult, error) {
	t.Helper()

	var plan *TargetLevelMaterialPlan
	planElapsed, _, err := bkrMeasurePhase(func() error {
		req, err := bkrResearchTargetLevelRequest(tc)
		if err != nil {
			return err
		}
		plan, err = NewTargetLevelMaterialPlan(req)
		return err
	})
	if err != nil {
		return bkrResearchBenchmarkResult{}, err
	}

	fullParams, err := ckks.NewParametersFromLiteral(tc.Spec.SchemeParams)
	if err != nil {
		return bkrResearchBenchmarkResult{}, err
	}
	sk, err := newDeterministicSecretKey(fullParams, tc.Spec.Seed+"/secret-key")
	if err != nil {
		return bkrResearchBenchmarkResult{}, err
	}

	var keys *ReusableEvaluationKeys
	keygenElapsed, _, err := bkrMeasurePhase(func() error {
		keys, err = plan.GenReusableEvaluationKeys(sk)
		return err
	})
	if err != nil {
		return bkrResearchBenchmarkResult{}, err
	}

	var bootstrapper *TargetLevelBootstrapper
	evaluatorElapsed, _, err := bkrMeasurePhase(func() error {
		bootstrapper, err = NewTargetLevelBootstrapper(plan, keys)
		return err
	})
	if err != nil {
		return bkrResearchBenchmarkResult{}, err
	}

	targetLevel := tc.FutureTargetLevels[0]
	params, _, err := buildTargetBootstrappingParameters(tc.Spec, targetLevel)
	if err != nil {
		return bkrResearchBenchmarkResult{}, err
	}
	ct, values := targetLevelInputCiphertextForTest(t, tc.Spec, params, sk, targetLevel)

	var out *rlwe.Ciphertext
	bootstrapElapsed, _, err := bkrMeasurePhase(func() error {
		out, err = bootstrapper.BootstrapAtLevel(ct, targetLevel)
		return err
	})
	if err != nil {
		return bkrResearchBenchmarkResult{}, err
	}

	ctMany0, valuesMany0 := targetLevelInputCiphertextForTest(t, tc.Spec, params, sk, targetLevel)
	ctMany1, valuesMany1 := targetLevelInputCiphertextForTest(t, tc.Spec, params, sk, targetLevel)
	bootstrapManyElapsed, _, err := bkrMeasurePhase(func() error {
		outs, err := bootstrapper.BootstrapManyAtLevel([]rlwe.Ciphertext{*ctMany0, *ctMany1}, targetLevel)
		if err != nil {
			return err
		}
		_, err = bkrValidateBootstrapKeyReuseOutputs(t, bkrResearchBenchmarkSchemaVersion, tc.Spec, targetLevel, targetLevel, params, outs, [][]complex128{valuesMany0, valuesMany1})
		return err
	})
	if err != nil {
		return bkrResearchBenchmarkResult{}, err
	}

	var report TargetLevelPlanReport
	reportElapsed, _, err := bkrMeasurePhase(func() error {
		report = bootstrapper.PlanReport()
		return nil
	})
	if err != nil {
		return bkrResearchBenchmarkResult{}, err
	}

	validation, err := bkrValidateBootstrapKeyReuseOutputs(t, bkrResearchBenchmarkSchemaVersion, tc.Spec, targetLevel, targetLevel, params, []rlwe.Ciphertext{*out}, [][]complex128{values})
	if err != nil {
		return bkrResearchBenchmarkResult{}, err
	}

	_, commit, dirty := bkrGitMetadata()
	return bkrResearchBenchmarkResult{
		SchemaVersion:            bkrResearchBenchmarkSchemaVersion,
		Name:                     tc.Name,
		Lane:                     tc.Lane,
		CaseID:                   tc.CaseID,
		ProfileID:                tc.ProfileID,
		TargetShape:              tc.TargetShape,
		TargetLevels:             append([]int(nil), tc.FutureTargetLevels...),
		OwnerTargetLevels:        append([]int(nil), report.OwnerTargetLevels...),
		Policy:                   tc.Policy,
		PlanMS:                   bkrDurationMS(planElapsed),
		KeygenMS:                 bkrDurationMS(keygenElapsed),
		EvaluatorConstructionMS:  bkrDurationMS(evaluatorElapsed),
		BootstrapMS:              bkrDurationMS(bootstrapElapsed),
		BootstrapManyMS:          bkrDurationMS(bootstrapManyElapsed),
		ReportGenerationMS:       bkrDurationMS(reportElapsed),
		PersistentKeyBytes:       bkrResearchPersistentKeyBytes(keys),
		PhysicalMaterialCount:    bkrResearchPhysicalMaterialCount(keys.SharedKeyPool()),
		LogicalViewCount:         bkrResearchLogicalViewCount(keys.SharedKeyPool()),
		SharedCount:              bkrResearchCountMapSum(report.SharedCounts),
		OutputLevelEquality:      validation.OutputLevelEquality,
		OutputScaleEquality:      validation.OutputScaleEquality,
		AverageLog2PrecisionReal: validation.AverageLog2PrecReal,
		AverageLog2PrecisionImag: validation.AverageLog2PrecImag,
		PrecisionThresholdBits:   validation.PrecisionThresholdBits,
		GoVersion:                runtime.Version(),
		GOMAXPROCS:               runtime.GOMAXPROCS(0),
		Commit:                   commit,
		DirtyState:               dirty,
	}, nil
}

func bkrRunTargetCountSweepScheme(t testing.TB, tc bkrTargetCountSweepCase, scheme string) (bkrTargetCountSweepResult, error) {
	t.Helper()
	switch scheme {
	case "original_lattigo":
		return bkrRunTargetCountSweepOriginal(t, tc)
	case "ours_reusable_planner":
		return bkrRunTargetCountSweepPlanner(t, tc)
	default:
		return bkrTargetCountSweepResult{}, fmt.Errorf("unknown target-count sweep scheme %q", scheme)
	}
}

func bkrRunTargetCountSweepOriginal(t testing.TB, tc bkrTargetCountSweepCase) (bkrTargetCountSweepResult, error) {
	t.Helper()
	var materialBytes int64
	var keyPrepElapsed time.Duration
	var bootstrapElapsed time.Duration
	correctnessPass := true

	for _, targetLevel := range tc.TargetLevels {
		residualParams, btpParams, err := buildTargetBootstrappingParameters(tc.Spec, targetLevel)
		if err != nil {
			return bkrTargetCountSweepResult{}, err
		}
		sk, err := newDeterministicSecretKey(residualParams, tc.Spec.Seed+"/secret-key")
		if err != nil {
			return bkrTargetCountSweepResult{}, err
		}

		start := time.Now()
		keys, _, err := btpParams.GenEvaluationKeys(sk)
		if err != nil {
			return bkrTargetCountSweepResult{}, err
		}
		eval, err := NewEvaluator(btpParams, keys)
		if err != nil {
			return bkrTargetCountSweepResult{}, err
		}
		keyPrepElapsed += time.Since(start)
		materialBytes += int64(keys.BinarySize())

		ct, values, err := bkrTargetCountSweepInput(tc.Spec, residualParams, sk, targetLevel)
		if err != nil {
			return bkrTargetCountSweepResult{}, err
		}
		start = time.Now()
		out, err := eval.Bootstrap(ct)
		bootstrapElapsed += time.Since(start)
		if err != nil {
			return bkrTargetCountSweepResult{}, err
		}
		if _, err := bkrValidateBootstrapKeyReuseOutputs(t, "TargetCountSweep/original_lattigo", tc.Spec, targetLevel, targetLevel, residualParams, []rlwe.Ciphertext{*out}, [][]complex128{values}); err != nil {
			correctnessPass = false
			return bkrTargetCountSweepResult{}, err
		}
	}

	return bkrTargetCountSweepResultFromMetrics(tc, "original_lattigo", materialBytes, keyPrepElapsed, bootstrapElapsed, append([]int(nil), tc.TargetLevels...), tc.TargetCount, 0, correctnessPass), nil
}

func bkrRunTargetCountSweepPlanner(t testing.TB, tc bkrTargetCountSweepCase) (bkrTargetCountSweepResult, error) {
	t.Helper()
	fullParams, err := ckks.NewParametersFromLiteral(tc.Spec.SchemeParams)
	if err != nil {
		return bkrTargetCountSweepResult{}, err
	}
	sk, err := newDeterministicSecretKey(fullParams, tc.Spec.Seed+"/secret-key")
	if err != nil {
		return bkrTargetCountSweepResult{}, err
	}
	btpLiteral := bkrCloneBootstrappingParametersLiteral(tc.Spec.BootstrappingParams)
	if tc.Spec.LogSlots >= 0 {
		btpLiteral.LogSlots = bkrPointyInt(tc.Spec.LogSlots)
	}

	var plan *TargetLevelMaterialPlan
	var keys *ReusableEvaluationKeys
	var bootstrapper *TargetLevelBootstrapper
	start := time.Now()
	req := TargetLevelMaterialRequest{
		FullResidualParameters: fullParams,
		BootstrappingLiteral:   btpLiteral,
		FutureTargetLevels:     append([]int(nil), tc.TargetLevels...),
		ReusePolicy:            ReuseExactPrefixAndSupersetDrop,
		EnableRNSSliceViews:    true,
		AllowSupersetDrop:      true,
	}
	plan, err = NewTargetLevelMaterialPlan(req)
	if err != nil {
		return bkrTargetCountSweepResult{}, err
	}
	keys, err = plan.GenReusableEvaluationKeys(sk)
	if err != nil {
		return bkrTargetCountSweepResult{}, err
	}
	bootstrapper, err = NewTargetLevelBootstrapper(plan, keys)
	if err != nil {
		return bkrTargetCountSweepResult{}, err
	}
	keyPrepElapsed := time.Since(start)

	var bootstrapElapsed time.Duration
	correctnessPass := true
	for _, targetLevel := range tc.TargetLevels {
		residualParams, _, err := buildTargetBootstrappingParameters(tc.Spec, targetLevel)
		if err != nil {
			return bkrTargetCountSweepResult{}, err
		}
		ct, values, err := bkrTargetCountSweepInput(tc.Spec, residualParams, sk, targetLevel)
		if err != nil {
			return bkrTargetCountSweepResult{}, err
		}
		start = time.Now()
		out, err := bootstrapper.BootstrapAtLevel(ct, targetLevel)
		bootstrapElapsed += time.Since(start)
		if err != nil {
			return bkrTargetCountSweepResult{}, err
		}
		if _, err := bkrValidateBootstrapKeyReuseOutputs(t, "TargetCountSweep/ours_reusable_planner", tc.Spec, targetLevel, targetLevel, residualParams, []rlwe.Ciphertext{*out}, [][]complex128{values}); err != nil {
			correctnessPass = false
			return bkrTargetCountSweepResult{}, err
		}
	}

	report := bootstrapper.PlanReport()
	return bkrTargetCountSweepResultFromMetrics(tc, "ours_reusable_planner", bkrResearchPersistentKeyBytes(keys), keyPrepElapsed, bootstrapElapsed, report.OwnerTargetLevels, bkrResearchPhysicalMaterialCount(keys.SharedKeyPool()), bkrResearchCountMapSum(report.SharedCounts), correctnessPass), nil
}

func bkrTargetCountSweepResultFromMetrics(tc bkrTargetCountSweepCase, scheme string, materialBytes int64, keyPrepElapsed, bootstrapElapsed time.Duration, ownerTargetLevels []int, physicalMaterialCount, sharedMaterialCount int, correctnessPass bool) bkrTargetCountSweepResult {
	_, commit, dirty := bkrGitMetadata()
	return bkrTargetCountSweepResult{
		SchemaVersion:          bkrTargetCountSweepSchemaVersion,
		ProfileID:              tc.ProfileID,
		CaseID:                 tc.CaseID,
		TargetCount:            tc.TargetCount,
		TargetCountLabel:       tc.TargetCountLabel,
		TargetLevels:           append([]int(nil), tc.TargetLevels...),
		Scheme:                 scheme,
		MaterialTotalMB:        bkrBytesToMiB(materialBytes),
		KeyPreparationTimeS:    bkrDurationSeconds(keyPrepElapsed),
		TotalBootstrapTimeS:    bkrDurationSeconds(bootstrapElapsed),
		MeanBootstrapLatencyMS: bkrDurationMS(bootstrapElapsed) / float64(tc.TargetCount),
		OwnerTargetLevels:      append([]int(nil), ownerTargetLevels...),
		PhysicalMaterialCount:  physicalMaterialCount,
		SharedMaterialCount:    sharedMaterialCount,
		CorrectnessPass:        correctnessPass,
		GoVersion:              runtime.Version(),
		GOMAXPROCS:             runtime.GOMAXPROCS(0),
		Commit:                 commit,
		DirtyState:             dirty,
	}
}

func bkrTargetCountSweepInput(spec bkrCaseSpec, params ckks.Parameters, sk *rlwe.SecretKey, targetLevel int) (*rlwe.Ciphertext, []complex128, error) {
	targetSK := sk.CopyNew()
	targetSK.Value.Resize(params.MaxLevel(), params.MaxLevelP())

	encoder := ckks.NewEncoder(params)
	encryptor := rlwe.NewEncryptor(params, targetSK)
	values := bkrComplexValues(params, fmt.Sprintf("%s/target-count-sweep/target/%d/values", spec.Seed, targetLevel))
	plaintext := ckks.NewPlaintext(params, 0)
	if err := encoder.Encode(values, plaintext); err != nil {
		return nil, nil, err
	}

	ct, err := encryptor.EncryptNew(plaintext)
	if err != nil {
		return nil, nil, err
	}
	if ct.Level() != 0 {
		return nil, nil, fmt.Errorf("input target %d ciphertext level=%d, want 0", targetLevel, ct.Level())
	}
	return ct, values, nil
}

func BenchmarkResearchTargetLevelFinalAPI(b *testing.B) {
	for _, tc := range bkrResearchBenchmarkSuiteManifest() {
		if tc.Lane != "final_api" {
			continue
		}
		if !bkrResearchCaseEnabledForMode(tc, *bkrResearchBenchmarkMode) {
			continue
		}
		tc := tc
		b.Run(tc.Name, func(b *testing.B) {
			bkrBenchmarkResearchTargetLevelFinalAPI(b, tc)
		})
	}
}

func BenchmarkTargetCountSweep(b *testing.B) {
	for _, tc := range bkrTargetCountSweepCases() {
		if tc.LongOnly && !*flagLongTest {
			continue
		}
		tc := tc
		b.Run(tc.Name, func(b *testing.B) {
			for _, scheme := range []string{"original_lattigo", "ours_reusable_planner"} {
				scheme := scheme
				b.Run(scheme, func(b *testing.B) {
					bkrBenchmarkTargetCountSweepScheme(b, tc, scheme)
				})
			}
		})
	}
}

func bkrBenchmarkTargetCountSweepScheme(b *testing.B, tc bkrTargetCountSweepCase, scheme string) {
	b.ReportAllocs()

	var materialTotalMB float64
	var keyPreparationTimeS float64
	var totalBootstrapTimeS float64
	var meanBootstrapLatencyMS float64
	var physicalMaterialCount float64
	var sharedMaterialCount float64
	var correctnessPass bool
	var lastResult bkrTargetCountSweepResult

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := bkrRunTargetCountSweepScheme(b, tc, scheme)
		if err != nil {
			b.Fatal(err)
		}
		materialTotalMB += result.MaterialTotalMB
		keyPreparationTimeS += result.KeyPreparationTimeS
		totalBootstrapTimeS += result.TotalBootstrapTimeS
		meanBootstrapLatencyMS += result.MeanBootstrapLatencyMS
		physicalMaterialCount += float64(result.PhysicalMaterialCount)
		sharedMaterialCount += float64(result.SharedMaterialCount)
		correctnessPass = result.CorrectnessPass
		lastResult = result
	}
	b.StopTimer()

	n := float64(b.N)
	avg := lastResult
	avg.MaterialTotalMB = materialTotalMB / n
	avg.KeyPreparationTimeS = keyPreparationTimeS / n
	avg.TotalBootstrapTimeS = totalBootstrapTimeS / n
	avg.MeanBootstrapLatencyMS = meanBootstrapLatencyMS / n
	avg.PhysicalMaterialCount = int(physicalMaterialCount / n)
	avg.SharedMaterialCount = int(sharedMaterialCount / n)
	avg.CorrectnessPass = correctnessPass

	b.ReportMetric(avg.MaterialTotalMB, "material_total_mb")
	b.ReportMetric(avg.KeyPreparationTimeS, "key_prep_s")
	b.ReportMetric(avg.TotalBootstrapTimeS, "total_bootstrap_s")
	b.ReportMetric(avg.MeanBootstrapLatencyMS, "mean_bootstrap_ms")
	b.ReportMetric(float64(avg.PhysicalMaterialCount), "physical_materials")
	b.ReportMetric(float64(avg.SharedMaterialCount), "shared_materials")
	if correctnessPass {
		b.ReportMetric(1, "correctness_pass")
	} else {
		b.ReportMetric(0, "correctness_pass")
	}

	if *bkrTargetCountSweepResultDir != "" {
		path := filepath.Join(*bkrTargetCountSweepResultDir, "target_count_sweep_results.csv")
		if err := bkrAppendTargetCountSweepCSV(path, avg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkResearchPrefixOnlyVsPlanner(b *testing.B) {
	for _, tc := range bkrResearchPrefixComparisonCases() {
		tc := tc
		b.Run(tc.Name, func(b *testing.B) {
			bkrBenchmarkResearchPrefixComparisonCase(b, tc)
		})
	}
}

func bkrBenchmarkResearchPrefixComparisonCase(b *testing.B, tc bkrResearchPrefixComparisonCase) {
	req, err := bkrResearchPrefixComparisonRequest(tc)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()

	var report TargetLevelPlanReport
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		plan, err := NewTargetLevelMaterialPlan(req)
		switch tc.ExpectedStatus {
		case "rejected":
			if !errors.Is(err, ErrTargetLevelUnavailable) {
				b.Fatalf("prefix-only plan error=%v, want ErrTargetLevelUnavailable", err)
			}
		case "executable":
			if err != nil {
				b.Fatalf("planner plan error=%v, want success", err)
			}
			report = plan.PlanReport()
		default:
			b.Fatalf("unknown expected status %q", tc.ExpectedStatus)
		}
	}
	b.StopTimer()

	switch tc.ExpectedStatus {
	case "rejected":
		b.ReportMetric(0, "executable_targets")
		b.ReportMetric(1, "prefix_rejections")
		b.ReportMetric(float64(len(tc.OwnerTargetLevels)), "owner_target_count")
		b.ReportMetric(0, "shared_material_count")
	case "executable":
		b.ReportMetric(float64(len(report.FutureTargetLevels)), "executable_targets")
		b.ReportMetric(0, "prefix_rejections")
		b.ReportMetric(float64(len(report.OwnerTargetLevels)), "owner_target_count")
		b.ReportMetric(float64(bkrResearchCountMapSum(report.SharedCounts)), "shared_material_count")
	}
}

func bkrBenchmarkResearchTargetLevelFinalAPI(b *testing.B, tc bkrResearchBenchmarkCase) {
	if tc.LongOnly && !*flagLongTest {
		b.Skip("long research benchmark; rerun with -args -long -bkr.research-mode=exhaustive")
	}
	b.ReportAllocs()

	req, err := bkrResearchTargetLevelRequest(tc)
	if err != nil {
		b.Fatal(err)
	}

	start := time.Now()
	plan, err := NewTargetLevelMaterialPlan(req)
	if err != nil {
		b.Fatal(err)
	}
	planElapsed := time.Since(start)

	fullParams, err := ckks.NewParametersFromLiteral(tc.Spec.SchemeParams)
	if err != nil {
		b.Fatal(err)
	}
	sk, err := newDeterministicSecretKey(fullParams, tc.Spec.Seed+"/secret-key")
	if err != nil {
		b.Fatal(err)
	}

	start = time.Now()
	keys, err := plan.GenReusableEvaluationKeys(sk)
	if err != nil {
		b.Fatal(err)
	}
	keygenElapsed := time.Since(start)

	start = time.Now()
	bootstrapper, err := NewTargetLevelBootstrapper(plan, keys)
	if err != nil {
		b.Fatal(err)
	}
	evaluatorElapsed := time.Since(start)

	type input struct {
		level int
		ct    *rlwe.Ciphertext
	}
	inputs := make([]input, 0, len(tc.FutureTargetLevels))
	for _, targetLevel := range tc.FutureTargetLevels {
		params, _, err := buildTargetBootstrappingParameters(tc.Spec, targetLevel)
		if err != nil {
			b.Fatal(err)
		}
		ct, _ := targetLevelInputCiphertextForBenchmark(b, tc.Spec, params, sk, targetLevel)
		inputs = append(inputs, input{level: targetLevel, ct: ct})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, in := range inputs {
			b.StopTimer()
			ct := in.ct.CopyNew()
			b.StartTimer()
			out, err := bootstrapper.BootstrapAtLevel(ct, in.level)
			if err != nil {
				b.Fatal(err)
			}
			if out.Level() != in.level {
				b.Fatalf("target %d output level=%d", in.level, out.Level())
			}
		}
	}

	report := bootstrapper.PlanReport()
	b.ReportMetric(bkrDurationMS(planElapsed), "plan_ms/setup")
	b.ReportMetric(bkrDurationMS(keygenElapsed), "keygen_ms/setup")
	b.ReportMetric(bkrDurationMS(evaluatorElapsed), "evaluator_ms/setup")
	b.ReportMetric(float64(bkrResearchPersistentKeyBytes(keys)), "persistent_key_bytes")
	b.ReportMetric(float64(bkrResearchPhysicalMaterialCount(keys.SharedKeyPool())), "physical_materials")
	b.ReportMetric(float64(bkrResearchLogicalViewCount(keys.SharedKeyPool())), "logical_views")
	b.ReportMetric(float64(bkrResearchCountMapSum(report.SharedCounts)), "shared_materials")
}

func targetLevelInputCiphertextForBenchmark(b *testing.B, spec bkrCaseSpec, params ckks.Parameters, sk *rlwe.SecretKey, targetLevel int) (*rlwe.Ciphertext, []complex128) {
	b.Helper()

	targetSK := sk.CopyNew()
	targetSK.Value.Resize(params.MaxLevel(), params.MaxLevelP())

	encoder := ckks.NewEncoder(params)
	encryptor := rlwe.NewEncryptor(params, targetSK)
	values := bkrComplexValues(params, spec.Seed+"/research-benchmark/values")
	plaintext := ckks.NewPlaintext(params, 0)
	if err := encoder.Encode(values, plaintext); err != nil {
		b.Fatal(err)
	}

	ct, err := encryptor.EncryptNew(plaintext)
	if err != nil {
		b.Fatal(err)
	}
	if ct.Level() != 0 {
		b.Fatalf("input target %d ciphertext level=%d, want 0", targetLevel, ct.Level())
	}
	return ct, values
}

func bkrResearchTargetLevelRequest(tc bkrResearchBenchmarkCase) (TargetLevelMaterialRequest, error) {
	fullParams, err := ckks.NewParametersFromLiteral(tc.Spec.SchemeParams)
	if err != nil {
		return TargetLevelMaterialRequest{}, err
	}
	btpLiteral := bkrCloneBootstrappingParametersLiteral(tc.Spec.BootstrappingParams)
	if tc.Spec.LogSlots >= 0 {
		btpLiteral.LogSlots = bkrPointyInt(tc.Spec.LogSlots)
	}
	policy, err := bkrResearchReusePolicy(tc.Policy)
	if err != nil {
		return TargetLevelMaterialRequest{}, err
	}
	return TargetLevelMaterialRequest{
		FullResidualParameters: fullParams,
		BootstrappingLiteral:   btpLiteral,
		FutureTargetLevels:     append([]int(nil), tc.FutureTargetLevels...),
		OwnerTargetLevels:      append([]int(nil), tc.OwnerTargetLevels...),
		ReusePolicy:            policy,
		EnableRNSSliceViews:    tc.EnableRNSSliceViews,
		AllowSupersetDrop:      tc.AllowSupersetDrop,
	}, nil
}

func bkrResearchPrefixComparisonRequest(tc bkrResearchPrefixComparisonCase) (TargetLevelMaterialRequest, error) {
	fullParams, err := ckks.NewParametersFromLiteral(tc.Spec.SchemeParams)
	if err != nil {
		return TargetLevelMaterialRequest{}, err
	}
	policy, err := bkrResearchReusePolicy(tc.Policy)
	if err != nil {
		return TargetLevelMaterialRequest{}, err
	}
	return TargetLevelMaterialRequest{
		FullResidualParameters: fullParams,
		BootstrappingLiteral:   tc.Spec.BootstrappingParams,
		FutureTargetLevels:     append([]int(nil), tc.FutureTargetLevels...),
		OwnerTargetLevels:      append([]int(nil), tc.OwnerTargetLevels...),
		ReusePolicy:            policy,
		EnableRNSSliceViews:    tc.EnableRNSSliceViews,
		AllowSupersetDrop:      tc.AllowSupersetDrop,
	}, nil
}

func bkrResearchReusePolicy(policy string) (ReusePolicy, error) {
	switch policy {
	case "exact_only":
		return ReuseExactOnly, nil
	case "exact_prefix":
		return ReuseExactAndPrefix, nil
	case "exact_prefix_superset_drop":
		return ReuseExactPrefixAndSupersetDrop, nil
	default:
		return 0, fmt.Errorf("unknown research reuse policy %q", policy)
	}
}

func bkrResearchPrefixComparisonCaseByName(cases []bkrResearchPrefixComparisonCase, name string) (bkrResearchPrefixComparisonCase, bool) {
	for _, tc := range cases {
		if tc.Name == name {
			return tc, true
		}
	}
	return bkrResearchPrefixComparisonCase{}, false
}

func bkrResearchBenchmarkCaseByName(manifest []bkrResearchBenchmarkCase, name string) (bkrResearchBenchmarkCase, bool) {
	for _, tc := range manifest {
		if tc.Name == name {
			return tc, true
		}
	}
	return bkrResearchBenchmarkCase{}, false
}

func bkrResearchCaseEnabledForMode(tc bkrResearchBenchmarkCase, mode string) bool {
	switch mode {
	case "smoke":
		return tc.Tier == "smoke"
	case "standard", "":
		return tc.Tier == "smoke" || tc.Tier == "standard"
	case "exhaustive":
		return true
	default:
		return tc.Tier == "smoke"
	}
}

func (tc bkrResearchBenchmarkCase) csvRow() []string {
	return []string{
		bkrResearchBenchmarkSchemaVersion,
		tc.Name,
		tc.Lane,
		tc.CaseID,
		tc.ProfileID,
		tc.TargetShape,
		bkrJoinInts(tc.FutureTargetLevels),
		bkrJoinInts(tc.OwnerTargetLevels),
		tc.Policy,
		strconv.FormatBool(tc.EnableRNSSliceViews),
		strconv.FormatBool(tc.AllowSupersetDrop),
		strconv.FormatBool(tc.LongOnly),
		strconv.Itoa(tc.GOMAXPROCS),
		strconv.Itoa(tc.Count),
		strings.Join(tc.Phases, ";"),
		tc.CorrectnessGate,
		tc.ProfileGate,
	}
}

func (r bkrResearchBenchmarkResult) csvRow() []string {
	return []string{
		r.SchemaVersion,
		r.Name,
		r.Lane,
		r.CaseID,
		r.ProfileID,
		r.TargetShape,
		bkrJoinInts(r.TargetLevels),
		bkrJoinInts(r.OwnerTargetLevels),
		r.Policy,
		bkrFormatFloat(r.PlanMS),
		bkrFormatFloat(r.KeygenMS),
		bkrFormatFloat(r.EvaluatorConstructionMS),
		bkrFormatFloat(r.BootstrapMS),
		bkrFormatFloat(r.BootstrapManyMS),
		bkrFormatFloat(r.ReportGenerationMS),
		strconv.FormatInt(r.PersistentKeyBytes, 10),
		strconv.Itoa(r.PhysicalMaterialCount),
		strconv.Itoa(r.LogicalViewCount),
		strconv.Itoa(r.SharedCount),
		strconv.FormatBool(r.OutputLevelEquality),
		strconv.FormatBool(r.OutputScaleEquality),
		bkrFormatFloat(r.AverageLog2PrecisionReal),
		bkrFormatFloat(r.AverageLog2PrecisionImag),
		bkrFormatFloat(r.PrecisionThresholdBits),
		r.GoVersion,
		strconv.Itoa(r.GOMAXPROCS),
		r.Commit,
		r.DirtyState,
	}
}

func (r bkrTargetCountSweepResult) csvRow() []string {
	return []string{
		r.SchemaVersion,
		r.ProfileID,
		r.CaseID,
		strconv.Itoa(r.TargetCount),
		r.TargetCountLabel,
		bkrJoinInts(r.TargetLevels),
		r.Scheme,
		bkrFormatFloat(r.MaterialTotalMB),
		bkrFormatFloat(r.KeyPreparationTimeS),
		bkrFormatFloat(r.TotalBootstrapTimeS),
		bkrFormatFloat(r.MeanBootstrapLatencyMS),
		bkrJoinInts(r.OwnerTargetLevels),
		strconv.Itoa(r.PhysicalMaterialCount),
		strconv.Itoa(r.SharedMaterialCount),
		strconv.FormatBool(r.CorrectnessPass),
		r.GoVersion,
		strconv.Itoa(r.GOMAXPROCS),
		r.Commit,
		r.DirtyState,
	}
}

func bkrBytesToMiB(bytes int64) float64 {
	return float64(bytes) / (1024 * 1024)
}

func bkrDurationSeconds(d time.Duration) float64 {
	return float64(d) / float64(time.Second)
}

func bkrResearchPersistentKeyBytes(keys *ReusableEvaluationKeys) int64 {
	if keys == nil || keys.pool == nil {
		return 0
	}
	var total int64
	for _, key := range keys.pool.evaluationKeys {
		if key != nil {
			total += int64(key.BinarySize())
		}
	}
	return total
}

func bkrResearchPhysicalMaterialCount(pool *SharedKeyPool) int {
	if pool == nil {
		return 0
	}
	count := 0
	for _, id := range pool.MaterialIDs() {
		if id.View == "physical" {
			count++
		}
	}
	return count
}

func bkrResearchLogicalViewCount(pool *SharedKeyPool) int {
	if pool == nil {
		return 0
	}
	count := 0
	for _, id := range pool.MaterialIDs() {
		if id.View != "" && id.View != "physical" {
			count++
		}
	}
	return count
}

func bkrResearchCountMapSum(counts map[string]int) int {
	total := 0
	for _, count := range counts {
		total += count
	}
	return total
}
