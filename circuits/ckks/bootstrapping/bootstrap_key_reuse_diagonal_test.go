package bootstrapping

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/tuneinsight/lattigo/v6/circuits/ckks/dft"
	"github.com/tuneinsight/lattigo/v6/ring/ringqp"
)

type bkrEncodedDiagonalPool struct {
	planID  string
	spec    bkrCaseSpec
	entries map[string]*bkrEncodedDiagonalPoolEntry
}

type bkrEncodedDiagonalPoolEntry struct {
	ownerTarget       int
	consumers         map[int]bool
	record            bkrEncodedDiagonalBaseline
	fingerprint       string
	encodedDiagonalID string
	diagonal          ringqp.Poly
}

type bkrEncodedDiagonalPoolRecord struct {
	PlanID               string
	CaseID               string
	ParamsProfile        string
	EncodedDiagonalID    string
	MatrixName           string
	TransformIndex       int
	DiagonalIndex        int
	OwnerTargetLevel     int
	ConsumerTargetLevels []int
	LevelQ               int
	LevelP               int
	N                    int
	PolyBinarySize       int64
}

type bkrEncodedDiagonalViewRecord struct {
	PlanID            string
	CaseID            string
	ParamsProfile     string
	TargetLevel       int
	MatrixName        string
	TransformIndex    int
	DiagonalIndex     int
	EncodedDiagonalID string
	OwnerTargetLevel  int
	IsShared          bool
	FallbackReason    string
	Diagonal          ringqp.Poly
}

func newBKREncodedDiagonalPool(planID string, spec bkrCaseSpec) *bkrEncodedDiagonalPool {
	return &bkrEncodedDiagonalPool{
		planID:  planID,
		spec:    spec,
		entries: map[string]*bkrEncodedDiagonalPoolEntry{},
	}
}

func (p *bkrEncodedDiagonalPool) acquire(record bkrEncodedDiagonalBaseline, diagonal ringqp.Poly) (bkrEncodedDiagonalViewRecord, bool, error) {
	if p == nil {
		return bkrEncodedDiagonalViewRecord{}, false, fmt.Errorf("encoded diagonal pool is nil")
	}
	if record.EncodedDiagonalID == "" {
		return bkrEncodedDiagonalViewRecord{}, false, fmt.Errorf("empty encoded_diagonal_id for target=%d matrix=%s transform=%d diagonal=%d", record.TargetLevel, record.MatrixName, record.TransformIndex, record.DiagonalIndex)
	}

	fingerprint := bkrEncodedDiagonalFingerprint(record)
	if entry, ok := p.entries[record.EncodedDiagonalID]; ok {
		if entry.fingerprint != fingerprint {
			return bkrEncodedDiagonalViewRecord{}, false, fmt.Errorf("encoded_diagonal_id %s maps to incompatible encoded diagonal fields", record.EncodedDiagonalID)
		}
		entry.consumers[record.TargetLevel] = true
		return p.viewRecord(record, entry.ownerTarget, true, "none", entry.diagonal), false, nil
	}

	p.entries[record.EncodedDiagonalID] = &bkrEncodedDiagonalPoolEntry{
		ownerTarget:       record.TargetLevel,
		consumers:         map[int]bool{record.TargetLevel: true},
		record:            record,
		fingerprint:       fingerprint,
		encodedDiagonalID: record.EncodedDiagonalID,
		diagonal:          diagonal,
	}
	return p.viewRecord(record, record.TargetLevel, false, "none", diagonal), true, nil
}

func (p *bkrEncodedDiagonalPool) records() []bkrEncodedDiagonalPoolRecord {
	entries := make([]*bkrEncodedDiagonalPoolEntry, 0, len(p.entries))
	for _, entry := range p.entries {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].ownerTarget != entries[j].ownerTarget {
			return entries[i].ownerTarget < entries[j].ownerTarget
		}
		if entries[i].record.MatrixName != entries[j].record.MatrixName {
			return entries[i].record.MatrixName < entries[j].record.MatrixName
		}
		if entries[i].record.TransformIndex != entries[j].record.TransformIndex {
			return entries[i].record.TransformIndex < entries[j].record.TransformIndex
		}
		if entries[i].record.DiagonalIndex != entries[j].record.DiagonalIndex {
			return entries[i].record.DiagonalIndex < entries[j].record.DiagonalIndex
		}
		return entries[i].encodedDiagonalID < entries[j].encodedDiagonalID
	})

	rows := make([]bkrEncodedDiagonalPoolRecord, 0, len(entries))
	for _, entry := range entries {
		record := entry.record
		rows = append(rows, bkrEncodedDiagonalPoolRecord{
			PlanID:               p.planID,
			CaseID:               p.spec.CaseID,
			ParamsProfile:        p.spec.ProfileID,
			EncodedDiagonalID:    entry.encodedDiagonalID,
			MatrixName:           record.MatrixName,
			TransformIndex:       record.TransformIndex,
			DiagonalIndex:        record.DiagonalIndex,
			OwnerTargetLevel:     entry.ownerTarget,
			ConsumerTargetLevels: bkrSortedIntSet(entry.consumers),
			LevelQ:               record.LevelQ,
			LevelP:               record.LevelP,
			N:                    record.N,
			PolyBinarySize:       record.PolyBinarySize,
		})
	}
	return rows
}

