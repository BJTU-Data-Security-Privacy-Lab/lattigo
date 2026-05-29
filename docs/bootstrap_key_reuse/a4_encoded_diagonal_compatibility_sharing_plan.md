# A4 Encoded-Diagonal Compatibility Sharing Plan

## Goal

A4 is `A3 + encoded diagonal compatibility sharing`. It keeps A3's
used-target dispatcher, rotation-key pool, and linear-transform schedule pool,
then interns encoded CoeffsToSlots and SlotsToCoeffs diagonals when, and only
when, the existing `encoded_diagonal_id` matches exactly.

## Scope

- Reuse A3 preparation, schedule interning, evaluator construction, and
  target-level dispatcher.
- Build an encoded-diagonal pool over `encoded_diagonal_baseline.csv` records.
- Treat `encoded_diagonal_id` as the exact compatibility predicate.
- Reject a duplicate `encoded_diagonal_id` if the baseline fields or poly hash
  differ.
- On an exact pool hit, replace the consumer evaluator matrix entry with the
  owner diagonal value so the runtime view uses one exact-compatible material
  object.
- Update per-target material metrics so
  `generated_encoded_diagonal_count + shared_encoded_diagonals` equals the
  encoded diagonal baseline count.
- Preserve A0/A1/A2/A3 bootstrap output contracts: output level, output scale,
  precision threshold, and target set semantics.

## Non-Goals

- No RNS prefix/superset slicing; that belongs to A5.
- No high-level bootstrap followed by `DropLevel`; that belongs to A6.
- No sharing on same poly hash with a different `encoded_diagonal_id`.
- No CSV schema change is required for A4. Exact compatibility is already
  recorded by `encoded_diagonal_baseline.csv` and summarized through
  `material_metrics.csv`.

## Implementation Steps

1. Add `bkrLaneA4`, `bkrPlanA4`, and `bkrA4RunConfig`.
2. Add `bkrEncodedDiagonalPool` that:
   - owns the first record for each non-empty `encoded_diagonal_id`;
   - returns the owner diagonal on exact-compatible later acquires;
   - rejects same-ID records with incompatible metadata or `poly_sha256`;
   - treats different IDs as distinct owners, including different-level records.
3. Add A4 target preparation on top of A3:
   - prepare A2/A3 material using the A4 plan ID;
   - intern schedules through the A3 schedule pool;
   - intern encoded diagonals through the A4 diagonal pool;
   - replace shared evaluator matrix diagonals with owner material.
4. Add `runBootstrapKeyReuseA4`.
5. Add tests that prove:
   - A4 only shares exact `encoded_diagonal_id` matches;
   - same ID with different encoded fields is rejected;
   - same poly hash with different level/ID does not share;
   - A4 keeps `rns_slice_success = not_applicable`;
   - A4 generated plus shared diagonal counts match the baseline;
   - A4 bootstrap target results match A0 for used targets.

## Verification

Run:

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run 'TestBootstrapKeyReuseA4' -count=1 -timeout=30m
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run 'TestBootstrapKeyReuse(A0_CSVOnlyOutputContract|A1_CSVOnlyUsedTargetContract|A1_MatchesA0UsedTargets|A2|A3|A4)' -count=1 -timeout=30m
```

## Current P2 Multi-Fast Behavior

For the current clustered and sparse P2 cases, encoded diagonal IDs differ
across target levels because level and Q-prefix context differ. A4 therefore
creates distinct encoded-diagonal owners and does not claim diagonal savings for
those cases. This is the expected exact-only behavior. The unit
encoded-diagonal-pool test covers the positive exact-match sharing path.
