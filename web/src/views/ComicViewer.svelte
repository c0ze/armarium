<script lang="ts">
  import { onDestroy } from 'svelte';
  import { api, pageUrl, type Item } from '../lib/api';
  import { link } from '../lib/router.svelte';
  import Icon from './Icon.svelte';

  // A plain page viewer for when PanelFlow isn't at hand. Progress uses the same
  // monotonic endpoint PanelFlow does, so both stay in step.
  let { id }: { id: number } = $props();
  let item = $state<Item | null>(null);
  let page = $state(1);
  let fit = $state<'height' | 'width'>((localStorage.getItem('armarium.fit') as 'height' | 'width') ?? 'height');
  let error = $state('');
  let saveTimer: ReturnType<typeof setTimeout>;

  $effect(() => {
    api.item(id).then((d) => {
      item = d.item;
      page = Math.min(Math.max(d.item.progress.page, 1), Math.max(d.item.pages, 1));
    }).catch((e) => (error = e.message));
  });

  $effect(() => {
    if (!item) return;
    for (const n of [page + 1, page + 2]) if (n <= item.pages) new Image().src = pageUrl(id, n);
  });

  function go(n: number) {
    if (!item || n < 1 || n > item.pages) return;
    page = n;
    clearTimeout(saveTimer);
    saveTimer = setTimeout(save, 800);
  }

  function save() {
    if (item) api.progress(id, { page }).catch(() => {});
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'ArrowRight' || e.key === ' ' || e.key === 'PageDown') { e.preventDefault(); go(page + 1); }
    if (e.key === 'ArrowLeft' || e.key === 'PageUp') { e.preventDefault(); go(page - 1); }
  }

  function onClick(e: MouseEvent) {
    const x = e.clientX / (e.currentTarget as HTMLElement).clientWidth;
    go(x < 0.35 ? page - 1 : page + 1);
  }

  function toggleFit() {
    fit = fit === 'height' ? 'width' : 'height';
    localStorage.setItem('armarium.fit', fit);
  }

  onDestroy(() => { clearTimeout(saveTimer); save(); });
</script>

<svelte:window onkeydown={onKey} />

<div class="viewer">
  <header>
    <a class="back" href="/item/{id}" use:link aria-label="Back to the item"><Icon name="back" /></a>
    <span class="title">{item?.title ?? ''}</span>
    <span class="caps">{item ? `${page} / ${item.pages}` : ''}</span>
    <button class="quiet" onclick={toggleFit}>Fit {fit === 'height' ? 'width' : 'height'}</button>
  </header>
  {#if error}<p class="error">{error}</p>{/if}
  {#if item && item.format !== 'cbz' && item.format !== 'cbr'}
    <p class="notice">This viewer is for comics. <a href="/item/{id}" use:link>Back to the item</a></p>
  {:else if item}
    <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
    <div class="stage" class:width={fit === 'width'} onclick={onClick}>
      <img src={pageUrl(id, page)} alt="Page {page}" />
    </div>
    <input type="range" min="1" max={item.pages} value={page} aria-label="Page" oninput={(e) => go(Number(e.currentTarget.value))} />
  {/if}
</div>

<style>
  .viewer { height: 100dvh; display: flex; flex-direction: column; background: #050608; color: var(--text); }
  header { display: flex; align-items: center; gap: 0.8rem; padding: 0.55rem 1rem; border-bottom: 1px solid var(--line); background: var(--ground); }
  .back { display: inline-flex; padding: 0.4rem; color: var(--text-2); }
  .back:hover { color: var(--amber); }
  .title { flex: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; font-family: var(--display); font-size: 1.5rem; letter-spacing: 0.03em; text-transform: uppercase; }
  .stage { flex: 1; min-height: 0; display: flex; justify-content: center; align-items: center; overflow: auto; cursor: pointer; user-select: none; }
  .stage img { max-height: 100%; max-width: 100%; object-fit: contain; }
  .stage.width { align-items: flex-start; }
  .stage.width img { max-height: none; width: min(100%, 60rem); }
  .notice { text-align: center; margin-top: 30vh; }
  .notice a { color: var(--amber); }
  input[type='range'] { margin: 0.6rem 1.2rem 0.9rem; padding: 0; border: 0; background: transparent; accent-color: var(--amber); }
</style>
