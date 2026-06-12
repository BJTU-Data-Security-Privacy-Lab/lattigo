# Target-Count Sweep Experiment Plan

本文档定义三张核心性能表和对应柱状图的实验方案。实验目标是回答：在固定 CKKS 参数、固定 RNS modulus chain、固定 secret key 和固定输入分布下，当需要支持的 bootstrap target level 数量增加时，原有 Lattigo Go 独立 `BK(r)` 方案与我们的 reusable target-level planner 在物料总大小、密钥准备时间和 bootstrap 时间上如何分离。

相关入口：

- 功能/benchmark 代码：`circuits/ckks/bootstrapping/bootstrap_key_reuse_research_benchmark_suite_test.go`
- 测试代码：`circuits/ckks/bootstrapping/bootstrap_key_reuse_research_benchmark_test.go`
- CL 计划：[research_benchmark_suite_cl_test_plan.md](research_benchmark_suite_cl_test_plan.md)

## 1. 实验对象

对比三个 scheme：

| Scheme | 含义 |
| --- | --- |
| `original_lattigo` | 对每个 target level `r` 独立构造 target residual parameters、独立生成 `GenEvaluationKeys`、独立构造 `NewEvaluator`、独立 bootstrap。 |
| `ours_single_superset_owner` | A6 fallback baseline：选择最高 owner evaluator，自举后对低 target 执行 `DropLevel`。该 scheme 只作为对照，不代表最终方法。 |
| `ours_key_material_pool_target_evaluator` | A7 final method：一次声明 `FutureTargetLevels=T`，生成 shared/private key-set material，按 target manifest 组装 `EvaluationKeys`，再为每个 `r in T` 构造 `OutputLevel()==r` 的 dedicated evaluator。 |

固定条件：

- 同一 profile 内固定 CKKS 参数和 RNS modulus chain；
- 同一 deterministic secret key domain；
- 同一 plaintext/ciphertext input seed；
- 同一 fixed target scale；
- scalar lane 固定 `GOMAXPROCS=1`。

## 2. 横轴定义

主论文实验使用 `P6_N16_DENSE_LONG`：

| Label | `|T|` | Target levels |
| --- | ---: | --- |
| `1` | 1 | `[13]` |
| `2` | 2 | `[1, 13]` |
| `4` | 4 | `[1, 5, 9, 13]` |
| `8` | 8 | `[1, 3, 5, 7, 9, 11, 12, 13]` |
| `all` | all legal | all constructible legal target levels for P6 |

快速 smoke 使用 `P4_N15_DENSE`：

| Label | `|T|` | Target levels |
| --- | ---: | --- |
| `1` | 1 | `[4]` |
| `2` | 2 | `[1, 4]` |
| `3` | 3 | `[1, 3, 4]` |
| `all` | all legal | all constructible legal target levels for P4 |

## 3. 输出 CSV

统一输出文件：

```text
target_count_sweep_results.csv
```

字段：

| Column | 用途 |
| --- | --- |
| `profile_id` | 区分 P4 smoke 和 P6 paper lane |
| `target_count` / `target_count_label` | 三张图的横轴 |
| `target_levels` | 固定 target set，保证可复现 |
| `scheme` | `original_lattigo`、`ours_single_superset_owner` 或 `ours_key_material_pool_target_evaluator` |
| `key_material_total_bytes` | 物料总大小的原始 byte 口径 |
| `key_material_total_mb` | 表 A 和图 A 的纵轴；仅包含 persistent key material |
| `shared_key_material_mb` | A7 共享 key-set 物料大小 |
| `private_key_material_mb` | A7 target-private key-set 物料大小 |
| `key_preparation_time_s` | 表 B 和图 B 的纵轴 |
| `evaluator_construction_time_s` | A7 target evaluator construction 时间；允许包含目标 evaluator 本地 DFT matrix 生成 |
| `total_bootstrap_time_s` | 表 C / 图 C 的 total runtime 纵轴 |
| `mean_bootstrap_latency_ms` | 表 C 的 per-target latency |
| `owner_target_levels` | 我们版本的 owner 选择；原版等于 `target_levels` |
| `physical_key_material_count` | 辅助解释 key-set 物料构成 |
| `shared_key_material_count` | 辅助解释 key-set 复用程度 |
| `private_key_material_count` | 辅助解释 target-private key-set 物料 |
| `target_evaluator_count` | A7 必须等于 `target_count` |
| `correctness_pass` | 该行是否通过 level/scale/precision 校验 |

`key_material_total_bytes` 是对象级统计口径：

```text
key_material_total_bytes = sum(BinarySize()) over unique physical key objects
key_material_total_mb = key_material_total_bytes / 1024 / 1024
```

可统计对象仅限：

1. 一个 `*rlwe.RelinearizationKey`，如果存在；
2. 每个需要的 `*rlwe.GaloisKey`；
3. 每个非空 ring/domain switch `*rlwe.EvaluationKey`；
4. 每个非空 dense/sparse `*rlwe.EvaluationKey`。

