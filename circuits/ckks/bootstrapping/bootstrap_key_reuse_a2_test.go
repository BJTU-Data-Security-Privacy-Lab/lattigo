package bootstrapping

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tuneinsight/lattigo/v6/core/rlwe"
	"github.com/tuneinsight/lattigo/v6/ring"
)

func runBootstrapKeyReuseA2(t *testing.T, spec bkrCaseSpec) error {
	t.Helper()

	if spec.LongOnly && !*flagLongTest {
		t.Skip("long A2 bootstrap key reuse case; rerun with -args -long")
	}

	rec, err := newBootstrapKeyReuseRecorder(t, spec, bkrA2RunConfig(spec))
	if err != nil {
		return err
	}
	defer rec.close()

	var runErr error
	successfulTargets := 0

	if err := rec.writeExperimentCase(); err != nil {
		runErr = errors.Join(runErr, err)
	}

	if err := bkrValidateA1TargetSelection(spec); err != nil {
		_ = rec.writeFailure(-1, "target_selection", err.Error(), true, false, false)
		runErr = errors.Join(runErr, err)
		summary, summaryErr := rec.writeSummary(successfulTargets)
		if summaryErr != nil {
			runErr = errors.Join(runErr, summaryErr)
		}
		return fmt.Errorf("A2 case %s failed; result_dir=%s: %w", spec.CaseID, summary.ResultDirectory, runErr)
	}

	pool := newBKRRotationKeyPool(bkrPlanA2, spec)
	prepared := make(map[int]*bkrA1PreparedTarget, len(spec.UsedTargetLevels))
	evaluators := make(map[int]*Evaluator, len(spec.UsedTargetLevels))

	for _, targetLevel := range spec.UsedTargetLevels {
		target, err := bkrPrepareA2Target(t, rec, spec, targetLevel, pool)
		if err != nil {
			runErr = errors.Join(runErr, err)
			continue
		}
		prepared[targetLevel] = target
		evaluators[targetLevel] = target.eval
	}

	rec.addRotationKeyPoolRecords(pool.records())

	if len(evaluators) > 0 {
		dispatcher, err := newTargetLevelBootstrapper(evaluators)
		if err != nil {
			_ = rec.writeFailure(-1, "dispatcher_construction", err.Error(), false, true, false)
			runErr = errors.Join(runErr, err)
		} else {
			for _, targetLevel := range spec.UsedTargetLevels {
				target := prepared[targetLevel]
				if target == nil {
					continue
				}
				if err := bkrRunA2BootstrapTarget(t, rec, spec, dispatcher, target); err != nil {
					runErr = errors.Join(runErr, err)
					continue
				}
				successfulTargets++
			}
		}
	}

	summary, summaryErr := rec.writeSummary(successfulTargets)
	if summaryErr != nil {
		runErr = errors.Join(runErr, summaryErr)
	}

	if runErr != nil {
		return fmt.Errorf("A2 case %s failed; result_dir=%s: %w", spec.CaseID, summary.ResultDirectory, runErr)
	}

	return nil
}

