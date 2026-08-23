<script lang="ts">
  import AdminLayout from './AdminLayout.svelte';
  import Card from './Card.svelte';
  import CodeText from './CodeText.svelte';
  import EmptyState from './EmptyState.svelte';
  import DataTable from './DataTable.svelte';
  import type { DataColumn } from './dataTable.js';

  interface Thread {
    ID: string;
    FrontendName: string;
    ExternalID: string;
    AgentID: string;
    CreatedAt: string;
    UpdatedAt: string;
  }

  interface Props {
    threads?: Thread[];
    userName?: string;
    csrfToken: string;
  }

  let { threads = [] as Thread[], userName = '', csrfToken }: Props = $props();
  let loading = $state(false);

  async function refresh() {
    loading = true;
    try {
      const res = await fetch('/api/admin/threads');
      if (res.ok) {
        threads = await res.json();
      }
    } finally {
      loading = false;
    }
  }

  function formatTime(iso: string): string {
    if (!iso) return '—';
    const d = new Date(iso);
    return d.toLocaleDateString('en-US', { month: 'short', day: '2-digit' }) +
      ' ' + d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false });
  }

  function truncateId(id: string): string {
    return id.length > 12 ? id.slice(0, 12) + '...' : id;
  }

  const columns = $derived([
    { key: 'ID', header: 'Thread', cell: threadCell },
    { key: 'AgentID', header: 'Agent', cell: agentCell },
    { key: 'FrontendName', header: 'Frontend', cell: frontendCell },
    { key: 'UpdatedAt', header: 'Updated', cell: updatedCell },
    { key: 'actions', header: 'Actions', align: 'right' as const, cell: actionsCell },
  ] satisfies DataColumn<Thread>[]);
</script>

{#snippet threadCell(thread: Thread)}
  <CodeText class="text-[length:var(--typography-fontSize-xs)]">
    {#snippet children()}{truncateId(thread.ID)}{/snippet}
  </CodeText>
{/snippet}

{#snippet agentCell(thread: Thread)}
  <span class="text-fgMuted">{truncateId(thread.AgentID)}</span>
{/snippet}

{#snippet frontendCell(thread: Thread)}
  <span class="font-[var(--typography-fontWeight-medium)] text-fg">{thread.FrontendName}</span>
{/snippet}

{#snippet updatedCell(thread: Thread)}
  <span class="text-fgMuted">{formatTime(thread.UpdatedAt)}</span>
{/snippet}

{#snippet actionsCell(thread: Thread)}
  <a
    href="/admin/threads/{thread.ID}"
    class="text-[length:var(--typography-fontSize-sm)] text-accent hover:underline"
  >
    View
  </a>
{/snippet}

<AdminLayout activePage="threads" {userName} {csrfToken}>
<div data-testid="threads-page" class="p-6">
  <Card>
    {#snippet children()}
      <div class="px-6 py-4 border-b border-border flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <h3 class="text-[length:var(--typography-fontSize-lg)] font-[var(--typography-fontWeight-semibold)] text-fg">
          Conversation Threads
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
        {#if threads.length === 0}
          <EmptyState
            heading="No threads yet"
            description="Conversations will appear here when clients interact with agents."
          />
        {:else}
          <DataTable {columns} rows={threads} rowKey={(t) => t.ID} />
        {/if}
      </div>
    {/snippet}
  </Card>
</div>
</AdminLayout>
