# A0 FullKeyPerTargetLevel Baseline Plan

统一测试环境、测试用例、Go 工具链命令和固定参数见
[a0_a6_test_standard.md](a0_a6_test_standard.md)。Go 工具链固定使用方式见
[toolchain_usage.md](toolchain_usage.md)。

## 1. Purpose and non-goals

A0 是 multi-target-level bootstrapping key reuse 工作的唯一 baseline。它必须对
`AllTargetLevels` 中每一个 target level `r` 独立生成完整 `BK(r)`，并记录后续
A1-A6 可直接消费的 correctness、precision、runtime、storage 和 material identity
明细。

A0 不追求节省 keygen、storage 或 RAM。A0 禁止：

- rotation key interning；
- linear-transform schedule interning；
- encoded diagonal sharing；
- RNS prefix / superset view；
- high-level bootstrap 后 `DropLevel` 替代 direct `BK(r)`；
- runtime lazy key generation。

A0 可以记录 canonical IDs，但这些 ID 只用于报告和后续 A1-A6 对照，不能用于 A0
内部共享决策。

## 2. A0 generation logic

A0 使用 `AllTargetLevels`，不是 `UsedTargetLevels`。A1 之后的 demand-driven lane
才允许只生成 `UsedTargetLevels`。

```text
for r in AllTargetLevels:
    build target-specific residual Q-prefix parameters for r
    build target-specific native bootstrapping parameters/evaluator
    reject before keygen if OutputLevel() cannot equal r
    independently generate complete bootstrapping.EvaluationKeys
    independently construct CoeffsToSlots / SlotsToCoeffs encoded matrices
    run bootstrap with direct BK(r)
    assert OutputLevel()==r, ctOut.Level()==r, ctOut.Scale==FixedTargetScale
```

当前 Lattigo public API 不支持 `Bootstrap(ct, targetLevel)`。`Evaluator.OutputLevel()`
由 `ResidualParameters.MaxLevel()` 决定。因此 A0 的非侵入式原生实现规则是：

```text
canonical full chain: Q[0], Q[1], ..., Q[L]
target level r: construct residual parameters with Q[:r+1]
```

这表示所有 target 都来自同一个 canonical full CKKS profile 和同一条 full modulus
chain，但每个 target-specific native evaluator 使用该 full chain 的 Q-prefix view。

## 3. Common experiment contract

所有 A0-A6 实验必须固定：

- 同一个 canonical full CKKS profile；
- 同一个 secret-key domain；
- 同一条 canonical full Q/P modulus chain；
- 同一个 fixed target scale；
- 同一个 plaintext / ciphertext input seed；
- 同一个 `AllTargetLevels` 和对应 `UsedTargetLevels` 定义。

每个 case 至少记录：

- `ExperimentCase`：profile、target sets、scale、logSlots、ring-switch mode、packed mode、seed、repeat count；
- `TargetRunResult`：target level、output level、scale equality、real/imag precision、bootstrap status；
- `MaterialMetrics`：generated key/rotation/diagonal counts、persistent bytes、A0 sharing defaults；
- `RuntimeMetrics`：keygen time、evaluator/matrix construction time、bootstrap latency、peak heaps；
- baseline material identity：parameter chain、Galois set、linear transform schedule、encoded diagonal IDs。

A0 metric defaults 固定为：

```text
shared_rotation_keys = 0
shared_encoded_diagonals = 0
rns_slice_success = not_applicable
fallback_reason = none
```

## 4. CSV-only output contract

A0 持久化输出必须是 CSV-only。run directory 中不允许生成或保留 `.json`、`.jsonl`
或 `.log` 审计文件。标准 run directory 为：

```text
docs/bootstrap_key_reuse/results/<YYYYMMDD-HHMMSS>_a0_<case_id>/
```

每个 A0 run directory 必须只包含下列 CSV 文件：

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

数组字段使用人类可读字符串编码：

```text
integer list: 1;2;3
hash list: sha256:a;sha256:b
key-value list: key1=value1;key2=value2
```

CSV header、文件名和字段顺序由统一测试标准固定。A0 harness 必须拒绝包含
`.json`、`.jsonl` 或 `.log` 的 result directory，避免旧审计产物混入新结果。

## 5. Baseline material identity

### GaloisKeyBaseline

每个 target level 一行，记录：

- generated Galois elements；
- required bootstrapping Galois elements；
- canonical rotation IDs，格式为 `gal:<uint64>`；
- `discrete_log_k`，仅用于分析；
- complex conjugation key 是否存在；
- rotation set hash。

canonical rotation ID 使用 Lattigo 实际 `gal_el`，不是用户 rotation `k`。

