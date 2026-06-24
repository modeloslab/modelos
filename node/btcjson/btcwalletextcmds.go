// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC commands that are supported by
// a wallet server with Oyster extensions.

package btcjson

// CreateNewAccountCmd defines the createnewaccount JSON-RPC command.
type CreateNewAccountCmd struct {
	Account string
}

// NewCreateNewAccountCmd returns a new instance which can be used to issue a
// createnewaccount JSON-RPC command.
func NewCreateNewAccountCmd(account string) *CreateNewAccountCmd {
	return &CreateNewAccountCmd{
		Account: account,
	}
}

// DumpWalletCmd defines the dumpwallet JSON-RPC command.
type DumpWalletCmd struct {
	Filename string
}

// NewDumpWalletCmd returns a new instance which can be used to issue a
// dumpwallet JSON-RPC command.
func NewDumpWalletCmd(filename string) *DumpWalletCmd {
	return &DumpWalletCmd{
		Filename: filename,
	}
}

// ImportAddressCmd defines the importaddress JSON-RPC command.
type ImportAddressCmd struct {
	Address string
	Account string
	Rescan  *bool `jsonrpcdefault:"true"`
}

// NewImportAddressCmd returns a new instance which can be used to issue an
// importaddress JSON-RPC command.
func NewImportAddressCmd(address string, account string, rescan *bool) *ImportAddressCmd {
	return &ImportAddressCmd{
		Address: address,
		Account: account,
		Rescan:  rescan,
	}
}

// ImportPubKeyCmd defines the importpubkey JSON-RPC command.
type ImportPubKeyCmd struct {
	PubKey string
	Rescan *bool `jsonrpcdefault:"true"`
}

// NewImportPubKeyCmd returns a new instance which can be used to issue an
// importpubkey JSON-RPC command.
func NewImportPubKeyCmd(pubKey string, rescan *bool) *ImportPubKeyCmd {
	return &ImportPubKeyCmd{
		PubKey: pubKey,
		Rescan: rescan,
	}
}

// ImportWalletCmd defines the importwallet JSON-RPC command.
type ImportWalletCmd struct {
	Filename string
}

// NewImportWalletCmd returns a new instance which can be used to issue a
// importwallet JSON-RPC command.
func NewImportWalletCmd(filename string) *ImportWalletCmd {
	return &ImportWalletCmd{
		Filename: filename,
	}
}

// RenameAccountCmd defines the renameaccount JSON-RPC command.
type RenameAccountCmd struct {
	OldAccount string
	NewAccount string
}

// NewRenameAccountCmd returns a new instance which can be used to issue a
// renameaccount JSON-RPC command.
func NewRenameAccountCmd(oldAccount, newAccount string) *RenameAccountCmd {
	return &RenameAccountCmd{
		OldAccount: oldAccount,
		NewAccount: newAccount,
	}
}

// SendInferenceTxCmd defines the sendinferencetx JSON-RPC command.
// It constructs and broadcasts a version-3 inference_tx to the modelOS network.
type SendInferenceTxCmd struct {
	// PromptHash is the hex-encoded 32-byte BLAKE3 hash of the off-chain prompt.
	PromptHash string
	// ResultAddress is the relay URL (e.g. "http://1.2.3.4:44211") where
	// miners POST the inference result. Maximum 64 bytes UTF-8.
	ResultAddress string
	// FeeGrains is the inference fee in grains (minimum 1 000 000 = 0.01 MDL).
	FeeGrains int64
	// MaxTokens is the maximum number of tokens the miner should generate
	// (consensus maximum 65535 = the uint16 format ceiling; the practical
	// per-model limit is enforced by the compute app + worker context window).
	MaxTokens int64
	// ModelVersion identifies the requested LLM (1 = DeepSeek R1 70B, default).
	ModelVersion *int64 `jsonrpcdefault:"1"`
}

// NewSendInferenceTxCmd returns a new instance which can be used to issue a
// sendinferencetx JSON-RPC command.
func NewSendInferenceTxCmd(promptHash, resultAddress string, feeGrains, maxTokens int64, modelVersion *int64) *SendInferenceTxCmd {
	return &SendInferenceTxCmd{
		PromptHash:    promptHash,
		ResultAddress: resultAddress,
		FeeGrains:     feeGrains,
		MaxTokens:     maxTokens,
		ModelVersion:  modelVersion,
	}
}

// SendInferenceProofCmd defines the sendinferenceproof JSON-RPC command.
// It constructs and broadcasts a version-4 inference_proof_tx to the modelOS
// network, spending the locked fee output of an earlier inference_tx and
// paying it to the worker who produced the inference result.
type SendInferenceProofCmd struct {
	// InferenceTxid is the hex-encoded txid of the version-3 inference_tx
	// whose fee output (TxOut[0]) is being claimed.
	InferenceTxid string
	// ProofScript is the hex-encoded 197-byte OP_RETURN script carrying the
	// serialised InferenceResultProof: OP_RETURN OP_PUSHDATA1 194 <proof>.
	ProofScript string
	// WorkerScript is the hex-encoded 34-byte P2TR scriptPubKey of the
	// inference worker claiming the locked fee.
	WorkerScript string
	// ResultText is the plaintext inference result the miner delivered to the
	// wallet relay.  When non-empty the wallet verifies
	// SHA3-256(ResultText) == ProofScript.ResultHash before signing TxOut[0].
	// If they differ, payment is refused — the miner committed to a different
	// result than they delivered.
	ResultText *string
}

// NewSendInferenceProofCmd returns a new instance which can be used to issue a
// sendinferenceproof JSON-RPC command.
func NewSendInferenceProofCmd(inferenceTxid, proofScript, workerScript string) *SendInferenceProofCmd {
	return &SendInferenceProofCmd{
		InferenceTxid: inferenceTxid,
		ProofScript:   proofScript,
		WorkerScript:  workerScript,
	}
}

func init() {
	// The commands in this file are only usable with a wallet server.
	flags := UFWalletOnly

	MustRegisterCmd("createnewaccount", (*CreateNewAccountCmd)(nil), flags)
	MustRegisterCmd("dumpwallet", (*DumpWalletCmd)(nil), flags)
	MustRegisterCmd("importaddress", (*ImportAddressCmd)(nil), flags)
	MustRegisterCmd("importpubkey", (*ImportPubKeyCmd)(nil), flags)
	MustRegisterCmd("importwallet", (*ImportWalletCmd)(nil), flags)
	MustRegisterCmd("renameaccount", (*RenameAccountCmd)(nil), flags)
	MustRegisterCmd("sendinferencetx", (*SendInferenceTxCmd)(nil), flags)
	MustRegisterCmd("sendinferenceproof", (*SendInferenceProofCmd)(nil), flags)
}
