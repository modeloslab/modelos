import { wordlists } from 'bip39';

// Word counts accepted by BIP39 (entropy 128..256 bits in 32-bit steps). The modelOS node
// accepts any of these via `github.com/tyler-smith/go-bip39`, so we mirror that here to
// catch mistakes (e.g. pasted 11 or 23 words) before we hand the seed to the node.
export const VALID_MNEMONIC_WORD_COUNTS = [12, 15, 18, 21, 24] as const;

// The Go node's `walletsetup.go` validates hex seeds against hdkeychain MinSeedBytes (16)
// and MaxSeedBytes (64). Keeping the numbers here rather than importing them keeps the
// renderer free of Node/Electron deps and mirrors the current node contract.
const HEX_MIN_BYTES = 16;
const HEX_MAX_BYTES = 64;

export type ParsedSeedInput =
  | { kind: 'mnemonic'; normalized: string; wordCount: number }
  | { kind: 'hex'; normalized: string }
  | { kind: 'invalid'; reason: string };

// Collapses any whitespace run (spaces, tabs, newlines) to a single space and lowercases
// the mnemonic. BIP39 English words are all lowercase ASCII so this is lossless for valid
// input and cleans up the common paste artefacts that trip the node's strict validator.
export function normalizeMnemonicInput(raw: string): string {
  return raw.trim().toLowerCase().replace(/\s+/g, ' ');
}

export function normalizeHexInput(raw: string): string {
  return raw.trim().toLowerCase().replace(/^0x/, '');
}

// We validate the BIP39 checksum ourselves instead of calling `bip39.validateMnemonic`
// because that function internally invokes `Buffer.from(...)` (line 91 of bip39's
// `src/index.js`). The Electron renderer runs without Node integration and Vite does not
// polyfill the `Buffer` global, so `Buffer.from` throws `ReferenceError` at runtime which
// the library catches and reports as `INVALID_CHECKSUM` — meaning every mnemonic, valid or
// not, is rejected. Web Crypto's `subtle.digest('SHA-256', ...)` is a standard browser API
// available in the renderer, returns a `Uint8Array`, and has no Node dependency.
async function isBip39ChecksumValid(words: string[]): Promise<boolean> {
  const english = wordlists.english;
  const indices: number[] = [];
  for (const word of words) {
    const idx = english.indexOf(word);
    if (idx < 0) return false;
    indices.push(idx);
  }

  let bits = '';
  for (const idx of indices) {
    bits += idx.toString(2).padStart(11, '0');
  }

  // Per BIP39: total bits = ENT + CS where ENT is a multiple of 32 and CS = ENT / 32.
  // Equivalently, bits.length = 33 * k for k in {4,5,6,7,8}.
  const dividerIndex = Math.floor(bits.length / 33) * 32;
  const entropyBits = bits.slice(0, dividerIndex);
  const checksumBits = bits.slice(dividerIndex);

  const entropyBytes = new Uint8Array(entropyBits.length / 8);
  for (let i = 0; i < entropyBytes.length; i++) {
    entropyBytes[i] = parseInt(entropyBits.slice(i * 8, (i + 1) * 8), 2);
  }

  const digest = await crypto.subtle.digest('SHA-256', entropyBytes);
  const hashBytes = new Uint8Array(digest);
  let hashBits = '';
  for (const byte of hashBytes) {
    hashBits += byte.toString(2).padStart(8, '0');
  }

  return hashBits.slice(0, checksumBits.length) === checksumBits;
}

export async function parseSeedInput(raw: string): Promise<ParsedSeedInput> {
  const trimmed = raw.trim();
  if (!trimmed) {
    return { kind: 'invalid', reason: 'Please enter your seed phrase' };
  }

  // If the input contains whitespace it's unambiguously a mnemonic attempt. Route
  // single-token input through the hex check first and fall through to mnemonic.
  const looksLikeSingleToken = !/\s/.test(trimmed);
  if (looksLikeSingleToken) {
    const hex = normalizeHexInput(trimmed);
    if (/^[0-9a-f]+$/.test(hex)) {
      if (hex.length % 2 !== 0) {
        return { kind: 'invalid', reason: 'Hex seed must have an even number of characters' };
      }
      const byteLen = hex.length / 2;
      if (byteLen < HEX_MIN_BYTES || byteLen > HEX_MAX_BYTES) {
        return {
          kind: 'invalid',
          reason: `Hex seed must be between ${HEX_MIN_BYTES * 8} and ${HEX_MAX_BYTES * 8} bits`,
        };
      }
      return { kind: 'hex', normalized: hex };
    }
    return {
      kind: 'invalid',
      reason: 'Enter a BIP39 mnemonic (12, 15, 18, 21, or 24 words) or a hex seed',
    };
  }

  const normalized = normalizeMnemonicInput(trimmed);
  const words = normalized.split(' ');
  const wordCount = words.length;

  if (!VALID_MNEMONIC_WORD_COUNTS.includes(wordCount as (typeof VALID_MNEMONIC_WORD_COUNTS)[number])) {
    return {
      kind: 'invalid',
      reason: `Mnemonic must be 12, 15, 18, 21, or 24 words (got ${wordCount})`,
    };
  }

  const english = wordlists.english;
  const unknownWord = words.find(word => !english.includes(word));
  if (unknownWord) {
    return {
      kind: 'invalid',
      reason: `"${unknownWord}" is not a BIP39 word. Double-check spelling.`,
    };
  }

  const checksumOk = await isBip39ChecksumValid(words);
  if (!checksumOk) {
    return {
      kind: 'invalid',
      reason: 'Mnemonic checksum is invalid. Double-check the word order and spelling.',
    };
  }

  return { kind: 'mnemonic', normalized, wordCount };
}
