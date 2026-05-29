package bootstrapping

import (
	"fmt"
	"sort"
	"testing"

	"github.com/tuneinsight/lattigo/v6/core/rlwe"
	"github.com/tuneinsight/lattigo/v6/ring"
	"github.com/tuneinsight/lattigo/v6/schemes/ckks"
	"github.com/tuneinsight/lattigo/v6/utils"
)

func TestBootstrapKeyReuseA5SourceValidationAllowsPrefixView(t *testing.T) {
	params := bkrA5ValidationParameters(t)
	levelQ := 1
	levelP := params.MaxLevelP()
	baseTwoDecomposition := 0
	highLevelQ := params.MaxLevel()

	sk, err := newDeterministicSecretKey(params, bkrDefaultSeed+"/a5/source-validation/sk")
	if err != nil {
		t.Fatal(err)
	}

	kgen := rlwe.NewKeyGenerator(params)
	evk := kgen.GenEvaluationKeyNew(sk, sk, bkrA5EvaluationKeyParameters(highLevelQ, levelP, baseTwoDecomposition))

	if err := bkrA5CanSliceEvaluationKeyMaterial(params, evk, levelQ, levelP); err != nil {
		t.Fatal(err)
	}
	sliced, err := bkrSliceEvaluationKeyForLevel(params, evk, levelQ, levelP)
	if err != nil {
		t.Fatal(err)
	}
	bkrAssertA5EvaluationKeyPrefixView(t, evk, sliced, levelQ, levelP)
}

func TestBootstrapKeyReuseA5RNSSliceApplyEvaluationKeyEquivalence(t *testing.T) {
	params := bkrA5ValidationParameters(t)
	levelQ := 1
	levelP := params.MaxLevelP()
	highLevelQ := params.MaxLevel()
	baseTwoDecomposition := 0

	sk, err := newDeterministicSecretKey(params, bkrDefaultSeed+"/a5/apply-evaluation-key/sk")
	if err != nil {
		t.Fatal(err)
	}
	kgen := rlwe.NewKeyGenerator(params)
	exact := kgen.GenEvaluationKeyNew(sk, sk, bkrA5EvaluationKeyParameters(levelQ, levelP, baseTwoDecomposition))
	superset := kgen.GenEvaluationKeyNew(sk, sk, bkrA5EvaluationKeyParameters(highLevelQ, levelP, baseTwoDecomposition))
	sliced, err := bkrSliceEvaluationKeyForLevel(params, superset, levelQ, levelP)
	if err != nil {
		t.Fatal(err)
	}
	bkrAssertA5EvaluationKeyPrefixView(t, superset, sliced, levelQ, levelP)

	values := bkrComplexValues(params, bkrDefaultSeed+"/a5/apply-evaluation-key/values")
	ct := bkrA5EncryptAtLevel(t, params, sk, values, levelQ)

	for _, tc := range []struct {
		name string
		key  *rlwe.EvaluationKey
	}{
		{name: "exact", key: exact},
		{name: "superset", key: superset},
		{name: "slice", key: sliced},
	} {
		out := ckks.NewCiphertext(params, 1, levelQ)
		if err := ckks.NewEvaluator(params, nil).ApplyEvaluationKey(ct.CopyNew(), tc.key, out); err != nil {
			t.Fatalf("%s ApplyEvaluationKey failed: %v", tc.name, err)
		}
		bkrAssertA5CKKSOutput(t, params, sk, values, out, levelQ, ct.Scale.Log2(), tc.name)
	}
}

func TestBootstrapKeyReuseA5RNSSliceRotationStressBootstrappingRotations(t *testing.T) {
	params := bkrA5ValidationParameters(t)
	spec := bkrCaseA5P2MultiFastUsedSparse()
	_, btpParams, err := buildTargetBootstrappingParameters(spec, spec.UsedTargetLevels[0])
	if err != nil {
		t.Fatal(err)
	}

	rotationKs := bkrA5BootstrappingRotationKs(t, params, btpParams)
	if len(rotationKs) == 0 {
		t.Fatal("no bootstrapping rotations found to stress")
	}

	levelQ := 1
	levelP := params.MaxLevelP()
	highLevelQ := params.MaxLevel()
	baseTwoDecomposition := 0
	sk, err := newDeterministicSecretKey(params, bkrDefaultSeed+"/a5/rotation-stress/sk")
	if err != nil {
		t.Fatal(err)
	}
	values := bkrComplexValues(params, bkrDefaultSeed+"/a5/rotation-stress/values")
	ct := bkrA5EncryptAtLevel(t, params, sk, values, levelQ)
	kgen := rlwe.NewKeyGenerator(params)

	for _, rotation := range rotationKs {
		galEl := params.GaloisElementForRotation(rotation)
		exact := kgen.GenGaloisKeyNew(galEl, sk, bkrA5EvaluationKeyParameters(levelQ, levelP, baseTwoDecomposition))
		superset := kgen.GenGaloisKeyNew(galEl, sk, bkrA5EvaluationKeyParameters(highLevelQ, levelP, baseTwoDecomposition))
		sliced, err := bkrSliceGaloisKeyForLevel(params, superset, levelQ, levelP)
		if err != nil {
			t.Fatal(err)
		}
		bkrAssertA5EvaluationKeyPrefixView(t, &superset.EvaluationKey, &sliced.EvaluationKey, levelQ, levelP)

		want := utils.RotateSlice(values, rotation)
		for _, tc := range []struct {
			name string
			key  *rlwe.GaloisKey
		}{
			{name: "exact", key: exact},
			{name: "superset", key: superset},
			{name: "slice", key: sliced},
		} {
			eval := ckks.NewEvaluator(params, rlwe.NewMemEvaluationKeySet(nil, tc.key))
			out := ckks.NewCiphertext(params, 1, levelQ)
			if err := eval.Rotate(ct.CopyNew(), rotation, out); err != nil {
				t.Fatalf("%s rotation %d failed: %v", tc.name, rotation, err)
			}
			bkrAssertA5CKKSOutput(t, params, sk, want, out, levelQ, ct.Scale.Log2(), fmt.Sprintf("%s rotation %d", tc.name, rotation))
		}
	}
}

