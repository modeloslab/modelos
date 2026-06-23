// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package waddrmgr

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelos/modelos/node/btcutil/hdkeychain"
	"github.com/modelos/modelos/node/chaincfg"
	"github.com/modelos/modelos/wallet/walletdb"
	_ "github.com/modelos/modelos/wallet/walletdb/bdb"
)

var (
	// seed is the master seed used throughout the tests.
	seed = []byte{
		0x2a, 0x64, 0xdf, 0x08, 0x5e, 0xef, 0xed, 0xd8, 0xbf,
		0xdb, 0xb3, 0x31, 0x76, 0xb5, 0xba, 0x2e, 0x62, 0xe8,
		0xbe, 0x8b, 0x56, 0xc8, 0x83, 0x77, 0x95, 0x59, 0x8b,
		0xb6, 0xc4, 0x40, 0xc0, 0x64,
	}

	rootKey, _ = hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)

	pubPassphrase   = []byte("_DJr{fL4H0O}*-0\n:V1izc)(6BomK")
	privPassphrase  = []byte("81lUHXnOMZ@?XXd7O9xyDIWIbXX-lj")
	pubPassphrase2  = []byte("-0NV4P~VSJBWbunw}%<Z]fuGpbN[ZI")
	privPassphrase2 = []byte("~{<]08%6!-?2s<$(8$8:f(5[4/!/{Y")

	// fastScrypt are parameters used throughout the tests to speed up the
	// scrypt operations.
	fastScrypt = &FastScryptOptions

	// waddrmgrNamespaceKey is the namespace key for the waddrmgr package.
	waddrmgrNamespaceKey = []byte("waddrmgrNamespace")

	// expectedAddrs is the list of all expected addresses generated from the
	// seed. Addresses use BIP-86 key-only Taproot commitment.
	expectedAddrs = []expectedAddr{
		{
			address:     "prl1psayn40fkp3f3szxztahxwszhyjfxe3u9tqxycujd6q2zcw4uc99sm7r4na",
			addressHash: hexToBytes("87493abd360c531808c25f6e67405724926cc785580c4c724dd0142c3abcc14b"),
			internal:    false,
			compressed:  true,
			imported:    false,
			pubKey:      hexToBytes("02af08f3d1bb1ccaed1659e88b5a8d73f412628e44ddcbcbdc54b6b81f20c31375"),
			privKey:     hexToBytes("006499105900d03c7ef9cf27b87521b83848cd6836b1ddd75aa498329aaaf717"),
			privKeyWIF:  "KwEUVJA1MzXt8yaTzZSWbEsD5wwTtMcdiTNUxJH8MnnJmpz7Ufnd",
			derivationInfo: DerivationPath{
				InternalAccount: 0,
				Account:         hdkeychain.HardenedKeyStart,
				Branch:          0,
				Index:           0,
			},
		},
		{
			address:     "prl1pvdtcl546a64kk9p06nc75vrwlqfrl3m9myhl07wd69zvfma6fq3q9aduze",
			addressHash: hexToBytes("63578fd2baeeab6b142fd4f1ea306ef8123fc765d92ff7f9cdd144c4efba4822"),
			internal:    false,
			compressed:  true,
			imported:    false,
			pubKey:      hexToBytes("039879f3954272490ec03cc90377f77b8dd2df107cb011398f9d5b0b459f0a0978"),
			privKey:     hexToBytes("040c90c18fcfb63c5fbcf05c5bcaa03d7e9a3c196eb895517583f893d2b9ff8a"),
			privKeyWIF:  "KwMahLB6nVaRQoUqcfERGYP1id1NTUJtZ2zXoXyTfZNTRnVjt3AF",
			derivationInfo: DerivationPath{
				InternalAccount: 0,
				Account:         hdkeychain.HardenedKeyStart,
				Branch:          0,
				Index:           1,
			},
		},
		{
			address:     "prl1penz3j6q2rvnh02axv4ysxp6z789qtteezuq0jnm2nr5w562uaywqp6a2pw",
			addressHash: hexToBytes("ccc519680a1b2777aba66549030742f1ca05af391700f94f6a98e8ea695ce91c"),
			internal:    false,
			compressed:  true,
			imported:    false,
			pubKey:      hexToBytes("031dd01b86d496f37ed2b2daffb79adac90c13dd972733822bff8d15d0c2834a28"),
			privKey:     hexToBytes("f49ce36c93e23026f55d7498efd4691b06ad5d46f93a70206f55c41734567059"),
			privKeyWIF:  "L5RCvCshozHm5tha3e1k4gSVBrZKjvHiJptyc5bgR5aAmQ7Y6pnE",
			derivationInfo: DerivationPath{
				InternalAccount: 0,
				Account:         hdkeychain.HardenedKeyStart,
				Branch:          0,
				Index:           2,
			},
		},
		{
			address:     "prl1pdl6palz9pxn5zdufcx9wguuz95wruwwu2awmdkpjf5p2lvdk2snsq5kd62",
			addressHash: hexToBytes("6ff41efc4509a7413789c18ae473822d1c3e39dc575db6d8324d02afb1b65427"),
			internal:    false,
			compressed:  true,
			imported:    false,
			pubKey:      hexToBytes("03d2443e73ae88abf7980408cc5a8a0855a1b1ef491ad780f844b058e8ab64a768"),
			privKey:     hexToBytes("ba3b3fc97e1806b4e1aad94854d457bebe24e481b06920b003f851468f604bdb"),
			privKeyWIF:  "L3TijuoQs3wnewRDV6E2nLuMk5VhtycCivpTcnJHNJrdvo3oPN2g",
			derivationInfo: DerivationPath{
				InternalAccount: 0,
				Account:         hdkeychain.HardenedKeyStart,
				Branch:          0,
				Index:           3,
			},
		},
		{
			address:     "prl1p0qmn60e5r5y55ztcna3p7vnwdq5c2pp4xa69a0e7gxt5vvr5mspq9rv6jt",
			addressHash: hexToBytes("78373d3f341d094a09789f621f326e682985043537745ebf3e4197463074dc02"),
			internal:    false,
			compressed:  true,
			imported:    false,
			pubKey:      hexToBytes("0273e4a9dfd8f7c68e99c5c912634e76f27a80960d07e906caf7b1bf91faea0bd0"),
			privKey:     hexToBytes("569510b9bacee90b32dbdb8848228bbd15cafc01c587f830cfa144f5654e0b4f"),
			privKeyWIF:  "Kz81rW9iGh9xG19HBrgzdWo5oR5aA9UVJNA65GEiDpKrpaYUPsoM",
			derivationInfo: DerivationPath{
				InternalAccount: 0,
				Account:         hdkeychain.HardenedKeyStart,
				Branch:          0,
				Index:           4,
			},
		},
		{
			address:     "prl1pya7e4s6kl2tpf5awz34687j6cngw0lgrqd3qq927w2qzusxmgcgs4yad5m",
			addressHash: hexToBytes("277d9ac356fa9614d3ae146ba3fa5ac4d0e7fd03036200155e72802e40db4611"),
			internal:    true,
			compressed:  true,
			imported:    false,
			pubKey:      hexToBytes("03efe292b974eca9507b2fe823c256c4a2fb820d0ba3106bc68a16e871852e5503"),
			privKey:     hexToBytes("c690dbef68034da9065f0105e8206592ab72f0d51b50c68fac48a90a15e6f19d"),
			privKeyWIF:  "L3shNpUwT6TSxnwoqHJyLvWg1LBwtV4s6ARFXr2uNfoMKcaUeetD",
			derivationInfo: DerivationPath{
				InternalAccount: 0,
				Account:         hdkeychain.HardenedKeyStart,
				Branch:          1,
				Index:           0,
			},
		},
		{
			address:     "prl1pen8y8ph9jtqzn885qv4tlczmf5kg6k9cc53kkevu3wxenaq6zy0qs6rdc5",
			addressHash: hexToBytes("ccce4386e592c0299cf4032abfe05b4d2c8d58b8c5236b659c8b8d99f41a111e"),
			internal:    true,
			compressed:  true,
			imported:    false,
			pubKey:      hexToBytes("03894250e82aa208eb5660744374c9510704a1d559c92c85b9b33ee753db4a2e21"),
			privKey:     hexToBytes("4dffd85a035b89b0985044720704521ad827d7a1a8fbfb08e17dd3a231b77045"),
			privKeyWIF:  "KyqLBTknsGeTKM77xUsWan47ddXTyCg7CxSRtSUXxXnNYEFejwnc",
			derivationInfo: DerivationPath{
				InternalAccount: 0,
				Account:         hdkeychain.HardenedKeyStart,
				Branch:          1,
				Index:           1,
			},
		},
		{
			address:     "prl1pqz2jravv8q56wx2rpu8u84sdug5rah7rkt8dzqdvdndpclza0ysserpyel",
			addressHash: hexToBytes("009521f58c3829a719430f0fc3d60de2283edfc3b2ced101ac6cda1c7c5d7921"),
			internal:    true,
			compressed:  true,
			imported:    false,
			pubKey:      hexToBytes("023a16a31e71313a2ec965dab8bbe054481dcb878d6c33bbacd918ef3547911f95"),
			privKey:     hexToBytes("3c14dec5cab4f78a294b044b6bec19c4417c1ff5c8cb6c5af086d8a2a79b0b8d"),
			privKeyWIF:  "KyEW3Lv9oG7b4qzXjrGmSftsHjRpjEsrAm1UbZLBRbdEmm2WLPhT",
			derivationInfo: DerivationPath{
				InternalAccount: 0,
				Account:         hdkeychain.HardenedKeyStart,
				Branch:          1,
				Index:           2,
			},
		},
		{
			address:     "prl1pdcl4mszx3c44ywv75ff4nhk6mnvnt9gu298reyqk70p6murvvagsyt898x",
			addressHash: hexToBytes("6e3f5dc0468e2b52399ea25359dedadcd935951c514e3c9016f3c3adf06c6751"),
			internal:    true,
			compressed:  true,
			imported:    false,
			pubKey:      hexToBytes("0308ece0db6932bf9102577f492c6f1037f3fc7f25d66580a544c2abeddff3fb26"),
			privKey:     hexToBytes("4fc4cd687802340fad2f55370b09c1c66bc29773b02b22b720d5faabc4adde62"),
			privKeyWIF:  "KytmfeVMGDESoUGZpFy2qaqDtzp489iv4rbVk9U8WELXBjxzvzX5",
			derivationInfo: DerivationPath{
				InternalAccount: 0,
				Account:         hdkeychain.HardenedKeyStart,
				Branch:          1,
				Index:           3,
			},
		},
		{
			address:     "prl1plv3kmy9z60d7etcmwapjjfk0g0ahy8m9llt35pucs3zjdpkvffmqcgrxkq",
			addressHash: hexToBytes("fb236d90a2d3dbecaf1b77432926cf43fb721f65ffd71a079884452686cc4a76"),
			internal:    true,
			compressed:  true,
			imported:    false,
			pubKey:      hexToBytes("02cc2b418df288e09fa03018b0fee6ef9296834a9ddab04836ff707bc4bc4962f7"),
			privKey:     hexToBytes("cfcc43bb92fff0a6c370b6fb3deff52cd4f9f04316c6cf823cba38fbcd9f5fec"),
			privKeyWIF:  "L4BeEqff3kXjn6Egs9EddjjD7tXhYKKtcSmEg2X322rp8h8gRSeD",
			derivationInfo: DerivationPath{
				InternalAccount: 0,
				Account:         hdkeychain.HardenedKeyStart,
				Branch:          1,
				Index:           4,
			},
		},
	}

	// expectedExternalAddrs is the list of expected external addresses
	// generated from the seed
	expectedExternalAddrs = expectedAddrs[:5]

	// expectedInternalAddrs is the list of expected internal addresses
	// generated from the seed
	expectedInternalAddrs = expectedAddrs[5:]

	// defaultDBTimeout specifies the timeout value when opening the wallet
	// database.
	defaultDBTimeout = 10 * time.Second
)

