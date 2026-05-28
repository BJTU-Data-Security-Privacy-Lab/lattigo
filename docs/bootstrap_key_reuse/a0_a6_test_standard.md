# A0-A6 统一测试标准

本文档固定 A0-A6 后续测试环境、测试用例、Go 工具链命令、具体参数、输出位置、
CSV 格式和通过标准。标准只使用 Go 原生命令体系：`go test`、`go test -bench`、
`-benchmem`、`-count`、`-run`、`-timeout`、`-args`。不使用 `go test -json` 作为
A0-A6 标准采集方式，不引入 Python、shell 脚本或第三方 benchmark 工具作为标准依赖。

## 1. 测试目标

A0 是唯一 baseline。A1-A6 必须在相同 workload、params、secret-key domain、
modulus chain、target scale、input seed 下与 A0 对比。

同一 experiment case 必须固定：

- canonical full CKKS profile；
- canonical full Q/P modulus chain；
- secret-key domain；
- fixed target scale；
- plaintext / ciphertext input seed；
- `AllTargetLevels`；
- `UsedTargetLevels`。

A1-A6 的差异只能来自对应 ablation 允许的优化策略，不能通过改变参数、输入或
precision threshold 制造收益。

## 2. 测试环境

Go 版本基准是 `go.mod` 中的 `go 1.25.0`。本仓库使用 repo-local Go toolchain，
激活方式见 [toolchain_usage.md](toolchain_usage.md)。

每次结果必须记录：

- `go version`；
- `GOOS` / `GOARCH`；
- CPU / RAM；
- `GOMAXPROCS`；
- git branch；
- git commit；
- git dirty state；
- 测试命令原文。

latency / ablation 标准 lane 固定 `GOMAXPROCS=1`。throughput lane 可以单独使用
多核，但必须单独报告，不能与 scalar latency 混合比较。

## 3. Go 工具链命令

### 3.1 快速正确性

```bash
go test ./circuits/ckks/bootstrapping -run '^TestBootstrapping$' -count=1 -timeout=30m -args -print-precision
```

### 3.2 circuit 原始路径

```bash
go test ./circuits/ckks/bootstrapping -run '^TestCircuit(Original|WithEncapsulation)$' -count=1 -timeout=30m -args -print-precision
```

### 3.3 全默认参数长测

```bash
go test ./circuits/ckks/bootstrapping -run '^TestAllParameters$' -count=1 -timeout=0 -args -long -allparams -print-precision
```

### 3.4 并发吞吐 benchmark

```bash
go test ./circuits/ckks/bootstrapping -run '^$' -bench '^BenchmarkConcurrentBootstrap$' -benchmem -count=3 -timeout=0
```

### 3.5 A0 target-level harness

A0-A6 专用 harness 必须显式指定结果目录。PowerShell 下 `-bkr.result-dir` 必须加
引号，避免被拆成 `-bkr`。

每次命令只运行一个 A0 case，一个 command 对应一个 result directory：

```powershell
. .\.toolchain\use-go.ps1
$runDir = "docs/bootstrap_key_reuse/results/<YYYYMMDD-HHMMSS>_a0_<case_id>"
New-Item -ItemType Directory -Force -Path $runDir | Out-Null

go test ./circuits/ckks/bootstrapping `
  -run '^TestBootstrapKeyReuseA0_<CaseName>$' `
  -count=1 `
  -timeout=30m `
  -args -print-precision '-bkr.result-dir' $runDir

if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
```

A0 benchmark：

```powershell
$runDir = "docs/bootstrap_key_reuse/results/<YYYYMMDD-HHMMSS>_a0_bench_<case_id>"
New-Item -ItemType Directory -Force -Path $runDir | Out-Null

go test ./circuits/ckks/bootstrapping `
  -run '^$' `
  -bench '^BenchmarkBootstrapKeyReuseA0_<CaseName>$' `
  -benchmem `
  -count=3 `
  -timeout=0 `
  -args '-bkr.result-dir' $runDir
```

标准命令不使用 `go test -json`，不重定向 stdout/stderr，不使用 `Tee-Object`，不在
result directory 中写 `.log` 文件。

## 4. 固定测试参数

