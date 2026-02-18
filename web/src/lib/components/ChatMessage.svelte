<script lang="ts">
  import { marked } from 'marked';
  import DOMPurify from 'dompurify';
  import type { ChatMessage } from '../types/chat';
  import { getMessageSender } from '../types/chat';
  import ToolCallView from './ToolCallView.svelte';
  import ThinkingIndicator from './ThinkingIndicator.svelte';

  interface Props {
    message: ChatMessage;
    class?: string;
  }

  let { message, class: className = '' }: Props = $props();

  let sender = $derived(getMessageSender(message.type));
  let isUser = $derived(sender === 'user');

  /** Render markdown to sanitized HTML */
  let renderedContent = $derived.by(() => {
    if (!message.content) return '';
    if (message.type === 'tool_use' || message.type === 'tool_result') {
      return '';
    }
    const raw = marked.parse(message.content, { async: false }) as string;
    return DOMPurify.sanitize(raw);
  });

  function formatTime(date: Date): string {
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }
</script>

{#if message.type === 'thinking'}
  <div
    class="flex justify-start {className}"
    data-testid="chat-message"
    data-message-type="thinking"
  >
    <div class="max-w-[80%]">
      <ThinkingIndicator text={message.content || undefined} />
    </div>
  </div>

{:else if message.type === 'tool_use'}
  <div
    class="flex justify-start {className}"
    data-testid="chat-message"
    data-message-type="tool_use"
  >
    <div class="max-w-[80%] w-full">
      <ToolCallView variant="call" toolName={message.toolName} content={message.inputJson} />
    </div>
  </div>

{:else if message.type === 'tool_result'}
  <div
    class="flex justify-start {className}"
    data-testid="chat-message"
    data-message-type="tool_result"
  >
    <div class="max-w-[80%] w-full">
      <ToolCallView variant="result" content={message.content} />
    </div>
  </div>

{:else if message.type === 'error'}
  <div
    class="flex justify-center {className}"
    data-testid="chat-message"
    data-message-type="error"
  >
    <div class="max-w-[80%] rounded-[var(--border-radius-lg)] border border-danger-subtleBorder bg-danger-subtleBg px-4 py-3">
      <p class="text-[length:var(--typography-fontSize-sm)] text-danger-subtleFg">
        {message.content}
      </p>
    </div>
  </div>

{:else if message.type === 'canceled'}
  <div
    class="flex justify-center {className}"
    data-testid="chat-message"
    data-message-type="canceled"
  >
    <div class="rounded-[var(--border-radius-lg)] bg-surfaceAlt px-4 py-2">
      <p class="text-[length:var(--typography-fontSize-sm)] text-fgMuted italic">
        Canceled{message.reason ? `: ${message.reason}` : ''}
      </p>
    </div>
  </div>

{:else}
  <!-- user or text (agent) messages -->
  <div
    class="flex {isUser ? 'justify-end' : 'justify-start'} {className}"
    data-testid="chat-message"
    data-message-type={message.type}
  >
    <div
      class="max-w-[80%] rounded-[var(--border-radius-lg)] px-4 py-3
        {isUser
          ? 'bg-accent text-fgOnAccent'
          : 'bg-surfaceAlt text-fg'}"
    >
      <div class="mb-1 flex items-center gap-2">
        <span class="text-[length:var(--typography-fontSize-xs)] font-[var(--typography-fontWeight-medium)] {isUser ? 'text-fgOnAccent/70' : 'text-fgMuted'}">
          {isUser ? 'You' : 'Agent'}
        </span>
        <span class="text-[length:var(--typography-fontSize-xs)] {isUser ? 'text-fgOnAccent/50' : 'text-fgMuted/70'}">
          {formatTime(message.timestamp)}
        </span>
      </div>
      {#if isUser}
        <div class="text-[length:var(--typography-fontSize-sm)] leading-[var(--typography-lineHeight-normal)] whitespace-pre-wrap">
          {message.content}
        </div>
      {:else}
        <div class="chat-message-content text-[length:var(--typography-fontSize-sm)] leading-[var(--typography-lineHeight-normal)]">
          {@html renderedContent}
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .chat-message-content :global(p) {
    margin-bottom: 0.5em;
  }
  .chat-message-content :global(p:last-child) {
    margin-bottom: 0;
  }
  .chat-message-content :global(code) {
    font-family: var(--typography-fontFamily-mono);
    font-size: 0.875em;
    background: var(--cg-surfaceHover);
    border-radius: var(--border-radius-sm);
    padding: 0.1em 0.3em;
  }
  .chat-message-content :global(pre) {
    background: var(--cg-bg);
    border: 1px solid var(--cg-border);
    border-radius: var(--border-radius-md);
    padding: 0.75rem 1rem;
    overflow-x: auto;
    margin: 0.5em 0;
  }
  .chat-message-content :global(pre code) {
    background: none;
    padding: 0;
    font-size: var(--typography-fontSize-xs);
    line-height: var(--typography-lineHeight-relaxed);
  }
  .chat-message-content :global(ul),
  .chat-message-content :global(ol) {
    padding-left: 1.5em;
    margin: 0.5em 0;
  }
  .chat-message-content :global(li) {
    margin-bottom: 0.25em;
  }
  .chat-message-content :global(blockquote) {
    border-left: 3px solid var(--cg-border);
    padding-left: 1em;
    color: var(--cg-fgMuted);
    margin: 0.5em 0;
  }
  .chat-message-content :global(a) {
    color: var(--cg-accent);
    text-decoration: underline;
  }
  .chat-message-content :global(h1),
  .chat-message-content :global(h2),
  .chat-message-content :global(h3) {
    font-weight: var(--typography-fontWeight-semibold);
    margin: 0.75em 0 0.25em;
  }
  .chat-message-content :global(h1) { font-size: var(--typography-fontSize-lg); }
  .chat-message-content :global(h2) { font-size: var(--typography-fontSize-base); }
  .chat-message-content :global(h3) { font-size: var(--typography-fontSize-sm); }
</style>
