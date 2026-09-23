// Typed client for the Armarium JSON API. The UI authenticates with the session
// cookie; every write sends JSON so the server's CSRF rules are satisfied.

export type Status = 'unread' | 'reading' | 'read';

export interface Progress {
  page: number;
  locator: string;
  status: Status;
  updatedAt: number;
}

export interface Library {
  id: number;
  name: string;
  kind: 'comics' | 'books';
}

export interface Series {
  id: number;
  libraryId: number;
  name: string;
  items: number;
  unread: number;
  coverItemId: number;
}

export interface Item {
  id: number;
  libraryId: number;
  seriesId: number;
  seriesName: string;
  /** cbz, cbr, epub and pdf are read in the browser; mobi, azw3, … are download-only. */
  format: string;
  size: number;
  title: string;
  author: string;
  hasCover: boolean;
  /** Other formats of the same (Calibre) book, downloadable via fileUrl(id, false, format). */
  formats: string[];
  number: number | null;
  pages: number;
  source: string;
  addedAt: number;
  tags: string[];
  progress: Progress;
}

export interface Page<T> {
  items: T[];
  total: number;
  offset: number;
  limit: number;
}

export interface Session {
  authenticated: boolean;
  admin: boolean;
  passwordSet: boolean;
  panelflowUrl: string;
}

export interface Facet {
  name: string;
  count: number;
  label?: string;
}

export interface TocEntry {
  title: string;
  chapter: number;
  fragment?: string;
  depth: number;
}

export interface Token {
  id: number;
  name: string;
  createdAt: number;
  lastUsedAt: number | null;
}

export interface ScanStatus {
  running: boolean;
  startedAt: number;
  finishedAt: number;
  added: number;
  updated: number;
  missing: number;
  errors: number;
  lastError: string;
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method, credentials: 'same-origin', headers: {} };
  if (body !== undefined) {
    init.body = JSON.stringify(body);
    (init.headers as Record<string, string>)['Content-Type'] = 'application/json';
  }
  const res = await fetch(path, init);
  if (res.status === 204) return undefined as T;
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new ApiError(res.status, (data as { error?: string }).error ?? res.statusText);
  return data as T;
}

export function query(params: Record<string, string | number | undefined>): string {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== '' && v !== 0) q.set(k, String(v));
  const s = q.toString();
  return s ? `?${s}` : '';
}

export const api = {
  session: () => call<Session>('GET', '/api/session'),
  login: (password: string) => call<Session>('POST', '/api/login', { password }),
  logout: () => call<Session>('POST', '/api/logout', {}),
  libraries: () => call<Library[]>('GET', '/api/libraries'),
  series: (libraryId: number, q = '', offset = 0) =>
    call<Page<Series>>('GET', `/api/libraries/${libraryId}/series${query({ q, offset, limit: 200 })}`),
  seriesDetail: (id: number) => call<{ series: Series; items: Item[] }>('GET', `/api/series/${id}`),
  items: (f: Record<string, string | number | undefined>) => call<Page<Item>>('GET', `/api/items${query(f)}`),
  item: (id: number) => call<{ item: Item; library: Library }>('GET', `/api/items/${id}`),
  facets: (library?: number) => call<{ sources: Facet[]; tags: Facet[] }>('GET', `/api/facets${query({ library })}`),
  toc: (id: number) => call<{ chapters: number; toc: TocEntry[] }>('GET', `/api/items/${id}/toc`),
  progress: (id: number, body: { page?: number; locator?: string; status?: Status; reset?: boolean }) =>
    call<{ progress: Progress; applied: boolean }>('PUT', `/api/items/${id}/progress`, body),
  scanStatus: () => call<ScanStatus>('GET', '/api/scan'),
  scan: (library?: string) => call<{ started: boolean }>('POST', '/api/scan', library ? { library } : {}),
  tokens: () => call<Token[]>('GET', '/api/tokens'),
  createToken: (name: string) => call<{ token: Token; secret: string }>('POST', '/api/tokens', { name }),
  deleteToken: (id: number) => call<void>('DELETE', `/api/tokens/${id}`),
};

export const coverUrl = (id: number) => `/api/items/${id}/cover`;
export const pageUrl = (id: number, n: number) => `/api/items/${id}/pages/${n}`;
export const fileUrl = (id: number, inline = false, format = '') =>
  `/api/items/${id}/file${inline ? '?inline=1' : format ? `?format=${encodeURIComponent(format)}` : ''}`;
export const readable = (format: string) => ['cbz', 'cbr', 'epub', 'pdf'].includes(format);
export const chapterUrl = (id: number, n: number) => `/read/${id}/chapter/${n}`;

export function formatSize(bytes: number): string {
  if (bytes < 1 << 20) return `${Math.max(1, Math.round(bytes / 1024))} KB`;
  if (bytes < 1 << 30) return `${(bytes / (1 << 20)).toFixed(1)} MB`;
  return `${(bytes / (1 << 30)).toFixed(2)} GB`;
}

const norm = (s: string) => s.trim().toLocaleLowerCase();

/** The series worth naming for an item: none when it is only the author grouping
 *  that Calibre libraries use for books outside a series. */
export function seriesOf(it: Pick<Item, 'author' | 'seriesName'>): string {
  const authors = it.author.split(',').map(norm);
  return it.seriesName && !authors.includes(norm(it.seriesName)) ? it.seriesName : '';
}

/** Percent read, for progress bars. EPUB pages are chapters. */
export function percent(it: Item): number {
  if (it.progress.status === 'read') return 100;
  if (!it.pages || !it.progress.page) return 0;
  return Math.min(99, Math.round((it.progress.page / it.pages) * 100));
}
