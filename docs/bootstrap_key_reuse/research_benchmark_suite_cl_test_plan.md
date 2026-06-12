# Research Benchmark Suite CL Test Plan

本文档给出 target-level bootstrap key reuse 研究级 benchmark suite 的命令行测试计划。标准命令只使用 Go 原生命令体系，不依赖外部脚本或第三方 benchmark runner。

相关文档：

- [research_benchmark_suite_spec.md](research_benchmark_suite_spec.md)
- [prefix_only_vs_planner_comparison.md](prefix_only_vs_planner_comparison.md)
- [target_count_sweep_experiment_plan.md](target_count_sweep_experiment_plan.md)
- [research_benchmark_suite_report.md](research_benchmark_suite_report.md)
- [a0_a6_test_standard.md](a0_a6_test_standard.md)
- [toolchain_usage.md](toolchain_usage.md)

## 1. Preconditions

从仓库根目录运行：

```bash
pwd
git status --short --branch
go version
```

建议统一使用 repo-local Go toolchain；本仓库工具链说明见 [toolchain_usage.md](toolchain_usage.md)。

标准结果目录建议：

```text
/tmp/lattigo-bkr-research-smoke
/tmp/lattigo-bkr-research-standard
/tmp/lattigo-bkr-research-exhaustive
/tmp/lattigo-bkr-research-profile
```

## 2. Smoke Gate

用途：证明研究 suite 的 manifest、confidence audit、CSV schema、CSV writer、final API smoke path 均可执行。

```bash
env GOCACHE=/tmp/go-cache go test ./circuits/ckks/bootstrapping \
  -run 'TestResearchBenchmark' \
  -count=1 \
  -timeout=30m \
  -args -bkr.research-result-dir=/tmp/lattigo-bkr-research-smoke
```

用途：证明 final API benchmark path 实际执行 target-level bootstrap，并输出 setup/material metrics。

```bash
env GOCACHE=/tmp/go-cache go test ./circuits/ckks/bootstrapping \
  -run '^$' \
  -bench '^BenchmarkResearchTargetLevelFinalAPI' \
  -benchmem \
  -count=1 \
  -timeout=30m \
  -args -bkr.research-mode=smoke
```

Smoke gate 通过标准：

- `research_benchmark_manifest.csv` 至少包含 header plus data。
- `research_benchmark_results.csv` 至少包含 `final_p0_exact_single`。
- `research_benchmark_confidence_audit.csv` 中所有 `mitigated=true`。
- benchmark stdout 包含 `ns/op`、`B/op`、`allocs/op` 和 setup/material 自定义 metrics。

## 3. Prefix-Only Differential Gate

用途：用同一个 P2 sparse workload 具体体现单纯 RNS prefix 截断和当前 planner 的区别。

```bash
env GOCACHE=/tmp/go-cache go test ./circuits/ckks/bootstrapping \
  -run 'TestResearchBenchmarkPrefixComparison' \
  -count=1 \
  -timeout=30m
```

```bash
env GOCACHE=/tmp/go-cache go test ./circuits/ckks/bootstrapping \
  -run '^$' \
  -bench '^BenchmarkResearchPrefixOnlyVsPlanner' \
  -benchmem \
  -count=1 \
  -timeout=30m
```

该 gate 必须证明：

- `prefix_only_p2_sparse_rejected` reports `executable_targets=0` and `prefix_rejections=1`。
- `planner_p2_sparse_superset_success` reports `executable_targets=2`, `prefix_rejections=0`, and `shared_material_count=1`。
- 两个 sub-benchmark 使用相同 future target levels `[1,3]` 和 owner target level `[3]`。
- prefix-only rejection reason 包含 `rns_prefix_bootstrapping_q_prefix_mismatch_2`。

## 4. Target-Count Sweep Gate

用途：生成三张核心性能表和柱状图的数据：物料总大小 MB、密钥准备时间、bootstrap 时间，横轴为 target level 数量。

P4 smoke：

```bash
env GOMAXPROCS=1 GOCACHE=/tmp/go-cache go test ./circuits/ckks/bootstrapping \
  -run '^$' \
  -bench '^BenchmarkTargetCountSweep/P4' \
  -benchmem \
  -benchtime=1x \
  -count=1 \
  -timeout=60m \
  -args -bkr.target-count-sweep-result-dir=/tmp/lattigo-target-count-sweep-p4
```

P6 paper lane：

```bash
env GOMAXPROCS=1 GOCACHE=/tmp/go-cache go test ./circuits/ckks/bootstrapping \
  -run '^$' \
  -bench '^BenchmarkTargetCountSweep/P6' \
  -benchmem \
  -benchtime=1x \
  -count=5 \
  -timeout=0 \
  -args -long -bkr.target-count-sweep-result-dir=/tmp/lattigo-target-count-sweep-p6
```

Target-count sweep 通过标准：

