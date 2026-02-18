<script lang="ts">
  type Status = 'connecting' | 'open' | 'closed' | 'error';

  interface Props {
    url?: string;
    status?: Status;
    label?: string;
  }

  let { url, status: propStatus = 'connecting', label }: Props = $props();

  // Internal status tracked by the EventSource effect (only used when url is set)
  let internalStatus = $state<Status>('connecting');

  $effect(() => {
    if (!url) return;

    internalStatus = 'connecting';
    const source = new EventSource(url);

    source.onopen = () => {
      internalStatus = 'open';
    };

    source.onerror = () => {
      internalStatus = source.readyState === EventSource.CLOSED ? 'closed' : 'error';
    };

    return () => {
      source.close();
    };
  });

  // When url is set, use SSE-driven status; otherwise use the prop directly
  let sseStatus = $derived<Status>(url ? internalStatus : propStatus);

  const statusConfig: Record<Status, { color: string; text: string; pulse: boolean }> = {
    connecting: { color: 'var(--color-accent)', text: 'Connecting', pulse: true },
    open:       { color: 'var(--color-success-solidBg)', text: 'Connected', pulse: false },
    closed:     { color: 'var(--color-primitives-neutral-400)', text: 'Disconnected', pulse: false },
    error:      { color: 'var(--color-danger-solidBg)', text: 'Error', pulse: false },
  };

  let config = $derived(statusConfig[sseStatus]);
  let displayLabel = $derived(label ?? config.text);
</script>

<span class="connection-badge" role="status" aria-label={displayLabel}>
  <span
    class="dot"
    class:pulse={config.pulse}
    style:--dot-color="hsl({config.color})"
  ></span>
  <span class="label">{displayLabel}</span>
</span>

<style>
  .connection-badge {
    display: inline-flex;
    align-items: center;
    gap: 0.375rem;
    font-size: var(--typography-fontSize-xs);
    font-family: var(--typography-fontFamily-sans);
    color: hsl(var(--color-fgMuted));
  }

  .dot {
    width: 0.5rem;
    height: 0.5rem;
    border-radius: var(--border-radius-pill);
    background: var(--dot-color);
    flex-shrink: 0;
  }

  .pulse {
    animation: pulse 1.5s ease-in-out infinite;
  }

  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.4; }
  }

  .label {
    line-height: var(--typography-lineHeight-tight);
  }
</style>
