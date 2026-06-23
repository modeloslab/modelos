import { formatMDLAmount } from '../../lib/crypto';
import KeyValueRow from './KeyValueRow';

type WalletInfoProps = {
  walletName?: string;
  balance?: number;
};

export default function WalletInfo({ walletName, balance }: WalletInfoProps) {
  return (
    <div className="rounded-xl border border-gray-300 bg-white p-4 shadow-sm">
      <div className="space-y-1">
        <KeyValueRow label="From Wallet:" rightContent={walletName} />

        <KeyValueRow label="Balance:" rightContent={`${formatMDLAmount(balance ?? 0)} MDL`} />
      </div>
    </div>
  );
}
