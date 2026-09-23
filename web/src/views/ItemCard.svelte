<script lang="ts">
  import { coverUrl, percent, type Item } from '../lib/api';
  import { link } from '../lib/router.svelte';

  let { item }: { item: Item } = $props();
  let broken = $state(false);
  const pct = $derived(percent(item));
</script>

<a class="card" href="/item/{item.id}" use:link title={item.title}>
  <div class="cover">
    {#if !item.hasCover || broken}
      <div class="placeholder"><span>{item.format.toUpperCase()}</span></div>
    {:else}
      <img src={coverUrl(item.id)} alt="" loading="lazy" decoding="async" onerror={() => (broken = true)} />
    {/if}
    {#if item.progress.status === 'read'}<span class="badge">Read</span>{/if}
    {#if pct > 0 && pct < 100}<progress max="100" value={pct}></progress>{/if}
  </div>
  <span class="title">{item.title}</span>
  <span class="muted small sub">{item.author || item.seriesName}</span>
</a>

<style>
  .card { display: flex; flex-direction: column; gap: 0.3rem; color: inherit; text-decoration: none; min-width: 0; }
  .cover {
    position: relative;
    aspect-ratio: 2 / 3;
    border-radius: 8px;
    overflow: hidden;
    background: var(--surface-primary);
    border: 1px solid var(--surface-border);
    box-shadow: var(--shadow);
  }
  img { width: 100%; height: 100%; object-fit: cover; display: block; }
  .placeholder {
    height: 100%;
    display: grid;
    place-items: center;
    background: linear-gradient(160deg, var(--bg-gradient-1), var(--bg-gradient-3));
    font-weight: 700;
    color: var(--text-secondary);
  }
  .badge {
    position: absolute;
    top: 0.4rem;
    right: 0.4rem;
    font-size: 0.7rem;
    font-weight: 700;
    padding: 0.1rem 0.45rem;
    border-radius: 999px;
    background: var(--accent-secondary);
    color: var(--button-text);
  }
  progress {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    width: 100%;
    height: 4px;
    appearance: none;
    border: none;
    background: rgba(0, 0, 0, 0.25);
  }
  progress::-webkit-progress-bar { background: rgba(0, 0, 0, 0.25); }
  progress::-webkit-progress-value { background: var(--accent-primary); }
  progress::-moz-progress-bar { background: var(--accent-primary); }
  .title {
    font-weight: 600;
    font-size: 0.9rem;
    line-height: 1.3;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .sub { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .card:hover .cover { outline: 2px solid var(--accent-primary); }
</style>
