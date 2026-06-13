package bootstrapping

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/tuneinsight/lattigo/v6/core/rlwe"
)

type BootstrapSecretDomain struct {
	OwnerLevel     int
	ParametersHash string
	SecretDomain   string
}

type TargetKeyMaterialRequirement struct {
	TargetLevel           int
	CandidateOwnerLevel   int
	BootstrapSecretDomain string
	Key                   KeyMaterialKey
	Shareable             bool
}

type KeyMaterialKind string

const (
	KeyMaterialKindRelinearization KeyMaterialKind = "relinearization_key"
	KeyMaterialKindRotation        KeyMaterialKind = "rotation_key"
	KeyMaterialKindRingSwitch      KeyMaterialKind = "ring_switch_key"
	KeyMaterialKindDenseSparse     KeyMaterialKind = "dense_sparse_key"
)

type KeyMaterialKey struct {
	Kind                  KeyMaterialKind
	LevelQ                int
	LevelP                int
	ParametersHash        string
	BootstrapSecretDomain string
	SecretDomain          string
	GaloisElement         uint64
	Direction             string
	DescriptorHash        string
}

type KeyMaterialView string

const (
	KeyMaterialViewPhysical  KeyMaterialView = "physical"
	KeyMaterialViewShared    KeyMaterialView = "shared"
	KeyMaterialViewPrivate   KeyMaterialView = "private"
	KeyMaterialViewRNSPrefix KeyMaterialView = "rns_prefix"
)

type KeyMaterialObject struct {
	Key       KeyMaterialKey
	View      KeyMaterialView
	Owner     int
	Targets   []int
	Value     any
	BinaryLen int
}

type TargetKeyMaterialManifest struct {
	TargetLevel           int
	KeyMaterialOwnerLevel int
	BootstrapSecretDomain string
	Shared                []KeyMaterialKey
	Private               []KeyMaterialKey
}

type KeyMaterialPool struct {
	objects   map[KeyMaterialKey]KeyMaterialObject
	manifests map[int]TargetKeyMaterialManifest
}

func NewKeyMaterialPool() *KeyMaterialPool {
	return &KeyMaterialPool{
		objects:   map[KeyMaterialKey]KeyMaterialObject{},
		manifests: map[int]TargetKeyMaterialManifest{},
	}
}

func NewBootstrapSecretDomainDescriptor(ownerLevel int, params Parameters) BootstrapSecretDomain {
	paramHash := targetLevelParametersHash(params)
	return BootstrapSecretDomain{
		OwnerLevel:     ownerLevel,
		ParametersHash: paramHash,
		SecretDomain:   hashStrings("bootstrap-secret-domain", strconv.Itoa(ownerLevel), paramHash),
	}
}

func (p *KeyMaterialPool) Add(owner int, targets []int, key KeyMaterialKey, view KeyMaterialView, value any, binaryLen int) error {
	if p == nil {
		return fmt.Errorf("%w: key material pool is nil", ErrMissingReusableMaterial)
	}
	if !isAllowedKeyMaterialKind(key.Kind) {
		return fmt.Errorf("%w: unsupported key material kind %q", ErrIncompatibleReusableMaterial, key.Kind)
	}
	if !isAllowedKeyMaterialView(view) {
		return fmt.Errorf("%w: unsupported key material view %q", ErrIncompatibleReusableMaterial, view)
	}
	if !isAllowedKeyMaterialValue(key.Kind, value) {
		return fmt.Errorf("%w: key material kind %q has incompatible value %T", ErrIncompatibleReusableMaterial, key.Kind, value)
	}
	if binaryLen < 0 {
		return fmt.Errorf("%w: key material binary length %d is negative", ErrIncompatibleReusableMaterial, binaryLen)
	}

	canonicalTargets, err := canonicalKeyMaterialTargets(targets)
	if err != nil {
		return err
	}

	if existing, ok := p.objects[key]; ok {
		if existing.Owner != owner || existing.View != view || existing.BinaryLen != binaryLen || !sameKeyMaterialValue(existing.Value, value) {
			return fmt.Errorf("%w: conflicting physical key material for key %+v", ErrIncompatibleReusableMaterial, key)
		}
		existing.Targets = mergeKeyMaterialTargets(existing.Targets, canonicalTargets)
		p.objects[key] = existing
		p.refreshManifestRefs(key)
		return nil
	}

	p.objects[key] = KeyMaterialObject{
		Key:       key,
		View:      view,
		Owner:     owner,
		Targets:   canonicalTargets,
		Value:     value,
		BinaryLen: binaryLen,
	}
	p.refreshManifestRefs(key)
	return nil
}

