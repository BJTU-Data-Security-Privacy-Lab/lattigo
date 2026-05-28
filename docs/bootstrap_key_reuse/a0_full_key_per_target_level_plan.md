# A0 FullKeyPerTargetLevel Baseline Plan

统一测试环境、测试用例、Go 工具链命令和固定参数见：[a0_a6_test_standard.md](a0_a6_test_standard.md)。
Go 工具链固定使用方式见：[toolchain_usage.md](toolchain_usage.md)。

## 1. Purpose and non-goals

A0 is the exact baseline for the multi-target-level bootstrapping key reuse work.
It generates a complete and independent bootstrapping key package for every
configured target level in `AllTargetLevels`.

The purpose of A0 is to provide the correctness, precision, storage, keygen RAM,
runtime RAM, and latency reference that A1-A6 must compare against.

A0 is intentionally not optimized. It must not perform:

- rotation key interning;
- linear-transform schedule interning;
- encoded diagonal sharing;
- RNS prefix or superset views;
- bootstrapping to a higher level followed by `DropLevel`;
- runtime lazy key generation.

A0 may record canonical IDs for reporting and later comparison, but those IDs
must not be used to share or reuse material inside A0.

## 2. A0 generation logic

A0 uses `AllTargetLevels`, not `UsedTargetLevels`.

```text
for r in AllTargetLevels:
    build target-specific bootstrapping configuration for r
    reject before keygen if OutputLevel() cannot be made equal to r
    generate a complete independent BK(r)
    construct an independent native evaluator for BK(r)
    run bootstrap correctness and metric collection for r
```

For each `r`, A0 must independently generate or construct:

- `bootstrapping.EvaluationKeys`;
- all ring-switch and dense/sparse switch keys required by the native
  bootstrapping parameters;
- the relinearization key;
- all Galois keys required by `btpParams.GaloisElements(...)`;
- CoeffsToSlots and SlotsToCoeffs encoded matrices;
- the native evaluator used to execute bootstrapping for that target.

The A0 harness must assert the target-level contract before accepting a run:

```text
evaluator.OutputLevel() == r
ctOut.Level() == r
ctOut.Scale == FixedTargetScale
```

If a target-specific native evaluator cannot satisfy `OutputLevel() == r`, A0
must fail the case before recording correctness or performance metrics. This
prevents later A1-A6 experiments from comparing against an invalid baseline.

## 3. Common experiment data contract for A0-A6

All A0-A6 experiments must use the same case description and result schema so
that each ablation can be compared against A0 without interpretation changes.

### ExperimentCase

```text
ExperimentCase {
  case_id
  params_profile
  all_target_levels
  used_target_levels
  fixed_target_scale
  log_slots
  ring_switch_mode
  packed_mode
  seed
  repeat_count
}
```

Rules:

- `all_target_levels` is the A0 input set.
- `used_target_levels` is only used by A1 and later demand-driven variants.
- `fixed_target_scale` must be identical for A0-A6 in the same case.
- `seed` must drive plaintext generation and encryption randomness whenever the
  test harness exposes deterministic sampling.

### TargetRunResult

```text
TargetRunResult {
  case_id
  target_level
  output_level
  output_scale_equal_fixed_target_scale
  avg_log2_precision_real
  avg_log2_precision_imag
  bootstrap_error_status
}
```

Rules:

- `output_level` must equal `target_level`.
- `output_scale_equal_fixed_target_scale` must be true.
- Precision should follow the existing Lattigo bootstrapping test style using
  `ckks.GetPrecisionStats`.

### MaterialMetrics

```text
MaterialMetrics {
  case_id
  target_level
  generated_evaluation_key_count
  generated_rotation_key_count
  generated_encoded_diagonal_count
  persistent_key_bytes
  persistent_matrix_bytes
  shared_rotation_keys
  shared_encoded_diagonals
  rns_slice_success
  fallback_reason
}
```

A0 defaults:

```text
shared_rotation_keys = 0
shared_encoded_diagonals = 0
rns_slice_success = not_applicable
fallback_reason = none
```

Rules:

- `persistent_key_bytes` should use native `BinarySize` when available.
- If encoded matrices do not expose native binary size, estimate matrix bytes as
  `number_of_QP_limbs * ring_degree * 8` per stored polynomial and report that
  the value is estimated.
- Galois key count must be counted after native key generation, but without
  dedup across target levels.

### RuntimeMetrics

```text
RuntimeMetrics {
  case_id
  target_level
  keygen_time
  evaluator_and_matrix_construction_time
  bootstrap_latency
  peak_keygen_heap
  peak_runtime_heap
}
```

Rules:

- key generation time and evaluator/matrix construction time must be measured
  separately.
- bootstrap latency must be measured after the evaluator is constructed.
- peak heap measurements must separate keygen phase and runtime phase.

## 4. Complete A0 correctness and performance test plan

### Correctness coverage

A0 must include these scenarios:

- single target level;
- multiple target levels with clustered, sparse, and random distributions;
- bootstrapping without ring-degree switch;
- bootstrapping with ring-degree switch;
- conjugate-invariant to standard ring switch, if the target profile uses it;
- standard to conjugate-invariant ring switch, if the target profile uses it;
- packed bootstrapping through `BootstrapMany`;
- invalid target level rejected before keygen;
- forced output-level mismatch fails the test;
- forced output-scale mismatch fails the test.

Each correctness case must:

- generate random CKKS plaintext vectors;
- encrypt at a valid input level for the native bootstrapper;
- run A0 `BK(r)`;
- assert `ctOut.Level() == r`;
- assert `ctOut.Scale == FixedTargetScale`;
- decrypt and compare precision against the case threshold.

