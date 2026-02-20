# Create a Custom Provider

This guide shows how to implement a custom Shellfish provider — a mountable data source that integrates into the virtual filesystem.

## Decide What to Implement

Shellfish providers are built from composable interfaces. Start with the base `Provider`, then add capabilities as needed:

| Interface | Methods | When to implement |
|-----------|---------|-------------------|
| `Provider` | `Stat`, `List` | Always (required) |
| `Readable` | `Open` | Data source has readable content |
| `Writable` | `Write` | Data source accepts writes |
| `Executable` | `Exec` | Data source exposes callable operations |
| `Searchable` | `Search` | Data source supports query-based retrieval |
| `Mutable` | `Mkdir`, `Remove`, `Rename` | Data source supports structural changes |

## Example: A Weather Provider

Let's build a read-only provider that exposes weather data as files.

### Step 1: Define the struct

```go
package weather

import (
    "context"
    "fmt"
    "io"
    "strings"
    "time"

    "github.com/agentfs/afs"
)

type WeatherProvider struct {
    apiKey string
    cities []string
}

func New(apiKey string, cities []string) *WeatherProvider {
    return &WeatherProvider{apiKey: apiKey, cities: cities}
}
```

### Step 2: Implement Provider (Stat + List)

```go
func (wp *WeatherProvider) Stat(ctx context.Context, path string) (*afs.Entry, error) {
    path = strings.TrimPrefix(path, "/")

    if path == "" {
        return &afs.Entry{Name: "weather", IsDir: true, Perm: afs.PermRO}, nil
    }

    city := strings.TrimSuffix(path, ".md")
    for _, c := range wp.cities {
        if c == city {
            return &afs.Entry{
                Name:     city + ".md",
                Perm:     afs.PermRO,
                MimeType: "text/markdown",
                Modified: time.Now(),
            }, nil
        }
    }

    return nil, fmt.Errorf("not found: %s", path)
}

func (wp *WeatherProvider) List(ctx context.Context, path string, opts afs.ListOpts) ([]afs.Entry, error) {
    path = strings.TrimPrefix(path, "/")
    if path != "" {
        return nil, fmt.Errorf("not a directory: %s", path)
    }

    entries := make([]afs.Entry, len(wp.cities))
    for i, city := range wp.cities {
        entries[i] = afs.Entry{
            Name:     city + ".md",
            Perm:     afs.PermRO,
            MimeType: "text/markdown",
        }
    }
    return entries, nil
}
```

### Step 3: Implement Readable (Open)

```go
func (wp *WeatherProvider) Open(ctx context.Context, path string) (afs.File, error) {
    path = strings.TrimPrefix(path, "/")
    city := strings.TrimSuffix(path, ".md")

    data, err := wp.fetchWeather(city)
    if err != nil {
        return nil, err
    }

    entry := &afs.Entry{Name: city + ".md", Perm: afs.PermRO}
    reader := io.NopCloser(strings.NewReader(data))
    return afs.NewFile(city+".md", entry, reader), nil
}

func (wp *WeatherProvider) fetchWeather(city string) (string, error) {
    // Call weather API, return formatted markdown
    return fmt.Sprintf("# Weather: %s\n\nTemperature: 22°C\nCondition: Sunny\n", city), nil
}
```

### Step 4: Mount and use

```go
v := afs.New()
rootFS, _ := afs.Configure(v)
builtins.RegisterBuiltinsOnFS(v, rootFS)

wp := weather.New("api-key", []string{"tokyo", "london", "beijing"})
v.Mount("/weather", wp)

sh := v.Shell("agent")
sh.Execute(ctx, "ls /weather")
// tokyo.md  london.md  beijing.md

sh.Execute(ctx, "cat /weather/tokyo.md")
// # Weather: tokyo
// Temperature: 22°C
// Condition: Sunny
```

## Adding Search

If your data source supports queries, implement `Searchable`:

```go
func (wp *WeatherProvider) Search(ctx context.Context, query string, opts afs.SearchOpts) ([]afs.SearchResult, error) {
    var results []afs.SearchResult
    for _, city := range wp.cities {
        if strings.Contains(strings.ToLower(city), strings.ToLower(query)) {
            results = append(results, afs.SearchResult{
                Entry: afs.Entry{Name: city + ".md", Path: city + ".md"},
                Score: 1.0,
            })
        }
    }
    return results, nil
}
```

Now `search tokyo --scope /weather` finds the city.

## Adding MountInfo

Implement `MountInfoProvider` so the `mount` command shows useful descriptions:

```go
func (wp *WeatherProvider) MountInfo() (name, extra string) {
    return "weather-api", fmt.Sprintf("%d cities", len(wp.cities))
}
```

```bash
$ mount
/weather   weather-api   (ro, search) 3 cities
```

## Guidelines

- **Return clean paths.** `Stat` and `List` receive paths relative to the mount point, with the leading `/` stripped. `""` means the mount root.
- **Set `Perm` correctly.** Shellfish checks permissions before delegating to your provider. If an entry has `PermRO`, write attempts are rejected before reaching your code.
- **Use `Meta` for provider-specific data.** The `Entry.Meta` map carries arbitrary key-value pairs. Use it for schema info, scores, tiers, or any metadata the agent might need.
- **Be concurrent-safe.** Multiple shell instances may access your provider simultaneously. Use appropriate synchronization if your provider has mutable state.
