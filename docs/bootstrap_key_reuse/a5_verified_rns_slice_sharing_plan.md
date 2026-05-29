# A5 Verified RNS-Slice Sharing Plan

## Goal

A5 is `A4 + verified RNS-slice sharing`. It keeps A4's exact-safe sharing path
as the default runtime behavior, then adds a validated opt-in RNS prefix view for
evaluation-key material. A5 must never enable slicing from an `UNKNOWN` state.

## Scope

- Reuse A4 target preparation, rotation-key interning, schedule interning,
  encoded-diagonal exact sharing, evaluator construction, and dispatcher logic.
- Add an A5 lane and plan ID.
- Keep RNS slicing disabled by default and record that state in
  `rns_slice_success`.
- Add zero-copy prefix-view helpers for uncompressed RLWE evaluation keys and
  Galois keys.
- Validate the source-level assumptions:
  - `GadgetProduct` and `Automorphism` call the gadget product at the runtime
    ciphertext level;
  - the gadget product loops only over the decomposition vector size required
    for that runtime level;
  - `ring.Poly`/`ringqp.Poly` level changes can be represented as prefix views
    over the same coefficient backing arrays.
- Validate behavior with tests:
  - exact low-level key vs high-level superset key;
  - exact low-level key vs zero-copy sliced key view;
  - same plaintext/ciphertext seed;
  - output level and output scale are unchanged;
  - decoded precision remains above the A0-A6 threshold;
  - all rotations required by the current CoeffsToSlots and SlotsToCoeffs
    bootstrapping matrices are stressed.

## Non-Goals

- No default production-path RNS slicing.
- No compressed evaluation-key slicing.
- No cross-target bootstrap rotation-key slicing unless the secret/domain
  predicate is already exact-compatible.
- No high-level bootstrap followed by `DropLevel`; that belongs to A6.

## Implementation Steps

1. Add `bkrLaneA5`, `bkrPlanA5`, and `bkrA5RunConfig`.
2. Change summary aggregation so `summary.csv` reports the aggregate
   `rns_slice_success` from material metrics instead of always writing
   `not_applicable`.
3. Add `runBootstrapKeyReuseA5` as A4 plus an A5 default RNS status of
   `disabled`.
4. Add zero-copy RNS slice helpers for `rlwe.EvaluationKey` and
   `rlwe.GaloisKey`.
5. Add source validation tests for RNS prefix view preconditions.
6. Add equivalence tests that compare:
   - exact key vs superset key on `ApplyEvaluationKey`;
   - exact key vs sliced key on `ApplyEvaluationKey`;
   - exact Galois key vs superset/sliced Galois keys for every bootstrapping
     rotation.
7. Add A5 lane contract tests proving:
   - A5 defaults to disabled RNS slicing;
   - A5 CSV output uses A5 lane/plan IDs;
   - A5 preserves A0 output level, output scale, and bootstrap status for used
     targets.

## Verification

Run:

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run 'TestBootstrapKeyReuseA5' -count=1 -timeout=30m
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run 'TestBootstrapKeyReuse(A0_CSVOnlyOutputContract|A1_CSVOnlyUsedTargetContract|A1_MatchesA0UsedTargets|A2|A3|A4|A5)' -count=1 -timeout=30m
```

## Current P2 Multi-Fast Behavior

The current P2 multi-fast bootstrap lane has no default legal cross-target RNS
slice candidate: A2 rotation domains already fall back when domains differ, and
A4 encoded diagonals differ by level/Q-prefix. Therefore the A5 lane records
RNS slicing as `disabled` by default. The dedicated RNS validation tests cover
the positive legal prefix-view path on RLWE evaluation-key material and all
bootstrapping rotations.
