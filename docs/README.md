# AFS Documentation

AFS (Agent File System) is a mount-based virtual filesystem runtime for AI agents. It provides a unified namespace where files, tools, APIs, and knowledge bases are all accessed through standard filesystem operations and a built-in shell.

## Explanation — Understanding AFS

Conceptual discussions about why AFS exists and how it works.

- [Why AFS](explanation/why-afs.md) — The problem AFS solves and where it fits in the agent ecosystem
- [Architecture](explanation/architecture.md) — Core design: VirtualOS, MountTable, Shell
- [Provider Model](explanation/provider-model.md) — Capability-based interfaces and runtime detection
- [Shell as Universal Interface](explanation/shell-as-interface.md) — Why shell commands beat tool APIs for agents
- [Integration Strategy](explanation/integration-strategy.md) — MCP, 9P, OpenViking, and cross-language access

## Tutorials — Getting Started

Step-by-step learning for newcomers.

- [Getting Started](tutorials/getting-started.md) — Set up AFS, mount providers, run shell commands

## How-to Guides — Solving Specific Problems

Practical recipes for common tasks.

- [Create a Custom Provider](how-to/create-provider.md) — Implement your own data source as a mountable provider

## Reference — Technical Details

Precise descriptions of APIs and interfaces.

- [Provider Interfaces](reference/interfaces.md) — Complete API reference for all provider interfaces
