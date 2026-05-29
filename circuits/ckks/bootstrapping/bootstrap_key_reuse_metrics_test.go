package bootstrapping

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/tuneinsight/lattigo/v6/circuits/ckks/dft"
	"github.com/tuneinsight/lattigo/v6/ring/ringqp"
	"github.com/tuneinsight/lattigo/v6/schemes/ckks"
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

func collectBootstrapKeyReuseMetrics(planID string, spec bkrCaseSpec, targetLevel int, keys *EvaluationKeys, eval *Evaluator, baseline bkrMaterialBaselineDetails) bkrMaterialMetrics {
	return bkrMaterialMetrics{
		RecordType:                    "MaterialMetrics",
		SchemaVersion:                 bkrSchemaVersion,
		PlanID:                        planID,
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
		RotationSetHash:               baseline.Index.RotationSetHash,
		C2SScheduleHash:               baseline.Index.C2SScheduleHash,
		S2CScheduleHash:               baseline.Index.S2CScheduleHash,
		EncodedDiagonalSetHash:        baseline.Index.EncodedDiagonalSetHash,
	}
}

func collectBootstrapKeyReuseBaselineDetails(planID string, spec bkrCaseSpec, targetLevel int, btpParams Parameters, keys *EvaluationKeys, eval *Evaluator) (bkrMaterialBaselineDetails, error) {
	fullParams, err := ckks.NewParametersFromLiteral(spec.SchemeParams)
	if err != nil {
		return bkrMaterialBaselineDetails{}, fmt.Errorf("cannot instantiate full profile parameters for baseline details: %w", err)
	}

	galois := bkrCollectGaloisKeyBaseline(planID, spec, targetLevel, btpParams, keys)

	c2sSchedules := bkrMatrixScheduleRecords(planID, spec, targetLevel, "coeffs_to_slots", eval.C2SDFTMatrix, btpParams.BootstrappingParameters)
	s2cSchedules := bkrMatrixScheduleRecords(planID, spec, targetLevel, "slots_to_coeffs", eval.S2CDFTMatrix, btpParams.BootstrappingParameters)
	schedules := append(c2sSchedules, s2cSchedules...)

	c2sScheduleHash := ""
	if len(c2sSchedules) > 0 {
		c2sScheduleHash = c2sSchedules[0].MatrixScheduleID
	}
	s2cScheduleHash := ""
	if len(s2cSchedules) > 0 {
		s2cScheduleHash = s2cSchedules[0].MatrixScheduleID
	}

	c2sDiagonals, err := bkrEncodedDiagonalRecords(planID, spec, targetLevel, "coeffs_to_slots", eval.C2SDFTMatrix, btpParams.BootstrappingParameters)
	if err != nil {
		return bkrMaterialBaselineDetails{}, err
	}
	s2cDiagonals, err := bkrEncodedDiagonalRecords(planID, spec, targetLevel, "slots_to_coeffs", eval.S2CDFTMatrix, btpParams.BootstrappingParameters)
	if err != nil {
		return bkrMaterialBaselineDetails{}, err
	}
	encodedDiagonals := append(c2sDiagonals, s2cDiagonals...)
	encodedDiagonalIDs := make([]string, 0, len(encodedDiagonals))
	distinctEncodedDiagonalIDs := map[string]bool{}
	for _, record := range encodedDiagonals {
		encodedDiagonalIDs = append(encodedDiagonalIDs, record.EncodedDiagonalID)
		distinctEncodedDiagonalIDs[record.EncodedDiagonalID] = true
	}

	baseline := bkrMaterialBaselineDetails{
		ParameterChain: bkrParameterChainBaseline{
			RecordType:                    "ParameterChainBaseline",
			SchemaVersion:                 bkrSchemaVersion,
			PlanID:                        planID,
			CaseID:                        spec.CaseID,
			ParamsProfile:                 spec.ProfileID,
			TargetLevel:                   targetLevel,
			FullLogN:                      fullParams.LogN(),
			FullMaxLevel:                  fullParams.MaxLevel(),
			FullQCount:                    len(fullParams.Q()),
			FullPCount:                    len(fullParams.P()),
			FullQHash:                     bkrHashUint64List(fullParams.Q()),
			FullPHash:                     bkrHashUint64List(fullParams.P()),
			ResidualLogN:                  btpParams.ResidualParameters.LogN(),
			ResidualMaxLevel:              btpParams.ResidualParameters.MaxLevel(),
			ResidualQCount:                len(btpParams.ResidualParameters.Q()),
			ResidualPCount:                len(btpParams.ResidualParameters.P()),
			ResidualQPrefixHash:           bkrHashUint64List(btpParams.ResidualParameters.Q()),
			ResidualPHash:                 bkrHashUint64List(btpParams.ResidualParameters.P()),
			BootstrappingLogN:             btpParams.BootstrappingParameters.LogN(),
			BootstrappingMaxLevel:         btpParams.BootstrappingParameters.MaxLevel(),
			BootstrappingQCount:           len(btpParams.BootstrappingParameters.Q()),
			BootstrappingPCount:           len(btpParams.BootstrappingParameters.P()),
			BootstrappingQHash:            bkrHashUint64List(btpParams.BootstrappingParameters.Q()),
			BootstrappingPHash:            bkrHashUint64List(btpParams.BootstrappingParameters.P()),
			FixedTargetScaleLog:           spec.FixedTargetScaleLog,
			ResidualDefaultScaleLog2:      btpParams.ResidualParameters.DefaultScale().Log2(),
			BootstrappingDefaultScaleLog2: btpParams.BootstrappingParameters.DefaultScale().Log2(),
		},
		GaloisKeys:       galois,
		Schedules:        schedules,
		EncodedDiagonals: encodedDiagonals,
		Index: bkrMaterialBaselineIndex{
			RecordType:                   "MaterialBaselineIndex",
			SchemaVersion:                bkrSchemaVersion,
			PlanID:                       planID,
			CaseID:                       spec.CaseID,
			ParamsProfile:                spec.ProfileID,
			TargetLevel:                  targetLevel,
			RotationSetHash:              galois.RotationSetHash,
			C2SScheduleHash:              c2sScheduleHash,
			S2CScheduleHash:              s2cScheduleHash,
			EncodedDiagonalSetHash:       bkrHashStringList(encodedDiagonalIDs),
			GeneratedRotationKeyCount:    galois.GeneratedCount,
			ScheduleRecordCount:          len(schedules),
			EncodedDiagonalRecordCount:   len(encodedDiagonals),
			DistinctEncodedDiagonalCount: len(distinctEncodedDiagonalIDs),
		},
	}

	return baseline, nil
}

