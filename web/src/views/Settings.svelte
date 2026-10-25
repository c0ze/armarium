<script lang="ts">
  import { api, ApiError, type LibrarySettings, type SettingsInfo } from '../lib/api';
  import { libraryChanges, passwordProblem, settingsFor, waitFor, waitForRestart } from '../lib/setup';
  import LibraryRows from './LibraryRows.svelte';
  import NetworkChoice from './NetworkChoice.svelte';

  let info = $state<SettingsInfo | null>(null);
  let items = $state<Record<string, number>>({});
  let error = $state('');
  let saving = $state(false);
  let restarting = $state(false);

  let libs = $state<LibrarySettings[]>([]);
  let original = $state<LibrarySettings[]>([]);
  let removed = $state<string[]>([]);
  let confirming = $state<{ lib: LibrarySettings; index: number } | null>(null);
  let network = $state<'local' | 'home'>('local');
  let tailscale = $state(false);
  let port = $state(8580);

  let current = $state('');
  let next = $state('');
  let again = $state('');
  let passwordMsg = $state('');
  let passwordErr = $state('');

  async function load() {
    try {
      const i = await api.settings();
      info = i;
      items = i.items ?? {};
      libs = i.settings.libraries.map((l) => ({ ...l }));
      original = i.settings.libraries.map((l) => ({ ...l }));
      removed = [];
      network = i.settings.network;
      tailscale = i.settings.tailscale;
      port = i.settings.port || 8580;
    } catch (e) {
      error = (e as Error).message;
    }
  }
  load();

  function askRemove(lib: LibrarySettings, index: number) {
    // A library that isn't saved yet has nothing to lose.
    if (!original.some((o) => o.root === lib.root)) {
      libs = libs.filter((_, j) => j !== index);
      return;
    }
    confirming = { lib, index };
  }

  function confirmRemove() {
    if (!confirming) return;
    const old = original.find((o) => o.root === confirming!.lib.root);
    if (old) removed = [...removed, old.name];
    libs = libs.filter((_, j) => j !== confirming!.index);
    confirming = null;
  }

  async function save() {
    if (!info) return;
    error = '';
    saving = true;
    const settings = { ...settingsFor(libs, network, tailscale, port), publicUrl: info.settings.publicUrl, extraHosts: info.settings.extraHosts };
    try {
      const { renames, removals } = libraryChanges(original, libs, removed);
      const { reloadUrl } = await api.saveSettings({ settings, renames, removals });
      restarting = true;
      const target = new URL(reloadUrl);
      const healthy = async () => (await fetch('/healthz', { cache: 'no-store' })).ok;
      if (target.origin === location.origin) {
        await waitForRestart(healthy);
        restarting = false;
        await load();
      } else {
        // The page may only fetch its own origin (CSP connect-src 'self'), so
        // a new port can't be polled: wait for this one to close, give the
        // server a moment to start on the new one, then go there.
        await waitFor(async () => !(await healthy().catch(() => false)), 5000, 100);
        await new Promise((r) => setTimeout(r, 1500));
        location.assign(reloadUrl);
      }
    } catch (e) {
      error = e instanceof ApiError ? e.message : (e as Error).message;
    } finally {
      saving = false;
    }
  }

  async function changePassword(e: SubmitEvent) {
    e.preventDefault();
    passwordMsg = passwordErr = '';
    const problem = passwordProblem(next, again);
    if (problem) {
      passwordErr = problem;
      return;
    }
    try {
      await api.changePassword(current, next);
      current = next = again = '';
      passwordMsg = 'Password changed. Other browsers and devices logged in to this page have to log in again.';
    } catch (err) {
      passwordErr = err instanceof ApiError && err.status === 401 ? 'The current password is not right.' : (err as Error).message;
    }
  }
</script>

<header class="head"><h1>Settings</h1></header>

