package rlwe

import (
	"fmt"

	"github.com/tuneinsight/lattigo/v6/ring"
	"github.com/tuneinsight/lattigo/v6/ring/ringqp"
	"github.com/tuneinsight/lattigo/v6/utils"
)

// GadgetProduct evaluates poly x Gadget -> RLWE where
//
// ct = [<decomp(cx), gadget[0]>, <decomp(cx), gadget[1]>] mod Q
//
// Expects the flag IsNTT of ct to correctly reflect the domain of cx.
func (eval Evaluator) GadgetProduct(levelQ int, cx ring.Poly, gadgetCt *GadgetCiphertext, ct *Ciphertext) {

	levelQ = utils.Min(levelQ, gadgetCt.LevelQ())
	levelP := gadgetCt.LevelP()

	poolQP := eval.pool.AtLevel(levelQ, levelP)
	buffQP1 := poolQP.GetBuffPolyQP()
	defer poolQP.RecycleBuffPolyQP(buffQP1)
	buffQP2 := poolQP.GetBuffPolyQP()
	defer poolQP.RecycleBuffPolyQP(buffQP2)

	ctTmp := &Element[ringqp.Poly]{}
	ctTmp.Value = []ringqp.Poly{{Q: ct.Value[0], P: (*buffQP1).P}, {Q: ct.Value[1], P: (*buffQP2).P}}
	ctTmp.MetaData = ct.MetaData

	if err := eval.GadgetProductLazy(levelQ, cx, gadgetCt, ctTmp); err != nil {
		panic(fmt.Errorf("eval.GadgetProductLazy: %w", err))
	}

	eval.ModDown(levelQ, levelP, ctTmp, ct)
}

// ModDown takes ctQP (mod QP) and returns ct = (ctQP/P) (mod Q).
func (eval Evaluator) ModDown(levelQ, levelP int, ctQP *Element[ringqp.Poly], ct *Ciphertext) {

	ringQP := eval.params.RingQP().AtLevel(levelQ, levelP)

	if levelP != -1 {
		if ctQP.IsNTT {
			if ct.IsNTT {
				// NTT -> NTT
				eval.BasisExtender.ModDownQPtoQNTT(levelQ, levelP, ctQP.Value[0].Q, ctQP.Value[0].P, ct.Value[0])
				eval.BasisExtender.ModDownQPtoQNTT(levelQ, levelP, ctQP.Value[1].Q, ctQP.Value[1].P, ct.Value[1])
			} else {
				// NTT -> INTT
				ringQP := eval.params.RingQP().AtLevel(levelQ, levelP)

				ringQP.INTTLazy(ctQP.Value[0], ctQP.Value[0])
				ringQP.INTTLazy(ctQP.Value[1], ctQP.Value[1])

				eval.BasisExtender.ModDownQPtoQ(levelQ, levelP, ctQP.Value[0].Q, ctQP.Value[0].P, ct.Value[0])
				eval.BasisExtender.ModDownQPtoQ(levelQ, levelP, ctQP.Value[1].Q, ctQP.Value[1].P, ct.Value[1])
			}
		} else {
			if ct.IsNTT {
				// INTT -> NTT
				eval.BasisExtender.ModDownQPtoQ(levelQ, levelP, ctQP.Value[0].Q, ctQP.Value[0].P, ct.Value[0])
				eval.BasisExtender.ModDownQPtoQ(levelQ, levelP, ctQP.Value[1].Q, ctQP.Value[1].P, ct.Value[1])

				ringQP.RingQ.NTT(ct.Value[0], ct.Value[0])
				ringQP.RingQ.NTT(ct.Value[1], ct.Value[1])
			} else {
				// INTT -> INTT
				eval.BasisExtender.ModDownQPtoQ(levelQ, levelP, ctQP.Value[0].Q, ctQP.Value[0].P, ct.Value[0])
				eval.BasisExtender.ModDownQPtoQ(levelQ, levelP, ctQP.Value[1].Q, ctQP.Value[1].P, ct.Value[1])
			}
		}
	} else {
		if ctQP.IsNTT {
			if ct.IsNTT {
				// NTT -> NTT
				ctQP.Value[0].Q.CopyLvl(levelQ, ct.Value[0])
				ctQP.Value[1].Q.CopyLvl(levelQ, ct.Value[1])
			} else {
				// NTT -> INTT
				ringQP.RingQ.INTT(ctQP.Value[0].Q, ct.Value[0])
				ringQP.RingQ.INTT(ctQP.Value[1].Q, ct.Value[1])
			}
		} else {
			if ct.IsNTT {
				// INTT -> NTT
				ringQP.RingQ.NTT(ctQP.Value[0].Q, ct.Value[0])
				ringQP.RingQ.NTT(ctQP.Value[1].Q, ct.Value[1])

			} else {
				// INTT -> INTT
				ctQP.Value[0].Q.CopyLvl(levelQ, ct.Value[0])
				ctQP.Value[1].Q.CopyLvl(levelQ, ct.Value[1])
			}
		}
	}
}

