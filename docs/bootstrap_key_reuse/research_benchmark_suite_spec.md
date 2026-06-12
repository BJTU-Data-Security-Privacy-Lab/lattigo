# Target-Level Bootstrap Key Reuse Research Benchmark Suite Spec

本文档定义最终 target-level bootstrap key reuse 方法的研究级 benchmark suite。它补充：

- 原始设计：[2026_05_28_design.md](2026_05_28_design.md)
- A0-A6 统一测试标准：[a0_a6_test_standard.md](a0_a6_test_standard.md)
- 最终对外 API 规格：[final_target_level_reuse_spec.md](final_target_level_reuse_spec.md)
- Prefix-only 对照：[prefix_only_vs_planner_comparison.md](prefix_only_vs_planner_comparison.md)
- Target-count sweep 实验：[target_count_sweep_experiment_plan.md](target_count_sweep_experiment_plan.md)
- CL 测试计划：[research_benchmark_suite_cl_test_plan.md](research_benchmark_suite_cl_test_plan.md)
- 首轮报告：[research_benchmark_suite_report.md](research_benchmark_suite_report.md)

## 1. 目标

研究对象是当前仓库内的最终 API：

- `NewTargetLevelMaterialPlan`
- `(*TargetLevelMaterialPlan).GenReusableEvaluationKeys`
- `NewTargetLevelBootstrapper`
- `(*TargetLevelBootstrapper).BootstrapAtLevel`
- `(*TargetLevelBootstrapper).BootstrapManyAtLevel`
- `(*TargetLevelBootstrapper).PlanReport`

该 suite 必须回答两个问题：

1. 任意合法 target level 是否能被声明、规划、生成 reusable material，并在运行时自举到请求的 target level。
2. 不同 target level 的 bootstrap key-set material 有重叠时，计划、keygen、evaluator construction、运行时自举和报告是否能稳定复用、度量和审计。
   最终方法必须是 A7 `ReuseKeyMaterialPoolTargetEvaluator`：每个 target
   level 使用 dedicated target evaluator，evaluator construction 从
   manifest-selected `EvaluationKeys` 构造目标 evaluator。DFT matrix、
   linear transformation、matrix schedule 和 encoded diagonal 不属于 A7 复用物料。

非目标：

- 不把 smoke benchmark 当作完整性能结论。
- 不用外部脚本、`go test -json` 或第三方 benchmark 工具作为标准依赖。
- 不把 profiler lane 的数据混入 scalar latency baseline。

## 2. 实现入口

代码入口：

- `circuits/ckks/bootstrapping/bootstrap_key_reuse_research_benchmark_test.go`
- `circuits/ckks/bootstrapping/bootstrap_key_reuse_research_benchmark_suite_test.go`

Go flags：

- `-bkr.research-mode=smoke|standard|exhaustive`
- `-bkr.research-result-dir=<dir>`
- 已有长测开关：`-long`

CSV artifact：

- `research_benchmark_manifest.csv`
- `research_benchmark_results.csv`
- `research_benchmark_confidence_audit.csv`

Prefix-only differential benchmark:

- `BenchmarkResearchPrefixOnlyVsPlanner`
- `BenchmarkTargetCountSweep`

该 benchmark 使用同一 P2 sparse workload：future target levels `[1,3]`、owner target level `[3]`。prefix-only policy 必须暴露 `executable_targets=0` 和 prefix rejection；当前 planner policy 必须暴露 `executable_targets=2` 和 shared material count。

`BenchmarkTargetCountSweep` 固定 profile 和 RNS chain，横轴为 target level 数量，输出 `target_count_sweep_results.csv`，用于绘制：

- total key material size in MB；
- key preparation time in seconds；
- total bootstrap time and mean bootstrap latency。

该 sweep 必须输出三路 scheme：`original_lattigo`、
`ours_single_superset_owner` 和 `ours_key_material_pool_target_evaluator`。
`ours_single_superset_owner` 是 A6 fallback baseline，不能作为最终方法；
`ours_key_material_pool_target_evaluator` 才是 A7 最终方法。

## 3. Benchmark Matrix

### 3.1 Profiles

