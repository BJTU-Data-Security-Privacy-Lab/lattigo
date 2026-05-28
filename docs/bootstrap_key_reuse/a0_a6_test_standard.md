# A0-A6 统一测试标准

本文档固定 A0-A6 后续测试环境、测试用例、Go 工具链命令、具体参数和通过标准。所有测试标准只使用 Go 原生命令体系：`go test`、`go test -json`、`go test -bench`、`-benchmem`、`-count`、`-run`、`-args`。不把 Python、shell 脚本或第三方 benchmark 工具作为标准依赖。

## 1. 测试目标

A0 是唯一 baseline。A1-A6 的正确性、精度、存储、keygen 时间、运行时延迟、keygen RAM 和 runtime RAM 都必须与 A0 对比。

同一个 experiment case 中，A0-A6 必须固定：

- 同一 CKKS parameters；
- 同一 secret-key domain；
- 同一 modulus chain；
- 同一 fixed target scale；
- 同一 plaintext / ciphertext 输入 seed；
- 同一 `AllTargetLevels` 和对应的 `UsedTargetLevels` 定义。

A0-A6 的区别只能来自对应 ablation 允许的优化策略，不能通过改变参数、输入或精度阈值制造收益。

## 2. 测试环境

Go 版本基准是 `go.mod` 中的 `go 1.25.0`。每次实验报告必须记录实际 `go version`。

Go 工具链的固定激活方式见：[toolchain_usage.md](toolchain_usage.md)。A0-A6 测试必须先按该文档激活 repo-local Go toolchain，避免误用系统 Go 或不同 module proxy。

每次结果必须记录：

- `go version`；
- `GOOS` / `GOARCH`；
- CPU 型号和物理内存；
- `GOMAXPROCS`；
- git branch；
- git commit；
- git dirty state；
- 测试命令原文。

latency 和 ablation 标准 lane 固定 `GOMAXPROCS=1`。throughput lane 单独允许使用多核，并且必须单独报告，不能和 scalar latency 混合比较。

Windows PowerShell 示例：

```powershell
$env:GOMAXPROCS = "1"
go version
git branch --show-current
git rev-parse HEAD
git status --short
```

Linux/macOS shell 示例：

```bash
GOMAXPROCS=1 go version
git branch --show-current
git rev-parse HEAD
git status --short
```

## 3. Go 工具链命令

### 3.1 快速正确性

用于快速确认 native bootstrapping 主路径仍然正确。

```bash
go test ./circuits/ckks/bootstrapping -run '^TestBootstrapping$' -count=1 -timeout=30m -args -print-precision
```

### 3.2 circuit 原始路径

用于覆盖 `TestCircuitOriginal` 和 `TestCircuitWithEncapsulation`，包括原始 circuit 路径和 encapsulation 路径。

```bash
go test ./circuits/ckks/bootstrapping -run '^TestCircuit(Original|WithEncapsulation)$' -count=1 -timeout=30m -args -print-precision
```

### 3.3 全默认参数长测

用于覆盖 Lattigo 默认 dense/sparse bootstrapping 参数。该 lane 可能耗时很长，必须使用 `-timeout=0`。

```bash
go test ./circuits/ckks/bootstrapping -run '^TestAllParameters$' -count=1 -timeout=0 -args -long -allparams -print-precision
```

### 3.4 JSON 采集

用于后续从 Go 原生 JSON 事件中提取测试结果和日志。标准只要求生成 `go test -json` 输出，不要求本仓库依赖外部解析工具。

```bash
go test -json ./circuits/ckks/bootstrapping -run '^TestBootstrapping$' -count=1 -timeout=30m -args -print-precision
```

### 3.5 并发吞吐 benchmark

用于 throughput lane。该结果只报告并发吞吐，不参与 scalar latency 的直接比较。

```bash
go test ./circuits/ckks/bootstrapping -run '^$' -bench '^BenchmarkConcurrentBootstrap$' -benchmem -count=3 -timeout=0
```

### 3.6 A0 target-level harness