// GadgetProductLazy evaluates poly x Gadget -> RLWE where
//
// ctQP = [<decomp(cx), gadget[0]>, <decomp(cx), gadget[1]>] mod QP
//
// Expects the flag IsNTT of ctQP to correctly reflect the domain of cx.
//
// Result NTT domain is returned according to the NTT flag of ctQP.
//
// The method will return an error if ctQP.Level() < gadgetCt.Level().
func (eval Evaluator) GadgetProductLazy(levelQ int, cx ring.Poly, gadgetCt *GadgetCiphertext, ctQP *Element[ringqp.Poly]) (err error) {

	if ctQP.LevelP() < gadgetCt.LevelP() {
		return fmt.Errorf("ctQP.LevelP()=%d < gadgetCt.LevelP()=%d", ctQP.LevelP(), gadgetCt.LevelP())
	}

	if gadgetCt.LevelP() > 0 {
		eval.gadgetProductMultiplePLazy(levelQ, cx, gadgetCt, ctQP)
	} else if gadgetCt.LevelP() == 0 && gadgetCt.BaseTwoDecomposition == 0 {
		eval.gadgetProductSinglePNoBaseTwoLazy(levelQ, cx, gadgetCt, ctQP)
	} else {
		eval.gadgetProductSinglePAndBitDecompLazy(levelQ, cx, gadgetCt, ctQP)
	}

	if !ctQP.IsNTT {
		ringQP := eval.params.RingQP().AtLevel(levelQ, gadgetCt.LevelP())
		ringQP.INTT(ctQP.Value[0], ctQP.Value[0])
		ringQP.INTT(ctQP.Value[1], ctQP.Value[1])
	}

	return
}

func gadgetProductInputAliasesOutput(levelQ int, cx ring.Poly, ctQP *Element[ringqp.Poly]) bool {
	return polyAliases(levelQ, cx, ctQP.Value[0].Q) || polyAliases(levelQ, cx, ctQP.Value[1].Q)
}

func polyAliases(level int, p0, p1 ring.Poly) bool {
	for i := 0; i < level+1; i++ {
		p0Coeffs := p0.Coeffs[i]
		if len(p0Coeffs) == 0 {
			continue
		}

		if p1Coeffs := p1.Coeffs[i]; len(p1Coeffs) != 0 && &p0Coeffs[0] == &p1Coeffs[0] {
			return true
		}
	}

	return false
}

func (eval Evaluator) gadgetProductInputDomains(levelQ int, cx ring.Poly, cxIsNTT bool, aliasesOut bool, ringQ *ring.Ring, pool *BufferPool) (cxNTT, cxInvNTT ring.Poly, buff0, buff1 *ring.Poly) {
	if cxIsNTT {
		cxNTT = cx
		buff0 = pool.GetBuffPoly()
		cxInvNTT = *buff0
		ringQ.INTT(cxNTT, cxInvNTT)

		if aliasesOut {
			buff1 = pool.GetBuffPoly()
			buff1.CopyLvl(levelQ, cx)
			cxNTT = *buff1
		}
	} else {
		cxInvNTT = cx

		if aliasesOut {
			buff0 = pool.GetBuffPoly()
			buff0.CopyLvl(levelQ, cx)
			cxInvNTT = *buff0
		}

		buff1 = pool.GetBuffPoly()
		cxNTT = *buff1
		ringQ.NTT(cxInvNTT, cxNTT)
	}

	return
}

