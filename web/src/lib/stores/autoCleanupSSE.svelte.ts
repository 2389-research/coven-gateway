/**
 * Registers SSE stream cleanup with Svelte's $effect teardown.
 *
 * Must be called during component initialization (inside <script>).
 * When the component unmounts, stream.close() is called automatically.
 *
 * Usage:
 *   const stream = createSSEStream('/some/endpoint');
 *   autoCleanupSSE(stream);
 */
export function autoCleanupSSE(stream: { close(): void }): void {
  $effect(() => {
    return () => {
      stream.close();
    };
  });
}