A0-A6 专用 harness 必须显式指定结果目录。PowerShell 下 `-bkr.result-dir`
必须加引号，避免被拆成 `-bkr`。

每次命令只运行一个 A0 case，并把 `go test -json`、stdout、stderr 写入同一
run directory：

```powershell
. .\.toolchain\use-go.ps1
$runDir = "docs/bootstrap_key_reuse/results/<YYYYMMDD-HHMMSS>_a0_<case_id>"
New-Item -ItemType Directory -Force -Path $runDir | Out-Null

go test -json ./circuits/ckks/bootstrapping `
  -run '^TestBootstrapKeyReuseA0_<CaseName>$' `
  -count=1 `
  -timeout=30m `
  -args -print-precision '-bkr.result-dir' $runDir `
  2> "$runDir\stderr.log" |
  Tee-Object -FilePath "$runDir\go-test.jsonl" |
  Tee-Object -FilePath "$runDir\stdout.log"

if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
```

A0 benchmark 只使用 Go 原生 benchmark：

```powershell
go test ./circuits/ckks/bootstrapping `
  -run '^$' `
  -bench '^BenchmarkBootstrapKeyReuseA0_<CaseName>$' `
  -benchmem `
  -count=3 `
  -timeout=0 `
  -args '-bkr.result-dir' $runDir
