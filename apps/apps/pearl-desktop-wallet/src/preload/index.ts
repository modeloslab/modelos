import { contextBridge, ipcRenderer } from 'electron';
import {
  AppBridge,
  Ipc,
  WindowApi,
  WalletApi,
  ManagerApi,
  SyncApi,
  UpdateApi,
  UpdateStatus,
} from '../types/app-bridge';

const windowIpc: Ipc<WindowApi> = {
  getVersion: () => ipcRenderer.invoke('app-version'),
  minimizeWindow: () => ipcRenderer.invoke('window-minimize'),
  maximizeWindow: () => ipcRenderer.invoke('window-maximize'),
  closeWindow: () => ipcRenderer.invoke('window-close'),
  isMaximized: () => ipcRenderer.invoke('window-is-maximized'),
  showMessageBox: options => ipcRenderer.invoke('show-message-box', options),
  showOpenDialog: options => ipcRenderer.invoke('show-open-dialog', options),
  showSaveDialog: options => ipcRenderer.invoke('show-save-dialog', options),
  openExternal: url => ipcRenderer.invoke('open-external', url),
};

const walletIpc: Ipc<WalletApi> = {
  getNewAddress: () => ipcRenderer.invoke('wallet-get-new-address'),
  unlockWallet: (passphrase, timeout) => ipcRenderer.invoke('wallet-unlock', passphrase, timeout),
  lockWallet: () => ipcRenderer.invoke('wallet-lock'),
  forceLockWallet: () => ipcRenderer.invoke('wallet-force-lock'),
  changeWalletPassphrase: (currentPassword, newPassword) =>
    ipcRenderer.invoke('wallet-change-password', currentPassword, newPassword),
  sendFromDefaultAccount: (toAddress: string, amount: number, feeRate: number) =>
    ipcRenderer.invoke('wallet-send-from-default-account', toAddress, amount, feeRate),
  listAllTransactions: () => ipcRenderer.invoke('wallet-list-all-transactions'),
  listTransactions: (count, from) => ipcRenderer.invoke('wallet-list-transactions', count, from),
  getBalance: (account, minconf) => ipcRenderer.invoke('wallet-get-balance', account, minconf),
  validateAddress: address => ipcRenderer.invoke('wallet-validate-address', address),
  estimateFee: numBlocks => ipcRenderer.invoke('wallet-estimate-fee', numBlocks),
};

const managerIpc: Ipc<ManagerApi> = {
  getWalletsStats: () => ipcRenderer.invoke('get-wallets-stats'),
  selectWallet: walletName => ipcRenderer.invoke('select-wallet', walletName),
  create: options => ipcRenderer.invoke('wallet-create', options),
  import: options => ipcRenderer.invoke('wallet-import', options),
  getExistingWallets: () => ipcRenderer.invoke('get-existing-wallets'),
  getNetworkInfo: () => ipcRenderer.invoke('get-network-info'),
  setNetwork: network => ipcRenderer.invoke('set-network', network),
  getPeerSettings: () => ipcRenderer.invoke('get-peer-settings'),
  validatePeerAddress: (address, port) => ipcRenderer.invoke('validate-peer', address, port),
  setCustomPeerAddress: (address, port) => ipcRenderer.invoke('set-custom-peer', address, port),
  resetPeerToDefault: () => ipcRenderer.invoke('reset-peer-to-default'),
};

const syncIpc: Ipc<SyncApi> = {
  getSyncProgress: () => ipcRenderer.invoke('sync-get-progress'),
  isSyncing: () => ipcRenderer.invoke('sync-is-syncing'),
  waitForSync: () => ipcRenderer.invoke('sync-wait-for-sync'),
};

const updateIpc: UpdateApi = {
  getStatus: () => ipcRenderer.invoke('update-get-status'),
  checkForUpdates: () => ipcRenderer.invoke('update-check'),
  openReleasePage: () => ipcRenderer.invoke('update-open-release-page'),
  onStatusChanged: listener => {
    const handler = (_event: unknown, status: UpdateStatus) => listener(status);
    ipcRenderer.on('update-status-changed', handler);
    return () => {
      ipcRenderer.removeListener('update-status-changed', handler);
    };
  },
};

// Inference IPC — exposed separately as window.api for the InferencePage.
//
// Results are PUSHED from the wallet's relay server when the winning miner
// delivers the answer.  No polling needed — just register a listener.
const inferenceApi = {
  submitInferenceTx: (params: {
    prompt: string;
    feeGrains: number;
    maxTokens: number;
    feeTier: string;
  }) => ipcRenderer.invoke('inference-submit', params),

  // Call this if the page reloads and you need to check if a result arrived.
  getInferenceResult: (params: { promptHash: string }) =>
    ipcRenderer.invoke('inference-get-result', params),

  // Register a callback — fires immediately when miner delivers the answer.
  // Returns an unsubscribe function.
  onInferenceResult: (
    callback: (data: { promptHash: string; result: string; status: string }) => void
  ): (() => void) => {
    const handler = (_event: unknown, data: { promptHash: string; result: string; status: string }) =>
      callback(data);
    ipcRenderer.on('inference-result', handler);
    return () => ipcRenderer.removeListener('inference-result', handler);
  },

  getRelayInfo: () => ipcRenderer.invoke('inference-relay-info'),

  // Live fee suggestions from pool — slow/median/fast in grains + MDL.
  getFeeStats: () => ipcRenderer.invoke('inference-fee-stats'),

  // Per-model miner availability — { "1": N, "2": N, ... } derived from
  // confirmed inference_proof_tx wins in the last 24h, no miner self-reporting.
  getModelAvailability: (): Promise<Record<string, number>> =>
    ipcRenderer.invoke('inference-model-availability'),
};

const appBridge: AppBridge = {
  window: windowIpc,
  wallet: walletIpc,
  manager: managerIpc,
  sync: syncIpc,
  update: updateIpc,
};

contextBridge.exposeInMainWorld('appBridge', appBridge);
contextBridge.exposeInMainWorld('api', inferenceApi);
