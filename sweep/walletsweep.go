package sweep

import (
	"errors"
	"fmt"
	"math"

	"github.com/ltcsuite/lnd/fn"
	"github.com/ltcsuite/lnd/input"
	"github.com/ltcsuite/lnd/lnwallet"
	"github.com/ltcsuite/lnd/lnwallet/chainfee"
	"github.com/ltcsuite/ltcd/chaincfg/chainhash"
	"github.com/ltcsuite/ltcd/ltcutil"
	"github.com/ltcsuite/ltcd/ltcutil/mweb"
	"github.com/ltcsuite/ltcd/txscript"
	"github.com/ltcsuite/ltcd/wire"
	"github.com/ltcsuite/ltcwallet/wallet/txauthor"
	"github.com/ltcsuite/ltcwallet/wallet/txrules"
	"github.com/ltcsuite/ltcwallet/wallet/txsizes"
	"golang.org/x/exp/maps"
)

const (
	// defaultNumBlocksEstimate is the number of blocks that we fall back
	// to issuing an estimate for if a fee pre fence doesn't specify an
	// explicit conf target or fee rate.
	defaultNumBlocksEstimate = 6
)

var (
	// ErrUnknownUTXO is returned when creating a sweeping tx using an UTXO
	// that's unknown to the wallet.
	ErrUnknownUTXO = errors.New("unknown utxo")
)

// FeePreference allows callers to express their time value for inclusion of a
// transaction into a block via either a confirmation target, or a fee rate.
type FeePreference struct {
	// ConfTarget if non-zero, signals a fee preference expressed in the
	// number of desired blocks between first broadcast, and confirmation.
	ConfTarget uint32

	// FeeRate if non-zero, signals a fee pre fence expressed in the fee
	// rate expressed in sat/kw for a particular transaction.
	FeeRate chainfee.SatPerKWeight
}

// String returns a human-readable string of the fee preference.
func (p FeePreference) String() string {
	if p.ConfTarget != 0 {
		return fmt.Sprintf("%v blocks", p.ConfTarget)
	}
	return p.FeeRate.String()
}

// DetermineFeePerKw will determine the fee in sat/kw that should be paid given
// an estimator, a confirmation target, and a manual value for sat/byte. A
// value is chosen based on the two free parameters as one, or both of them can
// be zero.
func DetermineFeePerKw(feeEstimator chainfee.Estimator,
	feePref FeePreference) (chainfee.SatPerKWeight, error) {

	switch {
	// If both values are set, then we'll return an error as we require a
	// strict directive.
	case feePref.FeeRate != 0 && feePref.ConfTarget != 0:
		return 0, fmt.Errorf("only FeeRate or ConfTarget should " +
			"be set for FeePreferences")

	// If the target number of confirmations is set, then we'll use that to
	// consult our fee estimator for an adequate fee.
	case feePref.ConfTarget != 0:
		feePerKw, err := feeEstimator.EstimateFeePerKW(
			uint32(feePref.ConfTarget),
		)
		if err != nil {
			return 0, fmt.Errorf("unable to query fee "+
				"estimator: %v", err)
		}

		return feePerKw, nil

	// If a manual sat/byte fee rate is set, then we'll use that directly.
	// We'll need to convert it to sat/kw as this is what we use
	// internally.
	case feePref.FeeRate != 0:
		feePerKW := feePref.FeeRate

		// Because the user can specify 1 sat/vByte on the RPC
		// interface, which corresponds to 250 sat/kw, we need to bump
		// that to the minimum "safe" fee rate which is 253 sat/kw.
		if feePerKW == chainfee.AbsoluteFeePerKwFloor {
			log.Infof("Manual fee rate input of %d sat/kw is "+
				"too low, using %d sat/kw instead", feePerKW,
				chainfee.FeePerKwFloor)
			feePerKW = chainfee.FeePerKwFloor
		}

		// If that bumped fee rate of at least 253 sat/kw is still lower
		// than the relay fee rate, we return an error to let the user
		// know. Note that "Relay fee rate" may mean slightly different
		// things depending on the backend. For bitcoind, it is
		// effectively max(relay fee, min mempool fee).
		minFeePerKW := feeEstimator.RelayFeePerKW()
		if feePerKW < minFeePerKW {
			return 0, fmt.Errorf("manual fee rate input of %d "+
				"sat/kw is too low to be accepted into the "+
				"mempool or relayed to the network", feePerKW)
		}

		return feePerKW, nil

	// Otherwise, we'll attempt a relaxed confirmation target for the
	// transaction
	default:
		feePerKw, err := feeEstimator.EstimateFeePerKW(
			defaultNumBlocksEstimate,
		)
		if err != nil {
			return 0, fmt.Errorf("unable to query fee estimator: "+
				"%v", err)
		}

		return feePerKw, nil
	}
}

