# Getting Started

Welcome to Coven Gateway! This guide will help you get up and running quickly.

## What is Coven?

Coven is a control plane for AI agents. It connects your agents to various frontends (web chat, Matrix, Telegram) and manages conversations across all of them.

## Quick Start

1. **Launch an agent**: Run `coven agent` in your terminal to start an agent
2. **Start chatting**: Once connected, click "New Chat" to begin a conversation
3. **Manage agents**: Use the Agents tab in Settings to see connected agents

## Architecture Overview

```
Frontends (Web, Matrix, Telegram)
            │
            ▼
      Coven Gateway
            │
            ▼
    Connected Agents
```

The gateway acts as a central hub, routing messages between frontends and agents while maintaining conversation history.
