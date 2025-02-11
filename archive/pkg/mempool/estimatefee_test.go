package mempool

import (
	"bytes"
	"github.com/p9c/pod/pkg/amt"
	block2 "github.com/p9c/pod/pkg/block"
	"math/rand"
	"testing"
	
	"github.com/p9c/pod/pkg/chainhash"
	"github.com/p9c/pod/pkg/mining"
	"github.com/p9c/pod/pkg/util"
	"github.com/p9c/pod/pkg/wire"
)

// estimateFeeTester interacts with the FeeEstimator to keep track of its expected state.
type estimateFeeTester struct {
	ef      *FeeEstimator
	t       *testing.T
	version int32
	height  int32
	last    *lastBlock
}

// lastBlock is a linked list of the block hashes which have been processed by the test FeeEstimator.
type lastBlock struct {
	hash *chainhash.Hash
	prev *lastBlock
}

func (eft *estimateFeeTester) checkSaveAndRestore(
	previousEstimates [estimateFeeDepth]DUOPerKilobyte,
) {
	// Get the save state.
	save := eft.ef.Save()
	// Save and restore database.
	var e error
	eft.ef, e = RestoreFeeEstimator(save)
	if e != nil {
		eft.t.Fatalf("Could not restore database: %s", e)
	}
	// Save again and check that it matches the previous one.
	redo := eft.ef.Save()
	if !bytes.Equal(save, redo) {
		eft.t.Fatalf("Restored states do not match: %v %v", save, redo)
	}
	// Chk that the results match.
	newEstimates := eft.estimates()
	for i, prev := range previousEstimates {
		if prev != newEstimates[i] {
			eft.t.Error(
				"Mismatch in estimate ", i, " after restore; got ",
				newEstimates[i], " but expected ", prev,
			)
		}
	}
}

func (eft *estimateFeeTester) estimates() [estimateFeeDepth]DUOPerKilobyte {
	// Generate estimates
	var estimates [estimateFeeDepth]DUOPerKilobyte
	for i := 0; i < estimateFeeDepth; i++ {
		estimates[i], _ = eft.ef.EstimateFee(uint32(i + 1))
	}
	// Chk that all estimated fee results go in descending order.
	for i := 1; i < estimateFeeDepth; i++ {
		if estimates[i] > estimates[i-1] {
			eft.t.Error(
				"Estimates not in descending order; got ",
				estimates[i], " for estimate ", i, " and ", estimates[i-1],
				" for ", i-1,
			)
			panic("invalid state.")
		}
	}
	return estimates
}

func (eft *estimateFeeTester) newBlock(txs []*wire.MsgTx) {
	eft.height++
	block := block2.NewBlock(&wire.Block{Transactions: txs})
	block.SetHeight(eft.height)
	eft.last = &lastBlock{block.Hash(), eft.last}
	e := eft.ef.RegisterBlock(block)
	if e != nil {
		W.Ln("failed to register block:", e)
	}
}

func (eft *estimateFeeTester) rollback() {
	if eft.last == nil {
		return
	}
	e := eft.ef.Rollback(eft.last.hash)
	if e != nil {
		eft.t.Errorf("Could not rollback: %v", e)
	}
	eft.height--
	eft.last = eft.last.prev
}

func (eft *estimateFeeTester) round(
	txHistory [][]*TxDesc,
	estimateHistory [][estimateFeeDepth]DUOPerKilobyte, txPerRound,
	txPerBlock uint32,
) ([][]*TxDesc, [][estimateFeeDepth]DUOPerKilobyte) {
	// generate new txs.
	var newTxs []*TxDesc
	for i := uint32(0); i < txPerRound; i++ {
		newTx := eft.testTx(amt.Amount(rand.Intn(1000000)))
		eft.ef.ObserveTransaction(newTx)
		newTxs = append(newTxs, newTx)
	}
	// Generate mempool.
	mempool := make(map[*observedTransaction]*TxDesc)
	for _, h := range txHistory {
		for _, t := range h {
			if o, exists := eft.ef.observed[*t.Tx.Hash()]; exists && o.
				mined == mining.UnminedHeight {
				mempool[o] = t
			}
		}
	}
	// generate new block, with no duplicates.
	i := uint32(0)
	newBlockList := make([]*wire.MsgTx, 0, txPerBlock)
	for _, t := range mempool {
		newBlockList = append(newBlockList, t.TxDesc.Tx.MsgTx())
		i++
		if i == txPerBlock {
			break
		}
	}
	// Register a new block.
	eft.newBlock(newBlockList)
	// return results.
	estimates := eft.estimates()
	// Return results
	return append(txHistory, newTxs), append(estimateHistory, estimates)
}

