<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    children: Snippet;
    class?: string;
  }

  let { children, class: className = '' }: Props = $props();
</script>

<tbody class="divide-y divide-border {className}" data-testid="table-body">
  {@render children()}
</tbody>
