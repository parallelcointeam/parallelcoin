package votingpool

import (
	"bytes"
	"fmt"
	"testing"
	
	"github.com/p9c/pod/pkg/util/hdkeychain"
	"github.com/p9c/pod/pkg/waddrmgr"
)

func TestPoolEnsureUsedAddr(t *testing.T) {
	tearDown, db, pool := TstCreatePool(t)
	defer tearDown()
	dbtx, e := db.BeginReadWriteTx()
	if e != nil  {
		t.Fatal(e)
	}
	defer func() {
		e := dbtx.Commit()
		if e != nil  {
			t.Log(e)
		}
	}()
	ns, addrmgrNs := TstRWNamespaces(dbtx)
	var script []byte
	var addr waddrmgr.ManagedScriptAddress
	TstCreateSeries(t, dbtx, pool, []TstSeriesDef{{ReqSigs: 2, PubKeys: TstPubKeys[0:3], SeriesID: 1}})
	idx := Index(0)
	TstRunWithManagerUnlocked(t, pool.Manager(), addrmgrNs, func() {
		e = pool.EnsureUsedAddr(ns, addrmgrNs, 1, 0, idx)
	})
	if e != nil  {
		t.Fatalf("Failed to ensure used addresses: %v", e)
	}
	addr, e = pool.getUsedAddr(ns, addrmgrNs, 1, 0, 0)
	if e != nil  {
		t.Fatalf("Failed to get addr from used addresses set: %v", e)
	}
	TstRunWithManagerUnlocked(t, pool.Manager(), addrmgrNs, func() {
		script, e = addr.Script()
	})
	if e != nil  {
		t.Fatalf("Failed to get script: %v", e)
	}
	wantScript, _ := pool.DepositScript(1, 0, 0)
	if !bytes.Equal(script, wantScript) {
		t.Fatalf("Script from looked up addr is not what we expect")
	}
	idx = Index(3)
	TstRunWithManagerUnlocked(t, pool.Manager(), addrmgrNs, func() {
		e = pool.EnsureUsedAddr(ns, addrmgrNs, 1, 0, idx)
	})
	if e != nil  {
		t.Fatalf("Failed to ensure used addresses: %v", e)
	}
	for _, i := range []int{0, 1, 2, 3} {
		addr, e = pool.getUsedAddr(ns, addrmgrNs, 1, 0, Index(i))
		if e != nil  {
			t.Fatalf("Failed to get addr from used addresses set: %v", e)
		}
		TstRunWithManagerUnlocked(t, pool.Manager(), addrmgrNs, func() {
			script, e = addr.Script()
		})
		if e != nil  {
			t.Fatalf("Failed to get script: %v", e)
		}
		wantScript, _ := pool.DepositScript(1, 0, Index(i))
		if !bytes.Equal(script, wantScript) {
			t.Fatalf("Script from looked up addr is not what we expect")
		}
	}
}
func TestPoolGetUsedAddr(t *testing.T) {
	tearDown, db, pool := TstCreatePool(t)
	defer tearDown()
	dbtx, e := db.BeginReadWriteTx()
	if e != nil  {
		t.Fatal(e)
	}
	defer func() {
		e := dbtx.Commit()
		if e != nil  {
			t.Log(e)
		}
	}()
	ns, addrmgrNs := TstRWNamespaces(dbtx)
	TstCreateSeries(t, dbtx, pool, []TstSeriesDef{{ReqSigs: 2, PubKeys: TstPubKeys[0:3], SeriesID: 1}})
	// Addr with series=1, branch=0, index=10 has never been used, so it should return nil.
	addr, e := pool.getUsedAddr(ns, addrmgrNs, 1, 0, 10)
	if e != nil  {
		t.Fatalf("VPError when looking up used addr: %v", e)
	}
	if addr != nil {
		t.Fatalf("Unused address found in used addresses DB: %v", addr)
	}
	// Now we add that addr to the used addresses DB and check that the value returned by getUsedAddr() is what we
	// expect.
	TstRunWithManagerUnlocked(t, pool.Manager(), addrmgrNs, func() {
		e = pool.addUsedAddr(ns, addrmgrNs, 1, 0, 10)
	})
	if e != nil  {
		t.Fatalf("VPError when storing addr in used addresses DB: %v", e)
	}
	var script []byte
	addr, e = pool.getUsedAddr(ns, addrmgrNs, 1, 0, 10)
	if e != nil  {
		t.Fatalf("VPError when looking up used addr: %v", e)
	}
	TstRunWithManagerUnlocked(t, pool.Manager(), addrmgrNs, func() {
		script, e = addr.Script()
	})
	if e != nil  {
		t.Fatalf("Failed to get script: %v", e)
	}
	wantScript, _ := pool.DepositScript(1, 0, 10)
	if !bytes.Equal(script, wantScript) {
		t.Fatalf("Script from looked up addr is not what we expect")
	}
}
func TestSerializationErrors(t *testing.T) {
	tearDown, db, pool := TstCreatePool(t)
	defer tearDown()
	dbtx, e := db.BeginReadWriteTx()
	if e != nil  {
		t.Fatal(e)
	}
	defer func() {
		e := dbtx.Commit()
		if e != nil  {
			t.Log(e)
		}
	}()
	_, addrmgrNs := TstRWNamespaces(dbtx)
	tests := []struct {
		version  uint32
		pubKeys  []string
		privKeys []string
		reqSigs  uint32
		e        ErrorCode
	}{
		{
			version: 2,
			pubKeys: TstPubKeys[0:3],
			e:       ErrSeriesVersion,
		},
		{
			pubKeys: []string{"NONSENSE"},
			// Not a valid length public key.
			e: ErrSeriesSerialization,
		},
		{
			pubKeys:  TstPubKeys[0:3],
			privKeys: TstPrivKeys[0:1],
			// The number of public and private keys should be the same.
			e: ErrSeriesSerialization,
		},
		{
			pubKeys:  TstPubKeys[0:1],
			privKeys: []string{"NONSENSE"},
			// Not a valid length private key.
			e: ErrSeriesSerialization,
		},
	}
	active := true
	for testNum, test := range tests {
		encryptedPubs, e := encryptKeys(test.pubKeys, pool.Manager(), waddrmgr.CKTPublic)
		if e != nil  {
			t.Fatalf("Test #%d - VPError encrypting pubkeys: %v", testNum, e)
		}
		var encryptedPrivs [][]byte
		TstRunWithManagerUnlocked(t, pool.Manager(), addrmgrNs, func() {
			encryptedPrivs, e = encryptKeys(test.privKeys, pool.Manager(), waddrmgr.CKTPrivate)
		})
		if e != nil  {
			t.Fatalf("Test #%d - VPError encrypting privkeys: %v", testNum, e)
		}
		row := &dbSeriesRow{
			version:           test.version,
			active:            active,
			reqSigs:           test.reqSigs,
			pubKeysEncrypted:  encryptedPubs,
			privKeysEncrypted: encryptedPrivs}
		_, e = serializeSeriesRow(row)
		TstCheckError(t, fmt.Sprintf("Test #%d", testNum), e, test.e)
	}
}
func TestSerialization(t *testing.T) {
	tearDown, db, pool := TstCreatePool(t)
	defer tearDown()
	dbtx, e := db.BeginReadWriteTx()
	if e != nil  {
		t.Fatal(e)
	}
	defer func() {
		e := dbtx.Commit()
		if e != nil  {
			t.Log(e)
		}
	}()
	_, addrmgrNs := TstRWNamespaces(dbtx)
	tests := []struct {
		version  uint32
		active   bool
		pubKeys  []string
		privKeys []string
		reqSigs  uint32
	}{
		{
			version: 1,
			active:  true,
			pubKeys: TstPubKeys[0:1],
			reqSigs: 1,
		},
		{
			version:  0,
			active:   false,
			pubKeys:  TstPubKeys[0:1],
			privKeys: TstPrivKeys[0:1],
			reqSigs:  1,
		},
		{
			pubKeys:  TstPubKeys[0:3],
			privKeys: []string{TstPrivKeys[0], "", ""},
			reqSigs:  2,
		},
		{
			pubKeys: TstPubKeys[0:5],
			reqSigs: 3,
		},
		{
			pubKeys:  TstPubKeys[0:7],
			privKeys: []string{"", TstPrivKeys[1], "", TstPrivKeys[3], "", "", ""},
			reqSigs:  4,
		},
	}
	var encryptedPrivs [][]byte
	for testNum, test := range tests {
		encryptedPubs, e := encryptKeys(test.pubKeys, pool.Manager(), waddrmgr.CKTPublic)
		if e != nil  {
			t.Fatalf("Test #%d - VPError encrypting pubkeys: %v", testNum, e)
		}
		TstRunWithManagerUnlocked(t, pool.Manager(), addrmgrNs, func() {
			encryptedPrivs, e = encryptKeys(test.privKeys, pool.Manager(), waddrmgr.CKTPrivate)
		})
		if e != nil  {
			t.Fatalf("Test #%d - VPError encrypting privkeys: %v", testNum, e)
		}
		row := &dbSeriesRow{
			version:           test.version,
			active:            test.active,
			reqSigs:           test.reqSigs,
			pubKeysEncrypted:  encryptedPubs,
			privKeysEncrypted: encryptedPrivs,
		}
		serialized, e := serializeSeriesRow(row)
		if e != nil  {
			t.Fatalf("Test #%d - VPError in serialization %v", testNum, e)
		}
		row, e = deserializeSeriesRow(serialized)
		if e != nil  {
			t.Fatalf("Test #%d - Failed to deserialize %v %v", testNum, serialized, e)
		}
		if row.version != test.version {
			t.Errorf("Serialization #%d - version mismatch: got %d want %d",
				testNum, row.version, test.version)
		}
		if row.active != test.active {
			t.Errorf("Serialization #%d - active mismatch: got %v want %v",
				testNum, row.active, test.active)
		}
		if row.reqSigs != test.reqSigs {
			t.Errorf("Serialization #%d - row reqSigs off. Got %d, want %d",
				testNum, row.reqSigs, test.reqSigs)
		}
		if len(row.pubKeysEncrypted) != len(test.pubKeys) {
			t.Errorf("Serialization #%d - Wrong no. of pubkeys. Got %d, want %d",
				testNum, len(row.pubKeysEncrypted), len(test.pubKeys))
		}
		for i, encryptedPub := range encryptedPubs {
			got := string(row.pubKeysEncrypted[i])
			if got != string(encryptedPub) {
				t.Errorf("Serialization #%d - Pubkey deserialization. Got %v, want %v",
					testNum, got, string(encryptedPub))
			}
		}
		if len(row.privKeysEncrypted) != len(row.pubKeysEncrypted) {
			t.Errorf("Serialization #%d - no. privkeys (%d) != no. pubkeys (%d)",
				testNum, len(row.privKeysEncrypted), len(row.pubKeysEncrypted))
		}
		for i, encryptedPriv := range encryptedPrivs {
			got := string(row.privKeysEncrypted[i])
			if got != string(encryptedPriv) {
				t.Errorf("Serialization #%d - Privkey deserialization. Got %v, want %v",
					testNum, got, string(encryptedPriv))
			}
		}
	}
}
func TestDeserializationErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		serialized []byte
		e          ErrorCode
	}{
		{
			serialized: make([]byte, seriesMaxSerial+1),
			// Too many bytes (over seriesMaxSerial).
			e: ErrSeriesSerialization,
		},
		{
			serialized: make([]byte, seriesMinSerial-1),
			// Not enough bytes (under seriesMinSerial).
			e: ErrSeriesSerialization,
		},
		{
			serialized: []byte{
				1, 0, 0, 0, // 4 bytes (version)
				0,          // 1 byte (active)
				2, 0, 0, 0, // 4 bytes (reqSigs)
				3, 0, 0, 0, // 4 bytes (nKeys)
			},
			// Here we have the constant data but are missing any public/private keys.
			e: ErrSeriesSerialization,
		},
		{
			serialized: []byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			// Unsupported version.
			e: ErrSeriesVersion,
		},
	}
	for testNum, test := range tests {
		_, e := deserializeSeriesRow(test.serialized)
		TstCheckError(t, fmt.Sprintf("Test #%d", testNum), e, test.e)
	}
}
func TestValidateAndDecryptKeys(t *testing.T) {
	tearDown, db, pool := TstCreatePool(t)
	defer tearDown()
	dbtx, e := db.BeginReadWriteTx()
	if e != nil  {
		t.Fatal(e)
	}
	defer func() {
		e := dbtx.Commit()
		if e != nil  {
			t.Log(e)
		}
	}()
	_, addrmgrNs := TstRWNamespaces(dbtx)
	rawPubKeys, e := encryptKeys(TstPubKeys[0:2], pool.Manager(), waddrmgr.CKTPublic)
	if e != nil  {
		t.Fatalf("Failed to encrypt public keys: %v", e)
	}
	var rawPrivKeys [][]byte
	TstRunWithManagerUnlocked(t, pool.Manager(), addrmgrNs, func() {
		rawPrivKeys, e = encryptKeys([]string{TstPrivKeys[0], ""}, pool.Manager(), waddrmgr.CKTPrivate)
	})
	if e != nil  {
		t.Fatalf("Failed to encrypt private keys: %v", e)
	}
	var pubKeys, privKeys []*hdkeychain.ExtendedKey
	TstRunWithManagerUnlocked(t, pool.Manager(), addrmgrNs, func() {
		pubKeys, privKeys, e = validateAndDecryptKeys(rawPubKeys, rawPrivKeys, pool)
	})
	if e != nil  {
		t.Fatalf("VPError when validating/decrypting keys: %v", e)
	}
	if len(pubKeys) != 2 {
		t.Fatalf("Unexpected number of decrypted public keys: got %d, want 2", len(pubKeys))
	}
	if len(privKeys) != 2 {
		t.Fatalf("Unexpected number of decrypted private keys: got %d, want 2", len(privKeys))
	}
	if pubKeys[0].String() != TstPubKeys[0] || pubKeys[1].String() != TstPubKeys[1] {
		t.Fatalf("Public keys don't match: %v!=%v ", TstPubKeys[0:2], pubKeys)
	}
	if privKeys[0].String() != TstPrivKeys[0] || privKeys[1] != nil {
		t.Fatalf("Private keys don't match: %v, %v", []string{TstPrivKeys[0], ""}, privKeys)
	}
	neuteredKey, e := privKeys[0].Neuter()
	if e != nil  {
		t.Fatalf("Unable to neuter private key: %v", e)
	}
	if pubKeys[0].String() != neuteredKey.String() {
		t.Errorf("Public key (%v) does not match neutered private key (%v)",
			pubKeys[0].String(), neuteredKey.String())
	}
}
func TestValidateAndDecryptKeysErrors(t *testing.T) {
	tearDown, db, pool := TstCreatePool(t)
	defer tearDown()
	dbtx, e := db.BeginReadWriteTx()
	if e != nil  {
		t.Fatal(e)
	}
	defer func() {
		e := dbtx.Commit()
		if e != nil  {
			t.Log(e)
		}
	}()
	_, addrmgrNs := TstRWNamespaces(dbtx)
	encryptedPubKeys, e := encryptKeys(TstPubKeys[0:1], pool.Manager(), waddrmgr.CKTPublic)
	if e != nil  {
		t.Fatalf("Failed to encrypt public key: %v", e)
	}
	var encryptedPrivKeys [][]byte
	TstRunWithManagerUnlocked(t, pool.Manager(), addrmgrNs, func() {
		encryptedPrivKeys, e = encryptKeys(TstPrivKeys[1:2], pool.Manager(), waddrmgr.CKTPrivate)
	})
	if e != nil  {
		t.Fatalf("Failed to encrypt private key: %v", e)
	}
	tests := []struct {
		rawPubKeys  [][]byte
		rawPrivKeys [][]byte
		e           ErrorCode
	}{
		{
			// Number of public keys does not match number of private keys.
			rawPubKeys:  [][]byte{[]byte(TstPubKeys[0])},
			rawPrivKeys: [][]byte{},
			e:           ErrKeysPrivatePublicMismatch,
		},
		{
			// Failure to decrypt public key.
			rawPubKeys:  [][]byte{[]byte(TstPubKeys[0])},
			rawPrivKeys: [][]byte{[]byte(TstPrivKeys[0])},
			e:           ErrCrypto,
		},
		{
			// Failure to decrypt private key.
			rawPubKeys:  encryptedPubKeys,
			rawPrivKeys: [][]byte{[]byte(TstPrivKeys[0])},
			e:           ErrCrypto,
		},
		{
			// One public and one private key, but they don't match.
			rawPubKeys:  encryptedPubKeys,
			rawPrivKeys: encryptedPrivKeys,
			e:           ErrKeyMismatch,
		},
	}
	for i, test := range tests {
		TstRunWithManagerUnlocked(t, pool.Manager(), addrmgrNs, func() {
			_, _, e = validateAndDecryptKeys(test.rawPubKeys, test.rawPrivKeys, pool)
		})
		TstCheckError(t, fmt.Sprintf("Test #%d", i), e, test.e)
	}
}
func encryptKeys(keys []string, mgr *waddrmgr.Manager, keyType waddrmgr.CryptoKeyType) ([][]byte, error) {
	encryptedKeys := make([][]byte, len(keys))
	var e error
	for i, key := range keys {
		if key == "" {
			encryptedKeys[i] = nil
		} else {
			encryptedKeys[i], e = mgr.Encrypt(keyType, []byte(key))
		}
		if e != nil  {
			return nil, e
		}
	}
	return encryptedKeys, nil
}
