<script lang="ts">
  import type { ChatMessage as ChatMessageType } from '../types/chat';
  import { isDisplayableMessage } from '../types/chat';
  import ChatMessage from './ChatMessage.svelte';
  import Button from './Button.svelte';

  interface Props {
    messages: ChatMessageType[];
    class?: string;
  }

  let { messages, class: className = '' }: Props = $props();

  let containerEl: HTMLDivElement | undefined = $state();
  let isAtBottom = $state(true);
  let hasNewMessages = $state(false);

  /** Filter to displayable messages only */
  let displayMessages = $derived(messages.filter((m) => isDisplayableMessage(m.type)));

  /** Check if two dates are on different calendar days */
  function isDifferentDay(a: Date, b: Date): boolean {
    return (
      a.getFullYear() !== b.getFullYear() ||
      a.getMonth() !== b.getMonth() ||
      a.getDate() !== b.getDate()
    );
  }

  /** Format a date for the separator label */
  function formatDateSeparator(date: Date): string {
    const now = new Date();
    const yesterday = new Date(now);
    yesterday.setDate(yesterday.getDate() - 1);

    if (!isDifferentDay(date, now)) return 'Today';
    if (!isDifferentDay(date, yesterday)) return 'Yesterday';

    return date.toLocaleDateString(undefined, {
      weekday: 'long',
      month: 'long',
      day: 'numeric',
    });
  }

  /** Should we show a date separator before this message? */
  function showDateSeparator(index: number): boolean {
    if (index === 0) return true;
    const prev = displayMessages[index - 1];
    const curr = displayMessages[index];
    return isDifferentDay(prev.timestamp, curr.timestamp);
  }

  function handleScroll() {
    if (!containerEl) return;
    const { scrollTop, scrollHeight, clientHeight } = containerEl;
    // Consider "at bottom" if within 48px of the bottom
    const wasAtBottom = isAtBottom;
    isAtBottom = scrollHeight - scrollTop - clientHeight < 48;
    if (isAtBottom && hasNewMessages) {
      hasNewMessages = false;
    }
    // If user scrolled back down manually, clear the indicator
    if (!wasAtBottom && isAtBottom) {
      hasNewMessages = false;
    }
  }

  function scrollToBottom() {
    if (!containerEl) return;
    containerEl.scrollTo({ top: containerEl.scrollHeight, behavior: 'smooth' });
    hasNewMessages = false;
  }

  /** Auto-scroll on new messages if user is at bottom */
  $effect(() => {
    // Track displayMessages.length to trigger on new messages
    const _len = displayMessages.length;
    if (!containerEl) return;

    if (isAtBottom) {
      // Use requestAnimationFrame to scroll after DOM update
      requestAnimationFrame(() => {
        containerEl?.scrollTo({ top: containerEl.scrollHeight, behavior: 'smooth' });
      });
    } else if (_len > 0) {
      hasNewMessages = true;
    }
  });
</script>

<div class="relative flex flex-col {className}" data-testid="chat-thread">
  <div
    bind:this={containerEl}
    onscroll={handleScroll}
    class="flex-1 overflow-y-auto px-4 py-4"
  >
    {#each displayMessages as message, i (message.id)}
      {#if showDateSeparator(i)}
        <div class="my-4 flex items-center gap-3" data-testid="date-separator">
          <div class="h-px flex-1 bg-border"></div>
          <span class="text-[length:var(--typography-fontSize-xs)] font-[var(--typography-fontWeight-medium)] text-fgMuted shrink-0">
            {formatDateSeparator(message.timestamp)}
          </span>
          <div class="h-px flex-1 bg-border"></div>
        </div>
      {/if}
      <div class="mb-3">
        <ChatMessage {message} />
      </div>
    {/each}

    {#if displayMessages.length === 0}
      <div class="flex h-full items-center justify-center">
        <p class="text-[length:var(--typography-fontSize-sm)] text-fgMuted">
          No messages yet. Start a conversation!
        </p>
      </div>
    {/if}
  </div>

  <!-- Scroll to bottom button -->
  {#if !isAtBottom}
    <div class="absolute bottom-4 left-1/2 -translate-x-1/2 z-10">
      <Button
        variant="secondary"
        size="sm"
        onclick={scrollToBottom}
        class="shadow-[var(--shadow-md)]"
        data-testid="scroll-to-bottom"
      >
        {#snippet children()}
          <svg class="h-4 w-4 text-fgMuted" viewBox="0 0 16 16" fill="currentColor">
            <path fill-rule="evenodd" d="M8 1a.75.75 0 0 1 .75.75v10.19l2.72-2.72a.75.75 0 1 1 1.06 1.06l-4 4a.75.75 0 0 1-1.06 0l-4-4a.75.75 0 1 1 1.06-1.06l2.72 2.72V1.75A.75.75 0 0 1 8 1Z" clip-rule="evenodd"/>
          </svg>
          {#if hasNewMessages}
            <span class="text-[length:var(--typography-fontSize-xs)] font-[var(--typography-fontWeight-medium)] text-accent">
              New messages
            </span>
          {/if}
        {/snippet}
      </Button>
    </div>
  {/if}
</div>
