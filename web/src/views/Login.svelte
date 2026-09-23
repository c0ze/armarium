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
      error = err instanceof ApiError && err.status === 401 ? 'That password is not right.' : err instanceof ApiError ? err.message : 'Login failed.';
    } finally {
      busy = false;
    }
  }
</script>

<main class="login">
  <h1>Armarium</h1>
  {#if session.passwordSet}
    <form onsubmit={submit}>
      <label for="pw" class="caps">Owner password</label>
      <!-- svelte-ignore a11y_autofocus -->
      <input id="pw" type="password" autocomplete="current-password" bind:value={password} autofocus />
      <button disabled={busy || !password}>{busy ? 'Opening' : 'Open the armarium'}</button>
      {#if error}<p class="error" role="alert">{error}</p>{/if}
    </form>
  {:else}
    <p class="muted">
      No password is set. Run <code>armarium hash-password</code>, put the result in
      <code>password_hash</code> in <code>armarium.toml</code> (or <code>ARMARIUM_PASSWORD_HASH</code>), and restart.
    </p>
  {/if}
</main>

<style>
  .login { min-height: 100dvh; display: grid; place-content: center; justify-items: center; gap: 2.4rem; padding: 2rem; }
  h1 { font-size: clamp(4.5rem, 12vw, 9rem); letter-spacing: 0.06em; }
  form { display: grid; gap: 0.8rem; width: min(22rem, 90vw); }
  input { padding: 0.9rem 1rem; font-size: 16px; }
  button { justify-content: center; }
  .error { margin: 0; font-size: 13px; }
  p.muted { max-width: 30rem; text-align: center; }
  code { font-size: 0.9em; color: var(--text); }
</style>
