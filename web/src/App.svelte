<script lang="ts">
  import { api, ApiError, type Library, type Session } from './lib/api';
  import { router, navigate, link } from './lib/router.svelte';
  import Login from './views/Login.svelte';
  import Sidebar from './views/Sidebar.svelte';
  import Wall from './views/Wall.svelte';
  import LibraryView from './views/Library.svelte';
  import SeriesView from './views/Series.svelte';
  import ItemView from './views/Item.svelte';
  import BookReader from './views/BookReader.svelte';
  import ComicViewer from './views/ComicViewer.svelte';
  import Admin from './views/Admin.svelte';

  let session = $state<Session | null>(null);
  let libraries = $state<Library[]>([]);
  let failure = $state('');

  async function refresh() {
    try {
      session = await api.session();
      if (session.authenticated) libraries = await api.libraries();
    } catch (e) {
      failure = e instanceof ApiError ? e.message : 'Armarium is unreachable.';
    }
  }
  refresh();

  const route = $derived(router.route);
  const bare = $derived(route.name === 'reader' || route.name === 'viewer' || route.name === 'login');
  // The wall is the library view with no filters at all.
  const wall = $derived(route.name === 'library' && [...route.params.keys()].length === 0);

  $effect(() => {
    if (session && !session.authenticated && route.name !== 'login') navigate('/login', true);
  });

  async function logout() {
    await api.logout();
    await refresh();
  }
</script>

{#if failure}
  <main class="center"><h1>Unreachable</h1><p class="error">{failure}</p></main>
{:else if session}
  {#if route.name === 'login'}
    <Login {session} onLogin={async () => { await refresh(); navigate('/', true); }} />
  {:else if !session.authenticated}
    <p class="center muted">Redirecting to login…</p>
  {:else if bare}
    {#if route.name === 'reader'}
      {#key route.id}<BookReader id={route.id} />{/key}
    {:else}
      {#key route.id}<ComicViewer id={route.id} />{/key}
    {/if}
  {:else}
    <div class="shell">
      <Sidebar {libraries} admin={session.admin} onLogout={logout} />
      <main class="main">
        {#if wall}
          <Wall {libraries} />
        {:else if route.name === 'library'}
          <LibraryView params={route.params} {libraries} />
        {:else if route.name === 'series'}
          {#key route.id}<SeriesView id={route.id} />{/key}
        {:else if route.name === 'item'}
          {#key route.id}<ItemView id={route.id} panelflowUrl={session.panelflowUrl} />{/key}
        {:else if route.name === 'admin'}
          <Admin admin={session.admin} />
        {:else}
          <section class="center"><h1>Not on the shelf</h1><p><a class="button" href="/" use:link>Back to the wall</a></p></section>
        {/if}
      </main>
    </div>
  {/if}
{/if}

<style>
  .shell { display: flex; min-height: 100dvh; }
  .main { flex: 1; min-width: 0; padding: 0 clamp(1.2rem, 3vw, 3rem); }
  .center { max-width: 30rem; margin: 20vh auto; text-align: center; display: grid; gap: 1.2rem; justify-items: center; }
  .center h1 { font-size: 3.5rem; }
  @media (max-width: 900px) { .main { padding-bottom: 5rem; } }
</style>
