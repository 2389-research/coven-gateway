# Agents

Agents are the AI backends that process your messages and generate responses.

## Connecting an Agent

Agents are provided by the [coven](https://github.com/2389-research/coven) Rust project. After installing from there, run:

```bash
coven agent --gateway <your-gateway-address>
```

The agent will automatically register with the gateway and appear in your Agents list.

## Agent Status

Agents can have the following statuses:

- **pending**: Agent registered but awaiting admin approval
- **approved**: Agent is approved and can connect
- **revoked**: Agent access has been revoked
- **online**: Agent is currently connected to the gateway
- **offline**: Agent is approved but not currently connected

## Multiple Agents

You can connect multiple agents simultaneously. When starting a new chat, you'll be prompted to select which agent to use.

## Agent Capabilities

Each agent may have different capabilities and tools available. Check the Tools tab in Settings to see what tools are available from your connected agents.
