package bootstrapping

import (
	"fmt"
	"sort"

	"github.com/tuneinsight/lattigo/v6/core/rlwe"
)

func (p *TargetLevelMaterialPlan) BuildTargetEvaluationKeys(pool *KeyMaterialPool, targetLevel int) (*EvaluationKeys, error) {
	if p == nil {
		return nil, fmt.Errorf("%w: target-level material plan is nil", ErrMissingReusableMaterial)
	}
	if pool == nil {
		return nil, fmt.Errorf("%w: key material pool is nil", ErrMissingReusableMaterial)
	}
	manifest, ok := pool.ManifestForTarget(targetLevel)
	if !ok {
		return nil, fmt.Errorf("%w: target level %d has no key material manifest", ErrMissingReusableMaterial, targetLevel)
	}
	if manifest.BootstrapSecretDomain == "" {
		return nil, fmt.Errorf("%w: target level %d manifest has no bootstrap secret domain", ErrIncompatibleReusableMaterial, targetLevel)
	}

	out := &EvaluationKeys{}
	galoisKeys := map[uint64]*rlwe.GaloisKey{}
	var relinKey *rlwe.RelinearizationKey
	for _, key := range append(append([]KeyMaterialKey(nil), manifest.Shared...), manifest.Private...) {
		object, ok := pool.Object(key)
		if !ok {
			return nil, fmt.Errorf("%w: target level %d manifest references missing key %+v", ErrMissingReusableMaterial, targetLevel, key)
		}
		if object.View != KeyMaterialViewPhysical {
			return nil, fmt.Errorf("%w: target level %d manifest references non-physical key material view %q", ErrIncompatibleReusableMaterial, targetLevel, object.View)
		}
		if err := ensureKeyMaterialMatchesManifestDomain(manifest, object); err != nil {
			return nil, err
		}
		switch object.Key.Kind {
		case KeyMaterialKindRelinearization:
			rlk, ok := object.Value.(*rlwe.RelinearizationKey)
			if !ok || rlk == nil {
				return nil, fmt.Errorf("%w: relinearization material has value %T", ErrIncompatibleReusableMaterial, object.Value)
			}
			relinKey = rlk
		case KeyMaterialKindRotation:
			gk, ok := object.Value.(*rlwe.GaloisKey)
			if !ok || gk == nil {
				return nil, fmt.Errorf("%w: rotation material has value %T", ErrIncompatibleReusableMaterial, object.Value)
			}
			galoisKeys[gk.GaloisElement] = gk
		case KeyMaterialKindRingSwitch, KeyMaterialKindDenseSparse:
			evk, ok := object.Value.(*rlwe.EvaluationKey)
			if !ok || evk == nil {
				return nil, fmt.Errorf("%w: switch material has value %T", ErrIncompatibleReusableMaterial, object.Value)
			}
			if err := assignTargetEvaluationKey(out, object.Key.Direction, evk); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("%w: unsupported key material kind %q", ErrIncompatibleReusableMaterial, object.Key.Kind)
		}
	}

	galEls := make([]uint64, 0, len(galoisKeys))
	for galEl := range galoisKeys {
		galEls = append(galEls, galEl)
	}
	sort.Slice(galEls, func(i, j int) bool { return galEls[i] < galEls[j] })
	gks := make([]*rlwe.GaloisKey, 0, len(galEls))
	for _, galEl := range galEls {
		gks = append(gks, galoisKeys[galEl])
	}
	out.MemEvaluationKeySet = rlwe.NewMemEvaluationKeySet(relinKey, gks...)
	return out, nil
}

func (p *TargetLevelMaterialPlan) BuildTargetEvaluator(pool *KeyMaterialPool, targetLevel int) (*Evaluator, error) {
	if p == nil {
		return nil, fmt.Errorf("%w: target-level material plan is nil", ErrMissingReusableMaterial)
	}
	targetParams, ok := p.paramsByTargetLevel[targetLevel]
	if !ok {
		return nil, fmt.Errorf("%w: target level %d has no parameters", ErrTargetLevelUnavailable, targetLevel)
	}
	targetKeys, err := p.BuildTargetEvaluationKeys(pool, targetLevel)
	if err != nil {
		return nil, err
	}
	eval, err := NewEvaluator(targetParams, targetKeys)
	if err != nil {
		return nil, err
	}
	if outputLevel := eval.OutputLevel(); outputLevel != targetLevel {
		return nil, fmt.Errorf("%w: target level %d evaluator output level %d", ErrIncompatibleReusableMaterial, targetLevel, outputLevel)
	}
	return eval, nil
}

func ensureKeyMaterialMatchesManifestDomain(manifest TargetKeyMaterialManifest, object KeyMaterialObject) error {
	if object.Key.BootstrapSecretDomain == "" {
		return fmt.Errorf("%w: key material has empty bootstrap secret domain", ErrIncompatibleReusableMaterial)
	}
	if object.Key.BootstrapSecretDomain != manifest.BootstrapSecretDomain {
		return fmt.Errorf("%w: key material bootstrap domain %q differs from manifest domain %q", ErrIncompatibleReusableMaterial, object.Key.BootstrapSecretDomain, manifest.BootstrapSecretDomain)
	}
	switch object.Key.Kind {
	case KeyMaterialKindRelinearization, KeyMaterialKindRotation:
		if object.Key.SecretDomain != manifest.BootstrapSecretDomain {
			return fmt.Errorf("%w: key material domain %q differs from manifest domain %q", ErrIncompatibleReusableMaterial, object.Key.SecretDomain, manifest.BootstrapSecretDomain)
		}
	}
	return nil
}

func assignTargetEvaluationKey(keys *EvaluationKeys, direction string, evk *rlwe.EvaluationKey) error {
	switch direction {
	case "N1ToN2":
		keys.EvkN1ToN2 = evk
	case "N2ToN1":
		keys.EvkN2ToN1 = evk
	case "RealToCmplx":
		keys.EvkRealToCmplx = evk
	case "CmplxToReal":
		keys.EvkCmplxToReal = evk
	case "DenseToSparse":
		keys.EvkDenseToSparse = evk
	case "SparseToDense":
		keys.EvkSparseToDense = evk
	default:
		return fmt.Errorf("%w: unknown target evaluation-key direction %q", ErrIncompatibleReusableMaterial, direction)
	}
	return nil
}