// UtxoSource is an interface that allows a caller to access a source of UTXOs
// to use when crafting sweep transactions.
type UtxoSource interface {
	// ListUnspentWitness returns all UTXOs from the default wallet account
	// that have between minConfs and maxConfs number of confirmations.
	ListUnspentWitnessFromDefaultAccount(minConfs, maxConfs int32) (
		[]*lnwallet.Utxo, error)
}

// CoinSelectionLocker is an interface that allows the caller to perform an
// operation, which is synchronized with all coin selection attempts. This can
// be used when an operation requires that all coin selection operations cease
// forward progress. Think of this as an exclusive lock on coin selection
// operations.
type CoinSelectionLocker interface {
	// WithCoinSelectLock will execute the passed function closure in a
	// synchronized manner preventing any coin selection operations from
	// proceeding while the closure is executing. This can be seen as the
	// ability to execute a function closure under an exclusive coin
	// selection lock.
	WithCoinSelectLock(func() error) error
}

// OutpointLocker allows a caller to lock/unlock an outpoint. When locked, the
// outpoints shouldn't be used for any sort of channel funding of coin
// selection. Locked outpoints are not expected to be persisted between restarts.
type OutpointLocker interface {
	// LockOutpoint locks a target outpoint, rendering it unusable for coin
	// selection.
	LockOutpoint(o wire.OutPoint)

	// UnlockOutpoint unlocks a target outpoint, allowing it to be used for
	// coin selection once again.
	UnlockOutpoint(o wire.OutPoint)
}

// MwebSweeper handles MWEB signing for sweep transactions. It bridges the
// sweep package to the wallet's MWEB signing infrastructure.
type MwebSweeper interface {
	// SignMwebSweepTx signs an AuthoredTx that may contain both MWEB and
	// canonical inputs. It processes MWEB inputs/outputs via AddMweb
	// (building the extension block), then signs canonical inputs.
	SignMwebSweepTx(tx *txauthor.AuthoredTx,
		feeRate chainfee.SatPerKWeight) error
}

// WalletSweepPackage is a package that gives the caller the ability to sweep
// relevant funds from a wallet in a single transaction. We also package a
// function closure that allows one to abort the operation.
type WalletSweepPackage struct {
	// SweepTx is a fully signed, and valid transaction that is broadcast,
	// will sweep ALL relevant confirmed coins in the wallet with a single
	// transaction.
	SweepTx *wire.MsgTx

	// CancelSweepAttempt allows the caller to cancel the sweep attempt.
	//
	// NOTE: If the sweeping transaction isn't or cannot be broadcast, then
	// this closure MUST be called, otherwise all selected utxos will be
	// unable to be used.
	CancelSweepAttempt func()
}

// DeliveryAddr is a pair of (address, amount) used to craft a transaction
// paying to more than one specified address.
type DeliveryAddr struct {
	// Addr is the address to pay to.
	Addr ltcutil.Address

	// Amt is the amount to pay to the given address.
	Amt ltcutil.Amount
}

