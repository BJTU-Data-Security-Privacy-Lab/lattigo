package bootstrapping

import (
	"fmt"

	"github.com/tuneinsight/lattigo/v6/core/rlwe"
	"github.com/tuneinsight/lattigo/v6/ring"
	"github.com/tuneinsight/lattigo/v6/ring/ringqp"
	"github.com/tuneinsight/lattigo/v6/utils/structs"
)

func bkrA5DefaultRNSSliceStatus() string {
	return "disabled"
}

func bkrA5CanSliceEvaluationKeyMaterial(params rlwe.ParameterProvider, evk *rlwe.EvaluationKey, levelQ, levelP int) error {
	if params == nil {
		return fmt.Errorf("cannot validate RNS slice for nil params")
	}
	if evk == nil {
		return fmt.Errorf("cannot validate RNS slice for nil evaluation key")
	}
	if evk.IsCompressed() {
		return fmt.Errorf("compressed evaluation-key RNS slicing is not enabled")
	}
	if levelQ < 0 {
		return fmt.Errorf("required LevelQ=%d is invalid", levelQ)
	}
	if levelQ > evk.LevelQ() {
		return fmt.Errorf("required LevelQ=%d exceeds owner LevelQ=%d", levelQ, evk.LevelQ())
	}
	if levelP != evk.LevelP() {
		return fmt.Errorf("required LevelP=%d differs from owner LevelP=%d", levelP, evk.LevelP())
	}

	rlweParams := params.GetRLWEParameters()
	if levelQ > rlweParams.MaxLevelQ() {
		return fmt.Errorf("required LevelQ=%d exceeds params MaxLevelQ=%d", levelQ, rlweParams.MaxLevelQ())
	}
	if levelP > rlweParams.MaxLevelP() {
		return fmt.Errorf("required LevelP=%d exceeds params MaxLevelP=%d", levelP, rlweParams.MaxLevelP())
	}

	requiredRNS := rlweParams.BaseRNSDecompositionVectorSize(levelQ, levelP)
	if requiredRNS > evk.BaseRNSDecompositionVectorSize() {
		return fmt.Errorf("required BaseRNSDecompositionVectorSize=%d exceeds owner size=%d", requiredRNS, evk.BaseRNSDecompositionVectorSize())
	}

	requiredBaseTwo := rlweParams.BaseTwoDecompositionVectorSize(levelQ, levelP, evk.BaseTwoDecomposition)
	for i := 0; i < requiredRNS; i++ {
		if len(evk.Value[i]) < requiredBaseTwo[i] {
			return fmt.Errorf("required BaseTwoDecompositionVectorSize[%d]=%d exceeds owner size=%d", i, requiredBaseTwo[i], len(evk.Value[i]))
		}
		for j := 0; j < requiredBaseTwo[i]; j++ {
			for u, poly := range evk.Value[i][j] {
				if poly.LevelQ() < levelQ {
					return fmt.Errorf("owner Value[%d][%d][%d] LevelQ=%d below required LevelQ=%d", i, j, u, poly.LevelQ(), levelQ)
				}
				if poly.LevelP() != levelP {
					return fmt.Errorf("owner Value[%d][%d][%d] LevelP=%d differs from required LevelP=%d", i, j, u, poly.LevelP(), levelP)
				}
			}
		}
	}

	return nil
}

func bkrSliceEvaluationKeyForLevel(params rlwe.ParameterProvider, evk *rlwe.EvaluationKey, levelQ, levelP int) (*rlwe.EvaluationKey, error) {
	if err := bkrA5CanSliceEvaluationKeyMaterial(params, evk, levelQ, levelP); err != nil {
		return nil, err
	}

	gadget, err := bkrSliceGadgetCiphertextForLevel(params, &evk.GadgetCiphertext, levelQ, levelP)
	if err != nil {
		return nil, err
	}

	return &rlwe.EvaluationKey{
		GadgetCiphertext: *gadget,
		Seed:             evk.Seed,
	}, nil
}

func bkrSliceGaloisKeyForLevel(params rlwe.ParameterProvider, gk *rlwe.GaloisKey, levelQ, levelP int) (*rlwe.GaloisKey, error) {
	if gk == nil {
		return nil, fmt.Errorf("cannot slice nil Galois key")
	}
	evk, err := bkrSliceEvaluationKeyForLevel(params, &gk.EvaluationKey, levelQ, levelP)
	if err != nil {
		return nil, err
	}
	return &rlwe.GaloisKey{
		GaloisElement: gk.GaloisElement,
		NthRoot:       gk.NthRoot,
		EvaluationKey: *evk,
	}, nil
}

func bkrSliceGadgetCiphertextForLevel(params rlwe.ParameterProvider, gadget *rlwe.GadgetCiphertext, levelQ, levelP int) (*rlwe.GadgetCiphertext, error) {
	if gadget == nil {
		return nil, fmt.Errorf("cannot slice nil gadget ciphertext")
	}

	rlweParams := params.GetRLWEParameters()
	requiredRNS := rlweParams.BaseRNSDecompositionVectorSize(levelQ, levelP)
	requiredBaseTwo := rlweParams.BaseTwoDecompositionVectorSize(levelQ, levelP, gadget.BaseTwoDecomposition)

	value := make(structs.Matrix[rlwe.VectorQP], requiredRNS)
	for i := 0; i < requiredRNS; i++ {
		value[i] = make([]rlwe.VectorQP, requiredBaseTwo[i])
		for j := 0; j < requiredBaseTwo[i]; j++ {
			var err error
			value[i][j], err = bkrSliceVectorQPForLevel(gadget.Value[i][j], levelQ, levelP)
			if err != nil {
				return nil, fmt.Errorf("cannot slice gadget Value[%d][%d]: %w", i, j, err)
			}
		}
	}

	return &rlwe.GadgetCiphertext{
		BaseTwoDecomposition: gadget.BaseTwoDecomposition,
		Value:                value,
	}, nil
}

func bkrSliceVectorQPForLevel(vector rlwe.VectorQP, levelQ, levelP int) (rlwe.VectorQP, error) {
	out := make(rlwe.VectorQP, len(vector))
	for i, poly := range vector {
		sliced, err := bkrSlicePolyQPForLevel(poly, levelQ, levelP)
		if err != nil {
			return nil, fmt.Errorf("cannot slice VectorQP[%d]: %w", i, err)
		}
		out[i] = sliced
	}
	return out, nil
}

func bkrSlicePolyQPForLevel(poly ringqp.Poly, levelQ, levelP int) (ringqp.Poly, error) {
	q, err := bkrSliceRingPolyForLevel(poly.Q, levelQ)
	if err != nil {
		return ringqp.Poly{}, fmt.Errorf("Q: %w", err)
	}
	p, err := bkrSliceRingPolyForLevel(poly.P, levelP)
	if err != nil {
		return ringqp.Poly{}, fmt.Errorf("P: %w", err)
	}
	return ringqp.Poly{Q: q, P: p}, nil
}

func bkrSliceRingPolyForLevel(poly ring.Poly, level int) (ring.Poly, error) {
	if level < 0 {
		return ring.Poly{}, nil
	}
	if poly.Level() < level {
		return ring.Poly{}, fmt.Errorf("owner level=%d below required level=%d", poly.Level(), level)
	}
	return ring.Poly{Coeffs: poly.Coeffs[:level+1]}, nil
}
