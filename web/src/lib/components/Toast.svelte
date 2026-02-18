<script lang="ts">
  import { getToasts, removeToast, type ToastVariant } from '../stores/toast.svelte';

  interface Props {
    class?: string;
  }

  let { class: className = '' }: Props = $props();

  let toasts = $derived(getToasts());

  const variantClasses: Record<ToastVariant, string> = {
    info: 'bg-info-subtleBg text-info-subtleFg border-info-subtleBorder',
    success: 'bg-success-subtleBg text-success-subtleFg border-success-subtleBorder',
    warning: 'bg-warning-subtleBg text-warning-subtleFg border-warning-subtleBorder',
    danger: 'bg-danger-subtleBg text-danger-subtleFg border-danger-subtleBorder',
  };
</script>

{#if toasts.length > 0}
  <div
    class="fixed bottom-4 right-4 z-[var(--zIndex-toast)] flex flex-col gap-2 {className}"
    role="region"
    aria-label="Notifications"
    data-testid="toast-container"
  >
    {#each toasts as toast (toast.id)}
      <div
        class="flex items-center gap-3 rounded-[var(--border-radius-md)] border p-3 shadow-[var(--shadow-md)] text-[length:var(--typography-fontSize-sm)] min-w-[16rem] max-w-[24rem] animate-[slide-in_var(--motion-duration-normal)_var(--motion-easing)] {variantClasses[toast.variant]}"
        role="status"
        aria-live="polite"
        data-testid="toast"
      >
        <span class="flex-1">{toast.message}</span>
        <button
          type="button"
          onclick={() => removeToast(toast.id)}
          class="flex-shrink-0 p-0.5 rounded-[var(--border-radius-sm)] opacity-60 hover:opacity-100 transition-opacity duration-[var(--motion-duration-fast)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring focus-visible:opacity-100"
          aria-label="Dismiss"
          data-testid="toast-dismiss"
        >
          <svg class="h-3.5 w-3.5" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
            <path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.75.75 0 1 1 1.06 1.06L9.06 8l3.22 3.22a.75.75 0 1 1-1.06 1.06L8 9.06l-3.22 3.22a.75.75 0 0 1-1.06-1.06L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z" />
          </svg>
        </button>
      </div>
    {/each}
  </div>
{/if}