func (p *bkrEncodedDiagonalPool) viewRecord(record bkrEncodedDiagonalBaseline, ownerTarget int, shared bool, fallbackReason string, diagonal ringqp.Poly) bkrEncodedDiagonalViewRecord {
	return bkrEncodedDiagonalViewRecord{
		PlanID:            p.planID,
		CaseID:            p.spec.CaseID,
		ParamsProfile:     p.spec.ProfileID,
		TargetLevel:       record.TargetLevel,
		MatrixName:        record.MatrixName,
		TransformIndex:    record.TransformIndex,
		DiagonalIndex:     record.DiagonalIndex,
		EncodedDiagonalID: record.EncodedDiagonalID,
		OwnerTargetLevel:  ownerTarget,
		IsShared:          shared,
		FallbackReason:    fallbackReason,
		Diagonal:          diagonal,
	}
}

func bkrEncodedDiagonalFingerprint(record bkrEncodedDiagonalBaseline) string {
	return bkrHashStrings(
		"encoded-diagonal-fingerprint/v1",
		strconv.Itoa(record.N),
		strconv.Itoa(record.LevelQ),
		strconv.Itoa(record.LevelP),
		record.QPrefixHash,
		record.PPrefixHash,
		strconv.FormatInt(record.PolyBinarySize, 10),
		record.PolySHA256,
		record.EncodedDiagonalID,
	)
}

func bkrInternA4TargetEncodedDiagonals(pool *bkrEncodedDiagonalPool, target *bkrA1PreparedTarget) error {
	if target == nil {
		return fmt.Errorf("cannot intern encoded diagonals for nil target")
	}

	generated := 0
	shared := 0
	persistentMatrixBytes := int64(0)

	for _, record := range target.baseline.EncodedDiagonals {
		diagonal, err := bkrEncodedDiagonalFromEvaluator(target.eval, record)
		if err != nil {
			return err
		}
		view, generatedOwner, err := pool.acquire(record, diagonal)
		if err != nil {
			return err
		}
		if generatedOwner {
			generated++
			persistentMatrixBytes += record.PolyBinarySize
			continue
		}
		if err := bkrSetEncodedDiagonalOnEvaluator(target.eval, record, view.Diagonal); err != nil {
			return err
		}
		shared++
	}

	target.material.GeneratedEncodedDiagonalCount = generated
	target.material.SharedEncodedDiagonals = shared
	target.material.PersistentMatrixBytes = persistentMatrixBytes
	completeBootstrapKeyReuseBaselineIndex(&target.baseline, target.material)

	return nil
}

func bkrEncodedDiagonalFromEvaluator(eval *Evaluator, record bkrEncodedDiagonalBaseline) (ringqp.Poly, error) {
	if eval == nil {
		return ringqp.Poly{}, fmt.Errorf("cannot read encoded diagonal from nil evaluator")
	}

	switch record.MatrixName {
	case "coeffs_to_slots":
		return bkrEncodedDiagonalFromMatrix(eval.C2SDFTMatrix, record)
	case "slots_to_coeffs":
		return bkrEncodedDiagonalFromMatrix(eval.S2CDFTMatrix, record)
	default:
		return ringqp.Poly{}, fmt.Errorf("unknown encoded diagonal matrix_name=%q", record.MatrixName)
	}
}

func bkrSetEncodedDiagonalOnEvaluator(eval *Evaluator, record bkrEncodedDiagonalBaseline, diagonal ringqp.Poly) error {
	if eval == nil {
		return fmt.Errorf("cannot set encoded diagonal on nil evaluator")
	}

	switch record.MatrixName {
	case "coeffs_to_slots":
		return bkrSetEncodedDiagonalOnMatrix(&eval.C2SDFTMatrix, record, diagonal)
	case "slots_to_coeffs":
		return bkrSetEncodedDiagonalOnMatrix(&eval.S2CDFTMatrix, record, diagonal)
	default:
		return fmt.Errorf("unknown encoded diagonal matrix_name=%q", record.MatrixName)
	}
}

func bkrEncodedDiagonalFromMatrix(matrix dft.Matrix, record bkrEncodedDiagonalBaseline) (ringqp.Poly, error) {
	if record.TransformIndex < 0 || record.TransformIndex >= len(matrix.Matrices) {
		return ringqp.Poly{}, fmt.Errorf("matrix=%s target=%d transform_index=%d out of range", record.MatrixName, record.TargetLevel, record.TransformIndex)
	}
	diagonal, ok := matrix.Matrices[record.TransformIndex].Vec[record.DiagonalIndex]
	if !ok {
		return ringqp.Poly{}, fmt.Errorf("matrix=%s target=%d transform=%d missing diagonal_index=%d", record.MatrixName, record.TargetLevel, record.TransformIndex, record.DiagonalIndex)
	}
	return diagonal, nil
}

func bkrSetEncodedDiagonalOnMatrix(matrix *dft.Matrix, record bkrEncodedDiagonalBaseline, diagonal ringqp.Poly) error {
	if matrix == nil {
		return fmt.Errorf("cannot set encoded diagonal on nil matrix")
	}
	if record.TransformIndex < 0 || record.TransformIndex >= len(matrix.Matrices) {
		return fmt.Errorf("matrix=%s target=%d transform_index=%d out of range", record.MatrixName, record.TargetLevel, record.TransformIndex)
	}
	if _, ok := matrix.Matrices[record.TransformIndex].Vec[record.DiagonalIndex]; !ok {
		return fmt.Errorf("matrix=%s target=%d transform=%d missing diagonal_index=%d", record.MatrixName, record.TargetLevel, record.TransformIndex, record.DiagonalIndex)
	}
	matrix.Matrices[record.TransformIndex].Vec[record.DiagonalIndex] = diagonal
	return nil
}

func bkrZeroEncodedDiagonalForTest() ringqp.Poly {
	return ringqp.Poly{}
}
