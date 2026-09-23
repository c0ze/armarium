<script lang="ts">
  import { api, type Item, type Series } from '../lib/api';
  import ItemCard from './ItemCard.svelte';

  let { id }: { id: number } = $props();
  let series = $state<Series | null>(null);
  let items = $state<Item[]>([]);
  let error = $state('');

  $effect(() => {
    api.seriesDetail(id).then((d) => { series = d.series; items = d.items; }).catch((e) => (error = e.message));
  });

  // Continue with the first item that is not finished.
  const next = $derived(items.find((i) => i.progress.status !== 'read'));
</script>

<main>
  {#if error}<p class="error">{error}</p>{/if}
  {#if series}
    <div class="head">
      <div>
        <h1>{series.name}</h1>
        <p class="muted small">{series.items} items · {series.unread} unread</p>
      </div>
      {#if next}<a class="button primary" href="/item/{next.id}">Continue: {next.title}</a>{/if}
    </div>
    <div class="grid">
      {#each items as it (it.id)}<ItemCard item={it} />{/each}
    </div>
  {/if}
</main>

<style>
  main { max-width: 76rem; margin: 0 auto; }
  .head { display: flex; justify-content: space-between; align-items: end; gap: 1rem; flex-wrap: wrap; margin: 0.5rem 0 1.2rem; }
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(8.5rem, 1fr)); gap: 1.2rem 1rem; }
  @media (max-width: 40rem) { .grid { grid-template-columns: repeat(auto-fill, minmax(6.5rem, 1fr)); } }
</style>
