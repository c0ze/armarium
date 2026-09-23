<script lang="ts">
  import { api, type Item, type Series } from '../lib/api';
  import { link } from '../lib/router.svelte';
  import Card from './Card.svelte';
  import Icon from './Icon.svelte';

  let { id }: { id: number } = $props();
  let series = $state<Series | null>(null);
  let items = $state<Item[]>([]);
  let error = $state('');

  $effect(() => {
    api.seriesDetail(id).then((d) => { series = d.series; items = d.items; }).catch((e) => (error = e.message));
  });

  // Carry on with the first item that is not finished.
  const next = $derived(items.find((i) => i.progress.status !== 'read'));
  const read = $derived(items.filter((i) => i.progress.status === 'read').length);
</script>

{#if error}<p class="error">{error}</p>{/if}
{#if series}
  <header class="head">
    <div>
      <h1>{series.name}</h1>
      <p class="caps">{series.items} items · {read} read · {series.unread} to go</p>
    </div>
    {#if next}
      <a class="button primary" href="/item/{next.id}" use:link><Icon name="play" size={14} /> {next.progress.page ? 'Continue' : 'Start'}: {next.title}</a>
    {/if}
  </header>
  <div class="grid">
    {#each items as it (it.id)}<Card item={it} />{/each}
  </div>
{/if}

<style>
  .head { display: flex; justify-content: space-between; align-items: flex-end; gap: 2rem; flex-wrap: wrap; padding: 2.8rem 0 1.6rem; margin-bottom: 1.8rem; border-bottom: 1px solid var(--line); }
  h1 { font-size: clamp(3rem, 5vw, 4.6rem); margin-bottom: 0.7rem; }
  .button { max-width: 28rem; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; display: inline-block; }
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(9.5rem, 1fr)); gap: 1.6rem 1.1rem; padding-bottom: 3rem; }
</style>
