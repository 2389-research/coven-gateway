<script lang="ts">
  import AdminLayout from './AdminLayout.svelte';
  import Card from './Card.svelte';
  import CodeText from './CodeText.svelte';
  import EmptyState from './EmptyState.svelte';
  import Table from './Table.svelte';
  import TableHead from './TableHead.svelte';
  import TableBody from './TableBody.svelte';
  import TableRow from './TableRow.svelte';
  import TableHeader from './TableHeader.svelte';
  import TableCell from './TableCell.svelte';

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
</script>

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
          <Table>
            {#snippet children()}
              <TableHead>
                {#snippet children()}
                  <TableRow>
                    {#snippet children()}
                      <TableHeader>{#snippet children()}Thread{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Agent{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Frontend{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Updated{/snippet}</TableHeader>
                      <TableHeader align="right">{#snippet children()}Actions{/snippet}</TableHeader>
                    {/snippet}
                  </TableRow>
                {/snippet}
              </TableHead>
              <TableBody>
                {#snippet children()}
                  {#each threads as thread (thread.ID)}
                    <TableRow>
                      {#snippet children()}
                        <TableCell>
                          {#snippet children()}
                            <CodeText class="text-[length:var(--typography-fontSize-xs)]">
                              {#snippet children()}{truncateId(thread.ID)}{/snippet}
                            </CodeText>
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            <span class="text-fgMuted">{truncateId(thread.AgentID)}</span>
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            <span class="font-[var(--typography-fontWeight-medium)] text-fg">{thread.FrontendName}</span>
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            <span class="text-fgMuted">{formatTime(thread.UpdatedAt)}</span>
                          {/snippet}
                        </TableCell>
                        <TableCell align="right">
                          {#snippet children()}
                            <a
                              href="/admin/threads/{thread.ID}"
                              class="text-[length:var(--typography-fontSize-sm)] text-accent hover:underline"
                            >
                              View
                            </a>
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