不得统计 `EvaluationKeys`、`MemEvaluationKeySet` wrapper overhead、
manifest、logical/RNS-prefix view、DFT matrix、linear transformation、matrix
schedule 或 encoded diagonal。

`ours_key_material_pool_target_evaluator` 的物理对象集合来自
`KeyMaterialPool`。`shared_key_material_mb` 是被多个 target manifest 引用
的物理对象子集，`private_key_material_mb` 是只被一个 target manifest 引用
的物理对象子集，二者之和必须等于 `key_material_total_mb`。

`original_lattigo` 必须在每个独立生成的 per-target `EvaluationKeys` 上使用
相同对象级公式；不同 target evaluator 中重复生成的 key 需要重复计入，
因为它们是物理上独立的对象。

DFT encoded diagonal 大小可以作为诊断列输出，但不得计入 key material 总大小。

## 4. 三张性能表

### 表 A：物料总大小

| `|T|` | Target levels | 原有 Lattigo MB | 我们版本 MB | 节省比例 |
| ---: | --- | ---: | ---: | ---: |
| 1 | from CSV | from CSV | from CSV | `(orig - ours) / orig` |
| 2 | from CSV | from CSV | from CSV | `(orig - ours) / orig` |
| 4 | from CSV | from CSV | from CSV | `(orig - ours) / orig` |
| 8 | from CSV | from CSV | from CSV | `(orig - ours) / orig` |
| all | from CSV | from CSV | from CSV | `(orig - ours) / orig` |

图 A：

```text
X axis: Number of target levels |T|
Y axis: Total bootstrap key material size (MB)
Bars: original_lattigo, ours_single_superset_owner, ours_key_material_pool_target_evaluator
```

### 表 B：密钥准备时间

| `|T|` | Target levels | 原有 Lattigo key prep s | 我们版本 key prep s | 加速比 |
| ---: | --- | ---: | ---: | ---: |
| 1 | from CSV | from CSV | from CSV | `orig / ours` |
| 2 | from CSV | from CSV | from CSV | `orig / ours` |
| 4 | from CSV | from CSV | from CSV | `orig / ours` |
| 8 | from CSV | from CSV | from CSV | `orig / ours` |
| all | from CSV | from CSV | from CSV | `orig / ours` |

图 B：

```text
X axis: Number of target levels |T|
Y axis: Key preparation time (s)
Bars: original_lattigo, ours_single_superset_owner, ours_key_material_pool_target_evaluator
```

### 表 C：Bootstrap 时间

| `|T|` | Target levels | 原有 Lattigo total s | 我们版本 total s | 原版 mean ms | 我们 mean ms |
| ---: | --- | ---: | ---: | ---: | ---: |
| 1 | from CSV | from CSV | from CSV | from CSV | from CSV |
| 2 | from CSV | from CSV | from CSV | from CSV | from CSV |
| 4 | from CSV | from CSV | from CSV | from CSV | from CSV |
| 8 | from CSV | from CSV | from CSV | from CSV | from CSV |
| all | from CSV | from CSV | from CSV | from CSV | from CSV |

图 C：

```text
X axis: Number of target levels |T|
Y axis: Total bootstrap time (s)
Bars: original_lattigo, ours_single_superset_owner, ours_key_material_pool_target_evaluator
```

## 5. 命令

Schema 和 smoke CSV：

```bash
env GOCACHE=/tmp/go-cache go test ./circuits/ckks/bootstrapping \
  -run 'TestTargetCountSweep' \
  -count=1 \
  -timeout=30m
```

P4 smoke benchmark：

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

P6 paper benchmark：

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

`-benchtime=1x` 是有意设置：每个 sub-benchmark 已经完整生成 key material 并 bootstrap 对应 target set，一次运行就是一个完整实验点。

## 6. 首轮 P4 单点 smoke

命令：

```bash
env GOMAXPROCS=1 GOCACHE=/tmp/go-cache go test ./circuits/ckks/bootstrapping \
  -run '^$' \
  -bench '^BenchmarkTargetCountSweep/P4_target_count_1/(original_lattigo|ours_single_superset_owner|ours_key_material_pool_target_evaluator)$' \
  -benchmem \
  -benchtime=1x \
  -count=1 \
  -timeout=30m \
  -args -bkr.target-count-sweep-result-dir=/tmp/lattigo-target-count-sweep-smoke
```

历史结果（旧两路 runner，已废弃，不能作为 A7 三路实验结果引用）：

| Scheme | Material MB | Key prep s | Total bootstrap s | Mean bootstrap ms | Correct |
| --- | ---: | ---: | ---: | ---: | --- |
| `original_lattigo` | `2201.079366` | `42.100270` | `7.503769` | `7503.768636` | true |
| `legacy_ours_reusable_planner` | `2201.079366` | `41.173620` | `8.109299` | `8109.299278` | true |

该单点只验证 runner 和 CSV 口径。论文结论必须来自 P6 target-count sweep 全横轴结果。
