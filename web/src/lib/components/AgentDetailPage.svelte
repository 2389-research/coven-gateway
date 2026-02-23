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
    ID: string;
    Name: string;
    Connected: boolean;
    WorkingDir: string;
    Capabilities: string[];
    Workspaces: string[];
    InstanceID: string;
    Backend: string;
  }

  interface ThreadItem {
    ID: string;
    FrontendName: string;
    AgentID: string;
    CreatedAt: string;
    UpdatedAt: string;
  }

  interface Props {
    agent: Agent;
    threads?: ThreadItem[];
    userName?: string;
    csrfToken: string;
  }

  let { agent, threads = [] as ThreadItem[], userName = '', csrfToken }: Props = $props();

  function formatTime(iso: string): string {
    if (!iso) return '\u2014';
    const d = new Date(iso);
    return d.toLocaleDateString('en-US', { month: 'short', day: '2-digit' }) +
      ' ' + d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false });
  }
</script>

<AdminLayout activePage="agents" {userName} {csrfToken}>
<div data-testid="agent-detail-page" class="space-y-6 p-6">
  <!-- Agent Info Card -->
  <Card>
    {#snippet children()}
      <div class="p-6 border-b border-border">
        <div class="flex items-start justify-between">
          <div class="flex items-center gap-4">
            <div class="relative">
              <div class="w-16 h-16 rounded-xl {agent.Connected ? 'bg-[var(--cg-success-subtleBg)]' : 'bg-surfaceRaised'} flex items-center justify-center">
                <svg class="w-8 h-8 {agent.Connected ? 'text-[var(--cg-success-subtleFg)]' : 'text-fgMuted'}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/>
                </svg>
              </div>
              {#if agent.Connected}
                <div class="absolute -bottom-1 -right-1 w-4 h-4 bg-[var(--cg-success-solidBg)] rounded-full border-2 border-surface"></div>
              {/if}
            </div>
            <div>
              <h2 class="text-[length:var(--typography-fontSize-2xl)] font-[var(--typography-fontWeight-semibold)] text-fg">{agent.Name}</h2>
              <CodeText class="text-[length:var(--typography-fontSize-sm)] text-fgMuted mt-1">
                {#snippet children()}{agent.ID}{/snippet}
              </CodeText>
              <div class="flex items-center gap-2 mt-2">
                <Badge variant={agent.Connected ? 'success' : 'default'} size="sm">
                  {#snippet children()}{agent.Connected ? 'Online' : 'Offline'}{/snippet}
                </Badge>
                {#if agent.Backend}
                  <Badge variant="accent" fill="outline" size="sm">
                    {#snippet children()}{agent.Backend}{/snippet}
                  </Badge>
                {/if}
              </div>
            </div>
          </div>
          {#if agent.Connected}
            <a
              href="/?agent={agent.ID}"
              class="px-5 py-2.5 text-[length:var(--typography-fontSize-sm)] font-[var(--typography-fontWeight-medium)] bg-[var(--color-primary)] text-[var(--color-primaryFg)] rounded-[var(--border-radius-md)] hover:opacity-90 transition-opacity"
            >
              Start Chat
            </a>
          {/if}
        </div>
      </div>

      <!-- Agent Details Grid -->
      <div class="p-6 grid grid-cols-1 md:grid-cols-2 gap-6">
        <div>
          <h3 class="text-[length:var(--typography-fontSize-xs)] font-[var(--typography-fontWeight-semibold)] text-fgMuted uppercase tracking-wider mb-3">Connection Info</h3>
          <dl class="space-y-3">
            <div>
              <dt class="text-[length:var(--typography-fontSize-xs)] text-fgMuted">Status</dt>
              <dd class="text-[length:var(--typography-fontSize-sm)] text-fg font-[var(--typography-fontWeight-medium)]">{agent.Connected ? 'Connected' : 'Disconnected'}</dd>
            </div>
            {#if agent.InstanceID}
              <div>
                <dt class="text-[length:var(--typography-fontSize-xs)] text-fgMuted">Instance ID</dt>
                <dd class="text-[length:var(--typography-fontSize-sm)] text-fg font-mono">{agent.InstanceID}</dd>
              </div>
            {/if}
            {#if agent.WorkingDir}
              <div>
                <dt class="text-[length:var(--typography-fontSize-xs)] text-fgMuted">Working Directory</dt>
                <dd class="text-[length:var(--typography-fontSize-sm)] text-fg font-mono">{agent.WorkingDir}</dd>
              </div>
            {/if}
            {#if agent.Backend}
              <div>
                <dt class="text-[length:var(--typography-fontSize-xs)] text-fgMuted">Backend</dt>
                <dd class="text-[length:var(--typography-fontSize-sm)] text-fg font-[var(--typography-fontWeight-medium)]">{agent.Backend}</dd>
              </div>
            {/if}
          </dl>
        </div>
        <div>
          <h3 class="text-[length:var(--typography-fontSize-xs)] font-[var(--typography-fontWeight-semibold)] text-fgMuted uppercase tracking-wider mb-3">Capabilities</h3>
          {#if agent.Capabilities.length > 0}
            <div class="flex flex-wrap gap-2 mb-6">
              {#each agent.Capabilities as cap}
                <Badge variant="success" fill="outline" size="sm">
                  {#snippet children()}{cap}{/snippet}
                </Badge>
              {/each}
            </div>
          {:else}
            <p class="text-[length:var(--typography-fontSize-sm)] text-fgMuted italic mb-6">No capabilities reported</p>
          {/if}

          <h3 class="text-[length:var(--typography-fontSize-xs)] font-[var(--typography-fontWeight-semibold)] text-fgMuted uppercase tracking-wider mb-3">Workspaces</h3>
          {#if agent.Workspaces.length > 0}
            <div class="space-y-1">
              {#each agent.Workspaces as ws}
                <p class="text-[length:var(--typography-fontSize-sm)] text-fg font-mono bg-surfaceRaised px-2 py-1 rounded-[var(--border-radius-sm)] border border-border">{ws}</p>
              {/each}
            </div>
          {:else}
            <p class="text-[length:var(--typography-fontSize-sm)] text-fgMuted italic">No workspaces configured</p>
          {/if}
        </div>
      </div>
    {/snippet}
  </Card>

  <!-- Threads Section -->
  <Card>
    {#snippet children()}
      <div class="px-6 py-4 border-b border-border flex items-center justify-between">
        <h3 class="text-[length:var(--typography-fontSize-lg)] font-[var(--typography-fontWeight-semibold)] text-fg">
          Conversation Threads
        </h3>
        <span class="text-[length:var(--typography-fontSize-xs)] text-fgMuted font-[var(--typography-fontWeight-medium)]">
          {threads.length} thread{threads.length !== 1 ? 's' : ''}
        </span>
      </div>
      <div class="p-6">
        {#if threads.length === 0}
          <EmptyState
            heading="No conversation threads yet"
            description="Threads will appear here once you start chatting with this agent."
          />
        {:else}
          <Table>
            {#snippet children()}
              <TableHead>
                {#snippet children()}
                  <TableRow>
                    {#snippet children()}
                      <TableHeader>{#snippet children()}Frontend{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Thread ID{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Updated{/snippet}</TableHeader>
                    {/snippet}
                  </TableRow>
                {/snippet}
              </TableHead>
              <TableBody>
                {#snippet children()}
                  {#each threads as t (t.ID)}
                    <TableRow>
                      {#snippet children()}
                        <TableCell>
                          {#snippet children()}
                            <a href="/admin/threads/{t.ID}" class="font-[var(--typography-fontWeight-medium)] text-fg hover:text-[var(--color-primary)] transition-colors">
                              {t.FrontendName}
                            </a>
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            <CodeText class="text-[length:var(--typography-fontSize-xs)]">
                              {#snippet children()}{t.ID}{/snippet}
                            </CodeText>
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            <span class="text-fgMuted">{formatTime(t.UpdatedAt)}</span>
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