### 4.1 P0_TINY_NATIVE

用途：无 ring-degree switch 的最小快速正确性参数。

```text
LogN = 10
LogQ = [60, 40]
LogP = [61]
LogDefaultScale = 40
BootstrappingParams.LogN = 10
LogSlots = 9
AllTargetLevels = [1]
```

### 4.2 P1_TINY_RING_SWITCH

用途：最小 ring-degree switch 正确性参数。

```text
Residual LogN = 9
Residual LogNthRoot = 11
Bootstrapping LogN = 10
LogQ = [60, 40]
LogP = [61]
LogDefaultScale = 40
AllTargetLevels = [1]
```

### 4.3 P2_MULTI_FAST

用途：A0-A6 target-level harness 的快速多 target level 参数。

```text
LogN = 10
LogQ = [60, 40, 40, 40]
LogP = [61]
LogDefaultScale = 40
BootstrappingParams.LogN = 10
target sets:
  single = [3]
  clustered = [1, 2, 3]
  sparse = [1, 3]
  random fixed = [1, 2, 3]
```

### 4.4 P3_N15_SPARSE

用途：基于 `N15QP768H192H32` 的 sparse-secret 中等参数。

```text
Based on = N15QP768H192H32
LogN = 15
LogQ = [33, 50, 25]
LogP = [51, 51]
LogDefaultScale = 25
BootstrappingParams.LogN = 15
target sets:
  single = [2]
  clustered = [1, 2]
  sparse = [1, 2]
```

### 4.5 P4_N15_DENSE

用途：基于 `N15QP880H16384H32` 的 dense-secret 中等参数。

```text
Based on = N15QP880H16384H32
LogN = 15
LogQ = [40, 31, 31, 31, 31]
LogP = [56, 56]
LogDefaultScale = 31
target sets:
  single = [4]
  clustered = [2, 3, 4]
  sparse = [1, 4]
  random = [1, 3, 4]
```

### 4.6 P5_N16_SPARSE_LONG

用途：基于 `N16QP1546H192H32` 的 sparse-secret 长测参数。

```text
Based on = N16QP1546H192H32
LogN = 16
LogQ = [60, 40, 40, 40, 40, 40, 40, 40, 40, 40]
LogP = [61, 61, 61, 61, 61]
LogDefaultScale = 40
target sets:
  single = [9]
  clustered = [7, 8, 9]
  sparse = [1, 5, 9]
  random = [2, 4, 6, 8]
```

### 4.7 P6_N16_DENSE_LONG

用途：基于 `N16QP1767H32768H32` 的 dense-secret 长测参数。

```text
Based on = N16QP1767H32768H32
LogN = 16
LogQ = [60, 40, 40, 40, 40, 40, 40, 40, 40, 40, 40, 40, 40, 40]
LogP = [61, 61, 61, 61, 61, 61]
LogDefaultScale = 40
target sets:
  single = [13]
  clustered = [11, 12, 13]
  sparse = [1, 7, 13]
  random = [2, 5, 9, 12]
```

## 5. 测试用例标准

A0 必测：

- 对 `AllTargetLevels` 全量生成完整独立 `BK(r)`；
- 不允许任何 sharing、prefix view、superset view 或 `DropLevel` 替代；
- 每个 `r` 断言 `OutputLevel()==r`、`ctOut.Level()==r`、`ctOut.Scale==FixedTargetScale`；
- 每个 target 写出 correctness、metrics、runtime 和 baseline material identity CSV。

A1 必测：

- 只生成 `UsedTargetLevels`；
- unused target levels 必须无 keygen 记录；
- used levels 的 level、scale、precision 必须满足 A0 标准。

A2 必测：

- 使用 `galois_key_baseline.csv` 计算 A0 per-target Galois set union、overlap、共享率；
- canonical ID 使用 Lattigo 实际 `gal_el`；
- bootstrap 结果必须匹配 A0 的 level、scale、precision 标准。

A3 必测：

- 只允许 schedule ID 相同的 schedule interning；
- encoded diagonal 不在 A3 共享；
- 不改变 CoeffsToSlots / SlotsToCoeffs 数学语义。

A4 必测：

