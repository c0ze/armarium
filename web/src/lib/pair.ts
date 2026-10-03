// Pairing a phone: the link a QR code carries and the QR itself. The link is
// <reader>/#opds=<catalogue>, which comics.skriv.ist and books.skriv.ist read on
// startup; the token rides in the fragment, so it never reaches the reader's host.
import qrcode from 'qrcode-generator';
import { ApiError, type Token } from './api';

const trimSlash = (s: string) => s.replace(/\/$/, '');

export function pairLink(readerUrl: string, base: string, secret: string): string {
  const catalogue = `${trimSlash(base)}/opds/t/${secret}/v1.2/catalog`;
  return `${trimSlash(readerUrl)}/#opds=${encodeURIComponent(catalogue)}`;
}

const pad = (n: number) => String(n).padStart(2, '0');

/** A device label ("iPhone", "Arda's Pixel") as a token-name part: iphone, arda-s-pixel. */
export function deviceSlug(label: string): string {
  const slug = label.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
  return slug.slice(0, 32).replace(/-+$/, '') || 'phone';
}

export const DEVICES = ['iPhone', 'Android', 'iPad', 'Other'];

/** The device choice Admin remembers in localStorage; anything unexpected falls back to iPhone. */
export function readDeviceChoice(raw: string | null): { device: string; other: string } {
  let v: unknown;
  try {
    v = JSON.parse(raw || '{}');
  } catch {
    v = {};
  }
  const o = (v && typeof v === 'object' ? v : {}) as { device?: unknown; other?: unknown };
  return {
    device: typeof o.device === 'string' && DEVICES.includes(o.device) ? o.device : 'iPhone',
    other: typeof o.other === 'string' ? o.other.slice(0, 40) : '',
  };
}

/** A token name per pairing, e.g. books-iphone-2026-10-04-1532 (local time). */
export function pairTokenName(id: string, device: string, d: Date): string {
  return `${id}-${device}-${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}-${pad(d.getHours())}${pad(d.getMinutes())}`;
}

type Created = { token: Token; secret: string };

/** Creates the pairing token; a name already taken this minute gets -2. */
export async function createPairToken(
  id: string,
  device: string,
  now: Date,
  create: (name: string) => Promise<Created>,
): Promise<Created> {
  const name = pairTokenName(id, device, now);
  try {
    return await create(name);
  } catch (err) {
    if (!(err instanceof ApiError) || err.status !== 409) throw err;
    return create(`${name}-2`);
  }
}

/** The QR as one SVG path: a 1×1 square per dark module, inside a 4-module quiet zone. */
export function qrPath(text: string): { size: number; d: string } {
  let n: number;
  const qr = qrcode(0, 'M');
  try {
    qr.addData(text, 'Byte');
    qr.make();
    n = qr.getModuleCount();
  } catch (err) {
    // The generator throws plain strings when the data does not fit.
    throw new Error(err instanceof Error ? err.message : String(err));
  }
  let d = '';
  for (let r = 0; r < n; r++) for (let c = 0; c < n; c++) if (qr.isDark(r, c)) d += `M${c + 4} ${r + 4}h1v1h-1z`;
  return { size: n + 8, d };
}

/** True for addresses only this computer can reach. */
export function isLoopback(base: string): boolean {
  try {
    return ['localhost', '127.0.0.1', '[::1]', '::1'].includes(new URL(base).hostname);
  } catch {
    return false;
  }
}
