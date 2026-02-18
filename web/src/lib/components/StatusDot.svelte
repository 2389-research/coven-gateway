<script lang="ts">
  type Status = 'online' | 'offline' | 'error' | 'busy';

  interface Props {
    status?: Status;
    pulse?: boolean;
    label?: string;
    showLabel?: boolean;
    class?: string;
  }

  let {
    status = 'offline',
    pulse = false,
    label,
    showLabel = false,
    class: className = '',
  }: Props = $props();

  const statusConfig: Record<Status, { colorClass: string; defaultLabel: string }> = {
    online: { colorClass: 'bg-success-solidBg', defaultLabel: 'Online' },
    offline: { colorClass: 'bg-[var(--cg-primitives-neutral-400)]', defaultLabel: 'Offline' },
    error: { colorClass: 'bg-danger-solidBg', defaultLabel: 'Error' },
    busy: { colorClass: 'bg-warning-solidBg', defaultLabel: 'Busy' },
  };

  let config = $derived(statusConfig[status]);
  let displayLabel = $derived(label ?? config.defaultLabel);
</script>

<span
  class="inline-flex items-center gap-1.5 {className}"
  role="status"
  aria-label={displayLabel}
  data-testid="status-dot"
>
  <span
    class="inline-block h-2 w-2 rounded-full flex-shrink-0 {pulse ? 'animate-pulse' : ''} {config.colorClass}"
    aria-hidden="true"
  ></span>
  {#if showLabel}
    <span class="text-[length:var(--typography-fontSize-xs)] text-fgMuted leading-[var(--typography-lineHeight-tight)]">
      {displayLabel}
    </span>
  {/if}
</span>
