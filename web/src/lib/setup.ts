// First-run setup and Settings: the pure parts, kept out of the components so
// they can be tested.
import type { LibrarySettings, Settings } from './api';

/** The one-time code from the setup link's fragment (#code=…). */
export function readCode(hash: string): string | null {
  const code = new URLSearchParams(hash.replace(/^#/, '')).get('code');
  return code ? code : null;
}

export function passwordProblem(a: string, b: string): string | null {
  if (a.length < 8) return 'Use at least 8 characters.';
  if (a !== b) return "The two passwords don't match.";
  return null;
}

/** A library's first name: its folder's name, for / and \ paths alike. */
export function defaultName(path: string): string {
  const parts = path.split(/[\\/]+/).filter((p) => p && !/^[A-Za-z]:$/.test(p));
  return parts.at(-1) ?? 'Library';
}

export function uniqueName(name: string, taken: string[]): string {
  if (!taken.includes(name)) return name;
  let n = 2;
  while (taken.includes(`${name} ${n}`)) n++;
  return `${name} ${n}`;
}

export function settingsFor(libs: LibrarySettings[], network: 'local' | 'home', tailscale: boolean, port: number): Settings {
  return {
    libraries: libs.map((l) => ({ ...l, name: l.name.trim() })),
    network,
    tailscale,
    port,
    publicUrl: '',
    extraHosts: [],
  };
}

/** Polls check until it returns true or timeoutMs passes; a throw counts as "not yet". */
export async function waitFor(check: () => Promise<boolean>, timeoutMs: number, everyMs = 500): Promise<boolean> {
  const end = Date.now() + timeoutMs;
  for (;;) {
    try {
      if (await check()) return true;
    } catch {
      // The server is restarting.
    }
    if (Date.now() >= end) return false;
    await new Promise((r) => setTimeout(r, everyMs));
  }
}

/** What a Settings save does to existing libraries: a library still on the
 * same folder under another name is renamed (keeping its items); removals are
 * the ones the owner confirmed. */
export function libraryChanges(
  before: LibrarySettings[],
  after: LibrarySettings[],
  removed: string[],
): { renames: Record<string, string>; removals: string[] } {
  const renames: Record<string, string> = {};
  for (const old of before) {
    const now = after.find((l) => l.root === old.root);
    if (now && now.name.trim() !== old.name) renames[old.name] = now.name.trim();
  }
  return { renames, removals: [...removed] };
}

/** After a save the desktop app restarts the server about 300 ms later: wait
 * until it is gone (or a moment has passed), then until it answers again. */
export async function waitForRestart(
  up: () => Promise<boolean>,
  { downMs = 3000, upMs = 20000, everyMs = 100 } = {},
): Promise<boolean> {
  await waitFor(async () => !(await up().catch(() => false)), downMs, everyMs);
  return waitFor(up, upMs, everyMs);
}