func (
eft *estimateFeeTester,
) testTx(
	fee amt.Amount,
) *TxDesc {
	eft.version++
	return &TxDesc{
		TxDesc: mining.TxDesc{
			Tx: util.NewTx(
				&wire.MsgTx{
					Version: eft.version,
				},
			),
			Height: eft.height,
			Fee:    int64(fee),
		},
		StartingPriority: 0,
	}
}

// TestSave tests saving and restoring to a []byte.
func TestDatabase(t *testing.T) {
	txPerRound := uint32(7)
	txPerBlock := uint32(5)
	binSize := uint32(6)
	maxReplacements := uint32(4)
	rounds := 8
	eft := estimateFeeTester{ef: newTestFeeEstimator(binSize, maxReplacements, uint32(rounds)+1), t: t}
	var txHistory [][]*TxDesc
	estimateHistory := [][estimateFeeDepth]DUOPerKilobyte{eft.estimates()}
	for round := 0; round < rounds; round++ {
		eft.checkSaveAndRestore(estimateHistory[len(estimateHistory)-1])
		// Go forward one step.
		txHistory, estimateHistory =
			eft.round(txHistory, estimateHistory, txPerRound, txPerBlock)
	}
	// Reverse the process and try again.
	for round := 1; round <= rounds; round++ {
		eft.rollback()
		eft.checkSaveAndRestore(estimateHistory[len(estimateHistory)-round-1])
	}
}

// TestEstimateFee tests basic functionality in the FeeEstimator.
func TestEstimateFee(t *testing.T) {
	ef := newTestFeeEstimator(5, 3, 1)
	eft := estimateFeeTester{ef: ef, t: t}
	// Try with no txs and get zero for all queries.
	expected := DUOPerKilobyte(0.0)
	for i := uint32(1); i <= estimateFeeDepth; i++ {
		estimated, _ := ef.EstimateFee(i)
		if estimated != expected {
			t.Errorf(
				"Estimate fee error: expected %f when estimator is"+
					" empty; got %f", expected, estimated,
			)
		}
	}
	// Now insert a tx.
	tx := eft.testTx(1000000)
	ef.ObserveTransaction(tx)
	// Expected should still be zero because this is still in the mempool.
	expected = DUOPerKilobyte(0.0)
	for i := uint32(1); i <= estimateFeeDepth; i++ {
		estimated, _ := ef.EstimateFee(i)
		if estimated != expected {
			t.Errorf(
				"Estimate fee error: expected %f when estimator has one"+
					" tx in mempool; got %f", expected, estimated,
			)
		}
	}
	// Change minRegisteredBlocks to make sure that works. Error return value expected.
	ef.minRegisteredBlocks = 1
	expected = DUOPerKilobyte(-1.0)
	for i := uint32(1); i <= estimateFeeDepth; i++ {
		estimated, _ := ef.EstimateFee(i)
		if estimated != expected {
			t.Errorf(
				"Estimate fee error: expected %f before any blocks have"+
					" been registered; got %f", expected, estimated,
			)
		}
	}
	// Record a block with the new tx.
	eft.newBlock([]*wire.MsgTx{tx.Tx.MsgTx()})
	expected = expectedFeePerKilobyte(tx)
	for i := uint32(1); i <= estimateFeeDepth; i++ {
		estimated, _ := ef.EstimateFee(i)
		if estimated != expected {
			t.Errorf(
				"Estimate fee error: expected %f when one tx is binned"+
					"; got %f", expected, estimated,
			)
		}
	}
	// Roll back the last block; this was an orphan block.
	ef.minRegisteredBlocks = 0
	eft.rollback()
	expected = DUOPerKilobyte(0.0)
	for i := uint32(1); i <= estimateFeeDepth; i++ {
		estimated, _ := ef.EstimateFee(i)
		if estimated != expected {
			t.Errorf(
				"Estimate fee error: expected %f after rolling back"+
					" block; got %f", expected, estimated,
			)
		}
	}
	// Record an empty block and then a block with the new tx. This test was made because of a bug that only appeared
	// when there were no transactions in the first bin.
	eft.newBlock([]*wire.MsgTx{})
	eft.newBlock([]*wire.MsgTx{tx.Tx.MsgTx()})
	expected = expectedFeePerKilobyte(tx)
	for i := uint32(1); i <= estimateFeeDepth; i++ {
		estimated, _ := ef.EstimateFee(i)
		if estimated != expected {
			t.Errorf(
				"Estimate fee error: expected %f when one tx is binned"+
					"; got %f", expected, estimated,
			)
		}
	}
	// Create some more transactions.
	txA := eft.testTx(500000)
	txB := eft.testTx(2000000)
	txC := eft.testTx(4000000)
	ef.ObserveTransaction(txA)
	ef.ObserveTransaction(txB)
	ef.ObserveTransaction(txC)
	// Record 7 empty blocks.
	for i := 0; i < 7; i++ {
		eft.newBlock([]*wire.MsgTx{})
	}
	// Mine the first tx.
	eft.newBlock([]*wire.MsgTx{txA.Tx.MsgTx()})
	// Now the estimated amount should depend on the value of the argument to estimate fee.
	for i := uint32(1); i <= estimateFeeDepth; i++ {
		estimated, _ := ef.EstimateFee(i)
		if i > 2 {
			expected = expectedFeePerKilobyte(txA)
		} else {
			expected = expectedFeePerKilobyte(tx)
		}
		if estimated != expected {
			t.Errorf(
				"Estimate fee error: expected %f on round %d; got %f",
				expected, i, estimated,
			)
		}
	}
	// Record 5 more empty blocks.
	for i := 0; i < 5; i++ {
		eft.newBlock([]*wire.MsgTx{})
	}
	// Mine the next tx.
	eft.newBlock([]*wire.MsgTx{txB.Tx.MsgTx()})
	// Now the estimated amount should depend on the value of the argument to estimate fee.
	for i := uint32(1); i <= estimateFeeDepth; i++ {
		estimated, _ := ef.EstimateFee(i)
		switch {
		case i <= 2:
			expected = expectedFeePerKilobyte(txB)
		case i <= 8:
			expected = expectedFeePerKilobyte(tx)
		default:
			expected = expectedFeePerKilobyte(txA)
		}
		if estimated != expected {
			t.Errorf(
				"Estimate fee error: expected %f on round %d; got %f",
				expected, i, estimated,
			)
		}
	}
	// Record 9 more empty blocks.
	for i := 0; i < 10; i++ {
		eft.newBlock([]*wire.MsgTx{})
	}
	// Mine txC.
	eft.newBlock([]*wire.MsgTx{txC.Tx.MsgTx()})
	// This should have no effect on the outcome because too many blocks have been mined for txC to be recorded.
	for i := uint32(1); i <= estimateFeeDepth; i++ {
		estimated, _ := ef.EstimateFee(i)
		switch {
		case i <= 2:
			expected = expectedFeePerKilobyte(txC)
		case i <= 8:
			expected = expectedFeePerKilobyte(txB)
		case i <= 8+6:
			expected = expectedFeePerKilobyte(tx)
		default:
			expected = expectedFeePerKilobyte(txA)
		}
		if estimated != expected {
			t.Errorf("Estimate fee error: expected %f on round %d; got %f", expected, i, estimated)
		}
	}
}

