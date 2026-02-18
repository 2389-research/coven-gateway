/**
 * Chat-specific SSE adapter built on createSSEStream.
 *
 * Connects to GET /chat/{agentId}/stream, handles typed events
 * (text, thinking, tool_use, tool_result, done, error, etc.),
 * and accumulates messages in reactive state.
 *
 * Usage:
 *   const chat = createChatStream('agent-123');
 *   // chat.messages — reactive ChatMessage[]
 *   // chat.status — SSEStatus
 *   // chat.isStreaming — true while agent is responding
 *   // chat.close() — must be called on unmount
 */

import { createSSEStream, type SSEStatus } from './sse.svelte';
import type { ChatMessage, ChatMessageType } from '../types/chat';

export interface ChatStreamOptions {
  /** Max reconnection attempts. 0 = infinite. Default: 5. */
  maxRetries?: number;
  /** Initial reconnect delay in ms. Default: 1000. */
  initialDelay?: number;
  /** Max reconnect delay in ms. Default: 30000. */
  maxDelay?: number;
  /** Called when connected to agent stream. */
  onConnected?: (agentId: string) => void;
  /** Called when agent finishes a response. */
  onDone?: () => void;
  /** Called on stream error. */
  onError?: (error: string) => void;
}

export interface ChatStream {
  readonly messages: ChatMessage[];
  readonly status: SSEStatus;
  readonly isStreaming: boolean;
  close(): void;
}

/** Event types the backend sends as named SSE events */
const CHAT_EVENT_TYPES: ChatMessageType[] = [
  'user', 'text', 'thinking', 'tool_use', 'tool_result',
  'error', 'done', 'usage', 'tool_state', 'canceled',
  'tool_approval', 'user_question',
];

let idCounter = 0;

function nextId(): string {
  return `chat-${++idCounter}-${Date.now()}`;
}

export function createChatStream(agentId: string, options: ChatStreamOptions = {}): ChatStream {
  const {
    maxRetries = 5,
    initialDelay = 1000,
    maxDelay = 30000,
    onConnected,
    onDone,
    onError,
  } = options;

  let messages = $state<ChatMessage[]>([]);
  let isStreaming = $state(false);

  function handleEvent(type: ChatMessageType, event: MessageEvent) {
    let data: Record<string, unknown>;
    try {
      data = JSON.parse(event.data);
    } catch {
      return; // Skip malformed events
    }

    if (type === 'done') {
      isStreaming = false;
      onDone?.();
      return;
    }

    if (type === 'usage') {
      // Usage events are metadata, not displayed — skip accumulation
      return;
    }

    const msg: ChatMessage = {
      id: (data.id as string) ?? nextId(),
      type,
      content: (data.content as string) ?? '',
      timestamp: data.timestamp ? new Date(data.timestamp as string) : new Date(),
      toolName: data.tool_name as string | undefined,
      toolId: data.tool_id as string | undefined,
      inputJson: type === 'tool_use' ? (data.content as string) : undefined,
      state: data.state as string | undefined,
      detail: data.detail as string | undefined,
      reason: data.reason as string | undefined,
      questionId: data.question_id as string | undefined,
      question: data.question as string | undefined,
      multiSelect: data.multi_select as boolean | undefined,
      header: data.header as string | undefined,
      timeoutSeconds: data.timeout_seconds as number | undefined,
    };

    // For tool_use, the "content" field from backend is the input JSON
    if (type === 'tool_use') {
      msg.inputJson = msg.content;
      msg.content = '';
    }

    if (type === 'error') {
      onError?.(msg.content);
    }

    // Mark streaming when agent starts responding
    if (type === 'text' || type === 'thinking' || type === 'tool_use') {
      isStreaming = true;
    }

    messages.push(msg);
  }

  // Build named event handlers for createSSEStream
  const onevents: Record<string, (event: MessageEvent) => void> = {};

  for (const type of CHAT_EVENT_TYPES) {
    onevents[type] = (event: MessageEvent) => handleEvent(type, event);
  }

  // Handle the "connected" event separately (not a chat message)
  onevents['connected'] = (event: MessageEvent) => {
    try {
      const data = JSON.parse(event.data);
      onConnected?.(data.agent_id ?? agentId);
    } catch {
      onConnected?.(agentId);
    }
  };

  const url = `/chat/${encodeURIComponent(agentId)}/stream`;

  const stream = createSSEStream(url, {
    onevents,
    maxRetries,
    initialDelay,
    maxDelay,
  });

  function close() {
    stream.close();
    isStreaming = false;
  }

  return {
    get messages() {
      return messages;
    },
    get status() {
      return stream.status;
    },
    get isStreaming() {
      return isStreaming;
    },
    close,
  };
}
