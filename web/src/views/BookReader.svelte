<script lang="ts">
  import { onDestroy } from 'svelte';
  import { api, chapterUrl, type Item, type TocEntry } from '../lib/api';
  import { link } from '../lib/router.svelte';
  import { chapterFromPath, parseLocator, progressFor } from '../lib/reading';

  let { id }: { id: number } = $props();
  let item = $state<Item | null>(null);
  let toc = $state<TocEntry[]>([]);
  let chapters = $state(0);
  let chapter = $state(0);
  let showToc = $state(false);
  let error = $state('');
  let frame = $state<HTMLIFrameElement>();
  let fontSize = $state(Number(localStorage.getItem('armarium.fontSize') ?? 1.1));
  let dark = $state(localStorage.getItem('armarium.dark') === '1' || (localStorage.getItem('armarium.dark') === null && matchMedia('(prefers-color-scheme: dark)').matches));
  let restoreScroll = 0;
  let saveTimer: ReturnType<typeof setTimeout>;
  let fraction = 0;

  $effect(() => {
    Promise.all([api.item(id), api.toc(id)])
      .then(([d, t]) => {
        item = d.item;
        toc = t.toc;
        chapters = t.chapters;
        const loc = parseLocator(d.item.progress.locator);
        chapter = Math.min(loc?.chapter ?? Math.max(d.item.progress.page - 1, 0), Math.max(chapters - 1, 0));
        restoreScroll = loc?.scroll ?? 0;
      })
      .catch((e) => (error = e.message));
  });

  const src = $derived(item ? chapterUrl(id, chapter) : '');
  const title = $derived(toc.findLast((t) => t.chapter <= chapter)?.title ?? '');

  // The iframe is sandboxed without scripts; same-origin lets us style it and
  // read its scroll position from here.
  function onLoad() {
    const win = frame?.contentWindow;
    const doc = frame?.contentDocument;
    if (!win || !doc) return;
    const landed = chapterFromPath(win.location.pathname, id);
    if (landed !== null && landed !== chapter) chapter = landed; // followed an in-book link
    applyTheme();
    if (restoreScroll) {
      win.scrollTo(0, restoreScroll * (doc.documentElement.scrollHeight - win.innerHeight));
      restoreScroll = 0;
    }
    win.addEventListener('scroll', () => {
      const max = doc.documentElement.scrollHeight - win.innerHeight;
      fraction = max > 0 ? win.scrollY / max : 1;
      scheduleSave();
    }, { passive: true });
    doc.addEventListener('keydown', onKey);
    fraction = 0;
    scheduleSave();
  }

  function applyTheme() {
    const root = frame?.contentDocument?.documentElement;
    if (!root) return;
    root.classList.toggle('dark', dark);
    root.style.setProperty('--armarium-font-size', `${fontSize}rem`);
    localStorage.setItem('armarium.fontSize', String(fontSize));
    localStorage.setItem('armarium.dark', dark ? '1' : '0');
  }

  function scheduleSave() {
    clearTimeout(saveTimer);
    saveTimer = setTimeout(save, 1200);
  }

  function save() {
    if (!item || !chapters) return;
    api.progress(id, progressFor(chapter, chapters, fraction)).catch(() => {});
  }

  function go(n: number) {
    if (n < 0 || n >= chapters) return;
    save();
    chapter = n;
    showToc = false;
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'ArrowRight' && e.altKey === false) go(chapter + 1);
    if (e.key === 'ArrowLeft' && e.altKey === false) go(chapter - 1);
  }

  onDestroy(() => { clearTimeout(saveTimer); save(); });
</script>

<svelte:window onkeydown={onKey} />

<div class="reader" class:dark>
  <header>
    <a href="/item/{id}" use:link aria-label="Back">←</a>
    <span class="title">{item?.title ?? ''}{title ? ` · ${title}` : ''}</span>
    <span class="muted small">{chapters ? `${chapter + 1} / ${chapters}` : ''}</span>
    <button onclick={() => (showToc = !showToc)} aria-expanded={showToc}>Contents</button>
    <button onclick={() => { fontSize = Math.max(0.8, fontSize - 0.1); applyTheme(); }} aria-label="Smaller text">A−</button>
    <button onclick={() => { fontSize = Math.min(2, fontSize + 0.1); applyTheme(); }} aria-label="Larger text">A+</button>
    <button onclick={() => { dark = !dark; applyTheme(); }} aria-label="Toggle dark">{dark ? '☀' : '☾'}</button>
  </header>
  {#if error}<p class="error">{error}</p>{/if}
  <div class="body">
    {#if showToc}
      <nav class="toc panel">
        {#each toc as t, i (i)}
          <button class="entry" class:current={t.chapter === chapter} data-depth={Math.min(t.depth, 3)} onclick={() => go(t.chapter)}>{t.title}</button>
        {/each}
      </nav>
    {/if}
    {#if src}
      <iframe bind:this={frame} {src} title="Chapter" sandbox="allow-same-origin" onload={onLoad}></iframe>
    {/if}
  </div>
  <footer>
    <button disabled={chapter <= 0} onclick={() => go(chapter - 1)}>← Previous</button>
    <button disabled={chapter >= chapters - 1} onclick={() => go(chapter + 1)}>Next →</button>
  </footer>
</div>

<style>
  .reader { height: 100dvh; display: flex; flex-direction: column; background: #fbf8f1; color: #1f1d1a; }
  .reader.dark { background: #16181d; color: #d9d6cf; }
  header, footer { display: flex; align-items: center; gap: 0.5rem; padding: 0.45rem 0.8rem; border-bottom: 1px solid var(--divider); }
  footer { border-bottom: none; border-top: 1px solid var(--divider); justify-content: space-between; }
  header a { text-decoration: none; font-size: 1.2rem; color: inherit; }
  header button, footer button { padding: 0.3rem 0.6rem; font-size: 0.85rem; }
  .title { flex: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; font-weight: 600; }
  .body { flex: 1; display: flex; min-height: 0; position: relative; }
  iframe { flex: 1; border: 0; width: 100%; height: 100%; background: transparent; }
  .toc { position: absolute; z-index: 2; top: 0.5rem; left: 0.5rem; bottom: 0.5rem; width: min(22rem, 90vw); overflow: auto; padding: 0.5rem; }
  .entry { display: block; width: 100%; text-align: left; border: none; background: none; font-weight: 400; padding: 0.35rem 0.5rem; }
  .entry.current { font-weight: 700; color: var(--link-color); }
  .entry[data-depth='1'] { padding-left: 1.3rem; }
  .entry[data-depth='2'] { padding-left: 2.1rem; }
  .entry[data-depth='3'] { padding-left: 2.9rem; }
  @media (max-width: 40rem) { header .muted { display: none; } }
</style>
