<script lang="ts">
  import { api, fileUrl, formatSize, percent, readable, seriesOf, type Item, type Library } from '../lib/api';
  import { link } from '../lib/router.svelte';
  import Cover from './Cover.svelte';
  import Icon from './Icon.svelte';

  let { id, panelflowUrl }: { id: number; panelflowUrl: string } = $props();
  let item = $state<Item | null>(null);
  let library = $state<Library | null>(null);
  let error = $state('');
  let busy = $state(false);

  $effect(() => {
    api.item(id).then((d) => { item = d.item; library = d.library; }).catch((e) => (error = e.message));
  });

  const comic = $derived(item?.format === 'cbz' || item?.format === 'cbr');
  const pct = $derived(item ? percent(item) : 0);
  // PanelFlow's Armarium mode opens items by id; it holds its own Armarium token.
  const panelflowLink = $derived(panelflowUrl && comic ? `${panelflowUrl.replace(/\/$/, '')}/armarium-read/${id}` : '');
  const unit = $derived(item?.format === 'epub' ? 'chapters' : 'pages');

  async function mark(status: 'read' | 'unread') {
    if (!item) return;
    busy = true;
    try {
      item = { ...item, progress: (await api.progress(id, { status })).progress };
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = false;
    }
  }
</script>

{#if error}<p class="error">{error}</p>{/if}
{#if item}
  <article class="item">
    <div class="art">
      <Cover id={item.id} hasCover={item.hasCover} title={item.title} byline={item.author} format={item.format} natural />
    </div>
    <div class="body">
      <h1>{item.title}</h1>
      {#if item.author}<p class="author">{item.author}</p>{/if}
      <p class="where">
        {#if seriesOf(item)}In <a href="/series/{item.seriesId}" use:link>{item.seriesName}</a>{', '}{:else}More by <a href="/series/{item.seriesId}" use:link>{item.author}</a>{' in '}{/if}{library?.name ?? ''}
      </p>
      <p class="meta">
        {item.format.toUpperCase()}<span>·</span>{formatSize(item.size)}{#if item.pages}<span>·</span>{item.pages} {unit}{/if}{#if item.number !== null && comic}<span>·</span>No. {item.number}{/if}
      </p>

      <div class="state">
        {#if item.progress.status === 'read'}
          <span class="caps">Read</span>
        {:else if item.progress.status === 'reading'}
          <span class="track"><span style:width="{pct}%"></span></span>
          <span class="caps">{pct}% · {item.format === 'epub' ? 'chapter' : 'page'} {item.progress.page} of {item.pages}</span>
        {:else}
          <span class="caps">Unread</span>
        {/if}
      </div>

      <div class="actions">
        {#if item.format !== 'pdf' && readable(item.format) && !item.pages}
          <p class="muted broken">This file could not be opened; it can still be downloaded.</p>
        {:else if item.format === 'epub'}
          <a class="button primary" href="/reader/{item.id}" use:link><Icon name="play" size={14} /> {item.progress.page ? 'Resume' : 'Read'}</a>
        {:else if item.format === 'pdf'}
          <a class="button primary" href={fileUrl(item.id, true)} target="_blank" rel="noopener"><Icon name="play" size={14} /> Open PDF</a>
        {:else if readable(item.format)}
          {#if panelflowLink}<a class="button primary" href={panelflowLink} target="_blank" rel="noopener"><Icon name="play" size={14} /> Open in PanelFlow</a>{/if}
          <a class="button" class:primary={!panelflowLink} href="/viewer/{item.id}" use:link>{panelflowLink ? 'Quick view' : 'Read'}</a>
        {/if}
        {#if item.progress.status === 'read'}
          <button disabled={busy} onclick={() => mark('unread')}>Mark unread</button>
        {:else}
          <button disabled={busy} onclick={() => mark('read')}><Icon name="check" size={14} /> Mark read</button>
        {/if}
      </div>

      <div class="files">
        <p class="caps">Download</p>
        <a class="button" href={fileUrl(item.id)} download><Icon name="download" size={14} /> {item.format}</a>
        {#each item.formats as f (f)}
          <a class="button" href={fileUrl(item.id, false, f)} download><Icon name="download" size={14} /> {f}</a>
        {/each}
      </div>

      {#if item.tags.length}
        <p class="tags">{#each item.tags as t (t)}<a href="/?tag={encodeURIComponent(t)}" use:link>{t}</a>{/each}</p>
      {/if}
    </div>
  </article>
{/if}

<style>
  .broken { margin: 0; font-size: 14px; }
  .item { display: grid; grid-template-columns: clamp(12rem, 24vw, 20rem) minmax(0, 1fr); gap: clamp(2rem, 4vw, 4rem); padding: 3rem 0 4rem; align-items: start; }
  .art:has(:global(.plate)) { aspect-ratio: 2 / 3; }
  .art { border-radius: 4px; overflow: hidden; box-shadow: 0 24px 60px rgba(0, 0, 0, 0.6), 0 2px 6px rgba(0, 0, 0, 0.5); }
  .body { display: flex; flex-direction: column; gap: 1.1rem; min-width: 0; }
  .where { margin: -0.4rem 0 0; color: var(--text-3); font-size: 13px; }
  .where a { color: var(--text-2); text-decoration: underline; text-underline-offset: 3px; text-decoration-color: var(--line); }
  .where a:hover { color: var(--amber); text-decoration-color: var(--amber); }
  h1 { font-size: clamp(3rem, 5.5vw, 5.2rem); max-width: 18ch; }
  .author { margin: -0.3rem 0 0; font-size: 1.15rem; color: var(--text-2); }
  .meta { margin: 0; color: var(--text-3); font-size: 13px; letter-spacing: 0.04em; }
  .meta span { margin: 0 0.55rem; }
  .state { display: flex; align-items: center; gap: 1rem; max-width: 30rem; }
  .state .track { flex: 1; height: 3px; background: var(--raised); }
  .state .track span { display: block; height: 100%; background: var(--amber); }
  .actions, .files { display: flex; flex-wrap: wrap; gap: 0.7rem; align-items: center; }
  .actions { margin-top: 0.4rem; }
  .files { padding-top: 1.2rem; border-top: 1px solid var(--line); margin-top: 0.6rem; }
  .files .caps { margin: 0 0.6rem 0 0; }
  .tags { display: flex; flex-wrap: wrap; gap: 0.45rem; margin: 0.2rem 0 0; }
  .tags a { font-size: 12px; padding: 0.3rem 0.65rem; border: 1px solid var(--line); border-radius: 3px; color: var(--text-2); }
  .tags a:hover { border-color: var(--amber); color: var(--amber); }
  @media (max-width: 700px) { .item { grid-template-columns: 1fr; } .art { max-width: 13rem; } }
</style>