{#if error}<p class="error" role="alert">{error}</p>{/if}

{#if restarting}
  <p class="muted" role="status">Restarting Armarium…</p>
{:else if info}
  {#if info.editable}
    <section>
      <h2>Libraries</h2>
      <p class="muted">Armarium only reads these folders. Renaming a library keeps its reading progress.</p>
      <LibraryRows bind:libs canChoose={true} check={api.checkFolder} choose={api.chooseFolder} onremove={askRemove} />
      {#if confirming}
        <div class="confirm" role="alertdialog" aria-label="Remove library">
          <p>
            Delete "{confirming.lib.name}" and its {items[original.find((o) => o.root === confirming!.lib.root)?.name ?? ''] ?? 0} items from Armarium?
            Your files stay where they are. Reading progress for these items is lost.
          </p>
          <div class="row">
            <button class="danger" onclick={confirmRemove}>Delete from Armarium</button>
            <button class="quiet" onclick={() => (confirming = null)}>Keep it</button>
          </div>
        </div>
      {/if}
    </section>

    <section>
      <h2>Who can read</h2>
      <NetworkChoice bind:network bind:tailscale status={info.tailscale} />
      <label class="port">
        <span class="caps">Port</span>
        <input type="number" min="1024" max="65535" bind:value={port} />
      </label>
      {#if info.derived.publicUrl}
        <p class="muted small">Phones pair with <code>{info.derived.publicUrl}</code>.</p>
      {/if}
    </section>

    <div class="save">
      <button onclick={save} disabled={saving}>{saving ? 'Saving…' : 'Save and restart'}</button>
    </div>
  {:else}
    <section>
      <h2>Configuration</h2>
      <p class="muted">This Armarium reads its settings from <code>armarium.toml</code>. Edit that file and restart to change them.</p>
      <dl>
        <dt class="caps">Listening on</dt><dd><code>{info.derived.listen}</code></dd>
        <dt class="caps">Host names</dt><dd><code>{info.derived.allowedHosts.join(', ')}</code></dd>
        <dt class="caps">Phones pair with</dt><dd><code>{info.derived.publicUrl || 'not set (public_url)'}</code></dd>
        <dt class="caps">Libraries</dt><dd>{info.settings.libraries.map((l) => l.name).join(', ') || 'none'}</dd>
      </dl>
      <p class="muted small">For phones and the Skrivist readers, these lines in <code>armarium.toml</code> matter:</p>
      <pre><code>{info.snippet}</code></pre>
    </section>
  {/if}

  <section>
    <h2>Owner password</h2>
    {#if info.passwordFromConfig}
      <p class="muted">The password is set in armarium.toml (<code>password_hash</code>).</p>
    {:else}
      <form class="password" onsubmit={changePassword}>
        <label for="cur" class="caps">Current password</label>
        <input id="cur" type="password" autocomplete="current-password" bind:value={current} />
        <label for="new" class="caps">New password</label>
        <input id="new" type="password" autocomplete="new-password" bind:value={next} />
        <label for="new2" class="caps">Type it again</label>
        <input id="new2" type="password" autocomplete="new-password" bind:value={again} />
        <button disabled={!current || !next}>Change password</button>
        {#if passwordErr}<p class="error" role="alert">{passwordErr}</p>{/if}
        {#if passwordMsg}<p class="muted" role="status">{passwordMsg}</p>{/if}
      </form>
    {/if}
  </section>
{/if}

<style>
  .head { padding: 2.4rem 0 1.4rem; border-bottom: 1px solid var(--line); }
  h1 { font-size: clamp(2.6rem, 5vw, 4rem); }
  h2 { font-size: 2rem; margin-bottom: 0.6rem; }
  section { max-width: 56rem; padding: 2rem 0; border-bottom: 1px solid var(--line); display: grid; gap: 0.9rem; }
  .confirm { padding: 1rem 1.1rem; border: 1px solid #5a2e2a; border-radius: 3px; display: grid; gap: 0.8rem; }
  .confirm p { margin: 0; }
  .row { display: flex; gap: 0.7rem; flex-wrap: wrap; }
  .port { display: grid; gap: 0.3rem; width: 9rem; }
  .save { padding: 1.6rem 0; }
  .password { display: grid; gap: 0.6rem; width: min(22rem, 100%); }
  .password button { justify-self: start; margin-top: 0.4rem; }
  dl { display: grid; grid-template-columns: max-content 1fr; gap: 0.5rem 1.2rem; margin: 0; }
  dd { margin: 0; }
  pre { margin: 0; padding: 0.9rem 1rem; background: var(--surface); border: 1px solid var(--line); border-radius: 3px; overflow-x: auto; }
  code { font-size: 12.5px; color: var(--text); }
  .small { font-size: 13px; }
</style>