func bkrPrepareA2Target(t *testing.T, rec *bkrRecorder, spec bkrCaseSpec, targetLevel int, pool *bkrRotationKeyPool) (*bkrA1PreparedTarget, error) {
	t.Helper()

	expectedOutputLevel := targetLevel
	if spec.ExpectedOutputLevelOverride != nil {
		expectedOutputLevel = *spec.ExpectedOutputLevelOverride
	}

	residualParams, btpParams, err := buildTargetBootstrappingParameters(spec, targetLevel)
	if err != nil {
		_ = rec.writeFailure(targetLevel, "build_parameters", err.Error(), true, false, false)
		return nil, err
	}

	if have := btpParams.ResidualParameters.MaxLevel(); have != expectedOutputLevel {
		err := fmt.Errorf("pre-keygen output level check failed: residual max level=%d expected=%d", have, expectedOutputLevel)
		_ = rec.writeFailure(targetLevel, "output_level_pre_keygen", err.Error(), true, false, false)
		return nil, err
	}

	sk, err := newDeterministicSecretKey(residualParams, spec.Seed+"/secret-key")
	if err != nil {
		_ = rec.writeFailure(targetLevel, "secret_key", err.Error(), true, false, false)
		return nil, err
	}

	var keys *EvaluationKeys
	var rotationViewRecords []bkrRotationKeyViewRecord
	keygenElapsed, keygenPeak, err := bkrMeasurePhase(func() error {
		var keygenErr error
		keys, rotationViewRecords, keygenErr = bkrGenA2EvaluationKeysWithRotationPool(spec, targetLevel, btpParams, sk, pool)
		return keygenErr
	})
	if err != nil {
		_ = rec.writeFailure(targetLevel, "keygen", err.Error(), false, false, false)
		return nil, err
	}

	constructionStart := time.Now()
	eval, err := NewEvaluator(btpParams, keys)
	constructionElapsed := time.Since(constructionStart)
	if err != nil {
		_ = rec.writeFailure(targetLevel, "evaluator_construction", err.Error(), false, true, false)
		return nil, err
	}

	if have := eval.OutputLevel(); have != expectedOutputLevel {
		err := fmt.Errorf("evaluator OutputLevel=%d expected=%d", have, expectedOutputLevel)
		_ = rec.writeFailure(targetLevel, "output_level_after_evaluator", err.Error(), false, true, false)
		return nil, err
	}

	baseline, err := collectBootstrapKeyReuseBaselineDetails(bkrPlanA2, spec, targetLevel, btpParams, keys, eval)
	if err != nil {
		_ = rec.writeFailure(targetLevel, "baseline_details", err.Error(), false, true, false)
		return nil, err
	}

	material := collectBootstrapKeyReuseMetrics(bkrPlanA2, spec, targetLevel, keys, eval, baseline)
	shared, fallbackReason := bkrRotationViewStats(rotationViewRecords)
	owned := bkrOwnedRotationKeys(rotationViewRecords, targetLevel)
	material.GeneratedRotationKeyCount = len(owned)
	material.GeneratedEvaluationKeyCount = bkrEvaluationKeyCount(keys) - bkrRotationKeyCount(keys) + len(owned)
	material.PersistentKeyBytes = bkrPersistentKeyBytesWithOwnedRotations(keys, owned)
	material.SharedRotationKeys = shared
	material.FallbackReason = fallbackReason
	completeBootstrapKeyReuseBaselineIndex(&baseline, material)

	return &bkrA1PreparedTarget{
		targetLevel:         targetLevel,
		expectedOutputLevel: expectedOutputLevel,
		residualParams:      residualParams,
		btpParams:           btpParams,
		sk:                  sk,
		keys:                keys,
		eval:                eval,
		baseline:            baseline,
		material:            material,
		rotationViewRecords: rotationViewRecords,
		keygenElapsed:       keygenElapsed,
		keygenPeak:          keygenPeak,
		constructionElapsed: constructionElapsed,
	}, nil
}

