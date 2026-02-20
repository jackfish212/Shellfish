package types

import "errors"

var (
	ErrNotFound        = errors.New("afs: not found")
	ErrNotExecutable   = errors.New("afs: not executable")
	ErrNotReadable     = errors.New("afs: permission denied: not readable")
	ErrNotWritable     = errors.New("afs: permission denied: not writable")
	ErrIsDir           = errors.New("afs: is a directory")
	ErrNotDir          = errors.New("afs: not a directory")
	ErrAlreadyMounted  = errors.New("afs: path already mounted")
	ErrMountUnderMount = errors.New("afs: mount under existing mount point")
	ErrNotSupported    = errors.New("afs: operation not supported")
	ErrParentNotExist  = errors.New("afs: parent directory does not exist")
)
