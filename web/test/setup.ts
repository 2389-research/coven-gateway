/**
 * Vitest global setup — runs before every test file.
 * Provides browser API mocks that jsdom doesn't include.
 */

// Mock EventSource for components that use SSE (e.g. ConnectionBadge).
// Tests can control the mock instance via the exported helpers.
class MockEventSource {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSED = 2;

  readonly CONNECTING = 0;
  readonly OPEN = 1;
  readonly CLOSED = 2;

  url: string;
  readyState: number = MockEventSource.CONNECTING;
  withCredentials = false;

  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;

  private listeners: Record<string, EventListener[]> = {};

  constructor(url: string | URL, _init?: EventSourceInit) {
    this.url = typeof url === 'string' ? url : url.toString();
    MockEventSource._instances.push(this);
  }

  addEventListener(type: string, listener: EventListener): void {
    (this.listeners[type] ??= []).push(listener);
  }

  removeEventListener(type: string, listener: EventListener): void {
    const list = this.listeners[type];
    if (list) {
      this.listeners[type] = list.filter((l) => l !== listener);
    }
  }

  dispatchEvent(event: Event): boolean {
    const listeners = this.listeners[event.type] ?? [];
    listeners.forEach((l) => l(event));
    return true;
  }

  close(): void {
    this.readyState = MockEventSource.CLOSED;
  }

  // --- Test helpers ---

  /** Simulate a successful connection. */
  simulateOpen(): void {
    this.readyState = MockEventSource.OPEN;
    const event = new Event('open');
    this.onopen?.(event);
    this.dispatchEvent(event);
  }

  /** Simulate an error. */
  simulateError(): void {
    this.readyState = MockEventSource.CLOSED;
    const event = new Event('error');
    this.onerror?.(event);
    this.dispatchEvent(event);
  }

  /** Simulate a message event. */
  simulateMessage(data: string, type = 'message'): void {
    const event = new MessageEvent(type, { data });
    if (type === 'message') {
      this.onmessage?.(event);
    }
    this.dispatchEvent(event);
  }

  // Track all created instances for test assertions.
  static _instances: MockEventSource[] = [];
  static _reset(): void {
    MockEventSource._instances = [];
  }
  static _last(): MockEventSource | undefined {
    return MockEventSource._instances[MockEventSource._instances.length - 1];
  }
}

// Install the mock globally.
globalThis.EventSource = MockEventSource as unknown as typeof EventSource;

// Reset between tests.
beforeEach(() => {
  MockEventSource._reset();
});
