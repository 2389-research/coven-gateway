<script lang="ts">
  import Badge from './Badge.svelte';
  import Card from './Card.svelte';
  import EmptyState from './EmptyState.svelte';
  import Table from './Table.svelte';
  import TableHead from './TableHead.svelte';
  import TableBody from './TableBody.svelte';
  import TableRow from './TableRow.svelte';
  import TableHeader from './TableHeader.svelte';
  import TableCell from './TableCell.svelte';

  interface Tool {
    name: string;
    description: string;
    timeoutSeconds: number;
    requiredCapabilities: string[];
  }

  interface Pack {
    id: string;
    version: string;
    tools: Tool[];
  }

  interface Props {
    packs?: Pack[];
    csrfToken: string;
  }

  let { packs = [] as Pack[], csrfToken }: Props = $props();
  let loading = $state(false);

  let totalTools = $derived(packs.reduce((sum, p) => sum + p.tools.length, 0));

  async function refresh() {
    loading = true;
    try {
      const res = await fetch('/api/admin/tools');
      if (res.ok) {
        packs = await res.json();
      }
    } finally {
      loading = false;
    }
  }
</script>

<div data-testid="tools-page">
  <Card>
    {#snippet children()}
      <div class="px-6 py-4 border-b border-border flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div class="flex items-center gap-3">
          <h3 class="text-[length:var(--typography-fontSize-lg)] font-[var(--typography-fontWeight-semibold)] text-fg">
            Registered Tools
          </h3>
          <Badge variant="default" size="sm">
            {#snippet children()}{totalTools} tool{totalTools !== 1 ? 's' : ''}{/snippet}
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
        {#if packs.length === 0}
          <EmptyState
            heading="No tool packs registered"
            description="Packs and tools will appear here when agents register them."
          />
        {:else}
          <div class="space-y-6">
            {#each packs as pack (pack.id)}
              <div>
                <div class="flex items-center gap-2 mb-3">
                  <span class="font-[var(--typography-fontWeight-semibold)] text-fg">{pack.id}</span>
                  <Badge variant="accent" fill="outline" size="sm">
                    {#snippet children()}{pack.version}{/snippet}
                  </Badge>
                  <span class="text-[length:var(--typography-fontSize-xs)] text-fgMuted">
                    {pack.tools.length} tool{pack.tools.length !== 1 ? 's' : ''}
                  </span>
                </div>

                {#if pack.tools.length === 0}
                  <p class="text-[length:var(--typography-fontSize-sm)] text-fgMuted pl-2">No tools in this pack.</p>
                {:else}
                  <Table>
                    {#snippet children()}
                      <TableHead>
                        {#snippet children()}
                          <TableRow>
                            {#snippet children()}
                              <TableHeader>{#snippet children()}Tool{/snippet}</TableHeader>
                              <TableHeader>{#snippet children()}Description{/snippet}</TableHeader>
                            {/snippet}
                          </TableRow>
                        {/snippet}
                      </TableHead>
                      <TableBody>
                        {#snippet children()}
                          {#each pack.tools as tool (tool.name)}
                            <TableRow>
                              {#snippet children()}
                                <TableCell>
                                  {#snippet children()}
                                    <span class="font-[var(--typography-fontWeight-medium)] text-fg">{tool.name}</span>
                                  {/snippet}
                                </TableCell>
                                <TableCell>
                                  {#snippet children()}
                                    <span class="text-fgMuted">{tool.description || '—'}</span>
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
            {/each}
          </div>
        {/if}
      </div>
    {/snippet}
  </Card>
</div>
