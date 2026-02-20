# Why AFS

## The Problem

AI agent frameworks are proliferating — OpenClaw, BearClaw, PicoClaw, Nanobot, and many more. They all face the same structural problem: **how does an agent interact with the world?**

The dominant approach is **tool APIs**: define a JSON schema for each tool, register it with the agent framework, and let the LLM call them by name. This works, but it creates fragmentation:

- Each tool is an island with its own schema, its own error format, its own way of returning results.
- Composing tools requires the LLM to chain multiple API calls, burning tokens on orchestration.
- Adding a new data source means writing a new tool, updating schemas, and re-deploying.
- There's no uniform way to browse, search, or discover what's available.

Meanwhile, the context management problem is equally fragmented:

- Memory lives in one system (vector DB, JSON files, or proprietary stores).
- Tools live in another (function registries, MCP servers, plugin manifests).
- Resources live in yet another (local files, S3, databases).
- The agent has to learn three different interfaces to access its own context.

## The Unix Insight

Unix solved this exact problem 50 years ago with a single principle: **everything is a file**.

Disks, devices, processes, network sockets — they all appear as entries in one namespace. This means:

- `cat` works on any file, regardless of where it lives.
- `ls` shows you what's available, everywhere.
- Pipes compose arbitrary programs: `grep error /var/log/syslog | head -5`.
- New devices don't require new commands — just mount them and the existing tools work.

AFS brings this principle to AI agents.

## What AFS Does

AFS is a **virtual filesystem runtime** — not a framework, not a database, not a tool registry. It provides:

1. **A mount-based namespace.** Any data source implements the `Provider` interface and gets mounted at a path. Local files, SQLite databases, MCP servers, semantic search engines — they all appear as directories and files under one tree.

2. **A built-in shell.** Agents interact with AFS through shell commands — `ls`, `cat`, `grep`, `search`, pipes, redirects. No custom APIs to learn. No tool schemas to memorize.

3. **Capability-based access control.** Each provider declares what it can do (read, write, execute, search) through Go interface composition. AFS detects capabilities at runtime. A read-only knowledge base won't accidentally receive writes.

4. **Multi-protocol access.** The same filesystem is exposed through multiple protocols — Go SDK for embedded use, MCP for agent framework integration, 9P for cross-language POSIX access.

## Where AFS Fits

AFS is **not** an agent framework. It doesn't manage conversations, call LLMs, or orchestrate tasks. It's the layer below — the operating environment that any agent framework can use.

```
┌─────────────────────────────────┐
│  Agent Framework                │
│  (OpenClaw, PicoClaw, custom)   │
├─────────────────────────────────┤
│  AFS — Virtual Filesystem       │  ← this layer
│  mount, shell, providers        │
├─────────────────────────────────┤
│  Data Sources                   │
│  (local files, DBs, APIs, MCP)  │
└─────────────────────────────────┘
```

Think of it as Docker for agent context: Docker doesn't replace your application, it provides an isolated, composable runtime. AFS doesn't replace your agent, it provides a unified, composable data layer.

## Comparison with Alternatives

**vs. MCP Filesystem Server**: MCP filesystem servers expose flat tool lists (`read_file`, `write_file`). AFS provides a full VFS with mount points, shell composition, and capability detection. An MCP filesystem server could be *mounted into* AFS as one provider among many.

**vs. OpenViking**: OpenViking is a context database with filesystem metaphors — it uses `viking://` URIs and `ls`/`find` semantics, but it's fundamentally a Python library backed by vector search and VLMs. AFS is the actual filesystem runtime. OpenViking can be mounted into AFS as a `VikingProvider`, giving agents shell access to OpenViking's semantic retrieval through standard commands.

**vs. Agent tool registries**: Tool registries (LangChain tools, OpenAI function calling) are flat lists of callable functions. AFS organizes tools as executable files in a hierarchical namespace, making them discoverable (`ls /tools/`), composable (`tool1 | tool2`), and governed by permissions.
