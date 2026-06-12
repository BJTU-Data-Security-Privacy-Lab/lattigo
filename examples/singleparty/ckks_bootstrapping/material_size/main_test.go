package main

import (
	"testing"

	"github.com/tuneinsight/lattigo/v6/ring"
)

func TestDefaultMaterialSizeParametersUseNormalRNSChain(t *testing.T) {
	params, btpParams, inputLevel, err := newMaterialSizeParameters(defaultLogN)
	if err != nil {
		t.Fatalf("newMaterialSizeParameters() error = %v", err)
	}

	if got, want := params.LogN(), 16; got != want {
		t.Fatalf("residual LogN = %d, want %d", got, want)
	}

	if got, want := params.RingType(), ring.Standard; got != want {
		t.Fatalf("residual RingType = %v, want %v", got, want)
	}

	if got, want := params.QCount(), 12; got != want {
		t.Fatalf("residual QCount = %d, want %d", got, want)
	}

	if got, want := btpParams.BootstrappingParameters.LogN(), 16; got != want {
		t.Fatalf("bootstrapping LogN = %d, want %d", got, want)
	}

	if got, want := inputLevel, 2; got != want {
		t.Fatalf("bootstrap input level = %d, want %d", got, want)
	}

	if got, want := btpParams.ResidualParameters.MaxLevel(), 11; got != want {
		t.Fatalf("bootstrap output level = %d, want %d", got, want)
	}
}
