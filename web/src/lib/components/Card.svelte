<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    header?: Snippet;
    footer?: Snippet;
    children: Snippet;
    padding?: 'none' | 'sm' | 'md' | 'lg';
    class?: string;
  }

  let {
    header,
    footer,
    children,
    padding = 'md',
    class: className = '',
  }: Props = $props();

  const paddingMap: Record<string, string> = {
    none: '',
    sm: 'p-3',
    md: 'p-4',
    lg: 'p-6',
  };
</script>

<div
  class="rounded-[var(--border-radius-lg)] border border-[hsl(var(--color-border))] bg-[hsl(var(--color-surface))] shadow-[var(--shadow-xs)] {className}"
  data-testid="card"
>
  {#if header}
    <div class="border-b border-[hsl(var(--color-border))] px-4 py-3">
      {@render header()}
    </div>
  {/if}

  <div class={paddingMap[padding]}>
    {@render children()}
  </div>

  {#if footer}
    <div class="border-t border-[hsl(var(--color-border))] px-4 py-3">
      {@render footer()}
    </div>
  {/if}
</div>
