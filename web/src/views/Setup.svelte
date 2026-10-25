<script lang="ts">
  import { untrack } from 'svelte';
  import { api, ApiError, setupApi, type LibrarySettings, type SetupInfo } from '../lib/api';
  import { passwordProblem, readCode, settingsFor, waitFor } from '../lib/setup';
  import LibraryRows from './LibraryRows.svelte';
  import NetworkChoice from './NetworkChoice.svelte';

  // First-run setup. The one-time code comes from the setup link the desktop
  // app opens (or the server logs); it is moved out of the address bar at once.
  let { done = false }: { done?: boolean } = $props();

  const KEY = 'armarium.setup';
  function takeCode(): string | null {
    const fromLink = readCode(location.hash);
    try {
      if (fromLink) sessionStorage.setItem(KEY, fromLink);
      return fromLink ?? sessionStorage.getItem(KEY);
    } catch {
      return fromLink;
    } finally {
      if (fromLink) history.replaceState(null, '', location.pathname + location.search);
    }
  }
  const finished = untrack(() => done); // decided once, when the page opens
  const code = finished ? null : takeCode();
  const setup = code ? setupApi(code) : null;

  type Phase = 'loading' | 'nocode' | 'expired' | 'already' | 'steps' | 'starting' | 'failed';
  let phase = $state<Phase>(finished ? 'already' : code ? 'loading' : 'nocode');
  let info = $state<SetupInfo | null>(null);
  let step = $state(0);
  let error = $state('');
  let busy = $state(false);

  let password = $state('');
  let again = $state('');
  let libs = $state<LibrarySettings[]>([]);
  let network = $state<'local' | 'home'>('local');
  let tailscale = $state(false);
  let startAtLogin = $state(true);

  const steps = $derived(info?.managed ? ['Password', 'Libraries', 'Who can read', 'Finish'] : ['Password', 'Finish']);
  const last = $derived(step === steps.length - 1);

  function failed(e: unknown) {
    if (e instanceof ApiError && e.status === 410) phase = 'already';
    else if (e instanceof ApiError && e.status === 401) phase = 'expired';
    else error = e instanceof Error ? e.message : String(e);
  }

  if (setup) {
    setup
      .info()
      .then((i) => {
        info = i;
        if (i.settings) {
          libs = i.settings.libraries;
          network = i.settings.network;
          tailscale = i.settings.tailscale;
        }
        phase = 'steps';
      })
      .catch(failed);
  }

  function next() {
    error = '';
    if (step === 0) {
      const problem = passwordProblem(password, again);
      if (problem) {
        error = problem;
        return;
      }
    }
    step++;
  }

  async function finish() {
    if (!setup || !info) return;
    error = '';
    busy = true;
    const settings = info.managed ? settingsFor(libs, network, tailscale, info.settings?.port || 8580) : null;
    try {
      await setup.finish({ password, settings, startAtLogin: info.desktop ? startAtLogin : undefined });
    } catch (e) {
      busy = false;
      failed(e);
      return;
    }
    try {
      sessionStorage.removeItem(KEY);
    } catch {
      // Storage blocked: nothing was kept.
    }
    phase = 'starting';
    // The desktop app restarts the server on the new settings; wait for it,
    // then start the first scan.
    const up = await waitFor(async () => (await api.session()).admin, 20000);
    if (!up) {
      phase = 'failed';
      return;
    }
    if (settings?.libraries.length) await api.scan().catch(() => undefined);
    location.assign('/');
  }
</script>

