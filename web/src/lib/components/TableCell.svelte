<script lang="ts">
  import type { Snippet } from 'svelte';

  type Align = 'left' | 'center' | 'right';

  interface Props {
    align?: Align;
    colspan?: number;
    children: Snippet;
    class?: string;
  }

  let { align = 'left', colspan, children, class: className = '' }: Props = $props();

  const alignClasses: Record<Align, string> = {
    left: 'text-left',
    center: 'text-center',
    right: 'text-right',
  };
</script>

<td
  class="px-4 py-3 text-[length:var(--typography-fontSize-sm)] text-fg {alignClasses[align]} {className}"
  colspan={colspan}
  data-testid="table-cell"
>
  {@render children()}
</td>
