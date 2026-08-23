<script lang="ts">
  import AdminLayout from './AdminLayout.svelte';
  import Badge from './Badge.svelte';
  import Card from './Card.svelte';
  import EmptyState from './EmptyState.svelte';
  import DataTable from './DataTable.svelte';
  import type { DataColumn } from './dataTable.js';

  interface TodoItem {
    ID: string;
    AgentID: string;
    Description: string;
    Status: string;
    Priority: string;
    Notes: string;
    DueDate: string | null;
    CreatedAt: string;
    UpdatedAt: string;
  }

  interface Props {
    todos?: TodoItem[];
    userName?: string;
    csrfToken: string;
  }

  let { todos = [] as TodoItem[], userName = '', csrfToken }: Props = $props();
  let loading = $state(false);

  function formatTime(iso: string): string {
    if (!iso) return '—';
    const d = new Date(iso);
    return d.toLocaleDateString('en-US', { month: 'short', day: '2-digit' }) +
      ' ' + d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false });
  }

  function statusVariant(status: string): 'success' | 'warning' | 'default' | 'accent' {
    if (status === 'done' || status === 'completed') return 'success';
    if (status === 'in_progress' || status === 'active') return 'accent';
    if (status === 'blocked') return 'warning';
    return 'default';
  }

  function priorityVariant(priority: string): 'danger' | 'warning' | 'default' {
    if (priority === 'high' || priority === 'urgent') return 'danger';
    if (priority === 'medium') return 'warning';
    return 'default';
  }

  async function refresh() {
    loading = true;
    try {
      const res = await fetch('/api/admin/todos');
      if (res.ok) {
        const data = await res.json();
        todos = data.todos ?? [];
      }
    } finally {
      loading = false;
    }
  }

  const columns = $derived([
    { key: 'Description', header: 'Description', cell: descriptionCell },
    { key: 'AgentID', header: 'Agent', cell: agentCell },
    { key: 'Status', header: 'Status', cell: statusCell },
    { key: 'Priority', header: 'Priority', cell: priorityCell },
    { key: 'DueDate', header: 'Due', cell: dueCell },
    { key: 'CreatedAt', header: 'Created', cell: createdCell },
  ] satisfies DataColumn<TodoItem>[]);
</script>

{#snippet descriptionCell(todo: TodoItem)}
  <div class="max-w-md">
    <span class="text-fg">{todo.Description}</span>
    {#if todo.Notes}
      <p class="text-[length:var(--typography-fontSize-xs)] text-fgMuted mt-0.5 truncate">{todo.Notes}</p>
    {/if}
  </div>
{/snippet}

{#snippet agentCell(todo: TodoItem)}
  <span class="font-mono text-[length:var(--typography-fontSize-xs)]">{todo.AgentID || '—'}</span>
{/snippet}

{#snippet statusCell(todo: TodoItem)}
  <Badge variant={statusVariant(todo.Status)} size="sm">
    {#snippet children()}{todo.Status}{/snippet}
  </Badge>
{/snippet}

{#snippet priorityCell(todo: TodoItem)}
  {#if todo.Priority}
    <Badge variant={priorityVariant(todo.Priority)} size="sm">
      {#snippet children()}{todo.Priority}{/snippet}
    </Badge>
  {:else}
    <span class="text-fgMuted">{'—'}</span>
  {/if}
{/snippet}

{#snippet dueCell(todo: TodoItem)}
  <span class="text-fgMuted">{todo.DueDate ? formatTime(todo.DueDate) : '—'}</span>
{/snippet}

{#snippet createdCell(todo: TodoItem)}
  <span class="text-fgMuted whitespace-nowrap">{formatTime(todo.CreatedAt)}</span>
{/snippet}

<AdminLayout activePage="todos" {userName} {csrfToken}>
<div data-testid="todos-page" class="max-w-screen-xl mx-auto p-6">
  <Card>
    {#snippet children()}
      <div class="px-6 py-4 border-b border-border flex items-center justify-between">
        <div class="flex items-center gap-3">
          <h3 class="text-[length:var(--typography-fontSize-lg)] font-[var(--typography-fontWeight-semibold)] text-fg">
            All Todos
          </h3>
          <Badge variant="default" size="sm">
            {#snippet children()}{todos.length} item{todos.length !== 1 ? 's' : ''}{/snippet}
          </Badge>
        </div>
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
        {#if todos.length === 0}
          <EmptyState
            heading="No todos"
            description="Tasks created by agents will appear here."
          />
        {:else}
          <DataTable {columns} rows={todos} rowKey={(t) => t.ID} />
        {/if}
      </div>
    {/snippet}
  </Card>
</div>
</AdminLayout>
