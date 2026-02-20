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

  interface Agent {
    id: string;
    name: string;
    connected: boolean;
  }

  interface Tool {
    name: string;
    description: string;
  }

  interface Pack {
    id: string;
    version: string;
    tools: Tool[];
  }

  interface UsageStats {
    totalInput: number;
    totalOutput: number;
    totalCacheRead: number;
    totalCacheWrite: number;
    totalThinking: number;
    totalTokens: number;
    requestCount: number;
  }

  interface Props {
    agentCount?: number;
    packCount?: number;
    threadCount?: number;
    usage?: UsageStats;
    agents?: Agent[];
    packs?: Pack[];
    csrfToken: string;
  }

  let {
    agentCount = 0,
    packCount = 0,
    threadCount = 0,
    usage = {
      totalInput: 0,
      totalOutput: 0,
      totalCacheRead: 0,
      totalCacheWrite: 0,
      totalThinking: 0,
      totalTokens: 0,
      requestCount: 0,
    } as UsageStats,
    agents = [] as Agent[],
    packs = [] as Pack[],
    csrfToken,
  }: Props = $props();

  let loading = $state(false);

  async function refresh() {
    loading = true;
    try {
      const res = await fetch('/api/admin/dashboard');
      if (res.ok) {
        const data = await res.json();
        agentCount = data.agentCount;
        packCount = data.packCount;
        threadCount = data.threadCount;
        usage = data.usage;
        agents = data.agents;
        packs = data.packs;
      }
    } finally {
      loading = false;
    }
  }

  function formatNumber(n: number): string {
    if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
    if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
    return n.toString();
  }

  let totalPackTools = $derived(packs.reduce((sum, p) => sum + p.tools.length, 0));
</script>

