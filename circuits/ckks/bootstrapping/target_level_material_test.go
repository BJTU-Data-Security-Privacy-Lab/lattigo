package bootstrapping

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/tuneinsight/lattigo/v6/core/rlwe"
	"github.com/tuneinsight/lattigo/v6/schemes/ckks"
)

func TestTargetLevelMaterialPlanRejectsEmptyFutureTargets(t *testing.T) {
	req := targetLevelMaterialRequestForTest(t, nil, nil, false)

	_, err := NewTargetLevelMaterialPlan(req)
	if !errors.Is(err, ErrTargetLevelUnavailable) {
		t.Fatalf("NewTargetLevelMaterialPlan error=%v, want ErrTargetLevelUnavailable", err)
	}
}

func TestTargetLevelMaterialPlanRejectsInvalidFutureTargetBeforeKeygen(t *testing.T) {
	req := targetLevelMaterialRequestForTest(t, []int{99}, nil, false)

	_, err := NewTargetLevelMaterialPlan(req)
	if !errors.Is(err, ErrInvalidTargetLevel) {
		t.Fatalf("NewTargetLevelMaterialPlan error=%v, want ErrInvalidTargetLevel", err)
	}
}

func TestTargetLevelMaterialPlanRejectsOwnerThatServesNoFutureTarget(t *testing.T) {
	req := targetLevelMaterialRequestForTest(t, []int{1}, []int{0}, true)

	_, err := NewTargetLevelMaterialPlan(req)
	if !errors.Is(err, ErrTargetLevelUnavailable) {
		t.Fatalf("NewTargetLevelMaterialPlan error=%v, want ErrTargetLevelUnavailable", err)
	}
}

func TestTargetLevelMaterialPlanCanonicalizesFutureTargets(t *testing.T) {
	req := targetLevelMaterialRequestForTest(t, []int{3, 1, 3}, nil, false)

	plan, err := NewTargetLevelMaterialPlan(req)
	if err != nil {
		t.Fatal(err)
	}

	report := plan.PlanReport()
	if got, want := report.FutureTargetLevels, []int{1, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("FutureTargetLevels=%v, want %v", got, want)
	}
	if got, want := report.OwnerTargetLevels, []int{1, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("OwnerTargetLevels=%v, want %v", got, want)
	}
}

func TestTargetLevelMaterialPlanSelectsSupersetOwnerWhenAllowed(t *testing.T) {
	req := targetLevelMaterialRequestForTest(t, []int{1, 3}, nil, true)

	plan, err := NewTargetLevelMaterialPlan(req)
	if err != nil {
		t.Fatal(err)
	}

	report := plan.PlanReport()
	if got, want := report.OwnerTargetLevels, []int{3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("OwnerTargetLevels=%v, want %v", got, want)
	}
	for _, target := range report.Targets {
		if target.TargetLevel == 1 && (target.OwnerLevel != 3 || target.Strategy != "superset_output_drop") {
			t.Fatalf("target 1 report=%+v, want owner=3 strategy=superset_output_drop", target)
		}
		if target.TargetLevel == 3 && (target.OwnerLevel != 3 || target.Strategy != "exact") {
			t.Fatalf("target 3 report=%+v, want owner=3 strategy=exact", target)
		}
	}
}

func TestTargetLevelMaterialPlanRejectsSupersetOwnerWhenPolicyDisallowsIt(t *testing.T) {
	req := targetLevelMaterialRequestForTest(t, []int{1}, []int{3}, true)
	req.ReusePolicy = ReuseExactOnly

	_, err := NewTargetLevelMaterialPlan(req)
	if !errors.Is(err, ErrTargetLevelUnavailable) {
		t.Fatalf("NewTargetLevelMaterialPlan error=%v, want ErrTargetLevelUnavailable", err)
	}
}

func TestTargetLevelMaterialPlanRejectsIncompatiblePrefixOwnerWithReason(t *testing.T) {
	spec := bkrCaseA6P2MultiFastUsedSparse()
	req := targetLevelMaterialRequestFromSpecForTest(t, spec, []int{1}, []int{3}, false)
	req.ReusePolicy = ReuseExactAndPrefix
	req.EnableRNSSliceViews = true

	_, err := NewTargetLevelMaterialPlan(req)
	if !errors.Is(err, ErrTargetLevelUnavailable) {
		t.Fatalf("NewTargetLevelMaterialPlan error=%v, want ErrTargetLevelUnavailable", err)
	}
	if !strings.Contains(err.Error(), "rns_prefix_bootstrapping_") {
		t.Fatalf("NewTargetLevelMaterialPlan error=%q, want rns_prefix_bootstrapping reason", err)
	}
}

func TestTargetLevelPlanReportIncludesRejectedPrefixCandidateReasons(t *testing.T) {
	spec := bkrCaseA6P2MultiFastUsedSparse()
	req := targetLevelMaterialRequestFromSpecForTest(t, spec, []int{1, 3}, nil, true)
	req.EnableRNSSliceViews = true

	plan, err := NewTargetLevelMaterialPlan(req)
	if err != nil {
		t.Fatal(err)
	}
	reason, ok := rnsPrefixCompatibleParameters(plan.paramsByTargetLevel[3], plan.paramsByTargetLevel[1])
	if ok {
		t.Fatal("test parameters unexpectedly allow RNS prefix reuse")
	}
	wantReason := "rns_prefix_" + reason

	report := plan.PlanReport()
	if got := report.RejectedCandidateReasons[wantReason]; got == 0 {
		t.Fatalf("RejectedCandidateReasons[%q]=%d, want >0", wantReason, got)
	}
	for _, target := range report.Targets {
		if target.TargetLevel == 1 {
			if got := target.RejectedCandidateReasons[wantReason]; got == 0 {
				t.Fatalf("target RejectedCandidateReasons[%q]=%d, want >0", wantReason, got)
			}
			return
		}
	}
	t.Fatal("target 1 report not found")
}

