<script lang="ts">
  import AdminLayout from './AdminLayout.svelte';
  import Badge from './Badge.svelte';
  import Card from './Card.svelte';
  import EmptyState from './EmptyState.svelte';
  import Table from './Table.svelte';
  import TableHead from './TableHead.svelte';
  import TableBody from './TableBody.svelte';
  import TableRow from './TableRow.svelte';
  import TableHeader from './TableHeader.svelte';
  import TableCell from './TableCell.svelte';

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
    if (!iso) return '\u2014';
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
</script>

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
          <Table>
            {#snippet children()}
              <TableHead>
                {#snippet children()}
                  <TableRow>
                    {#snippet children()}
                      <TableHeader>{#snippet children()}Description{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Agent{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Status{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Priority{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Due{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Created{/snippet}</TableHeader>
                    {/snippet}
                  </TableRow>
                {/snippet}
              </TableHead>
              <TableBody>
                {#snippet children()}
                  {#each todos as todo (todo.ID)}
                    <TableRow>
                      {#snippet children()}
                        <TableCell>
                          {#snippet children()}
                            <div class="max-w-md">
                              <span class="text-fg">{todo.Description}</span>
                              {#if todo.Notes}
                                <p class="text-[length:var(--typography-fontSize-xs)] text-fgMuted mt-0.5 truncate">{todo.Notes}</p>
                              {/if}
                            </div>
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            <span class="font-mono text-[length:var(--typography-fontSize-xs)]">{todo.AgentID || '\u2014'}</span>
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            <Badge variant={statusVariant(todo.Status)} size="sm">
                              {#snippet children()}{todo.Status}{/snippet}
                            </Badge>
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            {#if todo.Priority}
                              <Badge variant={priorityVariant(todo.Priority)} size="sm">
                                {#snippet children()}{todo.Priority}{/snippet}
                              </Badge>
                            {:else}
                              <span class="text-fgMuted">{'\u2014'}</span>
                            {/if}
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            <span class="text-fgMuted">{todo.DueDate ? formatTime(todo.DueDate) : '\u2014'}</span>
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            <span class="text-fgMuted whitespace-nowrap">{formatTime(todo.CreatedAt)}</span>
                          {/snippet}
                        </TableCell>
                      {/snippet}
                    </TableRow>
                  {/each}
                {/snippet}
              </TableBody>
            {/snippet}
          </Table>
        {/if}
      </div>
    {/snippet}
  </Card>
</div>
</AdminLayout>