func (eval Evaluator) gadgetProductMultiplePLazy(levelQ int, cx ring.Poly, gadgetCt *GadgetCiphertext, ctQP *Element[ringqp.Poly]) {

	levelP := gadgetCt.LevelP()

	ringQP := eval.params.RingQP().AtLevel(levelQ, levelP)
	poolQP := eval.pool.AtLevel(levelQ, levelP)

	ringQ := ringQP.RingQ
	ringP := ringQP.RingP

	buff := poolQP.GetBuffPolyQP()
	defer poolQP.RecycleBuffPolyQP(buff)

	c2QP := *buff

	cxAliasesOut := gadgetProductInputAliasesOutput(levelQ, cx, ctQP)
	cxNTT, cxInvNTT, buff0, buff1 := eval.gadgetProductInputDomains(levelQ, cx, ctQP.IsNTT, cxAliasesOut, ringQ, poolQP)
	if buff0 != nil {
		defer poolQP.RecycleBuffPoly(buff0)
	}
	if buff1 != nil {
		defer poolQP.RecycleBuffPoly(buff1)
	}

	BaseRNSDecompositionVectorSize := eval.params.BaseRNSDecompositionVectorSize(levelQ, levelP)

	QiOverF := eval.params.QiOverflowMargin(levelQ) >> 1
	PiOverF := eval.params.PiOverflowMargin(levelP) >> 1

	el := gadgetCt.Value

	// Re-encryption with CRT decomposition for the Qi
	var reduce int
	for i := 0; i < BaseRNSDecompositionVectorSize; i++ {

		eval.DecomposeSingleNTT(levelQ, levelP, levelP+1, i, cxNTT, cxInvNTT, c2QP.Q, c2QP.P)

		if i == 0 {
			ringQP.MulCoeffsMontgomeryLazy(el[i][0][0], c2QP, ctQP.Value[0])
			ringQP.MulCoeffsMontgomeryLazy(el[i][0][1], c2QP, ctQP.Value[1])
		} else {
			ringQP.MulCoeffsMontgomeryLazyThenAddLazy(el[i][0][0], c2QP, ctQP.Value[0])
			ringQP.MulCoeffsMontgomeryLazyThenAddLazy(el[i][0][1], c2QP, ctQP.Value[1])
		}

		if reduce%QiOverF == QiOverF-1 {
			ringQ.Reduce(ctQP.Value[0].Q, ctQP.Value[0].Q)
			ringQ.Reduce(ctQP.Value[1].Q, ctQP.Value[1].Q)
		}

		if reduce%PiOverF == PiOverF-1 {
			ringP.Reduce(ctQP.Value[0].P, ctQP.Value[0].P)
			ringP.Reduce(ctQP.Value[1].P, ctQP.Value[1].P)
		}

		reduce++
	}

	if reduce%QiOverF != 0 {
		ringQ.Reduce(ctQP.Value[0].Q, ctQP.Value[0].Q)
		ringQ.Reduce(ctQP.Value[1].Q, ctQP.Value[1].Q)
	}

	if reduce%PiOverF != 0 {
		ringP.Reduce(ctQP.Value[0].P, ctQP.Value[0].P)
		ringP.Reduce(ctQP.Value[1].P, ctQP.Value[1].P)
	}
}

