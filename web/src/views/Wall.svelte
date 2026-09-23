<script lang="ts">
  import { api, coverUrl, fileUrl, percent, seriesOf, type Item, type Library } from '../lib/api';
  import { link, navigate } from '../lib/router.svelte';
  import Cover from './Cover.svelte';
  import Icon from './Icon.svelte';
  import Rail from './Rail.svelte';

  let { libraries }: { libraries: Library[] } = $props();

  // The resume band shows the item read most recently, or else the newest arrival.
  let lead = $state<Item | null>(null);
  let resuming = $state(false);
  $effect(() => {
    api.items({ status: 'reading', sort: 'recent', limit: 1 }).then(async (r) => {
      if (r.items[0]) { lead = r.items[0]; resuming = true; return; }
      lead = (await api.items({ sort: 'added', limit: 1 })).items[0] ?? null;
    }).catch(() => {});
  });

  const readHref = (it: Item) =>
    it.format === 'epub' ? `/reader/${it.id}` : it.format === 'cbz' || it.format === 'cbr' ? `/viewer/${it.id}` : '';

  function open(it: Item) {
    const href = readHref(it);
    if (href) navigate(href);
    else if (it.format === 'pdf') window.open(fileUrl(it.id, true), '_blank', 'noopener');
    else navigate(`/item/${it.id}`);
  }

  const where = (it: Item) => {
    const unit = it.format === 'epub' ? 'Chapter' : 'Page';
    return it.pages ? `${unit} ${Math.max(it.progress.page, 1)} of ${it.pages}` : it.format.toUpperCase();
  };
  const comics = $derived(libraries.filter((l) => l.kind === 'comics'));
  const books = $derived(libraries.filter((l) => l.kind === 'books'));
</script>

<div class="wall">
  {#if lead}
    <section class="lead">
      {#if lead.hasCover}<img class="backdrop" src={coverUrl(lead.id)} alt="" aria-hidden="true" />{/if}
      <div class="copy">
        <h1>{lead.title}</h1>
        <p class="by">{[lead.author, seriesOf(lead)].filter(Boolean).join(' · ')}</p>
        {#if resuming}
          <div class="progress" aria-label="{percent(lead)}% read">
            <span class="track"><span style:width="{percent(lead)}%"></span></span>
            <span class="caps">{where(lead)}</span>
          </div>
        {:else}
          <p class="caps">Newest in the collection</p>
        {/if}
        <div class="actions">
          <button class="primary" onclick={() => open(lead!)}>
            <Icon name="play" size={14} /> {resuming ? 'Resume' : 'Open'}
          </button>
          <a class="button" href="/item/{lead.id}" use:link>Details</a>
        </div>
      </div>
      <a class="art" href="/item/{lead.id}" use:link aria-label="{lead.title} details">
        <Cover id={lead.id} hasCover={lead.hasCover} title={lead.title} byline={lead.author} format={lead.format} natural />
      </a>
    </section>
  {/if}

  <Rail title="Continue reading" more="/?status=reading&sort=recent" eager
    load={async () => ({ items: (await api.items({ status: 'reading', sort: 'recent', limit: 24 })).items })} />
  <Rail title="Recently added" more="/?sort=added" eager
    load={async () => ({ items: (await api.items({ sort: 'added', limit: 24 })).items })} />
  {#each comics as l (l.id)}
    <Rail title={l.name} more="/?library={l.id}" load={async () => ({ series: (await api.series(l.id)).items })} />
  {/each}
  {#each books as l (l.id)}
    <Rail title={l.name} more="/?library={l.id}"
      load={async () => ({ items: (await api.items({ library: l.id, sort: 'added', limit: 24 })).items })} />
  {/each}
</div>

<style>
  .wall { display: flex; flex-direction: column; gap: 2.6rem; padding: 0 0 4rem; }
  .wall:has(:global(.rail:hover)) :global(.rail:not(:hover)) { opacity: 0.4; filter: saturate(0.5); }

  .lead {
    position: relative;
    isolation: isolate;
    overflow: hidden;
    /* Full bleed across the main column, padding restored inside. */
    margin-inline: calc(-1 * clamp(1.2rem, 3vw, 3rem));
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: end;
    gap: 3rem;
    padding: 3.2rem clamp(1.2rem, 3vw, 3rem) 1rem;
    border-bottom: 1px solid var(--line);
    margin-bottom: 0.4rem;
  }
  /* The lead's own cover, blown up and darkened behind the band: the only colour
     the world allows, and the dare the streaming wall takes on its hero. */
  .backdrop {
    position: absolute;
    /* Overhang the band so the blur's soft edge falls outside the clip. */
    inset: -3rem;
    width: calc(100% + 6rem);
    height: calc(100% + 6rem);
    object-fit: cover;
    object-position: center 35%;
    filter: blur(14px) saturate(1.2) brightness(1.15);
    opacity: 0.6;
    z-index: -2;
  }
  .lead::before {
    content: '';
    position: absolute;
    inset: 0;
    z-index: -1;
    background:
      linear-gradient(90deg, var(--ground) 12%, rgba(11, 14, 19, 0.7) 40%, transparent 72%),
      linear-gradient(0deg, var(--ground) 0%, transparent 30%);
  }
  .copy { display: flex; flex-direction: column; gap: 1.1rem; padding-bottom: 2rem; min-width: 0; }
  h1 { font-size: clamp(3.4rem, 6.2vw, 6rem); max-width: 14ch; }
  .by { margin: 0; color: var(--text-2); font-size: 1.05rem; }
  .progress { display: flex; align-items: center; gap: 1rem; max-width: 28rem; }
  .progress .track { flex: 1; height: 3px; background: var(--raised); }
  .progress .track span { display: block; height: 100%; background: var(--amber); }
  .actions { display: flex; gap: 0.8rem; margin-top: 0.6rem; }
  .art {
    width: clamp(13rem, 20vw, 17rem);
    border-radius: 4px;
    overflow: hidden;
    box-shadow: 0 24px 60px rgba(0, 0, 0, 0.6), 0 2px 6px rgba(0, 0, 0, 0.5);
    margin-bottom: 1.6rem;
  }
  .art:has(:global(.plate)) { aspect-ratio: 2 / 3; }

  @media (max-width: 760px) {
    .lead { grid-template-columns: 1fr; padding-top: 1.5rem; }
    .art { display: none; }
    .copy { padding-bottom: 1rem; }
  }
</style>
