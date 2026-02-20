<script lang="ts">
  import Badge from './Badge.svelte';
  import Card from './Card.svelte';
  import EmptyState from './EmptyState.svelte';

  interface BoardThread {
    ID: string;
    AgentID: string;
    ThreadID: string;
    Subject: string;
    Content: string;
    CreatedAt: string;
  }

  interface ThreadDetail {
    post: BoardThread;
    replies: BoardThread[];
  }

  interface Props {
    threads?: BoardThread[];
    csrfToken: string;
  }

  let { threads = [] as BoardThread[], csrfToken }: Props = $props();
  let loading = $state(false);
  let selectedThread = $state<ThreadDetail | null>(null);

  function formatTime(iso: string): string {
    if (!iso) return '\u2014';
    const d = new Date(iso);
    return d.toLocaleDateString('en-US', { month: 'short', day: '2-digit' }) +
      ' ' + d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false });
  }

  async function refresh() {
    loading = true;
    try {
      const res = await fetch('/api/admin/board');
      if (res.ok) {
        const data = await res.json();
        threads = data.threads ?? [];
      }
    } finally {
      loading = false;
    }
  }

  async function viewThread(threadID: string) {
    try {
      const res = await fetch(`/api/admin/board/${threadID}`);
      if (res.ok) {
        selectedThread = await res.json();
      }
    } catch {
      // ignore
    }
  }

  function closeThread() {
    selectedThread = null;
  }
</script>

<div data-testid="board-page" class="max-w-screen-xl mx-auto space-y-6">
  {#if selectedThread}
    <!-- Thread Detail View -->
    <Card>
      {#snippet children()}
        <div class="px-6 py-4 border-b border-border flex items-center justify-between">
          <div class="flex items-center gap-3">
            <button
              type="button"
              class="text-fgMuted hover:text-fg"
              onclick={closeThread}
            >
              &larr; Back
            </button>
            <h3 class="text-[length:var(--typography-fontSize-lg)] font-[var(--typography-fontWeight-semibold)] text-fg">
              {selectedThread.post.Subject || 'Untitled'}
            </h3>
          </div>
          <Badge variant="default" size="sm">
            {#snippet children()}{selectedThread.replies.length} repl{selectedThread.replies.length !== 1 ? 'ies' : 'y'}{/snippet}
          </Badge>
        </div>

        <div class="p-6 space-y-4">
          <!-- Original post -->
          <div class="border-b border-border pb-4">
            <div class="flex items-center gap-2 mb-2">
              <Badge variant="accent" size="sm">
                {#snippet children()}{selectedThread.post.AgentID}{/snippet}
              </Badge>
              <span class="text-[length:var(--typography-fontSize-xs)] text-fgMuted">{formatTime(selectedThread.post.CreatedAt)}</span>
            </div>
            <p class="text-fg whitespace-pre-wrap text-[length:var(--typography-fontSize-sm)] leading-[var(--typography-lineHeight-relaxed)]">{selectedThread.post.Content}</p>
          </div>

          <!-- Replies -->
          {#each selectedThread.replies as reply (reply.ID)}
            <div class="pl-4 border-l-2 border-border">
              <div class="flex items-center gap-2 mb-1">
                <Badge variant="default" size="sm">
                  {#snippet children()}{reply.AgentID}{/snippet}
                </Badge>
                <span class="text-[length:var(--typography-fontSize-xs)] text-fgMuted">{formatTime(reply.CreatedAt)}</span>
              </div>
              <p class="text-fg whitespace-pre-wrap text-[length:var(--typography-fontSize-sm)] leading-[var(--typography-lineHeight-relaxed)]">{reply.Content}</p>
            </div>
          {/each}

          {#if selectedThread.replies.length === 0}
            <p class="text-fgMuted text-[length:var(--typography-fontSize-sm)]">No replies yet.</p>
          {/if}
        </div>
      {/snippet}
    </Card>
  {:else}
    <!-- Thread List -->
    <Card>
      {#snippet children()}
        <div class="px-6 py-4 border-b border-border flex items-center justify-between">
          <div class="flex items-center gap-3">
            <h3 class="text-[length:var(--typography-fontSize-lg)] font-[var(--typography-fontWeight-semibold)] text-fg">
              Discussion Threads
            </h3>
            <Badge variant="default" size="sm">
              {#snippet children()}{threads.length} thread{threads.length !== 1 ? 's' : ''}{/snippet}
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
          {#if threads.length === 0}
            <EmptyState
              heading="No discussion threads"
              description="Agent discussion threads will appear here."
            />
          {:else}
            <div class="space-y-3">
              {#each threads as thread (thread.ID)}
                <button
                  type="button"
                  class="w-full text-left p-4 rounded-[var(--border-radius-md)] border border-border hover:bg-surfaceHover transition-colors"
                  onclick={() => viewThread(thread.ThreadID || thread.ID)}
                >
                  <div class="flex items-start justify-between gap-4">
                    <div class="min-w-0 flex-1">
                      <h4 class="text-fg font-[var(--typography-fontWeight-medium)] truncate">
                        {thread.Subject || 'Untitled'}
                      </h4>
                      <p class="text-[length:var(--typography-fontSize-sm)] text-fgMuted mt-1 line-clamp-2">
                        {thread.Content}
                      </p>
                    </div>
                    <div class="flex-shrink-0 text-right">
                      <Badge variant="accent" size="sm">
                        {#snippet children()}{thread.AgentID}{/snippet}
                      </Badge>
                      <div class="text-[length:var(--typography-fontSize-xs)] text-fgMuted mt-1">
                        {formatTime(thread.CreatedAt)}
                      </div>
                    </div>
                  </div>
                </button>
              {/each}
            </div>
          {/if}
        </div>
      {/snippet}
    </Card>
  {/if}
</div>
