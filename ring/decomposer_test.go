package ring

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	"github.com/tuneinsight/lattigo/v6/utils/bignum"
)

func testDecomposeAndSplit(tc *testParams, t *testing.T) {

	levelQMax := tc.ringQ.MaxLevel()
	levelPMax := tc.ringP.MaxLevel()

	for _, tcParams := range []struct {
		name   string
		levelQ int
		levelP int
		nbPi   int
		blocks []int
	}{
		{"Simple", levelQMax, 0, 1, []int{0, levelQMax}},
		{"Reconstruct/NbPi=2", levelQMax, 1, 2, []int{0, levelQMax / 2}},
		{"Reconstruct/NbPi=3", levelQMax, 2, 3, []int{0, levelQMax / 3}},
		{"Reconstruct/MaxP", levelQMax, levelPMax, levelPMax + 1, []int{0}},
		{"SimpleTail/NbPi=2", levelQMax - 1, 1, 2, []int{levelQMax / 2}},
		{"SimpleTail/NbPi=3", levelQMax - 1, 2, 3, []int{(levelQMax - 1) / 3}},
	} {
		t.Run(testString("DecomposeAndSplit/"+tcParams.name, tc.ringQ), func(t *testing.T) {

			ringQ := tc.ringQ.AtLevel(tcParams.levelQ)
			ringP := tc.ringP.AtLevel(tcParams.levelP)
			decomposer := NewDecomposer(tc.ringQ, tc.ringP)

			p0Q := tc.uniformSamplerQ.ReadNew()

			for _, block := range tcParams.blocks {
				haveQ := ringQ.NewPoly()
				haveP := ringP.NewPoly()
				wantQ := ringQ.NewPoly()
				wantP := ringP.NewPoly()

				decomposer.DecomposeAndSplit(tcParams.levelQ, tcParams.levelP, tcParams.nbPi, block, p0Q, haveQ, haveP)
				decomposeAndSplitReference(decomposer, tcParams.levelQ, tcParams.levelP, tcParams.nbPi, block, p0Q, wantQ, wantP)

				require.True(t, ringQ.Equal(haveQ, wantQ))
				require.True(t, ringP.Equal(haveP, wantP))
			}
		})
	}
}

func decomposeAndSplitReference(decomposer *Decomposer, levelQ, levelP, nbPi, BaseRNSDecompositionVectorSize int, p0Q, p1Q, p1P Poly) {

	ringQ := decomposer.ringQ.AtLevel(levelQ)

	var ringP *Ring
	if decomposer.ringP != nil {
		ringP = decomposer.ringP.AtLevel(levelP)
	}

	N := ringQ.N()

	lvlQStart := BaseRNSDecompositionVectorSize * nbPi

	var decompLvl int
	if levelQ > nbPi*(BaseRNSDecompositionVectorSize+1)-1 {
		decompLvl = nbPi - 2
	} else {
		decompLvl = (levelQ % nbPi) - 1
	}

	if decompLvl < 0 {

		var pos, neg, coeff, tmp uint64

		Q := ringQ.ModuliChain()
		BRCQ := ringQ.BRedConstants()

		var P []uint64
		var BRCP [][2]uint64

		if ringP != nil {
			P = ringP.ModuliChain()
			BRCP = ringP.BRedConstants()
		}

		for j := 0; j < N; j++ {

			coeff = p0Q.Coeffs[lvlQStart][j]
			pos, neg = 1, 0
			if coeff >= (Q[lvlQStart] >> 1) {
				coeff = Q[lvlQStart] - coeff
				pos, neg = 0, 1
			}

			for i := 0; i < levelQ+1; i++ {
				tmp = BRedAdd(coeff, Q[i], BRCQ[i])
				p1Q.Coeffs[i][j] = tmp*pos + (Q[i]-tmp)*neg
			}

			if ringP != nil {
				for i := 0; i < levelP+1; i++ {
					tmp = BRedAdd(coeff, P[i], BRCP[i])
					p1P.Coeffs[i][j] = tmp*pos + (P[i]-tmp)*neg
				}
			}
		}

		return
	}

	p0idxst := BaseRNSDecompositionVectorSize * nbPi
	p0idxed := p0idxst + nbPi

	if p0idxed > levelQ+1 {
		p0idxed = levelQ + 1
	}

	MUC := decomposer.ModUpConstants[nbPi-2][BaseRNSDecompositionVectorSize][decompLvl]

	var v, rlo, rhi [8]uint64
	var vi [8]float64
	var y0, y1, y2, y3, y4, y5, y6, y7 [32]uint64

	Q := ringQ.ModuliChain()
	P := ringP.ModuliChain()
	mredQ := ringQ.MRedConstants()
	mredP := ringP.MRedConstants()
	qoverqiinvqi := MUC.qoverqiinvqi
	vtimesqmodp := MUC.vtimesqmodp
	qoverqimodp := MUC.qoverqimodp

	QBig := bignum.NewInt(1)
	for i := p0idxst; i < p0idxed; i++ {
		QBig.Mul(QBig, bignum.NewInt(Q[i]))
	}

	QHalf := bignum.NewInt(QBig)
	QHalf.Rsh(QHalf, 1)
	QHalfModqi := make([]uint64, p0idxed-p0idxst)
	tmp := bignum.NewInt(0)
	for i, j := 0, p0idxst; j < p0idxed; i, j = i+1, j+1 {
		QHalfModqi[i] = tmp.Mod(QHalf, bignum.NewInt(Q[j])).Uint64()
	}

	for x := 0; x < N; x = x + 8 {

		reconstructRNSCentered(p0idxst, p0idxed, x, p0Q.Coeffs, &v, &vi, &y0, &y1, &y2, &y3, &y4, &y5, &y6, &y7, QHalfModqi, Q, mredQ, qoverqiinvqi)

		for j := 0; j < p0idxst; j++ {
			/* #nosec G103 -- test reference mirrors production behavior */
			multSum(decompLvl+1, (*[8]uint64)(unsafe.Pointer(&p1Q.Coeffs[j][x])), &rlo, &rhi, &v, &y0, &y1, &y2, &y3, &y4, &y5, &y6, &y7, Q[j], mredQ[j], vtimesqmodp[j], qoverqimodp[j])
		}

		for j := p0idxed; j < levelQ+1; j++ {
			/* #nosec G103 -- test reference mirrors production behavior */
			multSum(decompLvl+1, (*[8]uint64)(unsafe.Pointer(&p1Q.Coeffs[j][x])), &rlo, &rhi, &v, &y0, &y1, &y2, &y3, &y4, &y5, &y6, &y7, Q[j], mredQ[j], vtimesqmodp[j], qoverqimodp[j])
		}

		for j, u := 0, len(Q); j < levelP+1; j, u = j+1, u+1 {
			/* #nosec G103 -- test reference mirrors production behavior */
			multSum(decompLvl+1, (*[8]uint64)(unsafe.Pointer(&p1P.Coeffs[j][x])), &rlo, &rhi, &v, &y0, &y1, &y2, &y3, &y4, &y5, &y6, &y7, P[j], mredP[j], vtimesqmodp[u], qoverqimodp[u])
		}
	}

	ringQ.SubScalarBigint(p1Q, QHalf, p1Q)
	ringP.SubScalarBigint(p1P, QHalf, p1P)
}
