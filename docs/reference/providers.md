# Built-in Providers

Package: `github.com/jackfish212/grasp/mounts`

GRASP ships with a comprehensive set of providers for common use cases. Each provider implements a subset of the capability interfaces (Provider, Readable, Writable, Executable, Searchable, Mutable).

## Provider Summary

| Provider | Interfaces | Use Case |
|----------|------------|----------|
| MemFS | All | In-memory scratch space, custom commands |
| LocalFS | Read, Write, Search, Mutate | Host filesystem access |
| SQLiteFS | Read, Write, Mutate | Persistent storage |
| GitHubFS | Read, Search | GitHub API as filesystem |
| HTTPFS | Read | HTTP endpoints as filesystem |
| MCPToolProvider | Read, Exec, Search | MCP tools as executables |
| MCPResourceProvider | Read, Search | MCP resources as files |
| VikingProvider | Read, Search | OpenViking context database |

---

## MemFS — In-memory Filesystem

**Interfaces:** Provider, Readable, Writable, Executable, Mutable

The Swiss Army knife provider. Stores files and directories in memory. Supports registering Go functions as executable entries.

```go
fs := mounts.NewMemFS(grasp.PermRW)

// Add files
fs.AddFile("config.yaml", []byte("key: value"), grasp.PermRO)
fs.AddDir("data")

// Add executable functions
fs.AddExecFunc("hello", func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
    name := "world"
    if len(args) > 0 {
        name = args[0]
    }
    return io.NopCloser(strings.NewReader(fmt.Sprintf("Hello, %s!\n", name))), nil
}, mounts.FuncMeta{
    Description: "Say hello",
    Usage:       "hello [name]",
})
```

**When to use:**
- Temporary workspace for agent operations
- Custom commands via `AddExecFunc()`
- Testing and prototyping

---

## LocalFS — Host Filesystem Mount

**Interfaces:** Provider, Readable, Writable, Searchable, Mutable

Maps a host directory into GRASP. File operations delegate directly to the OS.

```go
fs := mounts.NewLocalFS("/home/user/projects", grasp.PermRW)
v.Mount("/projects", fs)

// Now: cat /projects/readme.md → reads /home/user/projects/readme.md
```

**When to use:**
- Accessing local project files
- Reading/writing host configuration
- File-based data pipelines

---

## SQLiteFS — Persistent Filesystem

**Interfaces:** Provider, Readable, Writable, Mutable

Stores files and metadata in a SQLite database. Data persists across process restarts.

```go
fs, err := mounts.NewSQLiteFS("/var/data/agent.db", grasp.PermRW)
if err != nil {
    panic(err)
}
v.Mount("/memory", fs)

// Data survives restarts
sh.Execute(ctx, "echo 'remember this' | write /memory/notes.md")
```

**When to use:**
- Agent memory that must survive restarts
- Session logs and audit trails
- Cross-session state persistence

---

## GitHubFS — GitHub API as Filesystem

**Interfaces:** Provider, Readable, Searchable

Mounts GitHub API as a virtual filesystem with the following structure:

```
/repos                              → list repositories
/repos/{owner}/{repo}               → repository info
/repos/{owner}/{repo}/contents/...  → repository files (read-only)
/repos/{owner}/{repo}/issues        → issues list
/repos/{owner}/{repo}/issues/{N}    → issue #N details
```

```go
fs := mounts.NewGitHubFS(
    mounts.WithGitHubToken("ghp_xxxx"),
    mounts.WithGitHubUser("myorg"),  // default user/org
    mounts.WithGitHubCacheTTL(10*time.Minute),
)
v.Mount("/github", fs)

// Browse repositories
sh.Execute(ctx, "ls /github/repos/myorg")
sh.Execute(ctx, "cat /github/repos/myorg/myrepo/contents/README.md")
sh.Execute(ctx, "cat /github/repos/myorg/myrepo/issues/42")
```

**Configuration options:**
| Option | Description |
|--------|-------------|
| `WithGitHubToken(token)` | GitHub personal access token |
| `WithGitHubUser(user)` | Default user/organization for relative paths |
| `WithGitHubBaseURL(url)` | Custom API URL (GitHub Enterprise) |
| `WithGitHubCacheTTL(ttl)` | Cache TTL (default: 5 minutes) |

**When to use:**
- Browsing repository contents
- Reading issues and discussions
- Code review workflows

---

## HTTPFS — HTTP Endpoints as Filesystem

> **Note:** HTTPFS has been moved to a separate package: `github.com/jackfish212/httpfs`

**Interfaces:** Provider, Readable

Maps HTTP endpoints to a virtual filesystem with automatic response parsing. Each configured source becomes a directory containing parsed content files.

