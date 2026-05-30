package bootstrapping

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/tuneinsight/lattigo/v6/circuits/ckks/dft"
	"github.com/tuneinsight/lattigo/v6/core/rlwe"
	"github.com/tuneinsight/lattigo/v6/ring/ringqp"
	"github.com/tuneinsight/lattigo/v6/schemes/ckks"
	"github.com/tuneinsight/lattigo/v6/utils/structs"
)

var (
	ErrInvalidTargetLevel           = errors.New("invalid target level")
	ErrTargetLevelUnavailable       = errors.New("target level unavailable")
	ErrMissingReusableMaterial      = errors.New("missing reusable material")
	ErrIncompatibleReusableMaterial = errors.New("incompatible reusable material")
	ErrReusableViewOutOfRange       = errors.New("reusable view out of range")
)

type ReusePolicy int

const (
	ReuseExactOnly ReusePolicy = iota
	ReuseExactAndPrefix
	ReuseExactPrefixAndSupersetDrop
)

type TargetLevelMaterialRequest struct {
	FullResidualParameters ckks.Parameters
	BootstrappingLiteral   ParametersLiteral
	FutureTargetLevels     []int
	OwnerTargetLevels      []int
	ReusePolicy            ReusePolicy
	EnableRNSSliceViews    bool
	AllowSupersetDrop      bool
}

type TargetLevelMaterialPlan struct {
	req                      TargetLevelMaterialRequest
	futureTargetLevels       []int
	ownerTargetLevels        []int
	paramsByTargetLevel      map[int]Parameters
	ownerByTargetLevel       map[int]int
	strategyByTargetLevel    map[int]string
	rejectedCandidateReasons map[int]map[string]int
}

type ReusableEvaluationKeys struct {
	plan *TargetLevelMaterialPlan
	pool *SharedKeyPool
}

type MaterialKind string

const (
	MaterialKindEvaluationKey      MaterialKind = "evaluation_key"
	MaterialKindRelinearizationKey MaterialKind = "relinearization_key"
	MaterialKindRotationKey        MaterialKind = "rotation_key"
	MaterialKindRingSwitchKey      MaterialKind = "ring_switch_key"
	MaterialKindDenseSparseKey     MaterialKind = "dense_sparse_key"
	MaterialKindMatrixSchedule     MaterialKind = "matrix_schedule"
	MaterialKindEncodedDiagonal    MaterialKind = "encoded_diagonal"
)

type MaterialID struct {
	Kind           MaterialKind
	OwnerLevel     int
	TargetLevel    int
	LevelQ         int
	LevelP         int
	ParametersHash string
	View           string
	SecretDomain   string
	GaloisElement  uint64
	Direction      string
	MatrixName     string
	TransformIndex int
	DiagonalIndex  int
	DescriptorHash string
}

type SharedKeyPool struct {
	ownerTargetLevels []int
	evaluationKeys    map[int]*EvaluationKeys
	materialIDs       []MaterialID
}

type TargetLevelBootstrapper struct {
	plan             *TargetLevelMaterialPlan
	keys             *ReusableEvaluationKeys
	evaluators       map[int]*Evaluator
	prefixEvaluators map[int]*Evaluator
}

type TargetLevelPlanReport struct {
	PlanID                   string
	FutureTargetLevels       []int
	OwnerTargetLevels        []int
	EnableRNSSliceViews      bool
	AllowSupersetDrop        bool
	Targets                  []TargetLevelPlanTargetReport
	GeneratedPhysicalCounts  map[string]int
	LogicalViewCounts        map[string]int
	SharedCounts             map[string]int
	RejectedCandidateReasons map[string]int
}

type TargetLevelPlanTargetReport struct {
	TargetLevel              int
	OwnerLevel               int
	Strategy                 string
	ReasonCode               string
	GeneratedPhysicalCounts  map[string]int
	LogicalViewCounts        map[string]int
	SharedCounts             map[string]int
	RejectedCandidateReasons map[string]int
}

func NewTargetLevelMaterialPlan(req TargetLevelMaterialRequest) (*TargetLevelMaterialPlan, error) {
	futureTargets, err := canonicalTargetLevels(req.FutureTargetLevels, "FutureTargetLevels")
	if err != nil {
		return nil, err
	}
	if len(futureTargets) == 0 {
		return nil, fmt.Errorf("%w: FutureTargetLevels must not be empty", ErrTargetLevelUnavailable)
	}

	paramsByTarget := make(map[int]Parameters, len(futureTargets)+len(req.OwnerTargetLevels))
	for _, targetLevel := range futureTargets {
		params, err := targetLevelParameters(req.FullResidualParameters, req.BootstrappingLiteral, targetLevel)
		if err != nil {
			return nil, err
		}
		paramsByTarget[targetLevel] = params
	}

	ownerTargets, err := canonicalOwnerTargetLevels(req, futureTargets)
	if err != nil {
		return nil, err
	}

	for _, ownerLevel := range ownerTargets {
		params, err := targetLevelParameters(req.FullResidualParameters, req.BootstrappingLiteral, ownerLevel)
		if err != nil {
			return nil, err
		}
		paramsByTarget[ownerLevel] = params
	}

	ownerByTarget, strategyByTarget, rejectedReasons, err := selectOwnerTargets(req, futureTargets, ownerTargets, paramsByTarget)
	if err != nil {
		return nil, err
	}

	return &TargetLevelMaterialPlan{
		req:                      req,
		futureTargetLevels:       futureTargets,
		ownerTargetLevels:        ownerTargets,
		paramsByTargetLevel:      paramsByTarget,
		ownerByTargetLevel:       ownerByTarget,
		strategyByTargetLevel:    strategyByTarget,
		rejectedCandidateReasons: rejectedReasons,
	}, nil
}

func (p *TargetLevelMaterialPlan) GenReusableEvaluationKeys(sk *rlwe.SecretKey) (*ReusableEvaluationKeys, error) {
	if p == nil {
		return nil, fmt.Errorf("%w: target-level material plan is nil", ErrMissingReusableMaterial)
	}
	if sk == nil {
		return nil, fmt.Errorf("%w: secret key is nil", ErrMissingReusableMaterial)
	}

	keysByOwner := make(map[int]*EvaluationKeys, len(p.ownerTargetLevels))
	materialIDs := make([]MaterialID, 0, len(p.ownerTargetLevels))
	for _, ownerLevel := range p.ownerTargetLevels {
		btpParams, ok := p.paramsByTargetLevel[ownerLevel]
		if !ok {
			return nil, fmt.Errorf("%w: owner target level %d has no parameters", ErrMissingReusableMaterial, ownerLevel)
		}

		ownerSK, err := secretKeyForResidualParameters(sk, btpParams.ResidualParameters)
		if err != nil {
			return nil, err
		}

		keys, ownerEphemeralSK, err := btpParams.GenEvaluationKeys(ownerSK)
		if err != nil {
			return nil, fmt.Errorf("cannot generate reusable evaluation keys for owner target level %d: %w", ownerLevel, err)
		}
		keysByOwner[ownerLevel] = keys
		materialIDs = append(materialIDs, MaterialID{
			Kind:           MaterialKindEvaluationKey,
			OwnerLevel:     ownerLevel,
			TargetLevel:    ownerLevel,
			LevelQ:         btpParams.BootstrappingParameters.MaxLevel(),
			LevelP:         btpParams.BootstrappingParameters.MaxLevelP(),
			ParametersHash: targetLevelParametersHash(btpParams),
			View:           "physical",
		})
		materialIDs = append(materialIDs, keyMaterialIDs(ownerLevel, btpParams, keys, ownerSK, ownerEphemeralSK)...)
	}

	return &ReusableEvaluationKeys{
		plan: p,
		pool: &SharedKeyPool{
			ownerTargetLevels: append([]int(nil), p.ownerTargetLevels...),
			evaluationKeys:    keysByOwner,
			materialIDs:       materialIDs,
		},
	}, nil
}

