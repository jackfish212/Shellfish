package shellfish

import (
	"context"

	"github.com/jackfish212/shellfish/shell"
)

type envKey struct{}

// WithEnv returns a context carrying the given environment variables.
func WithEnv(ctx context.Context, env map[string]string) context.Context {
	return shell.WithEnv(ctx, env)
}

// Env reads a single environment variable from the context.
func Env(ctx context.Context, key string) string {
	return shell.Env(ctx, key)
}
