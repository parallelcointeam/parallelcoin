package waddrmgr

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"github.com/p9c/pod/pkg/chaincfg"
	"time"
	
	"github.com/p9c/pod/pkg/chainhash"
	"github.com/p9c/pod/pkg/walletdb"
)

const (
	// LatestMgrVersion is the most recent manager version.
	LatestMgrVersion = 5
	
	// latestMgrVersion is the most recent manager version as a variable so the
	// tests can change it to force errors.
	latestMgrVersion = uint32(LatestMgrVersion)
)

// ObtainUserInputFunc is a function that reads a user input and returns it as a
// byte stream. It is used to accept data required during upgrades, for e.g.
// wallet seed and private passphrase.
type ObtainUserInputFunc func() ([]byte, error)

// maybeConvertDbError converts the passed error to a ManagerError with an error
// code of ErrDatabase if it is not already a ManagerError. This is useful for
// potential errors returned from managed transaction an other parts of the
// walletdb database.
func maybeConvertDbError(ee error) (e error) {
	// When the error is already a ManagerError, just return it.
	if _, ok := ee.(ManagerError); ok {
		return ee
	}
	return managerError(ErrDatabase, ee.Error(), e)
}

// syncStatus represents a address synchronization status stored in the
// database.
type syncStatus uint8

// These constants define the various supported sync status types.
//
// NOTE: These are currently unused but are being defined for the possibility of
// supporting sync status on a per-address basis.
const (
	ssNone syncStatus = 0 // not iota as they need to be stable for db
	// ssPartial syncStatus = 1
	ssFull syncStatus = 2
)

// addressType represents a type of address stored in the database.
type addressType uint8

// These constants define the various supported address types.
const (
	adtChain  addressType = 0
	adtImport addressType = 1 // not iota as they need to be stable for db
	adtScript addressType = 2
)

// accountType represents a type of address stored in the database.
type accountType uint8

// These constants define the various supported account types.
const (
	// accountDefault is the current "default" account type within the database.
	// This is an account that re-uses the key derivation schema of BIP0044-like
	// accounts.
	accountDefault accountType = 0 // not iota as they need to be stable
)

// dbAccountRow houses information stored about an account in the database.
type dbAccountRow struct {
	acctType accountType
	rawData  []byte // Varies based on account type field.
}

// dbDefaultAccountRow houses additional information stored about a default
// BIP0044-like account in the database.
type dbDefaultAccountRow struct {
	dbAccountRow
	pubKeyEncrypted   []byte
	privKeyEncrypted  []byte
	nextExternalIndex uint32
	nextInternalIndex uint32
	name              string
}

// dbAddressRow houses common information stored about an address in the
// database.
type dbAddressRow struct {
	addTime    uint64
	rawData    []byte // Varies based on address type field.
	account    uint32
	syncStatus syncStatus
	addrType   addressType
}

// dbChainAddressRow houses additional information stored about a chained
// address in the database.
type dbChainAddressRow struct {
	dbAddressRow
	branch uint32
	index  uint32
}

// dbImportedAddressRow houses additional information stored about an imported
// public key address in the database.
type dbImportedAddressRow struct {
	dbAddressRow
	encryptedPubKey  []byte
	encryptedPrivKey []byte
}

// dbImportedAddressRow houses additional information stored about a script
// address in the database.
type dbScriptAddressRow struct {
	dbAddressRow
	encryptedHash   []byte
	encryptedScript []byte
}

// Key names for various database fields. these are variables but only because
// they are not able to be constants
var (
	// nullVal is null byte used as a flag value in a bucket entry
	nullVal = []byte{0}
	// Bucket names.
	//
	// scopeSchemaBucket is the name of the bucket that maps a particular manager
	// scope to the type of addresses that should be derived for particular branches
	// during key derivation.
	scopeSchemaBucketName = []byte("scope-schema")
	// scopeBucketNme is the name of the top-level bucket within the hierarchy. It
	// maps: purpose || coinType to a new sub-bucket that will house a scoped
	// address manager. All buckets below are a child of this bucket:
	//
	// scopeBucket -> scope -> acctBucket
	// scopeBucket -> scope -> addrBucket
	// scopeBucket -> scope -> usedAddrBucket
	// scopeBucket -> scope -> addrAcctIdxBucket
	// scopeBucket -> scope -> acctNameIdxBucket
	// scopeBucket -> scope -> acctIDIdxBucketName
	// scopeBucket -> scope -> metaBucket
	// scopeBucket -> scope -> metaBucket -> lastAccountNameKey
	// scopeBucket -> scope -> coinTypePrivKey
	// scopeBucket -> scope -> coinTypePubKey
	scopeBucketName = []byte("scope")
	// coinTypePrivKeyName is the name of the key within a particular scope bucket
	// that stores the encrypted cointype private keys. Each scope within the
	// database will have its own set of coin type keys.
	coinTypePrivKeyName = []byte("ctpriv")
	// coinTypePrivKeyName is the name of the key within a particular scope bucket
	// that stores the encrypted cointype public keys. Each scope will have its own
	// set of coin type public keys.
	coinTypePubKeyName = []byte("ctpub")
	// acctBucketName is the bucket directly below the scope bucket in the
	// hierarchy. This bucket stores all the information and indexes relevant to an
	// account.
	acctBucketName = []byte("acct")
	// addrBucketName is the name of the bucket that stores a mapping of pubkey hash
	// to address type. This will be used to quickly determine if a given address is
	// under our control.
	addrBucketName = []byte("addr")
	// addrAcctIdxBucketName is used to index account addresses Entries in this
	// index may map:
	//
	//   * addr hash => account id
	//
	//   * account bucket -> addr hash => null
	//
	// To fetch the account of an address, lookup the value using the address hash.
	//
	// To fetch all addresses of an account, fetch the account bucket, iterate over
	// the keys and fetch the address row from the addr bucket.
	//
	// The index needs to be updated whenever an address is created e.g. NewAddress
	addrAcctIdxBucketName = []byte("addracctidx")
	// acctNameIdxBucketName is used to create an index mapping an account name
	// string to the corresponding account id. The index needs to be updated
	// whenever the account name and id changes e.g. RenameAccount
	//
	// string => account_id
	acctNameIdxBucketName = []byte("acctnameidx")
	// acctIDIdxBucketName is used to create an index mapping an account id to the
	// corresponding account name string. The index needs to be updated whenever the
	// account name and id changes e.g. RenameAccount
	//
	// account_id => string
	acctIDIdxBucketName = []byte("acctididx")
	// usedAddrBucketName is the name of the bucket that stores an addresses hash if
	// the address has been used or not.
	usedAddrBucketName = []byte("usedaddrs")
	// meta is used to store meta-data about the address manager e.g. last account
	// number
	metaBucketName = []byte("meta")
	// lastAccountName is used to store the metadata - last account in the manager
	lastAccountName = []byte("lastaccount")
	// mainBucketName is the name of the bucket that stores the encrypted crypto
	// keys that encrypt all other generated keys, the watch only flag, the master
	// private key (encrypted), the master HD private key (encrypted), and also
	// versioning information.
	mainBucketName = []byte("main")
	// masterHDPrivName is the name of the key that stores the master HD private
	// key. This key is encrypted with the master private crypto encryption key.
	// This resides under the main bucket.
	masterHDPrivName = []byte("mhdpriv")
	// masterHDPubName is the name of the key that stores the master HD public key.
	// This key is encrypted with the master public crypto encryption key. This
	// reside under the main bucket.
	masterHDPubName = []byte("mhdpub")
	// syncBucketName is the name of the bucket that stores the current sync state
	// of the root manager.
	syncBucketName = []byte("sync")
	// Db related key names (main bucket).
	mgrVersionName    = []byte("mgrver")
	mgrCreateDateName = []byte("mgrcreated")
	// Crypto related key names (main bucket).
	masterPrivKeyName   = []byte("mpriv")
	masterPubKeyName    = []byte("mpub")
	cryptoPrivKeyName   = []byte("cpriv")
	cryptoPubKeyName    = []byte("cpub")
	cryptoScriptKeyName = []byte("cscript")
	watchingOnlyName    = []byte("watchonly")
	// Sync related key names (sync bucket).
	syncedToName   = []byte("syncedto")
	startBlockName = []byte("startblock")
	birthdayName   = []byte("birthday")
)

// uint32ToBytes converts a 32 bit unsigned integer into a 4-byte slice in
// little-endian order: 1 -> [1 0 0 0].
func uint32ToBytes(number uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, number)
	return buf
}

// // uint64ToBytes converts a 64 bit unsigned integer into a 8-byte slice in
// // little-endian order: 1 -> [1 0 0 0 0 0 0 0].
// func uint64ToBytes(// 	number uint64) []byte {
// 	buf := make([]byte, 8)
// 	binary.LittleEndian.PutUint64(buf, number)
// 	return buf
// }

// stringToBytes converts a string into a variable length byte slice in
// little-endian order: "abc" -> [3 0 0 0 61 62 63]
func stringToBytes(s string) []byte {
	// The serialized format is:
	//   <size><string>
	//
	// 4 bytes string size + string
	size := len(s)
	buf := make([]byte, 4+size)
	copy(buf[0:4], uint32ToBytes(uint32(size)))
	copy(buf[4:4+size], s)
	return buf
}

// scopeKeySize is the size of a scope as stored within the database.
const scopeKeySize = 8

// scopeToBytes transforms a manager's scope into the form that will be used to
// retrieve the bucket that all information for a particular scope is stored
// under
func scopeToBytes(scope *KeyScope) [scopeKeySize]byte {
	var scopeBytes [scopeKeySize]byte
	binary.LittleEndian.PutUint32(scopeBytes[:], scope.Purpose)
	binary.LittleEndian.PutUint32(scopeBytes[4:], scope.Coin)
	return scopeBytes
}

// // scopeFromBytes decodes a serializes manager scope into its concrete manager
// // scope struct.
// func scopeFromBytes(// 	scopeBytes []byte) KeyScope {
// 	return KeyScope{
// 		Purpose: binary.LittleEndian.Uint32(scopeBytes[:]),
// 		Coin:    binary.LittleEndian.Uint32(scopeBytes[4:]),
// 	}
// }

