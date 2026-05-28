package bootstrapping

import (
	"fmt"
	"sort"

	"github.com/tuneinsight/lattigo/v6/core/rlwe"
)

type targetLevelBootstrapper struct {
	evaluators map[int]*Evaluator
	levels     []int
}

func newTargetLevelBootstrapper(evaluators map[int]*Evaluator) (*targetLevelBootstrapper, error) {
	if len(evaluators) == 0 {
		return nil, fmt.Errorf("cannot create target-level bootstrapper: evaluator map is empty")
	}

	levels := make([]int, 0, len(evaluators))
	copied := make(map[int]*Evaluator, len(evaluators))
	for targetLevel, eval := range evaluators {
		if eval == nil {
			return nil, fmt.Errorf("cannot create target-level bootstrapper: evaluator for target level %d is nil", targetLevel)
		}
		if outputLevel := eval.OutputLevel(); outputLevel != targetLevel {
			return nil, fmt.Errorf("cannot create target-level bootstrapper: evaluator for target level %d has OutputLevel=%d", targetLevel, outputLevel)
		}

		levels = append(levels, targetLevel)
		copied[targetLevel] = eval
	}
	sort.Ints(levels)

	return &targetLevelBootstrapper{
		evaluators: copied,
		levels:     levels,
	}, nil
}

func (b *targetLevelBootstrapper) bootstrapAtLevel(ct *rlwe.Ciphertext, targetLevel int) (*rlwe.Ciphertext, error) {
	eval, err := b.evaluatorAtLevel(targetLevel)
	if err != nil {
		return nil, err
	}
	return eval.Bootstrap(ct)
}

func (b *targetLevelBootstrapper) bootstrapManyAtLevel(cts []rlwe.Ciphertext, targetLevel int) ([]rlwe.Ciphertext, error) {
	eval, err := b.evaluatorAtLevel(targetLevel)
	if err != nil {
		return nil, err
	}
	return eval.BootstrapMany(cts)
}

func (b *targetLevelBootstrapper) outputLevel(targetLevel int) (int, error) {
	eval, err := b.evaluatorAtLevel(targetLevel)
	if err != nil {
		return 0, err
	}
	return eval.OutputLevel(), nil
}

func (b *targetLevelBootstrapper) targetLevels() []int {
	if b == nil {
		return nil
	}
	return append([]int(nil), b.levels...)
}

func (b *targetLevelBootstrapper) evaluatorAtLevel(targetLevel int) (*Evaluator, error) {
	if b == nil {
		return nil, fmt.Errorf("cannot bootstrap to target level %d: target-level bootstrapper is nil", targetLevel)
	}
	eval, ok := b.evaluators[targetLevel]
	if !ok {
		return nil, fmt.Errorf("cannot bootstrap to target level %d: target level is not configured", targetLevel)
	}
	return eval, nil
}
