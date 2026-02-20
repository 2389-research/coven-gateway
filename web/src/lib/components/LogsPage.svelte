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

  interface LogEntry {
    ID: string;
    AgentID: string;
    Message: string;
    Tags: string[];
    CreatedAt: string;
  }

  interface Props {
    entries?: LogEntry[];
    userName?: string;
    csrfToken: string;
  }

  let { entries = [] as LogEntry[], userName = '', csrfToken }: Props = $props();
  let loading = $state(false);
  let searchQuery = $state('');

  function formatTime(iso: string): string {
    if (!iso) return '\u2014';
    const d = new Date(iso);
    return d.toLocaleDateString('en-US', { month: 'short', day: '2-digit' }) +
      ' ' + d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
  }

  async function search() {
    loading = true;
    try {
      const params = new URLSearchParams();
      if (searchQuery) params.set('q', searchQuery);
      const res = await fetch(`/api/admin/logs?${params}`);
      if (res.ok) {
        const data = await res.json();
        entries = data.entries ?? [];
      }
    } finally {
      loading = false;
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') search();
  }
</script>

<AdminLayout activePage="logs" {userName} {csrfToken}>
<div data-testid="logs-page" class="max-w-screen-xl mx-auto space-y-6 p-6">
  <!-- Search -->
  <Card>
    {#snippet children()}
      <div class="p-4">
        <div class="flex gap-4">
          <input
            type="text"
            placeholder="Search logs..."
            bind:value={searchQuery}
            onkeydown={handleKeydown}
            class="flex-1 px-4 py-2 border border-border rounded-[var(--border-radius-md)] bg-surface text-fg text-[length:var(--typography-fontSize-sm)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]/20 focus:border-[var(--color-primary)]"
          />
          <button
            type="button"
            onclick={search}
            disabled={loading}
            class="px-4 py-2 bg-[var(--color-primary)] text-[var(--color-primaryFg)] rounded-[var(--border-radius-md)] text-[length:var(--typography-fontSize-sm)] font-[var(--typography-fontWeight-medium)] hover:opacity-90 transition-opacity disabled:opacity-50"
          >
            {loading ? 'Searching...' : 'Search'}
          </button>
        </div>
      </div>
    {/snippet}
  </Card>

  <!-- Log Entries Table -->
  <Card>
    {#snippet children()}
      <div class="px-6 py-4 border-b border-border flex items-center justify-between">
        <h3 class="text-[length:var(--typography-fontSize-lg)] font-[var(--typography-fontWeight-semibold)] text-fg">
          Log Entries
        </h3>
        <span class="text-[length:var(--typography-fontSize-xs)] text-fgMuted font-[var(--typography-fontWeight-medium)]">
          {entries.length} entr{entries.length !== 1 ? 'ies' : 'y'}
        </span>
      </div>

      <div class="p-6">
        {#if entries.length === 0}
          <EmptyState
            heading="No log entries"
            description="Activity log entries from agents will appear here."
          />
        {:else}
          <Table>
            {#snippet children()}
              <TableHead>
                {#snippet children()}
                  <TableRow>
                    {#snippet children()}
                      <TableHeader>{#snippet children()}Time{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Agent{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Message{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Tags{/snippet}</TableHeader>
                    {/snippet}
                  </TableRow>
                {/snippet}
              </TableHead>
              <TableBody>
                {#snippet children()}
                  {#each entries as entry (entry.ID)}
                    <TableRow>
                      {#snippet children()}
                        <TableCell>
                          {#snippet children()}
                            <span class="text-fgMuted whitespace-nowrap">{formatTime(entry.CreatedAt)}</span>
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            <span class="font-mono text-[length:var(--typography-fontSize-xs)]">{entry.AgentID || '\u2014'}</span>
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            <span class="text-fg">{entry.Message}</span>
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            <div class="flex flex-wrap gap-1">
                              {#each entry.Tags as tag}
                                <Badge variant="default" size="sm">
                                  {#snippet children()}{tag}{/snippet}
                                </Badge>
                              {/each}
                            </div>
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
