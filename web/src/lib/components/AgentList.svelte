<script lang="ts">
  interface Agent {
    id: string;
    name: string;
    connected: boolean;
  }

  interface Props {
    activeAgentId?: string;
    onSelect?: (agent: Agent) => void;
    pollInterval?: number;
    class?: string;
  }

  let {
    activeAgentId = '',
    onSelect,
    pollInterval = 5000,
    class: className = '',
  }: Props = $props();

  let agents = $state<Agent[]>([]);
  let loading = $state(true);

  async function fetchAgents() {
    try {
      const resp = await fetch('/api/agents');
      if (resp.ok) {
        agents = await resp.json();
      }
    } catch {
      // Silently retry on next poll
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    fetchAgents();
    const id = setInterval(fetchAgents, pollInterval);
    return () => clearInterval(id);
  });
</script>

<nav class="flex flex-col {className}" data-testid="agent-list" aria-label="Agent list">
  <div class="px-3 py-2">
    <h3 class="text-[length:var(--typography-fontSize-xs)] font-[var(--typography-fontWeight-semibold)] uppercase tracking-wider text-fgMuted">
      Agents
    </h3>
  </div>

  {#if loading}
    <div class="px-3 py-4 text-center">
      <span class="text-[length:var(--typography-fontSize-sm)] text-fgMuted">Loading...</span>
    </div>
  {:else if agents.length === 0}
    <div class="flex flex-col items-center px-3 py-6 text-center" data-testid="agent-list-empty">
      <svg class="mb-2 h-8 w-8 text-fgMuted opacity-40" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/>
      </svg>
      <p class="text-[length:var(--typography-fontSize-sm)] text-fgMuted">No agents connected</p>
      <p class="mt-1 text-[length:var(--typography-fontSize-xs)] text-fgMuted/70">Launch an agent to start chatting</p>
    </div>
  {:else}
    <ul class="flex flex-col gap-0.5 px-1.5">
      {#each agents as agent (agent.id)}
        <li>
          <button
            type="button"
            onclick={() => onSelect?.(agent)}
            class="flex w-full items-center gap-3 rounded-[var(--border-radius-md)] px-2.5 py-2 text-left transition-colors duration-[var(--motion-duration-fast)]
              {agent.id === activeAgentId
                ? 'bg-accent/10 text-accent'
                : 'text-fg hover:bg-surfaceHover'}"
            data-testid="agent-list-item"
            data-agent-id={agent.id}
            aria-current={agent.id === activeAgentId ? 'true' : undefined}
          >
            <div class="relative shrink-0">
              <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/>
              </svg>
              {#if agent.connected}
                <span class="absolute -bottom-0.5 -right-0.5 h-2 w-2 rounded-full bg-success-solidBg ring-2 ring-surface"></span>
              {/if}
            </div>
            <span class="truncate text-[length:var(--typography-fontSize-sm)] font-[var(--typography-fontWeight-medium)]">
              {agent.name}
            </span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</nav>
