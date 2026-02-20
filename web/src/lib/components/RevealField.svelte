<script lang="ts">
  interface Props {
    value: string;
    mask?: string;
    class?: string;
  }

  let {
    value,
    mask = '••••••••',
    class: className = '',
  }: Props = $props();

  let revealed = $state(false);

  function toggle() {
    revealed = !revealed;
  }
</script>

<div class="inline-flex items-center gap-2 {className}" data-testid="reveal-field">
  <span class="font-mono text-[length:var(--typography-fontSize-sm)] text-fg select-all">
    {revealed ? value : mask}
  </span>
  <button
    type="button"
    class="inline-flex items-center justify-center rounded-[var(--border-radius-md)] p-1 text-fgMuted hover:bg-surfaceHover hover:text-fg transition-colors duration-[var(--motion-duration-fast)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
    onclick={toggle}
    aria-label={revealed ? 'Hide value' : 'Reveal value'}
    data-testid="reveal-toggle"
  >
    {#if revealed}
      <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M10.733 5.076a10.744 10.744 0 0 1 11.205 6.575 1 1 0 0 1 0 .696 10.747 10.747 0 0 1-1.444 2.49"/><path d="M14.084 14.158a3 3 0 0 1-4.242-4.242"/><path d="M17.479 17.499a10.75 10.75 0 0 1-15.417-5.151 1 1 0 0 1 0-.696 10.75 10.75 0 0 1 4.446-5.143"/><path d="m2 2 20 20"/></svg>
    {:else}
      <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M2.062 12.348a1 1 0 0 1 0-.696 10.75 10.75 0 0 1 19.876 0 1 1 0 0 1 0 .696 10.75 10.75 0 0 1-19.876 0"/><circle cx="12" cy="12" r="3"/></svg>
    {/if}
  </button>
</div>
