<script lang="ts">
  import { createSSEStream, type SSEStatus } from '../stores/sse.svelte';

  interface Props {
    url?: string;
    status?: SSEStatus;
    label?: string;
  }

  let { url, status: propStatus = 'connecting', label }: Props = $props();

  // When url is set, create an SSEStream; otherwise use the prop directly
  let stream = $state<ReturnType<typeof createSSEStream> | null>(null);

  $effect(() => {
    if (!url) return;
    stream = createSSEStream(url);
    return () => stream?.close();
  });

  let sseStatus = $derived<SSEStatus>(stream ? stream.status : propStatus);

  const statusConfig: Record<SSEStatus, { colorClass: string; text: string; pulse: boolean }> = {
    connecting: { colorClass: 'bg-accent', text: 'Connecting', pulse: true },
    open:       { colorClass: 'bg-success-solidBg', text: 'Connected', pulse: false },
    closed:     { colorClass: 'bg-[var(--cg-primitives-neutral-400)]', text: 'Disconnected', pulse: false },
    error:      { colorClass: 'bg-danger-solidBg', text: 'Error', pulse: true },
  };

  let config = $derived(statusConfig[sseStatus]);
  let displayLabel = $derived(label ?? config.text);
</script>

<span
  class="inline-flex items-center gap-1.5"
  role="status"
  aria-label={displayLabel}
  data-testid="connection-badge"
>
  <span
    class="inline-block h-2 w-2 rounded-full flex-shrink-0 {config.pulse ? 'animate-pulse' : ''} {config.colorClass}"
    aria-hidden="true"
  ></span>
  <span class="text-[length:var(--typography-fontSize-xs)] text-fgMuted leading-[var(--typography-lineHeight-tight)]">
    {displayLabel}
  </span>
</span>
