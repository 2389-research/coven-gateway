<script lang="ts">
  import type { Snippet } from 'svelte';

  interface NavItem {
    id: string;
    label: string;
    href?: string;
    active?: boolean;
    icon?: Snippet;
  }

  interface NavGroup {
    label: string;
    items: NavItem[];
  }

  interface Props {
    items?: NavItem[];
    groups?: NavGroup[];
    onselect?: (itemId: string) => void;
    class?: string;
  }

  let {
    items = [],
    groups = [],
    onselect,
    class: className = '',
  }: Props = $props();

  function handleClick(item: NavItem, e: MouseEvent) {
    if (!item.href) {
      e.preventDefault();
    }
    onselect?.(item.id);
  }
</script>

{#snippet navItem(item: NavItem)}
  <a
    href={item.href ?? '#'}
    class="flex items-center gap-2 rounded-[var(--border-radius-md)] px-3 py-2 text-[length:var(--typography-fontSize-sm)] transition-colors duration-[var(--motion-duration-fast)]
      {item.active
        ? 'bg-accentMuted text-accent font-[var(--typography-fontWeight-medium)]'
        : 'text-fgMuted hover:bg-surfaceHover hover:text-fg'}"
    aria-current={item.active ? 'page' : undefined}
    onclick={(e) => handleClick(item, e)}
    data-testid="nav-item-{item.id}"
  >
    {#if item.icon}
      <span class="flex-shrink-0 [&>svg]:h-4 [&>svg]:w-4">
        {@render item.icon()}
      </span>
    {/if}
    <span class="truncate">{item.label}</span>
  </a>
{/snippet}

<nav aria-label="Sidebar" class="flex flex-col gap-1 {className}" data-testid="sidebar-nav">
  {#if items.length > 0}
    {#each items as item (item.id)}
      {@render navItem(item)}
    {/each}
  {/if}

  {#each groups as group}
    <div class="mt-2 first:mt-0">
      <h3 class="mb-1 px-3 text-[length:var(--typography-fontSize-xs)] font-[var(--typography-fontWeight-semibold)] uppercase tracking-wider text-fgMuted">
        {group.label}
      </h3>
      {#each group.items as item (item.id)}
        {@render navItem(item)}
      {/each}
    </div>
  {/each}
</nav>
