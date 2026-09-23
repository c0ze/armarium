<script lang="ts">
  import { api, coverUrl, fileUrl, formatSize, percent, readable, type Item, type Library } from '../lib/api';
  import { link } from '../lib/router.svelte';

  let { id, panelflowUrl }: { id: number; panelflowUrl: string } = $props();
  let item = $state<Item | null>(null);
  let library = $state<Library | null>(null);
  let error = $state('');
  let busy = $state(false);
  let broken = $state(false);

  $effect(() => {
    api.item(id).then((d) => { item = d.item; library = d.library; }).catch((e) => (error = e.message));
  });

  const comic = $derived(item?.format === 'cbz' || item?.format === 'cbr');
  const pct = $derived(item ? percent(item) : 0);
  // PanelFlow's Armarium mode opens items by id; it holds its own Armarium token.
  const panelflowLink = $derived(panelflowUrl && comic ? `${panelflowUrl.replace(/\/$/, '')}/armarium-read/${id}` : '');

  async function mark(status: 'read' | 'unread') {
    if (!item) return;
    busy = true;
    try {
      const r = await api.progress(id, { status });
      item = { ...item, progress: r.progress };
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = false;
    }
  }
</script>

<main>
  {#if error}<p class="error">{error}</p>{/if}
  {#if item}
    <article class="panel">
      <div class="cover">
        {#if !item.hasCover || broken}
          <div class="placeholder">{item.format.toUpperCase()}</div>
        {:else}
          <img src={coverUrl(item.id)} alt="" onerror={() => (broken = true)} />
        {/if}
      </div>
      <div class="info">
        <a class="small" href="/series/{item.seriesId}" use:link>{item.seriesName}</a>
        <h1>{item.title}</h1>
        {#if item.author}<p class="author">{item.author}</p>{/if}
        <p class="muted small meta">
          {item.format.toUpperCase()} · {formatSize(item.size)}
          {#if item.pages}· {item.pages} {item.format === 'epub' ? 'chapters' : 'pages'}{/if}
          {#if library}· {library.name}{/if}
        </p>
        {#if item.tags.length}
          <p class="tags">{#each item.tags as t (t)}<a class="tag" href="/?tag={encodeURIComponent(t)}" use:link>{t}</a>{/each}</p>
        {/if}
        <p class="status">
          {#if item.progress.status === 'read'}Read
          {:else if item.progress.status === 'reading'}Reading · {pct}%
          {:else}Unread{/if}
        </p>
        <div class="actions">
          {#if item.format === 'epub'}
            <a class="button primary" href="/reader/{item.id}" use:link>{item.progress.page ? 'Continue reading' : 'Read'}</a>
          {:else if item.format === 'pdf'}
            <a class="button primary" href={fileUrl(item.id, true)} target="_blank" rel="noopener">Open PDF</a>
          {:else if readable(item.format)}
            {#if panelflowLink}<a class="button primary" href={panelflowLink} target="_blank" rel="noopener">Open in PanelFlow</a>{/if}
            <a class="button" class:primary={!panelflowLink} href="/viewer/{item.id}" use:link>Quick view</a>
          {/if}
          <a class="button" class:primary={!readable(item.format)} href={fileUrl(item.id)} download>Download {item.format.toUpperCase()}</a>
          {#each item.formats as f (f)}<a class="button" href={fileUrl(item.id, false, f)} download>{f.toUpperCase()}</a>{/each}
          {#if item.progress.status === 'read'}
            <button disabled={busy} onclick={() => mark('unread')}>Mark unread</button>
          {:else}
            <button disabled={busy} onclick={() => mark('read')}>Mark read</button>
          {/if}
        </div>
      </div>
    </article>
  {/if}
</main>

<style>
  main { max-width: 60rem; margin: 1rem auto; }
  article { display: grid; grid-template-columns: minmax(8rem, 14rem) 1fr; gap: 1.5rem; align-items: start; }
  .cover { aspect-ratio: 2 / 3; border-radius: 8px; overflow: hidden; box-shadow: var(--shadow); }
  .cover img { width: 100%; height: 100%; object-fit: cover; display: block; }
  .placeholder { height: 100%; display: grid; place-items: center; font-weight: 700; color: var(--text-secondary);
    background: linear-gradient(160deg, var(--bg-gradient-1), var(--bg-gradient-3)); }
  h1 { font-size: 1.7rem; margin: 0.2rem 0 0.4rem; }
  .meta { margin: 0 0 0.8rem; }
  .author { margin: 0 0 0.3rem; font-weight: 600; color: var(--text-secondary); }
  .tags { display: flex; flex-wrap: wrap; gap: 0.35rem; margin: 0 0 0.8rem; }
  .tag { font-size: 0.78rem; padding: 0.1rem 0.55rem; border-radius: 999px; border: 1px solid var(--surface-border); text-decoration: none; color: var(--text-secondary); }
  .status { font-weight: 600; }
  .actions { display: flex; flex-wrap: wrap; gap: 0.5rem; margin-top: 1rem; }
  @media (max-width: 36rem) { article { grid-template-columns: 1fr; } .cover { max-width: 12rem; } }
</style>
