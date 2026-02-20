/**
 * Chat message types matching the backend chatMessage struct
 * in internal/webadmin/chat.go.
 */

export type ChatMessageType =
  | 'user'
  | 'text'
  | 'thinking'
  | 'tool_use'
  | 'tool_result'
  | 'error'
  | 'done'
  | 'usage'
  | 'tool_state'
  | 'canceled'
  | 'tool_approval'
  | 'user_question';

export interface ChatMessage {
  id: string;
  type: ChatMessageType;
  content: string;
  timestamp: Date;

  // Tool fields (tool_use, tool_result)
  toolName?: string;
  toolId?: string;
  inputJson?: string;

  // Usage fields
  inputTokens?: number;
  outputTokens?: number;
  cacheReadTokens?: number;
  cacheWriteTokens?: number;
  thinkingTokens?: number;

  // Tool state
  state?: string;
  detail?: string;

  // Canceled
  reason?: string;

  // User question
  questionId?: string;
  question?: string;
  options?: QuestionOption[];
  multiSelect?: boolean;
  header?: string;
  timeoutSeconds?: number;
}

export interface QuestionOption {
  label: string;
  description?: string;
}

/** Sender derived from message type */
export type MessageSender = 'user' | 'agent' | 'system';

export function getMessageSender(type: ChatMessageType): MessageSender {
  switch (type) {
    case 'user':
      return 'user';
    case 'error':
    case 'done':
    case 'usage':
    case 'canceled':
      return 'system';
    default:
      return 'agent';
  }
}

/** Check if a message type is displayable in the chat thread */
export function isDisplayableMessage(type: ChatMessageType): boolean {
  return type !== 'done' && type !== 'usage';
}

/** Group messages by date for date separator rendering */
export function groupMessagesByDate(messages: ChatMessage[]): Map<string, ChatMessage[]> {
  const groups = new Map<string, ChatMessage[]>();
  for (const msg of messages) {
    const key = msg.timestamp.toLocaleDateString();
    const group = groups.get(key);
    if (group) {
      group.push(msg);
    } else {
      groups.set(key, [msg]);
    }
  }
  return groups;
}