- `target_count_sweep_results.csv` 同时包含 `original_lattigo`、
  `ours_single_superset_owner` 和
  `ours_key_material_pool_target_evaluator`。
- 每个 profile/target-count label 的三种 scheme 均有行。
- `key_material_total_mb > 0`。
- `key_preparation_time_s > 0`。
- `total_bootstrap_time_s > 0`。
- `correctness_pass=true`。
- `ours_key_material_pool_target_evaluator` 行的
  `target_evaluator_count == target_count`。
- `key_material_total_mb` 使用对象级 key-material 公式：只统计
  relinearization key、每个 Galois key、非空 ring/domain switch key、非空
  dense/sparse key 的 `BinarySize()`；不统计 DFT encoded diagonals、
  linear transformations、matrix schedules、manifests 或 key wrapper overhead。

## 5. Standard Scalar Gate

用途：在 P0-P4 fast/medium profiles 上收集 scalar latency、allocation 和 material accounting。该 lane 固定 `GOMAXPROCS=1`，不能与 throughput 或 profiler lane 合并比较。

```bash
env GOMAXPROCS=1 GOCACHE=/tmp/go-cache go test ./circuits/ckks/bootstrapping \
  -run 'TestResearchBenchmark' \
  -count=1 \
  -timeout=30m \
  -args -bkr.research-result-dir=/tmp/lattigo-bkr-research-standard
```

```bash
env GOMAXPROCS=1 GOCACHE=/tmp/go-cache go test ./circuits/ckks/bootstrapping \
  -run '^$' \
  -bench '^BenchmarkResearchTargetLevelFinalAPI' \
  -benchmem \
  -count=5 \
  -timeout=60m \
  -args -bkr.research-mode=standard
```

Final API correctness gate：

```bash
env GOCACHE=/tmp/go-cache go test ./circuits/ckks/bootstrapping \
  -run '^TestTargetLevel' \
  -count=1 \
  -timeout=60m
```

A0-A6 lane evidence gate follows [a0_a6_test_standard.md](a0_a6_test_standard.md): run each selected A0-A6 case with an explicit `-bkr.result-dir`, one command per result directory.

Standard gate 通过标准：

- P0-P4 final API benchmark rows complete。
- No benchmark row fails correctness setup。
- `persistent_key_bytes > 0` for key-bearing cases。
- `physical_key_material_count`, `shared_key_material_count`,
  `private_key_material_count`, and `target_evaluator_count` are reported for
  every A7 final API benchmark row。
- Optional legacy fields such as `physical_materials`, `logical_views`, and
  `shared_materials` may remain for A0-A6 comparison lanes, but they must not
  be used as the A7 key-material accounting source of truth。
- A0-A6 evidence directories exist for selected comparison lanes。

## 6. Exhaustive Long Gate

用途：覆盖 P5/P6 long profiles，防止 “any target level” 只在小参数上成立。

```bash
env GOMAXPROCS=1 GOCACHE=/tmp/go-cache go test ./circuits/ckks/bootstrapping \
  -run '^$' \
  -bench '^BenchmarkResearchTargetLevelFinalAPI' \
  -benchmem \
  -count=5 \
  -timeout=0 \
  -args -long -bkr.research-mode=exhaustive
```

Long correctness gate：

```bash
env GOCACHE=/tmp/go-cache go test ./circuits/ckks/bootstrapping \
  -run '^TestTargetLevelBootstrapperExactOwnersCoverAllLegalLongTargets$' \
  -count=1 \
  -timeout=0 \
  -args -long
```

Exhaustive gate 通过标准：

- P5 and P6 benchmark rows are present。
- Long correctness gate passes。
- Report clearly labels long results separately from standard scalar results。

## 7. Profiler Lane

用途：分析 CPU、heap 和 trace bottleneck。Profiler lane 只用于解释性能，不作为 scalar latency baseline。

```bash
env GOMAXPROCS=1 GOCACHE=/tmp/go-cache go test ./circuits/ckks/bootstrapping \
  -run '^$' \
  -bench '^BenchmarkResearchTargetLevelFinalAPI/final_p2_' \
  -benchmem \
  -count=1 \
  -timeout=60m \
  -cpuprofile /tmp/lattigo-bkr-research-profile/cpu.pprof \
  -memprofile /tmp/lattigo-bkr-research-profile/mem.pprof \
  -trace /tmp/lattigo-bkr-research-profile/trace.out \
  -args -bkr.research-mode=standard
```

Profiler lane 通过标准：

- pprof/trace files are created。
- Report records the exact benchmark regex and target cases。
- Profile conclusions are marked explanatory, not baseline。

## 8. Report Checklist

每轮 CL report 必须包含：

- exact commands；
- pass/fail status；
- Go version；
- branch, commit, dirty state；
- CPU and `GOMAXPROCS`；
- result directory；
- CSV artifact list；
- benchmark stdout summary；
- material accounting summary；
- correctness status；
- unexecuted gates and reason。