func (k *ReusableEvaluationKeys) SharedKeyPool() *SharedKeyPool {
	if k == nil {
		return nil
	}
	return k.pool
}

func (p *SharedKeyPool) OwnerTargetLevels() []int {
	if p == nil {
		return nil
	}
	return append([]int(nil), p.ownerTargetLevels...)
}

func (p *SharedKeyPool) MaterialIDs() []MaterialID {
	if p == nil {
		return nil
	}
	return append([]MaterialID(nil), p.materialIDs...)
}

func (p *SharedKeyPool) addMaterialIDs(ids ...MaterialID) {
	if p == nil {
		return
	}
	for _, id := range ids {
		if !materialIDInSlice(p.materialIDs, id) {
			p.materialIDs = append(p.materialIDs, id)
		}
	}
}

func materialIDInSlice(ids []MaterialID, want MaterialID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func NewTargetLevelBootstrapper(plan *TargetLevelMaterialPlan, keys *ReusableEvaluationKeys) (*TargetLevelBootstrapper, error) {
	if plan == nil {
		return nil, fmt.Errorf("%w: target-level material plan is nil", ErrMissingReusableMaterial)
	}
	if keys == nil {
		return nil, fmt.Errorf("%w: reusable evaluation keys are nil", ErrMissingReusableMaterial)
	}
	if keys.pool == nil {
		return nil, fmt.Errorf("%w: shared key pool is nil", ErrMissingReusableMaterial)
	}
	if keys.plan != plan {
		return nil, fmt.Errorf("%w: reusable evaluation keys were generated for a different plan", ErrIncompatibleReusableMaterial)
	}

	evaluators := make(map[int]*Evaluator, len(plan.ownerTargetLevels))
	for _, ownerLevel := range plan.ownerTargetLevels {
		btpParams, ok := plan.paramsByTargetLevel[ownerLevel]
		if !ok {
			return nil, fmt.Errorf("%w: owner target level %d has no parameters", ErrMissingReusableMaterial, ownerLevel)
		}
		evk := keys.pool.evaluationKeys[ownerLevel]
		if evk == nil {
			return nil, fmt.Errorf("%w: owner target level %d has no evaluation keys", ErrMissingReusableMaterial, ownerLevel)
		}
		eval, err := NewEvaluator(btpParams, evk)
		if err != nil {
			return nil, fmt.Errorf("cannot create evaluator for owner target level %d: %w", ownerLevel, err)
		}
		if outputLevel := eval.OutputLevel(); outputLevel != ownerLevel {
			return nil, fmt.Errorf("%w: owner target level %d evaluator output level %d", ErrIncompatibleReusableMaterial, ownerLevel, outputLevel)
		}
		evaluators[ownerLevel] = eval
		keys.pool.addMaterialIDs(evaluatorMaterialIDs(ownerLevel, btpParams, eval)...)
	}
	if plan.req.EnableRNSSliceViews {
		viewIDs, err := plan.rnsPrefixViewMaterialIDs(keys.pool)
		if err != nil {
			return nil, err
		}
		keys.pool.addMaterialIDs(viewIDs...)
	}

	prefixEvaluators, err := plan.rnsPrefixEvaluators(keys.pool)
	if err != nil {
		return nil, err
	}

	return &TargetLevelBootstrapper{
		plan:             plan,
		keys:             keys,
		evaluators:       evaluators,
		prefixEvaluators: prefixEvaluators,
	}, nil
}

func (b *TargetLevelBootstrapper) BootstrapAtLevel(ct *rlwe.Ciphertext, targetLevel int) (*rlwe.Ciphertext, error) {
	eval, ownerLevel, err := b.evaluatorForTarget(targetLevel)
	if err != nil {
		return nil, err
	}
	out, err := eval.Bootstrap(ct)
	if err != nil {
		return nil, err
	}
	if ownerLevel > targetLevel {
		eval.Evaluator.DropLevel(out, ownerLevel-targetLevel)
	}
	if out.Level() != targetLevel {
		return nil, fmt.Errorf("%w: bootstrap to target level %d returned level %d", ErrIncompatibleReusableMaterial, targetLevel, out.Level())
	}
	return out, nil
}

func (b *TargetLevelBootstrapper) BootstrapManyAtLevel(cts []rlwe.Ciphertext, targetLevel int) ([]rlwe.Ciphertext, error) {
	eval, ownerLevel, err := b.evaluatorForTarget(targetLevel)
	if err != nil {
		return nil, err
	}
	outs, err := eval.BootstrapMany(cts)
	if err != nil {
		return nil, err
	}
	if ownerLevel > targetLevel {
		for i := range outs {
			eval.Evaluator.DropLevel(&outs[i], ownerLevel-targetLevel)
		}
	}
	for i := range outs {
		if outs[i].Level() != targetLevel {
			return nil, fmt.Errorf("%w: bootstrap-many target level %d output[%d] level %d", ErrIncompatibleReusableMaterial, targetLevel, i, outs[i].Level())
		}
	}
	return outs, nil
}

func (b *TargetLevelBootstrapper) PlanReport() TargetLevelPlanReport {
	if b == nil {
		return TargetLevelPlanReport{}
	}
	report := b.plan.planReport(b.keys.pool)
	for i := range report.Targets {
		targetLevel := report.Targets[i].TargetLevel
		ownerLevel := report.Targets[i].OwnerLevel
		if b.prefixEvaluators[targetLevel] != nil && ownerLevel > targetLevel {
			report.Targets[i].Strategy = "rns_prefix"
			report.Targets[i].ReasonCode = fmt.Sprintf("rns_prefix_owner_level=%d;target_level=%d", ownerLevel, targetLevel)
		}
	}
	return report
}

func (b *TargetLevelBootstrapper) evaluatorForTarget(targetLevel int) (*Evaluator, int, error) {
	if b == nil || b.plan == nil {
		return nil, 0, fmt.Errorf("%w: target-level bootstrapper is nil", ErrMissingReusableMaterial)
	}
	if targetLevel < 0 || targetLevel > b.plan.req.FullResidualParameters.MaxLevel() {
		return nil, 0, fmt.Errorf("%w: target level %d outside full profile range [0,%d]", ErrInvalidTargetLevel, targetLevel, b.plan.req.FullResidualParameters.MaxLevel())
	}
	ownerLevel, ok := b.plan.ownerByTargetLevel[targetLevel]
	if !ok {
		return nil, 0, fmt.Errorf("%w: target level %d was not declared in the material plan", ErrTargetLevelUnavailable, targetLevel)
	}
	strategy := b.plan.strategyByTargetLevel[targetLevel]
	if strategy == "rns_prefix" {
		if eval := b.prefixEvaluators[targetLevel]; eval != nil {
			return eval, targetLevel, nil
		}
		return nil, 0, fmt.Errorf("%w: target level %d has no RNS prefix evaluator", ErrMissingReusableMaterial, targetLevel)
	}
	eval := b.evaluators[ownerLevel]
	if eval == nil {
		return nil, 0, fmt.Errorf("%w: owner target level %d has no evaluator", ErrMissingReusableMaterial, ownerLevel)
	}
	return eval, ownerLevel, nil
}

func (p *TargetLevelMaterialPlan) PlanReport() TargetLevelPlanReport {
	return p.planReport(nil)
}

func (p *TargetLevelMaterialPlan) planReport(pool *SharedKeyPool) TargetLevelPlanReport {
	if p == nil {
		return TargetLevelPlanReport{}
	}

	targets := make([]TargetLevelPlanTargetReport, 0, len(p.futureTargetLevels))
	generatedCounts := generatedPhysicalCountsForPool(pool, p.ownerTargetLevels)
	logicalViewCounts := map[string]int{}
	sharedCounts := map[string]int{}
	rejectedCounts := map[string]int{}
	for _, targetLevel := range p.futureTargetLevels {
		ownerLevel := p.ownerByTargetLevel[targetLevel]
		strategy := p.strategyByTargetLevel[targetLevel]
		if strategy == "" {
			strategy = "exact"
		}
		reasonCode := "none"
		targetGeneratedCounts := map[string]int{}
		targetLogicalViewCounts := map[string]int{}
		targetSharedCounts := map[string]int{}
		targetRejectedCounts := cloneCounts(p.rejectedCandidateReasons[targetLevel])
		addCounts(rejectedCounts, targetRejectedCounts)
		if ownerLevel == targetLevel {
			targetGeneratedCounts = generatedPhysicalCountsForOwner(pool, targetLevel)
		}
		switch {
		case strategy == "rns_prefix":
			reasonCode = fmt.Sprintf("rns_prefix_owner_level=%d;target_level=%d", ownerLevel, targetLevel)
		case strategy == "superset_output_drop":
			reasonCode = fmt.Sprintf("superset_output_owner_level=%d;drop_levels=%d", ownerLevel, ownerLevel-targetLevel)
			logicalViewCounts[strategy]++
			addCounts(sharedCounts, generatedPhysicalCountsForOwner(pool, ownerLevel))
			targetLogicalViewCounts[strategy] = 1
			targetSharedCounts = generatedPhysicalCountsForOwner(pool, ownerLevel)
		}
		for _, id := range logicalMaterialIDsForTarget(pool, targetLevel) {
			logicalViewCounts[id.View]++
			targetLogicalViewCounts[id.View]++
		}
		targets = append(targets, TargetLevelPlanTargetReport{
			TargetLevel:              targetLevel,
			OwnerLevel:               ownerLevel,
			Strategy:                 strategy,
			ReasonCode:               reasonCode,
			GeneratedPhysicalCounts:  targetGeneratedCounts,
			LogicalViewCounts:        targetLogicalViewCounts,
			SharedCounts:             targetSharedCounts,
			RejectedCandidateReasons: targetRejectedCounts,
		})
	}

	return TargetLevelPlanReport{
		PlanID:                   "TargetLevelMaterialPlan/v1",
		FutureTargetLevels:       append([]int(nil), p.futureTargetLevels...),
		OwnerTargetLevels:        append([]int(nil), p.ownerTargetLevels...),
		EnableRNSSliceViews:      p.req.EnableRNSSliceViews,
		AllowSupersetDrop:        p.req.AllowSupersetDrop,
		Targets:                  targets,
		GeneratedPhysicalCounts:  generatedCounts,
		LogicalViewCounts:        logicalViewCounts,
		SharedCounts:             sharedCounts,
		RejectedCandidateReasons: rejectedCounts,
	}
}

func generatedPhysicalCountsForPool(pool *SharedKeyPool, ownerLevels []int) map[string]int {
	if pool == nil {
		return map[string]int{string(MaterialKindEvaluationKey): len(ownerLevels)}
	}

	counts := map[string]int{}
	for _, id := range pool.materialIDs {
		if id.View == "physical" {
			counts[string(id.Kind)]++
		}
	}
	return counts
}

func generatedPhysicalCountsForOwner(pool *SharedKeyPool, ownerLevel int) map[string]int {
	if pool == nil {
		return map[string]int{string(MaterialKindEvaluationKey): 1}
	}

	counts := map[string]int{}
	for _, id := range pool.materialIDs {
		if id.View == "physical" && id.OwnerLevel == ownerLevel {
			counts[string(id.Kind)]++
		}
	}
	return counts
}

func addCounts(dst map[string]int, src map[string]int) {
	for kind, count := range src {
		dst[kind] += count
	}
}

func cloneCounts(src map[string]int) map[string]int {
	dst := map[string]int{}
	addCounts(dst, src)
	return dst
}

func logicalMaterialIDsForTarget(pool *SharedKeyPool, targetLevel int) []MaterialID {
	if pool == nil {
		return nil
	}
	ids := []MaterialID{}
	for _, id := range pool.materialIDs {
		if id.TargetLevel == targetLevel && id.View != "" && id.View != "physical" {
			ids = append(ids, id)
		}
	}
	return ids
}

func (p *TargetLevelMaterialPlan) rnsPrefixViewMaterialIDs(pool *SharedKeyPool) ([]MaterialID, error) {
	if p == nil || pool == nil {
		return nil, fmt.Errorf("%w: cannot build RNS prefix views without plan and pool", ErrMissingReusableMaterial)
	}

	ids := []MaterialID{}
	for _, targetLevel := range p.futureTargetLevels {
		ownerLevel := p.ownerByTargetLevel[targetLevel]
		if ownerLevel <= targetLevel {
			continue
		}
		if p.strategyByTargetLevel[targetLevel] != "rns_prefix" {
			continue
		}

		targetParams := p.paramsByTargetLevel[targetLevel]
		ownerParams := p.paramsByTargetLevel[ownerLevel]
		if _, ok := rnsPrefixCompatibleParameters(ownerParams, targetParams); !ok {
			continue
		}
		targetLevelQ := targetParams.BootstrappingParameters.MaxLevel()
		targetLevelP := targetParams.BootstrappingParameters.MaxLevelP()
		if err := validateOwnerEvaluationKeysForRNSPrefix(pool.evaluationKeys[ownerLevel], targetParams); err != nil {
			return nil, fmt.Errorf("cannot create RNS prefix view owner=%d target=%d: %w", ownerLevel, targetLevel, err)
		}

		for _, physical := range pool.materialIDs {
			if physical.OwnerLevel != ownerLevel || physical.View != "physical" {
				continue
			}
			switch physical.Kind {
			case MaterialKindRelinearizationKey, MaterialKindRotationKey, MaterialKindRingSwitchKey, MaterialKindDenseSparseKey, MaterialKindEncodedDiagonal:
				if physical.LevelQ < targetLevelQ || physical.LevelP != targetLevelP {
					continue
				}
				view := physical
				view.TargetLevel = targetLevel
				view.LevelQ = targetLevelQ
				view.LevelP = targetLevelP
				view.View = "rns_prefix"
				view.DescriptorHash = hashStrings("rns-prefix-view", physical.DescriptorHash, strconv.Itoa(ownerLevel), strconv.Itoa(targetLevel))
				ids = append(ids, view)
			}
		}
	}

	return ids, nil
}

func (p *TargetLevelMaterialPlan) rnsPrefixEvaluators(pool *SharedKeyPool) (map[int]*Evaluator, error) {
	evaluators := map[int]*Evaluator{}
	if p == nil || pool == nil || !p.req.EnableRNSSliceViews {
		return evaluators, nil
	}

	for _, targetLevel := range p.futureTargetLevels {
		ownerLevel := p.ownerByTargetLevel[targetLevel]
		if ownerLevel <= targetLevel {
			continue
		}
		if p.strategyByTargetLevel[targetLevel] != "rns_prefix" {
			continue
		}

		ownerKeys := pool.evaluationKeys[ownerLevel]
		if ownerKeys == nil {
			return nil, fmt.Errorf("%w: owner target level %d has no keys for RNS prefix evaluator", ErrMissingReusableMaterial, ownerLevel)
		}
		targetParams := p.paramsByTargetLevel[targetLevel]
		ownerParams := p.paramsByTargetLevel[ownerLevel]
		if _, ok := rnsPrefixCompatibleParameters(ownerParams, targetParams); !ok {
			continue
		}
		targetKeys, err := rnsPrefixEvaluationKeysView(ownerKeys, targetParams)
		if err != nil {
			return nil, fmt.Errorf("cannot create RNS prefix keys owner=%d target=%d: %w", ownerLevel, targetLevel, err)
		}
		eval, err := NewEvaluator(targetParams, targetKeys)
		if err != nil {
			return nil, fmt.Errorf("cannot create RNS prefix evaluator owner=%d target=%d: %w", ownerLevel, targetLevel, err)
		}
		if outputLevel := eval.OutputLevel(); outputLevel != targetLevel {
			return nil, fmt.Errorf("%w: RNS prefix evaluator target %d output level %d", ErrIncompatibleReusableMaterial, targetLevel, outputLevel)
		}
		evaluators[targetLevel] = eval
	}

	return evaluators, nil
}

func validateOwnerEvaluationKeysForRNSPrefix(keys *EvaluationKeys, targetParams Parameters) error {
	if keys == nil {
		return fmt.Errorf("%w: owner evaluation keys are nil", ErrMissingReusableMaterial)
	}
	levelQ := targetParams.BootstrappingParameters.MaxLevel()
	levelP := targetParams.BootstrappingParameters.MaxLevelP()
	rlweParams := targetParams.BootstrappingParameters

	if keys.MemEvaluationKeySet != nil {
		if rlk, err := keys.GetRelinearizationKey(); err == nil {
			if err := validateRNSPrefixEvaluationKeyView(rlweParams, &rlk.EvaluationKey, levelQ, levelP); err != nil {
				return fmt.Errorf("relinearization key: %w", err)
			}
		}
		for _, galEl := range keys.GetGaloisKeysList() {
			gk, err := keys.GetGaloisKey(galEl)
			if err != nil {
				return err
			}
			if err := validateRNSPrefixEvaluationKeyView(rlweParams, &gk.EvaluationKey, levelQ, levelP); err != nil {
				return fmt.Errorf("rotation key galEl=%d: %w", galEl, err)
			}
		}
	}

	for _, item := range []struct {
		name string
		key  *rlwe.EvaluationKey
	}{
		{name: "EvkN1ToN2", key: keys.EvkN1ToN2},
		{name: "EvkN2ToN1", key: keys.EvkN2ToN1},
		{name: "EvkRealToCmplx", key: keys.EvkRealToCmplx},
		{name: "EvkCmplxToReal", key: keys.EvkCmplxToReal},
		{name: "EvkDenseToSparse", key: keys.EvkDenseToSparse},
		{name: "EvkSparseToDense", key: keys.EvkSparseToDense},
	} {
		if item.key == nil {
			continue
		}
		if item.key.LevelQ() < levelQ || item.key.LevelP() != levelP {
			continue
		}
		if err := validateRNSPrefixEvaluationKeyView(rlweParams, item.key, levelQ, levelP); err != nil {
			return fmt.Errorf("%s: %w", item.name, err)
		}
	}

	return nil
}

func rnsPrefixCompatibleParameters(ownerParams, targetParams Parameters) (string, bool) {
	if reason, ok := rnsPrefixCompatibleCKKSParameters(ownerParams.BootstrappingParameters, targetParams.BootstrappingParameters); !ok {
		return "bootstrapping_" + reason, false
	}
	if reason, ok := rnsPrefixCompatibleCKKSParameters(ownerParams.ResidualParameters, targetParams.ResidualParameters); !ok {
		return "residual_" + reason, false
	}
	if ownerParams.BootstrappingParameters.LogDefaultScale() != targetParams.BootstrappingParameters.LogDefaultScale() {
		return "bootstrapping_default_scale_mismatch", false
	}
	if ownerParams.ResidualParameters.LogDefaultScale() != targetParams.ResidualParameters.LogDefaultScale() {
		return "residual_default_scale_mismatch", false
	}
	return "compatible", true
}

func rnsPrefixCompatibleCKKSParameters(ownerParams, targetParams ckks.Parameters) (string, bool) {
	if ownerParams.LogN() != targetParams.LogN() {
		return "logn_mismatch", false
	}
	if ownerParams.RingType() != targetParams.RingType() {
		return "ring_type_mismatch", false
	}
	if ownerParams.LogMaxSlots() != targetParams.LogMaxSlots() {
		return "log_slots_mismatch", false
	}

	ownerQ := ownerParams.Q()
	targetQ := targetParams.Q()
	if len(ownerQ) < len(targetQ) {
		return "q_prefix_short", false
	}
	for i, qi := range targetQ {
		if ownerQ[i] != qi {
			return fmt.Sprintf("q_prefix_mismatch_%d", i), false
		}
	}

	ownerP := ownerParams.P()
	targetP := targetParams.P()
	if len(ownerP) != len(targetP) {
		return "p_chain_length_mismatch", false
	}
	for i, pi := range targetP {
		if ownerP[i] != pi {
			return fmt.Sprintf("p_chain_mismatch_%d", i), false
		}
	}

	return "compatible", true
}

func canonicalTargetLevels(levels []int, field string) ([]int, error) {
	copied := append([]int(nil), levels...)
	sort.Ints(copied)

	out := copied[:0]
	for _, level := range copied {
		if level < 0 {
			return nil, fmt.Errorf("%w: %s contains negative level %d", ErrInvalidTargetLevel, field, level)
		}
		if len(out) == 0 || out[len(out)-1] != level {
			out = append(out, level)
		}
	}
	return append([]int(nil), out...), nil
}

func canonicalOwnerTargetLevels(req TargetLevelMaterialRequest, futureTargets []int) ([]int, error) {
	if len(req.OwnerTargetLevels) > 0 {
		return canonicalTargetLevels(req.OwnerTargetLevels, "OwnerTargetLevels")
	}
	if req.AllowSupersetDrop && req.ReusePolicy == ReuseExactPrefixAndSupersetDrop {
		return []int{futureTargets[len(futureTargets)-1]}, nil
	}
	return append([]int(nil), futureTargets...), nil
}

func selectOwnerTargets(req TargetLevelMaterialRequest, futureTargets, ownerTargets []int, paramsByTarget map[int]Parameters) (map[int]int, map[int]string, map[int]map[string]int, error) {
	ownerSet := make(map[int]struct{}, len(ownerTargets))
	for _, ownerLevel := range ownerTargets {
		ownerSet[ownerLevel] = struct{}{}
	}

	servedByOwner := make(map[int]bool, len(ownerTargets))
	ownerByTarget := make(map[int]int, len(futureTargets))
	strategyByTarget := make(map[int]string, len(futureTargets))
	rejectedReasons := make(map[int]map[string]int, len(futureTargets))
	for _, targetLevel := range futureTargets {
		ownerLevel, strategy, ok := selectOwnerForTarget(req, targetLevel, ownerTargets, ownerSet, paramsByTarget, rejectedReasons)
		if !ok {
			return nil, nil, nil, fmt.Errorf("%w: target level %d has no compatible owner; rejected=%v", ErrTargetLevelUnavailable, targetLevel, rejectedReasons[targetLevel])
		}
		ownerByTarget[targetLevel] = ownerLevel
		strategyByTarget[targetLevel] = strategy
		servedByOwner[ownerLevel] = true
	}

	for _, ownerLevel := range ownerTargets {
		if !servedByOwner[ownerLevel] {
			return nil, nil, nil, fmt.Errorf("%w: owner target level %d serves no future target", ErrTargetLevelUnavailable, ownerLevel)
		}
	}

	return ownerByTarget, strategyByTarget, rejectedReasons, nil
}

func selectOwnerForTarget(req TargetLevelMaterialRequest, targetLevel int, ownerTargets []int, ownerSet map[int]struct{}, paramsByTarget map[int]Parameters, rejectedReasons map[int]map[string]int) (int, string, bool) {
	if _, ok := ownerSet[targetLevel]; ok {
		return targetLevel, "exact", true
	}
	for _, ownerLevel := range ownerTargets {
		if ownerLevel == targetLevel {
			continue
		}
		strategy, rejectedReason, ok := candidateReuseStrategy(req, targetLevel, ownerLevel, paramsByTarget)
		if rejectedReason != "" {
			addRejectedCandidateReason(rejectedReasons, targetLevel, rejectedReason)
		}
		if ok {
			return ownerLevel, strategy, true
		}
	}
	return 0, "", false
}

func candidateReuseStrategy(req TargetLevelMaterialRequest, targetLevel, ownerLevel int, paramsByTarget map[int]Parameters) (strategy, rejectedReason string, ok bool) {
	if ownerLevel < targetLevel {
		return "", "owner_below_target", false
	}

	prefixAllowed := req.EnableRNSSliceViews && (req.ReusePolicy == ReuseExactAndPrefix || req.ReusePolicy == ReuseExactPrefixAndSupersetDrop)
	if prefixAllowed {
		ownerParams, ownerOK := paramsByTarget[ownerLevel]
		targetParams, targetOK := paramsByTarget[targetLevel]
		if !ownerOK || !targetOK {
			return "", "rns_prefix_missing_parameters", false
		}
		if reason, compatible := rnsPrefixCompatibleParameters(ownerParams, targetParams); compatible {
			return "rns_prefix", "", true
		} else {
			rejectedReason = "rns_prefix_" + reason
		}
	}

	if req.AllowSupersetDrop && req.ReusePolicy == ReuseExactPrefixAndSupersetDrop {
		return "superset_output_drop", rejectedReason, true
	}
	if rejectedReason != "" {
		return "", rejectedReason, false
	}
	if req.ReusePolicy == ReuseExactOnly {
		return "", "exact_only_policy", false
	}
	if !req.EnableRNSSliceViews {
		return "", "rns_prefix_disabled", false
	}
	return "", "superset_drop_disabled", false
}

func addRejectedCandidateReason(rejectedReasons map[int]map[string]int, targetLevel int, reason string) {
	if rejectedReasons[targetLevel] == nil {
		rejectedReasons[targetLevel] = map[string]int{}
	}
	rejectedReasons[targetLevel][reason]++
}

func compatibleMaterialIDs(owner, consumer MaterialID) (string, bool) {
	if owner.Kind != consumer.Kind {
		return "kind_mismatch", false
	}
	if owner.SecretDomain != "" && consumer.SecretDomain != "" && owner.SecretDomain != consumer.SecretDomain {
		return "secret_domain_mismatch", false
	}
	if owner.ParametersHash != "" && consumer.ParametersHash != "" && owner.ParametersHash != consumer.ParametersHash {
		return "parameters_mismatch", false
	}
	if owner.LevelQ < consumer.LevelQ {
		return "levelq_insufficient", false
	}
	if owner.LevelP != consumer.LevelP {
		return "levelp_mismatch", false
	}
	if owner.Kind == MaterialKindRotationKey && owner.GaloisElement != consumer.GaloisElement {
		return "gal_el_mismatch", false
	}
	if owner.Direction != "" && consumer.Direction != "" && owner.Direction != consumer.Direction {
		return "direction_mismatch", false
	}
	if owner.MatrixName != "" && consumer.MatrixName != "" && owner.MatrixName != consumer.MatrixName {
		return "matrix_mismatch", false
	}
	if owner.TransformIndex != consumer.TransformIndex {
		return "transform_index_mismatch", false
	}
	if owner.Kind == MaterialKindEncodedDiagonal && owner.DiagonalIndex != consumer.DiagonalIndex {
		return "diagonal_index_mismatch", false
	}
	if owner.DescriptorHash != "" && consumer.DescriptorHash != "" && owner.DescriptorHash != consumer.DescriptorHash {
		return "descriptor_mismatch", false
	}
	return "compatible", true
}

func validateRNSPrefixEvaluationKeyView(params rlwe.ParameterProvider, evk *rlwe.EvaluationKey, levelQ, levelP int) error {
	if params == nil {
		return fmt.Errorf("%w: nil parameter provider", ErrMissingReusableMaterial)
	}
	if evk == nil {
		return fmt.Errorf("%w: nil evaluation key", ErrMissingReusableMaterial)
	}
	if evk.IsCompressed() {
		return fmt.Errorf("%w: compressed evaluation-key RNS slicing is not enabled", ErrIncompatibleReusableMaterial)
	}
	if levelQ < 0 {
		return fmt.Errorf("%w: required LevelQ=%d is invalid", ErrReusableViewOutOfRange, levelQ)
	}
	if levelQ > evk.LevelQ() {
		return fmt.Errorf("%w: required LevelQ=%d exceeds owner LevelQ=%d", ErrReusableViewOutOfRange, levelQ, evk.LevelQ())
	}
	if levelP != evk.LevelP() {
		return fmt.Errorf("%w: required LevelP=%d differs from owner LevelP=%d", ErrIncompatibleReusableMaterial, levelP, evk.LevelP())
	}

	rlweParams := params.GetRLWEParameters()
	if levelQ > rlweParams.MaxLevelQ() {
		return fmt.Errorf("%w: required LevelQ=%d exceeds params MaxLevelQ=%d", ErrReusableViewOutOfRange, levelQ, rlweParams.MaxLevelQ())
	}
	if levelP > rlweParams.MaxLevelP() {
		return fmt.Errorf("%w: required LevelP=%d exceeds params MaxLevelP=%d", ErrReusableViewOutOfRange, levelP, rlweParams.MaxLevelP())
	}

	requiredRNS := rlweParams.BaseRNSDecompositionVectorSize(levelQ, levelP)
	if requiredRNS > evk.BaseRNSDecompositionVectorSize() {
		return fmt.Errorf("%w: required BaseRNSDecompositionVectorSize=%d exceeds owner size=%d", ErrReusableViewOutOfRange, requiredRNS, evk.BaseRNSDecompositionVectorSize())
	}

	requiredBaseTwo := rlweParams.BaseTwoDecompositionVectorSize(levelQ, levelP, evk.BaseTwoDecomposition)
	for i := 0; i < requiredRNS; i++ {
		if len(evk.Value[i]) < requiredBaseTwo[i] {
			return fmt.Errorf("%w: required BaseTwoDecompositionVectorSize[%d]=%d exceeds owner size=%d", ErrReusableViewOutOfRange, i, requiredBaseTwo[i], len(evk.Value[i]))
		}
		for j := 0; j < requiredBaseTwo[i]; j++ {
			for u, poly := range evk.Value[i][j] {
				if poly.LevelQ() < levelQ {
					return fmt.Errorf("%w: owner Value[%d][%d][%d] LevelQ=%d below required LevelQ=%d", ErrReusableViewOutOfRange, i, j, u, poly.LevelQ(), levelQ)
				}
				if poly.LevelP() != levelP {
					return fmt.Errorf("%w: owner Value[%d][%d][%d] LevelP=%d differs from required LevelP=%d", ErrIncompatibleReusableMaterial, i, j, u, poly.LevelP(), levelP)
				}
			}
		}
	}

	return nil
}

func rnsPrefixEvaluationKeysView(owner *EvaluationKeys, targetParams Parameters) (*EvaluationKeys, error) {
	if owner == nil {
		return nil, fmt.Errorf("%w: owner evaluation keys are nil", ErrMissingReusableMaterial)
	}
	params := targetParams.BootstrappingParameters
	levelQ := params.MaxLevel()
	levelP := params.MaxLevelP()

	view := &EvaluationKeys{}
	var err error
	if view.EvkN1ToN2, err = rnsPrefixEvaluationKeyViewOrNil(params, owner.EvkN1ToN2, levelQ, levelP); err != nil {
		return nil, fmt.Errorf("EvkN1ToN2: %w", err)
	}
	if view.EvkN2ToN1, err = rnsPrefixEvaluationKeyViewOrNil(params, owner.EvkN2ToN1, levelQ, levelP); err != nil {
		return nil, fmt.Errorf("EvkN2ToN1: %w", err)
	}
	if view.EvkRealToCmplx, err = rnsPrefixEvaluationKeyViewOrNil(params, owner.EvkRealToCmplx, levelQ, levelP); err != nil {
		return nil, fmt.Errorf("EvkRealToCmplx: %w", err)
	}
	if view.EvkCmplxToReal, err = rnsPrefixEvaluationKeyViewOrNil(params, owner.EvkCmplxToReal, levelQ, levelP); err != nil {
		return nil, fmt.Errorf("EvkCmplxToReal: %w", err)
	}
	if view.EvkDenseToSparse, err = rnsPrefixEvaluationKeyViewOrKeep(params, owner.EvkDenseToSparse, levelQ, levelP); err != nil {
		return nil, fmt.Errorf("EvkDenseToSparse: %w", err)
	}
	if view.EvkSparseToDense, err = rnsPrefixEvaluationKeyViewOrKeep(params, owner.EvkSparseToDense, levelQ, levelP); err != nil {
		return nil, fmt.Errorf("EvkSparseToDense: %w", err)
	}

	rlk, err := owner.GetRelinearizationKey()
	if err != nil {
		return nil, err
	}
	rlkView, err := rnsPrefixEvaluationKeyView(params, &rlk.EvaluationKey, levelQ, levelP)
	if err != nil {
		return nil, fmt.Errorf("relinearization key: %w", err)
	}

	requiredGalEls := append([]uint64(nil), targetParams.GaloisElements(params)...)
	requiredGalEls = append(requiredGalEls, params.GaloisElementForComplexConjugation())
	gks := make([]*rlwe.GaloisKey, 0, len(requiredGalEls))
	seen := map[uint64]bool{}
	for _, galEl := range requiredGalEls {
		if seen[galEl] {
			continue
		}
		seen[galEl] = true
		gk, err := owner.GetGaloisKey(galEl)
		if err != nil {
			return nil, err
		}
		gkView, err := rnsPrefixEvaluationKeyView(params, &gk.EvaluationKey, levelQ, levelP)
		if err != nil {
			return nil, fmt.Errorf("galois key %d: %w", galEl, err)
		}
		gks = append(gks, &rlwe.GaloisKey{
			GaloisElement: galEl,
			NthRoot:       gk.NthRoot,
			EvaluationKey: *gkView,
		})
	}

	view.MemEvaluationKeySet = rlwe.NewMemEvaluationKeySet(&rlwe.RelinearizationKey{EvaluationKey: *rlkView}, gks...)
	return view, nil
}

func rnsPrefixEvaluationKeyViewOrNil(params rlwe.ParameterProvider, evk *rlwe.EvaluationKey, levelQ, levelP int) (*rlwe.EvaluationKey, error) {
	if evk == nil {
		return nil, nil
	}
	return rnsPrefixEvaluationKeyView(params, evk, levelQ, levelP)
}

func rnsPrefixEvaluationKeyViewOrKeep(params rlwe.ParameterProvider, evk *rlwe.EvaluationKey, levelQ, levelP int) (*rlwe.EvaluationKey, error) {
	if evk == nil {
		return nil, nil
	}
	if evk.LevelQ() < levelQ {
		return evk, nil
	}
	return rnsPrefixEvaluationKeyView(params, evk, levelQ, levelP)
}

func rnsPrefixEvaluationKeyView(params rlwe.ParameterProvider, evk *rlwe.EvaluationKey, levelQ, levelP int) (*rlwe.EvaluationKey, error) {
	if err := validateRNSPrefixEvaluationKeyView(params, evk, levelQ, levelP); err != nil {
		return nil, err
	}
	gadget, err := rnsPrefixGadgetCiphertextView(params, &evk.GadgetCiphertext, levelQ, levelP)
	if err != nil {
		return nil, err
	}
	return &rlwe.EvaluationKey{
		GadgetCiphertext: *gadget,
		Seed:             evk.Seed,
	}, nil
}

func rnsPrefixGadgetCiphertextView(params rlwe.ParameterProvider, gadget *rlwe.GadgetCiphertext, levelQ, levelP int) (*rlwe.GadgetCiphertext, error) {
	if gadget == nil {
		return nil, fmt.Errorf("%w: nil gadget ciphertext", ErrMissingReusableMaterial)
	}
	rlweParams := params.GetRLWEParameters()
	requiredRNS := rlweParams.BaseRNSDecompositionVectorSize(levelQ, levelP)
	requiredBaseTwo := rlweParams.BaseTwoDecompositionVectorSize(levelQ, levelP, gadget.BaseTwoDecomposition)

	value := make(structs.Matrix[rlwe.VectorQP], requiredRNS)
	for i := 0; i < requiredRNS; i++ {
		value[i] = make([]rlwe.VectorQP, requiredBaseTwo[i])
		for j := 0; j < requiredBaseTwo[i]; j++ {
			view, err := rnsPrefixVectorQPView(gadget.Value[i][j], levelQ, levelP)
			if err != nil {
				return nil, fmt.Errorf("Value[%d][%d]: %w", i, j, err)
			}
			value[i][j] = view
		}
	}

	return &rlwe.GadgetCiphertext{
		BaseTwoDecomposition: gadget.BaseTwoDecomposition,
		Value:                value,
	}, nil
}

func rnsPrefixVectorQPView(vector rlwe.VectorQP, levelQ, levelP int) (rlwe.VectorQP, error) {
	out := make(rlwe.VectorQP, len(vector))
	for i, poly := range vector {
		if poly.LevelQ() < levelQ {
			return nil, fmt.Errorf("%w: poly[%d] LevelQ=%d below %d", ErrReusableViewOutOfRange, i, poly.LevelQ(), levelQ)
		}
		if poly.LevelP() != levelP {
			return nil, fmt.Errorf("%w: poly[%d] LevelP=%d differs from %d", ErrIncompatibleReusableMaterial, i, poly.LevelP(), levelP)
		}
		out[i] = rnsPrefixPolyQPView(poly, levelQ, levelP)
	}
	return out, nil
}

func rnsPrefixPolyQPView(poly ringqp.Poly, levelQ, levelP int) ringqp.Poly {
	out := poly
	out.Resize(levelQ, levelP)
	return out
}

func keyMaterialIDs(ownerLevel int, params Parameters, keys *EvaluationKeys, inputSK, ephemeralSK *rlwe.SecretKey) []MaterialID {
	if keys == nil {
		return nil
	}

	paramHash := targetLevelParametersHash(params)
	inputDomain := secretKeyDomainHash("input-skN1", inputSK)
	ephemeralDomain := secretKeyDomainHash("ephemeral-skN2", ephemeralSK)
	sparseDomain := hashStrings("secret-domain", "sparse", ephemeralDomain, paramHash, strconv.Itoa(params.EphemeralSecretWeight))
	ids := []MaterialID{}

	if keys.MemEvaluationKeySet != nil {
		if _, err := keys.GetRelinearizationKey(); err == nil {
			rlk, _ := keys.GetRelinearizationKey()
			secretDomain := materialSecretDomain(MaterialKindRelinearizationKey, "skN2", ephemeralDomain)
			ids = append(ids, MaterialID{
				Kind:           MaterialKindRelinearizationKey,
				OwnerLevel:     ownerLevel,
				TargetLevel:    ownerLevel,
				LevelQ:         rlk.LevelQ(),
				LevelP:         rlk.LevelP(),
				ParametersHash: paramHash,
				View:           "physical",
				SecretDomain:   secretDomain,
				DescriptorHash: evaluationKeyDescriptorHash("relinearization-key", paramHash, secretDomain, &rlk.EvaluationKey),
			})
		}

		galEls := append([]uint64(nil), keys.GetGaloisKeysList()...)
		sort.Slice(galEls, func(i, j int) bool { return galEls[i] < galEls[j] })
		for _, galEl := range galEls {
			gk, err := keys.GetGaloisKey(galEl)
			if err != nil {
				continue
			}
			secretDomain := materialSecretDomain(MaterialKindRotationKey, "skN2", ephemeralDomain)
			ids = append(ids, MaterialID{
				Kind:           MaterialKindRotationKey,
				OwnerLevel:     ownerLevel,
				TargetLevel:    ownerLevel,
				LevelQ:         gk.LevelQ(),
				LevelP:         gk.LevelP(),
				ParametersHash: paramHash,
				View:           "physical",
				SecretDomain:   secretDomain,
				GaloisElement:  galEl,
				DescriptorHash: evaluationKeyDescriptorHash("rotation-key", paramHash, secretDomain, &gk.EvaluationKey, strconv.FormatUint(galEl, 10), strconv.FormatUint(gk.NthRoot, 10)),
			})
		}
	}

	ids = append(ids, evaluationKeyMaterialID(ownerLevel, params, keys.EvkN1ToN2, MaterialKindRingSwitchKey, "N1ToN2", materialSecretDomain(MaterialKindRingSwitchKey, "N1ToN2", inputDomain, ephemeralDomain), paramHash)...)
	ids = append(ids, evaluationKeyMaterialID(ownerLevel, params, keys.EvkN2ToN1, MaterialKindRingSwitchKey, "N2ToN1", materialSecretDomain(MaterialKindRingSwitchKey, "N2ToN1", ephemeralDomain, inputDomain), paramHash)...)
	ids = append(ids, evaluationKeyMaterialID(ownerLevel, params, keys.EvkRealToCmplx, MaterialKindRingSwitchKey, "RealToCmplx", materialSecretDomain(MaterialKindRingSwitchKey, "RealToCmplx", inputDomain, ephemeralDomain), paramHash)...)
	ids = append(ids, evaluationKeyMaterialID(ownerLevel, params, keys.EvkCmplxToReal, MaterialKindRingSwitchKey, "CmplxToReal", materialSecretDomain(MaterialKindRingSwitchKey, "CmplxToReal", ephemeralDomain, inputDomain), paramHash)...)
	ids = append(ids, evaluationKeyMaterialID(ownerLevel, params, keys.EvkDenseToSparse, MaterialKindDenseSparseKey, "DenseToSparse", materialSecretDomain(MaterialKindDenseSparseKey, "DenseToSparse", ephemeralDomain, sparseDomain), paramHash)...)
	ids = append(ids, evaluationKeyMaterialID(ownerLevel, params, keys.EvkSparseToDense, MaterialKindDenseSparseKey, "SparseToDense", materialSecretDomain(MaterialKindDenseSparseKey, "SparseToDense", sparseDomain, ephemeralDomain), paramHash)...)

	return ids
}

func evaluationKeyMaterialID(ownerLevel int, params Parameters, key *rlwe.EvaluationKey, kind MaterialKind, direction, secretDomain, paramHash string) []MaterialID {
	if key == nil {
		return nil
	}
	return []MaterialID{{
		Kind:           kind,
		OwnerLevel:     ownerLevel,
		TargetLevel:    ownerLevel,
		LevelQ:         key.LevelQ(),
		LevelP:         key.LevelP(),
		ParametersHash: paramHash,
		View:           "physical",
		SecretDomain:   secretDomain,
		Direction:      direction,
		DescriptorHash: evaluationKeyDescriptorHash(string(kind), paramHash, secretDomain, key, direction),
	}}
}

func evaluatorMaterialIDs(ownerLevel int, params Parameters, eval *Evaluator) []MaterialID {
	if eval == nil {
		return nil
	}
	ids := []MaterialID{}
	ids = append(ids, matrixMaterialIDs(ownerLevel, params, "CoeffsToSlots", eval.C2SDFTMatrix)...)
	ids = append(ids, matrixMaterialIDs(ownerLevel, params, "SlotsToCoeffs", eval.S2CDFTMatrix)...)
	return ids
}

func matrixMaterialIDs(ownerLevel int, params Parameters, matrixName string, matrix dft.Matrix) []MaterialID {
	paramHash := targetLevelParametersHash(params)
	ids := []MaterialID{}

	for transformIndex, lt := range matrix.Matrices {
		diagonalIndices := make([]int, 0, len(lt.Vec))
		for diagonalIndex := range lt.Vec {
			diagonalIndices = append(diagonalIndices, diagonalIndex)
		}
		sort.Ints(diagonalIndices)

		scheduleHashParts := []string{
			"matrix-schedule",
			matrixName,
			strconv.Itoa(transformIndex),
			strconv.Itoa(int(matrix.Type)),
			strconv.Itoa(int(matrix.Format)),
			strconv.Itoa(matrix.LogSlots),
			strconv.Itoa(matrix.LevelQ),
			strconv.Itoa(matrix.LevelP),
			strconv.Itoa(lt.LevelQ),
			strconv.Itoa(lt.LevelP),
			strconv.Itoa(lt.N1),
			strconv.Itoa(lt.LogDimensions.Rows),
			strconv.Itoa(lt.LogDimensions.Cols),
			paramHash,
		}
		for _, diagonalIndex := range diagonalIndices {
			scheduleHashParts = append(scheduleHashParts, strconv.Itoa(diagonalIndex))
		}

		scheduleHash := hashStrings(scheduleHashParts...)
		ids = append(ids, MaterialID{
			Kind:           MaterialKindMatrixSchedule,
			OwnerLevel:     ownerLevel,
			TargetLevel:    ownerLevel,
			LevelQ:         lt.LevelQ,
			LevelP:         lt.LevelP,
			ParametersHash: paramHash,
			View:           "physical",
			SecretDomain:   materialSecretDomain(MaterialKindMatrixSchedule, matrixName, "public", paramHash),
			MatrixName:     matrixName,
			TransformIndex: transformIndex,
			DescriptorHash: scheduleHash,
		})

		for _, diagonalIndex := range diagonalIndices {
			poly := lt.Vec[diagonalIndex]
			ids = append(ids, MaterialID{
				Kind:           MaterialKindEncodedDiagonal,
				OwnerLevel:     ownerLevel,
				TargetLevel:    ownerLevel,
				LevelQ:         poly.LevelQ(),
				LevelP:         poly.LevelP(),
				ParametersHash: paramHash,
				View:           "physical",
				SecretDomain:   materialSecretDomain(MaterialKindEncodedDiagonal, matrixName, "public", paramHash),
				MatrixName:     matrixName,
				TransformIndex: transformIndex,
				DiagonalIndex:  diagonalIndex,
				DescriptorHash: hashStrings("encoded-diagonal", scheduleHash, strconv.Itoa(diagonalIndex), strconv.Itoa(poly.LevelQ()), strconv.Itoa(poly.LevelP()), strconv.Itoa(poly.BinarySize())),
			})
		}
	}

	return ids
}

func secretKeyForResidualParameters(sk *rlwe.SecretKey, params ckks.Parameters) (*rlwe.SecretKey, error) {
	if sk.LevelQ() < params.MaxLevel() {
		return nil, fmt.Errorf("%w: secret key LevelQ=%d below target residual LevelQ=%d", ErrIncompatibleReusableMaterial, sk.LevelQ(), params.MaxLevel())
	}
	if params.MaxLevelP() >= 0 && sk.LevelP() < params.MaxLevelP() {
		return nil, fmt.Errorf("%w: secret key LevelP=%d below target residual LevelP=%d", ErrIncompatibleReusableMaterial, sk.LevelP(), params.MaxLevelP())
	}

	out := sk.CopyNew()
	out.Value.Resize(params.MaxLevel(), params.MaxLevelP())
	return out, nil
}

func targetLevelParametersHash(params Parameters) string {
	h := sha256.New()
	residual := params.ResidualParameters
	bootstrapping := params.BootstrappingParameters
	fmt.Fprintf(h, "residual-logN=%d;", residual.LogN())
	fmt.Fprintf(h, "residual-logSlots=%d;", residual.LogMaxSlots())
	fmt.Fprintf(h, "residual-ringType=%v;", residual.RingType())
	fmt.Fprintf(h, "residual-scale=%d;", residual.LogDefaultScale())
	fmt.Fprintf(h, "residual-Q=%v;", residual.Q())
	fmt.Fprintf(h, "residual-P=%v;", residual.P())
	fmt.Fprintf(h, "bootstrap-logN=%d;", bootstrapping.LogN())
	fmt.Fprintf(h, "bootstrap-ringType=%v;", bootstrapping.RingType())
	fmt.Fprintf(h, "bootstrap-Q=%v;", bootstrapping.Q())
	fmt.Fprintf(h, "bootstrap-P=%v;", bootstrapping.P())
	return hex.EncodeToString(h.Sum(nil))
}

func evaluationKeyDescriptorHash(prefix, paramHash, secretDomain string, evk *rlwe.EvaluationKey, extra ...string) string {
	parts := []string{
		prefix,
		paramHash,
		secretDomain,
	}
	if evk != nil {
		parts = append(parts,
			strconv.Itoa(evk.LevelQ()),
			strconv.Itoa(evk.LevelP()),
			strconv.Itoa(evk.BaseRNSDecompositionVectorSize()),
			fmt.Sprint(evk.BaseTwoDecompositionVectorSize()),
			strconv.Itoa(evk.BaseTwoDecomposition),
			strconv.Itoa(evk.BinarySize()),
			strconv.FormatBool(evk.IsCompressed()),
		)
	}
	parts = append(parts, extra...)
	return hashStrings(parts...)
}

func secretKeyDomainHash(label string, sk *rlwe.SecretKey) string {
	if sk == nil {
		return hashStrings("secret-domain", label, "nil")
	}
	encoded, err := sk.MarshalBinary()
	if err != nil {
		return hashStrings("secret-domain", label, "marshal-error", err.Error(), strconv.Itoa(sk.LevelQ()), strconv.Itoa(sk.LevelP()))
	}
	sum := sha256.Sum256(encoded)
	return hashStrings("secret-domain", label, strconv.Itoa(sk.LevelQ()), strconv.Itoa(sk.LevelP()), hex.EncodeToString(sum[:]))
}

func materialSecretDomain(kind MaterialKind, role string, domains ...string) string {
	parts := []string{"secret-domain", string(kind), role}
	parts = append(parts, domains...)
	return hashStrings(parts...)
}

func hashStrings(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func targetLevelParameters(fullParams ckks.Parameters, btpLiteral ParametersLiteral, targetLevel int) (Parameters, error) {
	if targetLevel < 0 || targetLevel > fullParams.MaxLevel() {
		return Parameters{}, fmt.Errorf("%w: target level %d outside full profile range [0,%d]", ErrInvalidTargetLevel, targetLevel, fullParams.MaxLevel())
	}

	residualLiteral := fullParams.ParametersLiteral()
	q := fullParams.Q()
	p := fullParams.P()
	residualLiteral.Q = append([]uint64(nil), q[:targetLevel+1]...)
	residualLiteral.P = append([]uint64(nil), p...)
	residualLiteral.LogQ = nil
	residualLiteral.LogP = nil
	residualLiteral.LogDefaultScale = fullParams.LogDefaultScale()

	residualParams, err := ckks.NewParametersFromLiteral(residualLiteral)
	if err != nil {
		return Parameters{}, fmt.Errorf("%w: cannot instantiate target residual parameters for level %d: %v", ErrInvalidTargetLevel, targetLevel, err)
	}

	btpParams, err := NewParametersFromLiteral(residualParams, btpLiteral)
	if err != nil {
		return Parameters{}, fmt.Errorf("%w: cannot instantiate bootstrapping parameters for target level %d: %v", ErrInvalidTargetLevel, targetLevel, err)
	}
	if got := btpParams.ResidualParameters.MaxLevel(); got != targetLevel {
		return Parameters{}, fmt.Errorf("%w: target level %d produced residual max level %d", ErrInvalidTargetLevel, targetLevel, got)
	}
	return btpParams, nil
}