| Profile | Purpose | Tier |
| --- | --- | --- |
| `P0_TINY_NATIVE` | 最小无 ring-switch smoke | smoke |
| `P1_TINY_RING_SWITCH` | 最小 ring-switch 正确性和开销 | standard |
| `P2_MULTI_FAST` | 快速多 target level、不同 target shape | standard |
| `P3_N15_SPARSE` | N15 sparse 中等参数 | standard |
| `P4_N15_DENSE` | N15 dense 中等参数 | standard |
| `P5_N16_SPARSE_LONG` | N16 sparse 长测 | exhaustive |
| `P6_N16_DENSE_LONG` | N16 dense 长测 | exhaustive |

### 3.2 Lanes

| Lane | Meaning | Required evidence |
| --- | --- | --- |
| `a0_oracle` | 每个 target level 独立完整 key 的 oracle baseline | A0 tests and benchmark CSV |
| `a1_used_only` | 只为 compiler-used target levels 生成材料 | A1 tests and runtime CSV |
| `a2_rotation_pool` | rotation key pool 复用 | A2 rotation pool CSV |
| `a3_schedule_pool` | linear-transform schedule interning | A3 schedule baseline CSV |
| `a4_diagonal_pool` | encoded diagonal compatibility sharing | A4 diagonal baseline CSV |
| `a5_rns_slice` | verified RNS prefix view | A5 slice correctness evidence |
| `a6_superset_drop` | superset owner output with `DropLevel` consumer view | A6 superset/drop evidence |
| `final_api` | 对外 target-level reusable material API | research benchmark results |

### 3.3 Target Shapes

| Shape | Required coverage |
| --- | --- |
| `single` | 单 target level，排除多 target overhead |
| `clustered` | 相邻 target levels，验证高 overlap 场景 |
| `sparse` | 非连续 target levels，验证 owner/view 选择 |
| `random` | 固定随机 target set，防止只适配手写样例 |
| `all_legal` | 覆盖 profile 下所有可构造合法 target levels |

### 3.4 Reuse Policies

| Policy | Meaning |
| --- | --- |
| `exact_only` | 仅 exact owner material |
| `exact_prefix` | 允许 verified RNS prefix view |
| `exact_prefix_superset_drop` | 允许 exact、prefix view、superset owner output drop |
| `key_material_pool_target_evaluator` | A7 final method：target evaluator 从 key material manifest 组装 `EvaluationKeys`，再调用 `NewEvaluator(targetParams, keys)` |

### 3.5 Measured Phases

每个 manifest row 声明需要覆盖的 phase：

- `plan`
- `keygen`
- `evaluator_construction`
- `bootstrap`
- `bootstrap_many`
- `parallel_bootstrap`
- `report_generation`
- `oracle_validation`

## 4. Metrics

`research_benchmark_results.csv` 固定记录：

- identity：`schema_version`、`name`、`lane`、`case_id`、`profile_id`、`target_shape`
- target planning：`target_levels`、`owner_target_levels`、`policy`
- latency：`plan_ms`、`keygen_ms`、`evaluator_construction_ms`、`bootstrap_ms`、`bootstrap_many_ms`、`report_generation_ms`
- material accounting：`persistent_key_bytes`、`key_material_total_bytes`、`key_material_total_mb`、`shared_key_material_mb`、`private_key_material_mb`、`physical_key_material_count`、`shared_key_material_count`、`private_key_material_count`、`target_evaluator_count`
- correctness：`output_level_equality`、`output_scale_equality`、`average_log2_precision_real`、`average_log2_precision_imag`、`precision_threshold_bits`
- environment：`go_version`、`gomaxprocs`、`commit`、`dirty_state`

`go test -bench` 输出还必须包含：

- `ns/op`
- `B/op`
- `allocs/op`
- `plan_ms/setup`
- `keygen_ms/setup`
- `evaluator_ms/setup`
- `persistent_key_bytes`
- `physical_key_materials`
- `shared_key_materials`
- `private_key_materials`
- `target_evaluator_count`

Legacy or A0-A6 comparison lanes may also emit `logical_views` or
`shared_materials`, but those fields are not the A7 key-material accounting
source of truth.