func completeBootstrapKeyReuseBaselineIndex(baseline *bkrMaterialBaselineDetails, material bkrMaterialMetrics) {
	baseline.Index.MetricsGeneratedRotationKeyCount = material.GeneratedRotationKeyCount + material.SharedRotationKeys
	baseline.Index.MetricsGeneratedEncodedDiagonalCount = material.GeneratedEncodedDiagonalCount + material.SharedEncodedDiagonals
	baseline.Index.RotationCountMatchesMetrics = baseline.Index.GeneratedRotationKeyCount == baseline.Index.MetricsGeneratedRotationKeyCount
	baseline.Index.EncodedDiagonalCountMatchesMetrics = baseline.Index.EncodedDiagonalRecordCount == baseline.Index.MetricsGeneratedEncodedDiagonalCount
}

func bkrCollectGaloisKeyBaseline(planID string, spec bkrCaseSpec, targetLevel int, btpParams Parameters, keys *EvaluationKeys) bkrGaloisKeyBaseline {
	generated := bkrSortedGaloisElements(keys)
	required := append([]uint64(nil), btpParams.GaloisElements(btpParams.BootstrappingParameters)...)
	sort.Slice(required, func(i, j int) bool { return required[i] < required[j] })

	canonicalIDs := make([]string, len(generated))
	discreteLogK := make([]int, len(generated))
	for i, galEl := range generated {
		canonicalIDs[i] = "gal:" + strconv.FormatUint(galEl, 10)
		discreteLogK[i] = btpParams.BootstrappingParameters.GetRLWEParameters().SolveDiscreteLogGaloisElement(galEl)
	}

	conjugation := btpParams.BootstrappingParameters.GaloisElementForComplexConjugation()

	return bkrGaloisKeyBaseline{
		RecordType:                          "GaloisKeyBaseline",
		SchemaVersion:                       bkrSchemaVersion,
		PlanID:                              planID,
		CaseID:                              spec.CaseID,
		ParamsProfile:                       spec.ProfileID,
		TargetLevel:                         targetLevel,
		GeneratedGaloisElementsSorted:       generated,
		RequiredBootstrappingGaloisElements: required,
		CanonicalRotationIDs:                canonicalIDs,
		DiscreteLogK:                        discreteLogK,
		ContainsComplexConjugation:          bkrUint64Contains(generated, conjugation),
		GeneratedCount:                      len(generated),
		RequiredCount:                       len(required),
		MissingRequiredGaloisElements:       bkrUint64Difference(required, generated),
		ExtraGeneratedGaloisElements:        bkrUint64Difference(generated, required),
		RotationSetHash:                     bkrHashUint64List(generated),
	}
}

