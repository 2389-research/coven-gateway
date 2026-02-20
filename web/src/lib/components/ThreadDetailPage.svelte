<script lang="ts">
  import Badge from './Badge.svelte';
  import Card from './Card.svelte';
  import CodeText from './CodeText.svelte';
  import EmptyState from './EmptyState.svelte';
  import ToolCallView from './ToolCallView.svelte';

  interface ThreadInfo {
    ID: string;
    FrontendName: string;
    AgentID: string;
    CreatedAt: string;
    UpdatedAt: string;
  }

  interface MessageItem {
    ID: string;
    Sender: string;
    Content: string;
    Type: string;
    ToolName: string;
    ToolID: string;
    CreatedAt: string;
  }

  interface Props {
    thread: ThreadInfo;
    messages?: MessageItem[];
    csrfToken: string;
  }

  let { thread, messages = [] as MessageItem[], csrfToken }: Props = $props();

  function formatTime(iso: string): string {
    if (!iso) return '\u2014';
    const d = new Date(iso);
    return d.toLocaleDateString('en-US', { month: 'short', day: '2-digit' }) +
      ' ' + d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false });
  }

  function isToolMessage(msg: MessageItem): boolean {
    return msg.Type === 'tool_use' || msg.Type === 'tool_result';
  }

  function senderLabel(sender: string): string {
    if (sender === 'agent') return 'Agent';
    if (sender === 'user') return 'User';
    if (sender === 'system') return 'System';
    return sender;
  }
</script>

<div data-testid="thread-detail-page" class="space-y-6">
  <!-- Thread Info Card -->
  <Card>
    {#snippet children()}
      <div class="p-6">
        <div class="flex items-start justify-between">
          <div>
            <h2 class="text-[length:var(--typography-fontSize-xl)] font-[var(--typography-fontWeight-semibold)] text-fg">
              {thread.FrontendName || 'Unnamed Thread'}
            </h2>
            <div class="mt-2 flex items-center gap-3">
              <CodeText class="text-[length:var(--typography-fontSize-xs)] text-fgMuted">
                {#snippet children()}{thread.ID}{/snippet}
              </CodeText>
            </div>
          </div>
          <a
            href="/?agent={thread.AgentID}&thread={thread.ID}"
            class="px-4 py-2 text-[length:var(--typography-fontSize-sm)] font-[var(--typography-fontWeight-medium)] bg-[var(--color-primary)] text-[var(--color-primaryFg)] rounded-[var(--border-radius-md)] hover:opacity-90 transition-opacity"
          >
            Resume Chat
          </a>
        </div>

        <div class="mt-4 grid grid-cols-2 md:grid-cols-4 gap-4">
          <div>
            <dt class="text-[length:var(--typography-fontSize-xs)] text-fgMuted">Agent</dt>
            <dd class="text-[length:var(--typography-fontSize-sm)] text-fg font-mono mt-0.5">{thread.AgentID}</dd>
          </div>
          <div>
            <dt class="text-[length:var(--typography-fontSize-xs)] text-fgMuted">Frontend</dt>
            <dd class="text-[length:var(--typography-fontSize-sm)] text-fg font-[var(--typography-fontWeight-medium)] mt-0.5">{thread.FrontendName}</dd>
          </div>
          <div>
            <dt class="text-[length:var(--typography-fontSize-xs)] text-fgMuted">Created</dt>
            <dd class="text-[length:var(--typography-fontSize-sm)] text-fg mt-0.5">{formatTime(thread.CreatedAt)}</dd>
          </div>
          <div>
            <dt class="text-[length:var(--typography-fontSize-xs)] text-fgMuted">Updated</dt>
            <dd class="text-[length:var(--typography-fontSize-sm)] text-fg mt-0.5">{formatTime(thread.UpdatedAt)}</dd>
          </div>
        </div>
      </div>
    {/snippet}
  </Card>

  <!-- Messages Section -->
  <Card>
    {#snippet children()}
      <div class="px-6 py-4 border-b border-border flex items-center justify-between">
        <h3 class="text-[length:var(--typography-fontSize-lg)] font-[var(--typography-fontWeight-semibold)] text-fg">
          Messages
        </h3>
        <span class="text-[length:var(--typography-fontSize-xs)] text-fgMuted font-[var(--typography-fontWeight-medium)]">
          {messages.length} message{messages.length !== 1 ? 's' : ''}
        </span>
      </div>
      <div class="p-6">
        {#if messages.length === 0}
          <EmptyState
            heading="No messages yet"
            description="Messages will appear here as the conversation progresses."
          />
        {:else}
          <div class="space-y-4">
            {#each messages as msg (msg.ID)}
              {#if isToolMessage(msg)}
                <ToolCallView
                  variant={msg.Type === 'tool_result' ? 'result' : 'call'}
                  toolName={msg.ToolName}
                  content={msg.Content}
                />
              {:else}
                <div class="flex gap-3">
                  <div class="flex-shrink-0 mt-1">
                    <Badge
                      variant={msg.Sender === 'agent' ? 'accent' : msg.Sender === 'user' ? 'default' : 'warning'}
                      size="sm"
                    >
                      {#snippet children()}{senderLabel(msg.Sender)}{/snippet}
                    </Badge>
                  </div>
                  <div class="flex-1 min-w-0">
                    <div class="text-[length:var(--typography-fontSize-sm)] text-fg whitespace-pre-wrap break-words leading-[var(--typography-lineHeight-relaxed)]">
                      {msg.Content}
                    </div>
                    <div class="mt-1 text-[length:var(--typography-fontSize-xs)] text-fgMuted">
                      {formatTime(msg.CreatedAt)}
                    </div>
                  </div>
                </div>
              {/if}
            {/each}
          </div>
        {/if}
      </div>
    {/snippet}
  </Card>
</div>
