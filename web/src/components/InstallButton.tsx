import { useEffect, useState } from 'react';
import { getPwaState, promptInstall, subscribePwa } from '../pwa';
import { InstallIcon } from './Icons';

/** Small "Install app" button, visible only when the browser offers it. */
export function InstallButton() {
  const [canInstall, setCanInstall] = useState(getPwaState().canInstall);

  useEffect(() => subscribePwa(() => setCanInstall(getPwaState().canInstall)), []);

  if (!canInstall) return null;

  return (
    <button className="install-btn" onClick={() => void promptInstall()}>
      <InstallIcon size={16} />
      <span>Install</span>
    </button>
  );
}
