# Getting Started

Welcome to Coven Gateway! This guide will help you get up and running quickly.

## What is Coven?

Coven is a control plane for AI agents. It connects your agents to various frontends (web chat, Matrix, Telegram) and manages conversations across all of them.

## Quick Start

1. **Launch an agent**: Install from [coven](https://github.com/2389-research/coven), then run `coven agent --server <your-gateway-address>`
2. **Start chatting**: Once an agent is connected, click on it in the sidebar to begin a conversation
3. **Manage agents**: Connected agents appear in the sidebar; settings are in the top-right menu

## Architecture Overview

```text
Frontends (Web, Matrix, Telegram)
            │
            ▼
      Coven Gateway
            │
            ▼
    Connected Agents
```

The gateway acts as a central hub, routing messages between frontends and agents while maintaining conversation history.
