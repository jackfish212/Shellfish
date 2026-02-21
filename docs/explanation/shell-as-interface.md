# Shell as Universal Interface

Most agent frameworks interact with tools through structured API calls — JSON-schema-defined functions that the LLM invokes by name. GRASP takes a different approach: **the shell is the interface.**

## The Problem with Tool APIs

Consider how a typical agent framework exposes a filesystem:

```json
{
  "tools": [
    {"name": "read_file", "parameters": {"path": "string"}},
    {"name": "write_file", "parameters": {"path": "string", "content": "string"}},
    {"name": "list_directory", "parameters": {"path": "string"}},
    {"name": "search_files", "parameters": {"query": "string", "path": "string"}}
  ]
}
```

To find TODO items in log files, the agent must:

1. Call `list_directory(path="/logs")` → get file list
2. For each file, call `read_file(path="/logs/file.md")` → get content
3. Parse results, filter for TODOs
4. Multiple round-trips, multiple tool calls, high token cost

With GRASP, the same task is one call:

```bash
grep TODO /logs/*.md
```

Or with composition:

```bash
cat /logs/2026-02-*.md | grep TODO | head -10
```

**One tool call. One round-trip. The shell handles the composition.**

## Why LLMs Are Good at Shell

LLMs have been trained on billions of lines of shell commands, man pages, and Stack Overflow answers. Shell syntax is deeply embedded in their weights. When you give an LLM a shell interface:

- It already knows `ls`, `cat`, `grep`, `head`, `tail`.
- It already knows pipes, redirects, and logical operators.
- It already knows how to compose commands to solve novel problems.
- It doesn't need to learn a custom tool schema.

This is not speculation — it's the reason projects like Claude Code, Cursor, and Codex all provide shell access to their agents. **Shell is the most token-efficient and composable interface available.**

## GRASP Shell vs. Real Shell

GRASP's shell is not bash. It deliberately limits scope:

**Included (useful for agents):**
- Command execution with arguments
- Pipes (`cmd1 | cmd2 | cmd3`)
- Output redirection (`>`, `>>`, `2>&1`)
- Logical operators (`&&`, `||`)
- Environment variables (`$HOME`, `${VAR}`)
- Here-documents (`<<EOF`)
- Command groups (`{ cmd1; cmd2; }`)
- Tilde expansion (`~`)
- History

**Excluded (unnecessary complexity or security risk):**
- Process management (`&`, `bg`, `fg`, `jobs`)
- Subshells (`$(...)`, backticks)
- Loops and conditionals (`for`, `if`, `while`)
- Globbing (`*.md` — files are matched by the commands themselves)
- Signal handling
- User/group permissions (simplified to read/write/exec flags)

This is intentional. Agents don't need a Turing-complete shell — they have the LLM for complex logic. They need a **composable data pipeline** with familiar syntax.

## One Tool, Many Operations

From the agent framework's perspective, GRASP exposes a single tool:

```json
{
  "name": "shell",
  "description": "Execute a command in the GRASP virtual filesystem",
  "parameters": {
    "command": {"type": "string", "description": "Shell command to execute"}
  }
}
```

Through this single tool, the agent can:

- Browse available resources: `ls /`
- Read files: `cat /data/config.yaml`
- Search across all data sources: `search "authentication" --scope /knowledge`
- Chain operations: `cat /data/users.json | grep admin | head -5`
- Write outputs: `echo "task completed" > /memory/log.md`
- Discover tools: `ls /tools/` or `which search`
- Inspect system state: `mount` or `cat /proc/version`

Compare this with traditional approaches that require 10+ separate tool definitions for the same functionality. Fewer tools means simpler system prompts, less schema overhead, and fewer opportunities for the LLM to pick the wrong tool.

## Discoverability

A key advantage of the filesystem metaphor is **discoverability**. An agent dropped into an unfamiliar GRASP environment can orient itself:

```bash
$ ls /
bin/  data/  knowledge/  memory/  proc/  tools/  tmp/

$ ls /tools/
notion/  github/  calendar/

$ ls /tools/notion/
search_pages  create_page  update_page

$ mount
/           memfs       (rw, exec, mutable)
/data       localfs     (rw, search, mutable)
/knowledge  viking      (ro, search)
/tools      mcp-tools   (ro, exec)
/memory     sqlitefs    (rw, mutable)
```

The agent learns the environment by exploring it — the same way a developer learns a new system by running `ls` and `cat`.