```

## 4. 固定测试参数

后续 A0-A6 harness 必须使用下列参数集合命名。现有 Lattigo 原生测试命令仍主要覆盖 native bootstrapping 行为；其中 `P2_MULTI_FAST` 是后续 target-level harness 的固定快速多 level 参数。

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

用途：后续 A0-A6 target-level harness 的快速多 target level 参数。

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

### 5.1 A0 必测

- 对 `AllTargetLevels` 全量生成完整独立 `BK(r)`。
- 不允许任何共享、prefix view、superset view 或 `DropLevel` 替代。
- 每个 `r` 断言 `OutputLevel() == r`。
- 每个输出 ciphertext 断言 `ctOut.Level() == r`。
- 每个输出 ciphertext 断言 `ctOut.Scale == FixedTargetScale`。

### 5.2 A1 必测

- 只生成 `UsedTargetLevels`。
- 未使用 target levels 必须无 keygen 记录。
- 对 used levels 的 level、scale、precision 必须满足 A0 标准。

### 5.3 A2 必测

- 使用 A0 per-target Galois sets 计算 union、overlap、共享率。
- rotation key interning 后 bootstrap 结果必须匹配 A0 的 level、scale、precision 标准。
- Galois key canonical ID 必须使用 Lattigo 实际 `gal_el`。

### 5.4 A3 必测

- schedule ID 相同才允许 schedule interning。
- encoded diagonal 不在 A3 共享。
- A3 不得改变 CoeffsToSlots / SlotsToCoeffs 数学语义。

### 5.5 A4 必测

- 只允许 exact-compatible encoded diagonal sharing。
- 不允许 RNS prefix slicing。
- exact-compatible 必须同时匹配 diagonal、scale、LevelQ、LevelP、modulus prefix、logSlots、encoding precision 和 NTT/Montgomery domain。

### 5.6 A5 必测

- 默认关闭。
- 必须通过 exact vs prefix/superset 等价测试。
- 等价测试必须比较 output level、output scale、decoded precision、rotation stress cases。

### 5.7 A6 必测

- 默认 exploratory。
- high-level bootstrap + `DropLevel` 必须与 A0 direct `BK(r)` 比较 level、scale、precision、latency。
- A6 不得把 latency 收益和 storage 收益混在一个指标里报告。

## 6. 测试代码位置与命名规范

后续 A0-A6 专用测试代码必须放在 Lattigo bootstrapping 包内，避免跨包访问 native evaluator、parameters、keys 和 DFT matrix 时引入额外 adapter：

```text
circuits/ckks/bootstrapping/
```

测试文件命名必须使用下列模式：

```text
bootstrap_key_reuse_<lane>_test.go
```

其中 `<lane>` 必须是下列值之一：

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

推荐文件划分：

```text
bootstrap_key_reuse_common_test.go
bootstrap_key_reuse_metrics_test.go
bootstrap_key_reuse_a0_test.go
bootstrap_key_reuse_a1_test.go
bootstrap_key_reuse_a2_test.go
bootstrap_key_reuse_a3_test.go
bootstrap_key_reuse_a4_test.go
bootstrap_key_reuse_a5_test.go
bootstrap_key_reuse_a6_test.go
```

测试函数命名必须使用下列模式：

```text
TestBootstrapKeyReuseA0_<CaseName>
TestBootstrapKeyReuseA1_<CaseName>
TestBootstrapKeyReuseA2_<CaseName>
TestBootstrapKeyReuseA3_<CaseName>
TestBootstrapKeyReuseA4_<CaseName>
TestBootstrapKeyReuseA5_<CaseName>
TestBootstrapKeyReuseA6_<CaseName>
```

benchmark 函数命名必须使用下列模式：

```text
BenchmarkBootstrapKeyReuseA0_<CaseName>
BenchmarkBootstrapKeyReuseA1_<CaseName>
BenchmarkBootstrapKeyReuseA2_<CaseName>
BenchmarkBootstrapKeyReuseA3_<CaseName>
BenchmarkBootstrapKeyReuseA4_<CaseName>
BenchmarkBootstrapKeyReuseA5_<CaseName>
BenchmarkBootstrapKeyReuseA6_<CaseName>
```

`<CaseName>` 必须来自固定参数集合和 target set 名称，例如：

```text
P2MultiFastClustered
P4N15DenseSparse
P5N16SparseLongRandom
```

公共 helper 只能放在 `bootstrap_key_reuse_common_test.go` 或 `bootstrap_key_reuse_metrics_test.go`。helper 命名使用小写开头，避免导出到包外：

```text
buildBootstrapKeyReuseCase
runBootstrapKeyReuseLane
collectBootstrapKeyReuseMetrics
writeBootstrapKeyReuseResult
```

测试代码不得新增 public API。若必须为测试暴露内部能力，优先通过同包 `_test.go` helper 实现。

## 7. 测试输出文件位置与格式

所有 A0-A6 专用测试输出必须写入：

```text
docs/bootstrap_key_reuse/results/
```

该目录只保存本地实验结果，已经通过 `.git/info/exclude` 排除，不加入 Git 追踪。测试代码必须在目录不存在时创建目录。

每次测试运行必须创建一个独立 run directory：

```text
docs/bootstrap_key_reuse/results/<YYYYMMDD-HHMMSS>_<lane>_<case_id>/
```

示例：

```text
docs/bootstrap_key_reuse/results/20260528-153000_a0_P2_MULTI_FAST_clustered/
```

run directory 内必须使用固定文件名：

```text
metadata.json
go-test.jsonl
results.jsonl
summary.json
metrics.csv
stdout.log
stderr.log
```

文件格式约束：

- `metadata.json`：单个 JSON object，记录环境、命令、git 状态、Go env、case 配置。
- `go-test.jsonl`：`go test -json` 原始事件流，每行一个 JSON object。
- `results.jsonl`：A0-A6 harness 输出的结构化结果，每行一个 JSON object。
- `summary.json`：单个 JSON object，记录本次 run 的汇总结果和通过/失败状态。
- `metrics.csv`：便于表格分析的指标摘要，第一行必须是 header。
- `stdout.log`：测试命令标准输出原文。
- `stderr.log`：测试命令标准错误原文。

`metadata.json` 必须至少包含：

```json
{
  "schema_version": "bootstrap-key-reuse-test/v1",
  "lane": "a0",
  "case_id": "P2_MULTI_FAST_clustered",
  "command": "go test ...",
  "go_version": "go1.25.0 windows/amd64",
  "go_env": {
    "GOVERSION": "go1.25.0",
    "GOOS": "windows",
    "GOARCH": "amd64",
    "GOMOD": "G:\\2026-project\\lattigo\\go.mod",
    "GOROOT": "G:\\2026-project\\lattigo\\.toolchain\\go",
    "GOPATH": "G:\\2026-project\\lattigo\\.toolchain\\gopath",
    "GOCACHE": "G:\\2026-project\\lattigo\\.toolchain\\go-cache",
    "GOPROXY": "https://goproxy.cn,direct",
    "GOSUMDB": "sum.golang.google.cn",
    "GOTOOLCHAIN": "local",
    "GOMAXPROCS": "1"
  },
  "git": {
    "branch": "bootstrap-key-develop",
    "commit": "<commit>",
    "dirty": true
  }
}
```

`results.jsonl` 中每一行必须包含 `record_type` 字段。允许的 `record_type` 为：

```text
ExperimentCase
TargetRunResult
MaterialMetrics
RuntimeMetrics
AblationComparison
Failure
```

`summary.json` 必须至少包含：

```json
{
  "schema_version": "bootstrap-key-reuse-test/v1",
  "lane": "a0",
  "case_id": "P2_MULTI_FAST_clustered",
  "status": "pass",
  "target_levels": [1, 2, 3],
  "failed_targets": [],
  "precision_min_real": 12.0,
  "precision_min_imag": 12.0,
  "keygen_time_ms": 0,
  "bootstrap_latency_ms": 0
}
```

`metrics.csv` header 必须固定为：

```text
schema_version,lane,case_id,params_profile,target_level,status,output_level,scale_ok,average_log2_precision_real,average_log2_precision_imag,generated_evaluation_key_count,generated_rotation_key_count,generated_encoded_diagonal_count,persistent_key_bytes,persistent_matrix_bytes,keygen_time_ms,evaluator_matrix_construction_time_ms,bootstrap_latency_ms,peak_keygen_heap_bytes,peak_runtime_heap_bytes,shared_rotation_keys,shared_encoded_diagonals,rns_slice_success,fallback_reason
```

本标准不要求测试代码实现外部解析器；但所有输出必须是上述 JSON/JSONL/CSV 形态，确保后续可以用任意工具离线分析。

## 8. 指标与通过标准

### 8.1 统一记录对象

所有 A0-A6 测试至少记录下列对象：

```text
ExperimentCase
TargetRunResult
MaterialMetrics
RuntimeMetrics
AblationComparison
```

### 8.2 精度标准

- 使用 `ckks.GetPrecisionStats`。
- 记录 `AVGLog2Prec.Real` 和 `AVGLog2Prec.Imag`。
- quick lane 最低阈值沿用现有测试风格，默认不低于 `12.0` bits。
- long lane 不低于对应 default parameter 注释中的预期 precision 下界减容忍值。
- 如果一个 case 显式设置更高阈值，以 case 阈值为准。

### 8.3 性能标准

- scalar latency 使用 `GOMAXPROCS=1` 的 ablation lane。
- throughput 只使用 `BenchmarkConcurrentBootstrap`，单独报告。
- benchmark 至少使用 `-count=3`。
- keygen time、evaluator/matrix construction time、bootstrap latency 必须分开记录。
- peak keygen heap 和 peak runtime heap 必须分开记录。

### 8.4 A1-A6 通过条件

- 不得破坏 A0 的 target level 和 fixed scale 语义。
- 不得低于 precision 阈值。
- 任一共享 predicate 不满足时必须 fallback 或拒绝，不能静默共享。
- 如果某个 optimization lane 默认关闭，报告中必须明确标注 `off`、`optional` 或 `exploratory`。

## 9. 与现有 Lattigo 测试的关系

现有 `circuits/ckks/bootstrapping` 测试已经提供 native bootstrapping 正确性路径、precision 输出和部分 benchmark。本文档固定的是 A0-A6 复用实验的统一标准；后续专用 target-level harness 必须遵守本文档的参数集合、Go 命令体系、记录对象和通过标准。

在没有专用 harness 前，Go 原生命令 lane 负责保证 native bootstrapping 行为没有回归。A0-A6 harness 落地后，必须继续保留这些 native lane 作为回归门槛。
