<script lang="ts">
  type Variant = 'call' | 'result';

  interface Props {
    variant?: Variant;
    toolName?: string;
    content?: string;
    expanded?: boolean;
    class?: string;
  }

  let {
    variant = 'call',
    toolName,
    content,
    expanded = false,
    class: className = '',
  }: Props = $props();

  function toggle() {
    expanded = !expanded;
  }

  function formatJson(json: string): string {
    try {
      return JSON.stringify(JSON.parse(json), null, 2);
    } catch {
      return json;
    }
  }

  let isResult = $derived(variant === 'result');
  let displayContent = $derived(content && variant === 'call' ? formatJson(content) : content ?? '');
</script>

{#if isResult}
  <div
    class="rounded-[var(--border-radius-lg)] border border-success-subtleBorder bg-success-subtleBg overflow-hidden {className}"
    data-testid="tool-call-view"
    data-variant="result"
  >
    <button
      type="button"
      onclick={toggle}
      class="flex w-full items-center gap-2 px-4 py-2.5 text-left transition-colors duration-[var(--motion-duration-fast)]"
    >
      <svg class="h-4 w-4 text-success-subtleFg shrink-0" viewBox="0 0 16 16" fill="currentColor">
        <path fill-rule="evenodd" d="M12.416 3.376a.75.75 0 0 1 .208 1.04l-5 7.5a.75.75 0 0 1-1.154.114l-3-3a.75.75 0 0 1 1.06-1.06l2.353 2.353 4.493-6.74a.75.75 0 0 1 1.04-.207Z" clip-rule="evenodd"/>
      </svg>
      <span class="text-[length:var(--typography-fontSize-sm)] font-[var(--typography-fontWeight-medium)] text-success-subtleFg">
        Tool Result
      </span>
      <svg
        class="h-3 w-3 text-success-subtleFg shrink-0 ml-auto transition-transform duration-[var(--motion-duration-fast)] {expanded ? 'rotate-180' : ''}"
        viewBox="0 0 16 16"
        fill="currentColor"
      >
        <path fill-rule="evenodd" d="M4.22 6.22a.75.75 0 0 1 1.06 0L8 8.94l2.72-2.72a.75.75 0 1 1 1.06 1.06l-3.25 3.25a.75.75 0 0 1-1.06 0L4.22 7.28a.75.75 0 0 1 0-1.06Z" clip-rule="evenodd"/>
      </svg>
    </button>
    {#if expanded && content}
      <div class="border-t border-success-subtleBorder px-4 py-3">
        <pre class="overflow-x-auto whitespace-pre-wrap text-[length:var(--typography-fontSize-xs)] text-fg font-[family-name:var(--typography-fontFamily-mono)] leading-[var(--typography-lineHeight-relaxed)]">{displayContent}</pre>
      </div>
    {/if}
  </div>

{:else}
  <div
    class="rounded-[var(--border-radius-lg)] border border-border bg-surface overflow-hidden {className}"
    data-testid="tool-call-view"
    data-variant="call"
  >
    <button
      type="button"
      onclick={toggle}
      class="flex w-full items-center gap-2 px-4 py-2.5 text-left hover:bg-surfaceHover transition-colors duration-[var(--motion-duration-fast)]"
    >
      <svg
        class="h-4 w-4 text-fgMuted shrink-0 transition-transform duration-[var(--motion-duration-fast)] {expanded ? 'rotate-90' : ''}"
        viewBox="0 0 16 16"
        fill="currentColor"
      >
        <path d="M6.22 4.22a.75.75 0 0 1 1.06 0l3.25 3.25a.75.75 0 0 1 0 1.06l-3.25 3.25a.75.75 0 0 1-1.06-1.06L8.94 8 6.22 5.28a.75.75 0 0 1 0-1.06Z"/>
      </svg>
      <span class="text-[length:var(--typography-fontSize-sm)] font-[var(--typography-fontWeight-medium)] text-fg">
        {toolName ?? 'Tool Call'}
      </span>
    </button>
    {#if expanded && content}
      <div class="border-t border-border bg-surfaceAlt px-4 py-3">
        <p class="mb-1 text-[length:var(--typography-fontSize-xs)] font-[var(--typography-fontWeight-medium)] text-fgMuted">
          Input
        </p>
        <pre class="overflow-x-auto text-[length:var(--typography-fontSize-xs)] text-fg font-[family-name:var(--typography-fontFamily-mono)] leading-[var(--typography-lineHeight-relaxed)]">{displayContent}</pre>
      </div>
    {/if}
  </div>
{/if}
