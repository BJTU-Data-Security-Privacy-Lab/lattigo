# MPCKKS Ciphertext-Ciphertext Multiplication Baseline

This page records the benchmark parameters, method, and result index for the current multi-key CKKS ciphertext-ciphertext multiplication baseline.

## Parameters

| Field | Value |
|---|---:|
| Parties | 3 |
| Per-party key | independent secret-key share |
| Collective key | sum of 3 secret-key shares |
| LogN | 14 |
| N | 16384 |
| LogQ | `[50, 40, 40, 40, 40, 40, 40, 40]` |
| Qi | 8 |
| logQ | 330 |
| LogP | `[60]` |
| Pi | 1 |
| logP | 60 |
| logPQ | 390 |
| LogDefaultScale | 40 |
| MaxLevel | 7 |

For `ring.Standard`, the input uses 8192 complex slots. For `ring.ConjugateInvariant`, the benchmark also reports 16384 real slots as a reference.

## Method

The benchmark runs the online evaluator phase only. Collective relinearization key generation is performed before timing and is not included in the measured operation latency.

Measured operations:

- `Evaluator/Mul/Ciphertext/Ciphertext`: degree-1 by degree-1 multiplication, output degree 2.
- `Evaluator/Relinearize/CiphertextDegree2`: relinearization of a precomputed degree-2 ciphertext, output degree 1.
- `Evaluator/MulRelin/Ciphertext/Ciphertext`: fused multiplication and relinearization, output degree 1.
- `Evaluator/MulThenRelinearize/Ciphertext/Ciphertext`: split multiply then relinearize path, output degree 1.
- `EvaluatorParallel/MulRelin/Ciphertext/Ciphertext`: parallel fused multiplication and relinearization throughput reference.

Command:

```powershell
. .\.tools\use-go.ps1
$params = '{\"LogN\":14,\"LogQ\":[50,40,40,40,40,40,40,40],\"LogP\":[60],\"LogDefaultScale\":40}'
go test -mod=readonly ./multiparty/mpckks -run '^$' -bench 'BenchmarkMultiPartyCKKS/Evaluator/(Mul|Relinearize|MulRelin|MulThenRelinearize)' -benchtime=1s -count=3 -params $params
```

## Result Index

- Baseline log: [../../logs/mpckks-ctct-mul-baseline-2026-05-20.md](../../logs/mpckks-ctct-mul-baseline-2026-05-20.md)
- Primary comparison point: `ring.Standard / Evaluator/MulRelin/Ciphertext/Ciphertext = 15.919 ms/op`
