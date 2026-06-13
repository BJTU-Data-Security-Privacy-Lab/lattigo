package bootstrapping

import (
	"errors"
	"reflect"
	"testing"

	"github.com/tuneinsight/lattigo/v6/core/rlwe"
)

func TestKeyMaterialPoolDeduplicatesSharedRotationMaterial(t *testing.T) {
	pool := NewKeyMaterialPool()
	key := KeyMaterialKey{
		Kind:           KeyMaterialKindRotation,
		LevelQ:         3,
		LevelP:         1,
		ParametersHash: "params",
		SecretDomain:   "skN2",
		GaloisElement:  5,
		DescriptorHash: "rotation-5",
	}
	value := &rlwe.GaloisKey{GaloisElement: 5}

	if err := pool.Add(3, []int{1}, key, KeyMaterialViewPhysical, value, 128); err != nil {
		t.Fatal(err)
	}
	if err := pool.Add(3, []int{3}, key, KeyMaterialViewPhysical, value, 128); err != nil {
		t.Fatal(err)
	}

	if got, want := pool.PhysicalMaterialCount(), 1; got != want {
		t.Fatalf("PhysicalMaterialCount=%d, want %d", got, want)
	}
	if got, want := pool.SharedMaterialCount(), 1; got != want {
		t.Fatalf("SharedMaterialCount=%d, want %d", got, want)
	}
	if got, want := pool.MaterialBinarySize(), 128; got != want {
		t.Fatalf("MaterialBinarySize=%d, want %d", got, want)
	}
	object, ok := pool.Object(key)
	if !ok {
		t.Fatal("Object returned ok=false")
	}
	if got, want := object.Targets, []int{1, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Targets=%v, want %v", got, want)
	}
}

func TestKeyMaterialPoolSeparatesIncompatibleSecretDomains(t *testing.T) {
	pool := NewKeyMaterialPool()
	keyA := KeyMaterialKey{
		Kind:           KeyMaterialKindRelinearization,
		LevelQ:         3,
		LevelP:         1,
		ParametersHash: "params",
		SecretDomain:   "skN2-a",
		DescriptorHash: "rlk-a",
	}
	keyB := keyA
	keyB.SecretDomain = "skN2-b"
	keyB.DescriptorHash = "rlk-b"

	if err := pool.Add(1, []int{1}, keyA, KeyMaterialViewPhysical, &rlwe.RelinearizationKey{}, 64); err != nil {
		t.Fatal(err)
	}
	if err := pool.Add(3, []int{3}, keyB, KeyMaterialViewPhysical, &rlwe.RelinearizationKey{}, 64); err != nil {
		t.Fatal(err)
	}

	if got, want := pool.CountByKind(KeyMaterialKindRelinearization), 2; got != want {
		t.Fatalf("CountByKind=%d, want %d", got, want)
	}
	if got, want := pool.PrivateMaterialCount(), 2; got != want {
		t.Fatalf("PrivateMaterialCount=%d, want %d", got, want)
	}
}

func TestKeyMaterialPoolBuildsTargetManifest(t *testing.T) {
	pool := NewKeyMaterialPool()
	sharedRotation := KeyMaterialKey{
		Kind:           KeyMaterialKindRotation,
		LevelQ:         3,
		LevelP:         1,
		ParametersHash: "params",
		SecretDomain:   "skN2",
		GaloisElement:  7,
		DescriptorHash: "rotation-7",
	}
	privateSwitch := KeyMaterialKey{
		Kind:           KeyMaterialKindRingSwitch,
		LevelQ:         3,
		LevelP:         1,
		ParametersHash: "params",
		SecretDomain:   "skN1-to-skN2",
		Direction:      "N1ToN2",
		DescriptorHash: "n1-to-n2",
	}

	if err := pool.Add(3, []int{1, 3}, sharedRotation, KeyMaterialViewPhysical, &rlwe.GaloisKey{GaloisElement: 7}, 128); err != nil {
		t.Fatal(err)
	}
	if err := pool.Add(1, []int{1}, privateSwitch, KeyMaterialViewPhysical, &rlwe.EvaluationKey{}, 96); err != nil {
		t.Fatal(err)
	}

	manifest, ok := pool.ManifestForTarget(1)
	if !ok {
		t.Fatal("ManifestForTarget returned ok=false")
	}
	if got, want := manifest.TargetLevel, 1; got != want {
		t.Fatalf("TargetLevel=%d, want %d", got, want)
	}
	if got, want := manifest.Shared, []KeyMaterialKey{sharedRotation}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Shared=%v, want %v", got, want)
	}
	if got, want := manifest.Private, []KeyMaterialKey{privateSwitch}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Private=%v, want %v", got, want)
	}
}

