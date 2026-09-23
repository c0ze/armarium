<script lang="ts">
  import { api, ApiError, type Session } from '../lib/api';

  let { session, onLogin }: { session: Session; onLogin: () => void } = $props();
  let password = $state('');
  let error = $state('');
  let busy = $state(false);

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    error = '';
    try {
      await api.login(password);
      onLogin();
    } catch (err) {
      error = err instanceof ApiError ? err.message : 'Login failed.';
    } finally {
      busy = false;
    }
  }
</script>

<main class="panel login">
  <h1>Arma<span>rium</span></h1>
  {#if session.passwordSet}
    <form onsubmit={submit}>
      <label for="pw" class="visually-hidden">Password</label>
      <!-- svelte-ignore a11y_autofocus -->
      <input id="pw" type="password" autocomplete="current-password" placeholder="Password" bind:value={password} autofocus />
      <button class="primary" disabled={busy || !password}>Sign in</button>
    </form>
    {#if error}<p class="error small">{error}</p>{/if}
  {:else}
    <p class="muted">
      No password is configured. Run <code>armarium hash-password</code> and put the result in
      <code>password_hash</code> in <code>armarium.toml</code> (or <code>ARMARIUM_PASSWORD_HASH</code>), then restart.
    </p>
  {/if}
</main>

<style>
  .login { max-width: 22rem; margin: 18vh auto 0; text-align: center; }
  h1 { font-size: 2rem; margin-bottom: 1.2rem; }
  h1 span {
    background: linear-gradient(100deg, var(--accent-primary), var(--accent-secondary));
    -webkit-background-clip: text;
    background-clip: text;
    color: transparent;
  }
  form { display: flex; flex-direction: column; gap: 0.7rem; }
  button { justify-content: center; }
  code { font-size: 0.85em; }
</style>
