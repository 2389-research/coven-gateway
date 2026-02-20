<script lang="ts">
  import type { HTMLButtonAttributes } from 'svelte/elements';

  interface Props extends Omit<HTMLButtonAttributes, 'class'> {
    value: string;
    label?: string;
    copiedLabel?: string;
    class?: string;
  }

  let {
    value,
    label = 'Copy',
    copiedLabel = 'Copied!',
    disabled = false,
    class: className = '',
    ...rest
  }: Props = $props();

  let copied = $state(false);
  let timer: ReturnType<typeof setTimeout> | undefined;

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(value);
      copied = true;
      clearTimeout(timer);
      timer = setTimeout(() => { copied = false; }, 2000);
    } catch {
      // Clipboard API unavailable (insecure context, denied permission)
    }
  }
</script>

<button
  type="button"
  class="inline-flex items-center gap-1.5 rounded-[var(--border-radius-md)] border border-border bg-surface px-2.5 py-1 text-[length:var(--typography-fontSize-sm)] font-[var(--typography-fontWeight-medium)] text-fg transition-colors duration-[var(--motion-duration-fast)] hover:bg-surfaceHover focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring disabled:opacity-50 disabled:cursor-not-allowed {className}"
  {disabled}
  onclick={handleCopy}
  aria-label={copied ? copiedLabel : label}
  data-testid="copy-button"
  {...rest}
>
  {#if copied}
    <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="text-success-subtleFg" aria-hidden="true"><path d="M20 6 9 17l-5-5"/></svg>
    <span class="text-success-subtleFg">{copiedLabel}</span>
  {:else}
    <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect width="14" height="14" x="8" y="8" rx="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg>
    <span>{label}</span>
  {/if}
</button>
