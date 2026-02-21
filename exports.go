// Package grasp implements an Agent File System that provides a unified
// filesystem interface where everything — files, tools, services — is accessed
// through mount points.
//
// The key abstraction is Provider: a minimal interface (Stat + List) that every
// data source implements. Additional capabilities (Readable, Writable,
// Executable, Searchable) are detected at runtime via type assertions.
package grasp

import (
	"github.com/jackfish212/grasp/shell"
	"github.com/jackfish212/grasp/types"
)

type (
	Perm              = types.Perm
	Entry             = types.Entry
	File              = types.File
	OpenFlag          = types.OpenFlag
	ListOpts          = types.ListOpts
	SearchOpts        = types.SearchOpts
	SearchResult      = types.SearchResult
	Provider          = types.Provider
	Readable          = types.Readable
	Writable          = types.Writable
	Executable        = types.Executable
	Searchable        = types.Searchable
	MountInfoProvider = types.MountInfoProvider
	Mutable           = types.Mutable
	Touchable         = types.Touchable
	ExecutableFile    = types.ExecutableFile
	WatchEvent        = types.WatchEvent
	EventType         = types.EventType
)

const (
	PermNone  = types.PermNone
	PermRead  = types.PermRead
	PermWrite = types.PermWrite
	PermExec  = types.PermExec
	PermRO    = types.PermRO
	PermRW    = types.PermRW
	PermRX    = types.PermRX
	PermRWX   = types.PermRWX
)

const (
	O_RDONLY = types.O_RDONLY
	O_WRONLY = types.O_WRONLY
	O_RDWR   = types.O_RDWR
	O_CREATE = types.O_CREATE
	O_TRUNC  = types.O_TRUNC
	O_APPEND = types.O_APPEND
)

const (
	EventCreate = types.EventCreate
	EventWrite  = types.EventWrite
	EventRemove = types.EventRemove
	EventRename = types.EventRename
	EventMkdir  = types.EventMkdir
	EventAll    = types.EventAll
)

var (
	NewFile           = types.NewFile
	NewSeekableFile   = types.NewSeekableFile
	NewExecutableFile = types.NewExecutableFile
)

var (
	ErrNotFound        = types.ErrNotFound
	ErrNotExecutable   = types.ErrNotExecutable
	ErrNotReadable     = types.ErrNotReadable
	ErrNotWritable     = types.ErrNotWritable
	ErrIsDir           = types.ErrIsDir
	ErrNotDir          = types.ErrNotDir
	ErrAlreadyMounted  = types.ErrAlreadyMounted
	ErrMountUnderMount = types.ErrMountUnderMount
	ErrNotSupported    = types.ErrNotSupported
	ErrParentNotExist  = types.ErrParentNotExist
)

// Shell types - re-exported for API compatibility
type (
	Shell      = shell.Shell
	ShellEnv   = shell.ShellEnv
	ExecResult = shell.ExecResult
	ExecHook   = shell.ExecHook
)

// Shell constructors and functions
var (
	NewShell = shell.NewShell
)
