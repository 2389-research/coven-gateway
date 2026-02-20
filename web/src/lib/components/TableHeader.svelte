<script lang="ts">
  import type { Snippet } from 'svelte';

  type Align = 'left' | 'center' | 'right';

  interface Props {
    align?: Align;
    children: Snippet;
    class?: string;
  }

  let { align = 'left', children, class: className = '' }: Props = $props();

  const alignClasses: Record<Align, string> = {
    left: 'text-left',
    center: 'text-center',
    right: 'text-right',
  };
</script>

<th class="px-4 py-3 text-[length:var(--typography-fontSize-xs)] font-[var(--typography-fontWeight-semibold)] text-fgMuted uppercase tracking-wider {alignClasses[align]} {className}" data-testid="table-header">
  {@render children()}
</th>
