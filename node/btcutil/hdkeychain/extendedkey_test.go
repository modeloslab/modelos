// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package hdkeychain

// References:
//   [BIP32]: BIP0032 - Hierarchical Deterministic Wallets
//   https://github.com/bitcoin/bips/blob/master/bip-0032.mediawiki

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"math"
	"testing"

	secp_ecdsa "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/modelos/modelos/node/btcec/schnorr"
	"github.com/modelos/modelos/node/btcutil"
	"github.com/modelos/modelos/node/chaincfg"
	"github.com/modelos/modelos/node/txscript"
	"github.com/stretchr/testify/require"
)

func TaprootAddress(key *ExtendedKey, net *chaincfg.Params) (*btcutil.AddressTaproot, error) {
	pubKey, err := key.ECPubKey()
	if err != nil {
		return nil, err
	}
	tapKey := txscript.ComputeTaprootKeyNoScript(pubKey)
	tapKeyBytes := schnorr.SerializePubKey(tapKey)

	return btcutil.NewAddressTaproot(tapKeyBytes, net)
}

// TestBIP0032Vectors tests the vectors provided by [BIP32] to ensure the
// derivation works as intended.
func TestBIP0032Vectors(t *testing.T) {
	// The master seeds for each of the two test vectors in [BIP32].
	testVec1MasterHex := "000102030405060708090a0b0c0d0e0f"
	testVec2MasterHex := "fffcf9f6f3f0edeae7e4e1dedbd8d5d2cfccc9c6c3c0bdbab7b4b1aeaba8a5a29f9c999693908d8a8784817e7b7875726f6c696663605d5a5754514e4b484542"
	testVec3MasterHex := "4b381541583be4423346c643850da4b320e46a87ae3d2a4e6da11eba819cd4acba45d239319ac14f863b8d5ab5a0d0c64d2e8a1e7d1457df2e5a3c51c73235be"
	hkStart := uint32(0x80000000)

	tests := []struct {
		name     string
		master   string
		path     []uint32
		wantPub  string
		wantPriv string
		net      *chaincfg.Params
	}{
		// Test vector 1
		{
			name:     "test vector 1 chain m",
			master:   testVec1MasterHex,
			path:     []uint32{},
			wantPub:  "zpub6jftahH18ngZxUuv6oSniLNrBCSSE1B4EEU59bwTCEt8x6aS6b2mdfLxbS4QS53g85SWWP6wexqeer516433gYpZQoJie2tcMYdJ1SYYYAL",
			wantPriv: "zprvAWgYBBk7JR8GjzqSzmunMCS7dAbwpYTCs1YUMDXqduMA5JFHZ3iX5s2UkAR6vBdcCYYa1S5o1fVLrKsrnpCQ4WpUd6aVUWP1bS2Yy5DoaKv",
			net:      &chaincfg.MainNetParams,
		},
		{
			name:     "test vector 1 chain m/0H",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart},
			wantPub:  "zpub6mwJaQaUE3oZ763dJZKRbNUxW1znc5f4uqty7hKaAS5RKNscWpZrkohNNhd7BNxD8Hj5NceNPbujdF3935mRkSHHcS6yZLnpsUkrK1XoMLr",
			wantPriv: "zprvAYwxAu3aPgFFtbyACXnREEYDwzAJCcwDYcyNKJuxc6YSSaYTyHFcD1NtXRmSKu1ZubSGNjQfQ2LDa5uaSQUaratzgyFdiU5uJSfQQEgCdm3",
			net:      &chaincfg.MainNetParams,
		},
		{
			name:     "test vector 1 chain m/0H/1",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1},
			wantPub:  "zpub6p7RnC8MckgcwY652pDJC7NcZPytc7PMrAZohy6P74sdYvNkEczAiNbbnn5gbKfZ61M8A36UWCQDDYmxWQwKS67Dudwq2yo6WDHdc193BuK",
			wantPriv: "zprvAb85NgbTnP8Kj41bvngHpyRt1N9QCefWUweCuagmYjLeg83bh5fvAaH7wUxVvpncMgBxBZ16UjHdi2RoZkGZdkPSCkDMnDrkyEymwBC4DQJ",
			net:      &chaincfg.MainNetParams,
		},
		{
			name:     "test vector 1 chain m/0H/1/2H",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1, hkStart + 2},
			wantPub:  "zpub6rihpixDKdY2ohtAHQcfZ7QknYrQseyMBhoUxJPYGwF4xmZs3Npbcp9XLoGX3C6sSXSrmR55t8yBfsvFDWUrVYG4VXuy7vrnDuCmhDp6Kf1",
			wantPriv: "zprvAdjMRDRKVFyjbDohBP5fByU2EX1vUCFVpUst9uyvibi65yEiVqWM51q3VXB7u4J6s9DLn1x1u97XwEAZ9mBkkqq6ZMmuwA6haWizZhbiNWS",
			net:      &chaincfg.MainNetParams,
		},
		{
			name:     "test vector 1 chain m/0H/1/2H/2",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1, hkStart + 2, 2},
			wantPub:  "zpub6tx6fA5AW7D1tBMsyfpkSca4sN99WFHYdBgnDFv1bNogKsCE79WxpPcT28FuSzFbuqa2uRZ2vssJEJGFJov4D8jKQKU9x7kcEb7cqtKdeLV",
			wantPriv: "zprvAfxkFeYGfjeifhHQseHk5UdLKLJf6nZhFxmBQsWQ33GhT4s5ZcCiGbHyAqaaMihXvwM3PwBaoxZhQR51MBj6xER8xbs1VEvQjVCA5GEFbKw",
			net:      &chaincfg.MainNetParams,
		},
		{
			name:     "test vector 1 chain m/0H/1/2H/2/1000000000",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1, hkStart + 2, 2, 1000000000},
			wantPub:  "zpub6vfs8qgQdEbDQX9L6DPUkKa4fbiTKk6aRS5LTYJ9oTCB2zbcdZmd5VmWexArJFLKPXraQmUqU5ZtpifGQfQu4PtSGrtEkbLF3YjY61c8fKG",
			wantPriv: "zprvAhgWjL9Wns2vC34rzBrUPBdL7ZsxvHNj4D9jf9tYF7fCACGU62TNXhT2ohMnGdpcYLkJBJAXf16pQb5NtFjq5C4gJ6dEc9g81pkWpPf5sTW",
			net:      &chaincfg.MainNetParams,
		},

		// Test vector 2
		{
			name:     "test vector 2 chain m",
			master:   testVec2MasterHex,
			path:     []uint32{},
			wantPub:  "zpub6jftahH18ngZx6RFCxX5AY6cRParudRsupFoSYgHAd11F5bdGjoi4ZWYwY9P9KQe2dgJR3m743LuNdoEDExCpTpWGjWBUXPGfvJQHKojT9s",
			wantPriv: "zprvAWgYBBk7JR8GjcLn6vz4oQ9ssMkNWAi2YbLCeAGfcHU2NHGUjCVTWmC56ECA6hBhSbHzkiYFHmXXwrbxqEb6ft9F5WejcrHkAgenKzz3sjB",
			net:      &chaincfg.MainNetParams,
		},
		{
			name:     "test vector 2 chain m/0",
			master:   testVec2MasterHex,
			path:     []uint32{0},
			wantPub:  "zpub6nwdrQxxcoWeUN1YLjtxELW8nML7cbvATz1AcMhjF2rxw43br19nDQ5oX17GwQR5ExAUHGkrT8BsXoDv6PedA4UnYnErNsXeoQR4iauuAP2",
			wantPriv: "zprvAZxHSuS4nRxMFsw5EiMwsCZQEKVdD9CK6m5ZoyJ7ghKz4FiTJTqXfbmKfjTxjm2XrFXpeiwyivwfJNEnPX69c7Je5h4o1LTVJ6ZLhNYFREZ",
			net:      &chaincfg.MainNetParams,
		},
		{
			name:     "test vector 2 chain m/0/2147483647H",
			master:   testVec2MasterHex,
			path:     []uint32{0, hkStart + 2147483647},
			wantPub:  "zpub6p6h71zUzhgkeEDimBtbhgBhB6Z2A65rZ95hXkcVtmdMPMH4Tta6RTsQ8PMPh92B2U61w7cTkLcUCU6FsJ8jtCwR5rrujbnffngfZKgFVCk",
			wantPriv: "zprvAb7LhWTbAL8TRk9FfAMbLYExd4iXkdN1BvA6jNCtLS6NWYwuvMFqsfYvH5wwU1m1kUriRk9cmwSmjbyZ7thHBQkCWRJ8mrBk5stsTk5YQDx",
			net:      &chaincfg.MainNetParams,
		},
		{
			name:     "test vector 2 chain m/0/2147483647H/1",
			master:   testVec2MasterHex,
			path:     []uint32{0, hkStart + 2147483647, 1},
			wantPub:  "zpub6rufX2yRALywjqtrpr3Afd7vWdRXGH17saaZNpGqCeyo4B9DmQ6oKNFmyBYXV1iTceYZmkhhPTvJHGMGnCZh9YQKg94kx9DsbZMcookAWLV",
			wantPriv: "zprvAdvK7XSXKyReXMpPipWAJVBBxbb2rpHGWMexaRsDeKSpBNp5DrnYmZwJ7t9ZVixi4SDRB3E1BV7z4JkiAnDfBtZVeBpwcxA8ZgKbwQnrNS3",
			net:      &chaincfg.MainNetParams,
		},
		{
			name:     "test vector 2 chain m/0/2147483647H/1/2147483646H",
			master:   testVec2MasterHex,
			path:     []uint32{0, hkStart + 2147483647, 1, hkStart + 2147483646},
			wantPub:  "zpub6t5hRzummjwf2o8TZKri38p7RMttKFdcuHJSuXyF3RF3WXU9s52oqtcsbqg2gPEua6CCaFmQS5F9vuWfU5gCru6mx3PFWSKHTnSCLjbc4XQ",
			wantPriv: "zprvAf6M2VNswNPMpK3zTJKhfzsNsL4PunumY4Nr79ZdV5i4dj91KXiZJ6JPkasx89Lzc1W7UrzzZrhZsRr5gj89sjEfU8NFmABkuGL81CwRQ7u",
			net:      &chaincfg.MainNetParams,
		},
		{
			name:     "test vector 2 chain m/0/2147483647H/1/2147483646H/2",
			master:   testVec2MasterHex,
			path:     []uint32{0, hkStart + 2147483647, 1, hkStart + 2147483646, 2},
			wantPub:  "zpub6uSjPS8HHvFtn4KMHPzQ1tC5pRRzZaYhZSvLKKKjgD6daioj2hxa2TZsgEkJoxbc69QRrBGpKnG9RLaAJBN7B9xMbDDLSoc295dP1QgCKz4",
			wantPriv: "zprvAgTNyvbPTYhbZaEtBNTPekFMGPbWA7prCDzjWvv87sZehvUaVAeKUfFPpzYxHAS4eKMqmcdp1xYFuyjbR3VNvhyPGyskpaKUbdfg5o8afXT",
			net:      &chaincfg.MainNetParams,
		},

		// Test vector 3
		{
			name:     "test vector 3 chain m",
			master:   testVec3MasterHex,
			path:     []uint32{},
			wantPub:  "zpub6jftahH18ngZw9sQjM1sNXMeJ2uxfVb9dA2MqqhqSK5oDKptvt3g1pk5RXgMxuzHCiSAF3e7SiAkowS93wzeEoUePyQQD5q7Tdp6ooFXLSf",
			wantPriv: "zprvAWgYBBk7JR8GifnwdKUs1PQuk15UG2sJFw6m3TJDsyYpLXVkPLjRU2RbaDvL3iR2XHxkKeEMCCGFJWDH4fjXTMeVboDNpHdiTQK8yuEsMtG",
			net:      &chaincfg.MainNetParams,
		},
		{
			name:     "test vector 3 chain m/0H",
			master:   testVec3MasterHex,
			path:     []uint32{hkStart},
			wantPub:  "zpub6n36Kf78pA3v8gxoxVMNRn7JzPXLpM142eGJf5PgNd1AULxMWNpY6HXnjFWshGb2Q5w121PrHfNFSQNUzBvpp3kb55MHkwKuwpB1UETtD11",
			wantPriv: "zprvAZ3jv9aEynVcvCtLrTpN4eAaSMgrQtHCfRLhrgz4pHUBbYdCxqWHYVDJszGgPfjwyScALb2Dqvthx3rRdtKHD7r82w8RZSuXaRGnwPrYcpQ",
			net:      &chaincfg.MainNetParams,
		},

		// Test vector 1 - Testnet
		{
			name:     "test vector 1 chain m - testnet",
			master:   testVec1MasterHex,
			path:     []uint32{},
			wantPub:  "vpub5SLqN2bLY4WeZJ9SmNJHsyzqVKreTXD4ZnPC22MugDNcjhKX5xNX9QiQWcE4SSRzVWyHWUihpKRT7hckDGNzVc69wSX2JPcfGeNiT5c2XZy",
			wantPriv: "vprv9DMUxX4ShgxMLp4yfLmHWr46wJ2A44VDCZTbDdxJ7sqdrtzNYR4GbcPvfLakvZ1vZz5M1XhZB259KBRbv2YLsa659jno8s74WXmyQmgaevA",
			net:      &chaincfg.TestNetParams,
		},
		{
			name:     "test vector 1 chain m/0H - testnet",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart},
			wantPub:  "vpub5UcFMjtodKddhuH9y8Avm26wp9Qzqbh5FPp5z7k2eQZu6ychWBucGZ4pHsnmBkLXVjFrNiG8YxVY66atAJ7NZVYt95KHDhWsnaWGkhF4DrT",
			wantPriv: "vprv9FctxEMunx5LVRCgs6dvPtADG7aWS8yDtAtVBjLR652vEBHYxebMikkLSbw6LGPtH2y3Nq2RZNv22wTKZcpXfeAbDcTwNpoxDYQpqu4SeuA",
			net:      &chaincfg.TestNetParams,
		},
		{
			name:     "test vector 1 chain m/0H/1 - testnet",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1},
			wantPub:  "vpub5WnNZXSh22WhYMKbhP4oMkzbsXQ6qdRNBiUvaPWqb3N7LX7qDzKvE7y3hxFLbh3sTSsuA8iEfYz1gQKhddHGF9NpSHA8hLX9RK343hQy5iD",
			wantPriv: "vprv9Ho2A1uoBexQKsF8bMXnzd3sKVZcSAhWpVZKn17E2hq8TinggT1fgKeZrf89wCAvj7ijBecre5sSAsyYgxcWSof2jPRfSaaotLjCNrg9kRe",
			net:      &chaincfg.TestNetParams,
		},
		{
			name:     "test vector 1 chain m/0H/1/2H - testnet",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1, hkStart + 2},
			wantPub:  "vpub5ZPec4GYiuN7QX7gwyUAim2k6gGd7B1MXFibpiozkujYkNJx2kAM8ZWyFySB3ZVBoxydmWgr3VYz8jTzLipoJbXf2B8GnHaq8zxC8um49mA",
			wantPriv: "vprv9LQJCYjetXopC33DqwwAMd61YeS8hiHWA2o12LQPCaCZsZyoVCr6amCVQhLmuRgREak7n7Zn4VhLQ5iJGyXhZu6h5zzDbWpkVcUR1TknHv2",
			net:      &chaincfg.TestNetParams,
		},
		{
			name:     "test vector 1 chain m/0H/1/2H/2 - testnet",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1, hkStart + 2, 2},
			wantPub:  "vpub5bd3SVPVuP36UzbQeEgFcGC4BVZMjmKYxjbu5gLU5MJA7TwK6WriL8ytwJRZTMdvHH6ouXAo6ET6h9ozS2G12BzuvxgTcUUf9gs3HWbwhUL",
			wantPriv: "vprv9Ndh2yrc51UoGWWwYD9FF8FKdTisLJbhbWgJHHvrX1mBEfcAYyYTnLfR61kEN65rJNspQ2oLyK9VsGckUQ53mHgjVF5K9beTeawaWwc1Bxt",
			net:      &chaincfg.TestNetParams,
		},
		{
			name:     "test vector 1 chain m/0H/1/2H/2/1000000000 - testnet",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1, hkStart + 2, 2, 1000000000},
			wantPub:  "vpub5dLovAzk2WRJ1LNrknEyuyC3yj8fZG8akyzTKxicHRgepbLhcw7NbF8xa8LWJcidkyPMQs6bdS9hHaD1XskqsTA2oW6YQx4HxeUxXiizsXi",
			wantPriv: "vprv9QMTWfTrC8rznrJPekhyYqFKRhJB9oQjPm4rXaJzj69fwo1Z5Po83SpUisXSH1CvunH5BPnHpMgcsSd81U5mtFLGpjqYGWQAvvVwG74GXji",
			net:      &chaincfg.TestNetParams,
		},
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			masterSeed, err := hex.DecodeString(test.master)
			require.NoError(t, err, "DecodeString #%d (%s)", i, test.name)

			extKey, err := NewMaster(masterSeed, test.net)
			require.NoError(t, err, "NewMaster #%d (%s)", i, test.name)

			for _, childNum := range test.path {
				extKey, err = extKey.Derive(childNum)
				require.NoError(t, err, "Derive childNum %d", childNum)
			}

			require.Equal(t, uint8(len(test.path)), extKey.Depth(), "Depth should match fixture path")

			privStr := extKey.String()
			require.Equal(t, test.wantPriv, privStr, "Serialize #%d (%s): mismatched serialized private extended key", i, test.name)

			pubKey, err := extKey.Neuter()
			require.NoError(t, err, "Neuter #%d (%s)", i, test.name)

			// Neutering a second time should have no effect.
			pubKey, err = pubKey.Neuter()
			require.NoError(t, err, "Neuter second time #%d (%s)", i, test.name)

			pubStr := pubKey.String()
			require.Equal(t, test.wantPub, pubStr, "Neuter #%d (%s): mismatched serialized public extended key", i, test.name)
		})
	}
}

