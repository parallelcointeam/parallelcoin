package headerfs

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	
	"github.com/davecgh/go-spew/spew"
	
	"github.com/p9c/pod/pkg/chaincfg"
	"github.com/p9c/pod/pkg/chainhash"
	"github.com/p9c/pod/pkg/wire"
	"github.com/p9c/pod/pkg/walletdb"
)

func createTestBlockHeaderStore() (
	func(), walletdb.DB, string,
	*blockHeaderStore, error,
) {
	tempDir, e := ioutil.TempDir("", "store_test")
	if e != nil {
		return nil, nil, "", nil, e
	}
	dbPath := filepath.Join(tempDir, "test.db")
	db, e := walletdb.Create("bdb", dbPath)
	if e != nil {
		return nil, nil, "", nil, e
	}
	hStore, e := NewBlockHeaderStore(tempDir, db, &chaincfg.SimNetParams)
	if e != nil {
		return nil, nil, "", nil, e
	}
	cleanUp := func() {
		if e := os.RemoveAll(tempDir); E.Chk(e) {
		}
		if e := db.Close(); E.Chk(e) {
		}
	}
	return cleanUp, db, tempDir, hStore.(*blockHeaderStore), nil
}
func createTestBlockHeaderChain(numHeaders uint32) []BlockHeader {
	blockHeaders := make([]BlockHeader, numHeaders)
	prevHeader := &chaincfg.SimNetParams.GenesisBlock.Header
	for i := uint32(1); i <= numHeaders; i++ {
		bitcoinHeader := &wire.BlockHeader{
			Bits:      uint32(rand.Int31()),
			Nonce:     uint32(rand.Int31()),
			Timestamp: prevHeader.Timestamp.Add(time.Minute * 1),
			PrevBlock: prevHeader.BlockHash(),
		}
		blockHeaders[i-1] = BlockHeader{
			BlockHeader: bitcoinHeader,
			Height:      i,
		}
		prevHeader = bitcoinHeader
	}
	return blockHeaders
}
func TestBlockHeaderStoreOperations(t *testing.T) {
	cleanUp, _, _, bhs, e := createTestBlockHeaderStore()
	if cleanUp != nil {
		defer cleanUp()
	}
	if e != nil {
		t.Fatalf("unable to create new block header store: %v", e)
	}
	rand.Seed(time.Now().Unix())
	// With our test instance created, we'll now generate a series of "fake" block headers to insert into the database.
	const numHeaders = 100
	blockHeaders := createTestBlockHeaderChain(numHeaders)
	// With all the headers inserted, we'll now insert them into the database in a single batch.
	if e := bhs.WriteHeaders(blockHeaders...); E.Chk(e) {
		t.Fatalf("unable to write block headers: %v", e)
	}
	// At this point, the _tip_ of the chain from the PoV of the database should be the very last header we inserted.
	lastHeader := blockHeaders[len(blockHeaders)-1]
	tipHeader, tipHeight, e := bhs.ChainTip()
	if e != nil {
		t.Fatalf("unable to fetch chain tip")
	}
	if !reflect.DeepEqual(lastHeader.BlockHeader, tipHeader) {
		t.Fatalf(
			"tip height headers don't match up: "+
				"expected %v, got %v", spew.Sdump(lastHeader),
			spew.Sdump(tipHeader),
		)
	}
	if tipHeight != lastHeader.Height {
		t.Fatalf(
			"chain tip doesn't match: expected %v, got %v",
			lastHeader.Height, tipHeight,
		)
	}
	// Ensure that from the PoV of the database, the headers perfectly connect.
	if e := bhs.CheckConnectivity(); E.Chk(e) {
		t.Fatalf("bhs detects that headers don't connect: %v", e)
	}
	// With all the headers written, we should be able to retrieve each header according to its hash _and_ height.
	for _, header := range blockHeaders {
		dbHeader, e := bhs.FetchHeaderByHeight(header.Height)
		if e != nil {
			t.Fatalf("unable to fetch header by height: %v", e)
		}
		if !reflect.DeepEqual(*header.BlockHeader, *dbHeader) {
			t.Fatalf(
				"retrieved by height headers don't match up: "+
					"expected %v, got %v", spew.Sdump(*header.BlockHeader),
				spew.Sdump(*dbHeader),
			)
		}
		blockHash := header.BlockHash()
		dbHeader, _, e = bhs.FetchHeader(&blockHash)
		if e != nil {
			t.Fatalf("unable to fetch header by hash: %v", e)
		}
		if !reflect.DeepEqual(*dbHeader, *header.BlockHeader) {
			t.Fatalf(
				"retrieved by hash headers don't match up: "+
					"expected %v, got %v", spew.Sdump(header),
				spew.Sdump(dbHeader),
			)
		}
	}
	// Finally, we'll test the roll back scenario. Roll back the chain by a single block, the returned block stamp
	// should exactly match the last header inserted, and the current chain tip should be the second to last header
	// inserted.
	secondToLastHeader := blockHeaders[len(blockHeaders)-2]
	blockStamp, e := bhs.RollbackLastBlock()
	if e != nil {
		t.Fatalf("unable to rollback chain: %v", e)
	}
	if secondToLastHeader.Height != uint32(blockStamp.Height) {
		t.Fatalf(
			"chain tip doesn't match: expected %v, got %v",
			secondToLastHeader.Height, blockStamp.Height,
		)
	}
	headerHash := secondToLastHeader.BlockHash()
	if !bytes.Equal(headerHash[:], blockStamp.Hash[:]) {
		t.Fatalf(
			"header hashes don't match: expected %v, got %v",
			headerHash, blockStamp.Hash,
		)
	}
	tipHeader, tipHeight, e = bhs.ChainTip()
	if e != nil {
		t.Fatalf("unable to fetch chain tip")
	}
	if !reflect.DeepEqual(secondToLastHeader.BlockHeader, tipHeader) {
		t.Fatalf(
			"tip height headers don't match up: "+
				"expected %v, got %v", spew.Sdump(secondToLastHeader),
			spew.Sdump(tipHeader),
		)
	}
	if tipHeight != secondToLastHeader.Height {
		t.Fatalf(
			"chain tip doesn't match: expected %v, got %v",
			secondToLastHeader.Height, tipHeight,
		)
	}
}
func TestBlockHeaderStoreRecovery(t *testing.T) {
	// In this test we want to exercise the ability of the block header store to recover in the face of a partial batch
	// write (the headers were written, but the index wasn't updated).
	cleanUp, db, tempDir, bhs, e := createTestBlockHeaderStore()
	if cleanUp != nil {
		defer cleanUp()
	}
	if e != nil {
		t.Fatalf("unable to create new block header store: %v", e)
	}
	// First we'll generate a test header chain of length 10, inserting it into the header store.
	blockHeaders := createTestBlockHeaderChain(10)
	if e := bhs.WriteHeaders(blockHeaders...); E.Chk(e) {
		t.Fatalf("unable to write block headers: %v", e)
	}
	// Next, in order to simulate a partial write, we'll roll back the internal index by 5 blocks.
	for i := 0; i < 5; i++ {
		newTip := blockHeaders[len(blockHeaders)-i-1].PrevBlock
		if e := bhs.truncateIndex(&newTip, true); E.Chk(e) {
			t.Fatalf("unable to truncate index: %v", e)
		}
	}
	// Next, we'll re-create the block header store in order to trigger the recovery logic.
	hs, e := NewBlockHeaderStore(tempDir, db, &chaincfg.SimNetParams)
	if e != nil {
		t.Fatalf("unable to re-create bhs: %v", e)
	}
	bhs = hs.(*blockHeaderStore)
	// The chain tip of this new instance should be of height 5, and match the 5th to last block header.
	tipHash, tipHeight, e := bhs.ChainTip()
	if e != nil {
		t.Fatalf("unable to get chain tip: %v", e)
	}
	if tipHeight != 5 {
		t.Fatalf("tip height mismatch: expected %v, got %v", 5, tipHeight)
	}
	prevHeaderHash := blockHeaders[5].BlockHash()
	tipBlockHash := tipHash.BlockHash()
	if bytes.Equal(prevHeaderHash[:], tipBlockHash[:]) {
		t.Fatalf(
			"block hash mismatch: expected %v, got %v",
			prevHeaderHash, tipBlockHash,
		)
	}
}
func createTestFilterHeaderStore() (
	func(), walletdb.DB, string,
	*FilterHeaderStore, error,
) {
	tempDir, e := ioutil.TempDir("", "store_test")
	if e != nil {
		return nil, nil, "", nil, e
	}
	dbPath := filepath.Join(tempDir, "test.db")
	db, e := walletdb.Create("bdb", dbPath)
	if e != nil {
		return nil, nil, "", nil, e
	}
	hStore, e := NewFilterHeaderStore(
		tempDir, db, RegularFilter,
		&chaincfg.SimNetParams,
	)
	if e != nil {
		return nil, nil, "", nil, e
	}
	cleanUp := func() {
		if e := os.RemoveAll(tempDir); E.Chk(e) {
		}
		if e := db.Close(); E.Chk(e) {
		}
	}
	return cleanUp, db, tempDir, hStore, nil
}
func createTestFilterHeaderChain(numHeaders uint32) []FilterHeader {
	filterHeaders := make([]FilterHeader, numHeaders)
	for i := uint32(1); i <= numHeaders; i++ {
		filterHeaders[i-1] = FilterHeader{
			HeaderHash: chainhash.DoubleHashH([]byte{byte(i)}),
			FilterHash: sha256.Sum256([]byte{byte(i)}),
			Height:     i,
		}
	}
	return filterHeaders
}
func TestFilterHeaderStoreOperations(t *testing.T) {
	cleanUp, _, _, fhs, e := createTestFilterHeaderStore()
	if cleanUp != nil {
		defer cleanUp()
	}
	if e != nil {
		t.Fatalf("unable to create new block header store: %v", e)
	}
	rand.Seed(time.Now().Unix())
	// With our test instance created, we'll now generate a series of "fake" filter headers to insert into the database.
	const numHeaders = 100
	blockHeaders := createTestFilterHeaderChain(numHeaders)
	// We simulate the expected behavior of the block headers being written to disk before the filter headers are.
	if e := walletdb.Update(
		fhs.db, func(tx walletdb.ReadWriteTx) (e error) {
			rootBucket := tx.ReadWriteBucket(indexBucket)
			for _, header := range blockHeaders {
				var heightBytes [4]byte
				binary.BigEndian.PutUint32(heightBytes[:], header.Height)
				e := rootBucket.Put(header.HeaderHash[:], heightBytes[:])
				if e != nil {
					return e
				}
			}
			return nil
		},
	); E.Chk(e) {
		t.Fatalf("unable to pre-load block index: %v", e)
	}
	// With all the headers inserted, we'll now insert them into the database in a single batch.
	if e := fhs.WriteHeaders(blockHeaders...); E.Chk(e) {
		t.Fatalf("unable to write block headers: %v", e)
	}
	// At this point, the _tip_ of the chain from the PoV of the database should be the very last header we inserted.
	lastHeader := blockHeaders[len(blockHeaders)-1]
	tipHeader, tipHeight, e := fhs.ChainTip()
	if e != nil {
		t.Fatalf("unable to fetch chain tip")
	}
	if !bytes.Equal(lastHeader.FilterHash[:], tipHeader[:]) {
		t.Fatalf(
			"tip height headers don't match up: "+
				"expected %v, got %v", lastHeader, tipHeader,
		)
	}
	if tipHeight != lastHeader.Height {
		t.Fatalf(
			"chain tip doesn't match: expected %v, got %v",
			lastHeader.Height, tipHeight,
		)
	}
	// With all the headers written, we should be able to retrieve each header according to its hash _and_ height.
	for _, header := range blockHeaders {
		dbHeader, e := fhs.FetchHeaderByHeight(header.Height)
		if e != nil {
			t.Fatalf("unable to fetch header by height: %v", e)
		}
		if !bytes.Equal(header.FilterHash[:], dbHeader[:]) {
			t.Fatalf(
				"retrieved by height headers don't match up: "+
					"expected %v, got %v", header.FilterHash,
				dbHeader,
			)
		}
		blockHash := header.HeaderHash
		dbHeader, e = fhs.FetchHeader(&blockHash)
		if e != nil {
			t.Fatalf("unable to fetch header by hash: %v", e)
		}
		if !bytes.Equal(dbHeader[:], header.FilterHash[:]) {
			t.Fatalf(
				"retrieved by hash headers don't match up: "+
					"expected %v, got %v", spew.Sdump(header),
				spew.Sdump(dbHeader),
			)
		}
	}
	// Finally, we'll test the roll back scenario. Roll back the chain by a single block, the returned block stamp
	// should exactly match the last header inserted, and the current chain tip should be the second to last header
	// inserted.
	secondToLastHeader := blockHeaders[len(blockHeaders)-2]
	blockStamp, e := fhs.RollbackLastBlock(&secondToLastHeader.HeaderHash)
	if e != nil {
		t.Fatalf("unable to rollback chain: %v", e)
	}
	if secondToLastHeader.Height != uint32(blockStamp.Height) {
		t.Fatalf(
			"chain tip doesn't match: expected %v, got %v",
			secondToLastHeader.Height, blockStamp.Height,
		)
	}
	if !bytes.Equal(secondToLastHeader.FilterHash[:], blockStamp.Hash[:]) {
		t.Fatalf(
			"header hashes don't match: expected %v, got %v",
			secondToLastHeader.FilterHash, blockStamp.Hash,
		)
	}
	tipHeader, tipHeight, e = fhs.ChainTip()
	if e != nil {
		t.Fatalf("unable to fetch chain tip")
	}
	if !bytes.Equal(secondToLastHeader.FilterHash[:], tipHeader[:]) {
		t.Fatalf(
			"tip height headers don't match up: "+
				"expected %v, got %v", spew.Sdump(secondToLastHeader),
			spew.Sdump(tipHeader),
		)
	}
	if tipHeight != secondToLastHeader.Height {
		t.Fatalf(
			"chain tip doesn't match: expected %v, got %v",
			secondToLastHeader.Height, tipHeight,
		)
	}
}
func TestFilterHeaderStoreRecovery(t *testing.T) {
	// In this test we want to exercise the ability of the filter header store to recover in the face of a partial batch
	// write (the headers were written, but the index wasn't updated).
	cleanUp, db, tempDir, fhs, e := createTestFilterHeaderStore()
	if cleanUp != nil {
		defer cleanUp()
	}
	if e != nil {
		t.Fatalf("unable to create new block header store: %v", e)
	}
	blockHeaders := createTestFilterHeaderChain(10)
	// We simulate the expected behavior of the block headers being written to disk before the filter headers are.
	if e := walletdb.Update(
		fhs.db, func(tx walletdb.ReadWriteTx) (e error) {
			rootBucket := tx.ReadWriteBucket(indexBucket)
			for _, header := range blockHeaders {
				var heightBytes [4]byte
				binary.BigEndian.PutUint32(heightBytes[:], header.Height)
				e := rootBucket.Put(header.HeaderHash[:], heightBytes[:])
				if e != nil {
					return e
				}
			}
			return nil
		},
	); E.Chk(e) {
		t.Fatalf("unable to pre-load block index: %v", e)
	}
	// Next, we'll insert the filter header chain itself in to the database.
	if e := fhs.WriteHeaders(blockHeaders...); E.Chk(e) {
		t.Fatalf("unable to write block headers: %v", e)
	}
	// Next, in order to simulate a partial write, we'll roll back the internal index by 5 blocks.
	for i := 0; i < 5; i++ {
		newTip := blockHeaders[len(blockHeaders)-i-2].HeaderHash
		if e := fhs.truncateIndex(&newTip, true); E.Chk(e) {
			t.Fatalf("unable to truncate index: %v", e)
		}
	}
	// Next, we'll re-create the block header store in order to trigger the recovery logic.
	fhs, e = NewFilterHeaderStore(
		tempDir, db, RegularFilter,
		&chaincfg.SimNetParams,
	)
	if e != nil {
		t.Fatalf("unable to re-create bhs: %v", e)
	}
	// The chain tip of this new instance should be of height 5, and match the 5th to last filter header.
	tipHash, tipHeight, e := fhs.ChainTip()
	if e != nil {
		t.Fatalf("unable to get chain tip: %v", e)
	}
	if tipHeight != 5 {
		t.Fatalf("tip height mismatch: expected %v, got %v", 5, tipHeight)
	}
	prevHeaderHash := blockHeaders[5].FilterHash
	if bytes.Equal(prevHeaderHash[:], tipHash[:]) {
		t.Fatalf(
			"block hash mismatch: expected %v, got %v",
			prevHeaderHash, tipHash[:],
		)
	}
}

