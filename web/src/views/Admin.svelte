<script lang="ts">
  import { onDestroy } from 'svelte';
  import { api, type Library, type ScanStatus, type Token } from '../lib/api';

  let { admin }: { admin: boolean } = $props();
  let libraries = $state<Library[]>([]);
  let tokens = $state<Token[]>([]);
  let status = $state<ScanStatus | null>(null);
  let newName = $state('');
  let created = $state<{ name: string; secret: string } | null>(null);
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

  async function revoke(t: Token) {
    if (!confirm(`Revoke token "${t.name}"? Apps using it stop working.`)) return;
    await api.deleteToken(t.id);
    tokens = await api.tokens();
  }

  const when = (s: number | null) => (s ? new Date(s * 1000).toLocaleString() : 'never');
  onDestroy(() => clearInterval(poll));
</script>

<main>
  {#if !admin}
    <p class="panel">Admin needs the owner password session; API tokens can't open this page.</p>
  {:else}
    {#if error}<p class="error">{error}</p>{/if}
    <section class="panel">
      <div class="row"><h2>Libraries</h2><button class="primary" disabled={status?.running} onclick={() => scan()}>Scan all</button></div>
      <p class="muted small">Libraries come from <code>armarium.toml</code>. Scans only open new or changed files.</p>
      <ul>
        {#each libraries as l (l.id)}
          <li><span><strong>{l.name}</strong> <span class="muted small">{l.kind}</span></span>
            <button disabled={status?.running} onclick={() => scan(l.name)}>Scan</button></li>
        {/each}
      </ul>
      {#if status && status.startedAt}
        <p class="small">
          {status.running ? 'Scanning…' : `Last scan finished ${when(status.finishedAt)}`}
          · {status.added} added · {status.updated} updated · {status.missing} missing
          {#if status.errors}<span class="error">· {status.errors} errors: {status.lastError}</span>{/if}
        </p>
      {/if}
    </section>

    <section class="panel">
      <h2>API tokens</h2>
      <p class="muted small">For PanelFlow, comics.skriv.ist and OPDS apps. A token can read and save progress, never administer.</p>
      <form onsubmit={createToken} class="row">
        <input placeholder="Name, e.g. panelflow or koreader" bind:value={newName} maxlength="64" aria-label="Token name" />
        <button class="primary" disabled={!newName.trim()}>Create</button>
      </form>
      {#if created}
        <div class="secret">
          <p><strong>{created.name}</strong>: copy this now; it is not shown again.</p>
          <code>{created.secret}</code>
          <p class="small muted">OPDS with the token in the path (for apps without Basic auth):</p>
          <code>{origin}/opds/t/{created.secret}/v1.2/catalog</code>
        </div>
      {/if}
      <ul>
        {#each tokens as t (t.id)}
          <li><span><strong>{t.name}</strong> <span class="muted small">last used {when(t.lastUsedAt)}</span></span>
            <button class="danger" onclick={() => revoke(t)}>Revoke</button></li>
        {/each}
      </ul>
    </section>

    <section class="panel">
      <h2>Connecting apps</h2>
      <dl class="small">
        <dt>OPDS (Basic auth: any username, token as password)</dt><dd><code>{origin}/opds/v1.2/catalog</code></dd>
        <dt>PanelFlow / comics.skriv.ist</dt><dd>Armarium server <code>{origin}</code> plus a token. Add the reader's origin to <code>cors_origins</code>.</dd>
      </dl>
    </section>
  {/if}
</main>

<style>
  main { max-width: 52rem; margin: 1rem auto; display: flex; flex-direction: column; gap: 1.2rem; }
  h2 { font-size: 1.25rem; }
  .row { display: flex; justify-content: space-between; align-items: center; gap: 0.6rem; }
  form.row input { flex: 1; }
  ul { list-style: none; padding: 0; margin: 0.8rem 0 0; }
  li { display: flex; justify-content: space-between; align-items: center; padding: 0.5rem 0; border-top: 1px solid var(--divider); gap: 1rem; }
  .secret { margin: 0.8rem 0; padding: 0.8rem; border-radius: 8px; border: 1px dashed var(--accent-primary); }
  .secret p { margin: 0.2rem 0; }
  code { word-break: break-all; font-size: 0.85em; }
  dt { font-weight: 600; margin-top: 0.6rem; }
  dd { margin: 0.2rem 0 0; }
</style>