// scopeSchemaToBytes encodes the passed scope schema as a set of bytes suitable
// for storage within the database.
func scopeSchemaToBytes(schema *ScopeAddrSchema) []byte {
	var schemaBytes [2]byte
	schemaBytes[0] = byte(schema.InternalAddrType)
	schemaBytes[1] = byte(schema.ExternalAddrType)
	return schemaBytes[:]
}

// scopeSchemaFromBytes decodes a new scope schema instance from the set of
// serialized bytes.
func scopeSchemaFromBytes(schemaBytes []byte) *ScopeAddrSchema {
	return &ScopeAddrSchema{
		InternalAddrType: AddressType(schemaBytes[0]),
		ExternalAddrType: AddressType(schemaBytes[1]),
	}
}

// fetchScopeAddrSchema will attempt to retrieve the address schema for a
// particular manager scope stored within the database. These are used in order
// to properly type each address generated by the scope address manager.
func fetchScopeAddrSchema(
	ns walletdb.ReadBucket,
	scope *KeyScope,
) (*ScopeAddrSchema, error) {
	schemaBucket := ns.NestedReadBucket(scopeSchemaBucketName)
	if schemaBucket == nil {
		str := fmt.Sprintf("unable to find scope schema bucket")
		return nil, managerError(ErrScopeNotFound, str, nil)
	}
	scopeKey := scopeToBytes(scope)
	schemaBytes := schemaBucket.Get(scopeKey[:])
	if schemaBytes == nil {
		str := fmt.Sprintf("unable to find scope %v", scope)
		return nil, managerError(ErrScopeNotFound, str, nil)
	}
	return scopeSchemaFromBytes(schemaBytes), nil
}

// putScopeAddrSchema attempts to store the passed addr scehma for the given
// manager scope.
func putScopeAddrTypes(ns walletdb.ReadWriteBucket, scope *KeyScope, schema *ScopeAddrSchema) (e error) {
	scopeSchemaBucket := ns.NestedReadWriteBucket(scopeSchemaBucketName)
	if scopeSchemaBucket == nil {
		str := fmt.Sprintf("unable to find scope schema bucket")
		return managerError(ErrScopeNotFound, str, nil)
	}
	scopeKey := scopeToBytes(scope)
	schemaBytes := scopeSchemaToBytes(schema)
	return scopeSchemaBucket.Put(scopeKey[:], schemaBytes)
}

func fetchReadScopeBucket(ns walletdb.ReadBucket, scope *KeyScope) (walletdb.ReadBucket, error) {
	rootScopeBucket := ns.NestedReadBucket(scopeBucketName)
	scopeKey := scopeToBytes(scope)
	scopedBucket := rootScopeBucket.NestedReadBucket(scopeKey[:])
	if scopedBucket == nil {
		str := fmt.Sprintf("unable to find scope %v", scope)
		return nil, managerError(ErrScopeNotFound, str, nil)
	}
	return scopedBucket, nil
}

func fetchWriteScopeBucket(
	ns walletdb.ReadWriteBucket,
	scope *KeyScope,
) (walletdb.ReadWriteBucket, error) {
	rootScopeBucket := ns.NestedReadWriteBucket(scopeBucketName)
	scopeKey := scopeToBytes(scope)
	scopedBucket := rootScopeBucket.NestedReadWriteBucket(scopeKey[:])
	if scopedBucket == nil {
		str := fmt.Sprintf("unable to find scope %v", scope)
		return nil, managerError(ErrScopeNotFound, str, nil)
	}
	return scopedBucket, nil
}

// fetchManagerVersion fetches the current manager version from the database.
func fetchManagerVersion(ns walletdb.ReadBucket) (uint32, error) {
	mainBucket := ns.NestedReadBucket(mainBucketName)
	verBytes := mainBucket.Get(mgrVersionName)
	if verBytes == nil {
		str := "required version number not stored in database"
		return 0, managerError(ErrDatabase, str, nil)
	}
	version := binary.LittleEndian.Uint32(verBytes)
	return version, nil
}

// putManagerVersion stores the provided version to the database.
func putManagerVersion(ns walletdb.ReadWriteBucket, version uint32) (e error) {
	bucket := ns.NestedReadWriteBucket(mainBucketName)
	verBytes := uint32ToBytes(version)
	if e = bucket.Put(mgrVersionName, verBytes); E.Chk(e) {
		str := "failed to store version"
		return managerError(ErrDatabase, str, e)
	}
	return nil
}

// fetchMasterKeyParams loads the master key parameters needed to derive them
// (when given the correct user-supplied passphrase) from the database. Either
// returned value can be nil, but in practice only the private key netparams
// will be nil for a watching-only database.
func fetchMasterKeyParams(ns walletdb.ReadBucket) ([]byte, []byte, error) {
	bucket := ns.NestedReadBucket(mainBucketName)
	// Load the master public key parameters.  Required.
	val := bucket.Get(masterPubKeyName)
	if val == nil {
		str := "required master public key parameters not stored in " +
			"database"
		return nil, nil, managerError(ErrDatabase, str, nil)
	}
	pubParams := make([]byte, len(val))
	copy(pubParams, val)
	// Load the master private key parameters if they were stored.
	var privParams []byte
	val = bucket.Get(masterPrivKeyName)
	if val != nil {
		privParams = make([]byte, len(val))
		copy(privParams, val)
	}
	return pubParams, privParams, nil
}

// putMasterKeyParams stores the master key parameters needed to derive them to
// the database. Either parameter can be nil in which case no value is written
// for the parameter.
func putMasterKeyParams(ns walletdb.ReadWriteBucket, pubParams, privParams []byte) (e error) {
	bucket := ns.NestedReadWriteBucket(mainBucketName)
	if privParams != nil {
		if e = bucket.Put(masterPrivKeyName, privParams); E.Chk(e) {
			str := "failed to store master private key parameters"
			return managerError(ErrDatabase, str, e)
		}
	}
	if pubParams != nil {
		if e = bucket.Put(masterPubKeyName, pubParams); E.Chk(e) {
			str := "failed to store master public key parameters"
			return managerError(ErrDatabase, str, e)
		}
	}
	return nil
}

// fetchCoinTypeKeys loads the encrypted cointype keys which are in turn used to
// derive the extended keys for all accounts. Each cointype key is associated
// with a particular manager scoped.
func fetchCoinTypeKeys(ns walletdb.ReadBucket, scope *KeyScope) ([]byte, []byte, error) {
	scopedBucket, e := fetchReadScopeBucket(ns, scope)
	if e != nil {
		return nil, nil, e
	}
	coinTypePubKeyEnc := scopedBucket.Get(coinTypePubKeyName)
	if coinTypePubKeyEnc == nil {
		str := "required encrypted cointype public key not stored in database"
		return nil, nil, managerError(ErrDatabase, str, nil)
	}
	coinTypePrivKeyEnc := scopedBucket.Get(coinTypePrivKeyName)
	if coinTypePrivKeyEnc == nil {
		str := "required encrypted cointype private key not stored in database"
		return nil, nil, managerError(ErrDatabase, str, nil)
	}
	return coinTypePubKeyEnc, coinTypePrivKeyEnc, nil
}

// putCoinTypeKeys stores the encrypted cointype keys which are in turn used to
// derive the extended keys for all accounts. Either parameter can be nil in
// which case no value is written for the parameter. Each cointype key is
// associated with a particular manager scope.
func putCoinTypeKeys(
	ns walletdb.ReadWriteBucket, scope *KeyScope,
	coinTypePubKeyEnc []byte, coinTypePrivKeyEnc []byte,
) (e error) {
	var scopedBucket walletdb.ReadWriteBucket
	if scopedBucket, e = fetchWriteScopeBucket(ns, scope); E.Chk(e) {
		return e
	}
	if coinTypePubKeyEnc != nil {
		if e = scopedBucket.Put(coinTypePubKeyName, coinTypePubKeyEnc); E.Chk(e) {
			str := "failed to store encrypted cointype public key"
			return managerError(ErrDatabase, str, e)
		}
	}
	if coinTypePrivKeyEnc != nil {
		if e = scopedBucket.Put(coinTypePrivKeyName, coinTypePrivKeyEnc); E.Chk(e) {
			str := "failed to store encrypted cointype private key"
			return managerError(ErrDatabase, str, e)
		}
	}
	return nil
}

// putMasterHDKeys stores the encrypted master HD keys in the top level main
// bucket. These are required in order to create any new manager scopes, as
// those are created via hardened derivation of the children of this key.
func putMasterHDKeys(ns walletdb.ReadWriteBucket, masterHDPrivEnc, masterHDPubEnc []byte) (e error) {
	// As this is the key for the root manager, we don't need to fetch any
	// particular scope, and can insert directly within the main bucket.
	bucket := ns.NestedReadWriteBucket(mainBucketName)
	// Now that we have the main bucket, we can directly store each of the relevant
	// keys. If we're in watch only mode, then some or all of these keys might not
	// be available.
	if masterHDPrivEnc != nil {
		if e = bucket.Put(masterHDPrivName, masterHDPrivEnc); E.Chk(e) {
			str := "failed to store encrypted master HD private key"
			return managerError(ErrDatabase, str, e)
		}
	}
	if masterHDPubEnc != nil {
		if e = bucket.Put(masterHDPubName, masterHDPubEnc); E.Chk(e) {
			str := "failed to store encrypted master HD public key"
			return managerError(ErrDatabase, str, e)
		}
	}
	return nil
}

// fetchMasterHDKeys attempts to fetch both the master HD private and public
// keys from the database. If this is a watch only wallet, then it's possible
// that the master private key isn't stored.
func fetchMasterHDKeys(ns walletdb.ReadBucket) ([]byte, []byte, error) {
	bucket := ns.NestedReadBucket(mainBucketName)
	var masterHDPrivEnc, masterHDPubEnc []byte
	// First, we'll try to fetch the master private key. If this database is watch
	// only, or the master has been neutered, then this won't be found on disk.
	key := bucket.Get(masterHDPrivName)
	if key != nil {
		masterHDPrivEnc = make([]byte, len(key))
		copy(masterHDPrivEnc, key)
	}
	key = bucket.Get(masterHDPubName)
	if key != nil {
		masterHDPubEnc = make([]byte, len(key))
		copy(masterHDPubEnc, key)
	}
	return masterHDPrivEnc, masterHDPubEnc, nil
}