func (eval Evaluator) gadgetProductSinglePNoBaseTwoLazy(levelQ int, cx ring.Poly, gadgetCt *GadgetCiphertext, ctQP *Element[ringqp.Poly]) {

	const levelP = 0

	ringQP := eval.params.RingQP().AtLevel(levelQ, levelP)
	poolQP := eval.pool.AtLevel(levelQ, levelP)

	ringQ := ringQP.RingQ
	ringP := ringQP.RingP

	cxAliasesOut := gadgetProductInputAliasesOutput(levelQ, cx, ctQP)
	cxNTT, cxInvNTT, buff0, buff1 := eval.gadgetProductInputDomains(levelQ, cx, ctQP.IsNTT, cxAliasesOut, ringQ, poolQP)
	if buff0 != nil {
		defer poolQP.RecycleBuffPoly(buff0)
	}
	if buff1 != nil {
		defer poolQP.RecycleBuffPoly(buff1)
	}

	QiOverF := eval.params.QiOverflowMargin(levelQ) >> 1
	PiOverF := eval.params.PiOverflowMargin(levelP) >> 1

	Q := ringQ.ModuliChain()
	BRCQ := ringQ.BRedConstants()
	P := ringP.ModuliChain()
	BRCP := ringP.BRedConstants()

	cwBuff := eval.pool.GetBuffUintArray()
	defer eval.pool.RecycleBuffUintArray(cwBuff)
	cw := *cwBuff

	el := gadgetCt.Value
	ct0QP := ctQP.Value[0]
	ct1QP := ctQP.Value[1]

	var reduce int
	for i := 0; i < levelQ+1; i++ {

		coeffs := cxInvNTT.Coeffs[i]
		q := Q[i]
		half := q >> 1
		gct0 := el[i][0][0]
		gct1 := el[i][0][1]

		for u, s := range ringQ.SubRings[:levelQ+1] {
			cwNTT := cxNTT.Coeffs[i]
			if u != i {
				centeredSinglePNoBaseTwoLimb(coeffs, q, half, Q[u], BRCQ[u], cw)
				s.NTTLazy(cw, cw)
				cwNTT = cw
			}

			if i == 0 {
				s.MulCoeffsMontgomeryLazy(gct0.Q.Coeffs[u], cwNTT, ct0QP.Q.Coeffs[u])
				s.MulCoeffsMontgomeryLazy(gct1.Q.Coeffs[u], cwNTT, ct1QP.Q.Coeffs[u])
			} else {
				s.MulCoeffsMontgomeryLazyThenAddLazy(gct0.Q.Coeffs[u], cwNTT, ct0QP.Q.Coeffs[u])
				s.MulCoeffsMontgomeryLazyThenAddLazy(gct1.Q.Coeffs[u], cwNTT, ct1QP.Q.Coeffs[u])
			}
		}

		centeredSinglePNoBaseTwoLimb(coeffs, q, half, P[0], BRCP[0], cw)
		ringP.SubRings[0].NTTLazy(cw, cw)

		if i == 0 {
			ringP.SubRings[0].MulCoeffsMontgomeryLazy(gct0.P.Coeffs[0], cw, ct0QP.P.Coeffs[0])
			ringP.SubRings[0].MulCoeffsMontgomeryLazy(gct1.P.Coeffs[0], cw, ct1QP.P.Coeffs[0])
		} else {
			ringP.SubRings[0].MulCoeffsMontgomeryLazyThenAddLazy(gct0.P.Coeffs[0], cw, ct0QP.P.Coeffs[0])
			ringP.SubRings[0].MulCoeffsMontgomeryLazyThenAddLazy(gct1.P.Coeffs[0], cw, ct1QP.P.Coeffs[0])
		}

		if reduce%QiOverF == QiOverF-1 {
			ringQ.Reduce(ct0QP.Q, ct0QP.Q)
			ringQ.Reduce(ct1QP.Q, ct1QP.Q)
		}

		if reduce%PiOverF == PiOverF-1 {
			ringP.Reduce(ct0QP.P, ct0QP.P)
			ringP.Reduce(ct1QP.P, ct1QP.P)
		}

		reduce++
	}

	if reduce%QiOverF != 0 {
		ringQ.Reduce(ct0QP.Q, ct0QP.Q)
		ringQ.Reduce(ct1QP.Q, ct1QP.Q)
	}

	if reduce%PiOverF != 0 {
		ringP.Reduce(ct0QP.P, ct0QP.P)
		ringP.Reduce(ct1QP.P, ct1QP.P)
	}
}

func centeredSinglePNoBaseTwoLimb(coeffs []uint64, q, half, modulus uint64, bred [2]uint64, out []uint64) {
	for j, coeff := range coeffs {
		pos, neg := uint64(1), uint64(0)
		if coeff >= half {
			coeff = q - coeff
			pos, neg = 0, 1
		}

		tmp := ring.BRedAdd(coeff, modulus, bred)
		out[j] = tmp*pos + (modulus-tmp)*neg
	}
}

