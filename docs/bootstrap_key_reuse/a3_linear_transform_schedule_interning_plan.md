# A3 Linear-Transform Schedule Interning Plan

## Goal

A3 is `A2 + linear-transform schedule interning`. It keeps A2's used-target
dispatcher and rotation-key pool, then interns CoeffsToSlots and SlotsToCoeffs
schedule identity when, and only when, the existing
`transform_schedule_id` matches exactly.

## Scope

- Reuse A2 key generation, rotation-key interning, evaluator construction, and
  target-level dispatcher.
- Build a schedule pool over `linear_transform_schedule_baseline.csv` records.
- Treat `transform_schedule_id` as the exact sharing predicate.
- Reject a duplicate `transform_schedule_id` if the schedule fields differ.
- Keep encoded diagonals private to each target. A3 must not share
  `encoded_diagonal_id` material or reduce `generated_encoded_diagonal_count`.
- Preserve A0/A1/A2 bootstrap output contracts: output level, output scale,
  precision threshold, and target set semantics.

## Non-Goals

- No encoded diagonal sharing; that starts at A4.
- No RNS prefix/superset slicing; that belongs to A5.
- No high-level bootstrap followed by `DropLevel`; that belongs to A6.
- No CSV schema change is required for A3. Schedule equality is already
  recorded by `linear_transform_schedule_baseline.csv`.

## Implementation Steps

1. Add `bkrLaneA3`, `bkrPlanA3`, and `bkrA3RunConfig`.
2. Generalize the A2 preparation/bootstrap helpers so A3 can reuse the A2
   rotation-key pool while writing A3 plan IDs.
3. Add `bkrLinearTransformSchedulePool`.
4. Add `runBootstrapKeyReuseA3`.
5. Add tests that prove:
   - A3 only interns schedules with identical `transform_schedule_id`;
   - same schedule ID with different schedule fields is rejected;
   - A3 keeps `shared_encoded_diagonals = 0`;
   - A3 generated encoded diagonal counts match the encoded diagonal baseline;
   - A3 bootstrap target results match A0 for used targets.

## Verification

Run:

```bash
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run 'TestBootstrapKeyReuseA3' -count=1 -timeout=30m
GOCACHE=/tmp/lattigo-gocache go test ./circuits/ckks/bootstrapping -run 'TestBootstrapKeyReuse(A0_CSVOnlyOutputContract|A1_CSVOnlyUsedTargetContract|A1_MatchesA0UsedTargets|A2|A3)' -count=1 -timeout=30m
```

## Current P2 Multi-Fast Behavior

For the current sparse P2 case, schedule IDs differ across target levels, so
A3 creates distinct schedule owners and does not claim schedule savings. This
is the expected exact-only behavior. The unit schedule-pool test covers the
positive exact-match sharing path.
