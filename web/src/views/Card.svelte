<script lang="ts">
  import { percent, type Item, type Series } from '../lib/api';
  import { link } from '../lib/router.svelte';
  import Cover from './Cover.svelte';

  // A cover in a grid: the grid does not reflow on hover, so the card lifts its
  // edge instead of widening.
  let { item, series }: { item?: Item; series?: Series } = $props();
  const pct = $derived(item ? percent(item) : 0);
</script>

<a class="card" href={item ? `/item/${item.id}` : `/series/${series!.id}`} use:link>
  <div class="art">
    {#if item}
      <Cover id={item.id} hasCover={item.hasCover} title={item.title} byline={item.author} format={item.format} />
    {:else if series}
      <Cover id={series.coverItemId} hasCover={series.coverItemId > 0} title={series.name} />
    {/if}
    {#if item && pct > 0 && pct < 100}<span class="bar"><span style:width="{pct}%"></span></span>{/if}
    {#if item?.progress.status === 'read'}<span class="done" title="Read"></span>{/if}
  </div>
  <span class="t">{item ? item.title : series!.name}</span>
  <span class="s">
    {#if item}{item.author || item.seriesName}{:else}{series!.items} items · {series!.unread} unread{/if}
  </span>
</a>

<style>
  .card { display: flex; flex-direction: column; gap: 0.35rem; min-width: 0; }
  .art {
    position: relative;
    aspect-ratio: 2 / 3;
    border-radius: 4px;
    overflow: hidden;
    border: 1px solid transparent;
    background: var(--surface);
    transition: border-color 0.25s var(--ease), transform 0.4s var(--ease), box-shadow 0.4s var(--ease);
  }
  .card:hover .art, .card:focus-visible .art {
    border-color: var(--amber);
    transform: translateY(-3px);
    box-shadow: 0 14px 30px rgba(0, 0, 0, 0.5);
  }
  .card:focus-visible { outline: none; }
  .t { font-size: 13.5px; font-weight: 500; line-height: 1.3; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
  .card:hover .t { color: var(--amber); }
  .s { font-size: 12px; color: var(--text-3); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .bar { position: absolute; left: 0; right: 0; bottom: 0; height: 3px; background: rgba(11, 14, 19, 0.7); }
  .bar span { display: block; height: 100%; background: var(--amber); }
  .done { position: absolute; top: 0.5rem; right: 0.5rem; width: 0.55rem; height: 0.55rem; border-radius: 50%; background: var(--text); box-shadow: 0 1px 4px rgba(0, 0, 0, 0.6); }
  @media (prefers-reduced-motion: reduce) { .art { transition: none; } }
</style>
