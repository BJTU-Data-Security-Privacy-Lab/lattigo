// Package main reports the size of the main CKKS bootstrapping materials.
//
// The default profile uses a normal N=2^16 CKKS parameter set with 12 RNS Q
// limbs and reports the materials needed to bootstrap from input level 2 to
// the residual parameter maximum level.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/tuneinsight/lattigo/v6/circuits/ckks/bootstrapping"
	"github.com/tuneinsight/lattigo/v6/circuits/ckks/dft"
	"github.com/tuneinsight/lattigo/v6/core/rlwe"
	"github.com/tuneinsight/lattigo/v6/ring"
	"github.com/tuneinsight/lattigo/v6/schemes/ckks"
	"github.com/tuneinsight/lattigo/v6/utils"
)

var (
	flagLogN    = flag.Int("logN", defaultLogN, "bootstrap and residual parameter LogN")
	flagVerbose = flag.Bool("verbose", false, "print per-Galois-key and per-DFT-transform material rows")
)

const (
	defaultLogN                = 16
	defaultBootstrapInputLevel = 2
)

type materialRow struct {
	group string
	name  string
	count int
	bytes int
}

type matrixMaterialStats struct {
	diagonalCount int
	bytes         int
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	flag.Parse()

	if *flagLogN < 2 {
		return fmt.Errorf("-logN must be at least 2")
	}

	params, btpParams, bootstrapInputLevel, err := newMaterialSizeParameters(*flagLogN)
	if err != nil {
		return err
	}

	sk := rlwe.NewKeyGenerator(params).GenSecretKeyNew()

	evk, _, err := btpParams.GenEvaluationKeys(sk)
	if err != nil {
		return fmt.Errorf("cannot generate bootstrapping evaluation keys: %w", err)
	}

	eval, err := bootstrapping.NewEvaluator(btpParams, evk)
	if err != nil {
		return fmt.Errorf("cannot instantiate bootstrapping evaluator: %w", err)
	}

	rows, err := materialRows(evk, eval, *flagVerbose)
	if err != nil {
		return err
	}

	printParameterSummary(params, btpParams, bootstrapInputLevel)
	printRows(rows)

	return nil
}

func newMaterialSizeParameters(logN int) (params ckks.Parameters, btpParams bootstrapping.Parameters, bootstrapInputLevel int, err error) {
	paramsLit := ckks.ParametersLiteral{
		LogN:            logN,
		LogQ:            []int{55, 40, 40, 40, 40, 40, 40, 40, 40, 40, 40, 40},
		LogP:            []int{61, 61, 61, 61},
		LogDefaultScale: 40,
		Xs:              ring.Ternary{H: 192},
	}

	if params, err = ckks.NewParametersFromLiteral(paramsLit); err != nil {
		return params, btpParams, 0, fmt.Errorf("cannot instantiate residual parameters: %w", err)
	}

	btpParamsLit := bootstrapping.ParametersLiteral{
		LogN: utils.Pointy(logN),
		LogP: []int{61, 61, 61, 61},
		Xs:   params.Xs(),
	}

	if btpParams, err = bootstrapping.NewParametersFromLiteral(params, btpParamsLit); err != nil {
		return params, btpParams, 0, fmt.Errorf("cannot instantiate bootstrapping parameters: %w", err)
	}

	return params, btpParams, defaultBootstrapInputLevel, nil
}

func materialRows(evk *bootstrapping.EvaluationKeys, eval *bootstrapping.Evaluator, verbose bool) ([]materialRow, error) {
	rows := []materialRow{}

	ringSwitchRows := []materialRow{
		evaluationKeyRow("ring_domain_switch_key", "EvkN1ToN2", evk.EvkN1ToN2),
		evaluationKeyRow("ring_domain_switch_key", "EvkN2ToN1", evk.EvkN2ToN1),
		evaluationKeyRow("ring_domain_switch_key", "EvkRealToCmplx", evk.EvkRealToCmplx),
		evaluationKeyRow("ring_domain_switch_key", "EvkCmplxToReal", evk.EvkCmplxToReal),
	}
	rows = append(rows, ringSwitchRows...)
	rows = append(rows, totalRow("ring_domain_switch_key", "ring_domain_switch_total", ringSwitchRows))

	rlk, err := evk.GetRelinearizationKey()
	if err != nil {
		return nil, fmt.Errorf("cannot get relinearization key: %w", err)
	}
	rows = append(rows, materialRow{
		group: "relinearization_key",
		name:  "relinearization_key",
		count: 1,
		bytes: rlk.BinarySize(),
	})

	galoisRows, galoisTotal, err := galoisKeyRows(evk)
	if err != nil {
		return nil, err
	}
	if verbose {
		rows = append(rows, galoisRows...)
	}
	rows = append(rows, galoisTotal)

	denseSparseRows := []materialRow{
		evaluationKeyRow("dense_sparse_key", "EvkDenseToSparse", evk.EvkDenseToSparse),
		evaluationKeyRow("dense_sparse_key", "EvkSparseToDense", evk.EvkSparseToDense),
	}
	rows = append(rows, denseSparseRows...)
	rows = append(rows, totalRow("dense_sparse_key", "dense_sparse_total", denseSparseRows))

	c2sRows, c2sTotal := dftMatrixRows("coeffs_to_slots", eval.C2SDFTMatrix)
	s2cRows, s2cTotal := dftMatrixRows("slots_to_coeffs", eval.S2CDFTMatrix)
	if verbose {
		rows = append(rows, c2sRows...)
		rows = append(rows, s2cRows...)
	}
	rows = append(rows, c2sTotal, s2cTotal)
	rows = append(rows, materialRow{
		group: "dft_encoded_diagonal",
		name:  "dft_encoded_diagonal_total",
		count: c2sTotal.count + s2cTotal.count,
		bytes: c2sTotal.bytes + s2cTotal.bytes,
	})

	return rows, nil
}

