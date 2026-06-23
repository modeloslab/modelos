export const MAINNET_DEFAULT_PEER_ADDRESSES = ['node1.modeloslab.xyz'];
export const TESTNET_DEFAULT_PEER_ADDRESSES = ['testnet-seeder1.modeloslab.xyz', 'testnet-seeder2.modeloslab.xyz'];

export const UPDATE_REPO_OWNER = 'modelos';
export const UPDATE_REPO_NAME = 'modelos';
export const UPDATE_RELEASE_TAG_PREFIX = 'modelos-wallet-v';

/** 1 MDL = 100,000,000 grains (smallest unit, like Bitcoin satoshis). */
export const GRAINS_PER_MDL = 100_000_000;

/** Minimum inference_tx fee: 0.01 MDL = 1,000,000 grains. */
export const MIN_INFERENCE_FEE_GRAINS = 1_000_000;

/**
 * Base fee tiers (priority component) for inference_tx, in grains.
 * Final fee = base_fee + GRAINS_PER_TOKEN × max_tokens.
 * Higher base = higher priority when multiple requests compete.
 */
export const INFERENCE_FEE_TIERS = {
  slow:     MIN_INFERENCE_FEE_GRAINS,       // 0.01 MDL base
  standard: MIN_INFERENCE_FEE_GRAINS * 5,   // 0.05 MDL base
  fast:     MIN_INFERENCE_FEE_GRAINS * 20,  // 0.20 MDL base
} as const;

/**
 * Per-output-token fee component: 2,000 grains = 0.00002 MDL per token.
 * Compensates miners for longer inference jobs.
 *
 * Example total fees:
 *   standard + 512  tokens = 0.05 + 0.010 = 0.060 MDL
 *   standard + 1024 tokens = 0.05 + 0.020 = 0.070 MDL
 *   fast     + 4096 tokens = 0.20 + 0.082 = 0.282 MDL
 */
export const GRAINS_PER_TOKEN = 2_000;
