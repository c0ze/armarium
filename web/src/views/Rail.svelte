<script lang="ts">
  import type { Item, Series } from '../lib/api';
  import { link } from '../lib/router.svelte';
  import Icon from './Icon.svelte';
  import Tile from './Tile.svelte';

  // One row of the wall. Loads when it first scrolls near the viewport, so a
  // wall of twenty libraries costs two requests up front, not twenty.
  let { title, more = '', load, eager = false }: {
    title: string;
    more?: string;
    load: () => Promise<{ items?: Item[]; series?: Series[] }>;
    eager?: boolean;
  } = $props();

  let items = $state<Item[]>([]);
  let series = $state<Series[]>([]);
  let phase = $state<'idle' | 'loading' | 'ready' | 'error'>('idle');
  let row = $state<HTMLElement>();
  let track = $state<HTMLElement>();

  function start() {
    if (phase !== 'idle') return;
    phase = 'loading';
    load()
      .then((r) => { items = r.items ?? []; series = r.series ?? []; phase = 'ready'; })
      .catch(() => (phase = 'error'));
  }

  $effect(() => {
    if (eager) return start();
    if (!row) return;
    const io = new IntersectionObserver((e) => { if (e.some((x) => x.isIntersecting)) { start(); io.disconnect(); } }, { rootMargin: '600px 0px' });
    io.observe(row);
    return () => io.disconnect();
  });

  function scroll(dir: number) {
    track?.scrollBy({ left: dir * track.clientWidth * 0.8, behavior: 'smooth' });
  }

  const empty = $derived(phase === 'ready' && items.length === 0 && series.length === 0);
</script>

{#if !empty}
  <section class="rail" bind:this={row}>
    <header>
      <h2>{title}</h2>
      {#if more}<a class="more caps" href={more} use:link>View all <Icon name="right" size={12} /></a>{/if}
      <span class="nav">
        <button class="arrow" aria-label="Scroll {title} left" onclick={() => scroll(-1)}><Icon name="left" size={16} /></button>
        <button class="arrow" aria-label="Scroll {title} right" onclick={() => scroll(1)}><Icon name="right" size={16} /></button>
      </span>
    </header>
    <div class="track" bind:this={track}>
      {#if phase === 'ready'}
        {#each series as s (s.id)}<Tile series={s} />{/each}
        {#each items as it (it.id)}<Tile item={it} />{/each}
      {:else if phase === 'error'}
        <p class="muted">Could not load this row.</p>
      {:else}
        {#each { length: 8 } as _, i (i)}<span class="ghost"></span>{/each}
      {/if}
    </div>
  </section>
{/if}

<style>
  .rail { transition: opacity 0.4s var(--ease), filter 0.4s var(--ease); }
  header { display: flex; align-items: baseline; gap: 1.25rem; margin: 0 0 0.9rem; }
  h2 { font-size: 2rem; }
  .more { display: inline-flex; align-items: center; gap: 0.35rem; white-space: nowrap; }
  h2 { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }
  .more:hover { color: var(--amber); }
  .nav { margin-left: auto; display: flex; gap: 0.5rem; align-self: center; }
  .arrow { width: 2.2rem; height: 2.2rem; padding: 0; justify-content: center; border-radius: 50%; border-color: var(--line); }
  .track {
    display: flex;
    gap: 0.7rem;
    overflow-x: auto;
    overscroll-behavior-x: contain;
    scroll-snap-type: x proximity;
    scrollbar-width: none;
    padding: 0.35rem 0.1rem 1rem;
  }
  .track::-webkit-scrollbar { display: none; }
  .ghost { flex: 0 0 var(--tile-w); height: calc(var(--tile-w) * 1.5); border-radius: 4px; background: var(--surface); }
  @media (hover: none), (max-width: 700px) { .nav { display: none; } }
  @media (max-width: 700px) { h2 { font-size: 1.7rem; } header { gap: 0.9rem; } }
</style>
