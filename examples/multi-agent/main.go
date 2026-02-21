// Multi-Agent Collaboration — Three agents share a grasp VOS namespace
// and collaborate through the filesystem: Explorer -> Architect -> Reporter.
//
// Each agent has an isolated shell (own cwd, env, history) but sees the same
// mounted filesystems. Communication is purely file-based — no message passing.
//
// Run:
//
//	cd examples/multi-agent
//	cp .env.example .env   # fill in credentials
//	go run .
//
// Flags:
//
//	-v    Show each shell command and its output
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	grasp "github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/builtins"
	"github.com/jackfish212/grasp/mounts"
	"github.com/joho/godotenv"
)

type Agent struct {
	Name   string
	Role   string
	System string
	Task   string
}

var verbose bool

func main() {
	flag.BoolVar(&verbose, "v", false, "Show shell commands and output")
	flag.Parse()

	if err := godotenv.Load(filepath.Join(".", ".env")); err != nil {
		log.Printf("Warning: .env not loaded: %v", err)
	}

	v := grasp.New()
	rootFS, err := grasp.Configure(v)
	if err != nil {
		log.Fatalf("Configure: %v", err)
	}
	builtins.RegisterBuiltinsOnFS(v, rootFS)

	if err := v.Mount("/project", mounts.NewMemFS(grasp.PermRW)); err != nil {
		log.Fatalf("mount /project: %v", err)
	}
	if err := v.Mount("/shared", mounts.NewMemFS(grasp.PermRW)); err != nil {
		log.Fatalf("mount /shared: %v", err)
	}

	outputDir := filepath.Join(".", "output")
	os.MkdirAll(outputDir, 0o755)
	if err := v.Mount("/output", mounts.NewLocalFS(outputDir, grasp.PermRW)); err != nil {
		log.Fatalf("mount /output: %v", err)
	}

	ctx := context.Background()
	setup := v.Shell("setup")
	setup.Execute(ctx, "mkdir /shared/explorer")
	setup.Execute(ctx, "mkdir /shared/architect")

	seedProject(v)

	fmt.Println("========================================")
	fmt.Println("  grasp Multi-Agent Collaboration")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("Three agents share one VOS namespace,")
	fmt.Println("coordinating through the filesystem.")
	fmt.Println()
	fmt.Println("  /project  [MemFS]   Code to analyze")
	fmt.Println("  /shared   [MemFS]   Inter-agent workspace")
	fmt.Println("  /output   [LocalFS] Final deliverables")
	fmt.Println()

	client := newClient()
	tool := shellToolDef()
	agents := defineAgents()

	for i, agent := range agents {
		fmt.Printf("--- [%d/%d] %s (%s) ---\n", i+1, len(agents), agent.Role, agent.Name)
		start := time.Now()

		runAgent(ctx, v, agent, client, tool)

		if !verbose {
			fmt.Println()
		}
		fmt.Printf("[done in %s]\n", time.Since(start).Round(time.Second))
		listCreatedFiles(v, agent.Name)
		fmt.Println()
	}

	fmt.Println("========== FINAL REPORT ==========")
	fmt.Println()
	viewer := v.Shell("viewer")
	result := viewer.Execute(ctx, "cat /output/report.md")
	if result.Output != "" {
		fmt.Println(result.Output)
	} else {
		fmt.Println("(no report generated)")
	}
	fmt.Printf("\nReport saved to: %s\n", filepath.Join(outputDir, "report.md"))
}

// defineAgents returns the three-stage pipeline.
// Each agent reads from the previous stage's output directory.
func defineAgents() []Agent {
	return []Agent{
		{
			Name: "explorer",
			Role: "Code Explorer",
			System: `You are a code explorer. Examine codebases and produce structured findings.
Available commands: ls, cat, grep, find, head, tail, stat, write, mkdir, echo, cd, pwd.
Write multi-line files using: cat << 'EOF' > /path/to/file.md
(content)
EOF`,
			Task: `Explore /project and create these files:

1. /shared/explorer/structure.md - file tree with one-line description per file
2. /shared/explorer/todos.md - every TODO/FIXME with file locations
3. /shared/explorer/summary.md - one paragraph describing what this project does

Read every file in /project first, then write your findings.`,
		},
		{
			Name: "architect",
			Role: "Software Architect",
			System: `You are a software architect analyzing code quality and design.
Available commands: ls, cat, grep, find, head, tail, stat, write, mkdir, echo, cd, pwd.
Explorer findings: /shared/explorer/. Source code: /project/.
Write multi-line files using: cat << 'EOF' > /path/to/file.md
(content)
EOF`,
			Task: `Read explorer findings (/shared/explorer/*) and source code (/project/*), then create:

1. /shared/architect/analysis.md - design patterns, strengths, issues found
2. /shared/architect/suggestions.md - prioritized improvements (critical / important / nice-to-have)

Read all input files before writing.`,
		},
		{
			Name: "reporter",
			Role: "Technical Writer",
			System: `You are a technical writer producing professional reports.
Available commands: ls, cat, grep, find, head, tail, stat, write, mkdir, echo, cd, pwd.
Explorer findings: /shared/explorer/. Architect analysis: /shared/architect/.
Write multi-line files using: cat << 'EOF' > /path/to/file.md
(content)
EOF`,
			Task: `Read all material from /shared/explorer/ and /shared/architect/, then create:

/output/report.md - a code review report with these sections:
1. Executive Summary
2. Project Structure
3. Architecture Analysis
4. Issues Found
5. Prioritized Action Items
6. Recommendations

Read all input files before writing the report.`,
		},
	}
}