// TestBlockHeadersFetchHeaderAncestors tests that we're able to properly fetch the ancestors of a particular block,
// going from a set distance back to the target block.
func TestBlockHeadersFetchHeaderAncestors(t *testing.T) {
	t.Parallel()
	cleanUp, _, _, bhs, e := createTestBlockHeaderStore()
	if cleanUp != nil {
		defer cleanUp()
	}
	if e != nil {
		t.Fatalf("unable to create new block header store: %v", e)
	}
	rand.Seed(time.Now().Unix())
	// With our test instance created, we'll now generate a series of "fake" block headers to insert into the database.
	const numHeaders = 100
	blockHeaders := createTestBlockHeaderChain(numHeaders)
	// With all the headers inserted, we'll now insert them into the database in a single batch.
	if e := bhs.WriteHeaders(blockHeaders...); E.Chk(e) {
		t.Fatalf("unable to write block headers: %v", e)
	}
	// Now that the headers have been written to disk, we'll attempt to query for all the ancestors of the final header
	// written, to query the entire range.
	lastHeader := blockHeaders[numHeaders-1]
	lastHash := lastHeader.BlockHash()
	diskHeaders, startHeight, e := bhs.FetchHeaderAncestors(
		numHeaders-1, &lastHash,
	)
	if e != nil {
		t.Fatalf("unable to fetch headers: %v", e)
	}
	// Ensure that the first height of the block is height 1, and not the genesis block.
	if startHeight != 1 {
		t.Fatalf("expected start height of %v got %v", 1, startHeight)
	}
	// Ensure that we retrieve the correct number of headers.
	if len(diskHeaders) != numHeaders {
		t.Fatalf(
			"expected %v headers got %v headers",
			numHeaders, len(diskHeaders),
		)
	}
	// We should get back the exact same set of headers that we inserted in the first place.
	for i := 0; i < len(diskHeaders); i++ {
		diskHeader := diskHeaders[i]
		blockHeader := blockHeaders[i].BlockHeader
		if !reflect.DeepEqual(diskHeader, *blockHeader) {
			t.Fatalf(
				"header mismatch, expected %v got %v",
				spew.Sdump(blockHeader), spew.Sdump(diskHeader),
			)
		}
	}
}

// TODO(roasbeef): combined re-org scenarios
