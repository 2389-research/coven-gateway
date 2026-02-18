<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    sidebar?: Snippet;
    header?: Snippet;
    children: Snippet;
    class?: string;
  }

  let {
    sidebar,
    header,
    children,
    class: className = '',
  }: Props = $props();
</script>

<div
  class="flex h-screen bg-[hsl(var(--color-bg))] text-[hsl(var(--color-fg))] {className}"
  data-testid="app-shell"
>
  {#if sidebar}
    <aside
      class="flex w-[var(--sizing-sidebar)] flex-shrink-0 flex-col border-r border-[hsl(var(--color-border))] bg-[hsl(var(--color-surface))]"
      data-testid="app-shell-sidebar"
    >
      {@render sidebar()}
    </aside>
  {/if}

  <div class="flex flex-1 flex-col overflow-hidden">
    {#if header}
      <header
        class="flex h-14 flex-shrink-0 items-center border-b border-[hsl(var(--color-border))] bg-[hsl(var(--color-surface))] px-4"
        data-testid="app-shell-header"
      >
        {@render header()}
      </header>
    {/if}

    <main class="flex-1 overflow-auto" data-testid="app-shell-main">
      {@render children()}
    </main>
  </div>
</div>
