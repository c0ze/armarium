import { describe, expect, it } from 'vitest';
import { defaultName, passwordProblem, readCode, settingsFor, uniqueName, waitFor } from './setup';

describe('readCode', () => {
  it('takes the code from the setup link fragment', () => {
    expect(readCode('#code=abc-DEF_123')).toBe('abc-DEF_123');
    expect(readCode('#x=1&code=abc')).toBe('abc');
  });
  it('is null without a code', () => {
    expect(readCode('')).toBeNull();
    expect(readCode('#code=')).toBeNull();
    expect(readCode('#other=1')).toBeNull();
  });
});

describe('passwordProblem', () => {
  it('needs 8 characters, typed the same twice', () => {
    expect(passwordProblem('short', 'short')).toBe('Use at least 8 characters.');
    expect(passwordProblem('long enough', 'long enougH')).toBe("The two passwords don't match.");
    expect(passwordProblem('long enough', 'long enough')).toBeNull();
  });
});

describe('defaultName', () => {
  it('names a library after its folder', () => {
    expect(defaultName('/Users/arda/Comics')).toBe('Comics');
    expect(defaultName('/Users/arda/Calibre Library/')).toBe('Calibre Library');
    expect(defaultName('C:\\Books\\Comics\\')).toBe('Comics');
    expect(defaultName('D:\\')).toBe('Library');
    expect(defaultName('/')).toBe('Library');
  });
});

describe('uniqueName', () => {
  it('adds a number when the name is taken', () => {
    expect(uniqueName('Comics', [])).toBe('Comics');
    expect(uniqueName('Comics', ['Comics'])).toBe('Comics 2');
    expect(uniqueName('Comics', ['Comics', 'Comics 2'])).toBe('Comics 3');
  });
});

describe('settingsFor', () => {
  it('builds the settings the server applies', () => {
    const s = settingsFor(
      [
        { name: ' Comics ', root: '/c', kind: 'comics', calibre: false },
        { name: 'Books', root: '/b', kind: 'books', calibre: true },
      ],
      'home',
      true,
      8580,
    );
    expect(s).toEqual({
      libraries: [
        { name: 'Comics', root: '/c', kind: 'comics', calibre: false },
        { name: 'Books', root: '/b', kind: 'books', calibre: true },
      ],
      network: 'home',
      tailscale: true,
      port: 8580,
      publicUrl: '',
      extraHosts: [],
    });
  });
});

describe('waitFor', () => {
  it('resolves true once the check passes', async () => {
    let n = 0;
    expect(await waitFor(async () => ++n >= 3, 1000, 1)).toBe(true);
    expect(n).toBe(3);
  });
  it('resolves false after the timeout, treating errors as not yet', async () => {
    expect(await waitFor(async () => { throw new Error('down'); }, 20, 5)).toBe(false);
  });
});