// fetchCryptoKeys loads the encrypted crypto keys which are in turn used to
// protect the extended keys, imported keys, and scripts. Any of the returned
// values can be nil, but in practice only the crypto private and script keys
// will be nil for a watching-only database.
func fetchCryptoKeys(ns walletdb.ReadBucket) ([]byte, []byte, []byte, error) {
	bucket := ns.NestedReadBucket(mainBucketName)
	// Load the crypto public key parameters.  Required.
	val := bucket.Get(cryptoPubKeyName)
	if val == nil {
		str := "required encrypted crypto public not stored in database"
		return nil, nil, nil, managerError(ErrDatabase, str, nil)
	}
	pubKey := make([]byte, len(val))
	copy(pubKey, val)
	// Load the crypto private key parameters if they were stored.
	var privKey []byte
	val = bucket.Get(cryptoPrivKeyName)
	if val != nil {
		privKey = make([]byte, len(val))
		copy(privKey, val)
	}
	// Load the crypto script key parameters if they were stored.
	var scriptKey []byte
	val = bucket.Get(cryptoScriptKeyName)
	if val != nil {
		scriptKey = make([]byte, len(val))
		copy(scriptKey, val)
	}
	return pubKey, privKey, scriptKey, nil
}

// putCryptoKeys stores the encrypted crypto keys which are in turn used to
// protect the extended and imported keys. Either parameter can be nil in which
// case no value is written for the parameter.
func putCryptoKeys(
	ns walletdb.ReadWriteBucket, pubKeyEncrypted, privKeyEncrypted,
	scriptKeyEncrypted []byte,
) (e error) {
	bucket := ns.NestedReadWriteBucket(mainBucketName)
	if pubKeyEncrypted != nil {
		if e = bucket.Put(cryptoPubKeyName, pubKeyEncrypted); E.Chk(e) {
			str := "failed to store encrypted crypto public key"
			return managerError(ErrDatabase, str, e)
		}
	}
	if privKeyEncrypted != nil {
		if e = bucket.Put(cryptoPrivKeyName, privKeyEncrypted); E.Chk(e) {
			str := "failed to store encrypted crypto private key"
			return managerError(ErrDatabase, str, e)
		}
	}
	if scriptKeyEncrypted != nil {
		if e = bucket.Put(cryptoScriptKeyName, scriptKeyEncrypted); E.Chk(e) {
			str := "failed to store encrypted crypto script key"
			return managerError(ErrDatabase, str, e)
		}
	}
	return nil
}

// fetchWatchingOnly loads the watching-only flag from the database.
func fetchWatchingOnly(ns walletdb.ReadBucket) (bool, error) {
	bucket := ns.NestedReadBucket(mainBucketName)
	buf := bucket.Get(watchingOnlyName)
	if len(buf) != 1 {
		str := "malformed watching-only flag stored in database"
		return false, managerError(ErrDatabase, str, nil)
	}
	return buf[0] != 0, nil
}

// putWatchingOnly stores the watching-only flag to the database.
func putWatchingOnly(ns walletdb.ReadWriteBucket, watchingOnly bool) (e error) {
	bucket := ns.NestedReadWriteBucket(mainBucketName)
	var encoded byte
	if watchingOnly {
		encoded = 1
	}
	if e = bucket.Put(watchingOnlyName, []byte{encoded}); E.Chk(e) {
		str := "failed to store watching only flag"
		return managerError(ErrDatabase, str, e)
	}
	return nil
}

// deserializeAccountRow deserializes the passed serialized account information.
// This is used as a common base for the various account types to deserialize
// the common parts.
func deserializeAccountRow(accountID []byte, serializedAccount []byte) (*dbAccountRow, error) {
	// The serialized account format is:
	//
	//   <acctType><rdlen><rawdata>
	//
	// 1 byte acctType + 4 bytes raw data length + raw data
	//
	// Given the above, the length of the entry must be at a minimum the constant
	// value txsizes.
	if len(serializedAccount) < 5 {
		str := fmt.Sprintf(
			"malformed serialized account for key %x",
			accountID,
		)
		return nil, managerError(ErrDatabase, str, nil)
	}
	row := dbAccountRow{}
	row.acctType = accountType(serializedAccount[0])
	rdlen := binary.LittleEndian.Uint32(serializedAccount[1:5])
	row.rawData = make([]byte, rdlen)
	copy(row.rawData, serializedAccount[5:5+rdlen])
	return &row, nil
}

// serializeAccountRow returns the serialization of the passed account row.
func serializeAccountRow(row *dbAccountRow) []byte {
	// The serialized account format is:
	//
	//   <acctType><rdlen><rawdata>
	//
	// 1 byte acctType + 4 bytes raw data length + raw data
	rdlen := len(row.rawData)
	buf := make([]byte, 5+rdlen)
	buf[0] = byte(row.acctType)
	binary.LittleEndian.PutUint32(buf[1:5], uint32(rdlen))
	copy(buf[5:5+rdlen], row.rawData)
	return buf
}

// deserializeDefaultAccountRow deserializes the raw data from the passed
// account row as a BIP0044-like account.
func deserializeDefaultAccountRow(accountID []byte, row *dbAccountRow) (*dbDefaultAccountRow, error) {
	// The serialized BIP0044 account raw data format is:
	//
	//   <encpubkeylen><encpubkey><encprivkeylen><encprivkey><nextextidx>
	//   <nextintidx><namelen><name>
	//
	// 4 bytes encrypted pubkey len + encrypted pubkey + 4 bytes encrypted
	//
	// privkey len + encrypted privkey + 4 bytes next external index +
	//
	// 4 bytes next internal index + 4 bytes name len + name
	//
	// Given the above, the length of the entry must be at a minimum the constant
	// value txsizes.
	if len(row.rawData) < 20 {
		str := fmt.Sprintf("malformed serialized bip0044 account for key %x", accountID)
		return nil, managerError(ErrDatabase, str, nil)
	}
	retRow := dbDefaultAccountRow{
		dbAccountRow: *row,
	}
	pubLen := binary.LittleEndian.Uint32(row.rawData[0:4])
	retRow.pubKeyEncrypted = make([]byte, pubLen)
	copy(retRow.pubKeyEncrypted, row.rawData[4:4+pubLen])
	offset := 4 + pubLen
	privLen := binary.LittleEndian.Uint32(row.rawData[offset : offset+4])
	offset += 4
	retRow.privKeyEncrypted = make([]byte, privLen)
	copy(retRow.privKeyEncrypted, row.rawData[offset:offset+privLen])
	offset += privLen
	retRow.nextExternalIndex = binary.LittleEndian.Uint32(row.rawData[offset : offset+4])
	offset += 4
	retRow.nextInternalIndex = binary.LittleEndian.Uint32(row.rawData[offset : offset+4])
	offset += 4
	nameLen := binary.LittleEndian.Uint32(row.rawData[offset : offset+4])
	offset += 4
	retRow.name = string(row.rawData[offset : offset+nameLen])
	return &retRow, nil
}

// serializeDefaultAccountRow returns the serialization of the raw data field for a BIP0044-like account.
func serializeDefaultAccountRow(
	encryptedPubKey, encryptedPrivKey []byte,
	nextExternalIndex, nextInternalIndex uint32, name string,
) []byte {
	// The serialized BIP0044 account raw data format is:
	//
	//   <encpubkeylen><encpubkey><encprivkeylen><encprivkey><nextextidx>
	//   <nextintidx><namelen><name>
	//
	// 4 bytes encrypted pubkey len + encrypted pubkey + 4 bytes encrypted
	//
	// privkey len + encrypted privkey + 4 bytes next external index +
	//
	// 4 bytes next internal index + 4 bytes name len + name
	pubLen := uint32(len(encryptedPubKey))
	privLen := uint32(len(encryptedPrivKey))
	nameLen := uint32(len(name))
	rawData := make([]byte, 20+pubLen+privLen+nameLen)
	binary.LittleEndian.PutUint32(rawData[0:4], pubLen)
	copy(rawData[4:4+pubLen], encryptedPubKey)
	offset := 4 + pubLen
	binary.LittleEndian.PutUint32(rawData[offset:offset+4], privLen)
	offset += 4
	copy(rawData[offset:offset+privLen], encryptedPrivKey)
	offset += privLen
	binary.LittleEndian.PutUint32(rawData[offset:offset+4], nextExternalIndex)
	offset += 4
	binary.LittleEndian.PutUint32(rawData[offset:offset+4], nextInternalIndex)
	offset += 4
	binary.LittleEndian.PutUint32(rawData[offset:offset+4], nameLen)
	offset += 4
	copy(rawData[offset:offset+nameLen], name)
	return rawData
}

// forEachKeyScope calls the given function for each known manager scope within
// the set of scopes known by the root manager.
func forEachKeyScope(ns walletdb.ReadBucket, fn func(KeyScope) error) (e error) {
	bucket := ns.NestedReadBucket(scopeBucketName)
	return bucket.ForEach(
		func(k, v []byte) (e error) {
			// skip non-bucket
			if len(k) != 8 {
				return nil
			}
			scope := KeyScope{
				Purpose: binary.LittleEndian.Uint32(k),
				Coin:    binary.LittleEndian.Uint32(k[4:]),
			}
			return fn(scope)
		},
	)
}

// forEachAccount calls the given function with each account stored in the
// manager, breaking early on error.
func forEachAccount(
	ns walletdb.ReadBucket, scope *KeyScope,
	fn func(account uint32) error,
) (e error) {
	var scopedBucket walletdb.ReadBucket
	if scopedBucket, e = fetchReadScopeBucket(ns, scope); E.Chk(e) {
		return e
	}
	acctBucket := scopedBucket.NestedReadBucket(acctBucketName)
	return acctBucket.ForEach(
		func(k, v []byte) (e error) {
			// Skip buckets.
			if v == nil {
				return nil
			}
			return fn(binary.LittleEndian.Uint32(k))
		},
	)
}

