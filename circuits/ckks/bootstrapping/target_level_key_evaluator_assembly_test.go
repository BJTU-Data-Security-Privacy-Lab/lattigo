package bootstrapping

import (
	"errors"
	"testing"
)

func TestBuildTargetEvaluationKeysUsesManifestKeys(t *testing.T) {
	plan, pool := a7KeyMaterialPlanAndPoolForTest(t, 1)

	keys, err := plan.BuildTargetEvaluationKeys(pool, 1)
	if err != nil {
		t.Fatal(err)
	}
	if keys.MemEvaluationKeySet == nil {
		t.Fatal("MemEvaluationKeySet is nil")
	}
	if _, err := keys.GetRelinearizationKey(); err != nil {
		t.Fatalf("GetRelinearizationKey error=%v", err)
	}
	requiredGalEls := plan.paramsByTargetLevel[1].GaloisElements(plan.paramsByTargetLevel[1].BootstrappingParameters)
	for _, galEl := range requiredGalEls {
		if _, err := keys.GetGaloisKey(galEl); err != nil {
			t.Fatalf("GetGaloisKey(%d) error=%v", galEl, err)
		}
	}
}

func TestBuildTargetEvaluationKeysRejectsMissingKeyMaterial(t *testing.T) {
	plan, pool := a7KeyMaterialPlanAndPoolForTest(t, 1)
	manifest, ok := pool.ManifestForTarget(1)
	if !ok {
		t.Fatal("ManifestForTarget returned ok=false")
	}
	var missing KeyMaterialKey
	if len(manifest.Shared) > 0 {
		missing = manifest.Shared[0]
	} else {
		missing = manifest.Private[0]
	}
	delete(pool.objects, missing)

	_, err := plan.BuildTargetEvaluationKeys(pool, 1)
	if !errors.Is(err, ErrMissingReusableMaterial) {
		t.Fatalf("BuildTargetEvaluationKeys error=%v, want ErrMissingReusableMaterial", err)
	}
}

func TestBuildTargetEvaluationKeysRejectsMixedBootstrapSecretDomain(t *testing.T) {
	plan, pool := a7KeyMaterialPlanAndPoolForTest(t, 1)
	manifest, ok := pool.ManifestForTarget(1)
	if !ok {
		t.Fatal("ManifestForTarget returned ok=false")
	}
	if len(manifest.Private) == 0 {
		t.Fatal("manifest has no private key material")
	}

	key := manifest.Private[0]
	object, ok := pool.objects[key]
	if !ok {
		t.Fatalf("missing private key material %+v", key)
	}
	delete(pool.objects, key)
	key.BootstrapSecretDomain = "other-bootstrap-secret-domain"
	object.Key = key
	pool.objects[key] = object
	manifest.Private[0] = key
	pool.manifests[manifest.TargetLevel] = manifest

	_, err := plan.BuildTargetEvaluationKeys(pool, 1)
	if !errors.Is(err, ErrIncompatibleReusableMaterial) {
		t.Fatalf("BuildTargetEvaluationKeys error=%v, want ErrIncompatibleReusableMaterial", err)
	}
}

func TestBuildTargetEvaluationKeysRejectsRNSPrefixView(t *testing.T) {
	plan, pool := a7KeyMaterialPlanAndPoolForTest(t, 1)
	manifest, ok := pool.ManifestForTarget(1)
	if !ok {
		t.Fatal("ManifestForTarget returned ok=false")
	}
	manifestKeys := append(append([]KeyMaterialKey(nil), manifest.Shared...), manifest.Private...)
	if len(manifestKeys) == 0 {
		t.Fatal("manifest has no key material")
	}

	key := manifestKeys[0]
	object, ok := pool.objects[key]
	if !ok {
		t.Fatalf("missing key material %+v", key)
	}
	object.View = KeyMaterialViewRNSPrefix
	pool.objects[key] = object

	_, err := plan.BuildTargetEvaluationKeys(pool, 1)
	if !errors.Is(err, ErrIncompatibleReusableMaterial) {
		t.Fatalf("BuildTargetEvaluationKeys error=%v, want ErrIncompatibleReusableMaterial", err)
	}
}

func TestBuildTargetEvaluatorUsesDedicatedTargetEvaluator(t *testing.T) {
	plan, pool := a7KeyMaterialPlanAndPoolForTest(t, 1)

	eval, err := plan.BuildTargetEvaluator(pool, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := eval.OutputLevel(), 1; got != want {
		t.Fatalf("OutputLevel=%d, want %d", got, want)
	}
}

func a7KeyMaterialPlanAndPoolForTest(t *testing.T, targetLevel int) (*TargetLevelMaterialPlan, *KeyMaterialPool) {
	t.Helper()

	req := targetLevelMaterialRequestFromSpecForTest(t, bkrCaseP0TinyNativeSingle(), []int{targetLevel}, nil, false)
	req.ReusePolicy = ReuseKeyMaterialPoolTargetEvaluator
	plan, err := NewTargetLevelMaterialPlan(req)
	if err != nil {
		t.Fatal(err)
	}
	sk, err := newDeterministicSecretKey(req.FullResidualParameters, bkrDefaultSeed+"/a7-target-evaluator/sk")
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
	return plan, pool
}