- 只允许 exact-compatible encoded diagonal sharing；
- 使用 `encoded_diagonal_id` 判定 exact compatibility；
- 不允许 RNS prefix slicing。

A5 必测：

- 默认关闭；
- 必须通过 exact vs prefix/superset 等价测试；
- 比较 output level、output scale、decoded precision、rotation stress cases。

A6 必测：

- 默认 exploratory；
- high-level bootstrap + `DropLevel` 必须与 A0 direct `BK(r)` 比较 level、scale、precision、latency。

## 6. 测试代码位置与命名规范

A0-A6 专用测试代码必须放在：

```text
circuits/ckks/bootstrapping/
```

文件命名：

```text
bootstrap_key_reuse_<lane>_test.go
```

允许的 `<lane>`：

```text
a0
a1
a2
a3
a4
a5
a6
common
metrics
```

测试函数命名：

```text
TestBootstrapKeyReuseA0_<CaseName>
TestBootstrapKeyReuseA1_<CaseName>
...
TestBootstrapKeyReuseA6_<CaseName>
```

benchmark 函数命名：

```text
BenchmarkBootstrapKeyReuseA0_<CaseName>
BenchmarkBootstrapKeyReuseA1_<CaseName>
...
BenchmarkBootstrapKeyReuseA6_<CaseName>
```

helper 必须是 unexported，不新增 public API。

## 7. CSV-only 输出文件位置与格式

所有 A0-A6 专用测试输出必须写入：

```text
docs/bootstrap_key_reuse/results/<YYYYMMDD-HHMMSS>_<lane>_<case_id>/
```

run directory 只允许 CSV 持久化产物，不允许 `.json`、`.jsonl`、`.log`。标准文件名：

```text
metadata.csv
experiment_case.csv
target_results.csv
material_metrics.csv
runtime_metrics.csv
parameter_chain_baseline.csv
galois_key_baseline.csv
linear_transform_schedule_baseline.csv
encoded_diagonal_baseline.csv
material_baseline_index.csv
failures.csv
summary.csv
```

数组和复合字段编码：

```text
整数列表: 1;2;3
hash 列表: sha256:a;sha256:b
key-value 列表: key1=value1;key2=value2
空值: 空字符串
```

### 7.1 CSV header

`metadata.csv`：

```text
schema_version,lane,plan_id,case_id,params_profile,command,go_version,go_env,goos,goarch,num_cpu,gomaxprocs,vcs_revision,vcs_time,vcs_modified,branch,commit,dirty_state,started_at
```

`experiment_case.csv`：

```text
record_type,schema_version,plan_id,case_id,params_profile,all_target_levels,used_target_levels,fixed_target_scale_log,log_slots,ring_switch_mode,packed_mode,seed,repeat_count
```

`target_results.csv`：

```text
record_type,schema_version,plan_id,case_id,params_profile,target_level,status,output_level,expected_output_level,output_level_equality,output_scale_log2,expected_output_scale_log2,output_scale_equality,average_log2_precision_real,average_log2_precision_imag,precision_threshold_bits,bootstrap_error_status,packed_ciphertexts
```

`material_metrics.csv`：

```text
record_type,schema_version,plan_id,case_id,params_profile,target_level,generated_evaluation_key_count,generated_rotation_key_count,generated_encoded_diagonal_count,persistent_key_bytes,persistent_matrix_bytes,shared_rotation_keys,shared_encoded_diagonals,rns_slice_success,fallback_reason,rotation_set_hash,c2s_schedule_hash,s2c_schedule_hash,encoded_diagonal_set_hash
```

`runtime_metrics.csv`：

```text
record_type,schema_version,plan_id,case_id,params_profile,target_level,keygen_time_ms,evaluator_matrix_construction_time_ms,bootstrap_latency_ms,peak_keygen_heap_bytes,peak_runtime_heap_bytes
```

`parameter_chain_baseline.csv`：

```text
record_type,schema_version,plan_id,case_id,params_profile,target_level,full_log_n,full_max_level,full_q_count,full_p_count,full_q_hash,full_p_hash,residual_log_n,residual_max_level,residual_q_count,residual_p_count,residual_q_prefix_hash,residual_p_hash,bootstrapping_log_n,bootstrapping_max_level,bootstrapping_q_count,bootstrapping_p_count,bootstrapping_q_hash,bootstrapping_p_hash,fixed_target_scale_log,residual_default_scale_log2,bootstrapping_default_scale_log2
```

