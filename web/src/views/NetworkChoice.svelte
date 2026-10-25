<script lang="ts">
  import type { TailscaleStatus } from '../lib/api';

  // Who can read: this computer, or devices at home too; Tailscale on top of either.
  let {
    network = $bindable(),
    tailscale = $bindable(),
    status,
  }: { network: 'local' | 'home'; tailscale: boolean; status?: TailscaleStatus } = $props();

  const windows = navigator.userAgent.includes('Windows');
  const tailscaleProblem = $derived(
    !status?.available
      ? "Tailscale isn't installed on this computer."
      : status.conflict
        ? status.conflict
        : !status.name
          ? "Tailscale is installed but isn't running or isn't logged in."
          : '',
  );
  $effect(() => {
    if (tailscaleProblem && tailscale && !status?.serving) tailscale = false;
  });
</script>

<fieldset>
  <legend class="visually-hidden">Who can read</legend>
  <label class="choice" class:on={network === 'local'}>
    <input type="radio" name="network" value="local" bind:group={network} />
    <span>
      <strong>This computer only</strong>
      <span class="muted">Read in this computer's browser and in the Skrivist apps for Mac and Windows.</span>
    </span>
  </label>
  <label class="choice" class:on={network === 'home'}>
    <input type="radio" name="network" value="home" bind:group={network} />
    <span>
      <strong>Devices at home</strong>
      <span class="muted">Phones and tablets on the same Wi-Fi can read too, with the Skrivist Books app.</span>
      {#if windows && network === 'home'}
        <span class="note">Windows may ask whether to allow Armarium on networks. Choose Allow.</span>
      {/if}
    </span>
  </label>
  <label class="choice" class:on={tailscale} class:off={!!tailscaleProblem}>
    <input type="checkbox" bind:checked={tailscale} disabled={!!tailscaleProblem} />
    <span>
      <strong>Also from anywhere, with Tailscale</strong>
      <span class="muted">
        A private HTTPS address on your Tailscale network{status?.name ? ` (${status.name})` : ''}. Works away from home,
        and the Skrivist web readers can use it too.
      </span>
      {#if tailscaleProblem}<span class="note">{tailscaleProblem}</span>{/if}
    </span>
  </label>
</fieldset>

<style>
  fieldset { border: 0; margin: 0; padding: 0; display: grid; gap: 0.7rem; }
  .choice { display: flex; gap: 0.9rem; align-items: flex-start; padding: 0.9rem 1rem; border: 1px solid var(--line); border-radius: 3px; cursor: pointer; }
  .choice.on { border-color: var(--text-2); background: var(--surface); }
  .choice.off { cursor: default; opacity: 0.75; }
  .choice input { margin-top: 0.2rem; accent-color: var(--text); }
  .choice > span { display: grid; gap: 0.2rem; }
  strong { font-weight: 600; }
  .muted { font-size: 14px; }
  .note { font-size: 13px; color: var(--amber); }
</style>
