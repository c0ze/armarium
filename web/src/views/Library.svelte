<script lang="ts">
  import { api, coverUrl, type Facet, type Item, type Library, type Series } from '../lib/api';
  import { link, navigate } from '../lib/router.svelte';
  import ItemCard from './ItemCard.svelte';

  let { params }: { params: URLSearchParams } = $props();

  const PAGE = 60;
  let libraries = $state<Library[]>([]);
  let sources = $state<Facet[]>([]);
  let tags = $state<Facet[]>([]);
  let items = $state<Item[]>([]);
  let series = $state<Series[]>([]);
  let total = $state(0);
  let loading = $state(false);
  let error = $state('');

  const library = $derived(Number(params.get('library') ?? 0));
  const q = $derived(params.get('q') ?? '');
  const status = $derived(params.get('status') ?? '');
  const source = $derived(params.get('source') ?? '');
  const tag = $derived(params.get('tag') ?? '');
  const sort = $derived(params.get('sort') ?? '');
  const current = $derived(libraries.find((l) => l.id === library));
  // Comics are browsed by series; filters and search switch to a flat item grid.
  const bySeries = $derived(current?.kind === 'comics' && !q && !status && !source && !tag && !sort);

  let search = $state('');
  $effect(() => { search = q; });

  api.libraries().then((l) => (libraries = l)).catch((e) => (error = e.message));
  $effect(() => {
    api.facets(library || undefined).then((f) => { sources = f.sources; tags = f.tags; }).catch(() => {});
  });

  $effect(() => {
    const filters = { library, q, status, source, tag, sort };
    loading = true;
    error = '';
    const load = bySeries
      ? api.series(library).then((p) => { series = p.items; total = p.total; items = []; })
      : api.items({ ...filters, limit: PAGE }).then((p) => { items = p.items; total = p.total; series = []; });
    load.catch((e) => (error = e.message)).finally(() => (loading = false));
  });

  async function more() {
    loading = true;
    try {
      const p = await api.items({ library, q, status, source, tag, sort, offset: items.length, limit: PAGE });
      items = [...items, ...p.items];
    } finally {
      loading = false;
    }
  }

  function set(key: string, value: string | number) {
    const next = new URLSearchParams(params);
    if (value) next.set(key, String(value));
    else next.delete(key);
    if (key === 'library') { next.delete('source'); next.delete('tag'); }
    navigate(`/?${next}`, key === 'q');
  }

  let timer: ReturnType<typeof setTimeout>;
  function onSearch() {
    clearTimeout(timer);
    timer = setTimeout(() => set('q', search.trim()), 250);
  }
</script>

<main>
  <div class="tabs" role="tablist">
    <button role="tab" aria-selected={!library} class:active={!library} onclick={() => set('library', 0)}>All</button>
    {#each libraries as l (l.id)}
      <button role="tab" aria-selected={library === l.id} class:active={library === l.id} onclick={() => set('library', l.id)}>{l.name}</button>
    {/each}
  </div>

  <div class="filters">
    <input type="search" placeholder="Search titles, authors and series" aria-label="Search" bind:value={search} oninput={onSearch} />
    <select aria-label="Status" value={status} onchange={(e) => set('status', e.currentTarget.value)}>
      <option value="">Any status</option>
      <option value="unread">Unread</option>
      <option value="reading">Reading</option>
      <option value="read">Read</option>
    </select>
    {#if sources.length}
      <select aria-label="Source" value={source} onchange={(e) => set('source', e.currentTarget.value)}>
        <option value="">All sources</option>
        {#each sources as s (s.name)}<option value={s.name}>{s.label || s.name} ({s.count})</option>{/each}
      </select>
    {/if}
    {#if tags.length}
      <select aria-label="Tag" value={tag} onchange={(e) => set('tag', e.currentTarget.value)}>
        <option value="">All tags</option>
        {#each tags as t (t.name)}<option value={t.name}>{t.name} ({t.count})</option>{/each}
      </select>
    {/if}
    <select aria-label="Sort" value={sort} onchange={(e) => set('sort', e.currentTarget.value)}>
      <option value="">Series order</option>
      <option value="title">Title</option>
      <option value="added">Recently added</option>
      <option value="recent">Recently read</option>
    </select>
  </div>

  {#if error}<p class="error">{error}</p>{/if}
  <p class="muted small count">{total} {bySeries ? 'series' : 'items'}{loading ? ' · loading…' : ''}</p>

  {#if bySeries}
    <div class="grid">
      {#each series as s (s.id)}
        <a class="series" href="/series/{s.id}" use:link>
          <div class="cover">
            {#if s.coverItemId}<img src={coverUrl(s.coverItemId)} alt="" loading="lazy" decoding="async" />{/if}
          </div>
          <span class="title">{s.name}</span>
          <span class="muted small">{s.items} items{s.unread ? ` · ${s.unread} unread` : ''}</span>
        </a>
      {/each}
    </div>
  {:else}
    <div class="grid">
      {#each items as it (it.id)}<ItemCard item={it} />{/each}
    </div>
    {#if items.length < total}
      <div class="more"><button onclick={more} disabled={loading}>Load more</button></div>
    {/if}
  {/if}
  {#if !loading && !error && total === 0}
    <p class="muted empty">Nothing here yet. {#if !libraries.length}Add a library to <code>armarium.toml</code>, then scan it from Admin.{/if}</p>
  {/if}
</main>

<style>
  main { max-width: 76rem; margin: 0 auto; }
  .tabs { display: flex; gap: 0.4rem; flex-wrap: wrap; margin-bottom: 0.8rem; }
  .tabs button { border-radius: 999px; padding: 0.35rem 0.9rem; }
  .tabs button.active { background: linear-gradient(100deg, var(--accent-primary), var(--accent-secondary)); color: var(--button-text); border-color: transparent; }
  .filters { display: flex; flex-wrap: wrap; gap: 0.5rem; }
  .filters input { flex: 1 1 16rem; }
  .filters select { flex: 0 1 auto; max-width: 16rem; }
  .count { margin: 0.8rem 0; }
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(8.5rem, 1fr)); gap: 1.2rem 1rem; }
  .series { display: flex; flex-direction: column; gap: 0.3rem; color: inherit; text-decoration: none; min-width: 0; }
  .series .cover {
    aspect-ratio: 2 / 3;
    border-radius: 8px;
    overflow: hidden;
    background: linear-gradient(160deg, var(--bg-gradient-1), var(--bg-gradient-3));
    border: 1px solid var(--surface-border);
    box-shadow: 4px 4px 0 -1px var(--surface-primary), 4px 4px 0 0 var(--surface-border), var(--shadow);
  }
  .series img { width: 100%; height: 100%; object-fit: cover; display: block; }
  .series:hover .cover { outline: 2px solid var(--accent-primary); }
  .title { font-weight: 600; font-size: 0.9rem; line-height: 1.3; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .more { display: flex; justify-content: center; margin-top: 1.5rem; }
  .empty { text-align: center; margin-top: 3rem; }
  @media (max-width: 40rem) {
    .filters select { flex: 1 1 calc(50% - 0.5rem); max-width: none; }
    .grid { grid-template-columns: repeat(auto-fill, minmax(6.5rem, 1fr)); gap: 1rem 0.7rem; }
  }
</style>
