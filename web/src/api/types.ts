// Types mirroring docs/API_CONTRACT.md exactly. Unknown fields are tolerated;
// nullable fields are marked.

export type ItemType = 'photo' | 'video';
export type SortField = 'name' | 'date' | 'size' | 'type';
export type SortOrder = 'asc' | 'desc';
export type TypeFilter = 'all' | 'photo' | 'video';

export interface Folder {
  id: string;
  name: string;
  parentId: string | null;
  relativePath: string;
  fileCount: number;
  childCount: number;
  coverItemId: string | null;
}

export interface Item {
  id: string;
  name: string;
  type: ItemType;
  size: number;
  modTime: string;
  width: number | null;
  height: number | null;
  durationMs: number | null;
  favorite: boolean;
  folderId: string;
  /** Signed path (not absolute); prefix the configured server origin. */
  thumbUrl: string;
  /** Signed path (not absolute); prefix the configured server origin. */
  streamUrl: string;
}

export interface FolderResponse {
  folder: Folder | null;
  children: Folder[];
  breadcrumbs: Folder[];
}

export interface ItemPage {
  items: Item[];
  page: number;
  pageSize: number;
  total: number;
  /** Present for folder item lists; absent for favorites/search. */
  folder?: Folder | null;
}

export interface LoginResponse {
  token: string;
  expiresAt: string;
}

export interface MeResponse {
  user: { id: number; username: string };
  server: { name: string; version: string };
}

export interface HealthResponse {
  ok: boolean;
  version: string;
}

export interface LibraryStatus {
  scanning: boolean;
  lastScanAt: string;
  counts: { photo: number; video: number };
}

export interface FavoriteResponse {
  favorite: boolean;
}

export interface ScanResponse {
  scanning: boolean;
}