// TestPrivateDerivation tests several vectors which derive private keys from
// other private keys works as intended.
func TestPrivateDerivation(t *testing.T) {
	// The private extended keys for test vectors in [BIP32].
	testVec1MasterPrivKey := "zprvAWgYBBk7JR8GjzqSzmunMCS7dAbwpYTCs1YUMDXqduMA5JFHZ3iX5s2UkAR6vBdcCYYa1S5o1fVLrKsrnpCQ4WpUd6aVUWP1bS2Yy5DoaKv"
	testVec2MasterPrivKey := "zprvAWgYBBk7JR8GjcLn6vz4oQ9ssMkNWAi2YbLCeAGfcHU2NHGUjCVTWmC56ECA6hBhSbHzkiYFHmXXwrbxqEb6ft9F5WejcrHkAgenKzz3sjB"

	tests := []struct {
		name     string
		master   string
		path     []uint32
		wantPriv string
	}{
		// Test vector 1
		{
			name:     "test vector 1 chain m",
			master:   testVec1MasterPrivKey,
			path:     []uint32{},
			wantPriv: "zprvAWgYBBk7JR8GjzqSzmunMCS7dAbwpYTCs1YUMDXqduMA5JFHZ3iX5s2UkAR6vBdcCYYa1S5o1fVLrKsrnpCQ4WpUd6aVUWP1bS2Yy5DoaKv",
		},
		{
			name:     "test vector 1 chain m/0",
			master:   testVec1MasterPrivKey,
			path:     []uint32{0},
			wantPriv: "zprvAYwxAu3S41iHji2nuWZ6o3bPbVqK4Aixv5vXuoaMzm6CuEjbTKXXeU7K1N76R4FZ8NzTA4TYJFaU1Bw59zpZAxTuox1iroNABspCJkanJ8r",
		},
		{
			name:     "test vector 1 chain m/0/1",
			master:   testVec1MasterPrivKey,
			path:     []uint32{0, 1},
			wantPriv: "zprvAbbeUgbBHfSxfhyigZSNCSNy2FqkZrk37qMMqNrkgPTDG8ayhRyeMdokPQ7qxiv644G5NT7RrLno3irCD1Y1YAGU8sxmbWWgy2apnUMcCd8",
		},
		{
			name:     "test vector 1 chain m/0/1/2",
			master:   testVec1MasterPrivKey,
			path:     []uint32{0, 1, 2},
			wantPriv: "zprvAcX9zT43Kh6WFo4eotDrds1DhVjMsd4K2tXRK4KTVCi6o1dwkcCMtTMtrCuSAZuSpdxHkxzFUAUkzjFsL9DfFYJ96Ff9MrBQm3JkAMqdhah",
		},
		{
			name:     "test vector 1 chain m/0/1/2/2",
			master:   testVec1MasterPrivKey,
			path:     []uint32{0, 1, 2, 2},
			wantPriv: "zprvAfxeuAQV1kCutpZcnQZfiSVny5VBLzDTvsVoveLsJRs3BoSBFSxhW1Xr1BDPfSfH8aBUJ9rHbBtRH5wks41SLJ1thn9qhGPNoiveQgfW7ee",
		},
		{
			name:     "test vector 1 chain m/0/1/2/2/1000000000",
			master:   testVec1MasterPrivKey,
			path:     []uint32{0, 1, 2, 2, 1000000000},
			wantPriv: "zprvAhCECLJcufvGZzzVnnqvWC2wjLccgq9nLxwqPX7TzyacfwJL9R7N6ZFjQMBp6iaMLfgbFWdadsJ8E6PtYoXyKbgtAJRNk2eyzM7BsvEfKoW",
		},

		// Test vector 2
		{
			name:     "test vector 2 chain m",
			master:   testVec2MasterPrivKey,
			path:     []uint32{},
			wantPriv: "zprvAWgYBBk7JR8GjcLn6vz4oQ9ssMkNWAi2YbLCeAGfcHU2NHGUjCVTWmC56ECA6hBhSbHzkiYFHmXXwrbxqEb6ft9F5WejcrHkAgenKzz3sjB",
		},
		{
			name:     "test vector 2 chain m/0",
			master:   testVec2MasterPrivKey,
			path:     []uint32{0},
			wantPriv: "zprvAZxHSuS4nRxMFsw5EiMwsCZQEKVdD9CK6m5ZoyJ7ghKz4FiTJTqXfbmKfjTxjm2XrFXpeiwyivwfJNEnPX69c7Je5h4o1LTVJ6ZLhNYFREZ",
		},
		{
			name:     "test vector 2 chain m/0/2147483647",
			master:   testVec2MasterPrivKey,
			path:     []uint32{0, 2147483647},
			wantPriv: "zprvAb7LhWTSpfbVG2CTGgKW6E3yNcCLrievS2LX7VtK8wsU7oAELTk4eEZDQEZS4Rcse4bSjuCjFrvDP1VZeHgvrutPqEqLP7YeuKgyNzuBVsj",
		},
		{
			name:     "test vector 2 chain m/0/2147483647/1",
			master:   testVec2MasterPrivKey,
			path:     []uint32{0, 2147483647, 1},
			wantPriv: "zprvAdXxgwBvmxbA7ngrMQW4tYYz3Roa6MMyemC8SHF8JnfBzmJbEKJfSCZF6s4D1KzMQwYYvB8nQaYapAFH8E7FCNc21HzqppjoBFu2rBQS1cy",
		},
		{
			name:     "test vector 2 chain m/0/2147483647/1/2147483646",
			master:   testVec2MasterPrivKey,
			path:     []uint32{0, 2147483647, 1, 2147483646},
			wantPriv: "zprvAg1CFqrmia3emKDxMKncCer2Zz5PMVjniqBVMZGJwuLuy24r3puFbfNEByCcLP4WsYMfybry9Bcb5cUpnjGLTkFPgoWttTGFvMQJ7XYctDs",
		},
		{
			name:     "test vector 2 chain m/0/2147483647/1/2147483646/2",
			master:   testVec2MasterPrivKey,
			path:     []uint32{0, 2147483647, 1, 2147483646, 2},
			wantPriv: "zprvAhngx8U1X6hNw3UN5o17pAS3d3oYrnXt7r1vD8D2wQ3J1Hpkwb54kEwnr4PgT4JHvcVfLhHMbRf8LP4La5izpivrMVZq6SLjR7UGTH2e8Ro",
		},

		// Custom tests to trigger specific conditions.
		{
			// Seed 000000000000000000000000000000da.
			name:     "Derived privkey with zero high byte m/0",
			master:   "xprv9s21ZrQH143K4FR6rNeqEK4EBhRgLjWLWhA3pw8iqgAKk82ypz58PXbrzU19opYcxw8JDJQF4id55PwTsN1Zv8Xt6SKvbr2KNU5y8jN8djz",
			path:     []uint32{0},
			wantPriv: "xprv9uC5JqtViMmgcAMUxcsBCBFA7oYCNs4bozPbyvLfddjHou4rMiGEHipz94xNaPb1e4f18TRoPXfiXx4C3cDAcADqxCSRSSWLvMBRWPctSN9",
		},
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			extKey, err := NewKeyFromString(test.master)
			require.NoError(t, err, "NewKeyFromString #%d (%s)", i, test.name)

			for _, childNum := range test.path {
				extKey, err = extKey.Derive(childNum)
				require.NoError(t, err, "Derive childNum %d", childNum)
			}

			privStr := extKey.String()
			require.Equal(t, test.wantPriv, privStr, "Derive #%d (%s): mismatched serialized private extended key", i, test.name)
		})
	}
}