// fetchLastAccount retrieves the last account from the database.
func fetchLastAccount(ns walletdb.ReadBucket, scope *KeyScope) (uint32, error) {
	var scopedBucket walletdb.ReadBucket
	var e error
	if scopedBucket, e = fetchReadScopeBucket(ns, scope); E.Chk(e) {
		return 0, e
	}
	metaBucket := scopedBucket.NestedReadBucket(metaBucketName)
	val := metaBucket.Get(lastAccountName)
	if len(val) != 4 {
		str := fmt.Sprintf(
			"malformed metadata '%s' stored in database",
			lastAccountName,
		)
		return 0, managerError(ErrDatabase, str, nil)
	}
	account := binary.LittleEndian.Uint32(val[0:4])
	return account, nil
}

// fetchAccountName retrieves the account name given an account number from the
// database.
func fetchAccountName(
	ns walletdb.ReadBucket, scope *KeyScope,
	account uint32,
) (string, error) {
	var scopedBucket walletdb.ReadBucket
	var e error
	if scopedBucket, e = fetchReadScopeBucket(ns, scope); E.Chk(e) {
		return "", e
	}
	acctIDxBucket := scopedBucket.NestedReadBucket(acctIDIdxBucketName)
	val := acctIDxBucket.Get(uint32ToBytes(account))
	if val == nil {
		str := fmt.Sprintf("account %d not found", account)
		return "", managerError(ErrAccountNotFound, str, nil)
	}
	offset := uint32(0)
	nameLen := binary.LittleEndian.Uint32(val[offset : offset+4])
	offset += 4
	acctName := string(val[offset : offset+nameLen])
	return acctName, nil
}

// fetchAccountByName retrieves the account number given an account name from
// the database.
func fetchAccountByName(
	ns walletdb.ReadBucket, scope *KeyScope,
	name string,
) (uint32, error) {
	var scopedBucket walletdb.ReadBucket
	var e error
	if scopedBucket, e = fetchReadScopeBucket(ns, scope); E.Chk(e) {
		return 0, e
	}
	idxBucket := scopedBucket.NestedReadBucket(acctNameIdxBucketName)
	val := idxBucket.Get(stringToBytes(name))
	if val == nil {
		str := fmt.Sprintf("account name '%s' not found", name)
		return 0, managerError(ErrAccountNotFound, str, nil)
	}
	return binary.LittleEndian.Uint32(val), nil
}

// fetchAccountInfo loads information about the passed account from the
// database.
func fetchAccountInfo(ns walletdb.ReadBucket, scope *KeyScope, account uint32,) (ii interface{}, e error) {
	var scopedBucket walletdb.ReadBucket
	if scopedBucket, e = fetchReadScopeBucket(ns, scope); E.Chk(e) {
		return nil, e
	}
	acctBucket := scopedBucket.NestedReadBucket(acctBucketName)
	accountID := uint32ToBytes(account)
	serializedRow := acctBucket.Get(accountID)
	if serializedRow == nil {
		str := fmt.Sprintf("account %d not found", account)
		return nil, managerError(ErrAccountNotFound, str, nil)
	}
	var row *dbAccountRow
	if row, e = deserializeAccountRow(accountID, serializedRow); E.Chk(e) {
		return nil, e
	}
	if row.acctType == accountDefault {
		return deserializeDefaultAccountRow(accountID, row)
	}
	str := fmt.Sprintf("unsupported account type '%d'", row.acctType)
	return nil, managerError(ErrDatabase, str, nil)
}

// deleteAccountNameIndex deletes the given key from the account name index of
// the database.
func deleteAccountNameIndex(
	ns walletdb.ReadWriteBucket, scope *KeyScope,
	name string,
) (e error) {
	var scopedBucket walletdb.ReadWriteBucket
	if scopedBucket, e = fetchWriteScopeBucket(ns, scope); E.Chk(e) {
		return e
	}
	bucket := scopedBucket.NestedReadWriteBucket(acctNameIdxBucketName)
	// Delete the account name key
	if e = bucket.Delete(stringToBytes(name)); E.Chk(e) {
		str := fmt.Sprintf("failed to delete account name index key %s", name)
		return managerError(ErrDatabase, str, e)
	}
	return nil
}

// deleteAccountIDIndex deletes the given key from the account id index of the
// database.
func deleteAccountIDIndex(
	ns walletdb.ReadWriteBucket, scope *KeyScope,
	account uint32,
) (e error) {
	var scopedBucket walletdb.ReadWriteBucket
	if scopedBucket, e = fetchWriteScopeBucket(ns, scope); E.Chk(e) {
		return e
	}
	bucket := scopedBucket.NestedReadWriteBucket(acctIDIdxBucketName)
	// Delete the account id key
	if e = bucket.Delete(uint32ToBytes(account)); E.Chk(e) {
		str := fmt.Sprintf("failed to delete account id index key %d", account)
		return managerError(ErrDatabase, str, e)
	}
	return nil
}

// putAccountNameIndex stores the given key to the account name index of the
// database.
func putAccountNameIndex(
	ns walletdb.ReadWriteBucket, scope *KeyScope,
	account uint32, name string,
) (e error) {
	var scopedBucket walletdb.ReadWriteBucket
	if scopedBucket, e = fetchWriteScopeBucket(ns, scope); E.Chk(e) {
		return e
	}
	bucket := scopedBucket.NestedReadWriteBucket(acctNameIdxBucketName)
	// Write the account number keyed by the account name.
	if e = bucket.Put(stringToBytes(name), uint32ToBytes(account)); E.Chk(e) {
		str := fmt.Sprintf("failed to store account name index key %s", name)
		return managerError(ErrDatabase, str, e)
	}
	return nil
}

// putAccountIDIndex stores the given key to the account id index of the
// database.
func putAccountIDIndex(
	ns walletdb.ReadWriteBucket, scope *KeyScope,
	account uint32, name string,
) (e error) {
	var scopedBucket walletdb.ReadWriteBucket
	if scopedBucket, e = fetchWriteScopeBucket(ns, scope); E.Chk(e) {
		return e
	}
	bucket := scopedBucket.NestedReadWriteBucket(acctIDIdxBucketName)
	// Write the account number keyed by the account id.
	if e = bucket.Put(uint32ToBytes(account), stringToBytes(name)); E.Chk(e) {
		str := fmt.Sprintf("failed to store account id index key %s", name)
		return managerError(ErrDatabase, str, e)
	}
	return nil
}

// putAddrAccountIndex stores the given key to the address account index of the
// database.
func putAddrAccountIndex(
	ns walletdb.ReadWriteBucket, scope *KeyScope,
	account uint32, addrHash []byte,
) (e error) {
	var scopedBucket walletdb.ReadWriteBucket
	if scopedBucket, e = fetchWriteScopeBucket(ns, scope); E.Chk(e) {
		return e
	}
	bucket := scopedBucket.NestedReadWriteBucket(addrAcctIdxBucketName)
	// Write account keyed by address hash
	if e = bucket.Put(addrHash, uint32ToBytes(account)); E.Chk(e) {
		return nil
	}
	if bucket, e = bucket.CreateBucketIfNotExists(uint32ToBytes(account)); E.Chk(e) {
		return e
	}
	// In account bucket, write a null value keyed by the address hash
	if e = bucket.Put(addrHash, nullVal); E.Chk(e) {
		str := fmt.Sprintf("failed to store address account index key %s", addrHash)
		return managerError(ErrDatabase, str, e)
	}
	return nil
}

// putAccountRow stores the provided account information to the database. This
// is used a common base for storing the various account types.
func putAccountRow(
	ns walletdb.ReadWriteBucket, scope *KeyScope,
	account uint32, row *dbAccountRow,
) (e error) {
	var scopedBucket walletdb.ReadWriteBucket
	if scopedBucket, e = fetchWriteScopeBucket(ns, scope); E.Chk(e) {
		return e
	}
	bucket := scopedBucket.NestedReadWriteBucket(acctBucketName)
	// Write the serialized value keyed by the account number.
	if e = bucket.Put(uint32ToBytes(account), serializeAccountRow(row)); E.Chk(e) {
		str := fmt.Sprintf("failed to store account %d", account)
		return managerError(ErrDatabase, str, e)
	}
	return nil
}

// putAccountInfo stores the provided account information to the database.
func putAccountInfo(
	ns walletdb.ReadWriteBucket, scope *KeyScope,
	account uint32, encryptedPubKey, encryptedPrivKey []byte,
	nextExternalIndex, nextInternalIndex uint32, name string,
) (e error) {
	rawData := serializeDefaultAccountRow(
		encryptedPubKey, encryptedPrivKey, nextExternalIndex,
		nextInternalIndex, name,
	)
	// TODO(roasbeef): pass scope bucket directly??
	acctRow := dbAccountRow{
		acctType: accountDefault,
		rawData:  rawData,
	}
	if e = putAccountRow(ns, scope, account, &acctRow); E.Chk(e) {
		return e
	}
	// Update account id index.
	if e = putAccountIDIndex(ns, scope, account, name); E.Chk(e) {
		return e
	}
	// Update account name index.
	if e = putAccountNameIndex(ns, scope, account, name); E.Chk(e) {
		return e
	}
	return nil
}

// putLastAccount stores the provided metadata - last account - to the database.
func putLastAccount(
	ns walletdb.ReadWriteBucket, scope *KeyScope,
	account uint32,
) (e error) {
	var scopedBucket walletdb.ReadWriteBucket
	if scopedBucket, e = fetchWriteScopeBucket(ns, scope); E.Chk(e) {
		return e
	}
	bucket := scopedBucket.NestedReadWriteBucket(metaBucketName)
	if e = bucket.Put(lastAccountName, uint32ToBytes(account)); E.Chk(e) {
		str := fmt.Sprintf("failed to update metadata '%s'", lastAccountName)
		return managerError(ErrDatabase, str, e)
	}
	return nil
}

// deserializeAddressRow deserializes the passed serialized address information.
// This is used as a common base for the various address types to deserialize
// the common parts.
func deserializeAddressRow(serializedAddress []byte) (*dbAddressRow, error) {
	// The serialized address format is:
	//
	//   <addrType><account><addedTime><syncStatus><rawdata>
	//
	// 1 byte addrType + 4 bytes account + 8 bytes addTime + 1 byte
	//
	// syncStatus + 4 bytes raw data length + raw data
	//
	// Given the above, the length of the entry must be at a minimum the constant
	// value txsizes.
	if len(serializedAddress) < 18 {
		str := "malformed serialized address"
		return nil, managerError(ErrDatabase, str, nil)
	}
	row := dbAddressRow{}
	row.addrType = addressType(serializedAddress[0])
	row.account = binary.LittleEndian.Uint32(serializedAddress[1:5])
	row.addTime = binary.LittleEndian.Uint64(serializedAddress[5:13])
	row.syncStatus = syncStatus(serializedAddress[13])
	rdlen := binary.LittleEndian.Uint32(serializedAddress[14:18])
	row.rawData = make([]byte, rdlen)
	copy(row.rawData, serializedAddress[18:18+rdlen])
	return &row, nil
}

