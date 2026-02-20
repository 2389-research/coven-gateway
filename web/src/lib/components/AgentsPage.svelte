<script lang="ts">
  import AdminLayout from './AdminLayout.svelte';
  import Badge from './Badge.svelte';
  import Card from './Card.svelte';
  import CodeText from './CodeText.svelte';
  import EmptyState from './EmptyState.svelte';
  import Table from './Table.svelte';
  import TableHead from './TableHead.svelte';
  import TableBody from './TableBody.svelte';
  import TableRow from './TableRow.svelte';
  import TableHeader from './TableHeader.svelte';
  import TableCell from './TableCell.svelte';

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
</script>

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
          <Table>
            {#snippet children()}
              <TableHead>
                {#snippet children()}
                  <TableRow>
                    {#snippet children()}
                      <TableHeader>{#snippet children()}Name{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Status{/snippet}</TableHeader>
                      <TableHeader align="right">{#snippet children()}Actions{/snippet}</TableHeader>
                    {/snippet}
                  </TableRow>
                {/snippet}
              </TableHead>
              <TableBody>
                {#snippet children()}
                  {#each agents as agent (agent.id)}
                    <TableRow>
                      {#snippet children()}
                        <TableCell>
                          {#snippet children()}
                            <div>
                              <div class="font-[var(--typography-fontWeight-medium)] text-fg">{agent.name}</div>
                              <CodeText class="text-[length:var(--typography-fontSize-xs)] text-fgMuted mt-0.5">
                                {#snippet children()}{agent.id}{/snippet}
                              </CodeText>
                            </div>
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            <Badge variant={agent.connected ? 'success' : 'default'} size="sm">
                              {#snippet children()}{agent.connected ? 'Online' : 'Offline'}{/snippet}
                            </Badge>
                          {/snippet}
                        </TableCell>
                        <TableCell align="right">
                          {#snippet children()}
                            <a
                              href="/admin/agents/{agent.id}"
                              class="text-[length:var(--typography-fontSize-sm)] text-accent hover:underline"
                            >
                              Details
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