func TestTargetLevelBootstrapperRejectsUndeclaredTarget(t *testing.T) {
	spec := bkrCaseP0TinyNativeSingle()
	bootstrapper, _ := targetLevelBootstrapperForTest(t, spec, []int{1}, nil, false)

	if _, err := bootstrapper.BootstrapAtLevel(nil, 0); !errors.Is(err, ErrTargetLevelUnavailable) {
		t.Fatalf("BootstrapAtLevel undeclared error=%v, want ErrTargetLevelUnavailable", err)
	}
}

func TestTargetLevelBootstrapperRejectsInvalidTarget(t *testing.T) {
	spec := bkrCaseP0TinyNativeSingle()
	bootstrapper, _ := targetLevelBootstrapperForTest(t, spec, []int{1}, nil, false)

	if _, err := bootstrapper.BootstrapAtLevel(nil, 99); !errors.Is(err, ErrInvalidTargetLevel) {
		t.Fatalf("BootstrapAtLevel invalid error=%v, want ErrInvalidTargetLevel", err)
	}
	if _, err := bootstrapper.BootstrapManyAtLevel(nil, 99); !errors.Is(err, ErrInvalidTargetLevel) {
		t.Fatalf("BootstrapManyAtLevel invalid error=%v, want ErrInvalidTargetLevel", err)
	}
}

func TestTargetLevelBootstrapperExactOwnerBootstrapsDeclaredTarget(t *testing.T) {
	spec := bkrCaseP0TinyNativeSingle()
	bootstrapper, sk := targetLevelBootstrapperForTest(t, spec, []int{1}, nil, false)
	params, _, err := buildTargetBootstrappingParameters(spec, 1)
	if err != nil {
		t.Fatal(err)
	}

	ct, values := targetLevelInputCiphertextForTest(t, spec, params, sk, 1)
	out, err := bootstrapper.BootstrapAtLevel(ct, 1)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := bkrValidateBootstrapKeyReuseOutputs(t, "TargetLevelMaterialPlan/v1", spec, 1, 1, params, []rlwe.Ciphertext{*out}, [][]complex128{values}); err != nil {
		t.Fatal(err)
	}
}

func TestTargetLevelBootstrapperExactOwnerBootstrapsTargetZero(t *testing.T) {
	spec := bkrCaseP0TinyNativeSingle()
	bootstrapper, sk := targetLevelBootstrapperForTest(t, spec, []int{0}, nil, false)
	params, _, err := buildTargetBootstrappingParameters(spec, 0)
	if err != nil {
		t.Fatal(err)
	}

	ct, values := targetLevelInputCiphertextForTest(t, spec, params, sk, 0)
	out, err := bootstrapper.BootstrapAtLevel(ct, 0)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := bkrValidateBootstrapKeyReuseOutputs(t, "TargetLevelMaterialPlan/v1", spec, 0, 0, params, []rlwe.Ciphertext{*out}, [][]complex128{values}); err != nil {
		t.Fatal(err)
	}
}

func TestTargetLevelBootstrapperExactOwnersCoverAllLegalFastTargets(t *testing.T) {
	if bkrRaceDetectorEnabled {
		t.Skip("all-legal target replay is covered by non-race gates; race gate keeps representative shared-material coverage under 30m")
	}
	for _, spec := range []bkrCaseSpec{
		bkrCaseP0TinyNativeSingle(),
		bkrCaseP1TinyRingSwitchSingle(),
		bkrCaseP2MultiFastClustered(),
		bkrCaseP3N15SparseClustered(),
		bkrCaseP4N15DenseClustered(),
	} {
		t.Run(spec.ProfileID, func(t *testing.T) {
			levels := legalTargetLevelsForSpecForTest(t, spec)
			bootstrapper, sk := targetLevelBootstrapperForTest(t, spec, levels, nil, false)
			for _, targetLevel := range levels {
				params, _, err := buildTargetBootstrappingParameters(spec, targetLevel)
				if err != nil {
					t.Fatal(err)
				}
				ct, values := targetLevelInputCiphertextForTest(t, spec, params, sk, targetLevel)
				out, err := bootstrapper.BootstrapAtLevel(ct, targetLevel)
				if err != nil {
					t.Fatalf("target %d: %v", targetLevel, err)
				}
				if _, err := bkrValidateBootstrapKeyReuseOutputs(t, "TargetLevelMaterialPlan/v1", spec, targetLevel, targetLevel, params, []rlwe.Ciphertext{*out}, [][]complex128{values}); err != nil {
					t.Fatalf("target %d: %v", targetLevel, err)
				}
			}
		})
	}
}

func TestTargetLevelBootstrapperExactOwnersCoverAllLegalLongTargets(t *testing.T) {
	if !*flagLongTest {
		t.Skip("long final target-level API legal-target replay; rerun with -args -long")
	}
	if bkrRaceDetectorEnabled {
		t.Skip("long all-legal target replay is covered by non-race gates")
	}
	for _, spec := range []bkrCaseSpec{
		bkrCaseP5N16SparseLongSingle(),
		bkrCaseP6N16DenseLongSingle(),
	} {
		t.Run(spec.ProfileID, func(t *testing.T) {
			levels := legalTargetLevelsForSpecForTest(t, spec)
			bootstrapper, sk := targetLevelBootstrapperForTest(t, spec, levels, nil, false)
			for _, targetLevel := range levels {
				params, _, err := buildTargetBootstrappingParameters(spec, targetLevel)
				if err != nil {
					t.Fatal(err)
				}
				ct, values := targetLevelInputCiphertextForTest(t, spec, params, sk, targetLevel)
				out, err := bootstrapper.BootstrapAtLevel(ct, targetLevel)
				if err != nil {
					t.Fatalf("target %d: %v", targetLevel, err)
				}
				if _, err := bkrValidateBootstrapKeyReuseOutputs(t, "TargetLevelMaterialPlan/v1", spec, targetLevel, targetLevel, params, []rlwe.Ciphertext{*out}, [][]complex128{values}); err != nil {
					t.Fatalf("target %d: %v", targetLevel, err)
				}
			}
		})
	}
}