// serializeAddressRow returns the serialization of the passed address row.
func serializeAddressRow(row *dbAddressRow) []byte {
	// The serialized address format is:
	//
	//   <addrType><account><addedTime><syncStatus><commentlen><comment>
	//   <rawdata>
	//
	// 1 byte addrType + 4 bytes account + 8 bytes addTime + 1 byte
	// syncStatus + 4 bytes raw data length + raw data
	rdlen := len(row.rawData)
	buf := make([]byte, 18+rdlen)
	buf[0] = byte(row.addrType)
	binary.LittleEndian.PutUint32(buf[1:5], row.account)
	binary.LittleEndian.PutUint64(buf[5:13], row.addTime)
	buf[13] = byte(row.syncStatus)
	binary.LittleEndian.PutUint32(buf[14:18], uint32(rdlen))
	copy(buf[18:18+rdlen], row.rawData)
	return buf
}

// deserializeChainedAddress deserializes the raw data from the passed address
// row as a chained address.
func deserializeChainedAddress(row *dbAddressRow) (*dbChainAddressRow, error) {
	// The serialized chain address raw data format is:
	//
	//   <branch><index>
	//
	// 4 bytes branch + 4 bytes address index
	if len(row.rawData) != 8 {
		str := "malformed serialized chained address"
		return nil, managerError(ErrDatabase, str, nil)
	}
	retRow := dbChainAddressRow{
		dbAddressRow: *row,
	}
	retRow.branch = binary.LittleEndian.Uint32(row.rawData[0:4])
	retRow.index = binary.LittleEndian.Uint32(row.rawData[4:8])
	return &retRow, nil
}

// serializeChainedAddress returns the serialization of the raw data field for a
// chained address.
func serializeChainedAddress(branch, index uint32) []byte {
	// The serialized chain address raw data format is:
	//
	//   <branch><index>
	//
	// 4 bytes branch + 4 bytes address index
	rawData := make([]byte, 8)
	binary.LittleEndian.PutUint32(rawData[0:4], branch)
	binary.LittleEndian.PutUint32(rawData[4:8], index)
	return rawData
}

// deserializeImportedAddress deserializes the raw data from the passed address
// row as an imported address.
func deserializeImportedAddress(row *dbAddressRow) (*dbImportedAddressRow, error) {
	// The serialized imported address raw data format is:
	//
	//   <encpubkeylen><encpubkey><encprivkeylen><encprivkey>
	//
	// 4 bytes encrypted pubkey len + encrypted pubkey + 4 bytes encrypted
	//
	// privkey len + encrypted privkey
	//
	// Given the above, the length of the entry must be at a minimum the constant
	// value txsizes.
	if len(row.rawData) < 8 {
		str := "malformed serialized imported address"
		return nil, managerError(ErrDatabase, str, nil)
	}
	retRow := dbImportedAddressRow{
		dbAddressRow: *row,
	}
	pubLen := binary.LittleEndian.Uint32(row.rawData[0:4])
	retRow.encryptedPubKey = make([]byte, pubLen)
	copy(retRow.encryptedPubKey, row.rawData[4:4+pubLen])
	offset := 4 + pubLen
	privLen := binary.LittleEndian.Uint32(row.rawData[offset : offset+4])
	offset += 4
	retRow.encryptedPrivKey = make([]byte, privLen)
	copy(retRow.encryptedPrivKey, row.rawData[offset:offset+privLen])
	return &retRow, nil
}

// serializeImportedAddress returns the serialization of the raw data field for
// an imported address.
func serializeImportedAddress(encryptedPubKey, encryptedPrivKey []byte) []byte {
	// The serialized imported address raw data format is:
	//
	//   <encpubkeylen><encpubkey><encprivkeylen><encprivkey>
	//
	// 4 bytes encrypted pubkey len + encrypted pubkey + 4 bytes encrypted
	//
	// privkey len + encrypted privkey
	pubLen := uint32(len(encryptedPubKey))
	privLen := uint32(len(encryptedPrivKey))
	rawData := make([]byte, 8+pubLen+privLen)
	binary.LittleEndian.PutUint32(rawData[0:4], pubLen)
	copy(rawData[4:4+pubLen], encryptedPubKey)
	offset := 4 + pubLen
	binary.LittleEndian.PutUint32(rawData[offset:offset+4], privLen)
	offset += 4
	copy(rawData[offset:offset+privLen], encryptedPrivKey)
	return rawData
}

// deserializeScriptAddress deserializes the raw data from the passed address
// row as a script address.
func deserializeScriptAddress(row *dbAddressRow) (*dbScriptAddressRow, error) {
	// The serialized script address raw data format is:
	//
	//   <encscripthashlen><encscripthash><encscriptlen><encscript>
	//
	// 4 bytes encrypted script hash len + encrypted script hash + 4 bytes
	//
	// encrypted script len + encrypted script
	//
	// Given the above, the length of the entry must be at a minimum the constant
	// value txsizes.
	if len(row.rawData) < 8 {
		str := "malformed serialized script address"
		return nil, managerError(ErrDatabase, str, nil)
	}
	retRow := dbScriptAddressRow{
		dbAddressRow: *row,
	}
	hashLen := binary.LittleEndian.Uint32(row.rawData[0:4])
	retRow.encryptedHash = make([]byte, hashLen)
	copy(retRow.encryptedHash, row.rawData[4:4+hashLen])
	offset := 4 + hashLen
	scriptLen := binary.LittleEndian.Uint32(row.rawData[offset : offset+4])
	offset += 4
	retRow.encryptedScript = make([]byte, scriptLen)
	copy(retRow.encryptedScript, row.rawData[offset:offset+scriptLen])
	return &retRow, nil
}

// serializeScriptAddress returns the serialization of the raw data field for a
// script address.
func serializeScriptAddress(encryptedHash, encryptedScript []byte) []byte {
	// The serialized script address raw data format is:
	//
	//   <encscripthashlen><encscripthash><encscriptlen><encscript>
	//
	// 4 bytes encrypted script hash len + encrypted script hash + 4 bytes
	//
	// encrypted script len + encrypted script
	hashLen := uint32(len(encryptedHash))
	scriptLen := uint32(len(encryptedScript))
	rawData := make([]byte, 8+hashLen+scriptLen)
	binary.LittleEndian.PutUint32(rawData[0:4], hashLen)
	copy(rawData[4:4+hashLen], encryptedHash)
	offset := 4 + hashLen
	binary.LittleEndian.PutUint32(rawData[offset:offset+4], scriptLen)
	offset += 4
	copy(rawData[offset:offset+scriptLen], encryptedScript)
	return rawData
}

// fetchAddressByHash loads address information for the provided address hash
// from the database. The returned value is one of the address rows for the
// specific address type. The caller should use type assertions to ascertain the
// type. The caller should prefix the error message with the address hash which
// caused the failure.
func fetchAddressByHash(
	ns walletdb.ReadBucket, scope *KeyScope,
	addrHash []byte,
) (interface{}, error) {
	var scopedBucket walletdb.ReadBucket
	var e error
	if scopedBucket, e = fetchReadScopeBucket(ns, scope); E.Chk(e) {
		return nil, e
	}
	bucket := scopedBucket.NestedReadBucket(addrBucketName)
	serializedRow := bucket.Get(addrHash)
	if serializedRow == nil {
		str := "address not found"
		return nil, managerError(ErrAddressNotFound, str, nil)
	}
	var row *dbAddressRow
	if row, e = deserializeAddressRow(serializedRow); E.Chk(e) {
		return nil, e
	}
	switch row.addrType {
	case adtChain:
		return deserializeChainedAddress(row)
	case adtImport:
		return deserializeImportedAddress(row)
	case adtScript:
		return deserializeScriptAddress(row)
	}
	str := fmt.Sprintf("unsupported address type '%d'", row.addrType)
	return nil, managerError(ErrDatabase, str, nil)
}

// fetchAddressUsed returns true if the provided address id was flagged as used.
func fetchAddressUsed(ns walletdb.ReadBucket, scope *KeyScope, addressID []byte) bool {
	var scopedBucket walletdb.ReadBucket
	var e error
	if scopedBucket, e = fetchReadScopeBucket(ns, scope); E.Chk(e) {
		return false
	}
	bucket := scopedBucket.NestedReadBucket(usedAddrBucketName)
	addrHash := sha256.Sum256(addressID)
	return bucket.Get(addrHash[:]) != nil
}

// markAddressUsed flags the provided address id as used in the database.
func markAddressUsed(
	ns walletdb.ReadWriteBucket, scope *KeyScope,
	addressID []byte,
) (e error) {
	var scopedBucket walletdb.ReadWriteBucket
	if scopedBucket, e = fetchWriteScopeBucket(ns, scope); E.Chk(e) {
		return e
	}
	bucket := scopedBucket.NestedReadWriteBucket(usedAddrBucketName)
	addrHash := sha256.Sum256(addressID)
	val := bucket.Get(addrHash[:])
	if val != nil {
		return nil
	}
	if e = bucket.Put(addrHash[:], []byte{0}); E.Chk(e) {
		str := fmt.Sprintf("failed to mark address used %x", addressID)
		return managerError(ErrDatabase, str, e)
	}
	return nil
}

// fetchAddress loads address information for the provided address id from the
// database. The returned value is one of the address rows for the specific
// address type. The caller should use type assertions to ascertain the type.
// The caller should prefix the error message with the address which caused the
// failure.
func fetchAddress(
	ns walletdb.ReadBucket, scope *KeyScope,
	addressID []byte,
) (interface{}, error) {
	addrHash := sha256.Sum256(addressID)
	return fetchAddressByHash(ns, scope, addrHash[:])
}