```go
import "github.com/jackfish212/httpfs"

fs := httpfs.NewHTTPFS()

// Add sources with parsers
fs.Add("news", "https://example.com/feed.xml", &httpfs.RSSParser{})
fs.Add("api", "https://api.example.com/data", &httpfs.JSONParser{})
fs.Add("raw", "https://example.com/readme.txt", &httpfs.RawParser{})

v.Mount("/http", fs)

// Browse parsed content
sh.Execute(ctx, "ls /http/news")          // list feed items
sh.Execute(ctx, "cat /http/news/item-1.txt")  // read specific item
```

**Available parsers:**

| Parser | Description | Output |
|--------|-------------|--------|
| `RSSParser` | RSS 2.0 and Atom feeds | One file per item |
| `JSONParser` | JSON responses | Parsed structure |
| `RawParser` | Raw response body | Single file |
| `AutoParser` | Auto-detect format | Tries RSS/Atom first, then raw |

**Dynamic source addition via shell:**
```bash
# Add source dynamically
echo "https://example.com/feed.xml" > /http/newsource
ls /http/newsource
```

**When to use:**
- RSS/Atom feed monitoring
- API endpoint polling
- Web content integration

---

## MCP Providers — Model Context Protocol

GRASP integrates with the [MCP ecosystem](https://modelcontextprotocol.io) through two provider types:

### MCPToolProvider

**Interfaces:** Provider, Readable, Executable, Searchable

Exposes MCP server tools as executable entries under `/tools/`.

```go
// Create MCP client (stdio transport)
client := mounts.NewStdioMCPClient("npx -y @modelcontextprotocol/server-filesystem /workspace")

// Or HTTP transport
client := mounts.NewHttpMCPClient("https://mcp.example.com",
    mounts.WithBearerToken("token"))

// Mount tools
toolProvider := mounts.NewMCPToolProvider(client)
v.Mount("/tools/fs", toolProvider)

// List and execute tools
sh.Execute(ctx, "ls /tools/fs")
sh.Execute(ctx, "/tools/fs/read_file --path /workspace/README.md")
```

### MCPResourceProvider

**Interfaces:** Provider, Readable, Searchable

Exposes MCP server resources as readable files under `/data/`.

```go
resourceProvider := mounts.NewMCPResourceProvider(client)
v.Mount("/data/mcp", resourceProvider)

// Read resources
sh.Execute(ctx, "ls /data/mcp")
sh.Execute(ctx, "cat /data/mcp/config.json")
```

### MountMCP — Convenience Function

Mounts both tools and resources in one call:

```go
mounts.MountMCP(v, "/mcp", client)

// Tools available at /mcp/tools/
// Resources available at /mcp/data/
// Prompts available at /mcp/prompts/
```

**When to use:**
- Connecting to MCP servers (filesystem, databases, APIs)
- Integrating with Claude Desktop / OpenAI Agents
- Tool discovery and execution

---

## VikingProvider — OpenViking Integration

**Interfaces:** Provider, Readable, Searchable

Connects to [OpenViking](https://github.com/volcengine/OpenViking) context database with L0/L1/L2 tiered content loading.

**Virtual files:**
- `.abstract` — L0 content (~100 tokens)
- `.overview` — L1 content (~2K tokens)

```go
fs := mounts.NewVikingProvider(vikingClient, "viking://")
v.Mount("/ctx", fs)

// Navigate context hierarchy
sh.Execute(ctx, "ls /ctx/resources")

// Read tiered content
sh.Execute(ctx, "cat /ctx/resources/my_project/.abstract")
sh.Execute(ctx, "cat /ctx/resources/my_project/.overview")
sh.Execute(ctx, "cat /ctx/resources/my_project/src/main.go")
```

**When to use:**
- Large codebase navigation
- Semantic search over knowledge bases
- Tiered context loading for cost optimization

---

## MCP Client Types

### StdioMCPClient

JSON-RPC over stdin/stdout for local MCP servers.

```go
client := mounts.NewStdioMCPClient("npx -y @modelcontextprotocol/server-filesystem /path")
```

### HttpMCPClient

HTTP transport for remote MCP servers (Streamable HTTP).

```go
client := mounts.NewHttpMCPClient("https://mcp.example.com",
    mounts.WithBearerToken("your-token"),
    mounts.WithHTTPTimeout(30*time.Second),
)
```

---

## MCP Interfaces

```go
type MCPClient interface {
    ListTools(ctx context.Context) ([]MCPTool, error)
    CallTool(ctx context.Context, name string, args map[string]any) (*MCPToolResult, error)
    ListResources(ctx context.Context) ([]MCPResource, error)
    ReadResource(ctx context.Context, uri string) (string, error)
    ListPrompts(ctx context.Context) ([]MCPPrompt, error)
    GetPrompt(ctx context.Context, name string, args map[string]any) (string, error)
}

type MCPTool struct {
    Name        string
    Description string
    InputSchema map[string]any
}

type MCPToolResult struct {
    Content []MCPContent
    IsError bool
}

type MCPResource struct {
    URI         string
    Name        string
    Description string
    MimeType    string
}

type MCPPrompt struct {
    Name        string
    Description string
    Arguments   []MCPPromptArgument
}
```
