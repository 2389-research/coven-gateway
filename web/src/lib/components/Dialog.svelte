<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    open: boolean;
    onclose?: () => void;
    header?: Snippet;
    footer?: Snippet;
    children: Snippet;
    class?: string;
  }

  let {
    open = $bindable(),
    onclose,
    header,
    footer,
    children,
    class: className = '',
  }: Props = $props();

  let dialogEl: HTMLDialogElement | undefined = $state();

  // Sync open state with the native dialog element
  $effect(() => {
    if (!dialogEl) return;
    if (open && !dialogEl.open) {
      dialogEl.showModal();
    } else if (!open && dialogEl.open) {
      dialogEl.close();
    }
  });

  /** Called by the native dialog close event (Escape key, or dialogEl.close()). */
  function handleNativeClose() {
    open = false;
    onclose?.();
  }

  /** Called by close button / backdrop click — only sets open=false.
   *  The $effect will call dialogEl.close() which fires the native onclose,
   *  and handleNativeClose will invoke the onclose callback exactly once. */
  function requestClose() {
    open = false;
  }

  function handleBackdropClick(e: MouseEvent) {
    if (e.target === dialogEl) {
      requestClose();
    }
  }
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<dialog
  bind:this={dialogEl}
  onclose={handleNativeClose}
  onclick={handleBackdropClick}
  aria-labelledby={header ? 'dialog-title' : undefined}
  class="m-auto max-h-[85vh] w-full max-w-[var(--sizing-container-narrow)] rounded-[var(--border-radius-xl)] border border-border bg-surface p-0 shadow-[var(--shadow-lg)] text-fg backdrop:bg-black/50 backdrop:animate-[fade-in_var(--motion-duration-fast)_var(--motion-easing)] open:animate-[slide-in_var(--motion-duration-normal)_var(--motion-easing)] {className}"
  data-testid="dialog"
>
  {#if header}
    <div class="flex items-center justify-between border-b border-border px-6 py-4">
      <div id="dialog-title" class="text-[length:var(--typography-fontSize-lg)] font-[var(--typography-fontWeight-semibold)]">
        {@render header()}
      </div>
      <button
        type="button"
        onclick={requestClose}
        class="rounded-[var(--border-radius-md)] p-1.5 text-fgMuted hover:bg-surfaceHover hover:text-fg transition-colors duration-[var(--motion-duration-fast)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
        aria-label="Close dialog"
        data-testid="dialog-close"
      >
        <svg class="h-5 w-5" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
          <path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.75.75 0 1 1 1.06 1.06L9.06 8l3.22 3.22a.75.75 0 1 1-1.06 1.06L8 9.06l-3.22 3.22a.75.75 0 0 1-1.06-1.06L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z" />
        </svg>
      </button>
    </div>
  {/if}

  <div class="px-6 py-4 overflow-y-auto">
    {@render children()}
  </div>

  {#if footer}
    <div class="flex items-center justify-end gap-3 border-t border-border px-6 py-4">
      {@render footer()}
    </div>
  {/if}
</dialog>
