import { useNavigate } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { useCoverItem } from '../hooks/useCoverItem';
import { joinUrl } from '../api/client';
import type { Folder } from '../api/types';
import { ChevronRightIcon } from './Icons';

interface FolderTileProps {
  folder: Folder;
}

function FolderTile({ folder }: FolderTileProps) {
  const { api, serverUrl } = useAuth();
  const navigate = useNavigate();
  const cover = useCoverItem(api, folder.coverItemId);
  const coverSrc = cover ? joinUrl(serverUrl, cover.thumbUrl) : null;

  return (
    <button className="folder-tile" onClick={() => navigate(`/folders/${folder.id}`)}>
      <span className="folder-tile-art">
        {coverSrc ? <img src={coverSrc} alt="" loading="lazy" decoding="async" /> : <span className="folder-tile-blank" />}
      </span>
      <span className="folder-tile-name">{folder.name}</span>
      <span className="folder-tile-count">
        {folder.fileCount} item{folder.fileCount === 1 ? '' : 's'}
        {folder.childCount > 0 && ` · ${folder.childCount} folder${folder.childCount === 1 ? '' : 's'}`}
      </span>
      <span className="folder-tile-chevron" aria-hidden="true">
        <ChevronRightIcon size={16} />
      </span>
    </button>
  );
}

export function FolderStrip({ folders }: { folders: Folder[] }) {
  if (folders.length === 0) return null;
  return (
    <div className="folder-strip" aria-label="Subfolders">
      {folders.map((folder) => (
        <FolderTile key={folder.id} folder={folder} />
      ))}
    </div>
  );
}
