import { ipcMain } from 'electron';
import { ManagerService } from '../services/manager-service';
import { BlockbookClient } from '../clients/blockbook-client';
import { inferenceRelay } from '../services/inference-relay-service';

function registerWalletIpc(ms: ManagerService) {
  ipcMain.handle('wallet-unlock', (_event, passphrase: string, timeout: number = 60) =>
    ms.ensureWalletService().unlockWallet(passphrase, timeout)
  );
  ipcMain.handle('wallet-lock', _event => ms.lockWallet());
  ipcMain.handle('wallet-force-lock', _event => ms.forceLockWallet());
  ipcMain.handle('wallet-change-password', (_event, currentPassword: string, newPassword: string) =>
    ms.ensureWalletService().changeWalletPassphrase(currentPassword, newPassword)
  );
  ipcMain.handle('wallet-send-from-default-account', (_event, toAddress: string, amount: number, feeRate: number) =>
    ms.ensureWalletService().sendFromDefaultAccount(toAddress, amount, feeRate)
  );
  ipcMain.handle('wallet-list-all-transactions', _event =>
    ms.ensureWalletService().listAllTransactions()
  );
  ipcMain.handle('wallet-list-transactions', (_event, count: number = 10, from: number = 0) =>
    ms.ensureWalletService().listTransactions(count, from)
  );
  ipcMain.handle('wallet-get-balance', (_event, account: string, minconf: number = 1) =>
    ms.ensureWalletService().getBalance(account, minconf)
  );
  ipcMain.handle('wallet-validate-address', (_event, address: string) =>
    ms.ensureWalletService().validateAddress(address)
  );
  ipcMain.handle('wallet-get-new-address', _event => ms.ensureWalletService().getNewAddress());
  ipcMain.handle('wallet-estimate-fee', (_event, numBlocks: number) =>
    BlockbookClient.estimateFee(numBlocks)
  );

  // ── Inference IPC ─────────────────────────────────────────────────────────
  //
  // The wallet IS the prompt server and result receiver — no pool involved.
  //
  // Flow:
  //   1. Wallet stores prompt locally, hashes it.
  //   2. inference_tx is built with result_address = wallet's relay URL
  //      (e.g. http://203.0.113.10:44211).
  //   3. Miners fetch GET /prompt/:hash from the wallet relay, race to answer.
  //   4. Winning miner POSTs result to wallet relay.
  //   5. Relay fires 'inference-result' IPC event — renderer updates chat live.
  //   6. No polling needed.

  ipcMain.handle('inference-submit', async (_event, params: {
    prompt: string;
    feeGrains: number;
    maxTokens: number;
    feeTier: string;
    modelVersion?: number;
  }) => {
    const walletService = ms.ensureWalletService();

    // Ensure relay is running before we store the prompt.
    inferenceRelay.start();

    // Store the prompt locally — returns its SHA3-256 hash.
    const promptHash = inferenceRelay.storePrompt(params.prompt);

    // Build the relay URL that goes into result_address on-chain.
    // Miners will fetch the prompt from and POST the result to this URL.
    const relayUrl = inferenceRelay.buildRelayUrl();

    // Submit the inference_tx via the wallet RPC.
    // result_address = relay URL (UTF-8, up to 64 bytes, null-padded on-chain).
    const txid = await walletService.sendInferenceTx({
      promptHash,
      resultAddress: relayUrl,
      feeGrains: params.feeGrains,
      maxTokens: params.maxTokens,
      modelVersion: params.modelVersion ?? 1,
    });

    return { txid, promptHash, relayUrl };
  });

  // Check if result has already been delivered (for reconnect / page reload).
  // Normal path: result arrives as 'inference-result' push event, not via this.
  ipcMain.handle('inference-get-result', (_event, params: { promptHash: string }) => {
    return inferenceRelay.getResult(params.promptHash);
  });

  // Allow renderer to check relay connectivity and get the relay URL.
  ipcMain.handle('inference-relay-info', () => ({
    relayUrl: inferenceRelay.buildRelayUrl(),
    port: 44211,
  }));

  // Live fee suggestions — fetched from pool API, fallback to protocol defaults.
  ipcMain.handle('inference-fee-stats', () =>
    BlockbookClient.getInferenceFeeStats()
  );

  // Per-model miner counts derived from confirmed inference_proof_tx wins (last 24h).
  // { "1": 12, "2": 31, "3": 47, "4": 89 } — keyed by model_version string.
  ipcMain.handle('inference-model-availability', () =>
    BlockbookClient.getModelAvailability()
  );
}

export { registerWalletIpc };