func (eval Evaluator) gadgetProductSinglePAndBitDecompLazy(levelQ int, cx ring.Poly, gadgetCt *GadgetCiphertext, ctQP *Element[ringqp.Poly]) {

	levelP := gadgetCt.LevelP()

	ringQP := eval.params.RingQP().AtLevel(levelQ, levelP)
	poolQP := eval.pool.AtLevel(levelQ, levelP)

	ringQ := ringQP.RingQ
	ringP := ringQP.RingP

	var cxInvNTT ring.Poly
	if ctQP.IsNTT {
		buffQ := poolQP.GetBuffPoly()
		defer poolQP.RecycleBuffPoly(buffQ)
		cxInvNTT = *buffQ
		ringQ.INTT(cx, cxInvNTT)
	} else {
		cxInvNTT = cx

		if gadgetProductInputAliasesOutput(levelQ, cx, ctQP) {
			buffQ := poolQP.GetBuffPoly()
			defer poolQP.RecycleBuffPoly(buffQ)
			buffQ.CopyLvl(levelQ, cx)
			cxInvNTT = *buffQ
		}
	}

	pw2 := gadgetCt.BaseTwoDecomposition

	BaseRNSDecompositionVectorSize := levelQ + 1
	BaseTwoDecompositionVectorSize := gadgetCt.BaseTwoDecompositionVectorSize()

	mask := uint64(((1 << pw2) - 1))

	buff := poolQP.GetBuffPolyQP()
	defer poolQP.RecycleBuffPolyQP(buff)

	cw := buff.Q.Coeffs[0]

	buffBitDecomp := eval.pool.GetBuffUintArray()
	defer eval.pool.RecycleBuffUintArray(buffBitDecomp)
	cwNTTBuff := *buffBitDecomp

	QiOverF := eval.params.QiOverflowMargin(levelQ) >> 1
	PiOverF := eval.params.PiOverflowMargin(levelP) >> 1

	el := gadgetCt.Value

	c2QP := buff

	// For the i-th RNS decomposition with no base-2 split, the residue modulo
	// qi is the input itself. If it is already in the NTT domain and does not
	// alias the output, reuse that limb instead of recomputing one NTT per qi.
	reuseCXNTT := mask == 0 && ctQP.IsNTT && !gadgetProductInputAliasesOutput(levelQ, cx, ctQP)

	// Re-encryption with CRT decomposition for the Qi
	var reduce int
	for i := 0; i < BaseRNSDecompositionVectorSize; i++ {

		// Only centers the coefficients if the mask is 0
		// As centering doesn't help reduce the noise if
		// the power of two decomposition is applied on top
		// of the RNS decomposition
		if mask == 0 {
			eval.Decomposer.DecomposeAndSplit(levelQ, levelP, levelP+1, i, cxInvNTT, c2QP.Q, c2QP.P)
		}

		for j := 0; j < BaseTwoDecompositionVectorSize[i]; j++ {

			if mask != 0 {
				ring.MaskVec(cxInvNTT.Coeffs[i], j*pw2, mask, cw)
			}

			if i == 0 && j == 0 {

				for u, s := range ringQ.SubRings[:levelQ+1] {
					cwNTT := cwNTTBuff
					if mask == 0 {
						if reuseCXNTT && u == i {
							cwNTT = cx.Coeffs[u]
						} else {
							s.NTTLazy(c2QP.Q.Coeffs[u], cwNTTBuff)
						}
					} else {
						s.NTTLazy(cw, cwNTTBuff)
					}
					s.MulCoeffsMontgomeryLazy(el[i][j][0].Q.Coeffs[u], cwNTT, ctQP.Value[0].Q.Coeffs[u])
					s.MulCoeffsMontgomeryLazy(el[i][j][1].Q.Coeffs[u], cwNTT, ctQP.Value[1].Q.Coeffs[u])
				}

				if ringP != nil {
					for u, s := range ringP.SubRings[:levelP+1] {
						if mask == 0 {
							s.NTTLazy(c2QP.P.Coeffs[u], cwNTTBuff)
						} else {
							s.NTTLazy(cw, cwNTTBuff)
						}
						s.MulCoeffsMontgomeryLazy(el[i][j][0].P.Coeffs[u], cwNTTBuff, ctQP.Value[0].P.Coeffs[u])
						s.MulCoeffsMontgomeryLazy(el[i][j][1].P.Coeffs[u], cwNTTBuff, ctQP.Value[1].P.Coeffs[u])
					}
				}

			} else {
				for u, s := range ringQ.SubRings[:levelQ+1] {
					cwNTT := cwNTTBuff
					if mask == 0 {
						if reuseCXNTT && u == i {
							cwNTT = cx.Coeffs[u]
						} else {
							s.NTTLazy(c2QP.Q.Coeffs[u], cwNTTBuff)
						}
					} else {
						s.NTTLazy(cw, cwNTTBuff)
					}
					s.MulCoeffsMontgomeryLazyThenAddLazy(el[i][j][0].Q.Coeffs[u], cwNTT, ctQP.Value[0].Q.Coeffs[u])
					s.MulCoeffsMontgomeryLazyThenAddLazy(el[i][j][1].Q.Coeffs[u], cwNTT, ctQP.Value[1].Q.Coeffs[u])
				}

				if ringP != nil {
					for u, s := range ringP.SubRings[:levelP+1] {
						if mask == 0 {
							s.NTTLazy(c2QP.P.Coeffs[u], cwNTTBuff)
						} else {
							s.NTTLazy(cw, cwNTTBuff)
						}
						s.MulCoeffsMontgomeryLazyThenAddLazy(el[i][j][0].P.Coeffs[u], cwNTTBuff, ctQP.Value[0].P.Coeffs[u])
						s.MulCoeffsMontgomeryLazyThenAddLazy(el[i][j][1].P.Coeffs[u], cwNTTBuff, ctQP.Value[1].P.Coeffs[u])
					}
				}
			}

			if reduce%QiOverF == QiOverF-1 {
				ringQ.Reduce(ctQP.Value[0].Q, ctQP.Value[0].Q)
				ringQ.Reduce(ctQP.Value[1].Q, ctQP.Value[1].Q)
			}

			if reduce%PiOverF == PiOverF-1 {
				ringP.Reduce(ctQP.Value[0].P, ctQP.Value[0].P)
				ringP.Reduce(ctQP.Value[1].P, ctQP.Value[1].P)
			}

			reduce++
		}
	}

	if reduce%QiOverF != 0 {
		ringQ.Reduce(ctQP.Value[0].Q, ctQP.Value[0].Q)
		ringQ.Reduce(ctQP.Value[1].Q, ctQP.Value[1].Q)
	}

	if ringP != nil {
		if reduce%PiOverF != 0 {
			ringP.Reduce(ctQP.Value[0].P, ctQP.Value[0].P)
			ringP.Reduce(ctQP.Value[1].P, ctQP.Value[1].P)
		}
	}
}

