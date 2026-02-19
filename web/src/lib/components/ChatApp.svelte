<script lang="ts">
  import ChatThread from './ChatThread.svelte';
  import ChatInput from './ChatInput.svelte';
  import ThinkingIndicator from './ThinkingIndicator.svelte';
  import AgentList from './AgentList.svelte';
  import SettingsModal from './SettingsModal.svelte';
  import Button from './Button.svelte';
  import IconButton from './IconButton.svelte';
  import StatusDot from './StatusDot.svelte';
  import { createChatStream, type ChatStream } from '../stores/chat.svelte';

  interface Props {
    agentId?: string;
    agentName?: string;
    csrfToken: string;
    class?: string;
  }

  let { agentId: initialAgentId, agentName: initialAgentName, csrfToken, class: className = '' }: Props = $props();

  let activeAgentId = $state(initialAgentId ?? '');
  let activeAgentName = $state(initialAgentName ?? '');
  let chat = $state<ChatStream | null>(null);
  let isSending = $state(false);
  let sidebarOpen = $state(true);
  let settingsOpen = $state(false);

  function connectToAgent(id: string, name: string) {
    // Close existing stream
    chat?.close();
    activeAgentId = id;
    activeAgentName = name;
    chat = createChatStream(id);
  }

  // Connect to initial agent if provided
  if (initialAgentId) {
    chat = createChatStream(initialAgentId);
  }

  // Cleanup on unmount
  $effect(() => {
    return () => {
      chat?.close();
    };
  });

  function handleAgentSelect(agent: { id: string; name: string }) {
    if (agent.id === activeAgentId) return;
    connectToAgent(agent.id, agent.name);
  }

  async function handleSend(text: string) {
    if (!activeAgentId) return;
    isSending = true;
    try {
      const form = new URLSearchParams();
      form.set('message', text);
      form.set('csrf_token', csrfToken);

      const resp = await fetch(`/chat/${encodeURIComponent(activeAgentId)}/send`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: form.toString(),
      });
      if (!resp.ok) {
        console.error('[chat] send failed:', resp.status, await resp.text());
      }
    } catch (e) {
      console.error('[chat] send error:', e);
    } finally {
      isSending = false;
    }
  }
</script>

<div class="flex h-full {className}" data-testid="chat-app">
  <!-- Sidebar -->
  <aside
    class="flex w-60 shrink-0 flex-col border-r border-border bg-surface transition-[margin] duration-[var(--motion-duration-normal)]
      {sidebarOpen ? '' : '-ml-60'}"
    data-testid="chat-sidebar"
  >
    <div class="flex items-center justify-between border-b border-border px-3 py-3">
      <a href="/" class="flex items-center gap-2">
        <div class="flex h-7 w-7 items-center justify-center rounded-md bg-accent">
          <svg class="h-4 w-4 text-fgOnAccent" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z"/>
          </svg>
        </div>
        <span class="text-[length:var(--typography-fontSize-sm)] font-[var(--typography-fontWeight-semibold)] text-fg">Coven</span>
      </a>
    </div>
    <AgentList
      {activeAgentId}
      onSelect={handleAgentSelect}
      class="flex-1 overflow-y-auto py-2"
    />
    <div class="border-t border-border px-3 py-2">
      <Button
        variant="ghost"
        size="sm"
        onclick={() => settingsOpen = true}
        aria-label="Settings"
        class="w-full justify-start text-fgMuted hover:text-fg"
      >
        {#snippet children()}
          <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"/>
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/>
          </svg>
          <span class="text-[length:var(--typography-fontSize-sm)]">Settings</span>
          <kbd class="ml-auto rounded-[var(--border-radius-xs)] border border-border bg-surfaceAlt px-1 py-0.5 text-[length:0.625rem] font-[family-name:var(--typography-fontFamily-mono)] text-fgMuted">
            ⌘K
          </kbd>
        {/snippet}
      </Button>
    </div>
  </aside>

  <!-- Main chat area -->
  <div class="flex flex-1 flex-col min-w-0">
    {#if activeAgentId && chat}
      <!-- Chat header -->
      <div class="flex items-center gap-3 border-b border-border bg-surface px-4 py-3">
        {#snippet sidebarToggleIcon()}
          <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 6h16M4 12h16M4 18h16"/>
          </svg>
        {/snippet}
        <IconButton
          variant="ghost"
          size="sm"
          icon={sidebarToggleIcon}
          aria-label="Toggle sidebar"
          onclick={() => sidebarOpen = !sidebarOpen}
          class="lg:hidden"
        />
        <div class="flex items-center gap-2">
          <StatusDot status={chat.status === 'connected' ? 'online' : 'offline'} />
          <h2 class="text-[length:var(--typography-fontSize-base)] font-[var(--typography-fontWeight-semibold)] text-fg">
            {activeAgentName}
          </h2>
        </div>
        {#if chat.isStreaming}
          <ThinkingIndicator />
        {/if}
      </div>

      <!-- Messages -->
      <ChatThread messages={chat.messages} class="flex-1 min-h-0" />

      <!-- Input -->
      <ChatInput onSend={handleSend} disabled={isSending} />
    {:else}
      <!-- Empty state -->
      <div class="flex flex-1 items-center justify-center">
        <div class="text-center px-6">
          <svg class="mx-auto mb-4 h-12 w-12 text-fgMuted opacity-30" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z"/>
          </svg>
          <p class="text-[length:var(--typography-fontSize-base)] text-fgMuted">
            Select an agent to start chatting
          </p>
        </div>
      </div>
    {/if}
  </div>
</div>

<SettingsModal bind:open={settingsOpen} {csrfToken} />