// CraftSweepAllTx attempts to craft a WalletSweepPackage which will allow the
// caller to sweep ALL outputs within the wallet to a list of outputs. Any
// leftover amount after these outputs and transaction fee, is sent to a single
// output, as specified by the change address. The sweep transaction will be
// crafted with the target fee rate, and will use the utxoSource and
// outpointLocker as sources for wallet funds.
func CraftSweepAllTx(feeRate chainfee.SatPerKWeight, blockHeight uint32,
	deliveryAddrs []DeliveryAddr, changeAddr ltcutil.Address,
	coinSelectLocker CoinSelectionLocker, utxoSource UtxoSource,
	outpointLocker OutpointLocker, feeEstimator chainfee.Estimator,
	signer input.Signer, minConfs int32,
	selectUtxos fn.Set[wire.OutPoint],
	mwebSweeper MwebSweeper) (*WalletSweepPackage, error) {

	// TODO(roasbeef): turn off ATPL as well when available?

	var outputsForSweep []*lnwallet.Utxo

	// We'll make a function closure up front that allows us to unlock all
	// selected outputs to ensure that they become available again in the
	// case of an error after the outputs have been locked, but before we
	// can actually craft a sweeping transaction.
	unlockOutputs := func() {
		for _, utxo := range outputsForSweep {
			outpointLocker.UnlockOutpoint(utxo.OutPoint)
		}
	}

	// Next, we'll use the coinSelectLocker to ensure that no coin
	// selection takes place while we fetch and lock outputs in the
	// wallet. Otherwise, it may be possible for a new funding flow to lock
	// an output while we fetch the set of unspent witnesses.
	err := coinSelectLocker.WithCoinSelectLock(func() error {
		// Now that we can be sure that no other coin selection
		// operations are going on, we can grab a clean snapshot of the
		// current UTXO state of the wallet.
		utxos, err := utxoSource.ListUnspentWitnessFromDefaultAccount(
			minConfs, math.MaxInt32,
		)
		if err != nil {
			return err
		}

		log.Trace("[WithCoinSelectLock] finished fetching UTXOs")

		// Use select utxos, if provided.
		if len(selectUtxos) > 0 {
			utxos, err = fetchUtxosFromOutpoints(
				utxos, selectUtxos.ToSlice(),
			)
			if err != nil {
				return err
			}
		}

		// We'll now lock each UTXO to ensure that other callers don't
		// attempt to use these UTXOs in transactions while we're
		// crafting out sweep all transaction.
		for _, utxo := range utxos {
			outpointLocker.LockOutpoint(utxo.OutPoint)
		}

		log.Trace("[WithCoinSelectLock] exited the lock")

		outputsForSweep = append(outputsForSweep, utxos...)

		return nil
	})
	if err != nil {
		// If we failed at all, we'll unlock any outputs selected just
		// in case we had any lingering outputs.
		unlockOutputs()

		return nil, fmt.Errorf("unable to fetch+lock wallet utxos: %w",
			err)
	}

	// Create a list of TxOuts from the given delivery addresses.
	var txOuts []*wire.TxOut
	for _, d := range deliveryAddrs {
		pkScript, err := txscript.PayToAddrScript(d.Addr)
		if err != nil {
			unlockOutputs()
			return nil, err
		}

		txOuts = append(txOuts, &wire.TxOut{
			PkScript: pkScript,
			Value:    int64(d.Amt),
		})
	}

	// Convert the change addr to a pkScript.
	changePkScript, err := txscript.PayToAddrScript(changeAddr)
	if err != nil {
		unlockOutputs()
		return nil, err
	}

	// Classify outputs into MWEB and canonical, and check if any
	// output or the change address is MWEB.
	var (
		canonicalOutputs []*lnwallet.Utxo
		mwebOutputs      []*lnwallet.Utxo
	)
	for _, output := range outputsForSweep {
		if output.AddressType == lnwallet.Mweb {
			mwebOutputs = append(mwebOutputs, output)
		} else {
			canonicalOutputs = append(canonicalOutputs, output)
		}
	}

	hasMweb := len(mwebOutputs) > 0 || txscript.IsMweb(changePkScript)
	for _, txOut := range txOuts {
		if txscript.IsMweb(txOut.PkScript) {
			hasMweb = true
			break
		}
	}

	// If MWEB is involved, use the MWEB-aware sweep path.
	if hasMweb {
		if mwebSweeper == nil {
			// If no MwebSweeper is provided, skip MWEB UTXOs
			// with a warning instead of erroring.
			log.Warnf("MWEB UTXOs present but no MwebSweeper "+
				"provided, skipping %d MWEB UTXOs",
				len(mwebOutputs))

			// Fall through to canonical-only path below.
		} else {
			sweepTx, err := craftMwebSweepTx(
				canonicalOutputs, mwebOutputs, txOuts,
				changePkScript, feeRate, mwebSweeper,
			)
			if err != nil {
				unlockOutputs()
				return nil, err
			}

			return &WalletSweepPackage{
				SweepTx:            sweepTx,
				CancelSweepAttempt: unlockOutputs,
			}, nil
		}
	}

	// Canonical-only path: assemble inputs for the sweeper.
	var inputsToSweep []input.Input
	for _, output := range canonicalOutputs {
		signDesc := &input.SignDescriptor{
			Output: &wire.TxOut{
				PkScript: output.PkScript,
				Value:    int64(output.Value),
			},
			HashType: txscript.SigHashAll,
		}

		pkScript := output.PkScript

		var witnessType input.WitnessType
		switch output.AddressType {
		case lnwallet.WitnessPubKey:
			witnessType = input.WitnessKeyHash

		case lnwallet.NestedWitnessPubKey:
			witnessType = input.NestedWitnessKeyHash

		case lnwallet.TaprootPubkey:
			witnessType = input.TaprootPubKeySpend
			signDesc.HashType = txscript.SigHashDefault

		default:
			unlockOutputs()
			return nil, fmt.Errorf("unable to sweep coins, "+
				"unknown script: %x", pkScript[:])
		}

		input := input.MakeBaseInput(
			&output.OutPoint, witnessType, signDesc, 0, nil,
		)
		inputsToSweep = append(inputsToSweep, &input)
	}

	sweepTx, err := createSweepTx(
		inputsToSweep, txOuts, changePkScript, blockHeight, feeRate,
		signer,
	)
	if err != nil {
		unlockOutputs()
		return nil, err
	}

	return &WalletSweepPackage{
		SweepTx:            sweepTx,
		CancelSweepAttempt: unlockOutputs,
	}, nil
}