// putAddress stores the provided address information to the database. This is
// used a common base for storing the various address types.
func putAddress(
	ns walletdb.ReadWriteBucket, scope *KeyScope,
	addressID []byte, row *dbAddressRow,
) (e error) {
	var scopedBucket walletdb.ReadWriteBucket
	if scopedBucket, e = fetchWriteScopeBucket(ns, scope); E.Chk(e) {
		return e
	}
	bucket := scopedBucket.NestedReadWriteBucket(addrBucketName)
	// Write the serialized value keyed by the hash of the address. The additional
	// hash is used to conceal the actual address while still allowed keyed lookups.
	addrHash := sha256.Sum256(addressID)
	if e = bucket.Put(addrHash[:], serializeAddressRow(row)); E.Chk(e) {
		str := fmt.Sprintf("failed to store address %x", addressID)
		return managerError(ErrDatabase, str, e)
	}
	// Update address account index
	return putAddrAccountIndex(ns, scope, row.account, addrHash[:])
}

// putChainedAddress stores the provided chained address information to the
// database.
func putChainedAddress(
	ns walletdb.ReadWriteBucket, scope *KeyScope,
	addressID []byte, account uint32, status syncStatus, branch,
	index uint32, addrType addressType,
) (e error) {
	var scopedBucket walletdb.ReadWriteBucket
	if scopedBucket, e = fetchWriteScopeBucket(ns, scope); E.Chk(e) {
		return e
	}
	addrRow := dbAddressRow{
		addrType:   addrType,
		account:    account,
		addTime:    uint64(time.Now().Unix()),
		syncStatus: status,
		rawData:    serializeChainedAddress(branch, index),
	}
	if e = putAddress(ns, scope, addressID, &addrRow); E.Chk(e) {
		return e
	}
	// Update the next index for the appropriate internal or external branch.
	accountID := uint32ToBytes(account)
	bucket := scopedBucket.NestedReadWriteBucket(acctBucketName)
	serializedAccount := bucket.Get(accountID)
	// Deserialize the account row.
	var row *dbAccountRow
	if row, e = deserializeAccountRow(accountID, serializedAccount); E.Chk(e) {
		return e
	}
	var arow *dbDefaultAccountRow
	if arow, e = deserializeDefaultAccountRow(accountID, row); E.Chk(e) {
		return e
	}
	// Increment the appropriate next index depending on whether the branch is
	// internal or external.
	nextExternalIndex := arow.nextExternalIndex
	nextInternalIndex := arow.nextInternalIndex
	if branch == InternalBranch {
		nextInternalIndex = index + 1
	} else {
		nextExternalIndex = index + 1
	}
	// Reserialize the account with the updated index and store it.
	row.rawData = serializeDefaultAccountRow(
		arow.pubKeyEncrypted, arow.privKeyEncrypted, nextExternalIndex,
		nextInternalIndex, arow.name,
	)
	if e = bucket.Put(accountID, serializeAccountRow(row)); E.Chk(e) {
		str := fmt.Sprintf(
			"failed to update next index for "+
				"address %x, account %d", addressID, account,
		)
		return managerError(ErrDatabase, str, e)
	}
	return nil
}

// putImportedAddress stores the provided imported address information to the
// database.
func putImportedAddress(
	ns walletdb.ReadWriteBucket, scope *KeyScope,
	addressID []byte, account uint32, status syncStatus,
	encryptedPubKey, encryptedPrivKey []byte,
) (e error) {
	rawData := serializeImportedAddress(encryptedPubKey, encryptedPrivKey)
	addrRow := dbAddressRow{
		addrType:   adtImport,
		account:    account,
		addTime:    uint64(time.Now().Unix()),
		syncStatus: status,
		rawData:    rawData,
	}
	return putAddress(ns, scope, addressID, &addrRow)
}

// putScriptAddress stores the provided script address information to the
// database.
func putScriptAddress(
	ns walletdb.ReadWriteBucket, scope *KeyScope,
	addressID []byte, account uint32, status syncStatus,
	encryptedHash, encryptedScript []byte,
) (e error) {
	rawData := serializeScriptAddress(encryptedHash, encryptedScript)
	addrRow := dbAddressRow{
		addrType:   adtScript,
		account:    account,
		addTime:    uint64(time.Now().Unix()),
		syncStatus: status,
		rawData:    rawData,
	}
	if e = putAddress(ns, scope, addressID, &addrRow); E.Chk(e) {
		return e
	}
	return nil
}

// existsAddress returns whether or not the address id exists in the database.
func existsAddress(ns walletdb.ReadBucket, scope *KeyScope, addressID []byte) bool {
	var scopedBucket walletdb.ReadBucket
	var e error
	if scopedBucket, e = fetchReadScopeBucket(ns, scope); E.Chk(e) {
		return false
	}
	bucket := scopedBucket.NestedReadBucket(addrBucketName)
	addrHash := sha256.Sum256(addressID)
	return bucket.Get(addrHash[:]) != nil
}

// fetchAddrAccount returns the account to which the given address belongs to.
// It looks up the account using the addracctidx index which maps the address
// hash to its corresponding account id.
func fetchAddrAccount(
	ns walletdb.ReadBucket, scope *KeyScope,
	addressID []byte,
) (uint32, error) {
	var scopedBucket walletdb.ReadBucket
	var e error
	if scopedBucket, e = fetchReadScopeBucket(ns, scope); E.Chk(e) {
		return 0, e
	}
	bucket := scopedBucket.NestedReadBucket(addrAcctIdxBucketName)
	addrHash := sha256.Sum256(addressID)
	val := bucket.Get(addrHash[:])
	if val == nil {
		str := "address not found"
		return 0, managerError(ErrAddressNotFound, str, nil)
	}
	return binary.LittleEndian.Uint32(val), nil
}

// forEachAccountAddress calls the given function with each address of the given
// account stored in the manager, breaking early on error.
func forEachAccountAddress(
	ns walletdb.ReadBucket, scope *KeyScope,
	account uint32, fn func(rowInterface interface{}) error,
) (e error) {
	var scopedBucket walletdb.ReadBucket
	if scopedBucket, e = fetchReadScopeBucket(ns, scope); E.Chk(e) {
		return e
	}
	bucket := scopedBucket.NestedReadBucket(addrAcctIdxBucketName).NestedReadBucket(uint32ToBytes(account))
	// If index bucket is missing the account, there hasn't been any address entries
	// yet
	if bucket == nil {
		return nil
	}
	if e = bucket.ForEach(
		func(k, v []byte) (e error) {
			// Skip buckets.
			if v == nil {
				return nil
			}
			var addrRow interface{}
			if addrRow, e = fetchAddressByHash(ns, scope, k); E.Chk(e) {
				if merr, ok := e.(*ManagerError); ok {
					desc := fmt.Sprintf(
						"failed to fetch address hash '%s': %v",
						k, merr.Description,
					)
					merr.Description = desc
					return merr
				}
				return e
			}
			return fn(addrRow)
		},
	); E.Chk(e) {
		return maybeConvertDbError(e)
	}
	return nil
}

// forEachActiveAddress calls the given function with each active address stored
// in the manager, breaking early on error.
func forEachActiveAddress(
	ns walletdb.ReadBucket, scope *KeyScope,
	fn func(rowInterface interface{}) error,
) (e error) {
	var scopedBucket walletdb.ReadBucket
	if scopedBucket, e = fetchReadScopeBucket(ns, scope); E.Chk(e) {
		return e
	}
	bucket := scopedBucket.NestedReadBucket(addrBucketName)
	if e = bucket.ForEach(
		func(k, v []byte) (e error) {
			// Skip buckets.
			if v == nil {
				return nil
			}
			// Deserialize the address row first to determine the field values.
			addrRow, e := fetchAddressByHash(ns, scope, k)
			if merr, ok := e.(*ManagerError); ok {
				desc := fmt.Sprintf(
					"failed to fetch address hash '%s': %v",
					k, merr.Description,
				)
				merr.Description = desc
				return merr
			}
			if e != nil {
				return e
			}
			return fn(addrRow)
		},
	); E.Chk(e) {
		return maybeConvertDbError(e)
	}
	return nil
}

