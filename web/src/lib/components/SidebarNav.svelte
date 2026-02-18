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

  function handleSelect(item: NavItem) {
    onselect?.(item.id);
  }

  function itemClasses(active: boolean): string {
    const base = 'flex items-center gap-2 rounded-[var(--border-radius-md)] px-3 py-2 text-[length:var(--typography-fontSize-sm)] transition-colors duration-[var(--motion-duration-fast)]';
    return active
      ? `${base} bg-accentMuted text-accent font-[var(--typography-fontWeight-medium)]`
      : `${base} text-fgMuted hover:bg-surfaceHover hover:text-fg`;
  }
</script>

{#snippet navItemContent(item: NavItem)}
  {#if item.icon}
    <span class="flex-shrink-0 [&>svg]:h-4 [&>svg]:w-4">
      {@render item.icon()}
    </span>
  {/if}
  <span class="truncate">{item.label}</span>
{/snippet}

{#snippet navItem(item: NavItem)}
  {#if item.href}
    <a
      href={item.href}
      class={itemClasses(!!item.active)}
      aria-current={item.active ? 'page' : undefined}
      onclick={() => handleSelect(item)}
      data-testid="nav-item-{item.id}"
    >
      {@render navItemContent(item)}
    </a>
  {:else}
    <button
      type="button"
      class="{itemClasses(!!item.active)} w-full text-left"
      aria-current={item.active ? 'page' : undefined}
      onclick={() => handleSelect(item)}
      data-testid="nav-item-{item.id}"
    >
      {@render navItemContent(item)}
    </button>
  {/if}
{/snippet}

<nav aria-label="Sidebar" class="flex flex-col gap-1 {className}" data-testid="sidebar-nav">
  {#if items.length > 0}
    {#each items as item (item.id)}
      {@render navItem(item)}
    {/each}
  {/if}

  {#each groups as group, i}
    <div class="mt-2 first:mt-0" role="group" aria-labelledby="nav-group-{i}">
      <h3 id="nav-group-{i}" class="mb-1 px-3 text-[length:var(--typography-fontSize-xs)] font-[var(--typography-fontWeight-semibold)] uppercase tracking-wider text-fgMuted">
        {group.label}
      </h3>
      {#each group.items as item (item.id)}
        {@render navItem(item)}
      {/each}
    </div>
  {/each}
</nav>
