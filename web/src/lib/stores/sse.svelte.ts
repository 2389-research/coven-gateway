/**
 * SSEStream: headless EventSource wrapper with reconnection.
 *
 * Usage:
 *   const stream = createSSEStream('/health/stream');
 *   // stream.status is reactive ('connecting' | 'open' | 'closed' | 'error')
 *   // stream.close() must be called on unmount
 */

export type SSEStatus = 'connecting' | 'open' | 'closed' | 'error';

export interface SSEStreamOptions {
  /** Callback for incoming messages. */
  onmessage?: (event: MessageEvent) => void;
  /** Callback for named events. Map of event name → handler. */
  onevents?: Record<string, (event: MessageEvent) => void>;
  /** Max reconnection attempts before giving up. 0 = infinite. Default: 0. */
  maxRetries?: number;
  /** Initial reconnect delay in ms. Doubles each attempt. Default: 1000. */
  initialDelay?: number;
  /** Max reconnect delay in ms. Default: 30000. */
  maxDelay?: number;
}

export interface SSEStream {
  readonly status: SSEStatus;
  close(): void;
}

export function createSSEStream(url: string, options: SSEStreamOptions = {}): SSEStream {
  const {
    onmessage,
    onevents,
    maxRetries = 0,
    initialDelay = 1000,
    maxDelay = 30000,
  } = options;

  let status = $state<SSEStatus>('connecting');
  let source: EventSource | null = null;
  let retryCount = 0;
  let retryTimer: ReturnType<typeof setTimeout> | null = null;
  let closed = false;

  function connect() {
    if (closed) return;

    status = 'connecting';
    source = new EventSource(url);

    source.onopen = () => {
      status = 'open';
      retryCount = 0; // Reset on successful connection
    };

    source.onerror = () => {
      if (closed) return;

      const wasClosed = source?.readyState === EventSource.CLOSED;
      source?.close();
      source = null;

      if (wasClosed || (maxRetries > 0 && retryCount >= maxRetries)) {
        status = 'closed';
        return;
      }

      status = 'error';
      const delay = Math.min(initialDelay * 2 ** retryCount, maxDelay);
      retryCount++;
      retryTimer = setTimeout(connect, delay);
    };

    if (onmessage) {
      source.onmessage = onmessage;
    }

    if (onevents) {
      for (const [eventName, handler] of Object.entries(onevents)) {
        source.addEventListener(eventName, handler as EventListener);
      }
    }
  }

  function close() {
    closed = true;
    if (retryTimer !== null) {
      clearTimeout(retryTimer);
      retryTimer = null;
    }
    source?.close();
    source = null;
    status = 'closed';
  }

  // Start connecting immediately
  connect();

  return {
    get status() {
      return status;
    },
    close,
  };
}