// GadgetProductHoisted applies the key-switch to the decomposed polynomial c2 mod QP (BuffQPDecompQP)
// and divides the result by P, reducing the basis from QP to Q.
//
// ct = [<BuffQPDecompQP,  gadgetCt[0]) mod Q
//
// BuffQPDecompQP is expected to be in the NTT domain.
//
// Result NTT domain is returned according to the NTT flag of ct.
func (eval Evaluator) GadgetProductHoisted(levelQ int, BuffQPDecompQP []ringqp.Poly, gadgetCt *GadgetCiphertext, ct *Ciphertext) {

	pool := eval.pool.PoolP.AtLevel(gadgetCt.LevelP())
	buffP1 := pool.GetBuffPoly()
	defer pool.RecycleBuffPoly(buffP1)
	buffP2 := pool.GetBuffPoly()
	defer pool.RecycleBuffPoly(buffP2)

	ctQP := &Element[ringqp.Poly]{}
	ctQP.Value = []ringqp.Poly{
		{Q: ct.Value[0], P: *buffP1},
		{Q: ct.Value[1], P: *buffP2},
	}
	ctQP.MetaData = ct.MetaData

	if err := eval.GadgetProductHoistedLazy(levelQ, BuffQPDecompQP, gadgetCt, ctQP); err != nil {
		panic(fmt.Errorf("GadgetProductHoistedLazy: %w", err))
	}
	eval.ModDown(levelQ, gadgetCt.LevelP(), ctQP, ct)
}

// GadgetProductHoistedLazy applies the gadget product to the decomposed polynomial c2 mod QP (BuffQPDecompQ and BuffQPDecompP)
//
// BuffQP2 = dot(BuffQPDecompQ||BuffQPDecompP * gadgetCt[0]) mod QP
// BuffQP3 = dot(BuffQPDecompQ||BuffQPDecompP * gadgetCt[1]) mod QP
//
// BuffQPDecompQP is expected to be in the NTT domain.
//
// Result NTT domain is returned according to the NTT flag of ctQP.
//
// The method will return an error if ctQP.Level() < gadgetCt.Level().
func (eval Evaluator) GadgetProductHoistedLazy(levelQ int, BuffQPDecompQP []ringqp.Poly, gadgetCt *GadgetCiphertext, ctQP *Element[ringqp.Poly]) (err error) {

	// Sanity check for invalid parameters.
	if gadgetCt.BaseTwoDecomposition != 0 {
		return fmt.Errorf("method is unsupported for BaseTwoDecomposition != 0")
	}

	if ctQP.LevelP() < gadgetCt.LevelP() {
		return fmt.Errorf("ctQP.LevelP()=%d < gadgetCt.LevelP()=%d", ctQP.Level(), gadgetCt.LevelP())
	}

	eval.gadgetProductMultiplePLazyHoisted(levelQ, BuffQPDecompQP, gadgetCt, ctQP)

	if !ctQP.IsNTT {
		ringQP := eval.params.RingQP().AtLevel(levelQ, gadgetCt.LevelP())
		ringQP.INTT(ctQP.Value[0], ctQP.Value[0])
		ringQP.INTT(ctQP.Value[1], ctQP.Value[1])
	}

	return
}

