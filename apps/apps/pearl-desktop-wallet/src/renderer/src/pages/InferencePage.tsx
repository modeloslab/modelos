import { useState, useRef, useEffect, useCallback, useMemo } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import {
  Send, Loader2, Plus,
  CheckCircle2, XCircle, AlertCircle, RefreshCw, History,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { GRAINS_PER_MDL, MIN_INFERENCE_FEE_GRAINS, INFERENCE_MODELS } from '@/config/consts';
import { MarkdownRenderer } from '@/components/markdown-renderer';
import {
  useInferenceHistory,
  type InferenceMessage,
  type FeeTier,
  type MessageStatus,
} from '@/hooks/use-inference-history';

// ─── Helpers ─────────────────────────────────────────────────────────────────

function grainsToMDL(grains: number): string {
  const v = grains / GRAINS_PER_MDL;
  return v < 0.01 ? v.toFixed(4) : v.toFixed(2);
}

/** Calculate total fee: model base + per-token component for that model. */
function calcFee(modelVersion: number, maxTokens: number): number {
  const m = INFERENCE_MODELS.find(m => m.version === modelVersion) ?? INFERENCE_MODELS[0];
  return m.baseGrains + m.perTokenGrains * maxTokens;
}

interface FeeStats {
  pending_count: number;
  slow_grains: number;
  median_grains: number;
  fast_grains: number;
  has_data: boolean;
  sample_size?: number;
  suggested: {
    slow:     { grains: number; est_blocks: string };
    standard: { grains: number; est_blocks: string };
    fast:     { grains: number; est_blocks: string };
  };
}

const DEFAULT_FEE_STATS: FeeStats = {
  pending_count: 0,
  slow_grains:   MIN_INFERENCE_FEE_GRAINS,
  median_grains: MIN_INFERENCE_FEE_GRAINS * 5,
  fast_grains:   MIN_INFERENCE_FEE_GRAINS * 20,
  has_data:      false,
  suggested: {
    slow:     { grains: MIN_INFERENCE_FEE_GRAINS,         est_blocks: '3–6' },
    standard: { grains: MIN_INFERENCE_FEE_GRAINS * 5,     est_blocks: '1–2' },
    fast:     { grains: MIN_INFERENCE_FEE_GRAINS * 20,    est_blocks: '<1'  },
  },
};

function StatusBadge({ status }: { status: MessageStatus }) {
  switch (status) {
    case 'pending':
      return <span className="flex items-center gap-1 text-xs text-yellow-400"><Loader2 className="h-3 w-3 animate-spin" /> Broadcasting…</span>;
    case 'submitted':
      return <span className="flex items-center gap-1 text-xs text-blue-400"><Loader2 className="h-3 w-3 animate-spin" /> On-chain — miners racing to answer…</span>;
    case 'fulfilled':
      return <span className="flex items-center gap-1 text-xs text-emerald-400"><CheckCircle2 className="h-3 w-3" /> Answered</span>;
    case 'refunded':
      return <span className="flex items-center gap-1 text-xs text-orange-400"><AlertCircle className="h-3 w-3" /> No response — fee refunded</span>;
    case 'error':
      return <span className="flex items-center gap-1 text-xs text-red-400"><XCircle className="h-3 w-3" /> Error</span>;
    default:
      return null;
  }
}

// ─── Main page ────────────────────────────────────────────────────────────────

export default function InferencePage() {
  const navigate        = useNavigate();
  const [searchParams]  = useSearchParams();
  const loadId          = searchParams.get('id');  // open a past conversation

  const { createConversation, updateConversation, getConversation } = useInferenceHistory();

  // Current conversation id — create new or load from URL.
  const [convId, setConvId] = useState<string>(() => {
    if (loadId) return loadId;
    return createConversation();
  });

  // Reload when URL param changes (user opens a history item).
  useEffect(() => {
    if (loadId && loadId !== convId) setConvId(loadId);
  }, [loadId]);

  // Derive messages from persisted store.
  const convo    = getConversation(convId);
  const messages = convo?.messages ?? [];

  const [input, setInput]               = useState('');
  const [feeTier, setFeeTier]           = useState<FeeTier>('standard');
  const [showFeeSelector, setShowFeeSelector] = useState(false);
  const [maxTokens, setMaxTokens]       = useState(512);
  const [modelVersion, setModelVersion] = useState(1); // 1=70B, 2=32B, 3=14B, 4=7B
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [feeStats, setFeeStats]         = useState<FeeStats>(DEFAULT_FEE_STATS);
  const [feeLoading, setFeeLoading]     = useState(false);
  // Per-model miner counts from pool — keyed by model_version string, value = distinct miner count.
  // Derived from confirmed inference_proof_tx wins (last 24h), not self-reported.
  const [minerCounts, setMinerCounts]   = useState<Record<string, number>>({});

  const bottomRef   = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: 'smooth' }); }, [messages]);

  // ── Live fee stats ────────────────────────────────────────────────────────
  const loadFeeStats = useCallback(async () => {
    setFeeLoading(true);
    try {
      const stats = await (window.api as any).getFeeStats?.();
      if (stats) setFeeStats(stats);
    } catch { /* keep defaults */ }
    finally { setFeeLoading(false); }
  }, []);

  // ── Live miner availability per model ─────────────────────────────────────
  const loadMinerCounts = useCallback(async () => {
    try {
      const counts = await (window.api as any).getModelAvailability?.();
      if (counts) setMinerCounts(counts);
    } catch { /* keep empty */ }
  }, []);

  useEffect(() => {
    loadFeeStats();
    const t = setInterval(loadFeeStats, 120_000);
    return () => clearInterval(t);
  }, [loadFeeStats]);

  // Refresh miner counts on mount and every 5 minutes.
  useEffect(() => {
    loadMinerCounts();
    const t = setInterval(loadMinerCounts, 300_000);
    return () => clearInterval(t);
  }, [loadMinerCounts]);

  // ── Push result listener ─────────────────────────────────────────────────
  useEffect(() => {
    const unsub = (window.api as any).onInferenceResult?.(
      (data: { promptHash: string; result: string }) => {
        // Find which conversation this result belongs to.
        updateConversation(convId, msgs => {
          const hasPrompt = msgs.some(m => m.promptHash === data.promptHash);
          if (!hasPrompt) return msgs;
          const updated = msgs.map(m =>
            m.promptHash === data.promptHash
              ? { ...m, status: 'fulfilled' as MessageStatus }
              : m
          );
          return [
            ...updated,
            {
              id: crypto.randomUUID(),
              role: 'assistant' as const,
              content: data.result,
              status: 'fulfilled' as MessageStatus,
              promptHash: data.promptHash,
              createdAt: new Date().toISOString(),
            },
          ];
        });
      }
    );
    return () => unsub?.();
  }, [convId, updateConversation]);

  // Fee is fully determined by model version + max tokens.
  // No separate slow/standard/fast tiers — the model IS the quality/cost tier.
  const selectedModel   = INFERENCE_MODELS.find(m => m.version === modelVersion) ?? INFERENCE_MODELS[0];
  const totalFeeGrains  = calcFee(modelVersion, maxTokens);
  const tokenFeeGrains  = selectedModel.perTokenGrains * maxTokens;

  // ── Submit ────────────────────────────────────────────────────────────────
  const handleSubmit = async () => {
    const prompt = input.trim();
    if (!prompt || isSubmitting) return;

    const msgId = crypto.randomUUID();

    const userMsg: InferenceMessage = {
      id: msgId,
      role: 'user',
      content: prompt,
      status: 'pending',
      feeGrains: totalFeeGrains,
      feeTier,
      maxTokens,
      createdAt: new Date().toISOString(),
    };

    updateConversation(convId, msgs => [...msgs, userMsg]);
    setInput('');
    setIsSubmitting(true);

    try {
      const result = await (window.api as any).submitInferenceTx({
        prompt,
        feeGrains: totalFeeGrains,
        maxTokens,
        feeTier,
        modelVersion,
      });

      updateConversation(convId, msgs =>
        msgs.map(m =>
          m.id === msgId
            ? { ...m, status: 'submitted' as MessageStatus, txid: result?.txid, promptHash: result?.promptHash }
            : m
        )
      );
    } catch {
      updateConversation(convId, msgs =>
        msgs.map(m => (m.id === msgId ? { ...m, status: 'error' as MessageStatus } : m))
      );
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleSubmit(); }
  };

  const handleNewConversation = () => {
    const id = createConversation();
    setConvId(id);
    navigate('/inference', { replace: true });
  };

  // ── Render ────────────────────────────────────────────────────────────────
  const isViewingHistory = Boolean(loadId);

  return (
    <div className="ml-20 flex h-screen flex-col bg-neutral-950 text-white">

      {/* Header */}
      <div className="flex items-center justify-between border-b border-neutral-800 px-6 py-4">
        <div>
          <h1 className="text-lg font-semibold text-white">AI Inference</h1>
          <p className="text-xs text-neutral-500">
            DeepSeek R1 Distill Llama 70B · result delivered directly to wallet
          </p>
        </div>

        <div className="flex items-center gap-3">
          {feeStats.pending_count > 0 && (
            <span className="rounded-full bg-yellow-500/10 px-2.5 py-1 text-xs text-yellow-400">
              {feeStats.pending_count} pending
            </span>
          )}

          {/* New conversation */}
          {isViewingHistory && (
            <button
              onClick={handleNewConversation}
              className="rounded-lg border border-neutral-700 px-3 py-1.5 text-xs text-neutral-400 hover:border-neutral-600 hover:text-white transition-colors"
            >
              New conversation
            </button>
          )}

          {/* History button */}
          <button
            onClick={() => navigate('/inference/history')}
            className="flex items-center gap-1.5 rounded-lg border border-neutral-700 px-3 py-1.5 text-xs text-neutral-400 hover:border-neutral-600 hover:text-white transition-colors"
          >
            <History className="h-3.5 w-3.5" />
            History
          </button>

          {/* Model selector */}
          <div className="flex items-center gap-2 rounded-lg border border-neutral-700 px-3 py-1.5 text-xs text-neutral-400">
            <span>Model</span>
            <select
              value={modelVersion}
              onChange={e => setModelVersion(Number(e.target.value))}
              className="bg-transparent text-white outline-none max-w-[200px]"
            >
              {INFERENCE_MODELS.map(m => {
                const count = minerCounts[String(m.version)];
                const minerLabel = count != null
                  ? count === 0
                    ? ' · no miners 24h'
                    : ` · ${count} miner${count === 1 ? '' : 's'}`
                  : '';
                return (
                  <option key={m.version} value={m.version} className="bg-neutral-900">
                    {m.name} ({m.vram}){minerLabel}
                  </option>
                );
              })}
            </select>
          </div>

          {/* Max tokens */}
          <div className="flex items-center gap-2 rounded-lg border border-neutral-700 px-3 py-1.5 text-xs text-neutral-400">
            <span>Max tokens</span>
            <select
              value={maxTokens}
              onChange={e => setMaxTokens(Number(e.target.value))}
              className="bg-transparent text-white outline-none"
            >
              {[256, 512, 1024, 2048, 4096].map(n => (
                <option key={n} value={n} className="bg-neutral-900">{n}</option>
              ))}
            </select>
          </div>
        </div>
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto px-6 py-4 space-y-4">
        {messages.length === 0 && (
          <div className="flex flex-col items-center justify-center py-20 text-center">
            <p className="text-sm text-neutral-500">Ask DeepSeek R1 anything.</p>
            <p className="mt-1 text-xs text-neutral-700">
              Your question is submitted on-chain · miners race to answer · result appears here instantly
            </p>
          </div>
        )}

        {messages.map(msg => (
          <div
            key={msg.id}
            className={cn('flex w-full', msg.role === 'user' ? 'justify-end' : 'justify-start')}
          >
            <div
              className={cn(
                'max-w-[78%] rounded-2xl px-4 py-3 text-sm',
                msg.role === 'user'
                  ? 'bg-emerald-600 text-white rounded-br-sm'
                  : 'bg-neutral-800 text-neutral-100 rounded-bl-sm'
              )}
            >
              {msg.role === 'user' ? (
                <p className="whitespace-pre-wrap leading-relaxed">{msg.content}</p>
              ) : (
                <MarkdownRenderer content={msg.content} />
              )}

              {msg.role === 'user' && msg.status && (
                <div className="mt-2 flex items-center justify-between gap-4 border-t border-white/10 pt-2">
                  <StatusBadge status={msg.status} />
                  {msg.feeGrains !== undefined && (
                    <span className="text-xs text-white/50">
                      {grainsToMDL(msg.feeGrains)} MDL
                    </span>
                  )}
                </div>
              )}

              {msg.txid && (
                <p className="mt-1 truncate text-xs text-white/25">tx: {msg.txid.slice(0, 16)}…</p>
              )}
            </div>
          </div>
        ))}
        <div ref={bottomRef} />
      </div>

      {/* Input area — hidden when viewing a past (completed) conversation */}
      {(!isViewingHistory || convo?.status === 'active') && (
        <div className="border-t border-neutral-800 px-6 py-4">

          {/* Fee summary — model + token pricing */}
          <div className="mb-3 flex items-center justify-between">
            <div className="flex items-center gap-2 text-xs text-neutral-500">
              <span className="rounded bg-neutral-800 px-2 py-1 text-neutral-300 font-medium">
                {grainsToMDL(totalFeeGrains)} MDL
              </span>
              <span className="text-neutral-700">
                {grainsToMDL(selectedModel.baseGrains)} base
                {tokenFeeGrains > 0 && ` + ${grainsToMDL(tokenFeeGrains)} for ${maxTokens} tokens`}
              </span>
              {feeStats.pending_count > 0 && (
                <span className="text-yellow-600">
                  · {feeStats.pending_count} pending in mempool
                </span>
              )}
            </div>
            <button
              onClick={loadFeeStats}
              disabled={feeLoading}
              className="flex items-center gap-1 text-xs text-neutral-700 hover:text-neutral-500 transition-colors"
            >
              <RefreshCw className={cn('h-3 w-3', feeLoading && 'animate-spin')} />
              {feeStats.has_data ? `${feeStats.sample_size} recent jobs` : 'refresh'}
            </button>
          </div>

          {/* Textarea + send */}
          <div className="flex items-end gap-3">
            <textarea
              ref={textareaRef}
              value={input}
              onChange={e => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Ask DeepSeek R1 anything… (Enter to send, Shift+Enter for newline)"
              rows={3}
              className="flex-1 resize-none rounded-xl border border-neutral-700 bg-neutral-900 px-4 py-3 text-sm text-white placeholder-neutral-600 outline-none focus:border-emerald-500 transition-colors"
            />
            <button
              onClick={handleSubmit}
              disabled={!input.trim() || isSubmitting}
              className={cn(
                'flex h-12 w-12 shrink-0 items-center justify-center rounded-xl transition-colors',
                input.trim() && !isSubmitting
                  ? 'bg-emerald-600 text-white hover:bg-emerald-500'
                  : 'bg-neutral-800 text-neutral-600 cursor-not-allowed'
              )}
            >
              {isSubmitting ? <Loader2 className="h-5 w-5 animate-spin" /> : <Send className="h-5 w-5" />}
            </button>
          </div>
        </div>
      )}

      {/* Completed history conversation — show "Continue" or start new */}
      {isViewingHistory && convo?.status !== 'active' && (
        <div className="border-t border-neutral-800 px-6 py-4 flex items-center justify-between">
          <p className="text-xs text-neutral-600">Past conversation · read only</p>
          <button
            onClick={handleNewConversation}
            className="flex items-center gap-2 rounded-lg bg-emerald-600 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-500 transition-colors"
          >
            <Plus className="h-4 w-4" /> New conversation
          </button>
        </div>
      )}
    </div>
  );
}

