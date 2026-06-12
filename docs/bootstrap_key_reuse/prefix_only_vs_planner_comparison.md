# RNS Prefix-Only vs Target-Level Planner Comparison

本文档对比“单纯 RNS prefix 截断方案”和 target-level reusable bootstrap
key planner 的能力边界。这里的 prefix-only 指：

- 预先选择一个较高 owner target level；
- 只允许把 owner evaluation key material 截成低 level RNS prefix view；
- 不允许 superset output `DropLevel`；
- 不做 A0-A6 material graph planning；
- 不为 prefix 不兼容场景提供其他 owner/view fallback。

> **Status note, 2026-06-12:** 本文的历史 benchmark 输出来自 A7
> key-material-pool target evaluator 降维之前。它仍可用于说明 prefix-only
> 的功能边界，但不能作为最终 A7 性能结论引用。最终 A7 复用边界只包含
> relinearization、Galois、ring/domain switch 和 dense/sparse key material；
> A7 性能报告使用 `key_material_total_mb`，不使用旧
> `physical_materials`、`logical_views` 或 `shared_materials` 作为源口径。

当前 API 入口：

- `NewTargetLevelMaterialPlan`
- `(*TargetLevelMaterialPlan).GenReusableEvaluationKeys`
- `NewTargetLevelBootstrapper`
- `(*TargetLevelBootstrapper).BootstrapAtLevel`
- `(*TargetLevelBootstrapper).BootstrapManyAtLevel`
- `(*TargetLevelBootstrapper).PlanReport`

相关文档：

- [research_benchmark_suite_spec.md](research_benchmark_suite_spec.md)
- [research_benchmark_suite_cl_test_plan.md](research_benchmark_suite_cl_test_plan.md)
- [final_target_level_reuse_spec.md](final_target_level_reuse_spec.md)

## 1. Detailed Comparison Table

