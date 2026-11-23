// Pairing the Skrivist apps: the link (and for phones the QR code that carries it).
// Every app takes skrivist-<app>://pair#opds=<catalogue>: the iPhone, iPad and Android
// apps scan it in Add library → Scan QR code, and the Mac and Windows apps open it as
// a link. Either app takes either app's link. The token rides in the fragment.
import qrcode from 'qrcode-generator';
import { ApiError, type Token } from './api';

const trimSlash = (s: string) => s.replace(/\/$/, '');

/** appLink is the app's own pairing link, e.g. skrivist-books://pair. */
export function pairLink(appLink: string, base: string, secret: string): string {
  const catalogue = `${trimSlash(base)}/opds/t/${secret}/v1.2/catalog`;
  return `${appLink}#opds=${encodeURIComponent(catalogue)}`;
}

/** The Skrivist apps' rule for plain http (contracts/pairing/url-policy.json in
 * skrivist-native): only private addresses and names, and only after the user confirms.
 * IPv4 10/8, 172.16/12, 192.168/16, 100.64/10 (Tailscale), 169.254/16, 127/8; IPv6
 * fc00::/7, fe80::/10, ::1 and IPv4-mapped private addresses; localhost, *.local, *.ts.net. */
export function isPrivateHost(base: string): boolean {
  let host: string;
  try {
    host = new URL(base).hostname.toLowerCase();
  } catch {
    return false;
  }
  if (host.endsWith('.')) host = host.slice(0, -1);
  if (host.startsWith('[')) {
    const b = ipv6(host.slice(1, -1));
    if (!b) return false;
    if (b.slice(0, 10).every((x) => x === 0) && b[10] === 0xff && b[11] === 0xff) return privateV4(b.slice(12));
    const loopback = b.slice(0, 15).every((x) => x === 0) && b[15] === 1;
    return loopback || (b[0] & 0xfe) === 0xfc || (b[0] === 0xfe && (b[1] & 0xc0) === 0x80);
  }
  const v4 = host.split('.');
  if (v4.length === 4 && v4.every((p) => /^(0|[1-9]\d{0,2})$/.test(p) && +p <= 255)) return privateV4(v4.map(Number));
  return (
    host === 'localhost' ||
    (host.endsWith('.local') && host.length > '.local'.length) ||
    (host.endsWith('.ts.net') && host.length > '.ts.net'.length)
  );
}

function privateV4([a, b]: number[]): boolean {
  return a === 10 || a === 127 || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168) ||
    (a === 100 && b >= 64 && b <= 127) || (a === 169 && b === 254);
}

/** The 16 bytes of an IPv6 address as the URL parser writes it (hex groups, `::`). */
function ipv6(text: string): number[] | null {
  const halves = text.split('::');
  if (halves.length > 2) return null;
  const groups = (s: string) => (s ? s.split(':').map((g) => (/^[0-9a-f]{1,4}$/.test(g) ? parseInt(g, 16) : NaN)) : []);
  const head = groups(halves[0]);
  const tail = halves.length === 2 ? groups(halves[1]) : [];
  if (halves.length === 2 && head.length + tail.length > 7) return null;
  const all = halves.length === 2 ? [...head, ...Array(8 - head.length - tail.length).fill(0), ...tail] : head;
  if (all.length !== 8 || all.some(Number.isNaN)) return null;
  return all.flatMap((g) => [g >> 8, g & 0xff]);
}

/** Whether this reader can load the catalogue from base. The desktop apps take any
 * address; the phone apps take HTTPS, or plain http to a private address. */
export const canPair = (reader: { desktop?: boolean }, base: string) =>
  !!reader.desktop || base.startsWith('https:') || isPrivateHost(base);

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