// TestPublicDerivation tests several vectors which derive public keys from
// other public keys works as intended.
func TestPublicDerivation(t *testing.T) {
	// The public extended keys for test vectors in [BIP32].
	testVec1MasterPubKey := "zpub6jftahH18ngZxUuv6oSniLNrBCSSE1B4EEU59bwTCEt8x6aS6b2mdfLxbS4QS53g85SWWP6wexqeer516433gYpZQoJie2tcMYdJ1SYYYAL"
	testVec2MasterPubKey := "zpub6jftahH18ngZx6RFCxX5AY6cRParudRsupFoSYgHAd11F5bdGjoi4ZWYwY9P9KQe2dgJR3m743LuNdoEDExCpTpWGjWBUXPGfvJQHKojT9s"

	tests := []struct {
		name    string
		master  string
		path    []uint32
		wantPub string
	}{
		// Test vector 1
		{
			name:    "test vector 1 chain m",
			master:  testVec1MasterPubKey,
			path:    []uint32{},
			wantPub: "zpub6jftahH18ngZxUuv6oSniLNrBCSSE1B4EEU59bwTCEt8x6aS6b2mdfLxbS4QS53g85SWWP6wexqeer516433gYpZQoJie2tcMYdJ1SYYYAL",
		},
		{
			name:    "test vector 1 chain m/0",
			master:  testVec1MasterPubKey,
			path:    []uint32{0},
			wantPub: "zpub6mwJaQaKtPGaxC7G1Y67ABY89XfoTdSpHJr8iByyZ6dBn34jzrqnCGRnrdVAsowq7HC8i8BWFpA3kAsCLnNn3DRTnNBhZfUj2bZGA9LYRAB",
		},
		{
			name:    "test vector 1 chain m/0/1",
			master:  testVec1MasterPubKey,
			path:    []uint32{0, 1},
			wantPub: "zpub6paztC85831FtC4BnayNZaKhaHgEyKTtV4GxdmGNEizC8vv8EyHtuS8EEgN7FAsZCTovuQCYdeDx6F8LvTkrGyEGGTXEzD6PaAESe59szwM",
		},
		{
			name:    "test vector 1 chain m/0/1/2",
			master:  testVec1MasterPubKey,
			path:    []uint32{0, 1, 2},
			wantPub: "zpub6qWWPxawA4eoUH97uukrzzwxFXZrH5nAQ7T27Sj53YF5foy6J9WcSFgNhWAcHKnXX294qc6iSJ7cRsrsTesusAHtTjEdrchtsuv5DptbJeH",
		},
		{
			name:    "test vector 1 chain m/0/1/2/2",
			master:  testVec1MasterPubKey,
			path:    []uint32{0, 1, 2, 2},
			wantPub: "zpub6tx1JfwNr7mD7Je5tS6g5aSXX7KfkSwKJ6RQj2kUrmQ24bmKnzGx3orKrTnBeCkfJ7Zz3i4CNRH95R3KiGpufEKoiWWjectMd4FcJDzkQwa",
		},
		{
			name:    "test vector 1 chain m/0/1/2/2/1000000000",
			master:  testVec1MasterPubKey,
			path:    []uint32{0, 1, 2, 2, 1000000000},
			wantPub: "zpub6vBabqqWk3UZnV4xtpNvsKygHNT76HsdiBsSBuX5ZK7bYjdUgxRceMaDFcjGvyieP9WeDnGuJ8KP19iMPaSqBsDxHt8Pfs3TKFLqmZKwHFC",
		},

		// Test vector 2
		{
			name:    "test vector 2 chain m",
			master:  testVec2MasterPubKey,
			path:    []uint32{},
			wantPub: "zpub6jftahH18ngZx6RFCxX5AY6cRParudRsupFoSYgHAd11F5bdGjoi4ZWYwY9P9KQe2dgJR3m743LuNdoEDExCpTpWGjWBUXPGfvJQHKojT9s",
		},
		{
			name:    "test vector 2 chain m/0",
			master:  testVec2MasterPubKey,
			path:    []uint32{0},
			wantPub: "zpub6nwdrQxxcoWeUN1YLjtxELW8nML7cbvATz1AcMhjF2rxw43br19nDQ5oX17GwQR5ExAUHGkrT8BsXoDv6PedA4UnYnErNsXeoQR4iauuAP2",
		},
		{
			name:    "test vector 2 chain m/0/2147483647",
			master:  testVec2MasterPubKey,
			path:    []uint32{0, 2147483647},
			wantPub: "zpub6p6h71zLf39nUWGvNhrWTMzhve2qGBNmoFG7utHvhHQSzbVNt14KC2shFXccD51PVC2SdsQoramgTaudRxouHHQgc1NxfCNBKtbb5GXCSbE",
		},
		{
			name:    "test vector 2 chain m/0/2147483647/1",
			master:  testVec2MasterPubKey,
			path:    []uint32{0, 2147483647, 1},
			wantPub: "zpub6rXK6SipcL9TLGmKTS35FgVibTe4Vp5q1z7jEfejs8CAsZdjmrcuyzsix9iTzTGVvqbvaRgygYEMAHTdU3SxsjsYywsoZ1n8yAxAeEz3WZy",
		},
		{
			name:    "test vector 2 chain m/0/2147483647/1/2147483646",
			master:  testVec2MasterPubKey,
			path:    []uint32{0, 2147483647, 1, 2147483646},
			wantPub: "zpub6tzYfMPfYwbwyoJRTMKcZnnm81uskxTe64769wfvWEstqpPzbNDW9Tgi3ECZcyeuSm3xCWJhQ5ji4fDNYiNqP8Rvu54H1GzRxdW3BTxaecL",
		},
		{
			name:    "test vector 2 chain m/0/2147483647/1/2147483646/2",
			master:  testVec2MasterPubKey,
			path:    []uint32{0, 2147483647, 1, 2147483646, 2},
			wantPub: "zpub6vn3MdzuMUFg9XYqBpY8BJNnB5e3GFFjV4wX1WceVjaGt69uV8PKJ3GGhMZoG3z1B9TaZgS3HnDzivCyGgNAnUPtJsAXeTNzjiuEMrY6CJa",
		},
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			extKey, err := NewKeyFromString(test.master)
			require.NoError(t, err, "NewKeyFromString #%d (%s)", i, test.name)

			for _, childNum := range test.path {
				extKey, err = extKey.Derive(childNum)
				require.NoError(t, err, "Derive childNum %d", childNum)
			}

			pubStr := extKey.String()
			require.Equal(t, test.wantPub, pubStr, "Derive #%d (%s): mismatched serialized public extended key", i, test.name)
		})
	}
}

