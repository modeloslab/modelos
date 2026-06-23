// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	bip39 "github.com/tyler-smith/go-bip39"

	"github.com/modelos/modelos/node/btcutil/hdkeychain"
	"github.com/modelos/modelos/node/chaincfg"
	"github.com/modelos/modelos/wallet/internal/prompt"
	"github.com/modelos/modelos/wallet/wallet"
	"github.com/modelos/modelos/wallet/walletdb"
	_ "github.com/modelos/modelos/wallet/walletdb/bdb"
)

// networkDir returns the directory name of a network directory to hold wallet
// files.
func networkDir(dataDir string, chainParams *chaincfg.Params) string {
	return filepath.Join(dataDir, chainParams.Name)
}

// createWallet prompts the user for information needed to generate a new wallet
// and generates the wallet accordingly.  The new wallet will reside at the
// provided path.
func createWallet(cfg *config) error {
	dbDir := networkDir(cfg.AppDataDir.Value, activeNet.Params)
	loader := wallet.NewLoader(
		activeNet.Params, dbDir, true, cfg.DBTimeout, 250,
	)

	// Start by prompting for the private passphrase
	reader := bufio.NewReader(os.Stdin)
	privPass, err := prompt.PrivatePass(reader)
	if err != nil {
		return err
	}

	// Ascertain the public passphrase.  This will either be a value
	// specified by the user or the default hard-coded public passphrase if
	// the user does not want the additional public data encryption.
	pubPass, err := prompt.PublicPass(reader, privPass,
		[]byte(wallet.InsecurePubPassphrase), []byte(cfg.WalletPass))
	if err != nil {
		return err
	}

	// Ascertain the wallet generation seed.  This will either be an
	// automatically generated value the user has already confirmed or a
	// value the user has entered which has already been validated.
	seed, err := prompt.Seed(reader)
	if err != nil {
		return err
	}

	fmt.Println("Creating the wallet...")
	w, err := loader.CreateNewWallet(pubPass, privPass, seed, time.Now())
	if err != nil {
		return err
	}

	w.Manager.Close()
	fmt.Println("The wallet has been created successfully.")
	return nil
}

// createSimulationWallet is intended to be called from the rpcclient
// and used to create a wallet for actors involved in simulations.
func createSimulationWallet(cfg *config) error {
	// Simulation wallet password is 'password'.
	privPass := []byte("password")

	// Public passphrase is the default.
	pubPass := []byte(wallet.InsecurePubPassphrase)

	netDir := networkDir(cfg.AppDataDir.Value, activeNet.Params)

	// Create the wallet.
	dbPath := filepath.Join(netDir, wallet.WalletDBName)
	fmt.Println("Creating the wallet...")

	// Create the wallet database backed by bolt db.
	db, err := walletdb.Create("bdb", dbPath, true, cfg.DBTimeout, false)
	if err != nil {
		return err
	}
	defer db.Close()

	// Create the wallet.
	err = wallet.Create(db, pubPass, privPass, nil, activeNet.Params, time.Now())
	if err != nil {
		return err
	}

	fmt.Println("The wallet has been created successfully.")
	return nil
}

// createWalletFromJSON reads a JSON blob with fields:
//
//	{
//	  PrivatePassphrase: string (required)
//	  PublicPassphrase: string (optional)
//	  Seed: string (hex, optional)
//	  Bday: string (unix seconds, optional)
//	}
//
// and creates a wallet accordingly. Returns the hex-encoded seed used.
func createWalletFromJSON(cfg *config, data []byte) (string, error) {
	var input struct {
		PrivatePassphrase string  `json:"PrivatePassphrase"`
		PublicPassphrase  *string `json:"PublicPassphrase"`
		Seed              *string `json:"Seed"`
		Bday              *string `json:"Bday"`
	}
	if err := json.Unmarshal(data, &input); err != nil {
		return "", fmt.Errorf("invalid JSON in createfromfile: %w", err)
	}

	dbDir := networkDir(cfg.AppDataDir.Value, activeNet.Params)
	loader := wallet.NewLoader(
		activeNet.Params, dbDir, true, cfg.DBTimeout, 250,
	)

	// Validate required fields.
	if strings.TrimSpace(input.PrivatePassphrase) == "" {
		return "", fmt.Errorf("PrivatePassphrase is required in createfromfile input")
	}

	privPass := []byte(input.PrivatePassphrase)

	pubPass := []byte(wallet.InsecurePubPassphrase)
	if input.PublicPassphrase != nil && strings.TrimSpace(*input.PublicPassphrase) != "" {
		pubPass = []byte(*input.PublicPassphrase)
	}

	// Decode optional seed (mnemonic or hex) if provided.
	var seedStr string
	if input.Seed != nil {
		seedStr = strings.TrimSpace(*input.Seed)
	}
	var seed []byte
	if seedStr == "" {
		// Generate a new 16-byte (128-bit) entropy → 12-word BIP39 mnemonic.
		// Use PBKDF2 (standard BIP39) to derive the 64-byte BIP32 master seed.
		gen, err := hdkeychain.GenerateSeed(hdkeychain.RecommendedSeedLen)
		if err != nil {
			return "", fmt.Errorf("failed to generate seed: %w", err)
		}
		mnemonic, err := bip39.NewMnemonic(gen)
		if err != nil {
			return "", fmt.Errorf("failed to generate mnemonic: %w", err)
		}
		seedStr = mnemonic
		seed = bip39.NewSeed(mnemonic, "")
	} else if bip39.IsMnemonicValid(seedStr) {
		// Input is a BIP39 mnemonic phrase — derive the 64-byte BIP32 seed via PBKDF2.
		// This matches the standard BIP39 derivation.
		seed = bip39.NewSeed(seedStr, "")
	} else {
		// Fall back to hex for backward compatibility.
		dec, err := hex.DecodeString(strings.ToLower(seedStr))
		if err != nil || len(dec) < hdkeychain.MinSeedBytes || len(dec) > hdkeychain.MaxSeedBytes {
			return "", fmt.Errorf("invalid seed: must be a BIP39 mnemonic or hex string (%d-%d bits)", hdkeychain.MinSeedBytes*8, hdkeychain.MaxSeedBytes*8)
		}
		seed = dec
	}

	// Parse optional birthday (unix seconds) if provided; default now.
	birthday := time.Now()
	if input.Bday != nil && strings.TrimSpace(*input.Bday) != "" {
		secs, err := strconv.ParseInt(strings.TrimSpace(*input.Bday), 10, 64)
		if err != nil {
			return "", fmt.Errorf("Bday must be unix timestamp in seconds")
		}
		birthday = time.Unix(secs, 0)
	}

	// Create the wallet via loader.
	if _, err := loader.CreateNewWallet(pubPass, privPass, seed, birthday); err != nil {
		return "", fmt.Errorf("unable to create wallet: %w", err)
	}

	return seedStr, nil
}

// checkCreateDir checks that the path exists and is a directory.
// If path does not exist, it is created.
func checkCreateDir(path string) error {
	if fi, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			// Attempt data directory creation
			if err = os.MkdirAll(path, 0700); err != nil {
				return fmt.Errorf("cannot create directory: %w", err)
			}
		} else {
			return fmt.Errorf("error checking directory: %w", err)
		}
	} else {
		if !fi.IsDir() {
			return fmt.Errorf("path '%s' is not a directory", path)
		}
	}

	return nil
}