| Dimension | RNS prefix-only | Current target-level planner | Functional test evidence | Performance test evidence |
| --- | --- | --- | --- | --- |
| Core idea | Reuse one higher-level key object by slicing its RNS `Q` prefix for a lower level. | Build a target-level material plan over declared future target levels, then choose exact, prefix, or superset/drop owners per target. | `TestRNSPrefixViewAllowsEvaluationKeyPrefix` proves raw evaluation-key prefix view construction; `TestTargetLevelMaterialPlanSelectsSupersetOwnerWhenAllowed` proves planner owner selection. | `BenchmarkResearchPrefixOnlyVsPlanner` compares the same P2 sparse workload under prefix-only and planner policies. |
| User API model | Usually implicit: caller owns a high-level object and tries to slice it. | Explicit: caller declares `FutureTargetLevels`, optional `OwnerTargetLevels`, `ReusePolicy`, `EnableRNSSliceViews`, and `AllowSupersetDrop`. | `TestTargetLevelMaterialPlanRejectsUnconfiguredTargetLevel` and `TestTargetLevelMaterialPlanDeduplicatesAndSortsFutureTargets`. | Plan metrics report `owner_target_count`, `executable_targets`, and `prefix_rejections`. |
| Legal target coverage | Only works when owner and target CKKS bootstrapping/residual parameters are prefix-compatible. | Works when exact owner exists, prefix is compatible, or superset owner can bootstrap and drop output to requested target. | `TestTargetLevelMaterialPlanRejectsIncompatibleRNSSliceForSupportedProfiles` and `TestTargetLevelBootstrapperSupersetDropMatchesDirectTarget`. | Prefix-only P2 sparse row reports `executable_targets=0`; planner row reports `executable_targets=2`. |
| Handling prefix incompatibility | Rejects or must silently fallback outside the prefix-only model. | Records prefix rejection reason and selects a legal alternate strategy when policy allows it. | `TestResearchBenchmarkPrefixComparisonContrastsPrefixOnlyWithPlanner` checks `rns_prefix_bootstrapping_q_prefix_mismatch_2` plus planner `superset_output_drop`. | Same benchmark reports `prefix_rejections=1` for prefix-only and `shared_material_count=1` for planner. |
| Output target level semantics | Prefix view alone does not solve cases where evaluator output level differs from requested target level. | `BootstrapAtLevel` validates output level and uses `DropLevel` only when the policy explicitly permits superset output. | `TestTargetLevelBootstrapperSupersetDropMatchesDirectTarget` validates direct target equivalence; `TestTargetLevelBootstrapperRNSSliceDispatchRequiresPrefixEvaluator` guards dispatch. | Full final API benchmark `BenchmarkResearchTargetLevelFinalAPI/final_p2_superset_sparse` measures end-to-end bootstrap for the planner path. |
| Material reuse scope | RNS-limb view of compatible key material only. | Historical planner evidence covers exact reuse, RNS prefix view, and superset/drop owner view. Final A7 narrows physical reuse to key material only and uses dedicated target evaluators. | `TestTargetLevelPlanReportAccountsSupersetSharedView` and `TestTargetLevelPlanReportCountsGeneratedMaterialKinds` remain historical A6 evidence; A7 adds key-material-pool tests. | Current A7 reports `key_material_total_mb`, shared/private key material counts, and `target_evaluator_count`. |
| Unused target levels | Prefix-only does not by itself model compiler-used vs unused target levels. | Planner only generates material for declared future/owner target levels and rejects undeclared runtime requests. | A1 tests in the A0-A6 harness and `TestTargetLevelMaterialPlanRejectsUnconfiguredTargetLevel`. | Standard CL gate compares A0-A6 result directories and final API benchmark rows. |
| Correctness gates | Needs separate checks to prove sliced key is accepted by evaluator and preserves precision/scale. | Correctness is attached to every benchmark case and checked by output level, output scale, and precision validation. | `TestResearchBenchmarkSuiteManifestCoversRequiredAxes` requires correctness gates; `TestResearchBenchmarkSmokeReportWritesCSV` validates smoke output. | `research_benchmark_results.csv` records output equality and precision columns. |
| Failure observability | A raw prefix attempt can fail without structured reason unless instrumented separately. | Plan report and benchmark comparison expose rejected reason counts. | `TestTargetLevelPlanReportIncludesRejectedPrefixCandidateReasons`. | `BenchmarkResearchPrefixOnlyVsPlanner` exposes `prefix_rejections`. |
| Research suitability | Useful micro-optimization baseline, but too narrow as the full method. | Suitable as research method because it exposes workload, policy, material accounting, correctness, and environment. | Research suite manifest covers P0-P6, shapes, A0-A6 lanes, final API, and policies. | Research CL plan separates smoke, standard scalar, exhaustive long, and profiler lanes. |

## 2. Functional Test Matrix

| Test | Prefix-only expectation | Current planner expectation | Why it matters |
| --- | --- | --- | --- |
| `TestRNSPrefixViewAllowsEvaluationKeyPrefix` | Pass: raw RLWE evaluation key can expose a lower RNS prefix view when parameters are compatible. | N/A; this is the primitive the planner may use. | Proves prefix view is a real mechanism, not just documentation. |
| `TestTargetLevelMaterialPlanRejectsIncompatibleRNSSliceForSupportedProfiles` | Reject incompatible cross-target prefix candidates. | N/A unless policy permits alternate strategy. | Prevents the false claim that integer target levels alone imply prefix legality. |
| `TestResearchBenchmarkPrefixComparisonContrastsPrefixOnlyWithPlanner` | Reject P2 sparse `[1,3]` with owner `[3]`, reason `rns_prefix_bootstrapping_q_prefix_mismatch_2`. | Succeed on the same workload via `superset_output_drop`, with owner `[3]` and shared evaluation key count `1`. | Directly compares the two schemes under the same target set and owner selection. |
| `TestTargetLevelBootstrapperSupersetDropMatchesDirectTarget` | Cannot cover this behavior; prefix-only disallows superset/drop. | Bootstrap through owner then drop to requested target, matching direct target validation. | Captures the main functional advantage beyond prefix slicing. |
| `TestResearchBenchmarkSuiteManifestCoversRequiredAxes` | Prefix-only appears only as a comparison baseline. | Full suite covers final API plus A0-A6 material reuse lanes. | Ensures benchmark coverage does not collapse into a single prefix microcase. |

