<script lang="ts">
  import type { FolderCheck, LibrarySettings } from '../lib/api';
  import { defaultName, uniqueName } from '../lib/setup';

  // The library list of setup and Settings. `checks` holds the last folder
  // check per row (by root); rows whose check failed show its message.
  let {
    libs = $bindable(),
    canChoose,
    check,
    choose,
    onremove,
  }: {
    libs: LibrarySettings[];
    canChoose: boolean;
    check: (path: string) => Promise<FolderCheck>;
    choose: () => Promise<{ path: string } | undefined>;
    onremove?: (lib: LibrarySettings, index: number) => void;
  } = $props();

  let checks = $state<Record<string, FolderCheck>>({});
  let typed = $state('');
  let error = $state('');
  let busy = $state(false);

  async function add(path: string) {
    path = path.trim();
    if (!path) return;
    error = '';
    if (libs.some((l) => l.root === path)) {
      error = 'That folder is already in the list.';
      return;
    }
    busy = true;
    try {
      const c = await check(path);
      checks[path] = c;
      if (!c.ok) {
        error = c.error;
        return;
      }
      const name = uniqueName(defaultName(path), libs.map((l) => l.name));
      libs = [...libs, { name, root: path, kind: c.kind || 'books', calibre: c.calibre }];
      typed = '';
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = false;
    }
  }

  async function pick() {
    error = '';
    try {
      const r = await choose();
      if (r?.path) await add(r.path);
    } catch (e) {
      error = (e as Error).message;
    }
  }

  function remove(i: number) {
    if (onremove) onremove(libs[i], i);
    else libs = libs.filter((_, j) => j !== i);
  }
</script>

{#if libs.length}
  <ul class="rows">
    {#each libs as lib, i (lib.root)}
      <li>
        <input aria-label="Library name" bind:value={lib.name} maxlength="60" />
        <span class="path" title={lib.root}><bdi dir="ltr">{lib.root}</bdi></span>
        {#if lib.calibre}
          <span class="caps tag">Calibre</span>
        {:else}
          <div class="seg" role="group" aria-label="What's in {lib.name}">
            <button class:on={lib.kind === 'books'} aria-pressed={lib.kind === 'books'} onclick={() => (lib.kind = 'books')}>Books</button>
            <button class:on={lib.kind === 'comics'} aria-pressed={lib.kind === 'comics'} onclick={() => (lib.kind = 'comics')}>Comics</button>
          </div>
        {/if}
        <button class="quiet" onclick={() => remove(i)} aria-label="Remove {lib.name}">Remove</button>
        {#if checks[lib.root] && !checks[lib.root].ok}<p class="error small">{checks[lib.root].error}</p>{/if}
      </li>
    {/each}
  </ul>
{:else}
  <p class="muted">No folders yet. Add the folder that holds your books, comics or Calibre library.</p>
{/if}

<div class="add">
  {#if canChoose}
    <button onclick={pick} disabled={busy}>Choose folder…</button>
    <span class="muted small">or type a path:</span>
  {/if}
  <form onsubmit={(e) => { e.preventDefault(); add(typed); }}>
    <input bind:value={typed} placeholder={navigator.userAgent.includes('Windows') ? 'D:\\Books' : '/Users/you/Books'} aria-label="Folder path" />
    <button disabled={busy || !typed.trim()}>Add</button>
  </form>
</div>
{#if error}<p class="error" role="alert">{error}</p>{/if}

<style>
  .rows { list-style: none; margin: 0 0 1rem; padding: 0; display: grid; gap: 0.6rem; }
  li { display: grid; grid-template-columns: minmax(8rem, 14rem) 1fr auto auto; gap: 0.7rem; align-items: center; padding: 0.6rem 0; border-bottom: 1px solid var(--line); }
  .path { font-size: 13px; color: var(--text-2); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; direction: rtl; text-align: left; }
  .tag { padding: 0.5rem 0.7rem; border: 1px solid var(--line); border-radius: 3px; }
  li .error { grid-column: 1 / -1; margin: 0; }
  .add { display: flex; flex-wrap: wrap; gap: 0.7rem; align-items: center; }
  .add form { display: flex; gap: 0.7rem; flex: 1; min-width: 16rem; }
  .add input { flex: 1; }
  .small { font-size: 13px; }
  @media (max-width: 640px) {
    li { grid-template-columns: 1fr auto; }
    .path { grid-column: 1 / -1; grid-row: 2; }
  }
</style>
