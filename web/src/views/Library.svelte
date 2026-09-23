<script lang="ts">
  import { api, type Facet, type Item, type Library, type Series } from '../lib/api';
  import { navigate } from '../lib/router.svelte';
  import Card from './Card.svelte';

  let { params, libraries }: { params: URLSearchParams; libraries: Library[] } = $props();

  const PAGE = 60;
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
  // Comics libraries open on their series; any filter drops to single items.
  const bySeries = $derived(current?.kind === 'comics' && !q && !status && !source && !tag && !sort);
  const heading = $derived(
    current?.name ?? (q ? `“${q}”` : status ? { reading: 'Reading', unread: 'Unread', read: 'Read' }[status] ?? status
      : sort === 'added' ? 'Recently added' : 'Everything'),
  );

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
    navigate(`/?${next}`, true);
  }
</script>

<header class="head">
  <h1>{heading}</h1>
  <p class="count caps">{total.toLocaleString()} {bySeries ? 'series' : total === 1 ? 'item' : 'items'}{loading ? ' · loading' : ''}</p>
</header>

<div class="tools">
  <div class="seg" role="group" aria-label="Reading status">
    {#each [['', 'All'], ['unread', 'Unread'], ['reading', 'Reading'], ['read', 'Read']] as [v, label] (v)}
      <button class:on={status === v} aria-pressed={status === v} onclick={() => set('status', v)}>{label}</button>
    {/each}
  </div>
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

<div class="grid">
  {#each series as s (s.id)}<Card series={s} />{/each}
  {#each items as it (it.id)}<Card item={it} />{/each}
</div>

{#if !bySeries && items.length < total}
  <div class="more"><button onclick={more} disabled={loading}>Load {Math.min(PAGE, total - items.length)} more</button></div>
{/if}
{#if !loading && !error && total === 0}
  <p class="empty muted">Nothing here. {#if !libraries.length}Add a library to <code>armarium.toml</code> and scan it from Admin.{/if}</p>
{/if}

<style>
  .head { display: flex; align-items: baseline; gap: 1.5rem; flex-wrap: wrap; padding: 2.8rem 0 1.4rem; }
  h1 { font-size: clamp(3rem, 5vw, 4.6rem); }
  .tools { display: flex; flex-wrap: wrap; gap: 0.6rem; align-items: center; padding-bottom: 1.6rem; border-bottom: 1px solid var(--line); margin-bottom: 1.8rem; }
  .seg { display: inline-flex; border: 1px solid var(--line); border-radius: 3px; overflow: hidden; }
  .seg button { border: 0; border-radius: 0; padding: 0.72rem 1rem; color: var(--text-2); }
  .seg button + button { border-left: 1px solid var(--line); }
  .seg button.on { color: var(--text); background: var(--raised); }
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(9.5rem, 1fr)); gap: 1.6rem 1.1rem; }
  .more { display: flex; justify-content: center; margin: 2.4rem 0 3rem; }
  .empty { margin: 4rem 0; text-align: center; }
  @media (max-width: 600px) { .grid { grid-template-columns: repeat(auto-fill, minmax(7rem, 1fr)); gap: 1.2rem 0.8rem; } }
</style>
