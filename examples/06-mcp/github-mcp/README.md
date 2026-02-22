# GitHub MCP Server Example (HTTP Transport)

This example demonstrates connecting grasp to the official [GitHub MCP Server](https://github.com/github/github-mcp-server) using the Streamable HTTP transport (`HttpMCPClient`).

## Prerequisites

1. **GitHub Token**:
   ```bash
   export GITHUB_TOKEN=your_github_token_here
   ```

2. **Optional: Custom MCP URL** (defaults to GitHub Copilot MCP):
   ```bash
   export GITHUB_MCP_URL=https://your-mcp-server.example.com/mcp/
   ```

3. **Anthropic API credentials** (for AI agent mode):
   ```bash
   export ANTHROPIC_AUTH_TOKEN=your_anthropic_token
   ```

## Running

```bash
cd examples/github-mcp
cp .env.example .env   # fill in credentials
go run .

# Interactive mode
go run . -i

# Verbose mode (show all shell commands)
go run . -v
```

## Architecture

```
┌─────────────────┐     HTTP POST     ┌───────────────────────┐
│   grasp     │◄─────────────────►│  GitHub MCP Server    │
│  VirtualOS      │   (JSON-RPC 2.0)  │  (Streamable HTTP)    │
│                 │                   │                       │
│  /github/tools/ │                   │  Tools: repos, issues │
│  /workspace/    │                   │  PRs, search, etc.    │
└─────────────────┘                   └───────────────────────┘
```

## How It Works

1. **HttpMCPClient** connects to the GitHub MCP Server via Streamable HTTP transport
2. **MCPToolProvider** exposes MCP tools as executable entries in the virtual filesystem
3. Shell commands like `/github/tools/search-repositories --query "..."` are translated to MCP tool calls
4. Results are returned as text output to the shell

## Usage Examples

```bash
# List available tools
ls /github/tools/

# Search repositories
/github/tools/search-repositories --query "mcp server language:go"

# Get file contents
/github/tools/get-file-contents --owner golang --repo go --path README.md

# Search code
/github/tools/search-code --query "context.Context repo:golang/go"

# List issues
/github/tools/list-issues --owner golang --repo go
```

## Flags

| Flag | Description |
|------|-------------|
| `-i` | Interactive mode - chat with AI about GitHub |
| `-v` | Verbose - show all shell commands and output |
