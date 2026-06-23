// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package hdkeychain_test

import (
	"fmt"

	"github.com/modelos/modelos/node/btcec/schnorr"
	"github.com/modelos/modelos/node/btcutil"
	"github.com/modelos/modelos/node/btcutil/hdkeychain"
	"github.com/modelos/modelos/node/chaincfg"
	"github.com/modelos/modelos/node/txscript"
)

func TaprootAddress(key *hdkeychain.ExtendedKey, net *chaincfg.Params) (*btcutil.AddressTaproot, error) {
	pubKey, err := key.ECPubKey()
	if err != nil {
		return nil, err
	}
	tapKey := txscript.ComputeTaprootKeyNoScript(pubKey)
	tapKeyBytes := schnorr.SerializePubKey(tapKey)

	return btcutil.NewAddressTaproot(tapKeyBytes, net)
}

// This example demonstrates how to generate a cryptographically random seed
// then use it to create a new master node (extended key).
func ExampleNewMaster() {
	// Generate a random seed at the recommended length.
	seed, err := hdkeychain.GenerateSeed(hdkeychain.RecommendedSeedLen)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Generate a new master node using the seed.
	key, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Show that the generated master node extended key is private.
	fmt.Println("Private Extended Key?:", key.IsPrivate())

	// Output:
	// Private Extended Key?: true
}

// This example demonstrates the default hierarchical deterministic wallet
// layout as described in BIP0032.
func Example_defaultWalletLayout() {
	// The default wallet layout described in BIP0032 is:
	//
	// Each account is composed of two keypair chains: an internal and an
	// external one. The external keychain is used to generate new public
	// addresses, while the internal keychain is used for all other
	// operations (change addresses, generation addresses, ..., anything
	// that doesn't need to be communicated).
	//
	//   * m/iH/0/k
	//     corresponds to the k'th keypair of the external chain of account
	//     number i of the HDW derived from master m.
	//   * m/iH/1/k
	//     corresponds to the k'th keypair of the internal chain of account
	//     number i of the HDW derived from master m.

	// Ordinarily this would either be read from some encrypted source
	// and be decrypted or generated as the NewMaster example shows, but
	// for the purposes of this example, the private extended key for the
	// master node is being hard coded here.
	master := "zprvAWgYBBk7JR8GjzqSzmunMCS7dAbwpYTCs1YUMDXqduMA5JFHZ3iX5s2UkAR6vBdcCYYa1S5o1fVLrKsrnpCQ4WpUd6aVUWP1bS2Yy5DoaKv"

	// Start by getting an extended key instance for the master node.
	// This gives the path:
	//   m
	masterKey, err := hdkeychain.NewKeyFromString(master)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Derive the extended key for account 0.  This gives the path:
	//   m/0H
	acct0, err := masterKey.Derive(hdkeychain.HardenedKeyStart + 0)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Derive the extended key for the account 0 external chain.  This
	// gives the path:
	//   m/0H/0
	acct0Ext, err := acct0.Derive(0)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Derive the extended key for the account 0 internal chain.  This gives
	// the path:
	//   m/0H/1
	acct0Int, err := acct0.Derive(1)
	if err != nil {
		fmt.Println(err)
		return
	}

	// At this point, acct0Ext and acct0Int are ready to derive the keys for
	// the external and internal wallet chains.

	// Derive the 10th extended key for the account 0 external chain.  This
	// gives the path:
	//   m/0H/0/10
	acct0Ext10, err := acct0Ext.Derive(10)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Derive the 1st extended key for the account 0 internal chain.  This
	// gives the path:
	//   m/0H/1/0
	acct0Int0, err := acct0Int.Derive(0)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Get and show the address associated with the extended keys for the
	// main Pearl network.
	acct0ExtAddr, err := TaprootAddress(acct0Ext10, &chaincfg.MainNetParams)
	if err != nil {
		fmt.Println(err)
		return
	}
	acct0IntAddr, err := TaprootAddress(acct0Int0, &chaincfg.MainNetParams)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("Account 0 External Address 10:", acct0ExtAddr)
	fmt.Println("Account 0 Internal Address 0:", acct0IntAddr)

	// Output:
	// Account 0 External Address 10: prl1p48eeyql92n6vy2xfwqsnl5w92vz3x35qcvlqdr8x43rcm9nxg5kqwkcp30
	// Account 0 Internal Address 0: prl1prrw760xke47jvnz30yc27pn7npkghquxrsj93xze84v5v8m9sdksfnsh6e
}

// This example demonstrates the audits use case in BIP0032.
func Example_audits() {
	// The audits use case described in BIP0032 is:
	//
	// In case an auditor needs full access to the list of incoming and
	// outgoing payments, one can share all account public extended keys.
	// This will allow the auditor to see all transactions from and to the
	// wallet, in all accounts, but not a single secret key.
	//
	//   * N(m/*)
	//   corresponds to the neutered master extended key (also called
	//   the master public extended key)

	// Ordinarily this would either be read from some encrypted source
	// and be decrypted or generated as the NewMaster example shows, but
	// for the purposes of this example, the private extended key for the
	// master node is being hard coded here.
	master := "zprvAWgYBBk7JR8GjzqSzmunMCS7dAbwpYTCs1YUMDXqduMA5JFHZ3iX5s2UkAR6vBdcCYYa1S5o1fVLrKsrnpCQ4WpUd6aVUWP1bS2Yy5DoaKv"

	// Start by getting an extended key instance for the master node.
	// This gives the path:
	//   m
	masterKey, err := hdkeychain.NewKeyFromString(master)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Neuter the master key to generate a master public extended key.  This
	// gives the path:
	//   N(m/*)
	masterPubKey, err := masterKey.Neuter()
	if err != nil {
		fmt.Println(err)
		return
	}

	// Share the master public extended key with the auditor.
	fmt.Println("Audit key N(m/*):", masterPubKey)

	// Output:
	// Audit key N(m/*): zpub6jftahH18ngZxUuv6oSniLNrBCSSE1B4EEU59bwTCEt8x6aS6b2mdfLxbS4QS53g85SWWP6wexqeer516433gYpZQoJie2tcMYdJ1SYYYAL
}
