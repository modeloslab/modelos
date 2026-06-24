import { getCurrentNetwork, type Network } from '../config/network-config';
import { MIN_INFERENCE_FEE_GRAINS, INFERENCE_FEE_TIERS } from '../config/consts';

// Blockbook is served by the explorer subdomain (deploy/nginx/conf.d/explorer.conf
// → explorer:9250). There is no separate blockbook.* host.
const BlockbookBaseUrlMap: Record<Network, string> = {
    testnet: 'https://explorer.testnet.modeloslab.xyz',
    mainnet: 'https://explorer.modeloslab.xyz',
};

// Pool API (dashboard) is served on standard HTTPS (443); nginx routes /inference/*
// to the pool dashboard (pool:8080). There is no :8443 listener.
const PoolApiUrlMap: Record<Network, string> = {
    testnet: 'https://pool.testnet.modeloslab.xyz',
    mainnet: 'https://pool.modeloslab.xyz',
};

function getBaseUrl(): string {
    return BlockbookBaseUrlMap[getCurrentNetwork()];
}

function getPoolUrl(): string {
    return PoolApiUrlMap[getCurrentNetwork()];
}

export interface InferenceFeeStats {
    pending_count: number;
    slow_grains: number;
    median_grains: number;
    fast_grains: number;
    slow_mdl: number;
    median_mdl: number;
    fast_mdl: number;
    has_data: boolean;
    sample_size?: number;
    suggested: {
        slow:     { grains: number; est_blocks: string };
        standard: { grains: number; est_blocks: string };
        fast:     { grains: number; est_blocks: string };
    };
}

export const BlockbookClient = {
    async estimateFee(numBlocks: number) {
        const response = await fetch(`${getBaseUrl()}/api/v1/estimatefee/${numBlocks}`, {
            signal: AbortSignal.timeout(5000),
        });
        if (!response.ok) throw new Error(`estimateFee HTTP ${response.status}`);
        const data = await response.json();
        if (typeof data.result !== 'number') throw new Error('estimateFee: unexpected response format');
        return data.result;
    },

    // Query the blockbook explorer for the fulfilment status of an inference_tx.
    async getInferenceResult(promptHash: string): Promise<{
        status: 'pending' | 'fulfilled' | 'refunded';
        text?: string;
    }> {
        const response = await fetch(`${getBaseUrl()}/api/v1/inference/${promptHash}`, {
            signal: AbortSignal.timeout(5000),
        });
        if (!response.ok) return { status: 'pending' };
        const data = await response.json();
        const _VALID_STATUSES = ['pending', 'fulfilled', 'refunded'] as const;
        const rawStatus = typeof data.result_status === 'string' ? data.result_status.toLowerCase() : '';
        const status = (_VALID_STATUSES as readonly string[]).includes(rawStatus)
            ? rawStatus as 'pending' | 'fulfilled' | 'refunded'
            : 'pending';
        const text = typeof data.result_text === 'string' && data.result_text.length <= 100_000
            ? data.result_text
            : undefined;
        return { status, text };
    },

    // Fetch live inference fee suggestions from the pool.
    // Returns slow/median/fast percentile fees derived from recent confirmed jobs.
    // Falls back to protocol defaults when the pool is unreachable or has no history.
    async getInferenceFeeStats(): Promise<InferenceFeeStats> {
        const fallback: InferenceFeeStats = {
            pending_count: 0,
            slow_grains:   MIN_INFERENCE_FEE_GRAINS,
            median_grains: INFERENCE_FEE_TIERS.standard,
            fast_grains:   INFERENCE_FEE_TIERS.fast,
            slow_mdl:      MIN_INFERENCE_FEE_GRAINS / 1e8,
            median_mdl:    INFERENCE_FEE_TIERS.standard / 1e8,
            fast_mdl:      INFERENCE_FEE_TIERS.fast / 1e8,
            has_data:      false,
            suggested: {
                slow:     { grains: MIN_INFERENCE_FEE_GRAINS,     est_blocks: '3–6' },
                standard: { grains: INFERENCE_FEE_TIERS.standard, est_blocks: '1–2' },
                fast:     { grains: INFERENCE_FEE_TIERS.fast,     est_blocks: '<1'  },
            },
        };
        try {
            const res = await fetch(`${getPoolUrl()}/inference/fees`, { signal: AbortSignal.timeout(5000) });
            if (!res.ok) return fallback;
            return await res.json() as InferenceFeeStats;
        } catch {
            return fallback;
        }
    },

    // Returns the number of distinct miners that have won an inference fee for
    // each model version in the last 24 hours.  Derived from confirmed
    // inference_proof_tx wins — no miner self-reporting required.
    async getModelAvailability(): Promise<Record<string, number>> {
        try {
            const res = await fetch(`${getPoolUrl()}/inference/miners`, { signal: AbortSignal.timeout(5000) });
            if (!res.ok) return {};
            const data = await res.json() as { by_version: Record<string, number> };
            return data.by_version ?? {};
        } catch {
            return {};
        }
    },
};
