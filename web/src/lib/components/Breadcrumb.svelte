<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    children: Snippet;
    class?: string;
  }

  let { children, class: className = '' }: Props = $props();
</script>

<nav aria-label="Breadcrumb" class={className} data-testid="breadcrumb">
  <ol class="flex items-center gap-1.5 text-[length:var(--typography-fontSize-sm)]">
    {@render children()}
  </ol>
</nav>
