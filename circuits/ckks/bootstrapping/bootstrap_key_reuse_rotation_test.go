package bootstrapping

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/tuneinsight/lattigo/v6/core/rlwe"
)

type bkrRotationKeyDomain struct {
	DomainID             string
	NthRoot              uint64
	LevelQ               int
	LevelP               int
	BaseTwoDecomposition int
	Compressed           bool
}

type bkrRotationKeyPool struct {
	planID       string
	spec         bkrCaseSpec
	entries      map[string]*bkrRotationKeyPoolEntry
	domainsByGal map[uint64]map[string]bool
}

type bkrRotationKeyPoolEntry struct {
	poolID        string
	domain        bkrRotationKeyDomain
	ownerTarget   int
	consumers     map[int]bool
	galoisElement uint64
	key           *rlwe.GaloisKey
	fallback      string
}

func newBKRRotationKeyPool(planID string, spec bkrCaseSpec) *bkrRotationKeyPool {
	return &bkrRotationKeyPool{
		planID:       planID,
		spec:         spec,
		entries:      map[string]*bkrRotationKeyPoolEntry{},
		domainsByGal: map[uint64]map[string]bool{},
	}
}

func (p *bkrRotationKeyPool) acquire(domain bkrRotationKeyDomain, galEl uint64, targetLevel int, generate func() *rlwe.GaloisKey) (*rlwe.GaloisKey, bkrRotationKeyViewRecord, bool, error) {
	if p == nil {
		return nil, bkrRotationKeyViewRecord{}, false, fmt.Errorf("rotation-key pool is nil")
	}

	poolID := bkrRotationKeyPoolID(domain.DomainID, galEl)
	if entry, ok := p.entries[poolID]; ok {
		entry.consumers[targetLevel] = true
		return entry.key, p.viewRecord(targetLevel, galEl, domain.DomainID, poolID, entry.ownerTarget, entry.ownerTarget != targetLevel, true, "none"), false, nil
	}

	fallbackReason := "none"
	domainCompatible := true
	for seenDomain := range p.domainsByGal[galEl] {
		if seenDomain != domain.DomainID {
			fallbackReason = "rotation_domain_mismatch"
			domainCompatible = false
			break
		}
	}

	key := generate()
	if key == nil {
		return nil, bkrRotationKeyViewRecord{}, false, fmt.Errorf("generated nil GaloisKey for gal_el=%d", galEl)
	}
	if key.GaloisElement != galEl {
		return nil, bkrRotationKeyViewRecord{}, false, fmt.Errorf("generated GaloisKey has gal_el=%d, want %d", key.GaloisElement, galEl)
	}
	if key.NthRoot != domain.NthRoot || key.LevelQ() != domain.LevelQ || key.LevelP() != domain.LevelP ||
		key.BaseTwoDecomposition != domain.BaseTwoDecomposition || key.IsCompressed() != domain.Compressed {
		return nil, bkrRotationKeyViewRecord{}, false, fmt.Errorf("generated GaloisKey domain mismatch for gal_el=%d", galEl)
	}

	if p.domainsByGal[galEl] == nil {
		p.domainsByGal[galEl] = map[string]bool{}
	}
	p.domainsByGal[galEl][domain.DomainID] = true

	p.entries[poolID] = &bkrRotationKeyPoolEntry{
		poolID:        poolID,
		domain:        domain,
		ownerTarget:   targetLevel,
		consumers:     map[int]bool{targetLevel: true},
		galoisElement: galEl,
		key:           key,
		fallback:      fallbackReason,
	}

	return key, p.viewRecord(targetLevel, galEl, domain.DomainID, poolID, targetLevel, false, domainCompatible, fallbackReason), true, nil
}