<main class="setup">
  <h1>Armarium</h1>

  {#if phase === 'nocode'}
    <section class="panel">
      <h2>Finish setting up</h2>
      <p>Open Armarium from its menu-bar icon (Mac) or tray icon (Windows) to finish setting it up.</p>
      <p class="muted">On a server, the setup link is in Armarium's log, for example <code>docker logs armarium</code>.</p>
    </section>
  {:else if phase === 'expired'}
    <section class="panel">
      <h2>This setup link has expired</h2>
      <p>Armarium makes a new link each time it starts. Quit Armarium and open it again, or copy the newest link from its log.</p>
    </section>
  {:else if phase === 'already'}
    <section class="panel">
      <h2>Armarium is already set up</h2>
      <p class="muted">Log in with the owner password.</p>
      <a class="button" href="/login">Log in</a>
    </section>
  {:else if phase === 'loading'}
    <p class="muted">Opening setup…</p>
  {:else if phase === 'starting'}
    <section class="panel"><h2>Starting your library…</h2><p class="muted">This takes a few seconds.</p></section>
  {:else if phase === 'failed'}
    <section class="panel">
      <h2>Armarium didn't come back</h2>
      <p>Your settings were saved. Open Armarium again from its menu-bar or tray icon. If it keeps failing, Show log folder in the same menu has the details.</p>
    </section>
  {:else if info}
    <ol class="progress" aria-label="Setup steps">
      {#each steps as s, i (s)}<li class:on={i === step} class:past={i < step} aria-current={i === step ? 'step' : undefined}>{s}</li>{/each}
    </ol>

    <section class="panel">
      {#if step === 0}
        <h2>Choose the owner password</h2>
        <p class="muted">You'll use it to log in to Armarium and to change its settings. At least 8 characters.</p>
        <form class="password" onsubmit={(e) => { e.preventDefault(); next(); }}>
          <label for="pw" class="caps">Password</label>
          <!-- svelte-ignore a11y_autofocus -->
          <input id="pw" type="password" autocomplete="new-password" bind:value={password} autofocus />
          <label for="pw2" class="caps">Type it again</label>
          <input id="pw2" type="password" autocomplete="new-password" bind:value={again} />
          <button>Next</button>
        </form>
      {:else if info.managed && step === 1}
        <h2>Where are your books?</h2>
        <p class="muted">Add the folders Armarium should show. It only reads them; it never changes or moves your files.</p>
        <LibraryRows bind:libs canChoose={info.desktop} check={setup!.checkFolder} choose={setup!.chooseFolder} />
        <div class="nav">
          <button class="quiet" onclick={() => step--}>Back</button>
          <button onclick={next}>{libs.length ? 'Next' : 'Skip for now'}</button>
        </div>
      {:else if info.managed && step === 2}
        <h2>Who can read?</h2>
        <NetworkChoice bind:network bind:tailscale status={info.tailscale} />
        <p class="muted small">You can change this later in Settings.</p>
        <div class="nav">
          <button class="quiet" onclick={() => step--}>Back</button>
          <button onclick={next}>Next</button>
        </div>
      {:else if last}
        <h2>Ready</h2>
        {#if info.managed}
          <ul class="summary">
            <li>{libs.length ? `${libs.length} ${libs.length === 1 ? 'library' : 'libraries'}: ${libs.map((l) => l.name).join(', ')}` : 'No libraries yet: add them later in Settings.'}</li>
            <li>{network === 'home' ? 'Devices at home can read' : 'Only this computer can read'}{tailscale ? ', and anywhere with Tailscale' : ''}.</li>
          </ul>
        {:else}
          <p class="muted">
            Libraries and network settings for this server are in <code>armarium.toml</code>. Admin explains what phones need.
          </p>
        {/if}
        {#if info.desktop}
          <label class="check"><input type="checkbox" bind:checked={startAtLogin} /> Start Armarium when you log in to this computer</label>
        {/if}
        <div class="nav">
          <button class="quiet" onclick={() => step--} disabled={busy}>Back</button>
          <button class="primary" onclick={finish} disabled={busy}>{busy ? 'Saving…' : 'Start reading'}</button>
        </div>
      {/if}
      {#if error}<p class="error" role="alert">{error}</p>{/if}
    </section>
  {/if}
</main>

<style>
  .setup { max-width: 46rem; margin: 0 auto; padding: clamp(1.5rem, 6vh, 4rem) 1rem 4rem; display: grid; gap: 1.6rem; }
  h1 { font-size: clamp(3.4rem, 9vw, 5.5rem); letter-spacing: 0.06em; }
  h2 { font-size: 2rem; }
  .panel { display: grid; gap: 1rem; justify-items: start; }
  .panel > :global(*) { max-width: 100%; }
  .progress { list-style: none; margin: 0; padding: 0; display: flex; flex-wrap: wrap; gap: 0.4rem 1.4rem; font: 500 11px/1.2 var(--ui); letter-spacing: 0.16em; text-transform: uppercase; color: var(--text-3); counter-reset: s; }
  .progress li { counter-increment: s; }
  .progress li::before { content: counter(s) ' '; }
  .progress li.on { color: var(--text); }
  .progress li.past { color: var(--text-2); }
  .password { display: grid; gap: 0.6rem; width: min(22rem, 100%); }
  .password button { justify-self: start; margin-top: 0.4rem; }
  .password input { padding: 0.9rem 1rem; font-size: 16px; }
  .nav { display: flex; gap: 0.7rem; margin-top: 0.6rem; }
  .summary { margin: 0; padding-left: 1.2rem; display: grid; gap: 0.3rem; }
  .check { display: flex; gap: 0.6rem; align-items: center; }
  .check input { accent-color: var(--text); }
  .small { font-size: 13px; }
  code { font-size: 0.9em; color: var(--text); }
  :global(.setup) :global(.panel) { width: 100%; }
</style>
