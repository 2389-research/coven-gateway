<script lang="ts">
  import type { HTMLTextareaAttributes } from 'svelte/elements';

  interface Props extends Omit<HTMLTextareaAttributes, 'class'> {
    label?: string;
    error?: string;
    autoResize?: boolean;
    class?: string;
  }

  let {
    label,
    error,
    autoResize = false,
    disabled = false,
    rows = 3,
    id,
    class: className = '',
    ...rest
  }: Props = $props();

  const fallbackId = `ta-${Math.random().toString(36).slice(2, 8)}`;
  let textareaId = $derived(id ?? fallbackId);
  let errorId = $derived(`${textareaId}-error`);

  let textareaEl: HTMLTextAreaElement | undefined = $state();

  function handleInput() {
    if (autoResize && textareaEl) {
      textareaEl.style.height = 'auto';
      textareaEl.style.height = `${textareaEl.scrollHeight}px`;
    }
  }
</script>

<div class="flex flex-col gap-1.5 {className}" data-testid="text-area">
  {#if label}
    <label
      for={textareaId}
      class="text-[length:var(--typography-fontSize-sm)] font-[var(--typography-fontWeight-medium)] text-fg"
    >
      {label}
    </label>
  {/if}

  <textarea
    bind:this={textareaEl}
    id={textareaId}
    {rows}
    {disabled}
    oninput={handleInput}
    aria-invalid={error ? 'true' : undefined}
    aria-describedby={error ? errorId : undefined}
    class="w-full rounded-[var(--border-radius-md)] border bg-surface px-3 py-2 text-[length:var(--typography-fontSize-sm)] text-fg placeholder:text-fgMuted transition-colors duration-[var(--motion-duration-fast)] focus:outline-none disabled:cursor-not-allowed disabled:opacity-50
      {error
        ? 'border-danger-solidBg'
        : 'border-border focus:border-ring'}
      {autoResize ? 'resize-none overflow-hidden' : 'resize-y'}"
    {...rest}
  ></textarea>

  {#if error}
    <p id={errorId} class="text-[length:var(--typography-fontSize-sm)] text-danger-subtleFg" role="alert">
      {error}
    </p>
  {/if}
</div>