func (p *bkrRotationKeyPool) records() []bkrRotationKeyPoolRecord {
	entries := make([]*bkrRotationKeyPoolEntry, 0, len(p.entries))
	for _, entry := range p.entries {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].ownerTarget != entries[j].ownerTarget {
			return entries[i].ownerTarget < entries[j].ownerTarget
		}
		if entries[i].galoisElement != entries[j].galoisElement {
			return entries[i].galoisElement < entries[j].galoisElement
		}
		return entries[i].domain.DomainID < entries[j].domain.DomainID
	})

	rows := make([]bkrRotationKeyPoolRecord, 0, len(entries))
	for _, entry := range entries {
		status := "exact_generated"
		if entry.fallback != "none" {
			status = "fallback_exact_generated"
		}
		rows = append(rows, bkrRotationKeyPoolRecord{
			RecordType:           "RotationKeyPool",
			SchemaVersion:        bkrSchemaVersion,
			PlanID:               p.planID,
			CaseID:               p.spec.CaseID,
			ParamsProfile:        p.spec.ProfileID,
			PoolID:               entry.poolID,
			DomainID:             entry.domain.DomainID,
			OwnerTargetLevel:     entry.ownerTarget,
			ConsumerTargetLevels: bkrSortedIntSet(entry.consumers),
			GaloisElement:        entry.galoisElement,
			CanonicalRotationID:  bkrCanonicalRotationID(entry.galoisElement),
			NthRoot:              entry.domain.NthRoot,
			LevelQ:               entry.domain.LevelQ,
			LevelP:               entry.domain.LevelP,
			BaseTwoDecomposition: entry.domain.BaseTwoDecomposition,
			Compressed:           entry.domain.Compressed,
			BinarySize:           int64(entry.key.BinarySize()),
			PhysicalKeyGenerated: true,
			Status:               status,
			FallbackReason:       entry.fallback,
		})
	}
	return rows
}

func (p *bkrRotationKeyPool) viewRecord(targetLevel int, galEl uint64, domainID, poolID string, ownerTarget int, shared, compatible bool, fallbackReason string) bkrRotationKeyViewRecord {
	return bkrRotationKeyViewRecord{
		RecordType:          "RotationKeyView",
		SchemaVersion:       bkrSchemaVersion,
		PlanID:              p.planID,
		CaseID:              p.spec.CaseID,
		ParamsProfile:       p.spec.ProfileID,
		TargetLevel:         targetLevel,
		GaloisElement:       galEl,
		CanonicalRotationID: bkrCanonicalRotationID(galEl),
		DomainID:            domainID,
		PoolID:              poolID,
		OwnerTargetLevel:    ownerTarget,
		IsShared:            shared,
		DomainCompatible:    compatible,
		FallbackReason:      fallbackReason,
	}
}

func bkrDefaultRotationKeyRecords(planID string, spec bkrCaseSpec, targetLevel int, btpParams Parameters, keys *EvaluationKeys) ([]bkrRotationKeyPoolRecord, []bkrRotationKeyViewRecord) {
	generated := bkrSortedGaloisElements(keys)
	poolRows := make([]bkrRotationKeyPoolRecord, 0, len(generated))
	for _, galEl := range generated {
		gk, err := keys.GetGaloisKey(galEl)
		if err != nil {
			continue
		}
		domain := bkrRotationKeyDomainFromGaloisKey(spec, btpParams, gk)
		poolRows = append(poolRows, bkrRotationKeyPoolRecord{
			RecordType:           "RotationKeyPool",
			SchemaVersion:        bkrSchemaVersion,
			PlanID:               planID,
			CaseID:               spec.CaseID,
			ParamsProfile:        spec.ProfileID,
			PoolID:               bkrRotationKeyPoolID(domain.DomainID, galEl),
			DomainID:             domain.DomainID,
			OwnerTargetLevel:     targetLevel,
			ConsumerTargetLevels: []int{targetLevel},
			GaloisElement:        galEl,
			CanonicalRotationID:  bkrCanonicalRotationID(galEl),
			NthRoot:              domain.NthRoot,
			LevelQ:               domain.LevelQ,
			LevelP:               domain.LevelP,
			BaseTwoDecomposition: domain.BaseTwoDecomposition,
			Compressed:           domain.Compressed,
			BinarySize:           int64(gk.BinarySize()),
			PhysicalKeyGenerated: true,
			Status:               "exact_generated",
			FallbackReason:       "none",
		})
	}

	required := bkrSortedUniqueUint64(btpParams.GaloisElements(btpParams.BootstrappingParameters))
	viewRows := make([]bkrRotationKeyViewRecord, 0, len(required))
	for _, galEl := range required {
		gk, err := keys.GetGaloisKey(galEl)
		if err != nil {
			viewRows = append(viewRows, bkrRotationKeyViewRecord{
				RecordType:          "RotationKeyView",
				SchemaVersion:       bkrSchemaVersion,
				PlanID:              planID,
				CaseID:              spec.CaseID,
				ParamsProfile:       spec.ProfileID,
				TargetLevel:         targetLevel,
				GaloisElement:       galEl,
				CanonicalRotationID: bkrCanonicalRotationID(galEl),
				OwnerTargetLevel:    -1,
				IsShared:            false,
				DomainCompatible:    false,
				FallbackReason:      "missing_required_galois_key",
			})
			continue
		}
		domain := bkrRotationKeyDomainFromGaloisKey(spec, btpParams, gk)
		viewRows = append(viewRows, bkrRotationKeyViewRecord{
			RecordType:          "RotationKeyView",
			SchemaVersion:       bkrSchemaVersion,
			PlanID:              planID,
			CaseID:              spec.CaseID,
			ParamsProfile:       spec.ProfileID,
			TargetLevel:         targetLevel,
			GaloisElement:       galEl,
			CanonicalRotationID: bkrCanonicalRotationID(galEl),
			DomainID:            domain.DomainID,
			PoolID:              bkrRotationKeyPoolID(domain.DomainID, galEl),
			OwnerTargetLevel:    targetLevel,
			IsShared:            false,
			DomainCompatible:    true,
			FallbackReason:      "none",
		})
	}

	return poolRows, viewRows
}

