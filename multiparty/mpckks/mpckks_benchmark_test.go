package mpckks

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tuneinsight/lattigo/v6/core/rlwe"
	"github.com/tuneinsight/lattigo/v6/multiparty"
	"github.com/tuneinsight/lattigo/v6/ring"
	"github.com/tuneinsight/lattigo/v6/schemes/ckks"
	"github.com/tuneinsight/lattigo/v6/utils"
	"github.com/tuneinsight/lattigo/v6/utils/bignum"
)

func BenchmarkMultiPartyCKKS(b *testing.B) {

	var err error

	var testParams []ckks.ParametersLiteral
	switch {
	case *flagParamString != "": // the custom test suite reads the parameters from the -params flag
		testParams = append(testParams, ckks.ParametersLiteral{})
		if err = json.Unmarshal([]byte(*flagParamString), &testParams[0]); err != nil {
			b.Fatal(err)
		}
	default:
		testParams = testParamsLiteral
	}

	for _, ringType := range []ring.Type{ring.Standard, ring.ConjugateInvariant} {

		for _, paramsLiteral := range testParams {

			paramsLiteral.RingType = ringType

			var params ckks.Parameters
			if params, err = ckks.NewParametersFromLiteral(paramsLiteral); err != nil {
				b.Fatal(err)
			}
			N := 3
			var tc *testContext
			if tc, err = genTestParams(params, N); err != nil {
				b.Fatal(err)
			}

			benchCiphertextMul(tc, b)
			benchRefresh(tc, b)
			benchMaskedTransform(tc, b)
		}
	}
}