func evaluationKeyRow(group, name string, key *rlwe.EvaluationKey) materialRow {
	if key == nil {
		return materialRow{group: group, name: name}
	}

	return materialRow{
		group: group,
		name:  name,
		count: 1,
		bytes: key.BinarySize(),
	}
}

func totalRow(group, name string, rows []materialRow) materialRow {
	total := materialRow{
		group: group,
		name:  name,
	}

	for _, row := range rows {
		total.count += row.count
		total.bytes += row.bytes
	}

	return total
}

func galoisKeyRows(evk *bootstrapping.EvaluationKeys) (rows []materialRow, total materialRow, err error) {
	galEls := evk.GetGaloisKeysList()
	sort.Slice(galEls, func(i, j int) bool { return galEls[i] < galEls[j] })

	total = materialRow{
		group: "galois_key",
		name:  "galois_keys_total",
		count: len(galEls),
	}

	for _, galEl := range galEls {
		gk, err := evk.GetGaloisKey(galEl)
		if err != nil {
			return nil, materialRow{}, fmt.Errorf("cannot get Galois key %d: %w", galEl, err)
		}

		row := materialRow{
			group: "galois_key",
			name:  fmt.Sprintf("galEl_%d", galEl),
			count: 1,
			bytes: gk.BinarySize(),
		}
		rows = append(rows, row)
		total.bytes += row.bytes
	}

	return rows, total, nil
}

func dftMatrixRows(matrixName string, matrix dft.Matrix) (rows []materialRow, total materialRow) {
	total = materialRow{
		group: "dft_encoded_diagonal",
		name:  matrixName + "_encoded_diagonal_total",
	}

	for transformIndex, lt := range matrix.Matrices {
		stats := matrixMaterialStats{
			diagonalCount: len(lt.Vec),
		}

		for _, poly := range lt.Vec {
			stats.bytes += poly.BinarySize()
		}

		rows = append(rows, materialRow{
			group: "dft_transform",
			name:  fmt.Sprintf("%s_transform_%02d", matrixName, transformIndex),
			count: stats.diagonalCount,
			bytes: stats.bytes,
		})

		total.count += stats.diagonalCount
		total.bytes += stats.bytes
	}

	return rows, total
}

func printParameterSummary(params ckks.Parameters, btpParams bootstrapping.Parameters, bootstrapInputLevel int) {
	fmt.Println("CKKS bootstrapping material size")
	fmt.Printf("Residual parameters: logN=%d, ringType=%s, maxLevelQ=%d, maxLevelP=%d, logQP=%.2f\n",
		params.LogN(),
		params.RingType(),
		params.MaxLevelQ(),
		params.MaxLevelP(),
		params.LogQP())
	fmt.Printf("Bootstrapping parameters: logN=%d, ringType=%s, maxLevelQ=%d, maxLevelP=%d, logQP=%.2f\n",
		btpParams.BootstrappingParameters.LogN(),
		btpParams.BootstrappingParameters.RingType(),
		btpParams.BootstrappingParameters.MaxLevelQ(),
		btpParams.BootstrappingParameters.MaxLevelP(),
		btpParams.BootstrappingParameters.LogQP())
	fmt.Printf("Bootstrap levels: bootstrap_input_level=%d, bootstrap_output_level=%d\n",
		bootstrapInputLevel,
		btpParams.ResidualParameters.MaxLevel())
	fmt.Println()
}

func printRows(rows []materialRow) {
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "group\tname\tcount\tbytes\tmb")

	for _, row := range rows {
		fmt.Fprintf(writer, "%s\t%s\t%d\t%d\t%.6f\n",
			row.group,
			row.name,
			row.count,
			row.bytes,
			float64(row.bytes)/(1024*1024))
	}

	_ = writer.Flush()
}