func bkrRotationKeyDomainFromGaloisKey(spec bkrCaseSpec, btpParams Parameters, gk *rlwe.GaloisKey) bkrRotationKeyDomain {
	return bkrRotationKeyDomainFromParameters(
		spec,
		btpParams,
		gk.NthRoot,
		gk.LevelQ(),
		gk.LevelP(),
		gk.BaseTwoDecomposition,
		gk.IsCompressed(),
	)
}

func bkrRotationKeyDomainFromParameters(spec bkrCaseSpec, btpParams Parameters, nthRoot uint64, levelQ, levelP, baseTwoDecomposition int, compressed bool) bkrRotationKeyDomain {
	qHash := bkrHashUint64List(btpParams.BootstrappingParameters.Q())
	pHash := bkrHashUint64List(btpParams.BootstrappingParameters.P())
	secretDomain := bkrHashStrings("secret-domain/v1", spec.Seed, "bootstrapping-sk")
	domainID := bkrHashStrings(
		"rotation-key-domain/v1",
		spec.ProfileID,
		secretDomain,
		strconv.Itoa(btpParams.BootstrappingParameters.LogN()),
		strconv.FormatUint(nthRoot, 10),
		qHash,
		pHash,
		strconv.Itoa(levelQ),
		strconv.Itoa(levelP),
		strconv.Itoa(baseTwoDecomposition),
		strconv.FormatBool(compressed),
	)

	return bkrRotationKeyDomain{
		DomainID:             domainID,
		NthRoot:              nthRoot,
		LevelQ:               levelQ,
		LevelP:               levelP,
		BaseTwoDecomposition: baseTwoDecomposition,
		Compressed:           compressed,
	}
}

func bkrRotationKeyPoolID(domainID string, galEl uint64) string {
	return bkrHashStrings("rotation-key-pool/v1", domainID, strconv.FormatUint(galEl, 10))
}

func bkrCanonicalRotationID(galEl uint64) string {
	return "gal:" + strconv.FormatUint(galEl, 10)
}

func bkrRotationViewStats(records []bkrRotationKeyViewRecord) (shared int, fallbackReason string) {
	fallbacks := map[string]bool{}
	for _, record := range records {
		if record.IsShared {
			shared++
		}
		if record.FallbackReason != "" && record.FallbackReason != "none" {
			fallbacks[record.FallbackReason] = true
		}
	}
	if len(fallbacks) == 0 {
		return shared, "none"
	}
	reasons := make([]string, 0, len(fallbacks))
	for reason := range fallbacks {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	return shared, bkrJoinStrings(reasons)
}

func bkrOwnedRotationKeys(records []bkrRotationKeyViewRecord, targetLevel int) map[uint64]bool {
	owned := map[uint64]bool{}
	for _, record := range records {
		if record.TargetLevel == targetLevel && record.OwnerTargetLevel == targetLevel && !record.IsShared {
			owned[record.GaloisElement] = true
		}
	}
	return owned
}

func bkrPersistentKeyBytesWithOwnedRotations(keys *EvaluationKeys, owned map[uint64]bool) int64 {
	if keys == nil {
		return 0
	}
	size := int64(keys.BinarySize())
	if keys.MemEvaluationKeySet == nil {
		return size
	}
	for _, galEl := range keys.GetGaloisKeysList() {
		if owned[galEl] {
			continue
		}
		gk, err := keys.GetGaloisKey(galEl)
		if err != nil {
			continue
		}
		size -= int64(gk.BinarySize())
	}
	if size < 0 {
		return 0
	}
	return size
}

func bkrSortedUniqueUint64(values []uint64) []uint64 {
	out := append([]uint64(nil), values...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	write := 0
	for _, value := range out {
		if write == 0 || out[write-1] != value {
			out[write] = value
			write++
		}
	}
	return out[:write]
}

func bkrSortedIntSet(values map[int]bool) []int {
	out := make([]int, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}