// checkManagerError ensures the passed error is a ManagerError with an error
// code that matches the passed  error code.
func checkManagerError(t *testing.T, testName string, gotErr error,
	wantErrCode ErrorCode) bool {

	merr, ok := gotErr.(ManagerError)
	if !ok {
		t.Errorf("%s: unexpected error type - got %T, want %T",
			testName, gotErr, ManagerError{})
		return false
	}
	if merr.ErrorCode != wantErrCode {
		t.Errorf("%s: unexpected error code - got %s (%s), want %s",
			testName, merr.ErrorCode, merr.Description, wantErrCode)
		return false
	}

	return true
}

// hexToBytes is a wrapper around hex.DecodeString that panics if there is an
// error.  It MUST only be used with hard coded values in the tests.
func hexToBytes(origHex string) []byte {
	buf, err := hex.DecodeString(origHex)
	if err != nil {
		panic(err)
	}
	return buf
}

func emptyDB(t *testing.T) (tearDownFunc func(), db walletdb.DB) {
	dirName := t.TempDir()
	dbPath := filepath.Join(dirName, "mgrtest.db")
	db, err := walletdb.Create(
		"bdb", dbPath, true, defaultDBTimeout, false,
	)
	if err != nil {
		_ = os.RemoveAll(dirName)
		t.Fatalf("createDbNamespace: unexpected error: %v", err)
	}
	tearDownFunc = func() {
		db.Close()
	}
	return
}

