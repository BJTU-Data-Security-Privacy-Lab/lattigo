# MPCKKS Ciphertext-Ciphertext Multiplication Baseline

Date: 2026-05-20

Environment:

- OS/arch: Windows amd64
- CPU: Intel(R) Core(TM) i7-14700K
- Go: go1.25.0 windows/amd64
- Package: `github.com/tuneinsight/lattigo/v6/multiparty/mpckks`

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

Command:

```powershell
. .\.tools\use-go.ps1
$params = '{\"LogN\":14,\"LogQ\":[50,40,40,40,40,40,40,40],\"LogP\":[60],\"LogDefaultScale\":40}'
go test -mod=readonly ./multiparty/mpckks -run '^$' -bench 'BenchmarkMultiPartyCKKS/Evaluator/(Mul|Relinearize|MulRelin|MulThenRelinearize)' -benchtime=1s -count=3 -params $params
```

## Average Results

The table reports the average over `-count=3`.

| RingType | Operation | ns/op avg | ms/op avg | B/op avg | allocs/op avg | input degree | output degree | slots |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| Standard | Mul | 544829 | 0.545 | 560 | 8 | 1 | 2 | 8192 |
| Standard | Relinearize | 15208915 | 15.209 | 7278 | 153 | 2 | 1 | 8192 |
| Standard | MulRelin | 15918953 | 15.919 | 7580 | 164 | 1 | 1 | 8192 |
| Standard | MulThenRelinearize | 16074507 | 16.075 | 7867 | 161 | 1 | 1 | 8192 |
| Standard | Parallel MulRelin | 2272540 | 2.273 | 842544 | 222 | 1 | 1 | 8192 |
| ConjugateInvariant | Mul | 554763 | 0.555 | 560 | 8 | 1 | 2 | 16384 |
| ConjugateInvariant | Relinearize | 16357086 | 16.357 | 8497 | 153 | 2 | 1 | 16384 |
| ConjugateInvariant | MulRelin | 17869380 | 17.869 | 9482 | 164 | 1 | 1 | 16384 |
| ConjugateInvariant | MulThenRelinearize | 17186255 | 17.186 | 9352 | 161 | 1 | 1 | 16384 |
| ConjugateInvariant | Parallel MulRelin | 2386612 | 2.387 | 861827 | 221 | 1 | 1 | 16384 |

## Raw Samples

| RingType | Operation | ns/op samples | B/op samples | allocs/op samples |
|---|---|---:|---:|---:|
| Standard | Mul | `552184, 534061, 548241` | `560, 560, 560` | `8, 8, 8` |
| Standard | Relinearize | `15195395, 15149741, 15281609` | `7896, 7826, 6111` | `153, 153, 153` |
| Standard | MulRelin | `15879204, 15798569, 16079085` | `8773, 6984, 6984` | `164, 164, 164` |
| Standard | MulThenRelinearize | `16031931, 16133858, 16057731` | `10258, 6672, 6672` | `161, 161, 161` |
| Standard | Parallel MulRelin | `2226192, 2289665, 2301764` | `862758, 862023, 802851` | `223, 223, 219` |
| ConjugateInvariant | Mul | `549005, 553715, 561569` | `560, 560, 560` | `8, 8, 8` |
| ConjugateInvariant | Relinearize | `15948024, 16456677, 16666557` | `6112, 7880, 11500` | `153, 153, 153` |
| ConjugateInvariant | MulRelin | `18441781, 17092967, 18073392` | `8875, 6986, 12585` | `164, 164, 164` |
| ConjugateInvariant | MulThenRelinearize | `17081136, 17837810, 16639818` | `6672, 11026, 10358` | `161, 161, 161` |
| ConjugateInvariant | Parallel MulRelin | `2369827, 2398469, 2391539` | `799413, 851990, 934078` | `217, 220, 226` |

## Notes

- Primary baseline for future optimization: `Standard / MulRelin = 15.919 ms/op`.
- The benchmark records online multiplication only; collective RKG setup is outside the timed section.
- `ring.ConjugateInvariant` is retained as a real-slot reference, not the primary optimization target.
