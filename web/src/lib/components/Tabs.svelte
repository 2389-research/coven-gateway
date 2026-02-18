<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Tab {
    id: string;
    label: string;
    disabled?: boolean;
  }

  interface Props {
    tabs: Tab[];
    activeTab?: string;
    onchange?: (tabId: string) => void;
    panel?: Snippet<[string]>;
    class?: string;
  }

  let {
    tabs,
    activeTab = $bindable(),
    onchange,
    panel,
    class: className = '',
  }: Props = $props();

  // Default to first non-disabled tab
  $effect(() => {
    if (!activeTab) {
      const first = tabs.find((t) => !t.disabled);
      if (first) activeTab = first.id;
    }
  });

  function selectTab(tabId: string) {
    activeTab = tabId;
    onchange?.(tabId);
  }

  function handleKeydown(e: KeyboardEvent) {
    const enabledTabs = tabs.filter((t) => !t.disabled);
    const currentIndex = enabledTabs.findIndex((t) => t.id === activeTab);
    let nextIndex = currentIndex;

    switch (e.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        e.preventDefault();
        nextIndex = (currentIndex + 1) % enabledTabs.length;
        break;
      case 'ArrowLeft':
      case 'ArrowUp':
        e.preventDefault();
        nextIndex = (currentIndex - 1 + enabledTabs.length) % enabledTabs.length;
        break;
      case 'Home':
        e.preventDefault();
        nextIndex = 0;
        break;
      case 'End':
        e.preventDefault();
        nextIndex = enabledTabs.length - 1;
        break;
      default:
        return;
    }

    selectTab(enabledTabs[nextIndex].id);
    // Focus the newly selected tab button
    const tabList = (e.currentTarget as HTMLElement).closest('[role="tablist"]');
    const buttons = tabList?.querySelectorAll<HTMLButtonElement>('[role="tab"]:not([disabled])');
    buttons?.[nextIndex]?.focus();
  }
</script>

<div class={className} data-testid="tabs">
  <div
    role="tablist"
    class="flex border-b border-[hsl(var(--color-border))]"
  >
    {#each tabs as tab (tab.id)}
      <button
        role="tab"
        id="tab-{tab.id}"
        aria-selected={activeTab === tab.id}
        aria-controls="tabpanel-{tab.id}"
        tabindex={activeTab === tab.id ? 0 : -1}
        disabled={tab.disabled}
        onclick={() => selectTab(tab.id)}
        onkeydown={handleKeydown}
        class="relative px-4 py-2 text-[length:var(--typography-fontSize-sm)] font-[var(--typography-fontWeight-medium)] transition-colors duration-[var(--motion-duration-fast)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[hsl(var(--color-ring))]
          {activeTab === tab.id
            ? 'text-[hsl(var(--color-accent))]'
            : 'text-[hsl(var(--color-fgMuted))] hover:text-[hsl(var(--color-fg))]'}
          disabled:opacity-50 disabled:cursor-not-allowed"
        data-testid="tab-{tab.id}"
      >
        {tab.label}
        {#if activeTab === tab.id}
          <span class="absolute bottom-0 left-0 right-0 h-0.5 bg-[hsl(var(--color-accent))]"></span>
        {/if}
      </button>
    {/each}
  </div>

  {#if panel}
    {#each tabs as tab (tab.id)}
      <div
        role="tabpanel"
        id="tabpanel-{tab.id}"
        aria-labelledby="tab-{tab.id}"
        hidden={activeTab !== tab.id}
        class="pt-4"
        data-testid="tabpanel-{tab.id}"
      >
        {#if activeTab === tab.id}
          {@render panel(tab.id)}
        {/if}
      </div>
    {/each}
  {/if}
</div>
