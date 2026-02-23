<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    children: Snippet;
    class?: string;
  }

  let { children, class: className = '' }: Props = $props();
</script>

<tr class="hover:bg-surfaceHover transition-colors duration-[var(--motion-duration-fast)] {className}" data-testid="table-row">
  {@render children()}
</tr>