func TestKeyMaterialPoolRecordsSingleBootstrapSecretDomainPerManifest(t *testing.T) {
	pool := NewKeyMaterialPool()
	key := KeyMaterialKey{
		Kind:           KeyMaterialKindRotation,
		LevelQ:         3,
		LevelP:         1,
		ParametersHash: "params",
		SecretDomain:   "rotation-skN2",
		GaloisElement:  5,
		DescriptorHash: "rotation-5",
	}
	if err := pool.Add(3, []int{1}, key, KeyMaterialViewPhysical, &rlwe.GaloisKey{GaloisElement: 5}, 128); err != nil {
		t.Fatal(err)
	}
	if err := pool.SetManifestBootstrapSecretDomain(1, 3, "owner-bootstrap-skN2"); err != nil {
		t.Fatal(err)
	}

	manifest, ok := pool.ManifestForTarget(1)
	if !ok {
		t.Fatal("ManifestForTarget returned ok=false")
	}
	if got, want := manifest.KeyMaterialOwnerLevel, 3; got != want {
		t.Fatalf("KeyMaterialOwnerLevel=%d, want %d", got, want)
	}
	if got, want := manifest.BootstrapSecretDomain, "owner-bootstrap-skN2"; got != want {
		t.Fatalf("BootstrapSecretDomain=%q, want %q", got, want)
	}

	err := pool.SetManifestBootstrapSecretDomain(1, 4, "other-bootstrap-skN2")
	if !errors.Is(err, ErrIncompatibleReusableMaterial) {
		t.Fatalf("SetManifestBootstrapSecretDomain error=%v, want ErrIncompatibleReusableMaterial", err)
	}
}

func TestKeyMaterialPoolRejectsConflictingObjectsWithSameKey(t *testing.T) {
	pool := NewKeyMaterialPool()
	key := KeyMaterialKey{
		Kind:           KeyMaterialKindRotation,
		LevelQ:         3,
		LevelP:         1,
		ParametersHash: "params",
		SecretDomain:   "skN2",
		GaloisElement:  5,
		DescriptorHash: "rotation-5",
	}

	if err := pool.Add(3, []int{1}, key, KeyMaterialViewPhysical, &rlwe.GaloisKey{GaloisElement: 5}, 128); err != nil {
		t.Fatal(err)
	}
	err := pool.Add(3, []int{3}, key, KeyMaterialViewPhysical, &rlwe.GaloisKey{GaloisElement: 5}, 128)
	if !errors.Is(err, ErrIncompatibleReusableMaterial) {
		t.Fatalf("Add error=%v, want ErrIncompatibleReusableMaterial", err)
	}
}

func TestKeyMaterialPoolAddsOnlyKeyMaterialKinds(t *testing.T) {
	pool := NewKeyMaterialPool()
	key := KeyMaterialKey{
		Kind:           KeyMaterialKind("linear_transformation"),
		LevelQ:         3,
		LevelP:         1,
		ParametersHash: "params",
		SecretDomain:   "skN2",
		DescriptorHash: "lt",
	}

	err := pool.Add(1, []int{1}, key, KeyMaterialViewPhysical, &rlwe.EvaluationKey{}, 32)
	if !errors.Is(err, ErrIncompatibleReusableMaterial) {
		t.Fatalf("Add error=%v, want ErrIncompatibleReusableMaterial", err)
	}
}
