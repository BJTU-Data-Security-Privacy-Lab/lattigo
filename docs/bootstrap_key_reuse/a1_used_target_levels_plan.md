# A1 UsedTargetLevelsOnly 计划

## 1. A1 目标

A1 同时完成两个目标：

- 只为 `UsedTargetLevels` 生成完整独立 `BK(r)`，不为 unused target levels 生成 key、evaluator、encoded matrices 或 runtime baseline 行。
- 在 `bootstrapping` 包内提供未导出的 target-level dispatcher，使内部代码可以显式请求 bootstrap 到指定 `targetLevel`。

A1 不是共享优化。A1 禁止 rotation key interning、schedule interning、encoded diagonal sharing、RNS prefix/superset view，以及 high-level bootstrap 后 `DropLevel`。

## 2. 内包 target-level 能力

A1 新增内包类型：

```go
type targetLevelBootstrapper struct
```

它维护：

```go
map[int]*Evaluator
```

语义是：

```text
target level r -> 完整独立 BK(r) 对应的 Evaluator
```

该类型只做 dispatch：

- 不负责 keygen。
- 不构造 CKKS parameters。
- 不构造 bootstrapping parameters。
- 不做 fallback。
- 不用更高 target level evaluator 替代低 target level evaluator。

构造时必须验证：

- evaluator map 非空。
- evaluator 非 nil。
- `eval.OutputLevel() == targetLevel`。
- target levels 排序保存，`targetLevels()` 返回 copy。

调用时必须验证请求的 target level 已配置；未配置时立即返回 error。

## 3. A1 generation logic

输入仍然包含两组 target levels：

- `AllTargetLevels`：配置层完整 target set。
- `UsedTargetLevels`：当前 workload 实际需要的 target set。

A1 预检查：

- `UsedTargetLevels` 非空。
- `UsedTargetLevels` 无重复。
- `UsedTargetLevels` 必须是 `AllTargetLevels` 的子集。
- 每个 target level 必须在当前 profile modulus chain 范围内。

对每个 `r in UsedTargetLevels`：

- 使用与 A0 相同的 Q-prefix target residual 参数构造规则：`LogQ[:r+1]`。
- 使用同一 profile、同一 fixed target scale、同一 seed 派生 deterministic secret-key domain。
- 独立调用 Lattigo keygen 生成完整 `bootstrapping.EvaluationKeys`。
- 独立构造 `Evaluator` 和 C2S/S2C encoded matrices。
- 将 evaluator 放入 `targetLevelBootstrapper`。
- 通过 `bootstrapAtLevel(ct, r)` 或 `bootstrapManyAtLevel(cts, r)` 执行 bootstrap。
- 断言 `OutputLevel()==r`、`ctOut.Level()==r`、`ctOut.Scale==FixedTargetScale`。

对 `AllTargetLevels - UsedTargetLevels`：

- 不构造 target residual parameters。
- 不生成 secret key。
- 不调用 keygen。
- 不构造 evaluator。
- 不写 target/material/runtime/baseline CSV 行。

## 4. CSV output contract

A1 仍使用 A0-A6 统一 CSV-only 文件集合。

关键语义：

- `metadata.csv` 中 `lane=a1`。
- `metadata.csv` 和所有 record CSV 中 `plan_id=A1_UsedTargetLevelsOnly`。
- `experiment_case.csv` 同时记录 `all_target_levels` 和 `used_target_levels`。
- `summary.csv` 的 `target_levels` 表示本次实际运行 target levels，即 `UsedTargetLevels`。
- `target_results.csv`、`material_metrics.csv`、`runtime_metrics.csv` 和所有 baseline CSV 的 target set 必须严格等于 `UsedTargetLevels`。

A1 默认 metrics 字段：

```text
shared_rotation_keys=0
shared_encoded_diagonals=0
rns_slice_success=not_applicable
fallback_reason=none
```

## 5. A1 test requirements

必测正向用例：

- `P2_MULTI_FAST`: `All=[1,2,3]`, `Used=[3]`。
- `P2_MULTI_FAST`: `All=[1,2,3]`, `Used=[1,3]`。
- `P2_MULTI_FAST`: `All=[1,2,3]`, `Used=[1,2,3]`。
- packed bootstrapping: 通过 `bootstrapManyAtLevel` 运行。
- `P4_N15_DENSE`: `All=[1,2,3,4]`, `Used=[1,4]`。

必测负向用例：

- `UsedTargetLevels` 包含不在 `AllTargetLevels` 中的 level，必须 keygen 前失败。
- `UsedTargetLevels` 包含重复 level，必须 keygen 前失败。
- `UsedTargetLevels` 为空，必须 keygen 前失败。
- 请求 dispatcher bootstrap 到未配置 target level，必须失败。
- evaluator `OutputLevel()!=targetLevel` 时 dispatcher 构造必须失败。

必测对照：

- 小参数下同时运行 A0 与 A1。
- A1 used targets 的 output level、scale、precision、material counts 必须与 A0 对应 target 对齐。
- A1 unused targets 在所有 target/material/runtime/baseline CSV 中不存在。

## 6. 标准命令

PowerShell 示例：

```powershell
. .\.toolchain\use-go.ps1
$runDir = "docs/bootstrap_key_reuse/results/dev_a1_p2_sparse"
New-Item -ItemType Directory -Force -Path $runDir | Out-Null

go test ./circuits/ckks/bootstrapping `
  -run '^TestBootstrapKeyReuseA1_P2MultiFastUsedSparse$' `
  -count=1 `
  -timeout=30m `
  -args -print-precision '-bkr.result-dir' $runDir
```

标准命令不使用 `go test -json`，不重定向 stdout/stderr，不使用 `Tee-Object`。

## 7. A1 与后续阶段的关系

A1 固定后续 A2-A6 的运行入口语义：

```text
调用方请求 bootstrap 到 r -> 系统必须使用 BK(r) 或后续阶段证明等价的优化 material
```

A2 可以在 A1 的 dispatcher 后面替换 rotation key pool；A3/A4 可以继续替换 schedule 或 encoded diagonal material；A5/A6 只有在证明等价后才能改变 material view 或执行路径。
