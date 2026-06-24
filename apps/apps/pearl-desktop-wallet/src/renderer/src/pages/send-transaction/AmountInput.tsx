import { formatMDLAmount } from '../../lib/crypto';
import { AlertCircle } from 'lucide-react';

type AmountInputProps = {
  amount: string;
  onChange: (value: string) => void;
  spendableAmount?: number;
  currentFee: number | string;
  onMax: () => void;
  error?: string | null;
};

export default function AmountInput({
  amount,
  onChange,
  spendableAmount,
  currentFee,
  onMax,
  error,
}: AmountInputProps) {
  function onPercent(percentage: number) {
    const percentageAmount = ((spendableAmount ?? 0) * percentage) / 100;
    onChange(percentageAmount.toString());
  }

  return (
    <div className="space-y-2">
      <label className="text-sm text-neutral-400">Amount</label>
      <div className="relative">
        <input
          type="number"
          value={formatMDLAmount(amount)}
          onChange={e => onChange(e.target.value)}
          placeholder="0.00"
          className="focus:border-brand-green focus:ring-brand-green/20 w-full rounded-lg border border-gray-300 bg-white px-4 py-3 text-gray-900 placeholder-gray-400 shadow-sm [appearance:textfield] focus:outline-none focus:ring-2 [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
          step="0.00000001"
          min="0"
          max={formatMDLAmount(spendableAmount ?? 0)}
        />
      </div>
      {error && (
        <div className="mt-1 flex items-center gap-1 rounded border border-red-700/30 bg-red-900/20 px-2 py-1 text-xs text-red-400">
          <AlertCircle className="h-3 w-3 flex-shrink-0" />
          <span>{error}</span>
        </div>
      )}
      <div className="text-xs text-neutral-500">
        Spendable: {formatMDLAmount(spendableAmount ?? 0)} MDL (after{' '}
        {formatMDLAmount(currentFee)} MDL / kb fee reduction)
      </div>
      <div className="mt-3 grid grid-cols-4 gap-2">
        <button
          type="button"
          onClick={() => onPercent(25)}
          className="rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-700 shadow-sm transition-all hover:border-gray-400 hover:bg-gray-50"
        >
          25%
        </button>
        <button
          type="button"
          onClick={() => onPercent(50)}
          className="rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-700 shadow-sm transition-all hover:border-gray-400 hover:bg-gray-50"
        >
          50%
        </button>
        <button
          type="button"
          onClick={() => onPercent(75)}
          className="rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-700 shadow-sm transition-all hover:border-gray-400 hover:bg-gray-50"
        >
          75%
        </button>
        <button
          type="button"
          onClick={onMax}
          className="rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-700 shadow-sm transition-all hover:border-gray-400 hover:bg-gray-50"
        >
          MAX
        </button>
      </div>
    </div>
  );
}
