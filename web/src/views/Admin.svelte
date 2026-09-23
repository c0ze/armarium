<script lang="ts">
  import { onDestroy } from 'svelte';
  import { api, type Library, type ScanStatus, type Token } from '../lib/api';
  import Icon from './Icon.svelte';

  let { admin }: { admin: boolean } = $props();
  let libraries = $state<Library[]>([]);
  let tokens = $state<Token[]>([]);
  let status = $state<ScanStatus | null>(null);
  let newName = $state('');
  let created = $state<{ name: string; secret: string } | null>(null);
  let confirmRevoke = $state<number | null>(null);
  let error = $state('');
  let poll: ReturnType<typeof setInterval> | undefined;

  const origin = location.origin;

  async function load() {
    try {
      [libraries, tokens, status] = await Promise.all([api.libraries(), api.tokens(), api.scanStatus()]);
      if (status.running) startPolling();
    } catch (e) {
      error = (e as Error).message;
    }
  }
  $effect(() => { if (admin) load(); });

  function startPolling() {
    if (poll) return;
    poll = setInterval(async () => {
      status = await api.scanStatus();
      if (!status.running) { clearInterval(poll); poll = undefined; }
    }, 1500);
  }

  async function scan(library?: string) {
    error = '';
    try {
      await api.scan(library);
      status = await api.scanStatus();
      startPolling();
    } catch (e) {
      error = (e as Error).message;
    }
  }

  async function createToken(e: SubmitEvent) {
    e.preventDefault();
    error = '';
    try {
      const r = await api.createToken(newName.trim());
      created = { name: r.token.name, secret: r.secret };
      newName = '';
      tokens = await api.tokens();
    } catch (err) {
      error = (err as Error).message;
    }
  }

  // Revoking is a two-step commit: the first press arms the row, the second revokes.
  async function revoke(t: Token) {
    if (confirmRevoke !== t.id) { confirmRevoke = t.id; return; }
    confirmRevoke = null;
    await api.deleteToken(t.id);
    tokens = await api.tokens();
  }

  const when = (s: number | null) => (s ? new Date(s * 1000).toLocaleString() : 'never');
  onDestroy(() => clearInterval(poll));
</script>