// deletePrivateKeys removes all private key material from the database.
//
// NOTE: Care should be taken when calling this function. It is primarily
// intended for use in converting to a watching-only copy.
//
// Removing the private keys from the main database without also marking it
// watching-only will result in an unusable database.
//
// It will also make any imported scripts and private keys unrecoverable unless
// there is a backup copy available.
func deletePrivateKeys(ns walletdb.ReadWriteBucket) (e error) {
	bucket := ns.NestedReadWriteBucket(mainBucketName)
	// Delete the master private key netparams and the crypto private and script keys.
	if e = bucket.Delete(masterPrivKeyName); E.Chk(e) {
		str := "failed to delete master private key parameters"
		return managerError(ErrDatabase, str, e)
	}
	if e = bucket.Delete(cryptoPrivKeyName); E.Chk(e) {
		str := "failed to delete crypto private key"
		return managerError(ErrDatabase, str, e)
	}
	if e = bucket.Delete(cryptoScriptKeyName); E.Chk(e) {
		str := "failed to delete crypto script key"
		return managerError(ErrDatabase, str, e)
	}
	if e = bucket.Delete(masterHDPrivName); E.Chk(e) {
		str := "failed to delete master HD priv key"
		return managerError(ErrDatabase, str, e)
	}
	// With the master key and meta encryption keys deleted, we'll need to delete
	// the keys for all known scopes as well.
	scopeBucket := ns.NestedReadWriteBucket(scopeBucketName)
	e = scopeBucket.ForEach(
		func(scopeKey, _ []byte) (e error) {
			if len(scopeKey) != 8 {
				return nil
			}
			managerScopeBucket := scopeBucket.NestedReadWriteBucket(scopeKey)
			if e = managerScopeBucket.Delete(coinTypePrivKeyName); E.Chk(e) {
				str := "failed to delete cointype private key"
				return managerError(ErrDatabase, str, e)
			}
			// Delete the account extended private key for all accounts.
			bucket = managerScopeBucket.NestedReadWriteBucket(acctBucketName)
			e = bucket.ForEach(
				func(k, v []byte) (e error) {
					// Skip buckets.
					if v == nil {
						return nil
					}
					// Deserialize the account row first to determine the type.
					row, e := deserializeAccountRow(k, v)
					if e != nil {
						return e
					}
					if row.acctType == accountDefault {
						arow, e := deserializeDefaultAccountRow(k, row)
						if e != nil {
							return e
						}
						// Reserialize the account without the private key and store it.
						row.rawData = serializeDefaultAccountRow(
							arow.pubKeyEncrypted, nil,
							arow.nextExternalIndex, arow.nextInternalIndex,
							arow.name,
						)
						e = bucket.Put(k, serializeAccountRow(row))
						if e != nil {
							str := "failed to delete account private key"
							return managerError(ErrDatabase, str, e)
						}
					}
					return nil
				},
			)
			if e != nil {
				return maybeConvertDbError(e)
			}
			// Delete the private key for all imported addresses.
			bucket = managerScopeBucket.NestedReadWriteBucket(addrBucketName)
			e = bucket.ForEach(
				func(k, v []byte) (e error) {
					// Skip buckets.
					if v == nil {
						return nil
					}
					// Deserialize the address row first to determine the field values.
					row, e := deserializeAddressRow(v)
					if e != nil {
						return e
					}
					switch row.addrType {
					case adtImport:
						irow, e := deserializeImportedAddress(row)
						if e != nil {
							return e
						}
						// Reserialize the imported address without the private key and store it.
						row.rawData = serializeImportedAddress(
							irow.encryptedPubKey, nil,
						)
						e = bucket.Put(k, serializeAddressRow(row))
						if e != nil {
							str := "failed to delete imported private key"
							return managerError(ErrDatabase, str, e)
						}
					case adtScript:
						srow, e := deserializeScriptAddress(row)
						if e != nil {
							return e
						}
						// Reserialize the script address without the script and store it.
						row.rawData = serializeScriptAddress(
							srow.encryptedHash,
							nil,
						)
						e = bucket.Put(k, serializeAddressRow(row))
						if e != nil {
							str := "failed to delete imported script"
							return managerError(ErrDatabase, str, e)
						}
					}
					return nil
				},
			)
			if e != nil {
				return maybeConvertDbError(e)
			}
			return nil
		},
	)
	if e != nil {
		return maybeConvertDbError(e)
	}
	return nil
}

// fetchSyncedTo loads the block stamp the manager is synced to from the
// database.
func fetchSyncedTo(ns walletdb.ReadBucket) (*BlockStamp, error) {
	bucket := ns.NestedReadBucket(syncBucketName)
	// The serialized synced to format is:
	//
	//   <blockheight><blockhash><timestamp>
	//
	// 4 bytes block height + 32 bytes hash length
	buf := bucket.Get(syncedToName)
	if len(buf) < 36 {
		str := "malformed sync information stored in database"
		return nil, managerError(ErrDatabase, str, nil)
	}
	var bs BlockStamp
	bs.Height = int32(binary.LittleEndian.Uint32(buf[0:4]))
	copy(bs.Hash[:], buf[4:36])
	if len(buf) == 40 {
		bs.Timestamp = time.Unix(
			int64(binary.LittleEndian.Uint32(buf[36:])), 0,
		)
	}
	return &bs, nil
}

// putSyncedTo stores the provided synced to blockstamp to the database.
func putSyncedTo(ns walletdb.ReadWriteBucket, bs *BlockStamp) (e error) {
	bucket := ns.NestedReadWriteBucket(syncBucketName)
	errStr := fmt.Sprintf("failed to store sync information %v", bs.Hash)
	// If the block height is greater than zero, check that the previous block
	// height exists. This prevents reorg issues in the future. We use BigEndian so
	// that keys/values are added to the bucket in order, making writes more
	// efficient for some database backends.
	if bs.Height > 0 {
		if _, e = fetchBlockHash(ns, bs.Height-1); E.Chk(e) {
			return managerError(ErrDatabase, errStr, e)
		}
	}
	// Store the block hash by block height.
	height := make([]byte, 4)
	binary.BigEndian.PutUint32(height, uint32(bs.Height))
	if e = bucket.Put(height, bs.Hash[0:32]); E.Chk(e) {
		return managerError(ErrDatabase, errStr, e)
	}
	// The serialized synced to format is:
	//
	//   <blockheight><blockhash><timestamp>
	//
	// 4 bytes block height + 32 bytes hash length + 4 byte timestamp length
	buf := make([]byte, 40)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(bs.Height))
	copy(buf[4:36], bs.Hash[0:32])
	binary.LittleEndian.PutUint32(buf[36:], uint32(bs.Timestamp.Unix()))
	if e = bucket.Put(syncedToName, buf); E.Chk(e) {
		return managerError(ErrDatabase, errStr, e)
	}
	return nil
}

// fetchBlockHash loads the block hash for the provided height from the database.
func fetchBlockHash(ns walletdb.ReadBucket, height int32) (h *chainhash.Hash, e error) {
	bucket := ns.NestedReadBucket(syncBucketName)
	errStr := fmt.Sprintf("failed to fetch block hash for height %d", height)
	heightBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(heightBytes, uint32(height))
	hashBytes := bucket.Get(heightBytes)
	if len(hashBytes) != 32 {
		e = fmt.Errorf("couldn't get hash from database")
		return nil, managerError(ErrDatabase, errStr, e)
	}
	var hash chainhash.Hash
	if e = hash.SetBytes(hashBytes); E.Chk(e) {
		return nil, managerError(ErrDatabase, errStr, e)
	}
	return &hash, nil
}

// fetchStartBlock loads the start block stamp for the manager from the
// database.
func fetchStartBlock(ns walletdb.ReadBucket) (*BlockStamp, error) {
	bucket := ns.NestedReadBucket(syncBucketName)
	// The serialized start block format is:
	//
	//   <blockheight><blockhash>
	//
	// 4 bytes block height + 32 bytes hash length
	buf := bucket.Get(startBlockName)
	if len(buf) != 36 {
		str := "malformed start block stored in database"
		return nil, managerError(ErrDatabase, str, nil)
	}
	var bs BlockStamp
	bs.Height = int32(binary.LittleEndian.Uint32(buf[0:4]))
	copy(bs.Hash[:], buf[4:36])
	return &bs, nil
}

// putStartBlock stores the provided start block stamp to the database.
func putStartBlock(ns walletdb.ReadWriteBucket, bs *BlockStamp) (e error) {
	bucket := ns.NestedReadWriteBucket(syncBucketName)
	// The serialized start block format is:
	//
	//   <blockheight><blockhash>
	//
	// 4 bytes block height + 32 bytes hash length
	buf := make([]byte, 36)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(bs.Height))
	copy(buf[4:36], bs.Hash[0:32])
	if e = bucket.Put(startBlockName, buf); E.Chk(e) {
		str := fmt.Sprintf("failed to store start block %v", bs.Hash)
		return managerError(ErrDatabase, str, e)
	}
	return nil
}

// fetchBirthday loads the manager's bithday timestamp from the database.
func fetchBirthday(ns walletdb.ReadBucket) (time.Time, error) {
	bucket := ns.NestedReadBucket(syncBucketName)
	var t time.Time
	buf := bucket.Get(birthdayName)
	if len(buf) != 8 {
		str := "malformed birthday stored in database"
		return t, managerError(ErrDatabase, str, nil)
	}
	t = time.Unix(int64(binary.BigEndian.Uint64(buf)), 0)
	return t, nil
}

// putBirthday stores the provided birthday timestamp to the database.
func putBirthday(ns walletdb.ReadWriteBucket, t time.Time) (e error) {
	bucket := ns.NestedReadWriteBucket(syncBucketName)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(t.Unix()))
	if e = bucket.Put(birthdayName, buf); E.Chk(e) {
		str := "failed to store birthday"
		return managerError(ErrDatabase, str, e)
	}
	return nil
}

// managerExists returns whether or not the manager has already been created in
// the given database namespace.
func managerExists(ns walletdb.ReadBucket) bool {
	if ns == nil {
		return false
	}
	mainBucket := ns.NestedReadBucket(mainBucketName)
	return mainBucket != nil
}

// createScopedManagerNS creates the namespace buckets for a new registered
// manager scope within the top level bucket. All relevant sub-buckets that a
// ScopedManager needs to perform its duties are also created.
func createScopedManagerNS(ns walletdb.ReadWriteBucket, scope *KeyScope) (e error) {
	// First, we'll create the scope bucket itself for this particular
	// scope.
	scopeKey := scopeToBytes(scope)
	var scopeBucket walletdb.ReadWriteBucket
	if scopeBucket, e = ns.CreateBucket(scopeKey[:]); E.Chk(e) {
		str := "failed to create sync bucket"
		return managerError(ErrDatabase, str, e)
	}
	if _, e = scopeBucket.CreateBucket(acctBucketName); E.Chk(e) {
		str := "failed to create account bucket"
		return managerError(ErrDatabase, str, e)
	}
	if _, e = scopeBucket.CreateBucket(addrBucketName); E.Chk(e) {
		str := "failed to create address bucket"
		return managerError(ErrDatabase, str, e)
	}
	// usedAddrBucketName bucket was added after manager version 1 release
	if _, e = scopeBucket.CreateBucket(usedAddrBucketName); E.Chk(e) {
		str := "failed to create used addresses bucket"
		return managerError(ErrDatabase, str, e)
	}
	if _, e = scopeBucket.CreateBucket(addrAcctIdxBucketName); E.Chk(e) {
		str := "failed to create address index bucket"
		return managerError(ErrDatabase, str, e)
	}
	if _, e = scopeBucket.CreateBucket(acctNameIdxBucketName); E.Chk(e) {
		str := "failed to create an account name index bucket"
		return managerError(ErrDatabase, str, e)
	}
	if _, e = scopeBucket.CreateBucket(acctIDIdxBucketName); E.Chk(e) {
		str := "failed to create an account id index bucket"
		return managerError(ErrDatabase, str, e)
	}
	if _, e = scopeBucket.CreateBucket(metaBucketName); E.Chk(e) {
		str := "failed to create a meta bucket"
		return managerError(ErrDatabase, str, e)
	}
	return nil
}

