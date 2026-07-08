/**
 * Central network configuration for the wallet
 * All network-specific settings should be consumed from here
 */

import { MAINNET_DEFAULT_PEER_ADDRESSES, TESTNET_DEFAULT_PEER_ADDRESSES } from "./consts";

export type Network = 'mainnet' | 'testnet';

export interface NetworkConfig {
    name: Network;
    displayName: string;
    rpcPort: number;
    walletFlag: string;
    dataSubdir: string;
    addressPrefix: string;
    defaultPeerAddress: string;
    defaultPeerPort: number;
}

const mainnetNodeIndex = Math.floor(Math.random() * MAINNET_DEFAULT_PEER_ADDRESSES.length);
const testnetNodeIndex = Math.floor(Math.random() * TESTNET_DEFAULT_PEER_ADDRESSES.length);

const mainnetDefaultPeerAddress = MAINNET_DEFAULT_PEER_ADDRESSES[mainnetNodeIndex];
const testnetDefaultPeerAddress = TESTNET_DEFAULT_PEER_ADDRESSES[testnetNodeIndex];

const NETWORK_CONFIGS: Record<Network, NetworkConfig> = {
    mainnet: {
        name: 'mainnet',
        displayName: 'Mainnet',
        // modelOS-specific wallet RPC port (was 8335). 8335 collides with the Pearl
        // desktop wallet, so opening modelOS while Pearl is running (or vice versa)
        // failed to bind. 8888 keeps the two apps' bundled wallets independent.
        rpcPort: 8888,
        walletFlag: '',  // No flag for mainnet (default)
        dataSubdir: 'mainnet',
        addressPrefix: 'mdl1',  // Taproot addresses: mdl1p...
        defaultPeerAddress: mainnetDefaultPeerAddress,
        defaultPeerPort: 44208,
    },
    testnet: {
        name: 'testnet',
        displayName: 'Testnet',
        // Distinct from mainnet (8888) so mainnet + testnet can run side by side.
        rpcPort: 8889,
        walletFlag: '--testnet2',
        dataSubdir: 'testnet2',
        addressPrefix: 'tmdl1',  // Testnet Taproot: tmdl1p...
        defaultPeerAddress: testnetDefaultPeerAddress,
        defaultPeerPort: 44210,
    },
};

let currentNetwork: Network = 'mainnet';

export function getCurrentNetwork(): Network {
    return currentNetwork;
}

export function setCurrentNetwork(network: Network): void {
    currentNetwork = network;
}

export function getCurrentNetworkConfig(): NetworkConfig {
    return NETWORK_CONFIGS[currentNetwork];
}

export function getAllNetworks(): Network[] {
    return Object.keys(NETWORK_CONFIGS) as Network[];
}
