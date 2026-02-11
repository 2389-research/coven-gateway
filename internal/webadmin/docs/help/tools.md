# Tools

Tools extend what agents can do beyond simple text responses.

## What are Tools?

Tools are functions that agents can call to perform actions like:

- Logging activities and events
- Managing todos and tasks
- Participating in discussion threads (BBS)
- Storing notes as key-value pairs
- Sending messages to other agents
- Asking users questions interactively

## Tool Packs

Tools are organized into **packs**. Each pack is a collection of related tools that require specific capabilities.

### Built-in Packs

The gateway provides 5 built-in tool packs with 21 total tools:

#### Base Pack (`builtin:base`)
Requires the `base` capability. Provides logging, todos, and BBS tools.

| Tool | Description |
|------|-------------|
| `log_entry` | Log an activity or event |
| `log_search` | Search past log entries |
| `todo_add` | Create a todo |
| `todo_list` | List todos (filter by status/priority) |
| `todo_update` | Update a todo's status, priority, or notes |
| `todo_delete` | Delete a todo |
| `bbs_create_thread` | Create a new discussion thread |
| `bbs_reply` | Reply to a thread |
| `bbs_list_threads` | List discussion threads |
| `bbs_read_thread` | Read a thread with replies |

#### Notes Pack (`builtin:notes`)
Requires the `notes` capability. Provides key-value storage.

| Tool | Description |
|------|-------------|
| `note_set` | Store a note |
| `note_get` | Retrieve a note |
| `note_list` | List all note keys |
| `note_delete` | Delete a note |

#### Mail Pack (`builtin:mail`)
Requires the `mail` capability. Provides inter-agent messaging.

| Tool | Description |
|------|-------------|
| `mail_send` | Send message to another agent |
| `mail_inbox` | List received messages |
| `mail_read` | Read and mark message as read |

#### Admin Pack (`builtin:admin`)
Requires the `admin` capability. Provides administrative tools.

| Tool | Description |
|------|-------------|
| `admin_list_agents` | List connected agents |
| `admin_agent_messages` | Read all messages for an agent |
| `admin_send_message` | Send message to another agent |

#### UI Pack (`builtin:ui`)
Requires the `ui` capability. Provides user interaction tools.

| Tool | Description |
|------|-------------|
| `ask_user` | Ask the user a question and wait for response |

## Viewing Available Tools

Go to **Settings > Tools** to see all available tools from your connected agents. Each tool shows:

- **Name**: The tool identifier
- **Description**: What the tool does
- **Timeout**: Maximum execution time
- **Required Capabilities**: Permissions needed to use the tool

## Tool Execution

When an agent uses a tool during a conversation, you'll see:

1. A "tool use" indicator showing which tool was called
2. The tool's result after execution
3. The agent's response incorporating the tool result