func TestTargetLevelBootstrapperSupersetDropMatchesDirectTarget(t *testing.T) {
	spec := bkrCaseA6P2MultiFastUsedSparse()
	bootstrapper, sk := targetLevelBootstrapperForTest(t, spec, []int{1, 3}, nil, true)
	params, _, err := buildTargetBootstrappingParameters(spec, 1)
	if err != nil {
		t.Fatal(err)
	}

	ct, values := targetLevelInputCiphertextForTest(t, spec, params, sk, 1)
	out, err := bootstrapper.BootstrapAtLevel(ct, 1)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := bkrValidateBootstrapKeyReuseOutputs(t, "TargetLevelMaterialPlan/v1", spec, 1, 1, params, []rlwe.Ciphertext{*out}, [][]complex128{values}); err != nil {
		t.Fatal(err)
	}
}

func TestReusableEvaluationKeysExposeImmutableSharedKeyPool(t *testing.T) {
	spec := bkrCaseA6P2MultiFastUsedSparse()
	req := targetLevelMaterialRequestFromSpecForTest(t, spec, []int{1, 3}, nil, true)
	plan, err := NewTargetLevelMaterialPlan(req)
	if err != nil {
		t.Fatal(err)
	}
	sk, err := newDeterministicSecretKey(req.FullResidualParameters, spec.Seed+"/secret-key")
	if err != nil {
		t.Fatal(err)
	}

	keys, err := plan.GenReusableEvaluationKeys(sk)
	if err != nil {
		t.Fatal(err)
	}

	pool := keys.SharedKeyPool()
	if pool == nil {
		t.Fatal("SharedKeyPool() returned nil")
	}
	owners := pool.OwnerTargetLevels()
	if got, want := owners, []int{3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("OwnerTargetLevels=%v, want %v", got, want)
	}
	owners[0] = 1
	if got, want := pool.OwnerTargetLevels(), []int{3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("OwnerTargetLevels mutated through returned slice: got %v want %v", got, want)
	}

	ids := pool.MaterialIDs()
	if len(ids) == 0 {
		t.Fatal("MaterialIDs() returned no physical material records")
	}
	if ids[0].OwnerLevel != 3 || ids[0].Kind != MaterialKindEvaluationKey {
		t.Fatalf("first MaterialID=%+v, want owner=3 kind=%s", ids[0], MaterialKindEvaluationKey)
	}
}

func TestSharedKeyPoolMaterialIDsCoverBootstrappingKinds(t *testing.T) {
	standardSpec := bkrCaseP0TinyNativeSingle()
	standardBootstrapper, _ := targetLevelBootstrapperForTest(t, standardSpec, []int{1}, nil, false)
	standardKinds := materialKindsForTest(standardBootstrapper.keys.SharedKeyPool().MaterialIDs())
	for _, want := range []MaterialKind{
		MaterialKindRelinearizationKey,
		MaterialKindRotationKey,
		MaterialKindDenseSparseKey,
		MaterialKindMatrixSchedule,
		MaterialKindEncodedDiagonal,
	} {
		if !slices.Contains(standardKinds, want) {
			t.Fatalf("standard material kinds=%v, want %s", standardKinds, want)
		}
	}

	ringSwitchSpec := bkrCaseP1TinyRingSwitchSingle()
	ringSwitchBootstrapper, _ := targetLevelBootstrapperForTest(t, ringSwitchSpec, []int{1}, nil, false)
	ringSwitchKinds := materialKindsForTest(ringSwitchBootstrapper.keys.SharedKeyPool().MaterialIDs())
	if !slices.Contains(ringSwitchKinds, MaterialKindRingSwitchKey) {
		t.Fatalf("ring-switch material kinds=%v, want %s", ringSwitchKinds, MaterialKindRingSwitchKey)
	}
}

func TestSharedKeyPoolMaterialIDsSeparateSecretDomains(t *testing.T) {
	spec := bkrCaseP1TinyRingSwitchSingle()
	bootstrapper, _ := targetLevelBootstrapperForTest(t, spec, []int{1}, nil, false)

	var relinDomain, rotationDomain, n1ToN2Domain, n2ToN1Domain string
	for _, id := range bootstrapper.keys.SharedKeyPool().MaterialIDs() {
		switch {
		case id.Kind == MaterialKindRelinearizationKey:
			relinDomain = id.SecretDomain
		case id.Kind == MaterialKindRotationKey:
			rotationDomain = id.SecretDomain
		case id.Kind == MaterialKindRingSwitchKey && id.Direction == "N1ToN2":
			n1ToN2Domain = id.SecretDomain
		case id.Kind == MaterialKindRingSwitchKey && id.Direction == "N2ToN1":
			n2ToN1Domain = id.SecretDomain
		}
	}

	for name, domain := range map[string]string{
		"relinearization": relinDomain,
		"rotation":        rotationDomain,
		"N1ToN2":          n1ToN2Domain,
		"N2ToN1":          n2ToN1Domain,
	} {
		if domain == "" {
			t.Fatalf("%s secret domain is empty", name)
		}
	}
	if relinDomain == n1ToN2Domain {
		t.Fatalf("relinearization and N1ToN2 secret domains unexpectedly match: %s", relinDomain)
	}
	if n1ToN2Domain == n2ToN1Domain {
		t.Fatalf("opposite ring-switch directions share secret domain: %s", n1ToN2Domain)
	}
}

func TestMaterialIDCompatibilityRejectsRotationGalElMismatch(t *testing.T) {
	owner := MaterialID{Kind: MaterialKindRotationKey, GaloisElement: 5, SecretDomain: "sk", ParametersHash: "params", LevelQ: 3, LevelP: 0}
	consumer := owner
	consumer.GaloisElement = 7

	if reason, ok := compatibleMaterialIDs(owner, consumer); ok || reason != "gal_el_mismatch" {
		t.Fatalf("compatibleMaterialIDs reason=%q ok=%v, want gal_el_mismatch,false", reason, ok)
	}
}