// TestGenerateSeed ensures the GenerateSeed function works as intended.
func TestGenerateSeed(t *testing.T) {
	wantErr := ErrInvalidSeedLen

	tests := []struct {
		name   string
		length uint8
		err    error
	}{
		// Test various valid lengths.
		{name: "16 bytes", length: 16},
		{name: "17 bytes", length: 17},
		{name: "20 bytes", length: 20},
		{name: "32 bytes", length: 32},
		{name: "64 bytes", length: 64},

		// Test invalid lengths.
		{name: "15 bytes", length: 15, err: wantErr},
		{name: "65 bytes", length: 65, err: wantErr},
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			seed, err := GenerateSeed(test.length)
			if test.err != nil {
				require.Error(t, err, "GenerateSeed #%d (%s) should have error", i, test.name)
				require.Contains(t, err.Error(), test.err.Error(), "GenerateSeed #%d (%s): unexpected error message", i, test.name)
			} else {
				require.NoError(t, err, "GenerateSeed #%d (%s)", i, test.name)
				require.Len(t, seed, int(test.length), "GenerateSeed #%d (%s): length mismatch", i, test.name)
			}
		})
	}
}

// TestExtendedKeyAPI ensures the API on the ExtendedKey type works as intended.
func TestExtendedKeyAPI(t *testing.T) {
	tests := []struct {
		name       string
		extKey     string
		isPrivate  bool
		parentFP   uint32
		chainCode  []byte
		childNum   uint32
		privKey    string
		privKeyErr error
		pubKey     string
		address    string
	}{
		{
			name:      "test vector 1 master node private",
			extKey:    "zprvAWgYBBk7JR8GjzqSzmunMCS7dAbwpYTCs1YUMDXqduMA5JFHZ3iX5s2UkAR6vBdcCYYa1S5o1fVLrKsrnpCQ4WpUd6aVUWP1bS2Yy5DoaKv",
			isPrivate: true,
			parentFP:  0,
			chainCode: []byte{135, 61, 255, 129, 192, 47, 82, 86, 35, 253, 31, 229, 22, 126, 172, 58, 85, 160, 73, 222, 61, 49, 75, 180, 46, 226, 39, 255, 237, 55, 213, 8},
			childNum:  0,
			privKey:   "e8f32e723decf4051aefac8e2c93c9c5b214313817cdb01a1494b917c8436b35",
			pubKey:    "0339a36013301597daef41fbe593a02cc513d0b55527ec2df1050e2e8ff49c85c2",
			address:   "prl1p7x4krdxtfwv5mwu2hhq6y2pnp5msc8mnczk5rr06zsulrn395zzqx54wvy",
		},
		{
			name:       "test vector 1 chain m/0H/1/2H public",
			extKey:     "xpub6D4BDPcP2GT577Vvch3R8wDkScZWzQzMMUm3PWbmWvVJrZwQY4VUNgqFJPMM3No2dFDFGTsxxpG5uJh7n7epu4trkrX7x7DogT5Uv6fcLW5",
			isPrivate:  false,
			parentFP:   3203769081,
			chainCode:  []byte{4, 70, 107, 156, 200, 225, 97, 233, 102, 64, 156, 165, 41, 134, 197, 132, 240, 126, 157, 200, 31, 115, 93, 182, 131, 195, 255, 110, 199, 177, 80, 63},
			childNum:   2147483650,
			privKeyErr: ErrNotPrivExtKey,
			pubKey:     "0357bfe1e341d01c69fe5654309956cbea516822fba8a601743a012a7896ee8dc2",
			address:    "prl1px0v4xlgzt2u90ght4q7c95zk7t3elv2pw5mvcxcd6pdyqf2ph8yq0fhp92",
		},
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			key, err := NewKeyFromString(test.extKey)
			require.NoError(t, err, "NewKeyFromString #%d (%s)", i, test.name)

			require.Equal(t, test.isPrivate, key.IsPrivate(), "IsPrivate #%d (%s): mismatched key type", i, test.name)

			parentFP := key.ParentFingerprint()
			require.Equal(t, test.parentFP, parentFP, "ParentFingerprint #%d (%s): mismatched parent fingerprint", i, test.name)

			chainCode := key.ChainCode()
			require.Equal(t, test.chainCode, chainCode, "ChainCode #%d (%s)", i, test.name)

			childIndex := key.ChildIndex()
			require.Equal(t, test.childNum, childIndex, "ChildIndex #%d (%s)", i, test.name)

			serializedKey := key.String()
			require.Equal(t, test.extKey, serializedKey, "String #%d (%s): mismatched serialized key", i, test.name)

			privKey, err := key.ECPrivKey()
			if test.privKeyErr != nil {
				require.ErrorIs(t, err, test.privKeyErr, "ECPrivKey #%d (%s): mismatched error", i, test.name)
			} else {
				require.NoError(t, err, "ECPrivKey #%d (%s)", i, test.name)
				privKeyStr := hex.EncodeToString(privKey.Serialize())
				require.Equal(t, test.privKey, privKeyStr, "ECPrivKey #%d (%s): mismatched private key", i, test.name)
			}

			pubKey, err := key.ECPubKey()
			require.NoError(t, err, "ECPubKey #%d (%s)", i, test.name)
			pubKeyStr := hex.EncodeToString(pubKey.SerializeCompressed())
			require.Equal(t, test.pubKey, pubKeyStr, "ECPubKey #%d (%s): mismatched public key", i, test.name)

			addr, err := TaprootAddress(key, &chaincfg.MainNetParams)
			require.NoError(t, err, "Address #%d (%s)", i, test.name)
			require.Equal(t, test.address, addr.EncodeAddress(), "Address #%d (%s): mismatched address", i, test.name)
		})
	}
}

