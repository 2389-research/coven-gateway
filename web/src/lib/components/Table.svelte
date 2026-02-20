<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    children: Snippet;
    class?: string;
  }

  let { children, class: className = '' }: Props = $props();
</script>

<div class="w-full overflow-x-auto rounded-[var(--border-radius-lg)] border border-border" data-testid="table">
  <table class="w-full text-left {className}">
    {@render children()}
  </table>
</div>