// TestEstimateFeeRollback tests the rollback function, which undoes the effect of a adding a new block.
func TestEstimateFeeRollback(t *testing.T) {
	txPerRound := uint32(7)
	txPerBlock := uint32(5)
	binSize := uint32(6)
	maxReplacements := uint32(4)
	stepsBack := 2
	rounds := 30
	eft := estimateFeeTester{
		ef: newTestFeeEstimator(
			binSize,
			maxReplacements, uint32(stepsBack),
		), t: t,
	}
	var txHistory [][]*TxDesc
	estimateHistory := [][estimateFeeDepth]DUOPerKilobyte{eft.estimates()}
	for round := 0; round < rounds; round++ {
		// Go forward a few rounds.
		for step := 0; step <= stepsBack; step++ {
			txHistory, estimateHistory =
				eft.round(txHistory, estimateHistory, txPerRound, txPerBlock)
		}
		// Now go back.
		for step := 0; step < stepsBack; step++ {
			eft.rollback()
			// After rolling back we should have the same estimated fees as before.
			expected := estimateHistory[len(estimateHistory)-step-2]
			estimates := eft.estimates()
			// Ensure that these are both the same.
			for i := 0; i < estimateFeeDepth; i++ {
				if expected[i] != estimates[i] {
					t.Errorf(
						"Rollback value mismatch. Expected %f, got %f. ",
						expected[i], estimates[i],
					)
					return
				}
			}
		}
		// Erase history.
		txHistory = txHistory[0 : len(txHistory)-stepsBack]
		estimateHistory = estimateHistory[0 : len(estimateHistory)-stepsBack]
	}
}
func expectedFeePerKilobyte(t *TxDesc) DUOPerKilobyte {
	size := float64(t.TxDesc.Tx.MsgTx().SerializeSize())
	fee := float64(t.TxDesc.Fee)
	return SatoshiPerByte(fee / size).ToBtcPerKb()
}

// newTestFeeEstimator creates a feeEstimator with some different parameters for testing purposes.
func newTestFeeEstimator(binSize, maxReplacements, maxRollback uint32) *FeeEstimator {
	return &FeeEstimator{
		maxRollback:         maxRollback,
		lastKnownHeight:     0,
		binSize:             int32(binSize),
		minRegisteredBlocks: 0,
		maxReplacements:     int32(maxReplacements),
		observed:            make(map[chainhash.Hash]*observedTransaction),
		dropped:             make([]*registeredBlock, 0, maxRollback),
	}
}
