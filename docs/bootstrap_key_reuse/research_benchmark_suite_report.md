# Research Benchmark Suite Report

Date: 2026-05-30

This report records the first CL execution of the research benchmark suite for target-level bootstrap key reuse.

> **Historical status, 2026-06-12:** This report predates the reduced A7
> key-material-pool target evaluator spec. It validates the first benchmark
> harness smoke path only. Raw commands and output below are preserved for
> traceability, but rows using `ours_reusable_planner`, `material_total_mb`,
> `physical_materials`, `logical_views`, or `shared_materials` are legacy
> pre-A7-key-material-pool evidence and must not be cited as final A7
> performance results.
>
> Current A7 reports must use `ours_key_material_pool_target_evaluator` and
> `key_material_total_mb`, where
> `key_material_total_bytes = sum(BinarySize())` over unique physical
> relinearization, Galois, ring/domain switch, and dense/sparse key objects.
> DFT matrices, encoded diagonals, linear transformations, schedules,
> manifests, logical views, and wrapper overhead are excluded.

Related documents:

- [research_benchmark_suite_spec.md](research_benchmark_suite_spec.md)
- [research_benchmark_suite_cl_test_plan.md](research_benchmark_suite_cl_test_plan.md)
- [prefix_only_vs_planner_comparison.md](prefix_only_vs_planner_comparison.md)
- [target_count_sweep_experiment_plan.md](target_count_sweep_experiment_plan.md)
- [final_target_level_reuse_spec.md](final_target_level_reuse_spec.md)

## 1. Environment

| Field | Value |
| --- | --- |
| Workspace | `/root/code/lattigo` |
| Branch | `bootstrap-key-develop` |
| Commit | `cb67b6aade0ecf63d650a4ebc381ea1ccfb64bc7` |
| Dirty state during run | `true` |
| Go version | `go1.25.9` |
| GOOS / GOARCH | `linux / amd64` |
| CPU | `INTEL(R) XEON(R) GOLD 6530` |
| Default GOMAXPROCS in smoke report | `48` |

Dirty state is expected for this first run because the benchmark suite and docs were uncommitted while being validated.

## 2. Commands Executed

Research benchmark tests and CSV smoke output:

```bash
env GOCACHE=/tmp/go-cache go test ./circuits/ckks/bootstrapping -run 'TestResearchBenchmark' -count=1 -timeout=30m -args -bkr.research-result-dir=/tmp/lattigo-bkr-research-smoke
```

Result:

```text
ok github.com/tuneinsight/lattigo/v6/circuits/ckks/bootstrapping 1.323s
```

Final API smoke benchmark:

```bash
env GOCACHE=/tmp/go-cache go test ./circuits/ckks/bootstrapping -run '^$' -bench '^BenchmarkResearchTargetLevelFinalAPI' -benchmem -count=1 -timeout=30m -args -bkr.research-mode=smoke
```

Result:

```text
BenchmarkResearchTargetLevelFinalAPI/final_p0_exact_single-48 12 91388817 ns/op 75.83 evaluator_ms/setup 196.3 keygen_ms/setup 0 logical_views 36200709 persistent_key_bytes 97.00 physical_materials 53.34 plan_ms/setup 0 shared_materials 10781487 B/op 33328 allocs/op
PASS
```

Final target-level API correctness regression:

```bash
env GOCACHE=/tmp/go-cache go test ./circuits/ckks/bootstrapping -run '^TestTargetLevel' -count=1 -timeout=30m
```

Result:

```text
ok github.com/tuneinsight/lattigo/v6/circuits/ckks/bootstrapping 370.026s
```

Prefix-only differential functional gate:

```bash
env GOCACHE=/tmp/go-cache go test ./circuits/ckks/bootstrapping -run 'TestResearchBenchmarkPrefixComparison' -count=1 -timeout=30m
```

Result:

```text
ok github.com/tuneinsight/lattigo/v6/circuits/ckks/bootstrapping 0.187s
```

Prefix-only differential benchmark:

```bash
env GOCACHE=/tmp/go-cache go test ./circuits/ckks/bootstrapping -run '^$' -bench '^BenchmarkResearchPrefixOnlyVsPlanner' -benchmem -count=1 -timeout=30m
```

Result:

```text
BenchmarkResearchPrefixOnlyVsPlanner/prefix_only_p2_sparse_rejected-48      13 84677772 ns/op 0 executable_targets 1.000 owner_target_count 1.000 prefix_rejections 0 shared_material_count 45792050 B/op 2499205 allocs/op
BenchmarkResearchPrefixOnlyVsPlanner/planner_p2_sparse_superset_success-48 13 84589611 ns/op 2.000 executable_targets 1.000 owner_target_count 0 prefix_rejections 1.000 shared_material_count 45794577 B/op 2499225 allocs/op
PASS
```

Planner end-to-end P2 sparse benchmark:

```bash
env GOCACHE=/tmp/go-cache go test ./circuits/ckks/bootstrapping -run '^$' -bench '^BenchmarkResearchTargetLevelFinalAPI/final_p2_superset_sparse$' -benchmem -count=1 -timeout=30m -args -bkr.research-mode=standard
```

Result:

```text
BenchmarkResearchTargetLevelFinalAPI/final_p2_superset_sparse-48 5 225940897 ns/op 80.68 evaluator_ms/setup 217.1 keygen_ms/setup 0 logical_views 39644709 persistent_key_bytes 97.00 physical_materials 88.15 plan_ms/setup 97.00 shared_materials 25408723 B/op 77174 allocs/op
PASS
```

