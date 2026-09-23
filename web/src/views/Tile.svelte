<script lang="ts">
  import { percent, type Item, type Series } from '../lib/api';
  import { link } from '../lib/router.svelte';
  import Cover from './Cover.svelte';

  // A cover on the wall. Hover or keyboard focus widens it into a card that says
  // what it is; its neighbours slide aside.
  let { item, series }: { item?: Item; series?: Series } = $props();

  const href = $derived(item ? `/item/${item.id}` : `/series/${series!.id}`);
  const title = $derived(item ? item.title : series!.name);
  const pct = $derived(item ? percent(item) : 0);
  const meta = $derived.by(() => {
    if (series) return `${series.items} ${series.items === 1 ? 'item' : 'items'} · ${series.unread} unread`;
    const it = item!;
    const bits = [it.format.toUpperCase()];
    if (it.pages) bits.push(`${it.pages} ${it.format === 'epub' ? 'ch.' : 'pp.'}`);
    if (it.number !== null && it.format !== 'epub') bits.push(`#${it.number}`);
    return bits.join(' · ');
  });
  const byline = $derived(item ? item.author || item.seriesName : '');
  const status = $derived(
    !item ? '' : item.progress.status === 'read' ? 'Read' : item.progress.status === 'reading' ? `Reading · ${pct}%` : '',
  );
</script>

<a class="tile" {href} use:link aria-label={title}>
  <div class="art">
    {#if item}
      <Cover id={item.id} hasCover={item.hasCover} {title} byline={item.author} format={item.format} />
    {:else if series}
      <Cover id={series.coverItemId} hasCover={series.coverItemId > 0} {title} />
    {/if}
    {#if item && pct > 0 && pct < 100}<span class="bar"><span style:width="{pct}%"></span></span>{/if}
    {#if item?.progress.status === 'read'}<span class="done" title="Read"></span>{/if}
  </div>
  <div class="info">
    <h3>{title}</h3>
    {#if byline}<p class="by">{byline}</p>{/if}
    <p class="meta">{meta}</p>
    {#if status}<p class="status">{status}</p>{/if}
  </div>
</a>

<style>
  .tile {
    position: relative;
    flex: 0 0 var(--tile-w);
    height: calc(var(--tile-w) * 1.5);
    display: flex;
    border: 1px solid transparent;
    border-radius: 4px;
    background: var(--surface);
    overflow: hidden;
    scroll-snap-align: start;
    transition: flex-basis 0.5s var(--ease), border-color 0.3s var(--ease), box-shadow 0.5s var(--ease);
  }
  .art { position: relative; flex: 0 0 var(--tile-w); height: 100%; }
  .info {
    flex: 1;
    min-width: 0;
    padding: 1.1rem 1.1rem 1rem;
    display: flex;
    flex-direction: column;
    gap: 0.45rem;
    opacity: 0;
    transition: opacity 0.2s ease;
  }
  h3 { font-size: 1.9rem; display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
  .by { margin: 0; color: var(--text-2); font-size: 13px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .meta { margin: 0; color: var(--text-3); font-size: 12px; letter-spacing: 0.04em; }
  .status { margin: auto 0 0; font: 500 11px/1 var(--ui); letter-spacing: 0.16em; text-transform: uppercase; color: var(--amber); }
  .bar { position: absolute; left: 0; right: 0; bottom: 0; height: 3px; background: rgba(11, 14, 19, 0.7); }
  .bar span { display: block; height: 100%; background: var(--amber); }
  .done {
    position: absolute;
    top: 0.5rem;
    right: 0.5rem;
    width: 0.55rem;
    height: 0.55rem;
    border-radius: 50%;
    background: var(--text);
    box-shadow: 0 1px 4px rgba(0, 0, 0, 0.6);
  }
  @media (hover: hover) {
    .tile:hover, .tile:focus-visible {
      flex-basis: var(--tile-open);
      border-color: var(--amber);
      box-shadow: 0 12px 32px rgba(0, 0, 0, 0.55);
      outline: none;
    }
    .tile:hover .info, .tile:focus-visible .info { opacity: 1; transition-delay: 0.15s; }
  }
  @media (prefers-reduced-motion: reduce) {
    .tile, .info { transition: none; }
  }
</style>