func bkrGenA2EvaluationKeysWithRotationPool(spec bkrCaseSpec, targetLevel int, p Parameters, skN1 *rlwe.SecretKey, pool *bkrRotationKeyPool) (*EvaluationKeys, []bkrRotationKeyViewRecord, error) {
	if pool == nil {
		return nil, nil, errors.New("rotation-key pool is nil")
	}

	var EvkN1ToN2, EvkN2ToN1 *rlwe.EvaluationKey
	var EvkRealToCmplx *rlwe.EvaluationKey
	var EvkCmplxToReal *rlwe.EvaluationKey

	paramsN2 := p.BootstrappingParameters
	kgen := rlwe.NewKeyGenerator(paramsN2)

	var skN2 *rlwe.SecretKey
	if p.ResidualParameters.N() != paramsN2.N() {
		skN2 = kgen.GenSecretKeyNew()

		if p.ResidualParameters.RingType() == ring.ConjugateInvariant {
			EvkCmplxToReal, EvkRealToCmplx = kgen.GenEvaluationKeysForRingSwapNew(skN2, skN1)
		} else {
			EvkN1ToN2 = kgen.GenEvaluationKeyNew(skN1, skN2)
			EvkN2ToN1 = kgen.GenEvaluationKeyNew(skN2, skN1)
		}
	} else {
		ringQ := paramsN2.RingQ()
		ringP := paramsN2.RingP()

		skN2 = rlwe.NewSecretKey(paramsN2)
		buff := ringQ.NewPoly()

		rlwe.ExtendBasisSmallNormAndCenterNTTMontgomery(ringQ, ringQ, skN1.Value.Q, buff, skN2.Value.Q)
		rlwe.ExtendBasisSmallNormAndCenterNTTMontgomery(ringQ, ringP, skN1.Value.Q, buff, skN2.Value.P)
	}

	EvkDenseToSparse, EvkSparseToDense := p.genEncapsulationEvaluationKeysNew(skN2)
	rlk := kgen.GenRelinearizationKeyNew(skN2)

	galEls := append([]uint64(nil), p.GaloisElements(paramsN2)...)
	galEls = append(galEls, paramsN2.GaloisElementForComplexConjugation())
	galEls = bkrSortedUniqueUint64(galEls)

	gks := make([]*rlwe.GaloisKey, 0, len(galEls))
	viewRecords := make([]bkrRotationKeyViewRecord, 0, len(galEls))
	domain := bkrDefaultA2RotationKeyDomain(spec, targetLevel, p)
	for _, galEl := range galEls {
		key, view, _, err := pool.acquire(domain, galEl, targetLevel, func() *rlwe.GaloisKey {
			return kgen.GenGaloisKeyNew(galEl, skN2)
		})
		if err != nil {
			return nil, nil, err
		}
		gks = append(gks, key)
		viewRecords = append(viewRecords, view)
	}

	return &EvaluationKeys{
		EvkN1ToN2:           EvkN1ToN2,
		EvkN2ToN1:           EvkN2ToN1,
		EvkRealToCmplx:      EvkRealToCmplx,
		EvkCmplxToReal:      EvkCmplxToReal,
		MemEvaluationKeySet: rlwe.NewMemEvaluationKeySet(rlk, gks...),
		EvkDenseToSparse:    EvkDenseToSparse,
		EvkSparseToDense:    EvkSparseToDense,
	}, viewRecords, nil
}

func bkrDefaultA2RotationKeyDomain(spec bkrCaseSpec, targetLevel int, btpParams Parameters) bkrRotationKeyDomain {
	params := btpParams.BootstrappingParameters.GetRLWEParameters()
	domain := bkrRotationKeyDomainFromParameters(
		spec,
		btpParams,
		params.RingQ().NthRoot(),
		params.MaxLevelQ(),
		params.MaxLevelP(),
		0,
		false,
	)
	if btpParams.ResidualParameters.N() != btpParams.BootstrappingParameters.N() {
		domain.DomainID = bkrHashStrings("rotation-key-domain-a2/v1", domain.DomainID, "ephemeral-sk-target", strconv.Itoa(targetLevel))
	}
	return domain
}

