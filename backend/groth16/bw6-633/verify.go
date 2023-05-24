// Copyright 2020 ConsenSys Software Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Code generated by gnark DO NOT EDIT

package groth16

import (
	"errors"
	"fmt"
	"github.com/consensys/gnark-crypto/ecc"
	curve "github.com/consensys/gnark-crypto/ecc/bw6-633"
	"github.com/consensys/gnark-crypto/ecc/bw6-633/fr"
	"github.com/consensys/gnark/logger"
	"io"
	"math/big"
	"time"
)

var (
	errPairingCheckFailed         = errors.New("pairing doesn't match")
	errCorrectSubgroupCheckFailed = errors.New("points in the proof are not in the correct subgroup")
)

// Verify verifies a proof with given VerifyingKey and publicWitness
func Verify(proof *Proof, vk *VerifyingKey, publicWitness fr.Vector) error {

	nbPublicVars := len(vk.G1.K) - len(vk.PublicCommitted)

	if len(publicWitness) != nbPublicVars-1 {
		return fmt.Errorf("invalid witness size, got %d, expected %d (public - ONE_WIRE)", len(publicWitness), len(vk.G1.K)-1)
	}
	log := logger.Logger().With().Str("curve", vk.CurveID().String()).Str("backend", "groth16").Logger()
	start := time.Now()

	// check that the points in the proof are in the correct subgroup
	if !proof.isValid() {
		return errCorrectSubgroupCheckFailed
	}

	var doubleML curve.GT
	chDone := make(chan error, 1)

	// compute (eKrsδ, eArBs)
	go func() {
		var errML error
		doubleML, errML = curve.MillerLoop([]curve.G1Affine{proof.Krs, proof.Ar}, []curve.G2Affine{vk.G2.deltaNeg, proof.Bs})
		chDone <- errML
		close(chDone)
	}()

	for i := range vk.PublicCommitted {

		if err := vk.CommitmentKey.Verify(proof.Commitments[i], proof.CommitmentPok); err != nil {
			return err
		}

		publicCommitted := make([]*big.Int, len(vk.PublicCommitted[i]))
		for j := range publicCommitted {
			var b big.Int
			publicWitness[vk.PublicCommitted[i][j]-1].BigInt(&b)
			publicCommitted[j] = &b
		}

		if res, err := solveCommitmentWire(&proof.Commitments[i], publicCommitted); err != nil {
			return err
		} else {
			publicWitness = append(publicWitness, res)
		}
	}

	// compute e(Σx.[Kvk(t)]1, -[γ]2)
	var kSum curve.G1Jac
	if _, err := kSum.MultiExp(vk.G1.K[1:], publicWitness, ecc.MultiExpConfig{}); err != nil {
		return err
	}
	kSum.AddMixed(&vk.G1.K[0])

	for i := range proof.Commitments {
		kSum.AddMixed(&proof.Commitments[i])
	}

	var kSumAff curve.G1Affine
	kSumAff.FromJacobian(&kSum)

	right, err := curve.MillerLoop([]curve.G1Affine{kSumAff}, []curve.G2Affine{vk.G2.gammaNeg})
	if err != nil {
		return err
	}

	// wait for (eKrsδ, eArBs)
	if err := <-chDone; err != nil {
		return err
	}

	right = curve.FinalExponentiation(&right, &doubleML)
	if !vk.e.Equal(&right) {
		return errPairingCheckFailed
	}

	log.Debug().Dur("took", time.Since(start)).Msg("verifier done")
	return nil
}

// ExportSolidity not implemented for BW6-633
func (vk *VerifyingKey) ExportSolidity(w io.Writer) error {
	return errors.New("not implemented")
}
