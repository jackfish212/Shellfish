// Basic dbfs Example
//
// This example demonstrates:
// 1. Creating a database-backed filesystem with dbfs
// 2. Using the filesystem with grasp VirtualOS
// 3. Basic file operations: write, read, list, delete
//
// The example uses SQLite as the database backend.
// All files are stored in a single "files" table.
//
// Usage:
//
//	go run main.go
//
// The program will create a temporary SQLite database and demonstrate
// various filesystem operations through the VirtualOS shell.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackfish212/grasp/dbfs"
	"github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/builtins"
	"github.com/jackfish212/grasp/types"

	_ "modernc.org/sqlite"
)

func main() {
	ctx := context.Background()

	// Create a temporary database file
	tmpDir := os.TempDir()
	dbPath := filepath.Join(tmpDir, "dbfs-example.db")
	defer os.Remove(dbPath) // Clean up after we're done

	fmt.Println("dbfs Basic Example")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Database: %s\n\n", dbPath)

	// Open the dbfs filesystem
	fs, err := dbfs.Open("sqlite", dbPath, types.PermRW)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open dbfs: %v\n", err)
		os.Exit(1)
	}
	defer fs.Close()

	// Setup VirtualOS
	v := grasp.New()
	rootFS, err := grasp.Configure(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to configure VirtualOS: %v\n", err)
		os.Exit(1)
	}
	builtins.RegisterBuiltinsOnFS(v, rootFS)

	// Mount dbfs at /db
	v.Mount("/db", fs)

	// Create a shell for interaction
	shell := v.Shell("user")

	// Demonstrate filesystem operations
	fmt.Println(strings.Repeat("-", 60))
	fmt.Println("1. Writing files to /db")
	fmt.Println(strings.Repeat("-", 60))

	// Write some files directly through dbfs
	if err := fs.Write(ctx, "hello.txt", strings.NewReader("Hello, dbfs!")); err != nil {
		fmt.Fprintf(os.Stderr, "Write error: %v\n", err)
	}
	fmt.Println("Created: hello.txt")

	if err := fs.Write(ctx, "notes/todo.txt", strings.NewReader("- Learn dbfs\n- Build something cool")); err != nil {
		fmt.Fprintf(os.Stderr, "Write error: %v\n", err)
	}
	fmt.Println("Created: notes/todo.txt")

	if err := fs.Write(ctx, "notes/ideas.txt", strings.NewReader("Database-backed filesystem ideas...")); err != nil {
		fmt.Fprintf(os.Stderr, "Write error: %v\n", err)
	}
	fmt.Println("Created: notes/ideas.txt")

	if err := fs.Write(ctx, "config.json", strings.NewReader(`{"app": "dbfs", "version": "1.0.0"}`)); err != nil {
		fmt.Fprintf(os.Stderr, "Write error: %v\n", err)
	}
	fmt.Println("Created: config.json")

	// Create a directory
	if err := fs.Mkdir(ctx, "archive", types.PermRW); err != nil {
		fmt.Fprintf(os.Stderr, "Mkdir error: %v\n", err)
	}
	fmt.Println("Created directory: archive/")

	fmt.Println("\n" + strings.Repeat("-", 60))
	fmt.Println("2. Listing files in /db")
	fmt.Println(strings.Repeat("-", 60) + "\n")

	result := shell.Execute(ctx, "ls -la /db")
	fmt.Print(result.Output)

	fmt.Println("\n" + strings.Repeat("-", 60))
	fmt.Println("3. Listing notes directory")
	fmt.Println(strings.Repeat("-", 60) + "\n")

	result = shell.Execute(ctx, "ls -la /db/notes")
	fmt.Print(result.Output)

	fmt.Println("\n" + strings.Repeat("-", 60))
	fmt.Println("4. Reading file contents")
	fmt.Println(strings.Repeat("-", 60) + "\n")

	result = shell.Execute(ctx, "cat /db/hello.txt")
	fmt.Println("Contents of /db/hello.txt:")
	fmt.Print(result.Output)

	result = shell.Execute(ctx, "cat /db/notes/todo.txt")
	fmt.Println("\nContents of /db/notes/todo.txt:")
	fmt.Print(result.Output)

	result = shell.Execute(ctx, "cat /db/config.json")
	fmt.Println("\nContents of /db/config.json:")
	fmt.Print(result.Output)

	fmt.Println("\n" + strings.Repeat("-", 60))
	fmt.Println("5. Using extended API - WriteFile with metadata")
	fmt.Println(strings.Repeat("-", 60) + "\n")

	// Write a file with custom metadata
	meta := map[string]string{
		"author":  "example",
		"purpose": "demonstration",
	}
	if err := fs.WriteFile(ctx, "document.md", []byte("# Welcome to dbfs\n\nThis is a markdown file with metadata."), meta); err != nil {
		fmt.Fprintf(os.Stderr, "WriteFile error: %v\n", err)
	}
	fmt.Println("Created: document.md (with metadata)")

	// Read the file to see its metadata
	entry, err := fs.Stat(ctx, "document.md")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Stat error: %v\n", err)
	} else {
		fmt.Printf("Metadata for document.md: %v\n", entry.Meta)
	}

	fmt.Println("\n" + strings.Repeat("-", 60))
	fmt.Println("6. Rename operation")
	fmt.Println(strings.Repeat("-", 60) + "\n")

	if err := fs.Rename(ctx, "hello.txt", "greeting.txt"); err != nil {
		fmt.Fprintf(os.Stderr, "Rename error: %v\n", err)
	}
	fmt.Println("Renamed: hello.txt -> greeting.txt")

	result = shell.Execute(ctx, "ls /db")
	fmt.Println("\nCurrent files in /db:")
	fmt.Print(result.Output)

	fmt.Println("\n" + strings.Repeat("-", 60))
	fmt.Println("7. Delete operation")
	fmt.Println(strings.Repeat("-", 60) + "\n")

	if err := fs.Remove(ctx, "config.json"); err != nil {
		fmt.Fprintf(os.Stderr, "Remove error: %v\n", err)
	}
	fmt.Println("Deleted: config.json")

	result = shell.Execute(ctx, "ls /db")
	fmt.Println("\nCurrent files in /db:")
	fmt.Print(result.Output)

	fmt.Println("\n" + strings.Repeat("-", 60))
	fmt.Println("8. Statistics")
	fmt.Println(strings.Repeat("-", 60) + "\n")

	count, err := fs.Count(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Count error: %v\n", err)
	} else {
		fmt.Printf("Total files: %d\n", count)
	}

	totalSize, err := fs.TotalSize(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "TotalSize error: %v\n", err)
	} else {
		fmt.Printf("Total size: %d bytes\n", totalSize)
	}

	fmt.Println("\n" + strings.Repeat("-", 60))
	fmt.Println("9. Purge old files demonstration")
	fmt.Println(strings.Repeat("-", 60) + "\n")

	// Create a file, wait a moment, then purge files older than a negative duration (none will match)
	if err := fs.Write(ctx, "temp.txt", strings.NewReader("temporary content")); err != nil {
		fmt.Fprintf(os.Stderr, "Write error: %v\n", err)
	}
	fmt.Println("Created: temp.txt")

	// Purge files older than 1 hour (should not delete anything just created)
	purged, err := fs.Purge(ctx, time.Hour)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Purge error: %v\n", err)
	} else {
		fmt.Printf("Purged %d files older than 1 hour\n", purged)
	}

	// Final listing
	result = shell.Execute(ctx, "ls -la /db")
	fmt.Println("\nFinal state of /db:")
	fmt.Print(result.Output)

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Example complete!")
	fmt.Println("The database-backed filesystem was successfully demonstrated.")
	fmt.Println(strings.Repeat("=", 60))
}
