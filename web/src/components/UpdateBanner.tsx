import { useEffect, useState } from 'react';
import { applyUpdate, getPwaState, subscribePwa } from '../pwa';
import { UpdateIcon } from './Icons';

/** "Update available — Reload" banner shown when a new SW has activated. */
export function UpdateBanner() {
  const [updateAvailable, setUpdateAvailable] = useState(getPwaState().updateAvailable);

  useEffect(() => subscribePwa(() => setUpdateAvailable(getPwaState().updateAvailable)), []);

  if (!updateAvailable) return null;

  return (
    <div className="update-banner" role="alertdialog" aria-live="polite" aria-label="Update available">
      <span>A new version is available.</span>
      <button className="btn btn-primary" onClick={applyUpdate}>
        <UpdateIcon size={16} />
        Update available — Reload
      </button>
    </div>
  );
}
