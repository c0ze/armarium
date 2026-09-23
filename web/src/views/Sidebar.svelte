<script lang="ts">
  import { api, type Library } from '../lib/api';
  import { link, navigate, router } from '../lib/router.svelte';
  import Icon from './Icon.svelte';

  let { libraries, admin, onLogout }: { libraries: Library[]; admin: boolean; onLogout: () => void } = $props();

  let counts = $state<Record<string, number>>({});
  $effect(() => {
    for (const s of ['reading', 'unread', 'read'] as const) {
      api.items({ status: s, limit: 1 }).then((p) => (counts = { ...counts, [s]: p.total })).catch(() => {});
    }
  });

  let q = $state('');
  function search(e: SubmitEvent) {
    e.preventDefault();
    if (q.trim()) navigate(`/?q=${encodeURIComponent(q.trim())}`);
  }

  const route = $derived(router.route);
  const onWall = $derived(route.name === 'library' && [...route.params.keys()].length === 0);
  const libActive = (id: number) => route.name === 'library' && route.params.get('library') === String(id);
  const statusActive = (s: string) => route.name === 'library' && route.params.get('status') === s && !route.params.get('library');
  const groups = $derived([
    { label: 'Comics', icon: 'comic', libs: libraries.filter((l) => l.kind === 'comics') },
    { label: 'Books', icon: 'book', libs: libraries.filter((l) => l.kind === 'books') },
  ]);
</script>

<nav class="side" aria-label="Library">
  <a class="mark" href="/" use:link>Armarium</a>

  <form class="search" onsubmit={search} role="search">
    <Icon name="search" size={16} />
    <input type="search" placeholder="Search titles, authors" aria-label="Search the collection" bind:value={q} />
  </form>

  <a class="nav" class:on={onWall} href="/" use:link><Icon name="wall" /><span>The wall</span></a>

  <div class="libs">
    {#each groups as g (g.label)}
      {#if g.libs.length}
        <p class="caps head">{g.label}</p>
        {#each g.libs as l (l.id)}
          <a class="lib" class:on={libActive(l.id)} href="/?library={l.id}" use:link>{l.name}</a>
        {/each}
      {/if}
    {/each}
  </div>

  <div class="status">
    <p class="caps head">Reading status</p>
    {#each [['reading', 'Reading', 'clock'], ['unread', 'Unread', 'circle'], ['read', 'Read', 'check']] as [key, label, icon] (key)}
      <a class="stat" class:on={statusActive(key)} href="/?status={key}&sort={key === 'unread' ? 'added' : 'recent'}" use:link>
        <Icon name={icon} size={16} /><span>{label}</span><span class="n">{counts[key]?.toLocaleString() ?? '–'}</span>
      </a>
    {/each}
  </div>

  <div class="foot">
    {#if admin}<a class="nav" class:on={route.name === 'admin'} href="/admin" use:link><Icon name="admin" /><span>Admin</span></a>{/if}
    {#if admin}<button class="nav out" onclick={onLogout}><Icon name="logout" /><span>Log out</span></button>{/if}
  </div>
</nav>

<style>
  .side {
    position: sticky;
    top: 0;
    height: 100dvh;
    width: var(--rail-w);
    flex: 0 0 var(--rail-w);
    display: flex;
    flex-direction: column;
    gap: 1.4rem;
    padding: 1.8rem 1.1rem 1.2rem;
    border-right: 1px solid var(--line);
    background: var(--ground);
    overflow-y: auto;
    scrollbar-width: thin;
  }
  .mark { font-family: var(--display); font-size: 2.3rem; letter-spacing: 0.08em; line-height: 1; padding: 0 0.4rem; }
  .search { display: flex; align-items: center; gap: 0.55rem; padding: 0 0.7rem; border: 1px solid var(--line); border-radius: 3px; color: var(--text-3); }
  .search:focus-within { border-color: var(--amber); color: var(--amber); }
  .search input { border: 0; padding: 0.65rem 0; background: transparent; flex: 1; }
  .nav, .lib, .stat {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.5rem 0.6rem;
    border-radius: 3px;
    color: var(--text-2);
    font: 500 12px/1.2 var(--ui);
    letter-spacing: 0.12em;
    text-transform: uppercase;
    border: 0;
    width: 100%;
  }
  .lib { letter-spacing: 0.06em; text-transform: none; font-size: 13.5px; font-weight: 400; padding-block: 0.36rem; }
  .nav:hover, .lib:hover, .stat:hover { color: var(--text); background: var(--surface); }
  .on, .on:hover { color: var(--amber); background: var(--surface); }
  .libs { display: flex; flex-direction: column; gap: 0.05rem; }
  .head { margin: 0.9rem 0 0.35rem 0.6rem; color: var(--text-3); }
  .libs .head:first-child { margin-top: 0; }
  .status { display: flex; flex-direction: column; gap: 0.1rem; padding-top: 1.2rem; border-top: 1px solid var(--line); }
  .status .head { margin-top: 0; }
  .n { margin-left: auto; color: var(--text-3); letter-spacing: 0.02em; }
  .foot { margin-top: auto; display: flex; flex-direction: column; gap: 0.1rem; }
  .out { background: none; justify-content: flex-start; }
  .out:hover { border: 0; }

  @media (max-width: 900px) {
    .side {
      position: fixed;
      inset: auto 0 0 0;
      top: auto;
      height: auto;
      width: 100%;
      flex-direction: row;
      align-items: center;
      padding: 0.5rem 0.8rem calc(0.5rem + env(safe-area-inset-bottom));
      border-right: 0;
      border-top: 1px solid var(--line);
      z-index: 10;
      overflow: visible;
    }
    .mark, .libs, .status, .foot .out, .nav span { display: none; }
    .search { flex: 1; }
    .nav { width: auto; padding: 0.55rem; }
    .foot { margin: 0; flex-direction: row; }
  }
</style>
