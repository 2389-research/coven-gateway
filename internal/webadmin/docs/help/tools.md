# Tools

Tools extend what agents can do beyond simple text responses.

## What are Tools?

Tools are functions that agents can call to perform actions like:

- Executing code
- Searching the web
- Reading and writing files
- Interacting with external services

## Tool Packs

Tools are organized into **packs**. Each pack is a collection of related tools provided by an agent.

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