func bkrRunA2BootstrapTarget(t *testing.T, rec *bkrRecorder, spec bkrCaseSpec, dispatcher *targetLevelBootstrapper, target *bkrA1PreparedTarget) error {
	t.Helper()

	runtimeSampler := newBKRHeapSampler()
	bootstrapStart := time.Now()
	outputs, wants, bootstrapErr := bkrBootstrapA1Ciphertexts(spec, target.residualParams, dispatcher, target.sk, target.targetLevel)
	bootstrapElapsed := time.Since(bootstrapStart)
	runtimePeak := runtimeSampler.stopAndMax()

	runtimeMetrics := bkrRuntimeMetrics{
		RecordType:                    "RuntimeMetrics",
		SchemaVersion:                 bkrSchemaVersion,
		PlanID:                        bkrPlanA2,
		CaseID:                        spec.CaseID,
		ParamsProfile:                 spec.ProfileID,
		TargetLevel:                   target.targetLevel,
		KeygenTimeMS:                  bkrDurationMS(target.keygenElapsed),
		EvaluatorMatrixConstructionMS: bkrDurationMS(target.constructionElapsed),
		BootstrapLatencyMS:            bkrDurationMS(bootstrapElapsed),
		PeakKeygenHeapBytes:           target.keygenPeak,
		PeakRuntimeHeapBytes:          runtimePeak,
	}
	rec.addRotationKeyViewRecords(target.rotationViewRecords)

	if bootstrapErr != nil {
		result := bkrEmptyTargetRunResult(bkrPlanA2, spec, target.targetLevel, target.expectedOutputLevel, target.residualParams, bootstrapErr.Error())
		if err := writeBootstrapKeyReuseResult(rec, result, target.material, runtimeMetrics, target.baseline); err != nil {
			return err
		}
		_ = rec.writeFailure(target.targetLevel, "bootstrap", bootstrapErr.Error(), false, true, true)
		return bootstrapErr
	}

	result, checkErr := bkrValidateBootstrapKeyReuseOutputs(t, bkrPlanA2, spec, target.targetLevel, target.expectedOutputLevel, target.residualParams, outputs, wants)
	if err := writeBootstrapKeyReuseResult(rec, result, target.material, runtimeMetrics, target.baseline); err != nil {
		return err
	}
	if checkErr != nil {
		_ = rec.writeFailure(target.targetLevel, "validate_output", checkErr.Error(), false, true, true)
		return checkErr
	}

	return nil
}

