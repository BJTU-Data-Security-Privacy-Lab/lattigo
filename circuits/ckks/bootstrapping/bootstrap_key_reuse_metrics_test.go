package bootstrapping

import (
	"runtime"
	"sync"
	"time"

	"github.com/tuneinsight/lattigo/v6/circuits/ckks/dft"
)

type bkrHeapSampler struct {
	stop chan struct{}
	done chan struct{}
	mu   sync.Mutex
	max  uint64
}

func newBKRHeapSampler() *bkrHeapSampler {
	s := &bkrHeapSampler{
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	s.sample()
	go s.loop()
	return s
}

func (s *bkrHeapSampler) loop() {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	defer close(s.done)

	for {
		select {
		case <-ticker.C:
			s.sample()
		case <-s.stop:
			s.sample()
			return
		}
	}
}

func (s *bkrHeapSampler) sample() {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	s.mu.Lock()
	if stats.HeapAlloc > s.max {
		s.max = stats.HeapAlloc
	}
	s.mu.Unlock()
}

func (s *bkrHeapSampler) stopAndMax() uint64 {
	close(s.stop)
	<-s.done

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.max
}

func bkrMeasurePhase(fn func() error) (time.Duration, uint64, error) {
	sampler := newBKRHeapSampler()
	start := time.Now()
	err := fn()
	elapsed := time.Since(start)
	peak := sampler.stopAndMax()
	return elapsed, peak, err
}

func collectBootstrapKeyReuseMetrics(spec bkrCaseSpec, targetLevel int, keys *EvaluationKeys, eval *Evaluator) bkrMaterialMetrics {
	return bkrMaterialMetrics{
		RecordType:                    "MaterialMetrics",
		SchemaVersion:                 bkrSchemaVersion,
		PlanID:                        bkrPlanA0,
		CaseID:                        spec.CaseID,
		ParamsProfile:                 spec.ProfileID,
		TargetLevel:                   targetLevel,
		GeneratedEvaluationKeyCount:   bkrEvaluationKeyCount(keys),
		GeneratedRotationKeyCount:     bkrRotationKeyCount(keys),
		GeneratedEncodedDiagonalCount: bkrEncodedDiagonalCount(eval.C2SDFTMatrix) + bkrEncodedDiagonalCount(eval.S2CDFTMatrix),
		PersistentKeyBytes:            int64(keys.BinarySize()),
		PersistentMatrixBytes:         bkrMatrixBytes(eval.C2SDFTMatrix) + bkrMatrixBytes(eval.S2CDFTMatrix),
		SharedRotationKeys:            0,
		SharedEncodedDiagonals:        0,
		RNSSliceSuccess:               "not_applicable",
		FallbackReason:                "none",
	}
}

func bkrRotationKeyCount(keys *EvaluationKeys) int {
	if keys == nil || keys.MemEvaluationKeySet == nil {
		return 0
	}
	return len(keys.GetGaloisKeysList())
}

func bkrEvaluationKeyCount(keys *EvaluationKeys) (count int) {
	if keys == nil {
		return 0
	}
	if keys.EvkN1ToN2 != nil {
		count++
	}
	if keys.EvkN2ToN1 != nil {
		count++
	}
	if keys.EvkRealToCmplx != nil {
		count++
	}
	if keys.EvkCmplxToReal != nil {
		count++
	}
	if keys.EvkDenseToSparse != nil {
		count++
	}
	if keys.EvkSparseToDense != nil {
		count++
	}
	if keys.MemEvaluationKeySet != nil {
		if _, err := keys.GetRelinearizationKey(); err == nil {
			count++
		}
		count += len(keys.GetGaloisKeysList())
	}
	return count
}

func bkrEncodedDiagonalCount(matrix dft.Matrix) (count int) {
	for _, lt := range matrix.Matrices {
		count += len(lt.Vec)
	}
	return count
}

func bkrMatrixBytes(matrix dft.Matrix) (size int64) {
	for _, lt := range matrix.Matrices {
		for _, poly := range lt.Vec {
			size += int64(poly.BinarySize())
		}
	}
	return size
}
