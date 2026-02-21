# GRASP Documentation

GRASP (**G**eneral **R**untime for **A**gent **S**hell **P**rimitives) is a mount-based virtual userland for AI agents. It provides a unified namespace where files, tools, APIs, and knowledge bases are all accessed through standard filesystem operations and a built-in shell.

## Explanation — Understanding GRASP

Conceptual discussions about why GRASP exists and how it works.

- [Why GRASP](explanation/why-grasp.md) — The problem GRASP solves, naming rationale, and where it fits in the agent ecosystem
- [Architecture](explanation/architecture.md) — Core design: VirtualOS, MountTable, Shell
- [Provider Model](explanation/provider-model.md) — Capability-based interfaces and runtime detection
- [Shell as Universal Interface](explanation/shell-as-interface.md) — Why shell commands beat tool APIs for agents
- [Integration Strategy](explanation/integration-strategy.md) — MCP, 9P, OpenViking, and cross-language access
- [Union Mount and Cache](explanation/union-mount-and-cache.md) — Plan 9–style union layers, cache-as-configuration, and three invalidation strategies

## Tutorials — Getting Started

Step-by-step learning for newcomers.

- [Getting Started](tutorials/getting-started.md) — Set up GRASP, mount providers, run shell commands
- [Cached Feeds](tutorials/cached-feeds.md) — Build a read-through cache (dbfs over httpfs) with union mount

## How-to Guides — Solving Specific Problems

Practical recipes for common tasks.

- [Create a Custom Provider](how-to/create-provider.md) — Implement your own data source as a mountable provider
- [Build an Agent with Shell Routing](how-to/build-agent.md) — Create an AI agent that routes `!xxx` to shell, other input to LLM
- [Reactive Agents with Hooks](how-to/use-hooks.md) — Use Watch and OnExec hooks for contextual assistance
- [Union Mount and Cache](how-to/union-mount-and-cache.md) — Use `bind`, build cached unions in code, and set up invalidation

## Reference — Technical Details

Precise descriptions of APIs and interfaces.

- [Provider Interfaces](reference/interfaces.md) — Complete API reference for all provider interfaces
- [Built-in Providers](reference/providers.md) — Detailed documentation for all included providers (MemFS, LocalFS, GitHubFS, HTTPFS, MCP, etc.)
- [Union Mount](reference/union-mount.md) — UnionProvider, Layer, BindMode, and the `bind` command