// craftMwebSweepTx builds a sweep transaction that handles MWEB inputs and/or
// MWEB outputs. It constructs an AuthoredTx with all inputs (canonical + MWEB),
// calculates fees for both canonical and MWEB portions, and delegates signing
// to the MwebSweeper.
func craftMwebSweepTx(canonicalUtxos, mwebUtxos []*lnwallet.Utxo,
	deliveryTxOuts []*wire.TxOut, changePkScript []byte,
	feeRate chainfee.SatPerKWeight,
	mwebSweeper MwebSweeper) (*wire.MsgTx, error) {

	feeRatePerKb := ltcutil.Amount(feeRate.FeePerKVByte())

	// Build the list of all TxIn entries, PrevScripts, PrevInputValues,
	// and PrevMwebOutputs. Canonical inputs come first, then MWEB.
	var (
		txIns           []*wire.TxIn
		prevScripts     [][]byte
		prevInputValues []ltcutil.Amount
		prevMwebOutputs []*wire.MwebOutput
		totalInput      ltcutil.Amount
	)

	// Count canonical input types for fee estimation.
	var numP2WPKH, numP2TR, numNested, numP2PKH int

	for _, utxo := range canonicalUtxos {
		txIns = append(txIns, wire.NewTxIn(
			&utxo.OutPoint, nil, nil,
		))
		prevScripts = append(prevScripts, utxo.PkScript)
		prevInputValues = append(prevInputValues, utxo.Value)
		prevMwebOutputs = append(prevMwebOutputs, nil)
		totalInput += utxo.Value

		switch utxo.AddressType {
		case lnwallet.WitnessPubKey:
			numP2WPKH++
		case lnwallet.TaprootPubkey:
			numP2TR++
		case lnwallet.NestedWitnessPubKey:
			numNested++
		default:
			numP2PKH++
		}
	}

	for _, utxo := range mwebUtxos {
		txIns = append(txIns, wire.NewTxIn(
			&utxo.OutPoint, nil, nil,
		))
		prevScripts = append(prevScripts, utxo.PkScript)
		prevInputValues = append(prevInputValues, utxo.Value)
		prevMwebOutputs = append(prevMwebOutputs, utxo.MwebOutput)
		totalInput += utxo.Value
	}

	// Calculate the delivery total.
	var deliveryTotal ltcutil.Amount
	for _, txOut := range deliveryTxOuts {
		deliveryTotal += ltcutil.Amount(txOut.Value)
	}

	// Build the output list for fee estimation. We include a placeholder
	// change output so MWEB fee estimation accounts for it.
	allOutputs := make([]*wire.TxOut, 0, len(deliveryTxOuts)+1)
	allOutputs = append(allOutputs, deliveryTxOuts...)
	changePlaceholder := &wire.TxOut{
		PkScript: changePkScript,
		Value:    0, // placeholder, will be set after fee calc
	}
	allOutputs = append(allOutputs, changePlaceholder)

	// Calculate MWEB fee.
	mwebFee := ltcutil.Amount(
		mweb.EstimateFee(allOutputs, feeRatePerKb, false),
	)

	// Calculate canonical fee (for the on-chain portion).
	var canonicalFee ltcutil.Amount
	if len(canonicalUtxos) > 0 {
		// If we have canonical inputs, the on-chain tx will have those
		// inputs plus a peg-in output (if there are MWEB outputs).
		var peginOutputs []*wire.TxOut
		hasMwebOutput := txscript.IsMweb(changePkScript)
		for _, txOut := range deliveryTxOuts {
			if txscript.IsMweb(txOut.PkScript) {
				hasMwebOutput = true
				break
			}
		}

		if hasMwebOutput || len(mwebUtxos) > 0 {
			// Add a dummy peg-in output for size estimation.
			peginOutputs = []*wire.TxOut{
				mweb.NewPegin(0, &chainhash.Hash{}),
			}
		} else {
			// Pure canonical outputs — include them for size est.
			peginOutputs = allOutputs
		}

		vsize := txsizes.EstimateVirtualSize(
			numP2PKH, numP2TR, numP2WPKH, numNested,
			peginOutputs, 0,
		)

		// Account for the peg-in TxIn weight, matching the
		// adjustment in txauthor.NewUnsignedTransaction.
		if hasMwebOutput || len(mwebUtxos) > 0 {
			vsize += new(wire.TxIn).SerializeSize()
		}

		canonicalFee = txrules.FeeForSerializeSize(
			feeRatePerKb, vsize,
		)
	}

	totalFee := canonicalFee + mwebFee
	changeAmt := totalInput - deliveryTotal - totalFee

	if changeAmt < 0 {
		return nil, fmt.Errorf("insufficient input to create sweep "+
			"tx: input_sum=%v, output_sum=%v, fee=%v",
			totalInput, deliveryTotal, totalFee)
	}

	// Set the actual change amount.
	changePlaceholder.Value = int64(changeAmt)

	// Build the unsigned transaction.
	sweepTx := &wire.MsgTx{
		Version:  wire.TxVersion,
		TxIn:     txIns,
		TxOut:    allOutputs,
		LockTime: 0,
	}

	authoredTx := &txauthor.AuthoredTx{
		Tx:              sweepTx,
		PrevScripts:     prevScripts,
		PrevInputValues: prevInputValues,
		PrevMwebOutputs: prevMwebOutputs,
		TotalInput:      totalInput,
		ChangeIndex:     -1,
	}

	// Sign the transaction: AddMweb handles the MWEB extension block,
	// then AddAllInputScripts signs the canonical inputs.
	if err := mwebSweeper.SignMwebSweepTx(authoredTx, feeRate); err != nil {
		return nil, fmt.Errorf("unable to sign mweb sweep tx: %w", err)
	}

	// After signing, verify that the total fee meets the minimum relay
	// fee based on the full serialized transaction size (including the
	// MWEB extension with range proofs, signatures, etc.).
	txSize := authoredTx.Tx.SerializeSize()
	minRelayFee := txrules.FeeForSerializeSize(
		txrules.DefaultRelayFeePerKb, txSize,
	)
	if totalFee < minRelayFee {
		// The MWEB extension bytes push the relay fee above our
		// estimate. Reduce the change output to cover the difference.
		deficit := minRelayFee - totalFee

		// Find and adjust the change output (the one we added).
		for _, txOut := range authoredTx.Tx.TxOut {
			if txOut.Value == int64(changeAmt) {
				txOut.Value -= int64(deficit)
				break
			}
		}

		// For MWEB transactions, the change is in the extension
		// block, so we need to rebuild. Re-sign with the corrected
		// change amount.
		changePlaceholder.Value = int64(changeAmt - deficit)

		// Rebuild the transaction with corrected outputs.
		sweepTx = &wire.MsgTx{
			Version:  wire.TxVersion,
			TxIn:     txIns,
			TxOut:    allOutputs,
			LockTime: 0,
		}
		authoredTx = &txauthor.AuthoredTx{
			Tx:              sweepTx,
			PrevScripts:     prevScripts,
			PrevInputValues: prevInputValues,
			PrevMwebOutputs: prevMwebOutputs,
			TotalInput:      totalInput,
			ChangeIndex:     -1,
		}

		if err := mwebSweeper.SignMwebSweepTx(authoredTx, feeRate); err != nil {
			return nil, fmt.Errorf("unable to sign mweb sweep "+
				"tx (relay fee adjustment): %w", err)
		}
	}

	return authoredTx.Tx, nil
}

// fetchUtxosFromOutpoints returns UTXOs for given outpoints. Errors if any
// outpoint is not in the passed slice of utxos.
func fetchUtxosFromOutpoints(utxos []*lnwallet.Utxo,
	outpoints []wire.OutPoint) ([]*lnwallet.Utxo, error) {

	lookup := fn.SliceToMap(utxos, func(utxo *lnwallet.Utxo) wire.OutPoint {
		return utxo.OutPoint
	}, func(utxo *lnwallet.Utxo) *lnwallet.Utxo {
		return utxo
	})

	subMap, err := fn.NewSubMap(lookup, outpoints)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnknownUTXO, err.Error())
	}

	fetchedUtxos := maps.Values(subMap)

	return fetchedUtxos, nil
}
