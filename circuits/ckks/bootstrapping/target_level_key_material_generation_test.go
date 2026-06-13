package bootstrapping

import (
	"slices"
	"testing"
)

func TestA7KeyMaterialRequirementsAreDescriptorOnly(t *testing.T) {
	req := targetLevelMaterialRequestForTest(t, []int{1}, nil, false)
	req.ReusePolicy = ReuseKeyMaterialPoolTargetEvaluator

	plan, err := NewTargetLevelMaterialPlan(req)
	if err != nil {
		t.Fatal(err)
	}

	requirements := plan.KeyMaterialRequirements()
	if len(requirements) == 0 {
		t.Fatal("KeyMaterialRequirements returned no descriptors")
	}

	kinds := map[KeyMaterialKind]bool{}
	for _, requirement := range requirements {
		if requirement.TargetLevel != 1 {
			t.Fatalf("TargetLevel=%d, want 1", requirement.TargetLevel)
		}
		if requirement.CandidateOwnerLevel != 1 {
			t.Fatalf("CandidateOwnerLevel=%d, want 1", requirement.CandidateOwnerLevel)
		}
		if requirement.BootstrapSecretDomain == "" {
			t.Fatalf("requirement %+v has empty BootstrapSecretDomain", requirement)
		}
		if !isAllowedKeyMaterialKind(requirement.Key.Kind) {
			t.Fatalf("unexpected key material kind %q", requirement.Key.Kind)
		}
		kinds[requirement.Key.Kind] = true
	}
	for _, want := range []KeyMaterialKind{KeyMaterialKindRelinearization, KeyMaterialKindRotation} {
		if !kinds[want] {
			t.Fatalf("requirements kinds=%v, want %s", kinds, want)
		}
	}
}

func TestA7GenReusableEvaluationKeysPopulatesKeyMaterialPool(t *testing.T) {
	req := targetLevelMaterialRequestForTest(t, []int{1}, nil, false)
	req.ReusePolicy = ReuseKeyMaterialPoolTargetEvaluator

	plan, err := NewTargetLevelMaterialPlan(req)
	if err != nil {
		t.Fatal(err)
	}
	sk, err := newDeterministicSecretKey(req.FullResidualParameters, bkrDefaultSeed+"/a7-key-material-pool/sk")
	if err != nil {
		t.Fatal(err)
	}

	keys, err := plan.GenReusableEvaluationKeys(sk)
	if err != nil {
		t.Fatal(err)
	}

	pool := keys.KeyMaterialPool()
	if pool == nil {
		t.Fatal("KeyMaterialPool returned nil")
	}
	if got := pool.PhysicalMaterialCount(); got == 0 {
		t.Fatal("PhysicalMaterialCount=0, want key material")
	}
	manifest, ok := pool.ManifestForTarget(1)
	if !ok {
		t.Fatal("ManifestForTarget returned ok=false")
	}
	if manifest.BootstrapSecretDomain == "" {
		t.Fatalf("manifest %+v has empty BootstrapSecretDomain", manifest)
	}
	if len(manifest.Shared)+len(manifest.Private) == 0 {
		t.Fatalf("manifest %+v has no key material", manifest)
	}

	kinds := []KeyMaterialKind{}
	for _, kind := range []KeyMaterialKind{
		KeyMaterialKindRelinearization,
		KeyMaterialKindRotation,
		KeyMaterialKindRingSwitch,
		KeyMaterialKindDenseSparse,
	} {
		if pool.CountByKind(kind) > 0 {
			kinds = append(kinds, kind)
		}
	}
	for _, want := range []KeyMaterialKind{KeyMaterialKindRelinearization, KeyMaterialKindRotation} {
		if !slices.Contains(kinds, want) {
			t.Fatalf("pool kinds=%v, want %s", kinds, want)
		}
	}
}