## 3. Performance Test Matrix

| Command | Compared schemes | Workload | Required differentiating metrics | Expected interpretation |
| --- | --- | --- | --- | --- |
| `go test ./circuits/ckks/bootstrapping -run '^$' -bench '^BenchmarkResearchPrefixOnlyVsPlanner' -benchmem -count=1 -timeout=30m` | prefix-only vs current planner | P2 sparse target levels `[1,3]`, owner `[3]` | `executable_targets`, `prefix_rejections`, `owner_target_count`, `shared_material_count`, `ns/op`, `B/op`, `allocs/op` | Prefix-only should show no executable targets and one prefix rejection; planner should show two executable targets and one shared material count. |
| `go test ./circuits/ckks/bootstrapping -run '^$' -bench '^BenchmarkResearchTargetLevelFinalAPI/final_p2_superset_sparse' -benchmem -count=1 -timeout=30m -args -bkr.research-mode=standard` | current planner only | Same P2 sparse target shape | `plan_ms/setup`, `keygen_ms/setup`, `evaluator_ms/setup`, `persistent_key_bytes`, `key_material_total_mb`, key-material counts, `target_evaluator_count`, `ns/op` | Measures end-to-end cost of the planner path that prefix-only cannot execute for this workload. |
| `go test ./circuits/ckks/bootstrapping -run 'TestResearchBenchmarkPrefixComparison|TestResearchBenchmarkSmokeReportWritesCSV' -count=1 -timeout=30m` | functional gate for both | Comparison case plus smoke report | prefix rejection reason, planner strategy, CSV artifacts | Ensures the performance comparison is backed by a correctness/capability distinction. |

## 4. First Observed Differential Result

Command:

```bash
env GOCACHE=/tmp/go-cache go test ./circuits/ckks/bootstrapping -run '^$' -bench '^BenchmarkResearchPrefixOnlyVsPlanner' -benchmem -count=1 -timeout=30m
```

Observed output on 2026-05-31:

```text
BenchmarkResearchPrefixOnlyVsPlanner/prefix_only_p2_sparse_rejected-48      13 84677772 ns/op 0 executable_targets 1.000 owner_target_count 1.000 prefix_rejections 0 shared_material_count 45792050 B/op 2499205 allocs/op
BenchmarkResearchPrefixOnlyVsPlanner/planner_p2_sparse_superset_success-48 13 84589611 ns/op 2.000 executable_targets 1.000 owner_target_count 0 prefix_rejections 1.000 shared_material_count 45794577 B/op 2499225 allocs/op
```

Interpretation:

- Prefix-only is not merely slower or faster here; it cannot serve the workload because the owner/target bootstrapping Q chain is not prefix-compatible.
- The current planner observes the same prefix incompatibility, records it, and still serves both target levels through `superset_output_drop`.
- Therefore the key differentiating performance/capability metric is not just `ns/op`; it is the tuple `(executable_targets, prefix_rejections, shared_material_count)` under the same workload.
- End-to-end bootstrap latency for the successful planner path must be measured with `BenchmarkResearchTargetLevelFinalAPI/final_p2_superset_sparse`.

Legacy end-to-end planner result from the same verification pass:

The raw line below keeps the old metric names for traceability. Re-run the
current CL plan for final A7 `key_material_total_mb` data.

```text
BenchmarkResearchTargetLevelFinalAPI/final_p2_superset_sparse-48 5 225940897 ns/op 80.68 evaluator_ms/setup 217.1 keygen_ms/setup 0 logical_views 39644709 persistent_key_bytes 97.00 physical_materials 88.15 plan_ms/setup 97.00 shared_materials 25408723 B/op 77174 allocs/op
```