func TestBootstrapKeyReuseA2_P2MultiFastRotationInterningContract(t *testing.T) {
	runDir := t.TempDir()
	bkrSetResultDirForTest(t, runDir)

	spec := bkrCaseA2P2MultiFastUsedSparse()
	if err := runBootstrapKeyReuseA2(t, spec); err != nil {
		t.Fatal(err)
	}

	bkrAssertCSVOnlyOutput(t, runDir)
	bkrAssertCSVTargetRowsForPlanForTest(t, runDir, spec.UsedTargetLevels, bkrLaneA2, bkrPlanA2)

	summaryRows := bkrReadCSVForTest(t, filepath.Join(runDir, "summary.csv"))
	if got := bkrCSVValueForTest(t, summaryRows, 1, "passed"); got != "true" {
		t.Fatalf("summary passed=%q, want true", got)
	}
	if got := bkrCSVValueForTest(t, summaryRows, 1, "fallback_reason"); got != "rotation_domain_mismatch" {
		t.Fatalf("summary fallback_reason=%q, want rotation_domain_mismatch", got)
	}
	if got := bkrCSVIntForTest(t, summaryRows, 1, "shared_rotation_keys"); got != 0 {
		t.Fatalf("summary shared_rotation_keys=%d, want 0 for exact-domain mismatch fallback", got)
	}

	galoisRows := bkrReadCSVForTest(t, filepath.Join(runDir, "galois_key_baseline.csv"))
	materialRows := bkrReadCSVForTest(t, filepath.Join(runDir, "material_metrics.csv"))
	indexRows := bkrReadCSVForTest(t, filepath.Join(runDir, "material_baseline_index.csv"))
	viewRows := bkrReadCSVForTest(t, filepath.Join(runDir, "rotation_key_view.csv"))
	poolRows := bkrReadCSVForTest(t, filepath.Join(runDir, "rotation_key_pool.csv"))

	baselineTotalGenerated := 0
	for row := 1; row < len(galoisRows); row++ {
		baselineTotalGenerated += bkrCSVIntForTest(t, galoisRows, row, "generated_count")
	}

	unionCount, overlapCount := bkrGaloisUnionOverlapForTest(t, galoisRows)
	if unionCount == 0 {
		t.Fatal("galois_key_baseline.csv produced empty union")
	}
	if overlapCount == 0 {
		t.Fatal("galois_key_baseline.csv has no canonical gal_el overlap to evaluate")
	}

	physicalPoolCount := len(poolRows) - 1
	if physicalPoolCount <= 0 {
		t.Fatal("rotation_key_pool.csv has no physical pool rows")
	}
	if physicalPoolCount != baselineTotalGenerated {
		t.Fatalf("physical pool rows=%d, want baseline generated rotations=%d when every overlap falls back on domain mismatch", physicalPoolCount, baselineTotalGenerated)
	}

	sharedViewCount := 0
	domainMismatchViewCount := 0
	for row := 1; row < len(viewRows); row++ {
		galEl := bkrCSVValueForTest(t, viewRows, row, "galois_element")
		if got, want := bkrCSVValueForTest(t, viewRows, row, "canonical_rotation_id"), "gal:"+galEl; got != want {
			t.Fatalf("rotation view row %d canonical_rotation_id=%q, want %q", row, got, want)
		}
		fallbackReason := bkrCSVValueForTest(t, viewRows, row, "fallback_reason")
		switch compatible := bkrCSVValueForTest(t, viewRows, row, "domain_compatible"); {
		case compatible == "true" && fallbackReason == "none":
		case compatible == "false" && fallbackReason == "rotation_domain_mismatch":
			domainMismatchViewCount++
		default:
			t.Fatalf("rotation view row %d domain_compatible=%q fallback_reason=%q, want exact view or domain-mismatch fallback", row, compatible, fallbackReason)
		}
		if bkrCSVValueForTest(t, viewRows, row, "is_shared") == "true" {
			sharedViewCount++
		}
	}
	if sharedViewCount != 0 {
		t.Fatalf("rotation_key_view.csv shared views=%d, want 0 for domain-mismatch fallback", sharedViewCount)
	}
	sharingRate := float64(sharedViewCount) / float64(overlapCount)
	if sharingRate != 0 {
		t.Fatalf("A2 sharing rate=%f, want 0 for exact-domain mismatch fallback", sharingRate)
	}
	if domainMismatchViewCount == 0 {
		t.Fatal("rotation_key_view.csv has no domain-mismatch fallback views")
	}

	pooledConsumers := 0
	for row := 1; row < len(poolRows); row++ {
		consumerLevels := strings.Split(bkrCSVValueForTest(t, poolRows, row, "consumer_target_levels"), ";")
		if len(consumerLevels) > 1 {
			pooledConsumers++
		}
		galEl := bkrCSVValueForTest(t, poolRows, row, "galois_element")
		if got, want := bkrCSVValueForTest(t, poolRows, row, "canonical_rotation_id"), "gal:"+galEl; got != want {
			t.Fatalf("rotation pool row %d canonical_rotation_id=%q, want %q", row, got, want)
		}
		if got := bkrCSVValueForTest(t, poolRows, row, "physical_key_generated"); got != "true" {
			t.Fatalf("rotation pool row %d physical_key_generated=%q, want true", row, got)
		}
	}
	if pooledConsumers != 0 {
		t.Fatalf("rotation_key_pool.csv multi-target consumers=%d, want 0 for domain-mismatch fallback", pooledConsumers)
	}

	for row := 1; row < len(materialRows); row++ {
		targetLevel := bkrCSVIntForTest(t, materialRows, row, "target_level")
		owned := bkrCSVIntForTest(t, materialRows, row, "generated_rotation_key_count")
		shared := bkrCSVIntForTest(t, materialRows, row, "shared_rotation_keys")
		baseline := bkrCSVIntForTargetForTest(t, galoisRows, targetLevel, "generated_count")
		if owned+shared != baseline {
			t.Fatalf("target %d owned+shared rotation count=%d, want baseline=%d", targetLevel, owned+shared, baseline)
		}
		if got := bkrCSVValueForTargetForTest(t, indexRows, targetLevel, "rotation_count_matches_metrics"); got != "true" {
			t.Fatalf("target %d rotation_count_matches_metrics=%q, want true", targetLevel, got)
		}
	}
}

