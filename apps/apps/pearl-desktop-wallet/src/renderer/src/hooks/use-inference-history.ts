/**
 * Persistent inference conversation history stored in localStorage.
 *
 * Each conversation is a list of messages (user + assistant) with metadata.
 * Survives page reloads and wallet restarts.
 */

import { useState, useCallback, useEffect } from 'react';

export type MessageStatus = 'pending' | 'submitted' | 'fulfilled' | 'refunded' | 'error';
export type FeeTier = 'slow' | 'standard' | 'fast';

export interface InferenceMessage {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  status?: MessageStatus;
  promptHash?: string;
  txid?: string;
  feeGrains?: number;
  feeTier?: FeeTier;
  maxTokens?: number;
  createdAt: string; // ISO string for JSON serialisation
}

export interface InferenceConversation {
  id: string;
  messages: InferenceMessage[];
  createdAt: string;
  updatedAt: string;
  totalFeeGrains: number;
  status: 'active' | 'fulfilled' | 'refunded' | 'mixed';
}

const STORAGE_KEY = 'modelos_inference_history_v1';
const MAX_CONVERSATIONS = 200;

function load(): InferenceConversation[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) : [];
  } catch {
    return [];
  }
}

function save(convos: InferenceConversation[]): void {
  try {
    // Keep only the most recent MAX_CONVERSATIONS to avoid unbounded growth.
    const trimmed = convos.slice(-MAX_CONVERSATIONS);
    localStorage.setItem(STORAGE_KEY, JSON.stringify(trimmed));
  } catch {
    // localStorage full — not fatal
  }
}

function deriveStatus(messages: InferenceMessage[]): InferenceConversation['status'] {
  const userMsgs = messages.filter(m => m.role === 'user');
  if (!userMsgs.length) return 'active';
  const statuses = new Set(userMsgs.map(m => m.status).filter(Boolean));
  if (statuses.has('pending') || statuses.has('submitted')) return 'active';
  if (statuses.has('fulfilled') && statuses.size === 1) return 'fulfilled';
  if (statuses.has('refunded') && statuses.size === 1) return 'refunded';
  return 'mixed';
}

export function useInferenceHistory() {
  const [conversations, setConversations] = useState<InferenceConversation[]>(load);

  // Persist on every change.
  useEffect(() => {
    save(conversations);
  }, [conversations]);

  /** Create a new empty conversation, return its id. */
  const createConversation = useCallback((): string => {
    const id = crypto.randomUUID();
    const now = new Date().toISOString();
    const convo: InferenceConversation = {
      id,
      messages: [],
      createdAt: now,
      updatedAt: now,
      totalFeeGrains: 0,
      status: 'active',
    };
    setConversations(prev => [...prev, convo]);
    return id;
  }, []);

  /** Append or replace messages in a conversation. */
  const updateConversation = useCallback(
    (id: string, updater: (msgs: InferenceMessage[]) => InferenceMessage[]) => {
      setConversations(prev =>
        prev.map(c => {
          if (c.id !== id) return c;
          const newMsgs = updater(c.messages);
          const totalFeeGrains = newMsgs
            .filter(m => m.role === 'user' && m.feeGrains)
            .reduce((sum, m) => sum + (m.feeGrains ?? 0), 0);
          return {
            ...c,
            messages: newMsgs,
            updatedAt: new Date().toISOString(),
            totalFeeGrains,
            status: deriveStatus(newMsgs),
          };
        })
      );
    },
    []
  );

  /** Delete a conversation. */
  const deleteConversation = useCallback((id: string) => {
    setConversations(prev => prev.filter(c => c.id !== id));
  }, []);

  /** Get a specific conversation by id. */
  const getConversation = useCallback(
    (id: string): InferenceConversation | undefined => {
      return conversations.find(c => c.id === id);
    },
    [conversations]
  );

  /** Most recent conversations first. */
  const sorted = [...conversations].sort(
    (a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime()
  );

  return {
    conversations: sorted,
    createConversation,
    updateConversation,
    deleteConversation,
    getConversation,
  };
}
