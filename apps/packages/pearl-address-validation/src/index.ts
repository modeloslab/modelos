import { base58_to_binary } from 'base58-js';
import { bech32, bech32m } from 'bech32';
import { createHash } from 'sha256-uint8array';

const sha256 = (payload: Uint8Array) => createHash().update(payload).digest();

enum Network {
  mainnet = 'mainnet',
  testnet = 'testnet',
  regtest = 'regtest',
  simnet = 'simnet',
}

enum AddressType {
  p2pkh = 'p2pkh',
  p2sh = 'p2sh',
  p2wpkh = 'p2wpkh',
  p2wsh = 'p2wsh',
  p2tr = 'p2tr',
}

type AddressInfo = {
  bech32: boolean;
  network: Network;
  address: string;
  type: AddressType;
};

const addressTypes: { [key: number]: { type: AddressType; network: Network } } = {
  0x00: {
    type: AddressType.p2pkh,
    network: Network.mainnet,
  },

  0x6f: {
    type: AddressType.p2pkh,
    network: Network.testnet,
  },

  0x05: {
    type: AddressType.p2sh,
    network: Network.mainnet,
  },

  0xc4: {
    type: AddressType.p2sh,
    network: Network.testnet,
  },
};

type Options = {
  castTestnetTo?: Network.regtest | Network.simnet;
};

function castTestnetTo(fromNetwork: Network, toNetwork?: Network.regtest | Network.simnet): Network {
  if (!toNetwork) {
    return fromNetwork;
  }

  if (fromNetwork === Network.mainnet) {
    throw new Error('Cannot cast mainnet to non-mainnet');
  }

  return toNetwork;
}

const normalizeAddressInfo = (addressInfo: AddressInfo, options?: Options): AddressInfo => {
  return {
    ...addressInfo,
    network: castTestnetTo(addressInfo.network, options?.castTestnetTo),
  };
};

const parseBech32 = (address: string, options?: Options): AddressInfo => {
  let decoded;

  const lowerAddress = address.toLowerCase();
  // modelOS uses Taproot-only addresses (witness v1, bech32m).
  // Valid prefixes: mdl1p (mainnet), tmdl1p (testnet), rmdl1p (regtest), smdl1p (simnet).
  if (
    !lowerAddress.startsWith('mdl1p') &&
    !lowerAddress.startsWith('tmdl1p') &&
    !lowerAddress.startsWith('rmdl1p') &&
    !lowerAddress.startsWith('smdl1p')
  ) {
    throw new Error('Invalid address');
  }

  try {
    // Taproot uses bech32m encoding
    decoded = bech32m.decode(address);
  } catch (error) {
    throw new Error('Invalid address');
  }

  const mapPrefixToNetwork: { [key: string]: Network } = {
    mdl:  Network.mainnet,
    tmdl: Network.testnet,
    rmdl: Network.regtest,
    smdl: Network.simnet,
  };

  const network: Network | undefined = mapPrefixToNetwork[decoded.prefix];

  if (network === undefined) {
    throw new Error('Invalid address');
  }

  const witnessVersion = decoded.words[0];

  // Only accept witness version 1 (Taproot)
  if (witnessVersion !== 1) {
    throw new Error('Only Taproot (witness v1) addresses are supported. Found witness version: ' + witnessVersion);
  }

  const data = bech32.fromWords(decoded.words.slice(1));

  // Taproot addresses must have 32-byte programs
  if (data.length !== 32) {
    throw new Error('Invalid Taproot address: witness program must be 32 bytes');
  }

  const type = AddressType.p2tr;

  return normalizeAddressInfo(
    {
      bech32: true,
      network,
      address,
      type,
    },
    options,
  );
};

const getAddressInfo = (address: string, options?: Options): AddressInfo => {
  let decoded: Uint8Array;

  const lowerAddress = address.toLowerCase();
  // Check if it's a bech32m address (starts with any valid modelOS network prefix).
  if (
    lowerAddress.startsWith('mdl1') ||
    lowerAddress.startsWith('tmdl1') ||
    lowerAddress.startsWith('rmdl1') ||
    lowerAddress.startsWith('smdl1')
  ) {
    try {
      return parseBech32(address, options);
    } catch (error) {
      throw new Error('Invalid address');
    }
  }

  try {
    decoded = base58_to_binary(address);
  } catch (error) {
    throw new Error('Invalid address');
  }

  const { length } = decoded;

  if (length !== 25) {
    throw new Error('Invalid address');
  }

  const version = decoded[0];

  const checksum = decoded.slice(length - 4, length);
  const body = decoded.slice(0, length - 4);

  const expectedChecksum = sha256(sha256(body)).slice(0, 4);

  if (checksum.some((value: number, index: number) => value !== expectedChecksum[index])) {
    throw new Error('Invalid address');
  }

  const validVersions = Object.keys(addressTypes).map(Number);

  if (version === undefined || !validVersions.includes(version)) {
    throw new Error('Invalid address');
  }

  const addressType = addressTypes[version];

  if (!addressType) {
    throw new Error('Invalid address');
  }

  return normalizeAddressInfo(
    {
      ...addressType,
      address,
      bech32: false,
    },
    options,
  );
};

const validate = (address: string, network?: Network, options?: Options) => {
  try {
    const addressInfo = getAddressInfo(address, options);

    if (network) {
      return network === addressInfo.network;
    }

    return true;
  } catch (error) {
    return false;
  }
};

export { getAddressInfo, Network, AddressType, validate };
export type { AddressInfo };
export default validate;