func TestMaterialIDCompatibilityRejectsSecretDomainMismatch(t *testing.T) {
	owner := MaterialID{Kind: MaterialKindRingSwitchKey, SecretDomain: "sk-a", ParametersHash: "params", LevelQ: 3, LevelP: 0}
	consumer := owner
	consumer.SecretDomain = "sk-b"

	if reason, ok := compatibleMaterialIDs(owner, consumer); ok || reason != "secret_domain_mismatch" {
		t.Fatalf("compatibleMaterialIDs reason=%q ok=%v, want secret_domain_mismatch,false", reason, ok)
	}
}

func TestMaterialIDCompatibilityAcceptsExactRelinearizationKey(t *testing.T) {
	owner := MaterialID{Kind: MaterialKindRelinearizationKey, SecretDomain: "sk", ParametersHash: "params", LevelQ: 3, LevelP: 0}
	consumer := owner

	if reason, ok := compatibleMaterialIDs(owner, consumer); !ok || reason != "compatible" {
		t.Fatalf("compatibleMaterialIDs reason=%q ok=%v, want compatible,true", reason, ok)
	}
}

func TestMaterialIDCompatibilityAcceptsExactEveryMaterialKind(t *testing.T) {
	for _, owner := range []MaterialID{
		{Kind: MaterialKindRelinearizationKey, SecretDomain: "sk", ParametersHash: "params", LevelQ: 3, LevelP: 0},
		{Kind: MaterialKindRotationKey, SecretDomain: "sk", ParametersHash: "params", LevelQ: 3, LevelP: 0, GaloisElement: 5},
		{Kind: MaterialKindRingSwitchKey, SecretDomain: "sk", ParametersHash: "params", LevelQ: 3, LevelP: 0, Direction: "N1ToN2"},
		{Kind: MaterialKindDenseSparseKey, SecretDomain: "sk", ParametersHash: "params", LevelQ: 3, LevelP: 0, Direction: "DenseToSparse"},
		{Kind: MaterialKindMatrixSchedule, SecretDomain: "sk", ParametersHash: "params", LevelQ: 3, LevelP: 0, MatrixName: "C2S", TransformIndex: 1, DescriptorHash: "schedule"},
		{Kind: MaterialKindEncodedDiagonal, SecretDomain: "sk", ParametersHash: "params", LevelQ: 3, LevelP: 0, MatrixName: "S2C", TransformIndex: 1, DiagonalIndex: 4, DescriptorHash: "diagonal"},
	} {
		t.Run(string(owner.Kind), func(t *testing.T) {
			if reason, ok := compatibleMaterialIDs(owner, owner); !ok || reason != "compatible" {
				t.Fatalf("compatibleMaterialIDs reason=%q ok=%v, want compatible,true", reason, ok)
			}
		})
	}
}

func TestMaterialIDCompatibilityRejectsEveryMaterialKindMismatch(t *testing.T) {
	for _, tc := range []struct {
		name   string
		owner  MaterialID
		mutate func(*MaterialID)
		reason string
	}{
		{
			name:   "relinearization levelq",
			owner:  MaterialID{Kind: MaterialKindRelinearizationKey, SecretDomain: "sk", ParametersHash: "params", LevelQ: 3, LevelP: 0},
			mutate: func(id *MaterialID) { id.LevelQ = 4 },
			reason: "levelq_insufficient",
		},
		{
			name:   "rotation galEl",
			owner:  MaterialID{Kind: MaterialKindRotationKey, SecretDomain: "sk", ParametersHash: "params", LevelQ: 3, LevelP: 0, GaloisElement: 5},
			mutate: func(id *MaterialID) { id.GaloisElement = 7 },
			reason: "gal_el_mismatch",
		},
		{
			name:   "ring-switch secret domain",
			owner:  MaterialID{Kind: MaterialKindRingSwitchKey, SecretDomain: "sk-a", ParametersHash: "params", LevelQ: 3, LevelP: 0, Direction: "N1ToN2"},
			mutate: func(id *MaterialID) { id.SecretDomain = "sk-b" },
			reason: "secret_domain_mismatch",
		},
		{
			name:   "dense-sparse direction",
			owner:  MaterialID{Kind: MaterialKindDenseSparseKey, SecretDomain: "sk", ParametersHash: "params", LevelQ: 3, LevelP: 0, Direction: "DenseToSparse"},
			mutate: func(id *MaterialID) { id.Direction = "SparseToDense" },
			reason: "direction_mismatch",
		},
		{
			name:   "matrix schedule transform index",
			owner:  MaterialID{Kind: MaterialKindMatrixSchedule, SecretDomain: "sk", ParametersHash: "params", LevelQ: 3, LevelP: 0, MatrixName: "C2S", TransformIndex: 1},
			mutate: func(id *MaterialID) { id.TransformIndex = 2 },
			reason: "transform_index_mismatch",
		},
		{
			name:   "encoded diagonal index",
			owner:  MaterialID{Kind: MaterialKindEncodedDiagonal, SecretDomain: "sk", ParametersHash: "params", LevelQ: 3, LevelP: 0, MatrixName: "S2C", TransformIndex: 1, DiagonalIndex: 4},
			mutate: func(id *MaterialID) { id.DiagonalIndex = 5 },
			reason: "diagonal_index_mismatch",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			consumer := tc.owner
			tc.mutate(&consumer)
			if reason, ok := compatibleMaterialIDs(tc.owner, consumer); ok || reason != tc.reason {
				t.Fatalf("compatibleMaterialIDs reason=%q ok=%v, want %s,false", reason, ok, tc.reason)
			}
		})
	}
}

func TestRNSPrefixEvaluationKeysRejectMissingGaloisElement(t *testing.T) {
	spec := bkrCaseP0TinyNativeSingle()
	params, btpParams, err := buildTargetBootstrappingParameters(spec, 1)
	if err != nil {
		t.Fatal(err)
	}
	sk, err := newDeterministicSecretKey(params, spec.Seed+"/missing-galel/sk")
	if err != nil {
		t.Fatal(err)
	}
	keys, _, err := btpParams.GenEvaluationKeys(sk)
	if err != nil {
		t.Fatal(err)
	}
	rlk, err := keys.GetRelinearizationKey()
	if err != nil {
		t.Fatal(err)
	}
	ownerMissingGalois := *keys
	ownerMissingGalois.MemEvaluationKeySet = rlwe.NewMemEvaluationKeySet(rlk)

	if _, err := rnsPrefixEvaluationKeysView(&ownerMissingGalois, btpParams); err == nil || !strings.Contains(err.Error(), "GaloisKey") {
		t.Fatalf("rnsPrefixEvaluationKeysView error=%v, want missing GaloisKey rejection", err)
	}
}