func TestBootstrapKeyReuseA2RotationPoolSharesExactDomain(t *testing.T) {
	spec := bkrCaseA2P2MultiFastUsedSparse()
	targetLevel := spec.UsedTargetLevels[0]
	residualParams, btpParams, err := buildTargetBootstrappingParameters(spec, targetLevel)
	if err != nil {
		t.Fatal(err)
	}
	sk, err := newDeterministicSecretKey(residualParams, spec.Seed+"/secret-key")
	if err != nil {
		t.Fatal(err)
	}
	keys, _, err := btpParams.GenEvaluationKeys(sk)
	if err != nil {
		t.Fatal(err)
	}

	galEl := bkrSortedGaloisElements(keys)[0]
	gk, err := keys.GetGaloisKey(galEl)
	if err != nil {
		t.Fatal(err)
	}
	domain := bkrRotationKeyDomainFromGaloisKey(spec, btpParams, gk)
	pool := newBKRRotationKeyPool(bkrPlanA2, spec)

	ownedKey, ownedView, generated, err := pool.acquire(domain, galEl, 1, func() *rlwe.GaloisKey {
		return gk
	})
	if err != nil {
		t.Fatal(err)
	}
	if !generated {
		t.Fatal("first exact-domain acquire did not generate the physical key")
	}
	if ownedView.IsShared {
		t.Fatal("owner view marked as shared")
	}

	sharedKey, sharedView, generated, err := pool.acquire(domain, galEl, 3, func() *rlwe.GaloisKey {
		t.Fatal("exact-domain pool hit called generate")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if generated {
		t.Fatal("exact-domain pool hit generated a duplicate key")
	}
	if sharedKey != ownedKey {
		t.Fatal("exact-domain pool hit did not return the owner key pointer")
	}
	if !sharedView.IsShared || !sharedView.DomainCompatible || sharedView.FallbackReason != "none" {
		t.Fatalf("shared view=%+v, want shared exact-compatible view without fallback", sharedView)
	}

	records := pool.records()
	if len(records) != 1 {
		t.Fatalf("pool records=%d, want one physical key", len(records))
	}
	if got, want := records[0].ConsumerTargetLevels, []int{1, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("consumer target levels=%v, want %v", got, want)
	}
}

func TestBootstrapKeyReuseA2_MatchesA0UsedTargets(t *testing.T) {
	a0Dir := t.TempDir()
	bkrSetResultDirForTest(t, a0Dir)
	if err := runBootstrapKeyReuseA0(t, bkrCaseP2MultiFastClustered()); err != nil {
		t.Fatal(err)
	}

	a2Dir := t.TempDir()
	bkrSetResultDirForTest(t, a2Dir)
	a2Spec := bkrCaseA2P2MultiFastUsedSparse()
	if err := runBootstrapKeyReuseA2(t, a2Spec); err != nil {
		t.Fatal(err)
	}

	a0Targets := bkrReadCSVForTest(t, filepath.Join(a0Dir, "target_results.csv"))
	a2Targets := bkrReadCSVForTest(t, filepath.Join(a2Dir, "target_results.csv"))

	for _, targetLevel := range []int{1, 3} {
		for _, column := range []string{
			"output_level",
			"expected_output_level",
			"output_level_equality",
			"output_scale_equality",
			"bootstrap_error_status",
			"packed_ciphertexts",
		} {
			got := bkrCSVValueForTargetForTest(t, a2Targets, targetLevel, column)
			want := bkrCSVValueForTargetForTest(t, a0Targets, targetLevel, column)
			if got != want {
				t.Fatalf("target %d %s: A2=%q A0=%q", targetLevel, column, got, want)
			}
		}
	}
}

func bkrAssertCSVTargetRowsForPlanForTest(t *testing.T, dir string, want []int, lane, planID string) {
	t.Helper()

	wantSet := bkrSortedUniqueInts(want)
	for _, file := range []string{
		"target_results.csv",
		"material_metrics.csv",
		"runtime_metrics.csv",
		"parameter_chain_baseline.csv",
		"galois_key_baseline.csv",
		"linear_transform_schedule_baseline.csv",
		"encoded_diagonal_baseline.csv",
		"material_baseline_index.csv",
		"rotation_key_view.csv",
	} {
		rows := bkrReadCSVForTest(t, filepath.Join(dir, file))
		got := bkrCSVTargetLevelsForTest(t, rows)
		if !reflect.DeepEqual(got, wantSet) {
			t.Fatalf("%s target rows=%v, want %v", file, got, wantSet)
		}
	}

	summaryRows := bkrReadCSVForTest(t, filepath.Join(dir, "summary.csv"))
	if got, want := bkrCSVValueForTest(t, summaryRows, 1, "target_levels"), bkrJoinInts(want); got != want {
		t.Fatalf("summary target_levels=%q, want %q", got, want)
	}
	if got, want := bkrCSVIntForTest(t, summaryRows, 1, "total_targets"), len(want); got != want {
		t.Fatalf("summary total_targets=%d, want %d", got, want)
	}
	if got, want := bkrCSVIntForTest(t, summaryRows, 1, "successful_targets"), len(want); got != want {
		t.Fatalf("summary successful_targets=%d, want %d", got, want)
	}
	if got := bkrCSVValueForTest(t, summaryRows, 1, "lane"); got != lane {
		t.Fatalf("summary lane=%q, want %q", got, lane)
	}
	if got := bkrCSVValueForTest(t, summaryRows, 1, "plan_id"); got != planID {
		t.Fatalf("summary plan_id=%q, want %q", got, planID)
	}

	failuresRows := bkrReadCSVForTest(t, filepath.Join(dir, "failures.csv"))
	if len(failuresRows) != 1 {
		t.Fatalf("failures.csv rows=%d, want header only", len(failuresRows))
	}
}

func bkrCSVIntForTargetForTest(t *testing.T, rows [][]string, targetLevel int, column string) int {
	t.Helper()
	value, err := strconv.Atoi(bkrCSVValueForTargetForTest(t, rows, targetLevel, column))
	if err != nil {
		t.Fatalf("target %d column %q is not an int: %v", targetLevel, column, err)
	}
	return value
}

func bkrGaloisUnionOverlapForTest(t *testing.T, rows [][]string) (unionCount, overlapCount int) {
	t.Helper()
	counts := map[uint64]int{}
	for row := 1; row < len(rows); row++ {
		for _, galEl := range bkrCSVUint64ListForTest(t, rows, row, "generated_galois_elements_sorted") {
			counts[galEl]++
		}
	}
	for _, count := range counts {
		unionCount++
		if count > 1 {
			overlapCount++
		}
	}
	return unionCount, overlapCount
}

func bkrCSVUint64ListForTest(t *testing.T, rows [][]string, row int, column string) []uint64 {
	t.Helper()
	value := bkrCSVValueForTest(t, rows, row, column)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ";")
	out := make([]uint64, 0, len(parts))
	for _, part := range parts {
		parsed, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			t.Fatalf("cannot parse CSV column %q value %q as uint64: %v", column, part, err)
		}
		out = append(out, parsed)
	}
	return out
}

func bkrCaseA2P2MultiFastUsedSparse() bkrCaseSpec {
	return bkrP2MultiFastBase("a2_p2_multi_fast_used_sparse", []int{1, 2, 3}, func(spec *bkrCaseSpec) {
		spec.UsedTargetLevels = []int{1, 3}
	})
}
