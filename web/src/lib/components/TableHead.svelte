<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    children: Snippet;
    class?: string;
  }

  let { children, class: className = '' }: Props = $props();
</script>

<thead class="bg-surfaceAlt border-b border-border {className}" data-testid="table-head">
  {@render children()}
</thead>