func TestTargetLevelPlanReportGoldenSupersetPlan(t *testing.T) {
	spec := bkrCaseA6P2MultiFastUsedSparse()
	req := targetLevelMaterialRequestFromSpecForTest(t, spec, []int{3, 1, 3}, nil, true)
	req.EnableRNSSliceViews = true

	plan, err := NewTargetLevelMaterialPlan(req)
	if err != nil {
		t.Fatal(err)
	}
	report := plan.PlanReport()

	wantTargets := []TargetLevelPlanTargetReport{
		{
			TargetLevel:              1,
			OwnerLevel:               3,
			Strategy:                 "superset_output_drop",
			ReasonCode:               "superset_output_owner_level=3;drop_levels=2",
			GeneratedPhysicalCounts:  map[string]int{},
			LogicalViewCounts:        map[string]int{"superset_output_drop": 1},
			SharedCounts:             map[string]int{string(MaterialKindEvaluationKey): 1},
			RejectedCandidateReasons: map[string]int{"rns_prefix_bootstrapping_q_prefix_mismatch_2": 1},
		},
		{
			TargetLevel:              3,
			OwnerLevel:               3,
			Strategy:                 "exact",
			ReasonCode:               "none",
			GeneratedPhysicalCounts:  map[string]int{string(MaterialKindEvaluationKey): 1},
			LogicalViewCounts:        map[string]int{},
			SharedCounts:             map[string]int{},
			RejectedCandidateReasons: map[string]int{},
		},
	}
	if got, want := report.FutureTargetLevels, []int{1, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("FutureTargetLevels=%v, want %v", got, want)
	}
	if got, want := report.OwnerTargetLevels, []int{3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("OwnerTargetLevels=%v, want %v", got, want)
	}
	if !report.EnableRNSSliceViews {
		t.Fatal("EnableRNSSliceViews=false, want true")
	}
	if !report.AllowSupersetDrop {
		t.Fatal("AllowSupersetDrop=false, want true")
	}
	if got, want := report.GeneratedPhysicalCounts, map[string]int{string(MaterialKindEvaluationKey): 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("GeneratedPhysicalCounts=%v, want %v", got, want)
	}
	if got, want := report.LogicalViewCounts, map[string]int{"superset_output_drop": 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("LogicalViewCounts=%v, want %v", got, want)
	}
	if got, want := report.SharedCounts, map[string]int{string(MaterialKindEvaluationKey): 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SharedCounts=%v, want %v", got, want)
	}
	if got, want := report.RejectedCandidateReasons, map[string]int{"rns_prefix_bootstrapping_q_prefix_mismatch_2": 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("RejectedCandidateReasons=%v, want %v", got, want)
	}
	if !reflect.DeepEqual(report.Targets, wantTargets) {
		t.Fatalf("Targets=%+v, want %+v", report.Targets, wantTargets)
	}
}

func TestTargetLevelPlanReportAccountsSupersetSharedView(t *testing.T) {
	spec := bkrCaseA6P2MultiFastUsedSparse()
	bootstrapper, _ := targetLevelBootstrapperForTest(t, spec, []int{1, 3}, nil, true)

	report := bootstrapper.PlanReport()
	if got, want := report.GeneratedPhysicalCounts[string(MaterialKindEvaluationKey)], 1; got != want {
		t.Fatalf("generated evaluation key count=%d, want %d", got, want)
	}
	if got, want := report.SharedCounts[string(MaterialKindEvaluationKey)], 1; got != want {
		t.Fatalf("shared evaluation key count=%d, want %d", got, want)
	}
	if got, want := report.LogicalViewCounts["superset_output_drop"], 1; got != want {
		t.Fatalf("superset logical view count=%d, want %d", got, want)
	}
	for _, target := range report.Targets {
		if target.TargetLevel == 1 {
			if target.ReasonCode != "superset_output_owner_level=3;drop_levels=2" {
				t.Fatalf("target 1 reason=%q", target.ReasonCode)
			}
			if target.GeneratedPhysicalCounts[string(MaterialKindEvaluationKey)] != 0 {
				t.Fatalf("target 1 generated evaluation keys=%d, want 0", target.GeneratedPhysicalCounts[string(MaterialKindEvaluationKey)])
			}
			if target.SharedCounts[string(MaterialKindEvaluationKey)] != 1 {
				t.Fatalf("target 1 shared evaluation keys=%d, want 1", target.SharedCounts[string(MaterialKindEvaluationKey)])
			}
		}
	}
}

func TestTargetLevelPlanReportCountsGeneratedMaterialKinds(t *testing.T) {
	spec := bkrCaseP0TinyNativeSingle()
	bootstrapper, _ := targetLevelBootstrapperForTest(t, spec, []int{1}, nil, false)

	report := bootstrapper.PlanReport()
	for _, want := range []MaterialKind{
		MaterialKindRelinearizationKey,
		MaterialKindRotationKey,
		MaterialKindDenseSparseKey,
		MaterialKindMatrixSchedule,
		MaterialKindEncodedDiagonal,
	} {
		if got := report.GeneratedPhysicalCounts[string(want)]; got == 0 {
			t.Fatalf("GeneratedPhysicalCounts[%s]=0, want >0", want)
		}
	}
}

