<script lang="ts">
  import AdminLayout from './AdminLayout.svelte';
  import Badge from './Badge.svelte';
  import Card from './Card.svelte';
  import CodeText from './CodeText.svelte';
  import EmptyState from './EmptyState.svelte';
  import DataTable from './DataTable.svelte';
  import type { DataColumn } from './dataTable.js';

  interface Agent {
    id: string;
    name: string;
    connected: boolean;
  }

  interface Props {
    agents?: Agent[];
    userName?: string;
    csrfToken: string;
  }

  let { agents = [] as Agent[], userName = '', csrfToken }: Props = $props();
  let loading = $state(false);

  async function refresh() {
    loading = true;
    try {
      const res = await fetch('/api/agents');
      if (res.ok) {
        agents = await res.json();
      }
    } finally {
      loading = false;
    }
  }

  const columns = $derived([
    { key: 'name', header: 'Name', cell: nameCell },
    { key: 'connected', header: 'Status', cell: statusCell },
    { key: 'actions', header: 'Actions', align: 'right' as const, cell: actionsCell },
  ] satisfies DataColumn<Agent>[]);
</script>

{#snippet nameCell(agent: Agent)}
  <div>
    <div class="font-[var(--typography-fontWeight-medium)] text-fg">{agent.name}</div>
    <CodeText class="text-[length:var(--typography-fontSize-xs)] text-fgMuted mt-0.5">
      {#snippet children()}{agent.id}{/snippet}
    </CodeText>
  </div>
{/snippet}

{#snippet statusCell(agent: Agent)}
  <Badge variant={agent.connected ? 'success' : 'default'} size="sm">
    {#snippet children()}{agent.connected ? 'Online' : 'Offline'}{/snippet}
  </Badge>
{/snippet}

{#snippet actionsCell(agent: Agent)}
  <a href="/admin/agents/{agent.id}" class="text-[length:var(--typography-fontSize-sm)] text-accent hover:underline">
    Details
  </a>
{/snippet}

<AdminLayout activePage="agents" {userName} {csrfToken}>
<div data-testid="agents-page" class="p-6">
  <Card>
    {#snippet children()}
      <div class="px-6 py-4 border-b border-border flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <h3 class="text-[length:var(--typography-fontSize-lg)] font-[var(--typography-fontWeight-semibold)] text-fg">
          Connected Agents
        </h3>
        <button
          type="button"
          class="text-[length:var(--typography-fontSize-sm)] text-fgMuted hover:text-fg"
          onclick={refresh}
          disabled={loading}
        >
          {loading ? 'Refreshing...' : 'Refresh'}
        </button>
      </div>

      <div class="p-6">
        {#if agents.length === 0}
          <EmptyState
            heading="No agents connected"
            description="Agents will appear here when they connect to the gateway."
          />
        {:else}
          <DataTable {columns} rows={agents} rowKey={(a) => a.id} />
        {/if}
      </div>
    {/snippet}
  </Card>
</div>
</AdminLayout>
