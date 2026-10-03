<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type PairInfo, type PairReader } from '../lib/api';
  import { DEVICES, createPairToken, deviceSlug, isLoopback, pairLink, qrPath, readDeviceChoice } from '../lib/pair';

  let { onchange }: { onchange: () => void | Promise<void> } = $props();

  type Pairing = { reader: PairReader; device: string; name: string; link: string; qr: { size: number; d: string } | null; qrError: string };

  let info = $state<PairInfo | null>(null);
  let pairing = $state<Pairing | null>(null);
  let busy = $state(false);
  let error = $state('');

  // Which device is being paired names its token (books-iphone-…), so the token list
  // tells devices apart. The last choice is remembered in this browser.
  const saved = readDeviceChoice(
    (() => {
      try {
        return localStorage.getItem('armarium.pairDevice');
      } catch {
        return null;
      }
    })(),
  );
  let device = $state(saved.device);
  let other = $state(saved.other);
  const deviceLabel = $derived(device === 'Other' ? other.trim() : device);
  $effect(() => {
    try {
      localStorage.setItem('armarium.pairDevice', JSON.stringify({ device, other }));
    } catch {
      // Private mode or blocked storage: just don't remember it.
    }
  });

  // Without public_url the QR points at the address this page was loaded from.
  const base = $derived(info?.publicUrl || location.origin);
  const plainHttp = $derived(base.startsWith('http:'));

  onMount(() => {
    api.pair().then((p) => (info = p)).catch((e) => (error = (e as Error).message));
  });

  async function pair(reader: PairReader) {
    error = '';
    pairing = null; // never leave the previous phone's QR up if this attempt fails
    busy = true;
    try {
      const { token, secret } = await createPairToken(reader.id, deviceSlug(deviceLabel), new Date(), api.createToken);
      const link = pairLink(reader.url, base, secret);
      let qr: Pairing['qr'] = null;
      let qrError = '';
      try {
        qr = qrPath(link);
      } catch (e) {
        qrError = (e as Error).message;
      }
      pairing = { reader, device: deviceLabel || 'phone', name: token.name, link, qr, qrError };
      await onchange();
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = false;
    }
  }
</script>

<section>
  <div class="bar"><h2>Pair a phone</h2></div>
  <p class="note muted">
    Opens Skrivist Comics or Skrivist Books on your phone with this library's OPDS catalogue filled in. Each pairing gets its own token.
  </p>
  {#if error}<p class="error">{error}</p>{/if}
  {#if info}
    {#if plainHttp}
      <p class="warn block">The hosted readers only load HTTPS catalogues. Set <code>public_url</code> to an HTTPS address.</p>
    {:else}
      {#if !info.publicUrl}
        <p class="warn">public_url is not set, so the QR uses this page's address (<code>{base}</code>). A phone may not reach it.</p>
      {/if}
      {#if isLoopback(base)}
        <p class="warn"><code>{base}</code> is this computer's loopback address; a phone can't reach it.</p>
      {/if}
    {/if}
    <div class="device">
      <label>
        <span class="caps">Device</span>
        <select bind:value={device} disabled={plainHttp || busy}>
          {#each DEVICES as d (d)}<option value={d}>{d}</option>{/each}
        </select>
      </label>
      {#if device === 'Other'}
        <input bind:value={other} maxlength="40" placeholder="Name it, e.g. Pixel 8" aria-label="Device name" disabled={plainHttp || busy} />
      {/if}
    </div>
    <div class="readers">
      {#each info.readers as r (r.id)}
        <button disabled={plainHttp || busy} onclick={() => pair(r)}>{r.name}</button>
      {/each}
    </div>
    {#if !plainHttp}
      {#each info.readers.filter((r) => !r.corsAllowed) as r (r.id)}
        <p class="warn">Add <code>{r.origin}</code> to <code>cors_origins</code>, or {r.name} can't load the catalogue.</p>
      {/each}
    {/if}
  {/if}

  {#if pairing}
    <div class="pairing" role="status">
      {#if pairing.qr}
        <svg
          viewBox="0 0 {pairing.qr.size} {pairing.qr.size}"
          width="264"
          height="264"
          shape-rendering="crispEdges"
          role="img"
          aria-label="Pairing QR code for {pairing.reader.name}"
        >
          <rect width={pairing.qr.size} height={pairing.qr.size} fill="#fff" />
          <path d={pairing.qr.d} fill="#000" />
        </svg>
      {:else}
        <p class="error">Couldn't draw the QR code: {pairing.qrError}</p>
      {/if}
      <div class="details">
        <p>Open the camera on {pairing.device}, scan this, then tap Connect in {pairing.reader.name}.</p>
        <p class="muted small">Token <strong>{pairing.name}</strong>. Revoke it under API tokens to unpair.</p>
        <code>{pairing.link}</code>
        <button onclick={() => (pairing = null)}>Done</button>
      </div>
    </div>
  {/if}
</section>

<style>
  section { max-width: 56rem; padding: 2rem 0; border-top: 1px solid var(--line); }
  .bar { display: flex; align-items: center; justify-content: space-between; gap: 1rem; }
  h2 { font-size: 2rem; }
  .note { margin: 0.5rem 0 1.2rem; font-size: 14px; }
  .device { display: flex; flex-wrap: wrap; align-items: end; gap: 0.7rem; margin-bottom: 0.9rem; }
  .device label { display: grid; gap: 0.3rem; }
  .device input { min-width: 14rem; }
  .readers { display: flex; flex-wrap: wrap; gap: 0.7rem; margin-bottom: 1rem; }
  .warn { margin: 0 0 0.8rem; font-size: 14px; color: var(--amber); }
  .warn.block { color: var(--danger); }
  .pairing { display: flex; flex-wrap: wrap; gap: 1.4rem; align-items: flex-start; padding: 1rem 1.1rem; background: var(--surface); border: 1px solid var(--amber); border-radius: 3px; }
  .pairing svg { flex: none; width: 264px; height: 264px; max-width: 100%; }
  .details { flex: 1; min-width: 16rem; display: grid; gap: 0.6rem; justify-items: start; }
  .details p { margin: 0; }
  .small { font-size: 12.5px; }
  code { font-size: 12.5px; word-break: break-all; color: var(--text); }
</style>
