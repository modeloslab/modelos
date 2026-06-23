/**
 * Renderer-side constants. Values must stay in sync with
 * src/main/config/consts.ts — the main process and renderer
 * are separate Vite builds and cannot share the same module.
 */

/** 1 MDL = 100,000,000 grains (smallest unit). */
export const GRAINS_PER_MDL = 100_000_000;

/** Minimum inference_tx fee: 0.01 MDL. */
export const MIN_INFERENCE_FEE_GRAINS = 1_000_000;

/**
 * Base priority fee tiers for inference_tx (in grains).
 * Final fee = base + GRAINS_PER_TOKEN × max_tokens.
 */
export const INFERENCE_FEE_TIERS = {
  slow:     MIN_INFERENCE_FEE_GRAINS,       // 0.01 MDL base
  standard: MIN_INFERENCE_FEE_GRAINS * 5,   // 0.05 MDL base
  fast:     MIN_INFERENCE_FEE_GRAINS * 20,  // 0.20 MDL base
} as const;

/**
 * Legacy flat per-token constant — superseded by per-model pricing below.
 * Kept for backward compatibility; UI now uses INFERENCE_MODELS[n].perTokenGrains.
 */
export const GRAINS_PER_TOKEN = 2_000;

/**
 * Available inference model versions with per-model pricing.
 * Must stay in sync with:
 *   - wire/msginferencetx.go  (ModelVersion constants)
 *   - vllm-miner/inference_tx.py (MODEL_FEES)
 *
 * Fee = baseGrains + perTokenGrains × max_tokens
 *
 * Larger models cost more: more GPU-hours per token, higher quality output.
 * Smaller models are cheaper and served by more miners (faster response).
 */
export const INFERENCE_MODELS = [
  {
    version:        1,
    name:           'DeepSeek R1 70B',
    hfId:           'deepseek-ai/DeepSeek-R1-Distill-Llama-70B',
    vram:           '~140GB (2 GPUs)',
    gpuClass:       '2× A100 80GB',
    quality:        'Highest',
    availability:   'Lower — fewer large-GPU miners',
    baseGrains:     10_000_000,   // 0.10 MDL base
    perTokenGrains: 5_000,        // 0.00005 MDL/token
  },
  {
    version:        2,
    name:           'Qwen3 32B',
    hfId:           'Qwen/Qwen3-32B',
    vram:           '~64GB',
    gpuClass:       'A100 80GB',
    quality:        'Very High',
    availability:   'Good',
    baseGrains:     5_000_000,    // 0.05 MDL base
    perTokenGrains: 2_000,        // 0.00002 MDL/token
  },
  {
    version:        3,
    name:           'Qwen3 14B',
    hfId:           'Qwen/Qwen3-14B',
    vram:           '~28GB',
    gpuClass:       'A100 / RTX 3090',
    quality:        'High',
    availability:   'High',
    baseGrains:     2_000_000,    // 0.02 MDL base
    perTokenGrains: 1_000,        // 0.00001 MDL/token
  },
  {
    version:        4,
    name:           'DeepSeek R1 0528 (Qwen3 8B)',
    hfId:           'deepseek-ai/DeepSeek-R1-0528-Qwen3-8B',
    vram:           '~16GB',
    gpuClass:       'RTX 4080 / 3090+',
    quality:        'Strong (SOTA 8B)',
    availability:   'Highest — most miners',
    baseGrains:     500_000,      // 0.005 MDL base
    perTokenGrains: 200,          // 0.000002 MDL/token
  },
] as const;
