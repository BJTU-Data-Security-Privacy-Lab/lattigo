package bootstrapping

import (
	"fmt"
	"sort"
)

type bkrLinearTransformSchedulePool struct {
	planID  string
	spec    bkrCaseSpec
	entries map[string]*bkrLinearTransformSchedulePoolEntry
}

type bkrLinearTransformSchedulePoolEntry struct {
	ownerTarget         int
	consumers           map[int]bool
	record              bkrLinearTransformScheduleBaseline
	scheduleFingerprint string
	transformScheduleID string
	matrixScheduleID    string
	matrixName          string
	transformIndex      int
}

type bkrLinearTransformSchedulePoolRecord struct {
	PlanID               string
	CaseID               string
	ParamsProfile        string
	TransformScheduleID  string
	MatrixScheduleID     string
	MatrixName           string
	TransformIndex       int
	OwnerTargetLevel     int
	ConsumerTargetLevels []int
}

type bkrLinearTransformScheduleViewRecord struct {
	PlanID              string
	CaseID              string
	ParamsProfile       string
	TargetLevel         int
	MatrixName          string
	TransformIndex      int
	TransformScheduleID string
	MatrixScheduleID    string
	OwnerTargetLevel    int
	IsShared            bool
	FallbackReason      string
}

func newBKRLinearTransformSchedulePool(planID string, spec bkrCaseSpec) *bkrLinearTransformSchedulePool {
	return &bkrLinearTransformSchedulePool{
		planID:  planID,
		spec:    spec,
		entries: map[string]*bkrLinearTransformSchedulePoolEntry{},
	}
}

func (p *bkrLinearTransformSchedulePool) acquire(record bkrLinearTransformScheduleBaseline) (bkrLinearTransformScheduleViewRecord, bool, error) {
	if p == nil {
		return bkrLinearTransformScheduleViewRecord{}, false, fmt.Errorf("linear-transform schedule pool is nil")
	}
	if record.TransformScheduleID == "" {
		return bkrLinearTransformScheduleViewRecord{}, false, fmt.Errorf("empty transform_schedule_id for target=%d matrix=%s transform=%d", record.TargetLevel, record.MatrixName, record.TransformIndex)
	}

	fingerprint := bkrLinearTransformScheduleFingerprint(record)
	if entry, ok := p.entries[record.TransformScheduleID]; ok {
		if entry.scheduleFingerprint != fingerprint {
			return bkrLinearTransformScheduleViewRecord{}, false, fmt.Errorf("transform_schedule_id %s maps to incompatible schedule fields", record.TransformScheduleID)
		}
		entry.consumers[record.TargetLevel] = true
		return p.viewRecord(record, entry.ownerTarget, entry.ownerTarget != record.TargetLevel, "none"), false, nil
	}

	p.entries[record.TransformScheduleID] = &bkrLinearTransformSchedulePoolEntry{
		ownerTarget:         record.TargetLevel,
		consumers:           map[int]bool{record.TargetLevel: true},
		record:              record,
		scheduleFingerprint: fingerprint,
		transformScheduleID: record.TransformScheduleID,
		matrixScheduleID:    record.MatrixScheduleID,
		matrixName:          record.MatrixName,
		transformIndex:      record.TransformIndex,
	}
	return p.viewRecord(record, record.TargetLevel, false, "none"), true, nil
}

func (p *bkrLinearTransformSchedulePool) records() []bkrLinearTransformSchedulePoolRecord {
	entries := make([]*bkrLinearTransformSchedulePoolEntry, 0, len(p.entries))
	for _, entry := range p.entries {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].ownerTarget != entries[j].ownerTarget {
			return entries[i].ownerTarget < entries[j].ownerTarget
		}
		if entries[i].matrixName != entries[j].matrixName {
			return entries[i].matrixName < entries[j].matrixName
		}
		if entries[i].transformIndex != entries[j].transformIndex {
			return entries[i].transformIndex < entries[j].transformIndex
		}
		return entries[i].transformScheduleID < entries[j].transformScheduleID
	})

	rows := make([]bkrLinearTransformSchedulePoolRecord, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, bkrLinearTransformSchedulePoolRecord{
			PlanID:               p.planID,
			CaseID:               p.spec.CaseID,
			ParamsProfile:        p.spec.ProfileID,
			TransformScheduleID:  entry.transformScheduleID,
			MatrixScheduleID:     entry.matrixScheduleID,
			MatrixName:           entry.matrixName,
			TransformIndex:       entry.transformIndex,
			OwnerTargetLevel:     entry.ownerTarget,
			ConsumerTargetLevels: bkrSortedIntSet(entry.consumers),
		})
	}
	return rows
}

func (p *bkrLinearTransformSchedulePool) viewRecord(record bkrLinearTransformScheduleBaseline, ownerTarget int, shared bool, fallbackReason string) bkrLinearTransformScheduleViewRecord {
	return bkrLinearTransformScheduleViewRecord{
		PlanID:              p.planID,
		CaseID:              p.spec.CaseID,
		ParamsProfile:       p.spec.ProfileID,
		TargetLevel:         record.TargetLevel,
		MatrixName:          record.MatrixName,
		TransformIndex:      record.TransformIndex,
		TransformScheduleID: record.TransformScheduleID,
		MatrixScheduleID:    record.MatrixScheduleID,
		OwnerTargetLevel:    ownerTarget,
		IsShared:            shared,
		FallbackReason:      fallbackReason,
	}
}

func bkrLinearTransformScheduleFingerprint(record bkrLinearTransformScheduleBaseline) string {
	return bkrHashStrings(
		"linear-transform-schedule-fingerprint/v1",
		record.MatrixName,
		fmt.Sprintf("%d", record.TransformIndex),
		record.DFTType,
		fmt.Sprintf("%d", record.DFTTypeValue),
		record.DFTFormat,
		fmt.Sprintf("%d", record.DFTFormatValue),
		fmt.Sprintf("%d", record.DFTLogSlots),
		fmt.Sprintf("%d", record.DFTLevelQ),
		fmt.Sprintf("%d", record.DFTLevelP),
		bkrJoinInts(record.DFTLevels),
		fmt.Sprintf("%t", record.DFTBitReversed),
		fmt.Sprintf("%d", record.DFTLogBSGSRatio),
		fmt.Sprintf("%d", record.LTLevelQ),
		fmt.Sprintf("%d", record.LTLevelP),
		fmt.Sprintf("%d", record.LTN1),
		fmt.Sprintf("%d", record.LTLogRows),
		fmt.Sprintf("%d", record.LTLogCols),
		record.LTScale,
		bkrJoinInts(record.DiagonalIndices),
		bkrJoinUint64s(record.GaloisElements),
		record.TransformScheduleID,
		record.MatrixScheduleID,
	)
}
