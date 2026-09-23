<script lang="ts">
  import { coverUrl } from '../lib/api';

  // A cover, or, when there is none (MOBI, PDF, a book Calibre has no art for),
  // a set title plate in the same proportions so the wall never shows a hole.
  let { id, hasCover, title, byline = '', format = '', natural = false }: {
    id: number; hasCover: boolean; title: string; byline?: string; format?: string; natural?: boolean;
  } = $props();
  let broken = $state(false);
</script>

{#if hasCover && !broken}
  <img class="cover" class:natural src={coverUrl(id)} alt="" loading="lazy" decoding="async" onerror={() => (broken = true)} />
{:else}
  <div class="plate" aria-hidden="true">
    <span class="t">{title}</span>
    {#if byline}<span class="b">{byline}</span>{/if}
    {#if format}<span class="f">{format}</span>{/if}
  </div>
{/if}

<style>
  .cover { width: 100%; height: 100%; object-fit: cover; display: block; background: var(--raised); }
  /* Rails crop to keep the row even; a whole-cover view never cuts the art. */
  .cover.natural { height: auto; object-fit: contain; }
  .plate {
    width: 100%;
    height: 100%;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding: 0.9rem 0.8rem;
    background: var(--raised);
    border-top: 3px solid #323a4a;
    overflow: hidden;
  }
  .t {
    font-family: var(--display);
    font-size: 1.35rem;
    line-height: 0.95;
    text-transform: uppercase;
    color: var(--text);
    display: -webkit-box;
    -webkit-line-clamp: 5;
    line-clamp: 5;
    -webkit-box-orient: vertical;
    overflow: hidden;
    overflow-wrap: anywhere;
  }
  .b { font-size: 11px; color: var(--text-2); line-height: 1.3; }
  .f { margin-top: auto; font: 500 10px/1 var(--ui); letter-spacing: 0.18em; text-transform: uppercase; color: var(--text-3); }
</style>
