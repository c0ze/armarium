<script lang="ts">
  import { onDestroy } from 'svelte';
  import { api, chapterUrl, type Item, type TocEntry } from '../lib/api';
  import { link } from '../lib/router.svelte';
  import Icon from './Icon.svelte';
  import { chapterFromPath, parseLocator, progressFor } from '../lib/reading';

  let { id }: { id: number } = $props();
  let item = $state<Item | null>(null);
  let toc = $state<TocEntry[]>([]);
  let chapters = $state(0);
  let chapter = $state(0);
  // Right-to-left books (Japanese, Arabic) turn pages the other way.
  let rtl = $state(false);
  let showToc = $state(false);
  let error = $state('');
  let frame = $state<HTMLIFrameElement>();
  let fontSize = $state(Number(localStorage.getItem('armarium.fontSize') ?? 1.1));
  // Dark (the app's ground) unless the reader chose paper.
  let dark = $state(localStorage.getItem('armarium.dark') !== '0');
  let restoreScroll = 0;
  let saveTimer: ReturnType<typeof setTimeout>;
  let fraction = 0;

  $effect(() => {
    Promise.all([api.item(id), api.toc(id)])
      .then(([d, t]) => {
        item = d.item;
        toc = t.toc;
        chapters = t.chapters;
        rtl = t.rtl;
        const loc = parseLocator(d.item.progress.locator);
        chapter = Math.min(loc?.chapter ?? Math.max(d.item.progress.page - 1, 0), Math.max(chapters - 1, 0));
        restoreScroll = loc?.scroll ?? 0;
      })
      .catch((e) => (error = e.message));
  });

  const src = $derived(item ? chapterUrl(id, chapter) : '');
  // Some TOCs title chapters with a lone "." or "*"; those say nothing.
  const title = $derived.by(() => { const name = toc.findLast((e) => e.chapter <= chapter)?.title ?? ''; return /[\p{L}\p{N}]/u.test(name) ? name : ''; });

  // The iframe is sandboxed without scripts; same-origin lets us style it and
  // read its scroll position from here.
  function onLoad() {
    const win = frame?.contentWindow;
    const doc = frame?.contentDocument;
    if (!win || !doc) return;
    const landed = chapterFromPath(win.location.pathname, id);
    if (landed !== null && landed !== chapter) chapter = landed; // followed an in-book link
    applyTheme();
    // Vertical text (tategaki) flows right to left, so the chapter scrolls
    // sideways: scrollX runs from 0 to minus the overflow.
    const vertical = win.getComputedStyle(doc.body).writingMode.startsWith('vertical');
    doc.documentElement.classList.toggle('vertical', vertical);
    const root = doc.documentElement;
    const span = () => (vertical ? root.scrollWidth - win.innerWidth : root.scrollHeight - win.innerHeight);
    if (restoreScroll) {
      if (vertical) win.scrollTo(-restoreScroll * span(), 0);
      else win.scrollTo(0, restoreScroll * span());
      restoreScroll = 0;
    }
    win.addEventListener('scroll', () => {
      const max = span();
      fraction = max > 0 ? Math.min(Math.abs(vertical ? win.scrollX : win.scrollY) / max, 1) : 1;
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
    if (e.altKey) return;
    const [next, prev] = rtl ? ['ArrowLeft', 'ArrowRight'] : ['ArrowRight', 'ArrowLeft'];
    if (e.key === next) go(chapter + 1);
    if (e.key === prev) go(chapter - 1);
  }

  onDestroy(() => { clearTimeout(saveTimer); save(); });
</script>

<svelte:window onkeydown={onKey} />

<div class="reader" class:dark>
  <header>
    <a class="back" href="/item/{id}" use:link aria-label="Back to the item"><Icon name="back" /></a>
    <span class="title"><strong>{item?.title ?? ''}</strong>{#if title}<span class="ch">{title}</span>{/if}</span>
    <span class="caps pos">{chapters ? `${chapter + 1} / ${chapters}` : ''}</span>
    <button class="quiet" onclick={() => (showToc = !showToc)} aria-expanded={showToc}><Icon name="list" size={16} /> Contents</button>
    <button class="quiet" onclick={() => { fontSize = Math.max(0.8, fontSize - 0.1); applyTheme(); }} aria-label="Smaller text">A−</button>
    <button class="quiet" onclick={() => { fontSize = Math.min(2, fontSize + 0.1); applyTheme(); }} aria-label="Larger text">A+</button>
    <button class="quiet" onclick={() => { dark = !dark; applyTheme(); }} aria-label={dark ? 'Light page' : 'Dark page'}><Icon name={dark ? 'sun' : 'moon'} size={16} /></button>
  </header>
  {#if error}<p class="error">{error}</p>{/if}
  <div class="body">
    {#if showToc}
      <nav class="toc" aria-label="Contents">
        {#each toc as t, i (i)}
          <button class="entry" class:current={t.chapter === chapter} data-depth={Math.min(t.depth, 3)} onclick={() => go(t.chapter)}>{t.title}</button>
        {/each}
      </nav>
    {/if}
    {#if src}
      <iframe bind:this={frame} {src} title="Chapter" sandbox="allow-same-origin" onload={onLoad}></iframe>
    {/if}
  </div>
  <footer dir={rtl ? 'rtl' : 'ltr'}>
    <button disabled={chapter <= 0} onclick={() => go(chapter - 1)}><Icon name={rtl ? 'right' : 'left'} size={14} /> Previous</button>
    <button disabled={chapter >= chapters - 1} onclick={() => go(chapter + 1)}>Next <Icon name={rtl ? 'left' : 'right'} size={14} /></button>
  </footer>
</div>

<style>
  .reader { height: 100dvh; display: flex; flex-direction: column; background: var(--ground); color: var(--text); }
  header, footer { display: flex; align-items: center; gap: 0.4rem; padding: 0.55rem 1rem; background: var(--ground); }
  header { border-bottom: 1px solid var(--line); }
  footer { border-top: 1px solid var(--line); justify-content: space-between; }
  .back { display: inline-flex; padding: 0.4rem; border-radius: 3px; color: var(--text-2); }
  .back:hover { color: var(--amber); }
  .title { flex: 1; min-width: 0; display: flex; align-items: baseline; gap: 0.9rem; white-space: nowrap; overflow: hidden; }
  .title strong { font-family: var(--display); font-weight: 400; font-size: 1.5rem; letter-spacing: 0.03em; text-transform: uppercase; overflow: hidden; text-overflow: ellipsis; }
  .ch { color: var(--text-3); font-size: 13px; overflow: hidden; text-overflow: ellipsis; }
  .pos { margin-right: 0.6rem; }
  footer button { padding: 0.6rem 0.9rem; }
  .body { flex: 1; display: flex; min-height: 0; position: relative; }
  iframe { flex: 1; border: 0; width: 100%; height: 100%; background: #fbf8f1; }
  .dark iframe { background: var(--ground); }
  .toc { position: absolute; z-index: 2; top: 0; left: 0; bottom: 0; width: min(24rem, 92vw); overflow: auto; padding: 0.8rem; background: var(--surface); border-right: 1px solid var(--line); box-shadow: 12px 0 40px rgba(0, 0, 0, 0.45); }
  .entry { display: block; width: 100%; text-align: left; border: 0; text-transform: none; letter-spacing: 0; font: 400 14px/1.35 var(--ui); padding: 0.45rem 0.6rem; color: var(--text-2); }
  .entry:hover { color: var(--text); background: var(--raised); }
  .entry.current { color: var(--amber); }
  .entry[data-depth='1'] { padding-left: 1.4rem; }
  .entry[data-depth='2'] { padding-left: 2.2rem; }
  .entry[data-depth='3'] { padding-left: 3rem; }
  @media (max-width: 40rem) { .ch, .pos { display: none; } }
</style>