`galois_key_baseline.csv`：

```text
record_type,schema_version,plan_id,case_id,params_profile,target_level,generated_galois_elements_sorted,required_bootstrapping_galois_elements_sorted,canonical_rotation_ids,discrete_log_k,contains_complex_conjugation,generated_count,required_count,missing_required_galois_elements,extra_generated_galois_elements,rotation_set_hash
```

`linear_transform_schedule_baseline.csv`：

```text
record_type,schema_version,plan_id,case_id,params_profile,target_level,matrix_name,transform_index,dft_type,dft_type_value,dft_format,dft_format_value,dft_log_slots,dft_level_q,dft_level_p,dft_levels,dft_bit_reversed,dft_log_bsgs_ratio,lt_level_q,lt_level_p,lt_n1,lt_log_rows,lt_log_cols,lt_scale,lt_scale_log2,diagonal_indices,galois_elements,transform_schedule_id,matrix_schedule_id
```

`encoded_diagonal_baseline.csv`：

```text
record_type,schema_version,plan_id,case_id,params_profile,target_level,matrix_name,transform_index,diagonal_index,level_q,level_p,n,q_prefix_hash,p_prefix_hash,poly_binary_size,poly_sha256,encoded_diagonal_id
```

`material_baseline_index.csv`：

```text
record_type,schema_version,plan_id,case_id,params_profile,target_level,rotation_set_hash,c2s_schedule_hash,s2c_schedule_hash,encoded_diagonal_set_hash,generated_rotation_key_count,schedule_record_count,encoded_diagonal_record_count,distinct_encoded_diagonal_count,metrics_generated_rotation_key_count,metrics_generated_encoded_diagonal_count,rotation_count_matches_metrics,encoded_diagonal_count_matches_metrics
```

`failures.csv`：

```text
record_type,schema_version,plan_id,case_id,params_profile,target_level,stage,message,before_keygen,generated_keygen,generated_runtime
```

`summary.csv`：

```text
schema_version,lane,plan_id,case_id,params_profile,status,target_levels,result_directory,started_at,finished_at,elapsed_ms,total_targets,successful_targets,failed_targets,passed,failure_count,shared_rotation_keys,shared_encoded_diagonals,rns_slice_success,fallback_reason
```

## 8. 指标与通过标准

统一记录对象：

- `ExperimentCase`
- `TargetRunResult`
- `MaterialMetrics`
- `RuntimeMetrics`
- `ParameterChainBaseline`
- `GaloisKeyBaseline`
- `LinearTransformScheduleBaseline`
- `EncodedDiagonalBaseline`
- `MaterialBaselineIndex`
- `Failure`
- `Summary`

精度标准：

- 使用 `ckks.GetPrecisionStats`；
- 记录 `AVGLog2Prec.Real` 和 `AVGLog2Prec.Imag`；
- quick lane 默认不低于 `12.0` bits；
- long lane 不低于对应 default parameter 注释中的预期 precision 下界减容忍值。

性能标准：

- scalar latency 使用 `GOMAXPROCS=1`；
- throughput 只使用 `BenchmarkConcurrentBootstrap`，单独报告；
- benchmark 至少 `-count=3`；
- keygen time、evaluator/matrix construction time、bootstrap latency 分开记录；
- peak keygen heap 和 peak runtime heap 分开记录。

A1-A6 通过条件：

- 不破坏 A0 target level 和 fixed scale 语义；
- 不低于 precision threshold；
- 共享 predicate 不满足时必须 fallback 或拒绝，不能静默共享；
- 默认关闭或 exploratory 的 optimization lane 必须在结果中明确标注。

## 9. 与现有 Lattigo 测试的关系

现有 `circuits/ckks/bootstrapping` 测试继续作为 native bootstrapping 回归门槛。
A0-A6 harness 是 target-level key reuse 实验的统一标准，必须遵守本文档的参数、
命令、CSV 输出和通过标准。
