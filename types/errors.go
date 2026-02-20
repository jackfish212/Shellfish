package types

import "errors"

var (
	ErrNotFound        = errors.New("shellfish: not found")
	ErrNotExecutable   = errors.New("shellfish: not executable")
	ErrNotReadable     = errors.New("shellfish: permission denied: not readable")
	ErrNotWritable     = errors.New("shellfish: permission denied: not writable")
	ErrIsDir           = errors.New("shellfish: is a directory")
	ErrNotDir          = errors.New("shellfish: not a directory")
	ErrAlreadyMounted  = errors.New("shellfish: path already mounted")
	ErrMountUnderMount = errors.New("shellfish: mount under existing mount point")
	ErrNotSupported    = errors.New("shellfish: operation not supported")
	ErrParentNotExist  = errors.New("shellfish: parent directory does not exist")
)