func bkrSortedGaloisElements(keys *EvaluationKeys) []uint64 {
	if keys == nil || keys.MemEvaluationKeySet == nil {
		return []uint64{}
	}
	galEls := append([]uint64(nil), keys.GetGaloisKeysList()...)
	sort.Slice(galEls, func(i, j int) bool { return galEls[i] < galEls[j] })
	return galEls
}

func bkrMatrixScheduleRecords(planID string, spec bkrCaseSpec, targetLevel int, matrixName string, matrix dft.Matrix, params ckks.Parameters) []bkrLinearTransformScheduleBaseline {
	records := make([]bkrLinearTransformScheduleBaseline, 0, len(matrix.Matrices))
	transformIDs := make([]string, 0, len(matrix.Matrices))

	for i, lt := range matrix.Matrices {
		diagonalIndices := bkrSortedIntKeys(lt.Vec)
		galEls := append([]uint64(nil), lt.GaloisElements(params)...)
		sort.Slice(galEls, func(i, j int) bool { return galEls[i] < galEls[j] })

		scaleString := lt.Scale.Value.Text('g', -1)
		transformID := bkrHashStrings(
			"linear-transform-schedule/v1",
			matrixName,
			strconv.Itoa(i),
			bkrDFTTypeName(matrix.Type),
			strconv.Itoa(int(matrix.Type)),
			bkrDFTFormatName(matrix.Format),
			strconv.Itoa(int(matrix.Format)),
			strconv.Itoa(matrix.LogSlots),
			strconv.Itoa(matrix.LevelQ),
			strconv.Itoa(matrix.LevelP),
			bkrJoinInts(matrix.Levels),
			strconv.FormatBool(matrix.BitReversed),
			strconv.Itoa(matrix.LogBSGSRatio),
			strconv.Itoa(lt.LevelQ),
			strconv.Itoa(lt.LevelP),
			strconv.Itoa(lt.N1),
			strconv.Itoa(lt.LogDimensions.Rows),
			strconv.Itoa(lt.LogDimensions.Cols),
			scaleString,
			bkrJoinInts(diagonalIndices),
			bkrJoinUint64s(galEls),
		)

		transformIDs = append(transformIDs, transformID)
		records = append(records, bkrLinearTransformScheduleBaseline{
			RecordType:          "LinearTransformScheduleBaseline",
			SchemaVersion:       bkrSchemaVersion,
			PlanID:              planID,
			CaseID:              spec.CaseID,
			ParamsProfile:       spec.ProfileID,
			TargetLevel:         targetLevel,
			MatrixName:          matrixName,
			TransformIndex:      i,
			DFTType:             bkrDFTTypeName(matrix.Type),
			DFTTypeValue:        int(matrix.Type),
			DFTFormat:           bkrDFTFormatName(matrix.Format),
			DFTFormatValue:      int(matrix.Format),
			DFTLogSlots:         matrix.LogSlots,
			DFTLevelQ:           matrix.LevelQ,
			DFTLevelP:           matrix.LevelP,
			DFTLevels:           append([]int(nil), matrix.Levels...),
			DFTBitReversed:      matrix.BitReversed,
			DFTLogBSGSRatio:     matrix.LogBSGSRatio,
			LTLevelQ:            lt.LevelQ,
			LTLevelP:            lt.LevelP,
			LTN1:                lt.N1,
			LTLogRows:           lt.LogDimensions.Rows,
			LTLogCols:           lt.LogDimensions.Cols,
			LTScale:             scaleString,
			LTScaleLog2:         lt.Scale.Log2(),
			DiagonalIndices:     diagonalIndices,
			GaloisElements:      galEls,
			TransformScheduleID: transformID,
		})
	}

	matrixScheduleID := bkrHashStrings(
		"matrix-schedule/v1",
		matrixName,
		bkrDFTTypeName(matrix.Type),
		strconv.Itoa(int(matrix.Type)),
		bkrDFTFormatName(matrix.Format),
		strconv.Itoa(int(matrix.Format)),
		strconv.Itoa(matrix.LogSlots),
		strconv.Itoa(matrix.LevelQ),
		strconv.Itoa(matrix.LevelP),
		bkrJoinInts(matrix.Levels),
		strconv.FormatBool(matrix.BitReversed),
		strconv.Itoa(matrix.LogBSGSRatio),
		bkrJoinStrings(transformIDs),
	)

	for i := range records {
		records[i].MatrixScheduleID = matrixScheduleID
	}

	return records
}

