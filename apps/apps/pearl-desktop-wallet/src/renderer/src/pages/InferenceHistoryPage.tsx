import { useNavigate } from 'react-router-dom';
import { Plus, MessageSquare, CheckCircle2, AlertCircle, Loader2, Trash2, Clock } from 'lucide-react';
import { cn } from '@/lib/utils';
import { GRAINS_PER_MDL } from '@/config/consts';
import { useInferenceHistory, type InferenceConversation } from '@/hooks/use-inference-history';

function grainsToMDL(g: number): string {
  const v = g / GRAINS_PER_MDL;
  return v < 0.001 ? v.toFixed(6) : v.toFixed(4);
}

function formatDate(iso: string): string {
  const d = new Date(iso);
  const now = new Date();
  const diffMs = now.getTime() - d.getTime();
  const diffMins = Math.floor(diffMs / 60_000);
  if (diffMins < 1)  return 'Just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  const diffHrs = Math.floor(diffMins / 60);
  if (diffHrs < 24)  return `${diffHrs}h ago`;
  const diffDays = Math.floor(diffHrs / 24);
  if (diffDays < 7)  return `${diffDays}d ago`;
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

function StatusIcon({ status }: { status: InferenceConversation['status'] }) {
  switch (status) {
    case 'fulfilled':
      return <CheckCircle2 className="h-4 w-4 text-emerald-400 shrink-0" />;
    case 'refunded':
      return <AlertCircle className="h-4 w-4 text-orange-400 shrink-0" />;
    case 'active':
      return <Loader2 className="h-4 w-4 text-blue-400 animate-spin shrink-0" />;
    default:
      return <Clock className="h-4 w-4 text-neutral-500 shrink-0" />;
  }
}

function ConversationCard({
  convo,
  onOpen,
  onDelete,
}: {
  convo: InferenceConversation;
  onOpen: () => void;
  onDelete: (e: React.MouseEvent) => void;
}) {
  const firstQuestion = convo.messages.find(m => m.role === 'user')?.content ?? '';
  const firstAnswer   = convo.messages.find(m => m.role === 'assistant' && m.content !== convo.messages[0]?.content)?.content ?? '';
  const questionCount = convo.messages.filter(m => m.role === 'user').length;

  return (
    <button
      onClick={onOpen}
      className="group relative flex w-full items-start gap-4 rounded-xl border border-neutral-800 bg-neutral-900 p-4 text-left transition-colors hover:border-neutral-700 hover:bg-neutral-800/60"
    >
      {/* Status icon */}
      <div className="mt-0.5">
        <StatusIcon status={convo.status} />
      </div>

      {/* Content */}
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium text-white">
          {firstQuestion || 'Empty conversation'}
        </p>
        {firstAnswer && (
          <p className="mt-1 line-clamp-2 text-xs text-neutral-500 leading-relaxed">
            {firstAnswer}
          </p>
        )}
        <div className="mt-2 flex items-center gap-3 text-xs text-neutral-600">
          <span>{formatDate(convo.updatedAt)}</span>
          {questionCount > 0 && <span>{questionCount} {questionCount === 1 ? 'question' : 'questions'}</span>}
          {convo.totalFeeGrains > 0 && (
            <span className="text-neutral-700">{grainsToMDL(convo.totalFeeGrains)} MDL paid</span>
          )}
        </div>
      </div>

      {/* Delete button — shown on hover */}
      <button
        onClick={onDelete}
        className="absolute right-3 top-3 hidden rounded p-1 text-neutral-600 hover:bg-neutral-700 hover:text-red-400 group-hover:flex"
        title="Delete conversation"
      >
        <Trash2 className="h-3.5 w-3.5" />
      </button>
    </button>
  );
}

export default function InferenceHistoryPage() {
  const navigate  = useNavigate();
  const { conversations, deleteConversation } = useInferenceHistory();

  const handleOpen = (id: string) => navigate(`/inference?id=${id}`);
  const handleNew  = () => navigate('/inference');
  const handleDelete = (e: React.MouseEvent, id: string) => {
    e.stopPropagation();
    if (window.confirm('Delete this conversation?')) {
      deleteConversation(id);
    }
  };

  return (
    <div className="ml-20 flex h-screen flex-col bg-neutral-950 text-white">

      {/* Header */}
      <div className="flex items-center justify-between border-b border-neutral-800 px-6 py-4">
        <div>
          <h1 className="text-lg font-semibold text-white">Inference History</h1>
          <p className="text-xs text-neutral-500">
            {conversations.length} conversation{conversations.length !== 1 ? 's' : ''} · stored locally
          </p>
        </div>
        <button
          onClick={handleNew}
          className="flex items-center gap-2 rounded-lg bg-emerald-600 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-500 transition-colors"
        >
          <Plus className="h-4 w-4" />
          New conversation
        </button>
      </div>

      {/* List */}
      <div className="flex-1 overflow-y-auto px-6 py-4">
        {conversations.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-24 text-center">
            <MessageSquare className="mb-4 h-10 w-10 text-neutral-700" />
            <p className="text-sm text-neutral-500">No inference history yet</p>
            <p className="mt-1 text-xs text-neutral-700">
              Questions you ask and their answers will appear here
            </p>
            <button
              onClick={handleNew}
              className="mt-6 flex items-center gap-2 rounded-lg bg-emerald-600/20 px-4 py-2 text-sm text-emerald-400 hover:bg-emerald-600/30 transition-colors"
            >
              <Plus className="h-4 w-4" /> Ask your first question
            </button>
          </div>
        ) : (
          <div className="space-y-2 pb-4">
            {conversations.map(convo => (
              <ConversationCard
                key={convo.id}
                convo={convo}
                onOpen={() => handleOpen(convo.id)}
                onDelete={e => handleDelete(e, convo.id)}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