// runAgent drives one agent through the LLM tool-use loop.
// The shell persists across all tool calls — cd, env, and history carry over.
func runAgent(ctx context.Context, v *grasp.VirtualOS, agent Agent, client anthropic.Client, tool anthropic.ToolParam) {
	sh := v.Shell(agent.Name)
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(agent.Task)),
	}

	model := getModel()

	for {
		resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     model,
			MaxTokens: 4096,
			System:    []anthropic.TextBlockParam{{Text: agent.System}},
			Messages:  messages,
			Tools:     []anthropic.ToolUnionParam{{OfTool: &tool}},
		})
		if err != nil {
			log.Printf("[%s] API error: %v", agent.Name, err)
			return
		}

		messages = append(messages, resp.ToParam())

		var toolResults []anthropic.ContentBlockParamUnion
		hasToolUse := false

		for _, block := range resp.Content {
			switch b := block.AsAny().(type) {
			case anthropic.TextBlock:
				if verbose {
					fmt.Printf("  [%s] %s\n", agent.Name, b.Text)
				}
			case anthropic.ToolUseBlock:
				hasToolUse = true
				var input struct {
					Command string `json:"command"`
				}
				json.Unmarshal([]byte(b.JSON.Input.Raw()), &input)

				if verbose {
					fmt.Printf("  $ %s\n", input.Command)
				} else {
					fmt.Print(".")
				}

				result := sh.Execute(ctx, input.Command)
				output := result.Output
				if result.Code != 0 {
					if output != "" && !strings.HasSuffix(output, "\n") {
						output += "\n"
					}
					output += fmt.Sprintf("[exit code: %d]", result.Code)
				}

				if verbose && output != "" {
					fmt.Printf("  %s\n", truncate(output, 500))
				}

				toolResults = append(toolResults, anthropic.NewToolResultBlock(b.ID, output, result.Code != 0))
			}
		}

		if !hasToolUse {
			break
		}
		messages = append(messages, anthropic.NewUserMessage(toolResults...))
	}
}

