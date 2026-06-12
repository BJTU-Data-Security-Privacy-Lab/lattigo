package bootstrapping

import (
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestResearchBenchmarkSuiteManifestCoversRequiredAxes(t *testing.T) {
	manifest := bkrResearchBenchmarkSuiteManifest()
	if len(manifest) == 0 {
		t.Fatal("research benchmark manifest is empty")
	}

	profiles := map[string]bool{}
	shapes := map[string]bool{}
	lanes := map[string]bool{}
	policies := map[string]bool{}
	phases := map[string]bool{}
	for _, tc := range manifest {
		profiles[tc.ProfileID] = true
		shapes[tc.TargetShape] = true
		lanes[tc.Lane] = true
		policies[tc.Policy] = true
		for _, phase := range tc.Phases {
			phases[phase] = true
		}
		if tc.Name == "" || tc.CaseID == "" {
			t.Fatalf("manifest entry has empty identity: %+v", tc)
		}
		if len(tc.FutureTargetLevels) == 0 {
			t.Fatalf("manifest entry %s has no future target levels", tc.Name)
		}
	}

	for _, want := range []string{"P0_TINY_NATIVE", "P1_TINY_RING_SWITCH", "P2_MULTI_FAST", "P3_N15_SPARSE", "P4_N15_DENSE", "P5_N16_SPARSE_LONG", "P6_N16_DENSE_LONG"} {
		if !profiles[want] {
			t.Fatalf("research benchmark manifest missing profile %s", want)
		}
	}
	for _, want := range []string{"single", "clustered", "sparse", "random", "all_legal"} {
		if !shapes[want] {
			t.Fatalf("research benchmark manifest missing target shape %s", want)
		}
	}
	for _, want := range []string{"a0_oracle", "a1_used_only", "a2_rotation_pool", "a3_schedule_pool", "a4_diagonal_pool", "a5_rns_slice", "a6_superset_drop", "final_api"} {
		if !lanes[want] {
			t.Fatalf("research benchmark manifest missing lane %s", want)
		}
	}
	for _, want := range []string{"exact_only", "exact_prefix", "exact_prefix_superset_drop"} {
		if !policies[want] {
			t.Fatalf("research benchmark manifest missing policy %s", want)
		}
	}
	for _, want := range []string{"plan", "keygen", "evaluator_construction", "bootstrap", "bootstrap_many", "parallel_bootstrap", "report_generation", "oracle_validation"} {
		if !phases[want] {
			t.Fatalf("research benchmark manifest missing measured phase %s", want)
		}
	}
}

func TestResearchBenchmarkConfidenceAuditIsFullyMitigated(t *testing.T) {
	audit := bkrResearchBenchmarkConfidenceAudit()
	if len(audit) == 0 {
		t.Fatal("research benchmark confidence audit is empty")
	}
	for _, item := range audit {
		if item.ID == "" || item.Loophole == "" || item.Mitigation == "" || item.Evidence == "" {
			t.Fatalf("audit item has an empty field: %+v", item)
		}
		if !item.Mitigated {
			t.Fatalf("audit item %s remains unmitigated: %+v", item.ID, item)
		}
	}
}

func TestResearchBenchmarkCSVHeadersAreStable(t *testing.T) {
	if got, want := bkrResearchBenchmarkManifestCSVHeader(), []string{
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
	}; !slices.Equal(got, want) {
		t.Fatalf("manifest CSV header=%v, want %v", got, want)
	}

	if got, want := bkrResearchBenchmarkResultCSVHeader(), []string{
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
	}; !slices.Equal(got, want) {
		t.Fatalf("result CSV header=%v, want %v", got, want)
	}
}

func TestResearchBenchmarkSmokeReportWritesCSV(t *testing.T) {
	dir := t.TempDir()
	result, err := bkrWriteResearchBenchmarkSmokeReport(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OutputLevelEquality || !result.OutputScaleEquality {
		t.Fatalf("smoke report result failed correctness checks: %+v", result)
	}
	if result.PersistentKeyBytes <= 0 {
		t.Fatalf("PersistentKeyBytes=%d, want >0", result.PersistentKeyBytes)
	}

	assertResearchBenchmarkCSVFiles(t, dir)

	if *bkrResearchBenchmarkResultDir != "" {
		if _, err := bkrWriteResearchBenchmarkSmokeReport(t, *bkrResearchBenchmarkResultDir); err != nil {
			t.Fatalf("cannot write research benchmark report to %s: %v", *bkrResearchBenchmarkResultDir, err)
		}
		assertResearchBenchmarkCSVFiles(t, *bkrResearchBenchmarkResultDir)
	}
}

func TestResearchBenchmarkPrefixComparisonContrastsPrefixOnlyWithPlanner(t *testing.T) {
	cases := bkrResearchPrefixComparisonCases()
	prefixOnly, ok := bkrResearchPrefixComparisonCaseByName(cases, "prefix_only_p2_sparse_rejected")
	if !ok {
		t.Fatal("missing prefix_only_p2_sparse_rejected comparison case")
	}
	planner, ok := bkrResearchPrefixComparisonCaseByName(cases, "planner_p2_sparse_superset_success")
	if !ok {
		t.Fatal("missing planner_p2_sparse_superset_success comparison case")
	}

	for _, tc := range []bkrResearchPrefixComparisonCase{prefixOnly, planner} {
		for _, want := range []string{"plan_ns_per_op", "executable_targets", "prefix_rejections", "owner_target_count", "shared_material_count"} {
			if !slices.Contains(tc.PerformanceMetrics, want) {
				t.Fatalf("%s missing performance metric %s: %v", tc.Name, want, tc.PerformanceMetrics)
			}
		}
		if got, want := tc.FutureTargetLevels, []int{1, 3}; !slices.Equal(got, want) {
			t.Fatalf("%s FutureTargetLevels=%v, want %v", tc.Name, got, want)
		}
		if got, want := tc.OwnerTargetLevels, []int{3}; !slices.Equal(got, want) {
			t.Fatalf("%s OwnerTargetLevels=%v, want %v", tc.Name, got, want)
		}
	}

	prefixReq, err := bkrResearchPrefixComparisonRequest(prefixOnly)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewTargetLevelMaterialPlan(prefixReq)
	if !errors.Is(err, ErrTargetLevelUnavailable) {
		t.Fatalf("prefix-only NewTargetLevelMaterialPlan error=%v, want ErrTargetLevelUnavailable", err)
	}
	if !strings.Contains(err.Error(), "rns_prefix_bootstrapping_q_prefix_mismatch_2") {
		t.Fatalf("prefix-only rejection error=%q, want q-prefix mismatch evidence", err)
	}

	plannerReq, err := bkrResearchPrefixComparisonRequest(planner)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewTargetLevelMaterialPlan(plannerReq)
	if err != nil {
		t.Fatalf("planner NewTargetLevelMaterialPlan error=%v, want success", err)
	}
	report := plan.PlanReport()
	if got, want := report.OwnerTargetLevels, []int{3}; !slices.Equal(got, want) {
		t.Fatalf("planner OwnerTargetLevels=%v, want %v", got, want)
	}
	if got := report.RejectedCandidateReasons["rns_prefix_bootstrapping_q_prefix_mismatch_2"]; got == 0 {
		t.Fatalf("planner rejected prefix reasons=%v, want q-prefix mismatch evidence", report.RejectedCandidateReasons)
	}
	foundSupersetTarget := false
	for _, target := range report.Targets {
		if target.TargetLevel == 1 {
			foundSupersetTarget = true
			if target.Strategy != "superset_output_drop" {
				t.Fatalf("planner target 1 strategy=%q, want superset_output_drop", target.Strategy)
			}
			if target.SharedCounts[string(MaterialKindEvaluationKey)] != 1 {
				t.Fatalf("planner target 1 shared evaluation keys=%d, want 1", target.SharedCounts[string(MaterialKindEvaluationKey)])
			}
		}
	}
	if !foundSupersetTarget {
		t.Fatal("planner report missing target level 1")
	}
}

func TestTargetCountSweepCasesCoverPaperAxes(t *testing.T) {
	cases := bkrTargetCountSweepCases()
	if len(cases) == 0 {
		t.Fatal("target-count sweep cases are empty")
	}

	byProfile := map[string]map[string]bkrTargetCountSweepCase{}
	for _, tc := range cases {
		if tc.Name == "" || tc.ProfileID == "" || tc.TargetCountLabel == "" {
			t.Fatalf("case has empty identity: %+v", tc)
		}
		if tc.TargetCount != len(tc.TargetLevels) {
			t.Fatalf("%s TargetCount=%d, want len(TargetLevels)=%d", tc.Name, tc.TargetCount, len(tc.TargetLevels))
		}
		if !slices.IsSorted(tc.TargetLevels) {
			t.Fatalf("%s TargetLevels not sorted: %v", tc.Name, tc.TargetLevels)
		}
		if byProfile[tc.ProfileID] == nil {
			byProfile[tc.ProfileID] = map[string]bkrTargetCountSweepCase{}
		}
		byProfile[tc.ProfileID][tc.TargetCountLabel] = tc
	}

	for _, label := range []string{"1", "2", "3", "all"} {
		if _, ok := byProfile["P4_N15_DENSE"][label]; !ok {
			t.Fatalf("missing P4_N15_DENSE target-count sweep label %s", label)
		}
	}
	for _, label := range []string{"1", "2", "4", "8", "all"} {
		if _, ok := byProfile["P6_N16_DENSE_LONG"][label]; !ok {
			t.Fatalf("missing P6_N16_DENSE_LONG target-count sweep label %s", label)
		}
	}
}

func TestTargetCountSweepCSVHeaderIsStable(t *testing.T) {
	if got, want := bkrTargetCountSweepCSVHeader(), []string{
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
	}; !slices.Equal(got, want) {
		t.Fatalf("target-count sweep CSV header=%v, want %v", got, want)
	}
}

func TestTargetCountSweepSmokeReportWritesOriginalAndPlannerRows(t *testing.T) {
	dir := t.TempDir()
	rows, err := bkrWriteTargetCountSweepSmokeReport(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("smoke rows=%d, want original and planner rows", len(rows))
	}

	path := filepath.Join(dir, "target_count_sweep_results.csv")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("missing target-count sweep CSV: %v", err)
	}
	records, err := csv.NewReader(f).ReadAll()
	closeErr := f.Close()
	if err != nil {
		t.Fatalf("cannot read target-count sweep CSV: %v", err)
	}
	if closeErr != nil {
		t.Fatalf("cannot close target-count sweep CSV: %v", closeErr)
	}
	if len(records) != 3 {
		t.Fatalf("CSV rows=%d, want header plus 2 rows", len(records))
	}

	header := records[0]
	schemeCol := slices.Index(header, "scheme")
	materialCol := slices.Index(header, "material_total_mb")
	keyPrepCol := slices.Index(header, "key_preparation_time_s")
	bootstrapCol := slices.Index(header, "total_bootstrap_time_s")
	if schemeCol < 0 || materialCol < 0 || keyPrepCol < 0 || bootstrapCol < 0 {
		t.Fatalf("CSV header missing table columns: %v", header)
	}

	seen := map[string]bool{}
	for _, record := range records[1:] {
		seen[record[schemeCol]] = true
		for _, col := range []int{materialCol, keyPrepCol, bootstrapCol} {
			value, err := strconv.ParseFloat(record[col], 64)
			if err != nil {
				t.Fatalf("cannot parse %s in row %v: %v", header[col], record, err)
			}
			if value <= 0 {
				t.Fatalf("%s=%f, want >0 in row %v", header[col], value, record)
			}
		}
	}
	for _, scheme := range []string{"original_lattigo", "ours_reusable_planner"} {
		if !seen[scheme] {
			t.Fatalf("missing scheme %s in target-count sweep CSV rows", scheme)
		}
	}
}

func assertResearchBenchmarkCSVFiles(t *testing.T, dir string) {
	t.Helper()
	for _, file := range []string{"research_benchmark_manifest.csv", "research_benchmark_results.csv", "research_benchmark_confidence_audit.csv"} {
		path := filepath.Join(dir, file)
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("missing research benchmark output %s: %v", file, err)
		}
		rows, err := csv.NewReader(f).ReadAll()
		closeErr := f.Close()
		if err != nil {
			t.Fatalf("cannot read %s: %v", file, err)
		}
		if closeErr != nil {
			t.Fatalf("cannot close %s: %v", file, closeErr)
		}
		if len(rows) < 2 {
			t.Fatalf("%s has %d rows, want header plus data", file, len(rows))
		}
	}
}
