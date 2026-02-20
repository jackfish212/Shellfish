package shell

import (
	"context"
	"io"

	"github.com/jackfish212/shellfish/types"
)

// VirtualOS is the interface that shell needs from VirtualOS.
// This allows the shell package to operate on VirtualOS without import cycles.
type VirtualOS interface {
	Stat(ctx context.Context, path string) (*types.Entry, error)
	List(ctx context.Context, path string, opts types.ListOpts) ([]types.Entry, error)
	Open(ctx context.Context, path string) (types.File, error)
	Write(ctx context.Context, path string, reader io.Reader) error
	Exec(ctx context.Context, path string, args []string, stdin io.Reader) (io.ReadCloser, error)
}

// EnvKey is the context key for environment variables.
type envKey struct{}

// WithEnv returns a context carrying the given environment variables.
func WithEnv(ctx context.Context, env map[string]string) context.Context {
	return context.WithValue(ctx, envKey{}, env)
}

// Env reads a single environment variable from the context.
func Env(ctx context.Context, key string) string {
	if env, ok := ctx.Value(envKey{}).(map[string]string); ok {
		return env[key]
	}
	return ""
}