// TestNet ensures the network related APIs work as intended.
func TestNet(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		origNet   *chaincfg.Params
		newNet    *chaincfg.Params
		newPriv   string
		newPub    string
		isPrivate bool
	}{
		// Private extended keys.
		{
			name:      "mainnet -> simnet",
			key:       "zprvAWgYBBk7JR8GjzqSzmunMCS7dAbwpYTCs1YUMDXqduMA5JFHZ3iX5s2UkAR6vBdcCYYa1S5o1fVLrKsrnpCQ4WpUd6aVUWP1bS2Yy5DoaKv",
			origNet:   &chaincfg.MainNetParams,
			newNet:    &chaincfg.SimNetParams,
			newPriv:   "sprv8Erh3X3hFeKunvVdAGQQtambRPapECWiTDtvsTGdyrhzhbYgnSZajRRWbihzvq4AM4ivm6uso31VfKaukwJJUs3GYihXP8ebhMb3F2AHu3P",
			newPub:    "spub4Tr3T2ab61tD1Qa6GHwRFiiKyRRJdfEZpSpXfqgFYCEyaPsqKysqHDjzSzMJSiUEGbcsG3w2SLMoTqn44B8x6u3MLRRkYfACTUBnHK79THk",
			isPrivate: true,
		},
		{
			name:      "simnet -> mainnet",
			key:       "sprv8Erh3X3hFeKunvVdAGQQtambRPapECWiTDtvsTGdyrhzhbYgnSZajRRWbihzvq4AM4ivm6uso31VfKaukwJJUs3GYihXP8ebhMb3F2AHu3P",
			origNet:   &chaincfg.SimNetParams,
			newNet:    &chaincfg.MainNetParams,
			newPriv:   "zprvAWgYBBk7JR8GjzqSzmunMCS7dAbwpYTCs1YUMDXqduMA5JFHZ3iX5s2UkAR6vBdcCYYa1S5o1fVLrKsrnpCQ4WpUd6aVUWP1bS2Yy5DoaKv",
			newPub:    "zpub6jftahH18ngZxUuv6oSniLNrBCSSE1B4EEU59bwTCEt8x6aS6b2mdfLxbS4QS53g85SWWP6wexqeer516433gYpZQoJie2tcMYdJ1SYYYAL",
			isPrivate: true,
		},
		{
			name:      "mainnet -> regtest",
			key:       "zprvAWgYBBk7JR8GjzqSzmunMCS7dAbwpYTCs1YUMDXqduMA5JFHZ3iX5s2UkAR6vBdcCYYa1S5o1fVLrKsrnpCQ4WpUd6aVUWP1bS2Yy5DoaKv",
			origNet:   &chaincfg.MainNetParams,
			newNet:    &chaincfg.RegressionNetParams,
			newPriv:   "vprv9DMUxX4ShgxMLp4yfLmHWr46wJ2A44VDCZTbDdxJ7sqdrtzNYR4GbcPvfLakvZ1vZz5M1XhZB259KBRbv2YLsa659jno8s74WXmyQmgaevA",
			newPub:    "vpub5SLqN2bLY4WeZJ9SmNJHsyzqVKreTXD4ZnPC22MugDNcjhKX5xNX9QiQWcE4SSRzVWyHWUihpKRT7hckDGNzVc69wSX2JPcfGeNiT5c2XZy",
			isPrivate: true,
		},
		{
			name:      "regtest -> mainnet",
			key:       "vprv9DMUxX4ShgxMLp4yfLmHWr46wJ2A44VDCZTbDdxJ7sqdrtzNYR4GbcPvfLakvZ1vZz5M1XhZB259KBRbv2YLsa659jno8s74WXmyQmgaevA",
			origNet:   &chaincfg.RegressionNetParams,
			newNet:    &chaincfg.MainNetParams,
			newPriv:   "zprvAWgYBBk7JR8GjzqSzmunMCS7dAbwpYTCs1YUMDXqduMA5JFHZ3iX5s2UkAR6vBdcCYYa1S5o1fVLrKsrnpCQ4WpUd6aVUWP1bS2Yy5DoaKv",
			newPub:    "zpub6jftahH18ngZxUuv6oSniLNrBCSSE1B4EEU59bwTCEt8x6aS6b2mdfLxbS4QS53g85SWWP6wexqeer516433gYpZQoJie2tcMYdJ1SYYYAL",
			isPrivate: true,
		},

		// Public extended keys.
		{
			name:      "mainnet -> simnet",
			key:       "zpub6jftahH18ngZxUuv6oSniLNrBCSSE1B4EEU59bwTCEt8x6aS6b2mdfLxbS4QS53g85SWWP6wexqeer516433gYpZQoJie2tcMYdJ1SYYYAL",
			origNet:   &chaincfg.MainNetParams,
			newNet:    &chaincfg.SimNetParams,
			newPub:    "spub4Tr3T2ab61tD1Qa6GHwRFiiKyRRJdfEZpSpXfqgFYCEyaPsqKysqHDjzSzMJSiUEGbcsG3w2SLMoTqn44B8x6u3MLRRkYfACTUBnHK79THk",
			isPrivate: false,
		},
		{
			name:      "simnet -> mainnet",
			key:       "spub4Tr3T2ab61tD1Qa6GHwRFiiKyRRJdfEZpSpXfqgFYCEyaPsqKysqHDjzSzMJSiUEGbcsG3w2SLMoTqn44B8x6u3MLRRkYfACTUBnHK79THk",
			origNet:   &chaincfg.SimNetParams,
			newNet:    &chaincfg.MainNetParams,
			newPub:    "zpub6jftahH18ngZxUuv6oSniLNrBCSSE1B4EEU59bwTCEt8x6aS6b2mdfLxbS4QS53g85SWWP6wexqeer516433gYpZQoJie2tcMYdJ1SYYYAL",
			isPrivate: false,
		},
		{
			name:      "mainnet -> regtest",
			key:       "zpub6jftahH18ngZxUuv6oSniLNrBCSSE1B4EEU59bwTCEt8x6aS6b2mdfLxbS4QS53g85SWWP6wexqeer516433gYpZQoJie2tcMYdJ1SYYYAL",
			origNet:   &chaincfg.MainNetParams,
			newNet:    &chaincfg.RegressionNetParams,
			newPub:    "vpub5SLqN2bLY4WeZJ9SmNJHsyzqVKreTXD4ZnPC22MugDNcjhKX5xNX9QiQWcE4SSRzVWyHWUihpKRT7hckDGNzVc69wSX2JPcfGeNiT5c2XZy",
			isPrivate: false,
		},
		{
			name:      "regtest -> mainnet",
			key:       "vpub5SLqN2bLY4WeZJ9SmNJHsyzqVKreTXD4ZnPC22MugDNcjhKX5xNX9QiQWcE4SSRzVWyHWUihpKRT7hckDGNzVc69wSX2JPcfGeNiT5c2XZy",
			origNet:   &chaincfg.RegressionNetParams,
			newNet:    &chaincfg.MainNetParams,
			newPub:    "zpub6jftahH18ngZxUuv6oSniLNrBCSSE1B4EEU59bwTCEt8x6aS6b2mdfLxbS4QS53g85SWWP6wexqeer516433gYpZQoJie2tcMYdJ1SYYYAL",
			isPrivate: false,
		},
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			extKey, err := NewKeyFromString(test.key)
			require.NoError(t, err, "NewKeyFromString #%d (%s)", i, test.name)

			require.True(t, extKey.IsForNet(test.origNet), "IsForNet #%d (%s): key is not for expected network %v", i, test.name, test.origNet.Name)

			extKey.SetNet(test.newNet)
			require.True(t, extKey.IsForNet(test.newNet), "SetNet/IsForNet #%d (%s): key is not for expected network %v", i, test.name, test.newNet.Name)

			if test.isPrivate {
				privStr := extKey.String()
				require.Equal(t, test.newPriv, privStr, "Serialize #%d (%s): mismatched serialized private extended key", i, test.name)

				extKey, err = extKey.Neuter()
				require.NoError(t, err, "Neuter #%d (%s)", i, test.name)
			}

			pubStr := extKey.String()
			require.Equal(t, test.newPub, pubStr, "Neuter #%d (%s): mismatched serialized public extended key", i, test.name)
		})
	}
}

