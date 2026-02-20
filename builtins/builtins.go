package builtins

import (
	shellfish "github.com/jackfish212/shellfish"
	"github.com/jackfish212/shellfish/mounts"
)

// RegisterBuiltins mounts standard shellfish utilities at the given path (e.g. "/bin").
func RegisterBuiltins(v *shellfish.VirtualOS, mountPath string) error {
	fs := mounts.NewMemFS(shellfish.PermRW)
	registerAllBuiltins(v, fs, "")
	return v.Mount(mountPath, fs)
}

// RegisterBuiltinsOnFS registers standard shellfish utilities on the given MemFS at /usr/bin.
func RegisterBuiltinsOnFS(v *shellfish.VirtualOS, fs *mounts.MemFS) error {
	registerAllBuiltins(v, fs, "usr/bin/")
	return nil
}

func registerAllBuiltins(v *shellfish.VirtualOS, fs *mounts.MemFS, prefix string) {
	fs.AddExecFunc(prefix+"ls", builtinLs(v), mounts.FuncMeta{
		Description: "List directory entries",
		Usage:       "ls [path]",
	})
	fs.AddExecFunc(prefix+"read", builtinRead(v), mounts.FuncMeta{
		Description: "Read file content",
		Usage:       "read <path>",
	})
	fs.AddExecFunc(prefix+"cat", builtinRead(v), mounts.FuncMeta{
		Description: "Read file content",
		Usage:       "cat <path>",
	})
	fs.AddExecFunc(prefix+"write", builtinWrite(v), mounts.FuncMeta{
		Description: "Write content to file",
		Usage:       "write <path> [content]",
	})
	fs.AddExecFunc(prefix+"stat", builtinStat(v), mounts.FuncMeta{
		Description: "Show entry metadata",
		Usage:       "stat <path>",
	})
	fs.AddExecFunc(prefix+"search", builtinSearch(v), mounts.FuncMeta{
		Description: "Cross-mount search",
		Usage:       "search <query> [--scope <path>] [--max N]",
	})
	fs.AddExecFunc(prefix+"grep", builtinSearch(v), mounts.FuncMeta{
		Description: "Cross-mount search",
		Usage:       "grep <query> [--scope <path>] [--max N]",
	})
	fs.AddExecFunc(prefix+"mount", builtinMount(v), mounts.FuncMeta{
		Description: "List mount points",
		Usage:       "mount",
	})
	fs.AddExecFunc(prefix+"which", builtinWhich(v), mounts.FuncMeta{
		Description: "Show full path of command",
		Usage:       "which <command>...",
	})
	fs.AddExecFunc(prefix+"find", builtinFind(v), mounts.FuncMeta{
		Description: "Search for files in a directory hierarchy",
		Usage:       "find [path] [-name PATTERN] [-type f|d] [-maxdepth N]",
	})
	fs.AddExecFunc(prefix+"head", builtinHead(v), mounts.FuncMeta{
		Description: "Output the first part of files",
		Usage:       "head [-n LINES | -c BYTES] [FILE]...",
	})
	fs.AddExecFunc(prefix+"tail", builtinTail(v), mounts.FuncMeta{
		Description: "Output the last part of files",
		Usage:       "tail [-n LINES | -c BYTES] [FILE]...",
	})
	fs.AddExecFunc(prefix+"mkdir", builtinMkdir(v), mounts.FuncMeta{
		Description: "Create directories",
		Usage:       "mkdir [-p] <path>...",
	})
	fs.AddExecFunc(prefix+"rm", builtinRm(v), mounts.FuncMeta{
		Description: "Remove files or directories",
		Usage:       "rm [-r|-rf] <path>...",
	})
	fs.AddExecFunc(prefix+"mv", builtinMv(v), mounts.FuncMeta{
		Description: "Move (rename) files",
		Usage:       "mv <source> <dest>",
	})
	fs.AddExecFunc(prefix+"cp", builtinCp(v), mounts.FuncMeta{
		Description: "Copy files",
		Usage:       "cp [-r] <source> <dest>",
	})
	fs.AddExecFunc(prefix+"uname", builtinUname(), mounts.FuncMeta{
		Description: "Print system information",
		Usage:       "uname [-a|-s|-n|-r|-v|-m]",
	})
}