func (eval Evaluator) gadgetProductMultiplePLazyHoisted(levelQ int, BuffQPDecompQP []ringqp.Poly, gadgetCt *GadgetCiphertext, ct *Element[ringqp.Poly]) {
	levelP := gadgetCt.LevelP()

	ringQP := eval.params.RingQP().AtLevel(levelQ, levelP)

	ringQ := ringQP.RingQ
	ringP := ringQP.RingP

	c0QP := ct.Value[0]
	c1QP := ct.Value[1]

	BaseRNSDecompositionVectorSize := eval.params.BaseRNSDecompositionVectorSize(levelQ, levelP)

	QiOverF := eval.params.QiOverflowMargin(levelQ) >> 1
	PiOverF := eval.params.PiOverflowMargin(levelP) >> 1

	// Key switching with CRT decomposition for the Qi
	var reduce int
	for i := 0; i < BaseRNSDecompositionVectorSize; i++ {

		gct := gadgetCt.Value[i][0]

		if i == 0 {
			ringQP.MulCoeffsMontgomeryLazy(gct[0], BuffQPDecompQP[i], c0QP)
			ringQP.MulCoeffsMontgomeryLazy(gct[1], BuffQPDecompQP[i], c1QP)
		} else {
			ringQP.MulCoeffsMontgomeryLazyThenAddLazy(gct[0], BuffQPDecompQP[i], c0QP)
			ringQP.MulCoeffsMontgomeryLazyThenAddLazy(gct[1], BuffQPDecompQP[i], c1QP)
		}

		if reduce%QiOverF == QiOverF-1 {
			ringQ.Reduce(c0QP.Q, c0QP.Q)
			ringQ.Reduce(c1QP.Q, c1QP.Q)
		}

		if reduce%PiOverF == PiOverF-1 {
			ringP.Reduce(c0QP.P, c0QP.P)
			ringP.Reduce(c1QP.P, c1QP.P)
		}

		reduce++
	}

	if reduce%QiOverF != 0 {
		ringQ.Reduce(c0QP.Q, c0QP.Q)
		ringQ.Reduce(c1QP.Q, c1QP.Q)
	}

	if reduce%PiOverF != 0 {
		ringP.Reduce(c0QP.P, c0QP.P)
		ringP.Reduce(c1QP.P, c1QP.P)
	}
}

func (eval Evaluator) decomposeSinglePNoBaseTwoNTT(levelQ, qi int, c2QiQ, c2QiP, c2NTT, c2InvNTT ring.Poly, lazy bool) {

	ringQ := eval.params.RingQ().AtLevel(levelQ)
	ringP := eval.params.RingP().AtLevel(0)

	Q := ringQ.ModuliChain()
	BRCQ := ringQ.BRedConstants()
	P := ringP.ModuliChain()
	BRCP := ringP.BRedConstants()

	q := Q[qi]
	coeffs := c2InvNTT.Coeffs[qi]

	for j := 0; j < ringQ.N(); j++ {

		coeff := coeffs[j]
		pos, neg := uint64(1), uint64(0)
		if coeff >= (q >> 1) {
			coeff = q - coeff
			pos, neg = 0, 1
		}

		for u := 0; u < levelQ+1; u++ {
			if u == qi {
				continue
			}

			tmp := ring.BRedAdd(coeff, Q[u], BRCQ[u])
			c2QiQ.Coeffs[u][j] = tmp*pos + (Q[u]-tmp)*neg
		}

		tmp := ring.BRedAdd(coeff, P[0], BRCP[0])
		c2QiP.Coeffs[0][j] = tmp*pos + (P[0]-tmp)*neg
	}

	copy(c2QiQ.Coeffs[qi], c2NTT.Coeffs[qi])

	for u, s := range ringQ.SubRings[:levelQ+1] {
		if u == qi {
			continue
		}

		if lazy {
			s.NTTLazy(c2QiQ.Coeffs[u], c2QiQ.Coeffs[u])
		} else {
			s.NTT(c2QiQ.Coeffs[u], c2QiQ.Coeffs[u])
		}
	}

	if lazy {
		ringP.NTTLazy(c2QiP, c2QiP)
	} else {
		ringP.NTT(c2QiP, c2QiP)
	}
}