func (p *KeyMaterialPool) Object(key KeyMaterialKey) (KeyMaterialObject, bool) {
	if p == nil {
		return KeyMaterialObject{}, false
	}
	object, ok := p.objects[key]
	if !ok {
		return KeyMaterialObject{}, false
	}
	object.Targets = append([]int(nil), object.Targets...)
	return object, true
}

func (p *KeyMaterialPool) ObjectsByKind(kind KeyMaterialKind) []KeyMaterialObject {
	if p == nil {
		return nil
	}
	objects := []KeyMaterialObject{}
	for _, object := range p.objects {
		if object.Key.Kind != kind {
			continue
		}
		object.Targets = append([]int(nil), object.Targets...)
		objects = append(objects, object)
	}
	sort.Slice(objects, func(i, j int) bool {
		return keyMaterialKeySortString(objects[i].Key) < keyMaterialKeySortString(objects[j].Key)
	})
	return objects
}

func (p *KeyMaterialPool) ManifestForTarget(targetLevel int) (TargetKeyMaterialManifest, bool) {
	if p == nil {
		return TargetKeyMaterialManifest{}, false
	}
	manifest, ok := p.manifests[targetLevel]
	if !ok {
		return TargetKeyMaterialManifest{}, false
	}
	manifest.Shared = append([]KeyMaterialKey(nil), manifest.Shared...)
	manifest.Private = append([]KeyMaterialKey(nil), manifest.Private...)
	return manifest, true
}

func (p *KeyMaterialPool) SetManifestBootstrapSecretDomain(targetLevel, ownerLevel int, domain string) error {
	if p == nil {
		return fmt.Errorf("%w: key material pool is nil", ErrMissingReusableMaterial)
	}
	if domain == "" {
		return fmt.Errorf("%w: bootstrap secret domain must not be empty", ErrIncompatibleReusableMaterial)
	}
	manifest, ok := p.manifests[targetLevel]
	if !ok {
		return fmt.Errorf("%w: target level %d has no key material manifest", ErrMissingReusableMaterial, targetLevel)
	}
	if manifest.BootstrapSecretDomain != "" && (manifest.BootstrapSecretDomain != domain || manifest.KeyMaterialOwnerLevel != ownerLevel) {
		return fmt.Errorf("%w: target level %d manifest already uses owner=%d domain=%q", ErrIncompatibleReusableMaterial, targetLevel, manifest.KeyMaterialOwnerLevel, manifest.BootstrapSecretDomain)
	}
	manifest.KeyMaterialOwnerLevel = ownerLevel
	manifest.BootstrapSecretDomain = domain
	p.manifests[targetLevel] = manifest
	return nil
}

func (p *KeyMaterialPool) CountByKind(kind KeyMaterialKind) int {
	if p == nil {
		return 0
	}
	count := 0
	for _, object := range p.objects {
		if object.Key.Kind == kind {
			count++
		}
	}
	return count
}

func (p *KeyMaterialPool) PhysicalMaterialCount() int {
	if p == nil {
		return 0
	}
	return len(p.objects)
}

func (p *KeyMaterialPool) MaterialBinarySize() int {
	if p == nil {
		return 0
	}
	total := 0
	for _, object := range p.objects {
		total += object.BinaryLen
	}
	return total
}

func (p *KeyMaterialPool) SharedMaterialCount() int {
	if p == nil {
		return 0
	}
	count := 0
	for _, object := range p.objects {
		if len(object.Targets) > 1 {
			count++
		}
	}
	return count
}

func (p *KeyMaterialPool) SharedMaterialBinarySize() int {
	if p == nil {
		return 0
	}
	total := 0
	for _, object := range p.objects {
		if len(object.Targets) > 1 {
			total += object.BinaryLen
		}
	}
	return total
}

func (p *KeyMaterialPool) PrivateMaterialCount() int {
	if p == nil {
		return 0
	}
	count := 0
	for _, object := range p.objects {
		if len(object.Targets) == 1 {
			count++
		}
	}
	return count
}

