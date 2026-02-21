// Cached Feeds Example — Demonstrating Cache Value
//
// This example showcases the value of dbfs caching httpfs through union mount:
// 1. Cold start: First read (penetrates to httpfs) → shows latency
// 2. Cache hit: Second read (from dbfs) → shows latency comparison
// 3. TTL expiry: Read after TTL → shows cache refresh
// 4. Offline mode: Source unavailable → shows cache still works
//
// Usage:
//
//	go run main.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackfish212/dbfs"
	"github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/builtins"
	"github.com/jackfish212/grasp/mounts"
	"github.com/jackfish212/grasp/types"
	httpfs "github.com/jackfish212/httpfs"

	_ "modernc.org/sqlite"
)

const (
	feedTTL       = 30 * time.Second // Short TTL for demo
	purgeInterval = 1 * time.Minute
)

// Mock feed data - simulates an API response
var feedData = []map[string]string{
	{"id": "1", "name": "welcome", "content": "Welcome to the cached feeds demo! This is the first item."},
	{"id": "2", "name": "about-cache", "content": "Caching dramatically improves performance by storing frequently accessed data locally."},
	{"id": "3", "name": "union-mount", "content": "Union mount combines multiple filesystems, with cache layer on top of origin."},
	{"id": "4", "name": "ttl-expiry", "content": "TTL (Time To Live) ensures cached data doesn't become too stale."},
}

