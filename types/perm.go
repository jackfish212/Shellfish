package types

// Perm represents simplified Unix-style permission bits (r/w/x).
type Perm uint8

const (
	PermRead  Perm = 1 << iota // r
	PermWrite                  // w
	PermExec                   // x
)

const (
	PermNone Perm = 0
	PermRO        = PermRead
	PermRW        = PermRead | PermWrite
	PermRX        = PermRead | PermExec
	PermRWX       = PermRead | PermWrite | PermExec
)

func (p Perm) CanRead() bool  { return p&PermRead != 0 }
func (p Perm) CanWrite() bool { return p&PermWrite != 0 }
func (p Perm) CanExec() bool  { return p&PermExec != 0 }

func (p Perm) String() string {
	s := [3]byte{'-', '-', '-'}
	if p.CanRead() {
		s[0] = 'r'
	}
	if p.CanWrite() {
		s[1] = 'w'
	}
	if p.CanExec() {
		s[2] = 'x'
	}
	return string(s[:])
}
