<script lang="ts">
  import Dialog from './Dialog.svelte';
  import Tabs from './Tabs.svelte';

  interface Props {
    open: boolean;
    csrfToken?: string;
    class?: string;
  }

  let { open = $bindable(false), csrfToken = '', class: className = '' }: Props = $props();

  const tabs = [
    { id: 'agents', label: 'Agents' },
    { id: 'tools', label: 'Tools' },
    { id: 'security', label: 'Security' },
    { id: 'help', label: 'Help' },
  ];

  let activeTab = $state('agents');
  let tabContent = $state<Record<string, string>>({});
  let loading = $state<Record<string, boolean>>({});
  let panelEl: HTMLDivElement | undefined = $state();

  async function loadTab(tabId: string) {
    if (tabContent[tabId]) {
      // Already loaded — re-process HTMX
      requestAnimationFrame(() => processHtmx());
      return;
    }
    loading[tabId] = true;
    try {
      const resp = await fetch(`/settings/${tabId}`);
      if (resp.ok) {
        tabContent[tabId] = await resp.text();
        // Process HTMX after DOM update
        requestAnimationFrame(() => processHtmx());
      }
    } catch {
      tabContent[tabId] = '<p class="text-fgMuted">Failed to load settings.</p>';
    } finally {
      loading[tabId] = false;
    }
  }

  function processHtmx() {
    if (!panelEl) return;
    // Initialize HTMX on dynamically inserted content
    const htmx = (window as any).htmx;
    if (htmx?.process) {
      htmx.process(panelEl);
    }
  }

  function handleTabChange(tabId: string) {
    activeTab = tabId;
    loadTab(tabId);
  }

  // Load initial tab when opened
  $effect(() => {
    if (open) {
      loadTab(activeTab);
    }
  });

  // Keyboard shortcut: Cmd/Ctrl+K to toggle
  function handleKeydown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault();
      open = !open;
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<Dialog bind:open class={className}>
  {#snippet header()}
    <h2 class="text-[length:var(--typography-fontSize-lg)] font-[var(--typography-fontWeight-semibold)] text-fg">
      Settings
    </h2>
  {/snippet}

  <div class="min-h-[400px]" data-testid="settings-modal">
    <Tabs {tabs} {activeTab} onchange={handleTabChange}>
      {#snippet panel(currentTab)}
        <div bind:this={panelEl} class="py-4">
          {#if loading[currentTab]}
            <div class="flex items-center justify-center py-8">
              <span class="text-[length:var(--typography-fontSize-sm)] text-fgMuted">Loading...</span>
            </div>
          {:else if tabContent[currentTab]}
            {@html tabContent[currentTab]}
          {:else}
            <div class="flex items-center justify-center py-8">
              <span class="text-[length:var(--typography-fontSize-sm)] text-fgMuted">No content</span>
            </div>
          {/if}
        </div>
      {/snippet}
    </Tabs>
  </div>
</Dialog>