func bkrEncodedDiagonalRecords(planID string, spec bkrCaseSpec, targetLevel int, matrixName string, matrix dft.Matrix, params ckks.Parameters) ([]bkrEncodedDiagonalBaseline, error) {
	records := []bkrEncodedDiagonalBaseline{}

	for transformIndex, lt := range matrix.Matrices {
		diagonalIndices := bkrSortedIntKeys(lt.Vec)
		for _, diagonalIndex := range diagonalIndices {
			poly := lt.Vec[diagonalIndex]
			polyHash, err := bkrHashPoly(poly)
			if err != nil {
				return nil, fmt.Errorf("cannot hash encoded diagonal matrix=%s transform=%d diagonal=%d: %w", matrixName, transformIndex, diagonalIndex, err)
			}

			levelQ := poly.LevelQ()
			levelP := poly.LevelP()
			qPrefixHash := bkrHashUint64Prefix(params.Q(), levelQ)
			pPrefixHash := bkrHashUint64Prefix(params.P(), levelP)
			n := poly.Q.N()
			if n == 0 {
				n = poly.P.N()
			}

			encodedDiagonalID := bkrHashStrings(
				"encoded-diagonal/v1",
				strconv.Itoa(n),
				strconv.Itoa(levelQ),
				strconv.Itoa(levelP),
				qPrefixHash,
				pPrefixHash,
				polyHash,
			)

			records = append(records, bkrEncodedDiagonalBaseline{
				RecordType:        "EncodedDiagonalBaseline",
				SchemaVersion:     bkrSchemaVersion,
				PlanID:            planID,
				CaseID:            spec.CaseID,
				ParamsProfile:     spec.ProfileID,
				TargetLevel:       targetLevel,
				MatrixName:        matrixName,
				TransformIndex:    transformIndex,
				DiagonalIndex:     diagonalIndex,
				LevelQ:            levelQ,
				LevelP:            levelP,
				N:                 n,
				QPrefixHash:       qPrefixHash,
				PPrefixHash:       pPrefixHash,
				PolyBinarySize:    int64(poly.BinarySize()),
				PolySHA256:        polyHash,
				EncodedDiagonalID: encodedDiagonalID,
			})
		}
	}

	return records, nil
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

func bkrHashPoly(poly ringqp.Poly) (string, error) {
	hasher := sha256.New()
	if _, err := poly.WriteTo(hasher); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func bkrHashUint64Prefix(values []uint64, level int) string {
	if level < 0 {
		return ""
	}
	if level >= len(values) {
		level = len(values) - 1
	}
	return bkrHashUint64List(values[:level+1])
}

func bkrHashUint64List(values []uint64) string {
	hasher := sha256.New()
	_, _ = hasher.Write([]byte("uint64-list/v1"))
	var buf [8]byte
	for _, value := range values {
		binary.LittleEndian.PutUint64(buf[:], value)
		_, _ = hasher.Write(buf[:])
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil))
}

func bkrHashStringList(values []string) string {
	copied := append([]string(nil), values...)
	sort.Strings(copied)
	return bkrHashStrings(append([]string{"string-list/v1"}, copied...)...)
}

func bkrHashStrings(values ...string) string {
	hasher := sha256.New()
	for _, value := range values {
		_, _ = hasher.Write([]byte(strconv.Itoa(len(value))))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(value))
		_, _ = hasher.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil))
}

func bkrSortedIntKeys[V any](m map[int]V) []int {
	keys := make([]int, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	return keys
}

func bkrUint64Contains(values []uint64, want uint64) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func bkrUint64Difference(left, right []uint64) []uint64 {
	rightSet := make(map[uint64]bool, len(right))
	for _, value := range right {
		rightSet[value] = true
	}

	diff := []uint64{}
	for _, value := range left {
		if !rightSet[value] {
			diff = append(diff, value)
		}
	}
	return diff
}

func bkrDFTTypeName(t dft.Type) string {
	switch t {
	case dft.HomomorphicEncode:
		return "homomorphic_encode"
	case dft.HomomorphicDecode:
		return "homomorphic_decode"
	default:
		return "unknown_" + strconv.Itoa(int(t))
	}
}

func bkrDFTFormatName(f dft.Format) string {
	switch f {
	case dft.Standard:
		return "standard"
	case dft.SplitRealAndImag:
		return "split_real_and_imag"
	case dft.RepackImagAsReal:
		return "repack_imag_as_real"
	default:
		return "unknown_" + strconv.Itoa(int(f))
	}
}