Legacy target-count sweep P4 single-target smoke:

This command used the old two-scheme runner and old material column names. It
is retained as harness history only. The current target-count sweep must emit
`original_lattigo`, `ours_single_superset_owner`, and
`ours_key_material_pool_target_evaluator` rows with `key_material_total_mb`.

```bash
env GOMAXPROCS=1 GOCACHE=/tmp/go-cache go test ./circuits/ckks/bootstrapping -run '^$' -bench '^BenchmarkTargetCountSweep/P4_target_count_1/(original_lattigo|ours_reusable_planner)$' -benchmem -benchtime=1x -count=1 -timeout=30m -args -bkr.target-count-sweep-result-dir=/tmp/lattigo-target-count-sweep-smoke
```

Result:

```text
BenchmarkTargetCountSweep/P4_target_count_1/original_lattigo-1       1 49741183868 ns/op 1.000 correctness_pass 42.10 key_prep_s 2201 material_total_mb 7504 mean_bootstrap_ms 1.000 physical_materials 0 shared_materials 7.504 total_bootstrap_s 21733434104 B/op 382642643 allocs/op
BenchmarkTargetCountSweep/P4_target_count_1/ours_reusable_planner-1 1 49444260373 ns/op 1.000 correctness_pass 41.17 key_prep_s 2201 material_total_mb 8109 mean_bootstrap_ms 827.0 physical_materials 0 shared_materials 8.109 total_bootstrap_s 21807161448 B/op 384469855 allocs/op
PASS
```

## 3. Artifacts

Result directory:

```text
/tmp/lattigo-bkr-research-smoke
```

Artifacts:

- `research_benchmark_manifest.csv`
- `research_benchmark_results.csv`
- `research_benchmark_confidence_audit.csv`

## 4. Smoke Result Summary

`research_benchmark_results.csv` row:

| Metric | Value |
| --- | --- |
| Case | `final_p0_exact_single` |
| Lane | `final_api` |
| Case ID | `a0_p0_tiny_native_single` |
| Profile | `P0_TINY_NATIVE` |
| Target levels | `1` |
| Owner target levels | `1` |
| Policy | `exact_only` |
| Plan time | `51.658104 ms` |
| Keygen time | `191.829618 ms` |
| Evaluator construction time | `63.983162 ms` |
| Bootstrap time | `96.776230 ms` |
| BootstrapMany time | `194.552246 ms` |
| Report generation time | `0.009558 ms` |
| Persistent key bytes | `36,200,709` |
| Physical material count | `97` |
| Logical view count | `0` |
| Shared count | `0` |
| Output level equality | `true` |
| Output scale equality | `true` |
| Average log2 precision real | `23.608218` |
| Average log2 precision imag | `23.704952` |
| Precision threshold | `12.000000` |

## 5. Manifest Coverage

The generated manifest contains:

- Profiles: P0, P1, P2, P3, P4, P5, P6
- Target shapes: `single`, `clustered`, `sparse`, `random`, `all_legal`
- Lanes: `a0_oracle`, `a1_used_only`, `a2_rotation_pool`, `a3_schedule_pool`, `a4_diagonal_pool`, `a5_rns_slice`, `a6_superset_drop`, `final_api`
- Policies: `exact_only`, `exact_prefix`, `exact_prefix_superset_drop`
- Phases: `plan`, `keygen`, `evaluator_construction`, `bootstrap`, `bootstrap_many`, `parallel_bootstrap`, `report_generation`, `oracle_validation`

## 6. Confidence Audit

`research_benchmark_confidence_audit.csv` contains RB1-RB10, all with `mitigated=true`.

Current conclusion: the benchmark suite has no known unmitigated spec loophole for the stated method under test. Any future addition of profile, policy, material kind, API method, or result column must update the manifest and confidence audit before the same confidence claim is allowed.

## 7. Analysis

Smoke evidence confirms that the suite can:

- enumerate the required research matrix;
- write stable CSV artifacts;
- validate correctness for a final API target-level bootstrap;
- pass the broader `^TestTargetLevel` final API correctness regression;
- expose the prefix-only rejection vs planner success difference on the same P2 sparse workload;
- write legacy target-count sweep CSV smoke rows, not final A7 target-count
  sweep rows;
- report setup latency, bootstrap latency, allocation, persistent key bytes,
  and legacy material counters.

The smoke case is `exact_only` on P0, so it intentionally shows `logical_view_count=0` and `shared_count=0`. It proves the measurement path, not the final reuse savings. Reuse-sensitive conclusions require the standard and exhaustive gates in [research_benchmark_suite_cl_test_plan.md](research_benchmark_suite_cl_test_plan.md).

For final A7 performance claims, rerun the current CL plan so the report
contains object-level `key_material_total_mb`, shared/private key material
split, and `target_evaluator_count` for
`ours_key_material_pool_target_evaluator`.

## 8. Remaining Gates

Not yet executed in this first report:

- Standard scalar gate with `GOMAXPROCS=1`, `-count=5`, and P0-P4 final API rows.
- Full P4/P6 target-count sweep across every planned `|T|` value.
- A0-A6 selected evidence directories following [a0_a6_test_standard.md](a0_a6_test_standard.md).
- Exhaustive long gate for P5/P6 with `-long`.
- Profiler lane with CPU, heap, and trace artifacts.

Until those gates are run, this report certifies the research benchmark suite and its smoke execution, not the complete performance characterization of the method.