func bkrA5ValidationParameters(t *testing.T) ckks.Parameters {
	t.Helper()
	params, err := ckks.NewParametersFromLiteral(bkrCaseP2MultiFastClustered().SchemeParams)
	if err != nil {
		t.Fatal(err)
	}
	return params
}

func bkrA5EvaluationKeyParameters(levelQ, levelP, baseTwoDecomposition int) rlwe.EvaluationKeyParameters {
	return rlwe.EvaluationKeyParameters{
		LevelQ:               &levelQ,
		LevelP:               &levelP,
		BaseTwoDecomposition: &baseTwoDecomposition,
		Compressed:           false,
	}
}

func bkrA5EncryptAtLevel(t *testing.T, params ckks.Parameters, sk *rlwe.SecretKey, values []complex128, level int) *rlwe.Ciphertext {
	t.Helper()
	encoder := ckks.NewEncoder(params)
	plaintext := ckks.NewPlaintext(params, level)
	if err := encoder.Encode(values, plaintext); err != nil {
		t.Fatal(err)
	}
	ct, err := rlwe.NewEncryptor(params, sk).EncryptNew(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if ct.Level() != level {
		t.Fatalf("ciphertext level=%d, want %d", ct.Level(), level)
	}
	return ct
}

func bkrAssertA5CKKSOutput(t *testing.T, params ckks.Parameters, sk *rlwe.SecretKey, want []complex128, out *rlwe.Ciphertext, wantLevel int, wantScaleLog2 float64, label string) {
	t.Helper()
	if out.Level() != wantLevel {
		t.Fatalf("%s output level=%d, want %d", label, out.Level(), wantLevel)
	}
	if out.Scale.Log2() != wantScaleLog2 {
		t.Fatalf("%s output scale log2=%f, want %f", label, out.Scale.Log2(), wantScaleLog2)
	}
	precision := ckks.GetPrecisionStats(params, ckks.NewEncoder(params), rlwe.NewDecryptor(params, sk), want, out, 0, false)
	if precision.AVGLog2Prec.Real < bkrMinPrecisionBits || precision.AVGLog2Prec.Imag < bkrMinPrecisionBits {
		t.Fatalf("%s precision below threshold %.2f: real=%f imag=%f", label, bkrMinPrecisionBits, precision.AVGLog2Prec.Real, precision.AVGLog2Prec.Imag)
	}
}

func bkrA5BootstrappingRotationKs(t *testing.T, params ckks.Parameters, btpParams Parameters) []int {
	t.Helper()
	conjugation := params.GaloisElementForComplexConjugation()
	seen := map[int]bool{}
	for _, galEl := range btpParams.GaloisElements(btpParams.BootstrappingParameters) {
		if galEl == 1 || galEl == conjugation {
			continue
		}
		rotation := params.GetRLWEParameters().SolveDiscreteLogGaloisElement(galEl)
		rotation %= params.MaxSlots()
		if rotation == 0 {
			continue
		}
		seen[rotation] = true
	}
	rotations := make([]int, 0, len(seen))
	for rotation := range seen {
		rotations = append(rotations, rotation)
	}
	sort.Ints(rotations)
	return rotations
}

func bkrAssertA5EvaluationKeyPrefixView(t *testing.T, owner, view *rlwe.EvaluationKey, levelQ, levelP int) {
	t.Helper()
	if view.LevelQ() != levelQ {
		t.Fatalf("view LevelQ=%d, want %d", view.LevelQ(), levelQ)
	}
	if view.LevelP() != levelP {
		t.Fatalf("view LevelP=%d, want %d", view.LevelP(), levelP)
	}
	if len(view.Value) == 0 || len(view.Value[0]) == 0 || len(view.Value[0][0]) == 0 {
		t.Fatal("view has empty gadget ciphertext")
	}
	bkrAssertA5PolyPrefixAlias(t, owner.Value[0][0][0].Q, view.Value[0][0][0].Q, levelQ)
	if levelP >= 0 {
		bkrAssertA5PolyPrefixAlias(t, owner.Value[0][0][0].P, view.Value[0][0][0].P, levelP)
	}
}

func bkrAssertA5PolyPrefixAlias(t *testing.T, owner, view ring.Poly, level int) {
	t.Helper()
	if owner.Level() < level {
		t.Fatalf("owner level=%d smaller than required level=%d", owner.Level(), level)
	}
	if view.Level() != level {
		t.Fatalf("view level=%d, want %d", view.Level(), level)
	}
	if len(view.Coeffs) == 0 || len(view.Coeffs[0]) == 0 {
		t.Fatal("view poly has empty coefficients")
	}
	if &owner.Coeffs[0][0] != &view.Coeffs[0][0] {
		t.Fatal("view poly does not share the owner coefficient backing array")
	}
}