func main() {
	ctx := context.Background()

	// Create temp cache file, delete on exit
	tmpDir, _ := os.MkdirTemp("", "grasp-cache-demo-*")
	cachePath := filepath.Join(tmpDir, "cache.db")
	defer os.RemoveAll(tmpDir)

	// Create mock HTTP server with simulated latency
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		// Simulate network latency (500ms - 1s)
		time.Sleep(time.Duration(500+requestCount*100) * time.Millisecond)

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"v1"`)
		data, _ := json.Marshal(feedData)
		w.Write(data)
	}))
	defer server.Close()

	// Print header
	fmt.Println()
	fmt.Println(strings.Repeat("=", 65))
	fmt.Println("  Cached Feeds — Demonstrating Union Mount Cache Value")
	fmt.Println(strings.Repeat("=", 65))
	fmt.Printf("  Mock API: %s\n", server.URL)
	fmt.Printf("  Cache DB: %s\n", cachePath)
	fmt.Printf("  TTL: %s (short for demo purposes)\n", feedTTL)
	fmt.Println(strings.Repeat("=", 65))

	// Cache layer: dbfs (SQLite)
	cache, err := dbfs.Open("sqlite", cachePath, types.PermRW)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open cache: %v\n", err)
		os.Exit(1)
	}
	defer cache.Close()

	// Origin: httpfs with JSON parser
	origin := httpfs.NewHTTPFS(
		httpfs.WithHTTPFSInterval(5*time.Minute),
		httpfs.WithHTTPFSOnEvent(func(ev types.EventType, path string) {
			_ = cache.Remove(ctx, path)
		}),
	)
	if err := origin.Add("api", server.URL, &httpfs.JSONParser{}); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to add source: %v\n", err)
		os.Exit(1)
	}

	// Start httpfs - this blocks until initial fetch completes
	origin.Start(ctx)

	// Union: cache on top, origin below, TTL
	union := mounts.NewCachedUnion(cache, origin, feedTTL)

	// VirtualOS + builtins
	v := grasp.New()
	rootFS, err := grasp.Configure(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configure: %v\n", err)
		os.Exit(1)
	}
	builtins.RegisterBuiltinsOnFS(v, rootFS)

	v.Mount("/feeds", union)

	shell := v.Shell("user")

	// Verify data is available
	fmt.Print("\n  Initializing httpfs...")
	result := shell.Execute(ctx, "ls /feeds/api")
	if result.Code != 0 || strings.TrimSpace(result.Output) == "" {
		fmt.Fprintf(os.Stderr, "\n  Failed to initialize: %s\n", result.Output)
		origin.Stop()
		os.Exit(1)
	}
	fmt.Println(" done!")

	// Get first file
	lines := strings.Split(strings.TrimSpace(result.Output), "\n")
	var firstFile string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasSuffix(line, "/") {
			continue
		}
		firstFile = strings.Fields(line)[0]
		break
	}

	if firstFile == "" {
		fmt.Fprintln(os.Stderr, "No files found")
		origin.Stop()
		os.Exit(1)
	}

	filePath := "/feeds/api/" + firstFile

	// ========================================
	// Scenario 1: Cold Start
	// ========================================
	fmt.Println()
	fmt.Println(strings.Repeat("-", 65))
	fmt.Println("  [Scenario 1] Cold Start — First read from origin")
	fmt.Println(strings.Repeat("-", 65))
	fmt.Printf("  Reading: %s\n", filePath)

	start := time.Now()
	r1 := shell.Execute(ctx, "cat "+filePath)
	coldLatency := time.Since(start)

	if r1.Code != 0 {
		fmt.Printf("  Error: %s\n", r1.Output)
		origin.Stop()
		os.Exit(1)
	}
	fmt.Printf("  Latency: %s (from httpfs, network request)\n", coldLatency)
	fmt.Printf("  Content: %s\n", truncate(r1.Output, 50))
	fmt.Println("  → Data backfilled to dbfs cache")

	// ========================================
	// Scenario 2: Cache Hit
	// ========================================
	fmt.Println()
	fmt.Println(strings.Repeat("-", 65))
	fmt.Println("  [Scenario 2] Cache Hit — Second read from cache")
	fmt.Println(strings.Repeat("-", 65))
	fmt.Printf("  Reading: %s\n", filePath)

	start = time.Now()
	r2 := shell.Execute(ctx, "cat "+filePath)
	cacheLatency := time.Since(start)

	if r2.Code != 0 {
		fmt.Printf("  Error: %s\n", r2.Output)
	} else {
		fmt.Printf("  Latency: %s (from dbfs cache, local read)\n", cacheLatency)
		speedup := float64(coldLatency) / float64(cacheLatency)
		if speedup > 1 {
			fmt.Printf("  Speed improvement: %.0fx faster!\n", speedup)
		}
	}

	// ========================================
	// Scenario 3: Read another file (also cached)
	// ========================================
	fmt.Println()
	fmt.Println(strings.Repeat("-", 65))
	fmt.Println("  [Scenario 3] List all files")
	fmt.Println(strings.Repeat("-", 65))
	rList := shell.Execute(ctx, "ls /feeds/api")
	fmt.Print(rList.Output)

	// ========================================
	// Scenario 4: TTL Expiry
	// ========================================
	fmt.Println()
	fmt.Println(strings.Repeat("-", 65))
	fmt.Println("  [Scenario 4] TTL Expiry — Cache refresh after TTL")
	fmt.Println(strings.Repeat("-", 65))
	fmt.Printf("  Waiting for TTL to expire (%s)...\n", feedTTL)

	// Countdown
	remaining := feedTTL
	for remaining > 0 {
		if remaining >= 10*time.Second {
			fmt.Printf("  %v remaining...\r", remaining.Round(time.Second))
			time.Sleep(5 * time.Second)
			remaining -= 5 * time.Second
		} else {
			fmt.Printf("  %v remaining...\r", remaining.Round(time.Second))
			time.Sleep(time.Second)
			remaining -= time.Second
		}
	}
	fmt.Println("  TTL expired!                                        ")

	start = time.Now()
	r3 := shell.Execute(ctx, "cat "+filePath)
	expiredLatency := time.Since(start)

	if r3.Code != 0 {
		fmt.Printf("  Error: %s\n", r3.Output)
	} else {
		fmt.Printf("  Latency: %s (cache expired, re-fetched from origin)\n", expiredLatency)
		fmt.Println("  → Cache refreshed with latest data")
	}

	// ========================================
	// Scenario 5: Offline Mode
	// ========================================
	fmt.Println()
	fmt.Println(strings.Repeat("-", 65))
	fmt.Println("  [Scenario 5] Offline — Source unavailable, cache works")
	fmt.Println(strings.Repeat("-", 65))
	fmt.Println("  Closing mock server (simulating network offline)...")

	// Close server to simulate offline
	server.Close()
	time.Sleep(100 * time.Millisecond)

	start = time.Now()
	r4 := shell.Execute(ctx, "cat "+filePath)
	offlineLatency := time.Since(start)

	if r4.Code != 0 {
		fmt.Printf("  Error: %s\n", r4.Output)
	} else {
		fmt.Printf("  Latency: %s (from cache, origin is down)\n", offlineLatency)
		fmt.Println("  ✓ Cache allows data access even when source is unavailable!")
	}

	// Stop httpfs
	origin.Stop()

	// ========================================
	// Summary
	// ========================================
	fmt.Println()
	fmt.Println(strings.Repeat("=", 65))
	fmt.Println("  Summary — Cache Value Demonstrated")
	fmt.Println(strings.Repeat("=", 65))
	fmt.Println()
	fmt.Println("  Latency Comparison:")
	fmt.Printf("    Cold start:   %s (network request)\n", coldLatency)
	fmt.Printf("    Cache hit:    %s (local read)\n", cacheLatency)
	fmt.Printf("    TTL expired:  %s (re-fetch)\n", expiredLatency)
	fmt.Printf("    Offline:      %s (cache only)\n", offlineLatency)
	fmt.Println()

	speedup := float64(coldLatency) / float64(cacheLatency)
	fmt.Printf("  Cache speedup: %.0fx faster\n\n", speedup)

	fmt.Println("  Key Benefits:")
	fmt.Println("    1. Speed — Cache reads are orders of magnitude faster")
	fmt.Println("    2. Availability — Data accessible even when source is down")
	fmt.Println("    3. Freshness — TTL ensures data doesn't get too stale")
	fmt.Println("    4. Efficiency — Reduces repeated network requests")
	fmt.Println()
	fmt.Println(strings.Repeat("=", 65))
}

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// readAll reads all content from an io.Reader
func readAll(r io.Reader) string {
	data, _ := io.ReadAll(r)
	return string(data)
}