// TestErrors performs some negative tests for various invalid cases to ensure
// the errors are handled properly.
func TestErrors(t *testing.T) {
	// Should get an error when seed has too few bytes.
	net := &chaincfg.MainNetParams
	_, err := NewMaster(bytes.Repeat([]byte{0x00}, 15), net)
	require.ErrorIs(t, err, ErrInvalidSeedLen)

	// Should get an error when seed has too many bytes.
	_, err = NewMaster(bytes.Repeat([]byte{0x00}, 65), net)
	require.ErrorIs(t, err, ErrInvalidSeedLen)

	// Generate a new key and neuter it to a public extended key.
	seed, err := GenerateSeed(RecommendedSeedLen)
	require.NoError(t, err)
	extKey, err := NewMaster(seed, net)
	require.NoError(t, err)
	pubKey, err := extKey.Neuter()
	require.NoError(t, err)

	// Deriving a hardened child extended key should fail from a public key.
	_, err = pubKey.Derive(HardenedKeyStart)
	require.ErrorIs(t, err, ErrDeriveHardFromPublic)

	// NewKeyFromString failure tests.
	tests := []struct {
		name      string
		key       string
		err       error
		neuter    bool
		neuterErr error
	}{
		{
			name: "invalid key length",
			key:  "xpub1234",
			err:  ErrInvalidKeyLen,
		},
		{
			name: "bad checksum",
			key:  "xpub661MyMwAqRbcFtXgS5sYJABqqG9YLmC4Q1Rdap9gSE8NqtwybGhePY2gZ29ESFjqJoCu1Rupje8YtGqsefD265TMg7usUDFdp6W1EBygr15",
			err:  ErrBadChecksum,
		},
		{
			name: "pubkey not on curve",
			key:  "xpub661MyMwAqRbcFtXgS5sYJABqqG9YLmC4Q1Rdap9gSE8NqtwybGhePY2gZ1hr9Rwbk95YadvBkQXxzHBSngB8ndpW6QH7zhhsXZ2jHyZqPjk",
			err:  secp_ecdsa.ErrPubKeyNotOnCurve,
		},
		{
			name:      "unsupported version",
			key:       "xbad4LfUL9eKmA66w2GJdVMqhvDmYGJpTGjWRAtjHqoUY17sGaymoMV9Cm3ocn9Ud6Hh2vLFVC7KSKCRVVrqc6dsEdsTjRV1WUmkK85YEUujAPX",
			err:       nil,
			neuter:    true,
			neuterErr: chaincfg.ErrUnknownHDKeyID,
		},
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			extKey, err := NewKeyFromString(test.key)
			require.ErrorIs(t, err, test.err, "NewKeyFromString #%d (%s): mismatched error", i, test.name)

			if test.neuter {
				_, err := extKey.Neuter()
				require.ErrorIs(t, err, test.neuterErr, "Neuter #%d (%s): mismatched error", i, test.name)
			}
		})
	}
}