func TestSharedKeyPoolRNSSliceViewsSkipIncompatibleParameters(t *testing.T) {
	spec := bkrCaseA6P2MultiFastUsedSparse()
	req := targetLevelMaterialRequestFromSpecForTest(t, spec, []int{1, 3}, nil, true)
	req.EnableRNSSliceViews = true

	plan, err := NewTargetLevelMaterialPlan(req)
	if err != nil {
		t.Fatal(err)
	}
	sk, err := newDeterministicSecretKey(req.FullResidualParameters, spec.Seed+"/secret-key")
	if err != nil {
		t.Fatal(err)
	}
	keys, err := plan.GenReusableEvaluationKeys(sk)
	if err != nil {
		t.Fatal(err)
	}
	bootstrapper, err := NewTargetLevelBootstrapper(plan, keys)
	if err != nil {
		t.Fatal(err)
	}

	ids := bootstrapper.keys.SharedKeyPool().MaterialIDs()
	for _, id := range ids {
		if id.View == "rns_prefix" && id.OwnerLevel == 3 && id.TargetLevel == 1 {
			t.Fatalf("unexpected incompatible rns_prefix material id=%+v", id)
		}
	}

	report := bootstrapper.PlanReport()
	if got := report.LogicalViewCounts["rns_prefix"]; got != 0 {
		t.Fatalf("PlanReport LogicalViewCounts[rns_prefix]=%d, want 0", got)
	}
}

func TestRNSPrefixParameterCompatibilityRejectsMismatchedBootstrappingPrefix(t *testing.T) {
	spec := bkrCaseA6P2MultiFastUsedSparse()
	_, ownerParams, err := buildTargetBootstrappingParameters(spec, 3)
	if err != nil {
		t.Fatal(err)
	}
	_, targetParams, err := buildTargetBootstrappingParameters(spec, 1)
	if err != nil {
		t.Fatal(err)
	}

	if reason, ok := rnsPrefixCompatibleParameters(ownerParams, targetParams); ok {
		t.Fatalf("rnsPrefixCompatibleParameters reason=%q ok=%v, want incompatible", reason, ok)
	}
	if reason, ok := rnsPrefixCompatibleParameters(targetParams, targetParams); !ok {
		t.Fatalf("rnsPrefixCompatibleParameters identical params reason=%q ok=%v, want compatible", reason, ok)
	}
}

func TestRNSPrefixViewAllowsEvaluationKeyPrefix(t *testing.T) {
	params := bkrA5ValidationParameters(t)
	levelQ := 1
	levelP := params.MaxLevelP()
	highLevelQ := params.MaxLevel()
	baseTwoDecomposition := 0

	sk, err := newDeterministicSecretKey(params, bkrDefaultSeed+"/final-rns/prefix-positive/sk")
	if err != nil {
		t.Fatal(err)
	}
	kgen := rlwe.NewKeyGenerator(params)
	evk := kgen.GenEvaluationKeyNew(sk, sk, bkrA5EvaluationKeyParameters(highLevelQ, levelP, baseTwoDecomposition))

	view, err := rnsPrefixEvaluationKeyView(params, evk, levelQ, levelP)
	if err != nil {
		t.Fatal(err)
	}
	bkrAssertA5EvaluationKeyPrefixView(t, evk, view, levelQ, levelP)
}

