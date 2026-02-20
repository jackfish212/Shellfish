package types

// ListOpts controls listing behaviour.
type ListOpts struct {
	Recursive bool
}

// SearchOpts controls search behaviour.
type SearchOpts struct {
	Scope      string // path prefix to limit search
	MaxResults int
}

// SearchResult represents a single search hit.
type SearchResult struct {
	Entry   Entry
	Snippet string  // context around the match
	Score   float64 // relevance score (higher = better)
}

// OpenFlag controls how a file is opened.
type OpenFlag int

const (
	O_RDONLY OpenFlag = 0
	O_WRONLY OpenFlag = 1 << iota
	O_RDWR
	O_CREATE
	O_TRUNC
	O_APPEND
)

func (f OpenFlag) Has(flag OpenFlag) bool { return f&flag == flag }
func (f OpenFlag) IsReadable() bool       { return f == O_RDONLY || f.Has(O_RDWR) }
func (f OpenFlag) IsWritable() bool       { return f.Has(O_WRONLY) || f.Has(O_RDWR) }
