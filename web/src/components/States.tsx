import type { ReactNode } from 'react';
import { RefreshIcon, WifiOffIcon } from './Icons';

interface StateProps {
  icon?: ReactNode;
  title: string;
  hint?: ReactNode;
  action?: ReactNode;
}

export function EmptyState({ icon, title, hint, action }: StateProps) {
  return (
    <div className="state state-empty" role="status">
      <div className="state-icon" aria-hidden="true">
        {icon}
      </div>
      <h3 className="state-title">{title}</h3>
      {hint && <p className="state-hint">{hint}</p>}
      {action}
    </div>
  );
}

export function ErrorState({ title, hint, onRetry }: { title: string; hint?: ReactNode; onRetry?: () => void }) {
  return (
    <div className="state state-error" role="alert">
      <div className="state-icon" aria-hidden="true">
        <WifiOffIcon size={28} />
      </div>
      <h3 className="state-title">{title}</h3>
      {hint && <p className="state-hint">{hint}</p>}
      {onRetry && (
        <button className="btn" onClick={onRetry}>
          <RefreshIcon size={16} />
          Retry
        </button>
      )}
    </div>
  );
}