// TestZero ensures that zeroing an extended key works as intended.
func TestZero(t *testing.T) {
	tests := []struct {
		name   string
		master string
		extKey string
		net    *chaincfg.Params
	}{
		// Test vector 1
		{
			name:   "test vector 1 chain m",
			master: "000102030405060708090a0b0c0d0e0f",
			extKey: "zprvAWgYBBk7JR8GjzqSzmunMCS7dAbwpYTCs1YUMDXqduMA5JFHZ3iX5s2UkAR6vBdcCYYa1S5o1fVLrKsrnpCQ4WpUd6aVUWP1bS2Yy5DoaKv",
			net:    &chaincfg.MainNetParams,
		},

		// Test vector 2
		{
			name:   "test vector 2 chain m",
			master: "fffcf9f6f3f0edeae7e4e1dedbd8d5d2cfccc9c6c3c0bdbab7b4b1aeaba8a5a29f9c999693908d8a8784817e7b7875726f6c696663605d5a5754514e4b484542",
			extKey: "zprvAWgYBBk7JR8GjcLn6vz4oQ9ssMkNWAi2YbLCeAGfcHU2NHGUjCVTWmC56ECA6hBhSbHzkiYFHmXXwrbxqEb6ft9F5WejcrHkAgenKzz3sjB",
			net:    &chaincfg.MainNetParams,
		},
	}

	// Use a helper to test that a key is zeroed since the tests create
	// keys in different ways and need to test the same things multiple
	// times.
	testZeroed := func(t *testing.T, testName string, key *ExtendedKey) {
		// Zeroing a key should result in it no longer being private
		require.False(t, key.IsPrivate(), "IsPrivate (%s): key should not be private after zeroing", testName)

		parentFP := key.ParentFingerprint()
		require.Equal(t, uint32(0), parentFP, "ParentFingerprint (%s): should be 0 after zeroing", testName)

		wantKey := "zeroed extended key"
		serializedKey := key.String()
		require.Equal(t, wantKey, serializedKey, "String (%s): mismatched serialized key after zeroing", testName)

		_, err := key.ECPrivKey()
		require.ErrorIs(t, err, ErrNotPrivExtKey, "ECPrivKey (%s): should return ErrNotPrivExtKey after zeroing", testName)

		_, err = key.ECPubKey()
		require.ErrorIs(t, err, secp_ecdsa.ErrPubKeyInvalidLen, "ECPubKey (%s): should return ErrPubKeyInvalidLen after zeroing", testName)

		// After zeroing, TaprootAddress should return an error due to invalid key
		_, err = TaprootAddress(key, &chaincfg.MainNetParams)
		require.Error(t, err, "Address (%s): TaprootAddress should error after zeroing due to invalid key", testName)
		require.ErrorIs(t, err, secp_ecdsa.ErrPubKeyInvalidLen, "Address (%s): should get malformed public key error after zeroing", testName)
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create new key from seed and get the neutered version.
			masterSeed, err := hex.DecodeString(test.master)
			require.NoError(t, err, "DecodeString #%d (%s)", i, test.name)

			key, err := NewMaster(masterSeed, test.net)
			require.NoError(t, err, "NewMaster #%d (%s)", i, test.name)

			neuteredKey, err := key.Neuter()
			require.NoError(t, err, "Neuter #%d (%s)", i, test.name)

			// Ensure both non-neutered and neutered keys are zeroed
			// properly.
			key.Zero()
			testZeroed(t, test.name+" from seed not neutered", key)

			neuteredKey.Zero()
			testZeroed(t, test.name+" from seed neutered", neuteredKey)

			// Deserialize key and get the neutered version.
			key, err = NewKeyFromString(test.extKey)
			require.NoError(t, err, "NewKeyFromString #%d (%s)", i, test.name)

			neuteredKey, err = key.Neuter()
			require.NoError(t, err, "Neuter #%d (%s)", i, test.name)

			// Ensure both non-neutered and neutered keys are zeroed
			// properly.
			key.Zero()
			testZeroed(t, test.name+" deserialized not neutered", key)

			neuteredKey.Zero()
			testZeroed(t, test.name+" deserialized neutered", neuteredKey)
		})
	}
}