### LinearTransformScheduleBaseline

每个 CoeffsToSlots / SlotsToCoeffs transform 一行，记录：

- matrix name：`coeffs_to_slots` 或 `slots_to_coeffs`；
- DFT literal：type、format、logSlots、levelQ、levelP、levels、bitReversed、logBSGSRatio；
- linear transformation：transform index、levelQ、levelP、N1、log dimensions、scale；
- diagonal indices；
- required Galois elements；
- transform schedule ID；
- matrix schedule ID。

A3 只能在 schedule ID 相等时做 schedule interning。A3 不共享 encoded diagonal。

### EncodedDiagonalBaseline

每个 encoded diagonal 一行，记录：

- matrix name；
- transform index；
- diagonal index；
- levelQ / levelP / N；
- Q prefix hash；
- P prefix hash；
- poly binary size；
- poly SHA256；
- encoded diagonal ID。

A4 只能用 `encoded_diagonal_id` 判定 exact-compatible encoded diagonal sharing。A4
不允许 RNS prefix slicing。

### MaterialBaselineIndex

每个 target level 一行，汇总：

- rotation set hash；
- C2S schedule hash；
- S2C schedule hash；
- encoded diagonal set hash；
- rotation / schedule / encoded diagonal counts；
- 与 `material_metrics.csv` 的 count 一致性字段。

## 6. Correctness and performance test plan

A0 必测场景：

- single target level；
- multiple target levels：clustered、sparse、random；
- no ring-degree switch；
- ring-degree switch；
- conjugate-invariant / standard ring switch，如果目标 profile 使用；
- packed bootstrapping / `BootstrapMany`；
- invalid target level rejects before keygen；
- output level mismatch fails；
- output scale mismatch fails。

每个 correctness case 必须：

- 生成 CKKS plaintext vector；
- 加密到 native bootstrapper 可接受的 input level；
- 使用 direct `BK(r)` bootstrapping；
- 断言 `ctOut.Level()==r`；
- 断言 `ctOut.Scale==FixedTargetScale`；
- 使用 `ckks.GetPrecisionStats` 记录 real/imag average log2 precision；
- 与 case precision threshold 比较。

每个 target level 必须记录：

- keygen time；
- evaluator/matrix construction time；
- bootstrap latency；
- peak keygen heap；
- peak runtime heap；
- key binary size；
- matrix binary size；
- generated evaluation-key count；
- generated rotation-key count；
- generated encoded diagonal count。

keygen 后发生 correctness failure 时仍要写 metrics 和 baseline 明细。keygen 前拒绝的
case 只写 failure 和 summary。

## 7. Required commands

激活 repo-local Go toolchain：

```powershell
. .\.toolchain\use-go.ps1
```

native quick regression：

```powershell
go test ./circuits/ckks/bootstrapping -run '^TestBootstrapping$' -count=1 -timeout=30m -args -print-precision
```

A0 单 case 标准命令：

```powershell
$runDir = "docs/bootstrap_key_reuse/results/<YYYYMMDD-HHMMSS>_a0_<case_id>"
New-Item -ItemType Directory -Force -Path $runDir | Out-Null

go test ./circuits/ckks/bootstrapping `
  -run '^TestBootstrapKeyReuseA0_<CaseName>$' `
  -count=1 `
  -timeout=30m `
  -args -print-precision '-bkr.result-dir' $runDir

if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
```

A0 benchmark 标准命令：

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

标准命令不使用 `go test -json`，不重定向 stdout/stderr，不使用 `Tee-Object`。

## 8. How A1-A6 consume A0 baseline

- A1：只在 `UsedTargetLevels` 上与 A0 对比，并证明 unused target levels 无 keygen。
- A2：读取 `galois_key_baseline.csv` 计算 union、overlap、rotation-key saving。
- A3：读取 `linear_transform_schedule_baseline.csv` 判断 schedule equality。
- A4：读取 `encoded_diagonal_baseline.csv` 和 `material_baseline_index.csv` 判断 exact-compatible sharing。
- A5：以 A0 exact direct run 作为 prefix/superset view 正确性参照。
- A6：以 A0 direct `BK(r)` 作为 high-level bootstrap + `DropLevel` 的参照。

任一 A1-A6 优化都必须保持 A0 的 target level、fixed scale 和 precision 语义。

## 9. Assumptions

- A0 是 correctness/performance/material identity baseline。
- A0 当前使用 Lattigo 原生 Q-prefix residual parameters 实现不同 target output level。
- A0 不改生产代码，不新增 public API。
- A0 不在 target levels 之间共享 native key、schedule 或 encoded diagonal pointer。