func TestTargetLevelBootstrapperRNSSliceSkipsIncompatibleRuntimeDispatch(t *testing.T) {
	spec := bkrCaseA6P2MultiFastUsedSparse()
	req := targetLevelMaterialRequestFromSpecForTest(t, spec, []int{1, 3}, nil, true)
	req.EnableRNSSliceViews = true

	plan, err := NewTargetLevelMaterialPlan(req)
	if err != nil {
		t.Fatal(err)
	}
	sk, err := newDeterministicSecretKey(req.FullResidualParameters, spec.Seed+"/secret-key")
	if err != nil {
		t.Fatal(err)
	}
	keys, err := plan.GenReusableEvaluationKeys(sk)
	if err != nil {
		t.Fatal(err)
	}
	bootstrapper, err := NewTargetLevelBootstrapper(plan, keys)
	if err != nil {
		t.Fatal(err)
	}

	report := bootstrapper.PlanReport()
	for _, target := range report.Targets {
		if target.TargetLevel == 1 && target.Strategy != "superset_output_drop" {
			t.Fatalf("target 1 strategy=%q, want superset_output_drop", target.Strategy)
		}
	}

	params, _, err := buildTargetBootstrappingParameters(spec, 1)
	if err != nil {
		t.Fatal(err)
	}
	ct, values := targetLevelInputCiphertextForTest(t, spec, params, sk, 1)
	out, err := bootstrapper.BootstrapAtLevel(ct, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bkrValidateBootstrapKeyReuseOutputs(t, "TargetLevelMaterialPlan/v1", spec, 1, 1, params, []rlwe.Ciphertext{*out}, [][]complex128{values}); err != nil {
		t.Fatal(err)
	}
}

func TestTargetLevelBootstrapperRNSSliceDispatchRequiresPrefixEvaluator(t *testing.T) {
	fullParams, err := ckks.NewParametersFromLiteral(bkrCaseP2MultiFastClustered().SchemeParams)
	if err != nil {
		t.Fatal(err)
	}
	plan := &TargetLevelMaterialPlan{
		req:                   TargetLevelMaterialRequest{FullResidualParameters: fullParams},
		ownerByTargetLevel:    map[int]int{1: 3},
		strategyByTargetLevel: map[int]string{1: "rns_prefix"},
	}
	bootstrapper := &TargetLevelBootstrapper{
		plan:             plan,
		evaluators:       map[int]*Evaluator{3: {}},
		prefixEvaluators: map[int]*Evaluator{1: {}},
	}

	eval, ownerLevel, err := bootstrapper.evaluatorForTarget(1)
	if err != nil {
		t.Fatal(err)
	}
	if eval != bootstrapper.prefixEvaluators[1] || ownerLevel != 1 {
		t.Fatalf("evaluatorForTarget returned eval=%p owner=%d, want prefix evaluator owner=1", eval, ownerLevel)
	}

	bootstrapper.prefixEvaluators = nil
	if _, _, err := bootstrapper.evaluatorForTarget(1); !errors.Is(err, ErrMissingReusableMaterial) {
		t.Fatalf("evaluatorForTarget missing prefix error=%v, want ErrMissingReusableMaterial", err)
	}
}

func TestRNSPrefixViewRejectsOutOfRangeEvaluationKey(t *testing.T) {
	params := bkrA5ValidationParameters(t)
	levelP := params.MaxLevelP()
	levelQ := 1
	sk, err := newDeterministicSecretKey(params, bkrDefaultSeed+"/final-rns/out-of-range/sk")
	if err != nil {
		t.Fatal(err)
	}
	kgen := rlwe.NewKeyGenerator(params)
	evk := kgen.GenEvaluationKeyNew(sk, sk, bkrA5EvaluationKeyParameters(levelQ, levelP, 0))

	if err := validateRNSPrefixEvaluationKeyView(params, evk, levelQ+1, levelP); !errors.Is(err, ErrReusableViewOutOfRange) {
		t.Fatalf("validateRNSPrefixEvaluationKeyView error=%v, want ErrReusableViewOutOfRange", err)
	}
}

func TestRNSPrefixViewRejectsCompressedEvaluationKey(t *testing.T) {
	params := bkrA5ValidationParameters(t)
	levelP := params.MaxLevelP()
	levelQ := 1
	sk, err := newDeterministicSecretKey(params, bkrDefaultSeed+"/final-rns/compressed/sk")
	if err != nil {
		t.Fatal(err)
	}
	kgen := rlwe.NewKeyGenerator(params)
	evk := kgen.GenEvaluationKeyNew(sk, sk, rlwe.EvaluationKeyParameters{
		LevelQ:               &levelQ,
		LevelP:               &levelP,
		BaseTwoDecomposition: bkrPointyInt(0),
		Compressed:           true,
	})

	if err := validateRNSPrefixEvaluationKeyView(params, evk, levelQ, levelP); !errors.Is(err, ErrIncompatibleReusableMaterial) {
		t.Fatalf("validateRNSPrefixEvaluationKeyView error=%v, want ErrIncompatibleReusableMaterial", err)
	}
}

func TestSharedKeyPoolConcurrentBootstrapReadOnly(t *testing.T) {
	spec := bkrCaseP0TinyNativeSingle()
	bootstrapper, sk := targetLevelBootstrapperForTest(t, spec, []int{1}, nil, false)
	params, _, err := buildTargetBootstrappingParameters(spec, 1)
	if err != nil {
		t.Fatal(err)
	}

	ct0, _ := targetLevelInputCiphertextForTest(t, spec, params, sk, 1)
	ct1, _ := targetLevelInputCiphertextForTest(t, spec, params, sk, 1)
	inputs := []*rlwe.Ciphertext{ct0, ct1}
	errs := make(chan error, len(inputs))

	var wg sync.WaitGroup
	for _, input := range inputs {
		wg.Add(1)
		go func(ct *rlwe.Ciphertext) {
			defer wg.Done()
			out, err := bootstrapper.BootstrapAtLevel(ct, 1)
			if err != nil {
				errs <- err
				return
			}
			if out.Level() != 1 {
				errs <- errors.New("unexpected concurrent bootstrap output level")
			}
		}(input)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestTargetLevelBootstrapperManyMatchesSingleCalls(t *testing.T) {
	spec := bkrCaseP0TinyNativeSingle()
	bootstrapper, sk := targetLevelBootstrapperForTest(t, spec, []int{1}, nil, false)
	params, _, err := buildTargetBootstrappingParameters(spec, 1)
	if err != nil {
		t.Fatal(err)
	}

	ct0, values0 := targetLevelInputCiphertextForTest(t, spec, params, sk, 1)
	ct1, values1 := targetLevelInputCiphertextForTest(t, spec, params, sk, 1)
	many, err := bootstrapper.BootstrapManyAtLevel([]rlwe.Ciphertext{*ct0, *ct1}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(many) != 2 {
		t.Fatalf("BootstrapManyAtLevel returned %d ciphertexts, want 2", len(many))
	}
	if _, err := bkrValidateBootstrapKeyReuseOutputs(t, "TargetLevelMaterialPlan/v1", spec, 1, 1, params, many, [][]complex128{values0, values1}); err != nil {
		t.Fatalf("BootstrapManyAtLevel output validation failed: %v", err)
	}

	single0, valuesSingle0 := targetLevelInputCiphertextForTest(t, spec, params, sk, 1)
	out0, err := bootstrapper.BootstrapAtLevel(single0, 1)
	if err != nil {
		t.Fatal(err)
	}
	single1, valuesSingle1 := targetLevelInputCiphertextForTest(t, spec, params, sk, 1)
	out1, err := bootstrapper.BootstrapAtLevel(single1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bkrValidateBootstrapKeyReuseOutputs(t, "TargetLevelMaterialPlan/v1", spec, 1, 1, params, []rlwe.Ciphertext{*out0, *out1}, [][]complex128{valuesSingle0, valuesSingle1}); err != nil {
		t.Fatalf("repeated BootstrapAtLevel output validation failed: %v", err)
	}
}

func TestTargetLevelMaterialPlanRejectsIncompatibleRNSSliceForSupportedProfiles(t *testing.T) {
	specs := []bkrCaseSpec{
		bkrCaseP2MultiFastClustered(),
		bkrCaseP2MultiFastSparse(),
		bkrCaseP3N15SparseClustered(),
		bkrCaseP4N15DenseClustered(),
		bkrCaseP4N15DenseSparse(),
		bkrCaseP4N15DenseRandom(),
		bkrCaseP5N16SparseLongClustered(),
		bkrCaseP5N16SparseLongSparse(),
		bkrCaseP6N16DenseLongClustered(),
		bkrCaseP6N16DenseLongSparse(),
	}
	checked := 0
	for _, spec := range specs {
		for _, target := range spec.AllTargetLevels {
			for _, owner := range spec.AllTargetLevels {
				if owner <= target {
					continue
				}
				_, targetParams, err := buildTargetBootstrappingParameters(spec, target)
				if err != nil {
					t.Fatal(err)
				}
				_, ownerParams, err := buildTargetBootstrappingParameters(spec, owner)
				if err != nil {
					t.Fatal(err)
				}
				reason, ok := rnsPrefixCompatibleParameters(ownerParams, targetParams)
				if ok {
					continue
				}
				req := targetLevelMaterialRequestFromSpecForTest(t, spec, []int{target}, []int{owner}, false)
				req.ReusePolicy = ReuseExactAndPrefix
				req.EnableRNSSliceViews = true
				_, err = NewTargetLevelMaterialPlan(req)
				if !errors.Is(err, ErrTargetLevelUnavailable) {
					t.Fatalf("%s owner=%d target=%d error=%v, want ErrTargetLevelUnavailable", spec.CaseID, owner, target, err)
				}
				wantReason := "rns_prefix_" + reason
				if !strings.Contains(err.Error(), wantReason) {
					t.Fatalf("%s owner=%d target=%d error=%q, want reason %q", spec.CaseID, owner, target, err, wantReason)
				}
				checked++
			}
		}
	}
	if checked == 0 {
		t.Fatal("no incompatible supported-profile RNS slice candidates checked")
	}
}

func targetLevelMaterialRequestForTest(t *testing.T, futureTargets, ownerTargets []int, allowSupersetDrop bool) TargetLevelMaterialRequest {
	t.Helper()

	spec := bkrCaseP2MultiFastClustered()
	fullParams, err := ckks.NewParametersFromLiteral(spec.SchemeParams)
	if err != nil {
		t.Fatal(err)
	}

	btpLiteral := bkrCloneBootstrappingParametersLiteral(spec.BootstrappingParams)
	if spec.LogSlots >= 0 {
		btpLiteral.LogSlots = bkrPointyInt(spec.LogSlots)
	}

	return TargetLevelMaterialRequest{
		FullResidualParameters: fullParams,
		BootstrappingLiteral:   btpLiteral,
		FutureTargetLevels:     append([]int(nil), futureTargets...),
		OwnerTargetLevels:      append([]int(nil), ownerTargets...),
		ReusePolicy:            ReuseExactPrefixAndSupersetDrop,
		AllowSupersetDrop:      allowSupersetDrop,
	}
}

func targetLevelBootstrapperForTest(t *testing.T, spec bkrCaseSpec, futureTargets, ownerTargets []int, allowSupersetDrop bool) (*TargetLevelBootstrapper, *rlwe.SecretKey) {
	t.Helper()

	req := targetLevelMaterialRequestFromSpecForTest(t, spec, futureTargets, ownerTargets, allowSupersetDrop)
	plan, err := NewTargetLevelMaterialPlan(req)
	if err != nil {
		t.Fatal(err)
	}

	sk, err := newDeterministicSecretKey(req.FullResidualParameters, spec.Seed+"/secret-key")
	if err != nil {
		t.Fatal(err)
	}

	keys, err := plan.GenReusableEvaluationKeys(sk)
	if err != nil {
		t.Fatal(err)
	}

	bootstrapper, err := NewTargetLevelBootstrapper(plan, keys)
	if err != nil {
		t.Fatal(err)
	}

	return bootstrapper, sk
}

func targetLevelMaterialRequestFromSpecForTest(t *testing.T, spec bkrCaseSpec, futureTargets, ownerTargets []int, allowSupersetDrop bool) TargetLevelMaterialRequest {
	t.Helper()

	fullParams, err := ckks.NewParametersFromLiteral(spec.SchemeParams)
	if err != nil {
		t.Fatal(err)
	}

	btpLiteral := bkrCloneBootstrappingParametersLiteral(spec.BootstrappingParams)
	if spec.LogSlots >= 0 {
		btpLiteral.LogSlots = bkrPointyInt(spec.LogSlots)
	}

	return TargetLevelMaterialRequest{
		FullResidualParameters: fullParams,
		BootstrappingLiteral:   btpLiteral,
		FutureTargetLevels:     append([]int(nil), futureTargets...),
		OwnerTargetLevels:      append([]int(nil), ownerTargets...),
		ReusePolicy:            ReuseExactPrefixAndSupersetDrop,
		AllowSupersetDrop:      allowSupersetDrop,
	}
}

func targetLevelInputCiphertextForTest(t *testing.T, spec bkrCaseSpec, params ckks.Parameters, sk *rlwe.SecretKey, targetLevel int) (*rlwe.Ciphertext, []complex128) {
	t.Helper()

	targetSK := sk.CopyNew()
	targetSK.Value.Resize(params.MaxLevel(), params.MaxLevelP())

	encoder := ckks.NewEncoder(params)
	encryptor := rlwe.NewEncryptor(params, targetSK)
	values := bkrComplexValues(params, spec.Seed+"/target-level-bootstrapper/values")
	plaintext := ckks.NewPlaintext(params, 0)
	if err := encoder.Encode(values, plaintext); err != nil {
		t.Fatal(err)
	}

	ct, err := encryptor.EncryptNew(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if ct.Level() != 0 {
		t.Fatalf("input target %d ciphertext level=%d, want 0", targetLevel, ct.Level())
	}

	return ct, values
}

func legalTargetLevelsForSpecForTest(t *testing.T, spec bkrCaseSpec) []int {
	t.Helper()

	fullParams, err := ckks.NewParametersFromLiteral(spec.SchemeParams)
	if err != nil {
		t.Fatal(err)
	}
	levels := make([]int, 0, fullParams.MaxLevel()+1)
	for targetLevel := 0; targetLevel <= fullParams.MaxLevel(); targetLevel++ {
		if _, _, err := buildTargetBootstrappingParameters(spec, targetLevel); err == nil {
			levels = append(levels, targetLevel)
		}
	}
	if len(levels) == 0 {
		t.Fatalf("%s has no legal target levels", spec.CaseID)
	}
	return levels
}

func materialKindsForTest(ids []MaterialID) []MaterialKind {
	seen := map[MaterialKind]bool{}
	kinds := make([]MaterialKind, 0, len(ids))
	for _, id := range ids {
		if !seen[id.Kind] {
			seen[id.Kind] = true
			kinds = append(kinds, id.Kind)
		}
	}
	return kinds
}
