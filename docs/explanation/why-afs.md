# Why Shellfish

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

Shellfish brings this principle to AI agents.

## What Shellfish Does

Shellfish is a **virtual userland** — not a framework, not a database, not a tool registry. It provides:

1. **A mount-based namespace.** Any data source implements the `Provider` interface and gets mounted at a path. Local files, SQLite databases, MCP servers, semantic search engines — they all appear as directories and files under one tree.

2. **A built-in shell.** Agents interact through shell commands — `ls`, `cat`, `grep`, `search`, pipes, redirects. No custom APIs to learn. No tool schemas to memorize.

3. **Capability-based access control.** Each provider declares what it can do (read, write, execute, search) through Go interface composition. Shellfish detects capabilities at runtime. A read-only knowledge base won't accidentally receive writes.

4. **Multi-protocol access.** The same filesystem is exposed through multiple protocols — Go SDK for embedded use, MCP for agent framework integration, 9P for cross-language POSIX access.

## Where Shellfish Fits

Shellfish is **not** an agent framework. It doesn't manage conversations, call LLMs, or orchestrate tasks. It's the layer below — the operating environment that any agent framework can use.

```
┌─────────────────────────────────┐
│  Agent Framework                │
│  (OpenClaw, PicoClaw, custom)   │
├─────────────────────────────────┤
│  Shellfish — Virtual Userland   │  ← this layer
│  mount, shell, providers        │
├─────────────────────────────────┤
│  Data Sources                   │
│  (local files, DBs, APIs, MCP)  │
└─────────────────────────────────┘
```

Think of it as Docker for agent context: Docker doesn't replace your application, it provides an isolated, composable runtime. Shellfish doesn't replace your agent, it provides a unified, composable data layer.

## Comparison with Alternatives

### vs. OpenClaw

OpenClaw is a complete agent runtime — it manages conversations, calls LLMs, and provides built-in tools for shell, browser, and filesystem access. But its tools are hardcoded to the host OS: `read_file` reads a real file, `exec` runs a real shell command.

Shellfish is the layer *underneath*. It virtualizes the environment: mount multiple data sources into one namespace, with capability-based access control and a sandboxed shell. OpenClaw could use Shellfish as an MCP backend, gaining mount composition without changing its architecture.

### vs. Turso AgentFS

AgentFS provides SQLite-backed copy-on-write file isolation — one portable file containing the agent's entire state. It's excellent for sandboxing and reproducibility.

Shellfish solves a different problem: unifying heterogeneous data sources. AgentFS isolates *one* filesystem; Shellfish *composes* many. They're complementary — a SQLiteFS provider could wrap AgentFS as one mount among many.

### vs. OpenViking

OpenViking is ByteDance's context database — it organizes memories, resources, and skills under `viking://` URIs with L0/L1/L2 tiered loading and semantic retrieval. It's fundamentally a storage and retrieval system.

Shellfish is a runtime, not a database. OpenViking can be mounted into Shellfish as a `VikingProvider`, giving agents shell access to semantic retrieval through `cat`, `search`, and pipes — combining OpenViking's intelligence with Shellfish's composability.

### vs. ToolFS

Both are Go virtual filesystems for agents. Key differences:

- ToolFS requires FUSE (a kernel module); Shellfish is pure userspace with protocol-native access (MCP, 9P).
- ToolFS bundles RAG, WASM skills, and memory into one monolith; Shellfish keeps the core minimal and mounts these as separate providers.
- ToolFS has a single `/toolfs` namespace; Shellfish supports multiple independent mount points with longest-prefix resolution.

### vs. AIOS

AIOS is an academic "LLM operating system" that manages multiple agents with scheduling, context switching, and resource allocation. It operates at a higher level of abstraction — orchestrating agents, not providing their runtime environment.

Shellfish operates at a lower level: giving individual agents a composable data layer. AIOS could use Shellfish as the filesystem substrate for each managed agent.

### vs. Agent OS (smartcomputer-ai)

Agent OS is a Rust-based runtime for self-evolving agents with capability security, deterministic replay, and constitutional self-modification loops. It focuses on agent governance and autonomy.

Shellfish focuses on data access and tool composition. They target different layers — Agent OS could use Shellfish for its filesystem needs.

### vs. MCP Filesystem Server

MCP filesystem servers expose flat tool lists (`read_file`, `write_file`, `search_files`) for a single directory. Shellfish provides a full VFS with mount points, shell composition, and capability detection. An MCP filesystem server could be *mounted into* Shellfish as one provider among many.

### vs. Tool APIs (LangChain, OpenAI Function Calling)

Tool registries are flat lists of callable functions. Shellfish organizes tools as executable files in a hierarchical namespace, making them discoverable (`ls /tools/`), composable (`tool1 | tool2`), and governed by permissions.
