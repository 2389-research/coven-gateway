<script lang="ts">
  interface Props {
    onSend: (text: string) => void;
    disabled?: boolean;
    maxLength?: number;
    placeholder?: string;
    class?: string;
  }

  import IconButton from './IconButton.svelte';

  let {
    onSend,
    disabled = false,
    maxLength = 10000,
    placeholder = 'Type a message...',
    class: className = '',
  }: Props = $props();

  let value = $state('');
  let textareaEl: HTMLTextAreaElement | undefined = $state();

  let charCount = $derived(value.length);
  let isOverLimit = $derived(maxLength > 0 && charCount > maxLength);
  let canSend = $derived(value.trim().length > 0 && !disabled && !isOverLimit);

  function send() {
    if (!canSend) return;
    const text = value.trim();
    value = '';
    resetHeight();
    onSend(text);
  }

  function handleKeydown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.preventDefault();
      send();
    }
  }

  function handleInput() {
    autoResize();
  }

  function autoResize() {
    if (!textareaEl) return;
    textareaEl.style.height = 'auto';
    // Cap at roughly 10 lines (~200px)
    const maxHeight = 200;
    textareaEl.style.height = `${Math.min(textareaEl.scrollHeight, maxHeight)}px`;
    textareaEl.style.overflowY = textareaEl.scrollHeight > maxHeight ? 'auto' : 'hidden';
  }

  function resetHeight() {
    if (!textareaEl) return;
    textareaEl.style.height = 'auto';
    textareaEl.style.overflowY = 'hidden';
  }
</script>

<div class="border-t border-border bg-surface px-4 py-3 {className}" data-testid="chat-input">
  <div class="flex items-end gap-3">
    <div class="relative flex-1">
      <textarea
        bind:this={textareaEl}
        bind:value
        oninput={handleInput}
        onkeydown={handleKeydown}
        {placeholder}
        {disabled}
        rows={1}
        class="w-full resize-none overflow-hidden rounded-[var(--border-radius-lg)] border bg-bg px-3 py-2.5 text-[length:var(--typography-fontSize-sm)] text-fg placeholder:text-fgMuted transition-colors duration-[var(--motion-duration-fast)] focus:outline-none disabled:cursor-not-allowed disabled:opacity-50
          {isOverLimit
            ? 'border-danger-solidBg'
            : 'border-border focus:border-ring'}"
        data-testid="chat-input-textarea"
      ></textarea>
    </div>

    {#snippet sendIcon()}
      <svg class="h-4 w-4" viewBox="0 0 16 16" fill="currentColor">
        <path d="M.2 1.065a.625.625 0 0 1 .88-.585l14 6.5a.625.625 0 0 1 0 1.14l-14 6.5a.625.625 0 0 1-.862-.733L1.94 8 .218 2.213A.625.625 0 0 1 .2 1.065ZM3.169 8.75l-1.2 4.015L13.006 8.5H3.169Zm-.032-1.5h9.87L1.968 3.235 3.137 7.25Z"/>
      </svg>
    {/snippet}
    <IconButton
      variant="primary"
      size="md"
      icon={sendIcon}
      aria-label="Send message"
      onclick={send}
      disabled={!canSend}
      class="shrink-0"
      data-testid="chat-input-send"
    />
  </div>

  <div class="mt-1.5 flex items-center justify-between px-1">
    <span class="text-[length:var(--typography-fontSize-xs)] text-fgMuted">
      {#if disabled}
        Sending...
      {:else}
        <kbd class="rounded-[var(--border-radius-xs)] border border-border bg-surfaceAlt px-1 py-0.5 text-[length:0.625rem] font-[family-name:var(--typography-fontFamily-mono)]">
          {navigator?.platform?.includes('Mac') ? '⌘' : 'Ctrl'}+Enter
        </kbd>
        to send
      {/if}
    </span>
    {#if maxLength > 0}
      <span
        class="text-[length:var(--typography-fontSize-xs)] {isOverLimit ? 'text-danger-subtleFg font-[var(--typography-fontWeight-medium)]' : 'text-fgMuted'}"
        data-testid="chat-input-charcount"
      >
        {charCount.toLocaleString()}/{maxLength.toLocaleString()}
      </span>
    {/if}
  </div>
</div>
