<script lang="ts">
  import { api, ApiError, type Session } from './lib/api';
  import { router, navigate, link } from './lib/router.svelte';
  import Login from './views/Login.svelte';
  import Library from './views/Library.svelte';
  import SeriesView from './views/Series.svelte';
  import ItemView from './views/Item.svelte';
  import BookReader from './views/BookReader.svelte';
  import ComicViewer from './views/ComicViewer.svelte';
  import Admin from './views/Admin.svelte';

  let session = $state<Session | null>(null);
  let failure = $state('');

  async function refresh() {
    try {
      session = await api.session();
    } catch (e) {
      failure = e instanceof ApiError ? e.message : 'Armarium is unreachable.';
    }
  }
  refresh();

  const route = $derived(router.route);
  const bare = $derived(route.name === 'reader' || route.name === 'viewer');

  $effect(() => {
    if (session && !session.authenticated && route.name !== 'login') navigate('/login', true);
  });

  async function logout() {
    await api.logout();
    await refresh();
  }
</script>

{#if failure}
  <main class="center"><p class="error">{failure}</p></main>
{:else if session}
  <div class="app" class:bare>
    {#if !bare && route.name !== 'login'}
      <header>
        <a class="logo" href="/" use:link>Arma<span>rium</span></a>
        <nav>
          {#if session.admin}<a href="/admin" use:link>Admin</a>{/if}
          {#if session.admin}<button onclick={logout}>Log out</button>{/if}
        </nav>
      </header>
    {/if}
    {#if route.name === 'login'}
      <Login {session} onLogin={async () => { await refresh(); navigate('/', true); }} />
    {:else if !session.authenticated}
      <p class="center muted">Redirecting to login…</p>
    {:else if route.name === 'library'}
      <Library params={route.params} />
    {:else if route.name === 'series'}
      <SeriesView id={route.id} />
    {:else if route.name === 'item'}
      {#key route.id}<ItemView id={route.id} panelflowUrl={session.panelflowUrl} />{/key}
    {:else if route.name === 'reader'}
      {#key route.id}<BookReader id={route.id} />{/key}
    {:else if route.name === 'viewer'}
      {#key route.id}<ComicViewer id={route.id} />{/key}
    {:else if route.name === 'admin'}
      <Admin admin={session.admin} />
    {:else}
      <main class="center"><h2>Not found</h2><p><a href="/" use:link>Back to the library</a></p></main>
    {/if}
  </div>
{/if}

<style>
  .app { min-height: 100dvh; padding: 0 max(1rem, env(safe-area-inset-left)) 3rem; }
  .app.bare { padding: 0; }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    max-width: 76rem;
    margin: 0 auto;
    padding-block: 1.2rem 0.8rem;
  }
  nav { display: flex; align-items: center; gap: 0.9rem; }
  .logo {
    font-family: 'Playfair Display', Georgia, serif;
    font-weight: 700;
    font-size: 1.35rem;
    color: var(--text-primary);
    text-decoration: none;
  }
  .logo span {
    background: linear-gradient(100deg, var(--accent-primary), var(--accent-secondary));
    -webkit-background-clip: text;
    background-clip: text;
    color: transparent;
  }
  .center { max-width: 30rem; margin: 20vh auto; text-align: center; }
</style>
