<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    children: Snippet;
    class?: string;
  }

  let { children, class: className = '' }: Props = $props();
</script>

<code
  class="rounded-[var(--border-radius-sm)] bg-surfaceAlt px-1.5 py-0.5 font-mono text-[length:var(--typography-fontSize-sm)] text-fg select-all {className}"
  data-testid="code-text"
>{@render children()}</code>
