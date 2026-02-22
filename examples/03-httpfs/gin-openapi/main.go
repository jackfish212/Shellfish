// Gin OpenAPI Server Example
//
// This example demonstrates:
// 1. Creating a simple REST API with Gin framework
// 2. Serving OpenAPI specification
// 3. Using httpfs to mount the API as a virtual filesystem
//
// The API provides a simple todo list with CRUD operations.
// httpfs automatically discovers all GET endpoints from the OpenAPI spec.
//
// Usage:
//
//	# Terminal 1: Start the API server
//	go run main.go
//
//	# Terminal 2: Mount with httpfs (in another program or shell)
//	# The httpfs can load the OpenAPI spec from http://localhost:8080/openapi.json
//
// Environment variables:
//
//	PORT - Server port (default: 8080)
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/builtins"
	httpfs "github.com/jackfish212/grasp/httpfs"
)

// Todo represents a todo item
type Todo struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Completed bool      `json:"completed"`
	CreatedAt time.Time `json:"createdAt"`
}

// OpenAPI specification as JSON
const openAPISpec = `{
  "openapi": "3.0.0",
  "info": {
    "title": "Todo API",
    "version": "1.0.0",
    "description": "A simple todo list API for httpfs demo"
  },
  "servers": [
    { "url": "http://localhost:8080" }
  ],
  "paths": {
    "/api/todos": {
      "get": {
        "operationId": "listTodos",
        "summary": "List all todos",
        "responses": {
          "200": {
            "description": "List of todos",
            "content": {
              "application/json": {
                "schema": {
                  "type": "array",
                  "items": {
                    "$ref": "#/components/schemas/Todo"
                  }
                }
              }
            }
          }
        }
      }
    },
    "/api/stats": {
      "get": {
        "operationId": "getStats",
        "summary": "Get todo statistics",
        "responses": {
          "200": {
            "description": "Statistics",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "total": { "type": "integer" },
                    "completed": { "type": "integer" },
                    "pending": { "type": "integer" }
                  }
                }
              }
            }
          }
        }
      }
    },
    "/health": {
      "get": {
        "operationId": "healthCheck",
        "summary": "Health check endpoint",
        "responses": {
          "200": {
            "description": "Healthy",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "status": { "type": "string" }
                  }
                }
              }
            }
          }
        }
      }
    }
  },
  "components": {
    "schemas": {
      "Todo": {
        "type": "object",
        "properties": {
          "id": { "type": "string" },
          "title": { "type": "string" },
          "completed": { "type": "boolean" },
          "createdAt": { "type": "string", "format": "date-time" }
        }
      }
    }
  }
}`

var todos = []Todo{
	{ID: "1", Title: "Learn httpfs", Completed: true, CreatedAt: time.Now().Add(-24 * time.Hour)},
	{ID: "2", Title: "Build something cool", Completed: false, CreatedAt: time.Now().Add(-12 * time.Hour)},
	{ID: "3", Title: "Share with others", Completed: false, CreatedAt: time.Now()},
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Update OpenAPI spec with actual port
	spec := strings.Replace(openAPISpec, "http://localhost:8080", "http://localhost:"+port, 1)

	// Setup Gin router
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// OpenAPI spec endpoint
	r.GET("/openapi.json", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json", []byte(spec))
	})

	// API routes
	api := r.Group("/api")
	{
		api.GET("/todos", listTodos)
		api.GET("/stats", getStats)
	}

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Start server in background
	go func() {
		fmt.Printf("Starting API server on :%s\n", port)
		fmt.Println("OpenAPI spec available at: http://localhost:" + port + "/openapi.json")
		if err := r.Run(":" + port); err != nil {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	}()

	// Wait for server to start
	time.Sleep(500 * time.Millisecond)

	// Now mount the API using httpfs
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Mounting API with httpfs...")
	fmt.Println(strings.Repeat("=", 60) + "\n")

	mountAPIWithHTTPFS()
}

func listTodos(c *gin.Context) {
	c.JSON(http.StatusOK, todos)
}

func getStats(c *gin.Context) {
	completed := 0
	for _, t := range todos {
		if t.Completed {
			completed++
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"total":     len(todos),
		"completed": completed,
		"pending":   len(todos) - completed,
	})
}

func mountAPIWithHTTPFS() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	ctx := context.Background()

	// Setup VirtualOS
	v := grasp.New()
	rootFS, err := grasp.Configure(v)
	if err != nil {
		panic(err)
	}
	builtins.RegisterBuiltinsOnFS(v, rootFS)

	// Create HTTPFS and load OpenAPI spec
	fs := httpfs.NewHTTPFS(
		httpfs.WithHTTPFSInterval(10*time.Second), // Poll every 10 seconds for demo
	)

	// Load OpenAPI spec from the running server
	specURL := fmt.Sprintf("http://localhost:%s/openapi.json", port)
	if err := fs.LoadOpenAPIFromURL(ctx, specURL); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load OpenAPI spec: %v\n", err)
		os.Exit(1)
	}

	// Mount httpfs
	v.Mount("/api", fs)

	// Start polling
	fs.Start(ctx)
	defer fs.Stop()

	// Create shell for interaction
	shell := v.Shell("user")

	// List what was mounted
	fmt.Println("The following sources are now available:")
	fmt.Println()

	result := shell.Execute(ctx, "ls /api")
	fmt.Print(result.Output)

	// Wait for initial fetch
	time.Sleep(1 * time.Second)

	fmt.Println("\nExample - reading todo list:")
	fmt.Println()

	result = shell.Execute(ctx, "ls /api/api-todos")
	fmt.Print(result.Output)
	// Get the first file
	if files := strings.Fields(result.Output); len(files) > 0 {
		firstFile := strings.TrimSuffix(files[0], "/")
		result = shell.Execute(ctx, "cat /api/api-todos/"+firstFile)
		fmt.Print("\nContent of " + firstFile + ":\n")
		fmt.Print(result.Output)
	}

	fmt.Println("\n\nExample - reading stats:")
	fmt.Println()

	result = shell.Execute(ctx, "ls /api/api-stats")
	fmt.Print(result.Output)
	if files := strings.Fields(result.Output); len(files) > 0 {
		firstFile := strings.TrimSuffix(files[0], "/")
		result = shell.Execute(ctx, "cat /api/api-stats/"+firstFile)
		fmt.Print("\nContent of " + firstFile + ":\n")
		fmt.Print(result.Output)
	}

	fmt.Println("\n\nExample - health check:")
	fmt.Println()

	result = shell.Execute(ctx, "ls /api/health")
	fmt.Print(result.Output)
	if files := strings.Fields(result.Output); len(files) > 0 {
		firstFile := strings.TrimSuffix(files[0], "/")
		result = shell.Execute(ctx, "cat /api/health/"+firstFile)
		fmt.Print("\nContent of " + firstFile + ":\n")
		fmt.Print(result.Output)
	}

	fmt.Println("\n\nThe API is now mounted as a filesystem at /api")
	fmt.Println("Each endpoint is a directory containing parsed response files.")
	fmt.Println("Endpoints are polled every 10 seconds for updates.")
}
