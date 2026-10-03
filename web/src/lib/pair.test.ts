import { describe, expect, it, vi } from 'vitest';
import qrcode from 'qrcode-generator';
import { ApiError, type Token } from './api';
import { createPairToken, isLoopback, pairLink, pairTokenName, qrPath } from './pair';

const secret = 'Ab-_' + 'x'.repeat(39);

describe('pairLink', () => {
  it('encodes the catalogue URL once, after a single trailing slash is trimmed', () => {
    const link = pairLink('https://books.skriv.ist/', 'https://armarium.example/', secret);
    expect(link.startsWith('https://books.skriv.ist/#opds=https%3A%2F%2F')).toBe(true);
    expect(link).not.toContain('%253A');
    expect(new URLSearchParams(link.split('#')[1]).get('opds')).toBe(
      `https://armarium.example/opds/t/${secret}/v1.2/catalog`,
    );
  });
});

describe('pairTokenName', () => {
  it('uses zero-padded local 24-hour time', () => {
    expect(pairTokenName('comics', new Date(2026, 0, 5, 9, 7))).toBe('comics-phone-2026-01-05-0907');
    expect(pairTokenName('books', new Date(2026, 9, 4, 15, 32))).toBe('books-phone-2026-10-04-1532');
  });
});

describe('qrPath', () => {
  it('draws the generator matrix inside a 4-module quiet zone, version chosen automatically', () => {
    const long = pairLink('https://comics.skriv.ist', 'https://' + 'a'.repeat(253), secret);
    const q = qrPath(long);
    const ref = qrcode(0, 'M');
    ref.addData(long, 'Byte');
    ref.make();
    const n = ref.getModuleCount();
    expect(n).toBe(77); // version 15 for this 372-byte payload
    expect(q.size).toBe(n + 8);
    const squares = [...q.d.matchAll(/M(\d+) (\d+)h1v1h-1z/g)].map((m) => [+m[1], +m[2]]);
    const dark: number[][] = [];
    for (let r = 0; r < n; r++) for (let c = 0; c < n; c++) if (ref.isDark(r, c)) dark.push([c + 4, r + 4]);
    expect(squares).toEqual(dark);
    expect(q.d).not.toContain('style');
  });

  it('throws an Error when the data does not fit', () => {
    expect(() => qrPath('x'.repeat(3000))).toThrow(Error);
  });
});

describe('isLoopback', () => {
  it('spots addresses a phone cannot reach', () => {
    expect(isLoopback('http://127.0.0.1:8580')).toBe(true);
    expect(isLoopback('http://[::1]:8580')).toBe(true);
    expect(isLoopback('http://localhost:8580')).toBe(true);
    expect(isLoopback('https://armarium.example')).toBe(false);
  });
});

describe('createPairToken', () => {
  const now = new Date(2026, 9, 4, 15, 32);
  const made = (name: string) => ({ token: { id: 1, name, createdAt: 0, lastUsedAt: null } as Token, secret: 's' });

  it('names the token after the reader and the minute', async () => {
    const create = vi.fn(async (name: string) => made(name));
    await createPairToken('comics', now, create);
    expect(create.mock.calls).toEqual([['comics-phone-2026-10-04-1532']]);
  });

  it('retries once with -2 when the name is taken', async () => {
    const create = vi.fn(async (name: string) => made(name));
    create.mockRejectedValueOnce(new ApiError(409, 'a token with that name exists'));
    const r = await createPairToken('comics', now, create);
    expect(create.mock.calls).toEqual([['comics-phone-2026-10-04-1532'], ['comics-phone-2026-10-04-1532-2']]);
    expect(r.token.name).toBe('comics-phone-2026-10-04-1532-2');
  });

  it('gives up after the second conflict', async () => {
    const create = vi.fn(async (_: string) => {
      throw new ApiError(409, 'a token with that name exists');
    });
    await expect(createPairToken('comics', now, create)).rejects.toThrow('exists');
    expect(create).toHaveBeenCalledTimes(2);
  });

  it('does not retry other errors', async () => {
    const create = vi.fn(async (_: string) => {
      throw new ApiError(500, 'x');
    });
    await expect(createPairToken('comics', now, create)).rejects.toThrow('x');
    expect(create).toHaveBeenCalledTimes(1);
  });
});
