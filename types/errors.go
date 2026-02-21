package types

import "errors"

var (
	ErrNotFound        = errors.New("grasp: not found")
	ErrNotExecutable   = errors.New("grasp: not executable")
	ErrNotReadable     = errors.New("grasp: permission denied: not readable")
	ErrNotWritable     = errors.New("grasp: permission denied: not writable")
	ErrIsDir           = errors.New("grasp: is a directory")
	ErrNotDir          = errors.New("grasp: not a directory")
	ErrAlreadyMounted  = errors.New("grasp: path already mounted")
	ErrMountUnderMount = errors.New("grasp: mount under existing mount point")
	ErrNotSupported    = errors.New("grasp: operation not supported")
	ErrParentNotExist  = errors.New("grasp: parent directory does not exist")
)