{#if !admin}
  <p class="center muted">Admin needs the owner's password session; API tokens cannot open it.</p>
{:else}
  <header class="head"><h1>Admin</h1></header>
  {#if error}<p class="error">{error}</p>{/if}

  <section>
    <div class="bar">
      <h2>Libraries</h2>
      <button disabled={status?.running} onclick={() => scan()}>{status?.running ? 'Scanning' : 'Scan all'}</button>
    </div>
    <p class="note muted">Libraries come from <code>armarium.toml</code>. A scan opens only new and changed files.</p>
    {#if status && status.startedAt}
      <p class="scan" class:live={status.running}>
        {#if status.running}<span class="pulse" aria-hidden="true"></span>Scanning now{:else}Last scan {when(status.finishedAt)}{/if}
        <span>{status.added.toLocaleString()} added</span><span>{status.updated.toLocaleString()} updated</span><span>{status.missing.toLocaleString()} missing</span>
        {#if status.errors}<span class="error">{status.errors} failed: {status.lastError}</span>{/if}
      </p>
    {/if}
    <ul class="rows">
      {#each libraries as l (l.id)}
        <li>
          <Icon name={l.kind === 'comics' ? 'comic' : 'book'} />
          <span class="name">{l.name}</span>
          <span class="caps">{l.kind}</span>
          <button class="quiet" disabled={status?.running} onclick={() => scan(l.name)}>Scan</button>
        </li>
      {/each}
    </ul>
  </section>

  <section>
    <div class="bar"><h2>API tokens</h2></div>
    <p class="note muted">For PanelFlow, comics.skriv.ist and OPDS apps. A token reads and saves progress; it can never administer.</p>
    <form onsubmit={createToken} class="new">
      <input placeholder="Name it after the app, e.g. panelflow" bind:value={newName} maxlength="64" aria-label="Token name" />
      <button disabled={!newName.trim()}>Create token</button>
    </form>
    {#if created}
      <div class="secret" role="status">
        <p><strong>{created.name}</strong>: copy it now; it is shown only once.</p>
        <code>{created.secret}</code>
        <p class="muted">OPDS with the token in the path, for apps without Basic auth:</p>
        <code>{origin}/opds/t/{created.secret}/v1.2/catalog</code>
      </div>
    {/if}
    <ul class="rows">
      {#each tokens as t (t.id)}
        <li>
          <span class="name">{t.name}</span>
          <span class="muted small">last used {when(t.lastUsedAt)}</span>
          <button class="quiet" class:armed={confirmRevoke === t.id} onclick={() => revoke(t)} onblur={() => (confirmRevoke = null)}>
            {confirmRevoke === t.id ? 'Press again to revoke' : 'Revoke'}
          </button>
        </li>
      {:else}
        <li class="muted">No tokens yet.</li>
      {/each}
    </ul>
  </section>

  <section>
    <div class="bar"><h2>Connecting apps</h2></div>
    <dl>
      <dt class="caps">OPDS, Basic auth</dt><dd><code>{origin}/opds/v1.2/catalog</code> · any user name, a token as the password</dd>
      <dt class="caps">PanelFlow · comics.skriv.ist</dt><dd>Server <code>{origin}</code> and a token; the reader's origin must be in <code>cors_origins</code></dd>
    </dl>
  </section>
{/if}

<style>
  .head { padding: 2.8rem 0 1rem; }
  h1 { font-size: clamp(3rem, 5vw, 4.6rem); }
  section { max-width: 56rem; padding: 2rem 0; border-top: 1px solid var(--line); }
  .bar { display: flex; align-items: center; justify-content: space-between; gap: 1rem; }
  h2 { font-size: 2rem; }
  .note { margin: 0.5rem 0 1.2rem; font-size: 14px; }
  .scan { display: flex; flex-wrap: wrap; align-items: center; gap: 0.4rem 1.4rem; margin: 0 0 1rem; font-size: 13px; color: var(--text-2); }
  .scan.live { color: var(--amber); }
  .pulse { width: 0.5rem; height: 0.5rem; border-radius: 50%; background: var(--amber); margin-right: -0.8rem; animation: pulse 1.4s var(--ease) infinite; }
  @keyframes pulse { 50% { opacity: 0.25; } }
  .rows { list-style: none; padding: 0; margin: 0; }
  .rows li { display: flex; align-items: center; gap: 1rem; padding: 0.55rem 0.2rem; border-bottom: 1px solid var(--line); color: var(--text-2); }
  .rows .name { color: var(--text); flex: 1; }
  .small { font-size: 12.5px; }
  .armed { color: var(--danger); }
  .new { display: flex; gap: 0.7rem; margin-bottom: 1rem; }
  .new input { flex: 1; }
  .secret { margin: 0 0 1.2rem; padding: 1rem 1.1rem; background: var(--surface); border: 1px solid var(--amber); border-radius: 3px; display: grid; gap: 0.5rem; }
  .secret p { margin: 0; }
  code { font-size: 12.5px; word-break: break-all; color: var(--text); }
  dl { margin: 0.8rem 0 0; display: grid; gap: 0.4rem 1.6rem; grid-template-columns: max-content 1fr; }
  dt { padding-top: 0.15rem; }
  dd { margin: 0; color: var(--text-2); font-size: 14px; }
  .center { margin: 20vh auto; text-align: center; }
  @media (max-width: 700px) { dl { grid-template-columns: 1fr; } .new { flex-direction: column; } }
  @media (prefers-reduced-motion: reduce) { .pulse { animation: none; } }
</style>
