package mounts

import "github.com/agentfs/afs/types"

// FuncFS is deprecated. Use MemFS instead.
type FuncFS = MemFS

// NewFuncFS creates a new function filesystem.
// Deprecated: Use NewMemFS instead.
func NewFuncFS() *FuncFS {
	return NewMemFS(types.PermRW)
}