// createManagerNS creates the initial namespace structure needed for all of the
// manager data. This includes things such as all of the buckets as well as the
// version and creation date. In addition to creating the key space for the root
// address manager, we'll also create internal scopes for all the default
// manager scope types.
func createManagerNS(
	ns walletdb.ReadWriteBucket,
	defaultScopes map[KeyScope]ScopeAddrSchema,
) (e error) {
	// First, we'll create all the relevant buckets that stem off of the main bucket.
	var mainBucket walletdb.ReadWriteBucket
	if mainBucket, e = ns.CreateBucket(mainBucketName); E.Chk(e) {
		str := "failed to create main bucket"
		return managerError(ErrDatabase, str, e)
	}
	if _, e = ns.CreateBucket(syncBucketName); E.Chk(e) {
		str := "failed to create sync bucket"
		return managerError(ErrDatabase, str, e)
	}
	// We'll also create the two top-level scope related buckets as preparation for
	// the operations below.
	var scopeBucket walletdb.ReadWriteBucket
	if scopeBucket, e = ns.CreateBucket(scopeBucketName); E.Chk(e) {
		str := "failed to create scope bucket"
		return managerError(ErrDatabase, str, e)
	}
	var scopeSchemas walletdb.ReadWriteBucket
	if scopeSchemas, e = ns.CreateBucket(scopeSchemaBucketName); E.Chk(e) {
		str := "failed to create scope schema bucket"
		return managerError(ErrDatabase, str, e)
	}
	// Next, we'll create the namespace for each of the relevant default manager
	// scopes.
	for sc, scc := range defaultScopes {
		// Before we create the entire namespace of this scope, we'll update the schema
		// mapping to note what types of addresses it prefers.
		scope := sc
		scopeSchema := scc
		scopeKey := scopeToBytes(&scope)
		schemaBytes := scopeSchemaToBytes(&scopeSchema)
		if e = scopeSchemas.Put(scopeKey[:], schemaBytes); E.Chk(e) {
			return e
		}
		if e = createScopedManagerNS(scopeBucket, &scope); E.Chk(e) {
			return e
		}
		if e = putLastAccount(ns, &scope, DefaultAccountNum); E.Chk(e) {
			return e
		}
	}
	if e = putManagerVersion(ns, latestMgrVersion); E.Chk(e) {
		return e
	}
	createDate := uint64(time.Now().Unix())
	var dateBytes [8]byte
	binary.LittleEndian.PutUint64(dateBytes[:], createDate)
	if e = mainBucket.Put(mgrCreateDateName, dateBytes[:]); E.Chk(e) {
		str := "failed to store database creation time"
		return managerError(ErrDatabase, str, e)
	}
	return nil
}

// // upgradeToVersion2 upgrades the database from version 1 to version 2
// // 'usedAddrBucketName' a bucket for storing addrs flagged as marked is
// // initialized and it will be updated on the next rescan.
//
// func upgradeToVersion2(// 	ns walletdb.ReadWriteBucket) (e error) {
// 	currentMgrVersion := uint32(2)
// 	_, e := ns.CreateBucketIfNotExists(usedAddrBucketName)
// 	if e != nil  {
// DB// 		str := "failed to create used addresses bucket"
// 		return managerError(ErrDatabase, str, err)
// 	}
// 	return putManagerVersion(ns, currentMgrVersion)
// }

// upgradeManager upgrades the data in the provided manager namespace to newer
// versions as neeeded.
func upgradeManager(
	db walletdb.DB, namespaceKey []byte, pubPassPhrase []byte,
	chainParams *chaincfg.Params, cbs *OpenCallbacks,
) (e error) {
	var version uint32
	if e = walletdb.View(
		db, func(tx walletdb.ReadTx) (e error) {
			ns := tx.ReadBucket(namespaceKey)
			version, e = fetchManagerVersion(ns)
			return e
		},
	); E.Chk(e) {
		str := "failed to fetch version for update"
		return managerError(ErrDatabase, str, e)
	}
	if version < 5 {
		if e = walletdb.Update(
			db, func(tx walletdb.ReadWriteTx) (e error) {
				ns := tx.ReadWriteBucket(namespaceKey)
				return upgradeToVersion5(ns, pubPassPhrase)
			},
		); E.Chk(e) {
			return e
		}
		// The manager is now at version 5.
		version = 5
	}
	// Ensure the manager is upgraded to the latest version. This check is to
	// intentionally cause a failure if the manager version is updated without
	// writing code to handle the upgrade.
	if version < latestMgrVersion {
		str := fmt.Sprintf(
			"the latest manager version is %d, but the "+
				"current version after upgrades is only %d",
			latestMgrVersion, version,
		)
		return managerError(ErrUpgrade, str, nil)
	}
	return nil
}

// upgradeToVersion5 upgrades the database from version 4 to version 5. After
// this update, the new ScopedKeyManager features cannot be used. This is due to
// the fact that in version 5, we now store the encrypted master private keys on
// disk. However, using the BIP0044 key scope, users will still be able to
// create old p2pkh addresses.
func upgradeToVersion5(ns walletdb.ReadWriteBucket, pubPassPhrase []byte) (e error) {
	// First, we'll check if there are any existing segwit addresses, which can't be
	// upgraded to the new version. If so, we abort and warn the user.
	if e = ns.NestedReadBucket(addrBucketName).ForEach(
		func(k []byte, v []byte) (e error) {
			row, e := deserializeAddressRow(v)
			if e != nil {
				return e
			}
			if row.addrType > adtScript {
				return fmt.Errorf(
					"segwit address exists in " +
						"wallet, can't upgrade from v4 to " +
						"v5: well, we tried  ¯\\_(ツ)_/¯",
				)
			}
			return nil
		},
	); E.Chk(e) {
		return e
	}
	// Next, we'll write out the new database version.
	if e = putManagerVersion(ns, 5); E.Chk(e) {
		return e
	}
	// First, we'll need to create the new buckets that are used in the new database
	// version.
	var scopeBucket walletdb.ReadWriteBucket
	if scopeBucket, e = ns.CreateBucket(scopeBucketName); E.Chk(e) {
		str := "failed to create scope bucket"
		return managerError(ErrDatabase, str, e)
	}
	var scopeSchemas walletdb.ReadWriteBucket
	if scopeSchemas, e = ns.CreateBucket(scopeSchemaBucketName); E.Chk(e) {
		str := "failed to create scope schema bucket"
		return managerError(ErrDatabase, str, e)
	}
	// With the buckets created, we can now create the default BIP0044 scope which
	// will be the only scope usable in the database after this update.
	scopeKey := scopeToBytes(&KeyScopeBIP0044)
	scopeSchema := ScopeAddrMap[KeyScopeBIP0044]
	schemaBytes := scopeSchemaToBytes(&scopeSchema)
	if e = scopeSchemas.Put(scopeKey[:], schemaBytes); E.Chk(e) {
		return e
	}
	if e = createScopedManagerNS(scopeBucket, &KeyScopeBIP0044); E.Chk(e) {
		return e
	}
	bip44Bucket := scopeBucket.NestedReadWriteBucket(scopeKey[:])
	// With the buckets created, we now need to port over *each* item in the prior
	// main bucket, into the new default scope.
	mainBucket := ns.NestedReadWriteBucket(mainBucketName)
	// First, we'll move over the encrypted coin type private and public keys to the
	// new sub-bucket.
	encCoinPrivKeys := mainBucket.Get(coinTypePrivKeyName)
	encCoinPubKeys := mainBucket.Get(coinTypePubKeyName)
	if e = bip44Bucket.Put(coinTypePrivKeyName, encCoinPrivKeys); E.Chk(e) {
		return e
	}
	if e = bip44Bucket.Put(coinTypePubKeyName, encCoinPubKeys); E.Chk(e) {
		return e
	}
	if e = mainBucket.Delete(coinTypePrivKeyName); E.Chk(e) {
		return e
	}
	if e = mainBucket.Delete(coinTypePubKeyName); E.Chk(e) {
		return e
	}
	// Next, we'll move over everything that was in the meta bucket to the meta
	// bucket within the new scope.
	metaBucket := ns.NestedReadWriteBucket(metaBucketName)
	lastAccount := metaBucket.Get(lastAccountName)
	if e = metaBucket.Delete(lastAccountName); E.Chk(e) {
		return e
	}
	scopedMetaBucket := bip44Bucket.NestedReadWriteBucket(metaBucketName)
	if e = scopedMetaBucket.Put(lastAccountName, lastAccount); E.Chk(e) {
		return e
	}
	// Finally, we'll recursively move over a set of keys which were formerly under
	// the main bucket, into the new scoped buckets. We'll do so by obtaining a
	// slice of all the keys that we need to modify and then recursing through each
	// of them, moving both nested buckets and key/value pairs.
	keysToMigrate := [][]byte{
		acctBucketName, addrBucketName, usedAddrBucketName,
		addrAcctIdxBucketName, acctNameIdxBucketName, acctIDIdxBucketName,
	}
	// Migrate each bucket recursively.
	for _, bucketKey := range keysToMigrate {
		if e = migrateRecursively(ns, bip44Bucket, bucketKey); E.Chk(e) {
			return e
		}
	}
	return nil
}

// migrateRecursively moves a nested bucket from one bucket to another,
// recursing into nested buckets as required.
func migrateRecursively(
	src, dst walletdb.ReadWriteBucket,
	bucketKey []byte,
) (e error) {
	// Within this bucket key, we'll migrate over, then delete each key.
	bucketToMigrate := src.NestedReadWriteBucket(bucketKey)
	var newBucket walletdb.ReadWriteBucket
	if newBucket, e = dst.CreateBucketIfNotExists(bucketKey); E.Chk(e) {
		return e
	}
	if e = bucketToMigrate.ForEach(
		func(k, v []byte) (e error) {
			if nestedBucket := bucketToMigrate.
				NestedReadBucket(k); nestedBucket != nil {
				// We have a nested bucket, so recurse into it.
				return migrateRecursively(bucketToMigrate, newBucket, k)
			}
			if e := newBucket.Put(k, v); E.Chk(e) {
				return e
			}
			return bucketToMigrate.Delete(k)
		},
	); E.Chk(e) {
		return e
	}
	// Finally, we'll delete the bucket itself.
	if e = src.DeleteNestedBucket(bucketKey); E.Chk(e) {
		return e
	}
	return nil
}
