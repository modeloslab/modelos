/**
 * Peer settings management - allows users to configure custom peer addresses per network
 */
import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';
import { getCurrentNetwork, getCurrentNetworkConfig } from './network-config';
import { MAINNET_DEFAULT_PEER_ADDRESSES, TESTNET_DEFAULT_PEER_ADDRESSES } from './consts';

interface NetworkPeerSettings {
  customPeerAddress?: string;
  customPeerPort?: number;
}

interface AllNetworksPeerSettings {
  mainnet: NetworkPeerSettings;
  testnet: NetworkPeerSettings;
}

const SETTINGS_DIR = path.join(os.homedir(), '.modelos-wallet', 'settings');
const SETTINGS_FILE = path.join(SETTINGS_DIR, 'peer-settings.json');

// Ensure settings directory exists
function ensureSettingsDir() {
  if (!fs.existsSync(SETTINGS_DIR)) {
    fs.mkdirSync(SETTINGS_DIR, { recursive: true });
  }
}

function loadAllNetworksPeerSettings(): AllNetworksPeerSettings {
  ensureSettingsDir();

  if (fs.existsSync(SETTINGS_FILE)) {
    try {
      const data = fs.readFileSync(SETTINGS_FILE, 'utf-8');
      const parsed = JSON.parse(data);
      return {
        mainnet: parsed.mainnet ?? {},
        testnet: parsed.testnet ?? {},
      };
    } catch (error) {
      console.error('Failed to load peer settings:', error);
    }
  }

  return { mainnet: {}, testnet: {} };
}

// Load peer settings for the current network
function loadPeerSettings(): NetworkPeerSettings {
  const peerSettings = loadAllNetworksPeerSettings();
  return peerSettings[getCurrentNetwork()] ?? {};
}

// Save peer settings for the current network
function savePeerSettings(settings: NetworkPeerSettings) {
  ensureSettingsDir();

  const peerSettings = loadAllNetworksPeerSettings();
  peerSettings[getCurrentNetwork()] = settings;

  try {
    fs.writeFileSync(SETTINGS_FILE, JSON.stringify(peerSettings, null, 2), 'utf-8');
  } catch (error) {
    console.error('Failed to save peer settings:', error);
  }
}

// Get current peer address (custom or default) for active network
export function getPeerAddress(): string {
  const settings = loadPeerSettings();
  if (settings.customPeerAddress) {
    return settings.customPeerAddress;
  }

  const networkConfig = getCurrentNetworkConfig();
  return networkConfig.defaultPeerAddress;
}

// Get current peer port (custom or default) for active network
export function getPeerPort(): number {
  const settings = loadPeerSettings();
  if (settings.customPeerPort) {
    return settings.customPeerPort;
  }

  const networkConfig = getCurrentNetworkConfig();
  return networkConfig.defaultPeerPort;
}

// Set custom peer address for the active network only
export function setCustomPeer(address: string, port: number) {
  const settings = loadPeerSettings();
  settings.customPeerAddress = address;
  settings.customPeerPort = port;
  savePeerSettings(settings);
}

// Reset to default peer for the active network only
export function resetToDefaultPeer() {
  const settings = loadPeerSettings();
  delete settings.customPeerAddress;
  delete settings.customPeerPort;
  savePeerSettings(settings);
}

// Get peer settings info for the active network
export function getPeerSettings() {
  const network = getCurrentNetwork();
  const networkConfig = getCurrentNetworkConfig();
  const settings = loadPeerSettings();

  const defaultAddresses =
    network === 'mainnet' ? MAINNET_DEFAULT_PEER_ADDRESSES : TESTNET_DEFAULT_PEER_ADDRESSES;
  const isDefaultFoundationNode = defaultAddresses.includes(settings.customPeerAddress ?? '');

  return {
    network,
    currentAddress: getPeerAddress(),
    currentPort: getPeerPort(),
    defaultAddress: networkConfig.defaultPeerAddress,
    defaultPort: networkConfig.defaultPeerPort,
    isCustom: (settings.customPeerAddress && !isDefaultFoundationNode) || false,
  };
}