// benchCiphertextMul benchmarks the online evaluator part of multi-party CKKS
// ciphertext-ciphertext multiplication.
//
// Input: two degree-1 ciphertexts encrypting random complex vectors under the
// collective public key, at MaxLevel, DefaultScale, and LogMaxSlots.
//
// Output:
//   - Mul returns a degree-2 ciphertext without relinearization.
//   - Relinearize returns a degree-1 ciphertext from a precomputed degree-2 input.
//   - MulRelin returns a degree-1 ciphertext through the fused multiply/relinearize path.
//   - MulThenRelinearize returns a degree-1 ciphertext through the split path.
//
// Encryption parameters: the existing mpckks benchmark matrix is reused: the
// default testParamsLiteral or the -params override, both supported ring types,
// and the benchmark party count from the test context.
//
// Flow: collective RKG is performed before timing and only the evaluator call
// inside each sub-benchmark is measured.
func benchCiphertextMul(tc *testContext, b *testing.B) {

	params := tc.params
	logSlots := params.LogMaxSlots()
	level := params.MaxLevel()

	_, _, ciphertext0 := newTestVectors(tc, tc.encryptorPk0, -1-1i, 1+1i, logSlots)
	_, _, ciphertext1 := newTestVectors(tc, tc.encryptorPk0, -1-1i, 1+1i, logSlots)

	evkParams := rlwe.EvaluationKeyParameters{
		LevelQ: utils.Pointy(params.MaxLevelQ()),
		LevelP: utils.Pointy(params.MaxLevelP()),
	}

	evk := rlwe.NewMemEvaluationKeySet(genCollectiveRelinearizationKey(tc, evkParams))
	eval := tc.evaluator.WithKey(evk)

	b.Run(GetTestName("Evaluator/Mul/Ciphertext/Ciphertext", tc.NParties, params), func(b *testing.B) {
		b.ReportAllocs()

		receiver := ckks.NewCiphertext(params, 2, level)
		require.NoError(b, eval.Mul(ciphertext0, ciphertext1, receiver))
		require.Equal(b, 2, receiver.Degree())

		b.ResetTimer()
		reportCiphertextMulParameters(b, tc, 1, 2, level, logSlots)

		for i := 0; i < b.N; i++ {
			if err := eval.Mul(ciphertext0, ciphertext1, receiver); err != nil {
				b.Fatal(err)
			}
		}
	})

	product := ckks.NewCiphertext(params, 2, level)
	require.NoError(b, eval.Mul(ciphertext0, ciphertext1, product))

	b.Run(GetTestName("Evaluator/Relinearize/CiphertextDegree2", tc.NParties, params), func(b *testing.B) {
		b.ReportAllocs()

		receiver := ckks.NewCiphertext(params, 1, level)
		require.NoError(b, eval.Relinearize(product, receiver))
		require.Equal(b, 1, receiver.Degree())

		b.ResetTimer()
		reportCiphertextMulParameters(b, tc, 2, 1, level, logSlots)

		for i := 0; i < b.N; i++ {
			if err := eval.Relinearize(product, receiver); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run(GetTestName("Evaluator/MulRelin/Ciphertext/Ciphertext", tc.NParties, params), func(b *testing.B) {
		b.ReportAllocs()

		receiver := ckks.NewCiphertext(params, 1, level)
		require.NoError(b, eval.MulRelin(ciphertext0, ciphertext1, receiver))
		require.Equal(b, 1, receiver.Degree())

		b.ResetTimer()
		reportCiphertextMulParameters(b, tc, 1, 1, level, logSlots)

		for i := 0; i < b.N; i++ {
			if err := eval.MulRelin(ciphertext0, ciphertext1, receiver); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run(GetTestName("Evaluator/MulThenRelinearize/Ciphertext/Ciphertext", tc.NParties, params), func(b *testing.B) {
		b.ReportAllocs()

		product := ckks.NewCiphertext(params, 2, level)
		receiver := ckks.NewCiphertext(params, 1, level)
		require.NoError(b, eval.Mul(ciphertext0, ciphertext1, product))
		require.NoError(b, eval.Relinearize(product, receiver))
		require.Equal(b, 1, receiver.Degree())

		b.ResetTimer()
		reportCiphertextMulParameters(b, tc, 1, 1, level, logSlots)

		for i := 0; i < b.N; i++ {
			if err := eval.Mul(ciphertext0, ciphertext1, product); err != nil {
				b.Fatal(err)
			}

			if err := eval.Relinearize(product, receiver); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run(GetTestName("EvaluatorParallel/MulRelin/Ciphertext/Ciphertext", tc.NParties, params), func(b *testing.B) {
		b.ReportAllocs()

		receiver := ckks.NewCiphertext(params, 1, level)
		require.NoError(b, eval.MulRelin(ciphertext0, ciphertext1, receiver))
		require.Equal(b, 1, receiver.Degree())

		b.ResetTimer()
		reportCiphertextMulParameters(b, tc, 1, 1, level, logSlots)

		b.RunParallel(func(pb *testing.PB) {
			eval := ckks.NewEvaluator(params, evk)
			receiver := ckks.NewCiphertext(params, 1, level)

			for pb.Next() {
				if err := eval.MulRelin(ciphertext0, ciphertext1, receiver); err != nil {
					b.Log(err)
					b.Fail()
				}
			}
		})
	})
}

func genCollectiveRelinearizationKey(tc *testContext, evkParams rlwe.EvaluationKeyParameters) *rlwe.RelinearizationKey {
	rkg := multiparty.NewRelinearizationKeyGenProtocol(tc.params)
	crp := rkg.SampleCRP(tc.crs, evkParams)

	ephSk := make([]*rlwe.SecretKey, tc.NParties)
	shareRoundOne := make([]multiparty.RelinearizationKeyGenShare, tc.NParties)
	shareRoundTwo := make([]multiparty.RelinearizationKeyGenShare, tc.NParties)

	for i := 0; i < tc.NParties; i++ {
		ephSk[i], shareRoundOne[i], shareRoundTwo[i] = rkg.AllocateShare(evkParams)
	}

	for i := 0; i < tc.NParties; i++ {
		rkg.GenShareRoundOne(tc.sk0Shards[i], crp, ephSk[i], &shareRoundOne[i])
		if i != 0 {
			rkg.AggregateShares(shareRoundOne[0], shareRoundOne[i], &shareRoundOne[0])
		}
	}

	for i := 0; i < tc.NParties; i++ {
		rkg.GenShareRoundTwo(ephSk[i], tc.sk0Shards[i], shareRoundOne[0], &shareRoundTwo[i])
		if i != 0 {
			rkg.AggregateShares(shareRoundTwo[0], shareRoundTwo[i], &shareRoundTwo[0])
		}
	}

	rlk := rlwe.NewRelinearizationKey(tc.params, evkParams)
	rkg.GenRelinearizationKey(shareRoundOne[0], shareRoundTwo[0], rlk)

	return rlk
}

func reportCiphertextMulParameters(b *testing.B, tc *testContext, inputDegree, outputDegree, level, logSlots int) {
	params := tc.params
	slots := 1 << logSlots

	b.ReportMetric(float64(tc.NParties), "parties")
	b.ReportMetric(float64(params.LogN()), "logN")
	b.ReportMetric(float64(level), "level")
	b.ReportMetric(float64(slots), "slots")
	b.ReportMetric(float64(params.QCount()), "Qi")
	b.ReportMetric(float64(params.PCount()), "Pi")
	b.ReportMetric(float64(inputDegree), "input_degree")
	b.ReportMetric(float64(outputDegree), "output_degree")
}

func benchRefresh(tc *testContext, b *testing.B) {

	params := tc.params

	minLevel, logBound, ok := GetMinimumLevelForRefresh(128, params.DefaultScale(), tc.NParties, params.Q())

	if ok {

		sk0Shards := tc.sk0Shards

		type Party struct {
			RefreshProtocol
			s     *rlwe.SecretKey
			share multiparty.RefreshShare
		}

		p := new(Party)
		var err error
		p.RefreshProtocol, err = NewRefreshProtocol(params, logBound, params.Xe())
		require.NoError(b, err)
		p.s = sk0Shards[0]
		p.share = p.AllocateShare(minLevel, params.MaxLevel())

		ciphertext := ckks.NewCiphertext(params, 1, minLevel)

		crp := p.SampleCRP(params.MaxLevel(), tc.crs)

		b.Run(GetTestName("Refresh/Round1/Gen", tc.NParties, params), func(b *testing.B) {

			for i := 0; i < b.N; i++ {
				p.GenShare(p.s, logBound, ciphertext, crp, &p.share)
			}
		})

		b.Run(GetTestName("Refresh/Round1/Agg", tc.NParties, params), func(b *testing.B) {

			for i := 0; i < b.N; i++ {
				p.AggregateShares(&p.share, &p.share, &p.share)
			}
		})

		b.Run(GetTestName("Refresh/Finalize", tc.NParties, params), func(b *testing.B) {
			opOut := ckks.NewCiphertext(params, 1, params.MaxLevel())
			for i := 0; i < b.N; i++ {
				p.Finalize(ciphertext, crp, p.share, opOut)
			}
		})

	} else {
		b.Log("bench skipped : not enough level to ensure correctness and 128 bit security")
	}
}

func benchMaskedTransform(tc *testContext, b *testing.B) {

	params := tc.params

	minLevel, logBound, ok := GetMinimumLevelForRefresh(128, params.DefaultScale(), tc.NParties, params.Q())

	if ok {

		sk0Shards := tc.sk0Shards

		type Party struct {
			MaskedLinearTransformationProtocol
			s     *rlwe.SecretKey
			share multiparty.RefreshShare
		}

		ciphertext := ckks.NewCiphertext(params, 1, minLevel)

		p := new(Party)
		p.MaskedLinearTransformationProtocol, _ = NewMaskedLinearTransformationProtocol(params, params, logBound, params.Xe())
		p.s = sk0Shards[0]
		p.share = p.AllocateShare(ciphertext.Level(), params.MaxLevel())

		crp := p.SampleCRP(params.MaxLevel(), tc.crs)

		transform := &MaskedLinearTransformationFunc{
			Decode: true,
			Func: func(coeffs []*bignum.Complex) {
				for i := range coeffs {
					coeffs[i][0].Mul(coeffs[i][0], bignum.NewFloat(0.9238795325112867, logBound))
					coeffs[i][1].Mul(coeffs[i][1], bignum.NewFloat(0.7071067811865476, logBound))
				}
			},
			Encode: true,
		}

		b.Run(GetTestName("Refresh&Transform/Round1/Gen", tc.NParties, params), func(b *testing.B) {

			for i := 0; i < b.N; i++ {
				p.GenShare(p.s, p.s, logBound, ciphertext, crp, transform, &p.share)
			}
		})

		b.Run(GetTestName("Refresh&Transform/Round1/Agg", tc.NParties, params), func(b *testing.B) {

			for i := 0; i < b.N; i++ {
				p.AggregateShares(&p.share, &p.share, &p.share)
			}
		})

		b.Run(GetTestName("Refresh&Transform/Transform", tc.NParties, params), func(b *testing.B) {
			opOut := ckks.NewCiphertext(params, 1, params.MaxLevel())
			for i := 0; i < b.N; i++ {
				p.Transform(ciphertext, transform, crp, p.share, opOut)
			}
		})

	} else {
		b.Log("bench skipped : not enough level to ensure correctness and 128 bit security")
	}
}