<div data-testid="dashboard-page" class="space-y-6">
  <!-- Stats Grid -->
  <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
    <Card>
      {#snippet children()}
        <div class="p-5">
          <div class="flex items-center justify-between mb-2">
            <span class="text-[length:var(--typography-fontSize-sm)] text-fgMuted">Agents Online</span>
          </div>
          <p class="text-3xl font-[var(--typography-fontWeight-bold)] text-fg">{agentCount}</p>
          <p class="text-[length:var(--typography-fontSize-xs)] text-fgMuted mt-1">Active connections</p>
        </div>
      {/snippet}
    </Card>

    <Card>
      {#snippet children()}
        <div class="p-5">
          <div class="flex items-center justify-between mb-2">
            <span class="text-[length:var(--typography-fontSize-sm)] text-fgMuted">Active Threads</span>
          </div>
          <p class="text-3xl font-[var(--typography-fontWeight-bold)] text-fg">{threadCount}</p>
          <p class="text-[length:var(--typography-fontSize-xs)] text-fgMuted mt-1">Conversations</p>
        </div>
      {/snippet}
    </Card>

    <Card>
      {#snippet children()}
        <div class="p-5">
          <div class="flex items-center justify-between mb-2">
            <span class="text-[length:var(--typography-fontSize-sm)] text-fgMuted">Tool Packs</span>
          </div>
          <p class="text-3xl font-[var(--typography-fontWeight-bold)] text-fg">{packCount}</p>
          <p class="text-[length:var(--typography-fontSize-xs)] text-fgMuted mt-1">{totalPackTools} tool{totalPackTools !== 1 ? 's' : ''} total</p>
        </div>
      {/snippet}
    </Card>

    <Card>
      {#snippet children()}
        <div class="p-5">
          <div class="flex items-center justify-between mb-2">
            <span class="text-[length:var(--typography-fontSize-sm)] text-fgMuted">Total Requests</span>
          </div>
          <p class="text-3xl font-[var(--typography-fontWeight-bold)] text-fg">{formatNumber(usage.requestCount)}</p>
          <p class="text-[length:var(--typography-fontSize-xs)] text-fgMuted mt-1">API calls</p>
        </div>
      {/snippet}
    </Card>
  </div>

  <!-- Token Usage Panel -->
  <Card>
    {#snippet children()}
      <div class="px-6 py-4 border-b border-border flex items-center justify-between">
        <h3 class="text-[length:var(--typography-fontSize-lg)] font-[var(--typography-fontWeight-semibold)] text-fg">
          Token Usage
        </h3>
        <div class="flex items-center gap-4">
          <a href="/admin/usage" class="text-[length:var(--typography-fontSize-sm)] text-accent hover:underline">
            View Details
          </a>
          <button
            type="button"
            class="text-[length:var(--typography-fontSize-sm)] text-fgMuted hover:text-fg"
            onclick={refresh}
            disabled={loading}
          >
            {loading ? 'Refreshing...' : 'Refresh'}
          </button>
        </div>
      </div>

      <div class="p-6">
        <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-4">
          <div>
            <p class="text-[length:var(--typography-fontSize-xs)] text-fgMuted uppercase tracking-wide mb-1">Total</p>
            <p class="text-xl font-[var(--typography-fontWeight-bold)] text-fg">{formatNumber(usage.totalTokens)}</p>
          </div>
          <div>
            <p class="text-[length:var(--typography-fontSize-xs)] text-fgMuted uppercase tracking-wide mb-1">Input</p>
            <p class="text-xl font-[var(--typography-fontWeight-bold)] text-fg">{formatNumber(usage.totalInput)}</p>
          </div>
          <div>
            <p class="text-[length:var(--typography-fontSize-xs)] text-fgMuted uppercase tracking-wide mb-1">Output</p>
            <p class="text-xl font-[var(--typography-fontWeight-bold)] text-fg">{formatNumber(usage.totalOutput)}</p>
          </div>
          <div>
            <p class="text-[length:var(--typography-fontSize-xs)] text-fgMuted uppercase tracking-wide mb-1">Cache Read</p>
            <p class="text-xl font-[var(--typography-fontWeight-bold)] text-fg">{formatNumber(usage.totalCacheRead)}</p>
          </div>
          <div>
            <p class="text-[length:var(--typography-fontSize-xs)] text-fgMuted uppercase tracking-wide mb-1">Thinking</p>
            <p class="text-xl font-[var(--typography-fontWeight-bold)] text-fg">{formatNumber(usage.totalThinking)}</p>
          </div>
          <div>
            <p class="text-[length:var(--typography-fontSize-xs)] text-fgMuted uppercase tracking-wide mb-1">Requests</p>
            <p class="text-xl font-[var(--typography-fontWeight-bold)] text-fg">{formatNumber(usage.requestCount)}</p>
          </div>
        </div>
      </div>
    {/snippet}
  </Card>

  <!-- Connected Agents -->
  <Card>
    {#snippet children()}
      <div class="px-6 py-4 border-b border-border">
        <h3 class="text-[length:var(--typography-fontSize-lg)] font-[var(--typography-fontWeight-semibold)] text-fg">
          Connected Agents
        </h3>
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
                            <span class="font-[var(--typography-fontWeight-medium)] text-fg">{agent.name}</span>
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

  <!-- Connected Packs -->
  <Card>
    {#snippet children()}
      <div class="px-6 py-4 border-b border-border flex items-center gap-3">
        <h3 class="text-[length:var(--typography-fontSize-lg)] font-[var(--typography-fontWeight-semibold)] text-fg">
          Tool Packs
        </h3>
        <Badge variant="default" size="sm">
          {#snippet children()}{totalPackTools} tool{totalPackTools !== 1 ? 's' : ''}{/snippet}
        </Badge>
      </div>

      <div class="p-6">
        {#if packs.length === 0}
          <EmptyState
            heading="No tool packs registered"
            description="Packs and tools will appear here when agents register them."
          />
        {:else}
          <div class="space-y-4">
            {#each packs as pack (pack.id)}
              <div>
                <div class="flex items-center gap-2 mb-2">
                  <span class="font-[var(--typography-fontWeight-semibold)] text-fg">{pack.id}</span>
                  <Badge variant="accent" fill="outline" size="sm">
                    {#snippet children()}{pack.version}{/snippet}
                  </Badge>
                  <span class="text-[length:var(--typography-fontSize-xs)] text-fgMuted">
                    {pack.tools.length} tool{pack.tools.length !== 1 ? 's' : ''}
                  </span>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {/snippet}
  </Card>
</div>