// setupManager creates a new address manager and returns a teardown function
// that should be invoked to ensure it is closed and removed upon completion.
func setupManager(t *testing.T) (tearDownFunc func(), db walletdb.DB, mgr *Manager) {
	// Create a new manager in a temp directory.
	dirName := t.TempDir()

	dbPath := filepath.Join(dirName, "mgrtest.db")
	db, err := walletdb.Create("bdb", dbPath, true, defaultDBTimeout, false)
	if err != nil {
		_ = os.RemoveAll(dirName)
		t.Fatalf("createDbNamespace: unexpected error: %v", err)
	}
	err = walletdb.Update(db, func(tx walletdb.ReadWriteTx) error {
		ns, err := tx.CreateTopLevelBucket(waddrmgrNamespaceKey)
		if err != nil {
			return err
		}
		err = Create(
			ns, rootKey, pubPassphrase, privPassphrase,
			&chaincfg.MainNetParams, fastScrypt, time.Time{},
		)
		if err != nil {
			return err
		}
		mgr, err = Open(ns, pubPassphrase, &chaincfg.MainNetParams)
		return err
	})
	if err != nil {
		db.Close()
		_ = os.RemoveAll(dirName)
		t.Fatalf("Failed to create Manager: %v", err)
	}
	tearDownFunc = func() {
		mgr.Close()
		db.Close()
	}
	return tearDownFunc, db, mgr
}
