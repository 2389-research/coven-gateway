<script lang="ts">
  interface Props {
    text?: string;
    class?: string;
  }

  let { text, class: className = '' }: Props = $props();
</script>

<div
  class="flex items-center gap-3 rounded-[var(--border-radius-lg)] bg-surfaceAlt px-4 py-3 {className}"
  data-testid="thinking-indicator"
>
  <div class="flex items-center gap-1 shrink-0" aria-label="Agent is thinking">
    <span class="thinking-dot h-1.5 w-1.5 rounded-[var(--border-radius-pill)] bg-fgMuted"></span>
    <span class="thinking-dot h-1.5 w-1.5 rounded-[var(--border-radius-pill)] bg-fgMuted" style="animation-delay: 150ms"></span>
    <span class="thinking-dot h-1.5 w-1.5 rounded-[var(--border-radius-pill)] bg-fgMuted" style="animation-delay: 300ms"></span>
  </div>
  {#if text}
    <p class="text-[length:var(--typography-fontSize-sm)] italic text-fgMuted leading-[var(--typography-lineHeight-normal)] min-w-0">
      {text}
    </p>
  {/if}
</div>

<style>
  @keyframes thinking-pulse {
    0%, 80%, 100% { opacity: 0.3; transform: scale(0.8); }
    40% { opacity: 1; transform: scale(1); }
  }

  .thinking-dot {
    animation: thinking-pulse 1.4s ease-in-out infinite;
  }
</style>
