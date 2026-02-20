<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    href?: string;
    children: Snippet;
    class?: string;
  }

  let { href, children, class: className = '' }: Props = $props();
</script>

<li class="inline-flex items-center gap-1.5 {className}" data-testid="breadcrumb-item">
  {#if href}
    <a {href} class="text-fgMuted hover:text-fg transition-colors duration-[var(--motion-duration-fast)]">
      {@render children()}
    </a>
  {:else}
    <span class="text-fg font-[var(--typography-fontWeight-medium)]">
      {@render children()}
    </span>
  {/if}
  {#if href}
    <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="text-fgMuted" aria-hidden="true"><path d="m9 18 6-6-6-6"/></svg>
  {/if}
</li>