// seedProject populates /project with a small Go HTTP API that has
// intentional design issues for the agents to discover and analyze.
func seedProject(v *grasp.VirtualOS) {
	ctx := context.Background()

	v.Write(ctx, "/project/README.md", strings.NewReader(`# BookStore API

A REST API for managing a book catalog.

## Endpoints

- GET    /books      List all books
- GET    /books/:id  Get a book by ID
- POST   /books      Add a new book
- DELETE /books/:id  Delete a book

## TODO

- [ ] Add pagination to list endpoint
- [ ] Implement search by author/title
- [ ] Add book categories and tags
- [ ] Replace in-memory store with database
- [ ] Add authentication and authorization
- [ ] Write unit and integration tests
`))

	v.Write(ctx, "/project/go.mod", strings.NewReader("module github.com/example/bookstore\n\ngo 1.22\n"))

	v.Write(ctx, "/project/main.go", strings.NewReader(`package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

var store = NewStore()

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /books", handleListBooks)
	mux.HandleFunc("GET /books/{id}", handleGetBook)
	mux.HandleFunc("POST /books", handleCreateBook)
	mux.HandleFunc("DELETE /books/{id}", handleDeleteBook)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// TODO: add graceful shutdown with signal handling
	// TODO: add request logging middleware
	fmt.Printf("listening on :%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
`))

	v.Write(ctx, "/project/book.go", strings.NewReader("package main\n\nimport \"time\"\n\ntype Book struct {\n\tID        string    `json:\"id\"`\n\tTitle     string    `json:\"title\"`\n\tAuthor    string    `json:\"author\"`\n\tYear      int       `json:\"year\"`\n\tCreatedAt time.Time `json:\"created_at\"`\n}\n"))

	v.Write(ctx, "/project/store.go", strings.NewReader(`package main

import (
	"fmt"
	"time"
)

// FIXME: not safe for concurrent access - needs sync.Mutex
type Store struct {
	books  map[string]*Book
	nextID int
}

func NewStore() *Store {
	return &Store{books: make(map[string]*Book), nextID: 1}
}

func (s *Store) List() []*Book {
	// TODO: add pagination and sorting
	result := make([]*Book, 0, len(s.books))
	for _, b := range s.books {
		result = append(result, b)
	}
	return result
}

func (s *Store) Get(id string) (*Book, error) {
	b, ok := s.books[id]
	if !ok {
		return nil, fmt.Errorf("book not found: %s", id)
	}
	return b, nil
}

func (s *Store) Create(title, author string, year int) *Book {
	id := fmt.Sprintf("%d", s.nextID)
	s.nextID++
	b := &Book{
		ID: id, Title: title, Author: author,
		Year: year, CreatedAt: time.Now(),
	}
	s.books[id] = b
	return b
}

func (s *Store) Delete(id string) error {
	if _, ok := s.books[id]; !ok {
		return fmt.Errorf("book not found: %s", id)
	}
	delete(s.books, id)
	return nil
}
`))

	v.Write(ctx, "/project/handlers.go", strings.NewReader("package main\n\nimport (\n\t\"encoding/json\"\n\t\"net/http\"\n)\n\nfunc handleListBooks(w http.ResponseWriter, r *http.Request) {\n\tbooks := store.List()\n\tw.Header().Set(\"Content-Type\", \"application/json\")\n\tjson.NewEncoder(w).Encode(books)\n}\n\nfunc handleGetBook(w http.ResponseWriter, r *http.Request) {\n\tid := r.PathValue(\"id\")\n\tbook, err := store.Get(id)\n\tif err != nil {\n\t\thttp.Error(w, err.Error(), http.StatusNotFound)\n\t\treturn\n\t}\n\tw.Header().Set(\"Content-Type\", \"application/json\")\n\tjson.NewEncoder(w).Encode(book)\n}\n\nfunc handleCreateBook(w http.ResponseWriter, r *http.Request) {\n\tvar input struct {\n\t\tTitle  string `json:\"title\"`\n\t\tAuthor string `json:\"author\"`\n\t\tYear   int    `json:\"year\"`\n\t}\n\t// TODO: validate required fields (title, author)\n\t// TODO: validate year is reasonable (1000-2100)\n\tjson.NewDecoder(r.Body).Decode(&input)\n\n\tbook := store.Create(input.Title, input.Author, input.Year)\n\tw.Header().Set(\"Content-Type\", \"application/json\")\n\tw.WriteHeader(http.StatusCreated)\n\tjson.NewEncoder(w).Encode(book)\n}\n\nfunc handleDeleteBook(w http.ResponseWriter, r *http.Request) {\n\tid := r.PathValue(\"id\")\n\tif err := store.Delete(id); err != nil {\n\t\thttp.Error(w, err.Error(), http.StatusNotFound)\n\t\treturn\n\t}\n\tw.WriteHeader(http.StatusNoContent)\n}\n"))
}

func listCreatedFiles(v *grasp.VirtualOS, agentName string) {
	sh := v.Shell("system")
	ctx := context.Background()

	dir := "/shared/" + agentName
	if agentName == "reporter" {
		dir = "/output"
	}

	result := sh.Execute(ctx, "ls "+dir)
	for _, line := range strings.Split(strings.TrimSpace(result.Output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			fmt.Printf("    %s/%s\n", dir, line)
		}
	}
}

func getModel() anthropic.Model {
	if m := os.Getenv("ANTHROPIC_MODEL"); m != "" {
		return anthropic.Model(m)
	}
	return anthropic.ModelClaudeSonnet4_5_20250929
}

func newClient() anthropic.Client {
	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	authToken := os.Getenv("ANTHROPIC_AUTH_TOKEN")
	if baseURL != "" && authToken != "" {
		return anthropic.NewClient(
			option.WithBaseURL(baseURL),
			option.WithAPIKey(authToken),
		)
	}
	return anthropic.NewClient()
}

func shellToolDef() anthropic.ToolParam {
	return anthropic.ToolParam{
		Name:        "shell",
		Description: anthropic.String("Execute a command in the grasp virtual filesystem. Commands: ls, cat, grep, find, head, tail, stat, write, mkdir, rm, mv, cp, echo, cd, pwd. Supports pipes (|), redirects (>, >>), here-documents (<< 'EOF'), env vars ($VAR)."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: constant.ValueOf[constant.Object](),
			Properties: map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "Shell command to execute",
				},
			},
			Required: []string{"command"},
		},
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n... (truncated)"
}
