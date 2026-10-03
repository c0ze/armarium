// Pairing a phone: the link a QR code carries and the QR itself. The link is
// <reader>/#opds=<catalogue>, which comics.skriv.ist and books.skriv.ist read on
// startup; the token rides in the fragment, so it never reaches the reader's host.
import qrcode from 'qrcode-generator';

const trimSlash = (s: string) => s.replace(/\/$/, '');

export function pairLink(readerUrl: string, base: string, secret: string): string {
  const catalogue = `${trimSlash(base)}/opds/t/${secret}/v1.2/catalog`;
  return `${trimSlash(readerUrl)}/#opds=${encodeURIComponent(catalogue)}`;
}

const pad = (n: number) => String(n).padStart(2, '0');

/** A token name per pairing, e.g. comics-phone-2026-10-04-1532 (local time). */
export function pairTokenName(id: string, d: Date): string {
  return `${id}-phone-${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}-${pad(d.getHours())}${pad(d.getMinutes())}`;
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
