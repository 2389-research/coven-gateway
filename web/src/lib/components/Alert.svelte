<script lang="ts">
  import type { Snippet } from 'svelte';

  type Variant = 'info' | 'success' | 'warning' | 'danger';

  interface Props {
    variant?: Variant;
    title?: string;
    dismissible?: boolean;
    ondismiss?: () => void;
    children: Snippet;
    class?: string;
  }

  let {
    variant = 'info',
    title,
    dismissible = false,
    ondismiss,
    children,
    class: className = '',
  }: Props = $props();

  let visible = $state(true);

  function dismiss() {
    visible = false;
    ondismiss?.();
  }

  const variantClasses: Record<Variant, string> = {
    info: 'bg-info-subtleBg text-info-subtleFg border-info-subtleBorder',
    success: 'bg-success-subtleBg text-success-subtleFg border-success-subtleBorder',
    warning: 'bg-warning-subtleBg text-warning-subtleFg border-warning-subtleBorder',
    danger: 'bg-danger-subtleBg text-danger-subtleFg border-danger-subtleBorder',
  };
</script>

{#if visible}
  <div
    class="flex gap-3 rounded-[var(--border-radius-md)] border p-3 text-[length:var(--typography-fontSize-sm)] {variantClasses[variant]} {className}"
    role={variant === 'danger' || variant === 'warning' ? 'alert' : 'status'}
    data-testid="alert"
  >
    <div class="flex-1">
      {#if title}
        <p class="font-[var(--typography-fontWeight-semibold)] mb-1">{title}</p>
      {/if}
      {@render children()}
    </div>

    {#if dismissible}
      <button
        onclick={dismiss}
        class="flex-shrink-0 self-start p-0.5 rounded-[var(--border-radius-sm)] opacity-60 hover:opacity-100 transition-opacity duration-[var(--motion-duration-fast)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
        aria-label="Dismiss"
        data-testid="alert-dismiss"
      >
        <svg class="h-4 w-4" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
          <path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.75.75 0 1 1 1.06 1.06L9.06 8l3.22 3.22a.75.75 0 1 1-1.06 1.06L8 9.06l-3.22 3.22a.75.75 0 0 1-1.06-1.06L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z" />
        </svg>
      </button>
    {/if}
  </div>
{/if}