func (p *KeyMaterialPool) PrivateMaterialBinarySize() int {
	if p == nil {
		return 0
	}
	total := 0
	for _, object := range p.objects {
		if len(object.Targets) == 1 {
			total += object.BinaryLen
		}
	}
	return total
}

func (p *KeyMaterialPool) refreshManifestRefs(key KeyMaterialKey) {
	object := p.objects[key]
	for targetLevel, manifest := range p.manifests {
		manifest.Shared = removeKeyMaterialKey(manifest.Shared, key)
		manifest.Private = removeKeyMaterialKey(manifest.Private, key)
		p.manifests[targetLevel] = manifest
	}

	for _, targetLevel := range object.Targets {
		manifest, ok := p.manifests[targetLevel]
		if !ok {
			manifest.TargetLevel = targetLevel
		}
		if len(object.Targets) > 1 {
			manifest.Shared = appendKeyMaterialKeyOnce(manifest.Shared, key)
		} else {
			manifest.Private = appendKeyMaterialKeyOnce(manifest.Private, key)
		}
		p.manifests[targetLevel] = manifest
	}
}

func isAllowedKeyMaterialKind(kind KeyMaterialKind) bool {
	switch kind {
	case KeyMaterialKindRelinearization, KeyMaterialKindRotation, KeyMaterialKindRingSwitch, KeyMaterialKindDenseSparse:
		return true
	default:
		return false
	}
}

func isAllowedKeyMaterialView(view KeyMaterialView) bool {
	switch view {
	case KeyMaterialViewPhysical, KeyMaterialViewShared, KeyMaterialViewPrivate, KeyMaterialViewRNSPrefix:
		return true
	default:
		return false
	}
}

func isAllowedKeyMaterialValue(kind KeyMaterialKind, value any) bool {
	switch kind {
	case KeyMaterialKindRelinearization:
		v, ok := value.(*rlwe.RelinearizationKey)
		return ok && v != nil
	case KeyMaterialKindRotation:
		v, ok := value.(*rlwe.GaloisKey)
		return ok && v != nil
	case KeyMaterialKindRingSwitch, KeyMaterialKindDenseSparse:
		v, ok := value.(*rlwe.EvaluationKey)
		return ok && v != nil
	default:
		return false
	}
}

func sameKeyMaterialValue(left, right any) bool {
	switch l := left.(type) {
	case *rlwe.RelinearizationKey:
		r, ok := right.(*rlwe.RelinearizationKey)
		return ok && l == r
	case *rlwe.GaloisKey:
		r, ok := right.(*rlwe.GaloisKey)
		return ok && l == r
	case *rlwe.EvaluationKey:
		r, ok := right.(*rlwe.EvaluationKey)
		return ok && l == r
	default:
		return false
	}
}

func canonicalKeyMaterialTargets(targets []int) ([]int, error) {
	canonical, err := canonicalTargetLevels(targets, "KeyMaterialObject.Targets")
	if err != nil {
		return nil, err
	}
	if len(canonical) == 0 {
		return nil, fmt.Errorf("%w: key material targets must not be empty", ErrTargetLevelUnavailable)
	}
	return canonical, nil
}

func mergeKeyMaterialTargets(left, right []int) []int {
	seen := make(map[int]struct{}, len(left)+len(right))
	out := make([]int, 0, len(left)+len(right))
	for _, target := range append(append([]int(nil), left...), right...) {
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		out = append(out, target)
	}
	sort.Ints(out)
	return out
}

func removeKeyMaterialKey(keys []KeyMaterialKey, remove KeyMaterialKey) []KeyMaterialKey {
	out := keys[:0]
	for _, key := range keys {
		if key != remove {
			out = append(out, key)
		}
	}
	return append([]KeyMaterialKey(nil), out...)
}

func appendKeyMaterialKeyOnce(keys []KeyMaterialKey, add KeyMaterialKey) []KeyMaterialKey {
	for _, key := range keys {
		if key == add {
			return keys
		}
	}
	return append(keys, add)
}

func keyMaterialKeySortString(key KeyMaterialKey) string {
	return fmt.Sprintf("%s/%d/%d/%s/%s/%s/%d/%s/%s", key.Kind, key.LevelQ, key.LevelP, key.ParametersHash, key.BootstrapSecretDomain, key.SecretDomain, key.GaloisElement, key.Direction, key.DescriptorHash)
}
