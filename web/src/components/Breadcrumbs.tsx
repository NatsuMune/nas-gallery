import { useNavigate } from 'react-router-dom';
import type { Folder } from '../api/types';

export function Breadcrumbs({ crumbs }: { crumbs: Folder[] }) {
  const navigate = useNavigate();
  if (crumbs.length === 0) return null;
  return (
    <nav className="crumbs" aria-label="Breadcrumb">
      <ol className="crumbs-list">
        {crumbs.map((crumb, i) => {
          const isLast = i === crumbs.length - 1;
          return (
            <li key={crumb.id}>
              <button
                type="button"
                className="crumb"
                aria-current={isLast ? 'page' : undefined}
                onClick={() => navigate(`/folders/${crumb.id}`)}
              >
                {crumb.name}
              </button>
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
