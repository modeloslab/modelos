import {Link, useLocation} from 'react-router-dom';
import {Wallet, Settings} from 'lucide-react';
import {cn} from '@/lib/utils';
import {HIDDEN_SIDEBAR_PATHS} from '@/lib/constants';

export default function SideNav() {
  const location = useLocation();
  const pathname = location.pathname;

  // Inference moved out of the wallet into the standalone Compute web app.
  const navItems = [
    {href: '/wallet',   icon: Wallet,   label: 'Wallet',   match: (p: string) => p === '/wallet'},
    {href: '/settings', icon: Settings, label: 'Settings', match: (p: string) => p === '/settings'},
  ];

  if (HIDDEN_SIDEBAR_PATHS.some(path => pathname.startsWith(path))) {
    return null;
  }

  return (
    <div className="fixed bottom-0 left-0 top-0 z-50 flex w-20 flex-col items-center justify-center border-r border-neutral-700 bg-neutral-900/80 backdrop-blur-sm">
      <div className="flex flex-col gap-8">
        {navItems.map(item => (
          <Link
            key={item.href}
            to={item.href}
            className={cn(
              'flex flex-col items-center gap-2 rounded-lg p-3 transition-colors duration-200 hover:bg-neutral-800',
              item.match(pathname)
                ? 'bg-neutral-800 text-emerald-400'
                : 'text-neutral-400 hover:text-white'
            )}
          >
            <item.icon className="h-6 w-6" />
            <span className="text-center text-xs font-medium leading-tight">{item.label}</span>
          </Link>
        ))}
      </div>
    </div>
  );
}
