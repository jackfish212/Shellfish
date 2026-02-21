package main

import (
	"context"
	"fmt"
	"log"

	"github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/builtins"
	"github.com/jackfish212/grasp/mounts"
)

func main() {
	// Create a new virtual OS
	v := grasp.New()

	// Mount root filesystem
	root := mounts.NewMemFS(grasp.PermRW)
	if err := v.Mount("/", root); err != nil {
		log.Fatal(err)
	}

	// Create directory structure
	root.AddDir("bin")
	root.AddDir("usr")
	root.AddDir("usr/bin")
	root.AddDir("mnt")
	root.AddDir("home")
	root.AddDir("home/user")

	// Register builtins
	if err := builtins.RegisterBuiltinsOnFS(v, root); err != nil {
		log.Fatal(err)
	}

	// Create a shell with PATH set
	sh := v.Shell("user")
	sh.Env.Set("PATH", "/usr/bin:/bin")
	ctx := context.Background()

	// Example 1: List current mounts
	fmt.Println("=== Current Mounts ===")
	result := sh.Execute(ctx, "mount")
	fmt.Print(result.Output)

	// Example 2: Mount a new in-memory filesystem
	fmt.Println("\n=== Mounting memfs at /mnt/memory ===")
	result = sh.Execute(ctx, "mount -t memfs - /mnt/memory")
	fmt.Print(result.Output)

	// Example 3: Create files in the new mount
	fmt.Println("\n=== Creating files in /mnt/memory ===")
	result = sh.Execute(ctx, "write /mnt/memory/test.txt Hello from mounted filesystem")
	fmt.Print(result.Output)

	// Example 4: Read the file
	fmt.Println("\n=== Reading file from /mnt/memory ===")
	result = sh.Execute(ctx, "cat /mnt/memory/test.txt")
	fmt.Print(result.Output)

	// Example 5: Mount a SQLite database (if you have one)
	// Uncomment to test with a real SQLite database
	/*
		fmt.Println("\n=== Mounting SQLite database ===")
		result = sh.Execute(ctx, "mount -t sqlitefs /path/to/database.db /mnt/db")
		fmt.Print(result.Output)
	*/

	// Example 6: List all mounts again
	fmt.Println("\n=== All Mounts After Changes ===")
	result = sh.Execute(ctx, "mount")
	fmt.Print(result.Output)

	// Example 7: Mount with read-only permission
	fmt.Println("\n=== Mounting read-only memfs at /mnt/readonly ===")
	result = sh.Execute(ctx, "mount -t memfs -o ro - /mnt/readonly")
	fmt.Print(result.Output)

	// Example 8: Try to write to read-only mount (should fail)
	fmt.Println("\n=== Attempting to write to read-only mount ===")
	result = sh.Execute(ctx, "write /mnt/readonly/test.txt This should fail")
	if result.Code != 0 {
		fmt.Printf("Expected failure: %s", result.Output)
	}

	// Example 9: Show help
	fmt.Println("\n=== Mount Command Help ===")
	result = sh.Execute(ctx, "mount -h")
	fmt.Print(result.Output)
}