`key_material_total_bytes` is the sum of `BinarySize()` over unique physical
key objects, counted once per physical object:

1. one `*rlwe.RelinearizationKey` when present;
2. each required `*rlwe.GaloisKey`;
3. each non-nil ring/domain switch `*rlwe.EvaluationKey`;
4. each non-nil dense/sparse `*rlwe.EvaluationKey`.

`key_material_total_mb = key_material_total_bytes / 1024 / 1024`.

For A7 rows, the unique physical-object set is the `KeyMaterialPool`. Shared MB
is the subset referenced by more than one target manifest. Private MB is the
subset referenced by exactly one target manifest. Shared MB plus private MB
must equal `key_material_total_mb`.

Original Lattigo comparison rows must use the same object-level decomposition
over each independently generated per-target `EvaluationKeys`; repeated key
objects from different independent target evaluators are counted once per
evaluator instance because they are physically distinct objects.

DFT encoded diagonal bytes may appear only in optional diagnostic columns and
must not be included in key material totals. `evaluator_construction_ms` is
allowed to include normal target-local `NewEvaluator` DFT matrix generation
because A7 does not pool DFT material.

## 5. Completion Definition

研究级 suite 的完成分成两层。

### 5.1 Harness 完成

以下全部满足时，benchmark suite 本身完成：

1. Manifest 覆盖 P0-P6、target shapes、A0-A6 lanes、final API lane、三种 reuse policy 和全部 phase。
2. Confidence audit 中不存在未缓解项。
3. CSV schema 有稳定测试。
4. Smoke report 能在显式目录下写出三个 CSV artifact。
5. Smoke benchmark 实际执行 final API target-level bootstrap 并输出 setup/material 自定义 metrics。

### 5.2 实验完成

以下全部满足时，才能声称“当前方法已经完成研究级性能评测”：

1. smoke、standard、exhaustive 三组 CL 命令全部通过。
2. standard lane 使用 `GOMAXPROCS=1` 和 `-count=5`，不与 throughput/profiler 混用。
3. long profiles P5/P6 通过 `-long -bkr.research-mode=exhaustive` 覆盖。
4. 报告写入全部 CSV、命令原文、环境信息和 git state。
5. 分析报告明确区分 correctness、latency、memory/material accounting、profiler evidence 和 residual risk。

## 6. Confidence Audit

| ID | Potential loophole | Mitigation | Evidence |
| --- | --- | --- | --- |
| RB1 | 只挑选有利 lane 报数 | manifest 枚举 A0-A6 与 final API | `research_benchmark_manifest.csv` |
| RB2 | 性能数据隐藏正确性回归 | 每个 case 绑定 correctness gate，smoke 运行 precision/scale/level 校验 | `correctness_gate` and `research_benchmark_results.csv` |
| RB3 | scalar latency 与 throughput 混用 | manifest 固定 scalar `GOMAXPROCS=1`，parallel phase 单列 | manifest `gomaxprocs` and `phases` |
| RB4 | long profiles 被静默跳过 | P5/P6 rows 显式 `long_only=true` and `tier=exhaustive` | manifest P5/P6 rows |
| RB5 | RNS prefix 只靠整数 level 推断 | final API rows 覆盖 `exact_prefix` policy 和 RNS prefix correctness gate | policy and correctness gate |
| RB6 | 复用收益没有 material accounting | results 记录 persistent bytes、physical/logical/shared counts | results material columns |
| RB7 | 环境漂移导致不可比 | results 记录 Go、GOMAXPROCS、commit、dirty state | results environment columns |
| RB8 | profiler lane 混入 baseline | CL plan 将 profiler lane 单独列出 | CL profile section |
| RB9 | benchmark cache/stale run | CL commands 固定 `-count` 和显式 mode | CL command gates |
| RB10 | smoke 被误读为完整研究结论 | spec 区分 harness 完成和实验完成 | completion definition |

当前 spec 对上述已知漏洞没有未缓解项。若新增 profile、policy、material kind 或 API surface，必须同步新增 manifest coverage、CSV columns 或 confidence audit item；否则不能声称仍有事实上的 100% 覆盖信心。