### Precision checks

Precision checks must follow the existing bootstrapping test pattern:

```text
precStats = ckks.GetPrecisionStats(...)
avgReal = precStats.AVGLog2Prec.Real
avgImag = precStats.AVGLog2Prec.Imag
```

Each target level must record `avgReal` and `avgImag`. The minimum threshold
should match the native bootstrapping test policy for the chosen parameter
profile unless the experiment case explicitly defines a stricter threshold.

### Metric checks

For every `r`, A0 must record:

- keygen time;
- evaluator and matrix construction time;
- bootstrap latency;
- peak keygen heap;
- peak runtime heap;
- key binary size;
- encoded matrix byte estimate or native size;
- generated evaluation-key count;
- generated rotation-key count;
- generated encoded diagonal count.

The test harness must report metrics even when a correctness assertion fails,
as long as the failure happens after target-specific keygen has completed. A
case rejected before keygen must record only the rejection reason.

## 5. A0 test code placement and naming

A0 implementation must follow the global test-code layout in
[a0_a6_test_standard.md](a0_a6_test_standard.md). The A0-specific test code must
live in the native bootstrapping package:

```text
circuits/ckks/bootstrapping/
```

The A0 test file must be:

```text
bootstrap_key_reuse_a0_test.go
```

Shared helpers used by A0 and later A1-A6 lanes must live in:

```text
bootstrap_key_reuse_common_test.go
bootstrap_key_reuse_metrics_test.go
```

A0 test functions must use:

```text
TestBootstrapKeyReuseA0_<CaseName>
```

A0 benchmark functions must use:

```text
BenchmarkBootstrapKeyReuseA0_<CaseName>
```

`<CaseName>` must be derived from the fixed parameter profile and target-set
name in the unified test standard, for example:

```text
P0TinyNativeSingle
P2MultiFastClustered
P4N15DenseSparse
P5N16SparseLongRandom
```

A0 helper names must be unexported and must not introduce public API. Preferred
helper names are:

```text
buildBootstrapKeyReuseCase
runBootstrapKeyReuseA0
collectBootstrapKeyReuseMetrics
writeBootstrapKeyReuseResult
```

## 6. A0 output location and file format

A0 output must follow the global output contract in
[a0_a6_test_standard.md](a0_a6_test_standard.md). Each A0 run must write to:

```text
docs/bootstrap_key_reuse/results/<YYYYMMDD-HHMMSS>_a0_<case_id>/
```

Example:

```text
docs/bootstrap_key_reuse/results/20260528-153000_a0_P2_MULTI_FAST_clustered/
```

Each A0 run directory must contain:

```text
metadata.json
go-test.jsonl
results.jsonl
summary.json
metrics.csv
stdout.log
stderr.log
```

A0-specific output rules:

- `metadata.json` must record `lane = "a0"`, the exact Go command, Go env,
  git branch, git commit, dirty state, fixed parameter profile, and
  `AllTargetLevels`.
- `results.jsonl` must include one `ExperimentCase` record per case and one
  `TargetRunResult`, `MaterialMetrics`, and `RuntimeMetrics` record per target
  level.
- `summary.json` must set `lane = "a0"` and must report all target levels in
  `AllTargetLevels`, including failed target levels.
- `metrics.csv` must use the fixed header defined in the unified test standard.
- `stdout.log` and `stderr.log` must preserve raw command output.

A0 default metric values are fixed:

```text
shared_rotation_keys = 0
shared_encoded_diagonals = 0
rns_slice_success = not_applicable
fallback_reason = none
```

If A0 rejects a target before keygen because `OutputLevel() != r`, it must still
write a `Failure` record to `results.jsonl` and mark the run `status = "fail"`
in `summary.json`.

## 7. A0 required commands

Before running A0 tests, the local Go toolchain must be activated:

```powershell
. .\.toolchain\use-go.ps1
```

The minimum native regression command that must pass before A0-specific tests is:

```powershell
go test ./circuits/ckks/bootstrapping -run '^TestBootstrapping$' -count=1 -timeout=30m -args -print-precision
```

Once `bootstrap_key_reuse_a0_test.go` exists, the A0-specific quick command must
be:

```powershell
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

A0 benchmark commands must use Go's native benchmark toolchain:

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

## 8. How A1-A6 consume A0 baseline results

A0 produces the reference data used by all later ablations.

- A1 compares only target levels in `UsedTargetLevels` against A0 and must prove
  that unused levels in `AllTargetLevels - UsedTargetLevels` are not generated.
- A2 uses A0 per-target Galois element sets to compute union size, overlap
  ratio, and rotation-key savings.
- A3 uses A0 per-target linear-transform schedules to compute schedule equality
  and schedule reuse opportunities.
- A4 uses A0 encoded diagonal hashes and metadata to decide exact-compatible
  encoded diagonal sharing.
- A5 uses A0 exact runs as the correctness reference for evaluation-key prefix
  views and encoded diagonal RNS prefix views.
- A6 uses A0 direct `BK(r)` outputs as the reference for any high-level
  bootstrap followed by `DropLevel` strategy.

The acceptance rule for A1-A6 is simple: an optimized run may reduce generated
material, storage, or RAM, but it must preserve the A0 target-level contract and
must not reduce precision beyond the experiment threshold.

## 9. Assumptions

- A0 is a correctness and performance baseline, not an optimization.
- A0 uses the same CKKS parameters, secret-key domain, modulus chain, and fixed
  target scale as A1-A6 in the same experiment case.
- The target-specific evaluator must expose `OutputLevel() == r`; otherwise the
  case is invalid for A0.
- A0 stores no shared native pointers between target levels, even when two
  independently generated materials have the same canonical ID.