// TestMaximumDepth ensures that attempting to retrieve a child key when already
// at the maximum depth is not allowed.  The serialization of a BIP32 key uses
// uint8 to encode the depth.  This implicitly bounds the depth of the tree to
// 255 derivations.  Here we test that an error is returned after 'max uint8'.
func TestMaximumDepth(t *testing.T) {
	net := &chaincfg.MainNetParams
	extKey, err := NewMaster([]byte(`abcd1234abcd1234abcd1234abcd1234`), net)
	require.NoError(t, err)

	for i := uint8(0); i < math.MaxUint8; i++ {
		require.Equalf(t, i, extKey.Depth(), "extendedkey depth %d should match expected value %d", extKey.Depth(), i)
		newKey, err := extKey.Derive(1)
		require.NoError(t, err)
		extKey = newKey
	}

	noKey, err := extKey.Derive(1)
	require.ErrorIs(t, err, ErrDeriveBeyondMaxDepth)
	require.Nil(t, noKey, "Derive: deriving 256th key should not succeed")
}

// TestCloneWithVersion ensures proper conversion between standard and SLIP132
// extended keys.
//
// The following tool was used for generating the tests:
//
//	https://jlopp.github.io/xpub-converter
func TestCloneWithVersion(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		version []byte
		want    string
		wantErr error
	}{
		{
			name:    "test xpub to zpub",
			key:     "zpub6jftahH18ngZxUuv6oSniLNrBCSSE1B4EEU59bwTCEt8x6aS6b2mdfLxbS4QS53g85SWWP6wexqeer516433gYpZQoJie2tcMYdJ1SYYYAL",
			version: []byte{0x04, 0xb2, 0x47, 0x46},
			want:    "zpub6jftahH18ngZxUuv6oSniLNrBCSSE1B4EEU59bwTCEt8x6aS6b2mdfLxbS4QS53g85SWWP6wexqeer516433gYpZQoJie2tcMYdJ1SYYYAL",
		},
		{
			name:    "test zpub to xpub",
			key:     "zpub6jftahH18ngZxUuv6oSniLNrBCSSE1B4EEU59bwTCEt8x6aS6b2mdfLxbS4QS53g85SWWP6wexqeer516433gYpZQoJie2tcMYdJ1SYYYAL",
			version: []byte{0x04, 0x88, 0xb2, 0x1e},
			want:    "xpub661MyMwAqRbcFtXgS5sYJABqqG9YLmC4Q1Rdap9gSE8NqtwybGhePY2gZ29ESFjqJoCu1Rupje8YtGqsefD265TMg7usUDFdp6W1EGMcet8",
		},
		{
			name:    "test xprv to zprv",
			key:     "zprvAWgYBBk7JR8GjzqSzmunMCS7dAbwpYTCs1YUMDXqduMA5JFHZ3iX5s2UkAR6vBdcCYYa1S5o1fVLrKsrnpCQ4WpUd6aVUWP1bS2Yy5DoaKv",
			version: []byte{0x04, 0xb2, 0x43, 0x0c},
			want:    "zprvAWgYBBk7JR8GjzqSzmunMCS7dAbwpYTCs1YUMDXqduMA5JFHZ3iX5s2UkAR6vBdcCYYa1S5o1fVLrKsrnpCQ4WpUd6aVUWP1bS2Yy5DoaKv",
		},
		{
			name:    "test zprv to xprv",
			key:     "zprvAWgYBBk7JR8GjzqSzmunMCS7dAbwpYTCs1YUMDXqduMA5JFHZ3iX5s2UkAR6vBdcCYYa1S5o1fVLrKsrnpCQ4WpUd6aVUWP1bS2Yy5DoaKv",
			version: []byte{0x04, 0x88, 0xad, 0xe4},
			want:    "xprv9s21ZrQH143K3QTDL4LXw2F7HEK3wJUD2nW2nRk4stbPy6cq3jPPqjiChkVvvNKmPGJxWUtg6LnF5kejMRNNU3TGtRBeJgk33yuGBxrMPHi",
		},
		{
			name:    "test invalid key id",
			key:     "zprvAWgYBBk7JR8GjzqSzmunMCS7dAbwpYTCs1YUMDXqduMA5JFHZ3iX5s2UkAR6vBdcCYYa1S5o1fVLrKsrnpCQ4WpUd6aVUWP1bS2Yy5DoaKv",
			version: []byte{0x4B, 0x1D},
			wantErr: chaincfg.ErrUnknownHDKeyID,
		},
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			extKey, err := NewKeyFromString(test.key)
			require.NoError(t, err, "NewKeyFromString should not fail for test #%d (%s)", i, test.name)

			got, err := extKey.CloneWithVersion(test.version)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr, "CloneWithVersion #%d (%s): unexpected error", i, test.name)
			} else {
				require.NoError(t, err, "CloneWithVersion #%d (%s)", i, test.name)
				k := got.String()
				require.Equal(t, test.want, k, "CloneWithVersion #%d (%s): unexpected result", i, test.name)
			}
		})
	}
}

// TestLeadingZero ensures that deriving children from keys with a leading zero byte is done according
// to the BIP-32 standard and that the legacy method generates a backwards-compatible result.
func TestLeadingZero(t *testing.T) {
	// The 400th seed results in a m/0' public key with a leading zero, allowing us to test
	// the desired behavior.
	ii := 399
	seed := make([]byte, 32)
	binary.BigEndian.PutUint32(seed[28:], uint32(ii))
	masterKey, err := NewMaster(seed, &chaincfg.MainNetParams)
	require.NoError(t, err)
	child0, err := masterKey.Derive(0 + HardenedKeyStart)
	require.NoError(t, err)
	require.True(t, child0.IsAffectedByIssue172(), "expected child0 to be affected by issue 172")
	child1, err := child0.Derive(0 + HardenedKeyStart)
	require.NoError(t, err)
	require.False(t, child1.IsAffectedByIssue172(), "did not expect child1 to be affected by issue 172")

	child1nonstandard, err := child0.DeriveNonStandard(0 + HardenedKeyStart)
	require.NoError(t, err)

	// This is the correct result based on BIP32
	require.Equal(t, "a9b6b30a5b90b56ed48728c73af1d8a7ef1e9cc372ec21afcc1d9bdf269b0988", hex.EncodeToString(child1.key), "incorrect standard BIP32 derivation")

	require.Equal(t, "ea46d8f58eb863a2d371a938396af8b0babe85c01920f59a8044412e70e837ee", hex.EncodeToString(child1nonstandard.key), "incorrect btcutil backwards compatible BIP32-like derivation")

	require.True(t, child0.IsAffectedByIssue172(), "child 0 should be affected by issue 172")

	require.False(t, child1.IsAffectedByIssue172(), "child 1 should not be affected by issue 172")
}