// DecomposeNTT applies the full RNS basis decomposition on c2.
// Expects the IsNTT flag of c2 to correctly reflect the domain of c2.
// BuffQPDecompQ and BuffQPDecompQ are vectors of polynomials (mod Q and mod P) that store the
// special RNS decomposition of c2 (in the NTT domain)
func (eval Evaluator) DecomposeNTT(levelQ, levelP, nbPi int, c2 ring.Poly, c2IsNTT bool, decompQP []ringqp.Poly) {

	ringQ := eval.params.RingQ().AtLevel(levelQ)
	poolQ := eval.pool.AtLevel(levelQ)

	var polyNTT, polyInvNTT ring.Poly

	buffQ := poolQ.GetBuffPoly()
	defer poolQ.RecycleBuffPoly(buffQ)

	if c2IsNTT {
		polyNTT = c2
		polyInvNTT = *buffQ
		ringQ.INTT(polyNTT, polyInvNTT)
	} else {
		polyNTT = *buffQ
		polyInvNTT = c2
		ringQ.NTT(polyInvNTT, polyNTT)
	}

	BaseRNSDecompositionVectorSize := eval.params.BaseRNSDecompositionVectorSize(levelQ, levelP)
	for i := 0; i < BaseRNSDecompositionVectorSize; i++ {
		eval.DecomposeSingleNTT(levelQ, levelP, nbPi, i, polyNTT, polyInvNTT, decompQP[i].Q, decompQP[i].P)
	}
}

// DecomposeSingleNTT takes the input polynomial c2 (c2NTT and c2InvNTT, respectively in the NTT and out of the NTT domain)
// modulo the RNS basis, and returns the result on c2QiQ and c2QiP, the receiver polynomials respectively mod Q and mod P (in the NTT domain)
func (eval Evaluator) DecomposeSingleNTT(levelQ, levelP, nbPi, BaseRNSDecompositionVectorSize int, c2NTT, c2InvNTT, c2QiQ, c2QiP ring.Poly) {

	if levelP == 0 && nbPi == 1 {
		eval.decomposeSinglePNoBaseTwoNTT(levelQ, BaseRNSDecompositionVectorSize, c2QiQ, c2QiP, c2NTT, c2InvNTT, false)
		return
	}

	ringQ := eval.params.RingQ().AtLevel(levelQ)
	ringP := eval.params.RingP().AtLevel(levelP)

	eval.Decomposer.DecomposeAndSplit(levelQ, levelP, nbPi, BaseRNSDecompositionVectorSize, c2InvNTT, c2QiQ, c2QiP)

	p0idxst := BaseRNSDecompositionVectorSize * nbPi
	p0idxed := p0idxst + nbPi

	// c2_qi = cx mod qi mod qi
	for x := 0; x < levelQ+1; x++ {
		if p0idxst <= x && x < p0idxed {
			copy(c2QiQ.Coeffs[x], c2NTT.Coeffs[x])
		} else {
			ringQ.SubRings[x].NTT(c2QiQ.Coeffs[x], c2QiQ.Coeffs[x])
		}
	}

	if ringP != nil {
		// c2QiP = c2 mod qi mod pj
		ringP.NTT(c2QiP, c2QiP)
	}
}

/*
type DecompositionBuffer [][]ringqp.Poly

func (eval Evaluator) ALlocateDecompositionBuffer(levelQ, levelP, Pow2Base int) (DecompositionBuffer){

	decompQP := make([][]ringqp.Poly, BaseRNSDecompositionVectorSize)
	for i := 0; i < BaseRNSDecompositionVectorSize; i++ {

		for j := 0; j < BaseTwoDecompositionVectorSize; j++{
			DecompositionBuffer[i][j] = ringQP.NewPoly()
		}
	}

	return decompQPs
}
*/
