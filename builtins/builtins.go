package builtins

import (
	grasp "github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/mounts"
)

// RegisterBuiltins mounts standard grasp utilities at the given path (e.g. "/bin").
func RegisterBuiltins(v *grasp.VirtualOS, mountPath string) error {
	fs := mounts.NewMemFS(grasp.PermRW)
	registerAllBuiltins(v, fs, "")
	return v.Mount(mountPath, fs)
}

// RegisterBuiltinsOnFS registers standard grasp utilities on the given MemFS at /usr/bin.
func RegisterBuiltinsOnFS(v *grasp.VirtualOS, fs *mounts.MemFS) error {
	registerAllBuiltins(v, fs, "usr/bin/")
	return nil
}

func registerAllBuiltins(v *grasp.VirtualOS, fs *mounts.MemFS, prefix string) {
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
	fs.AddExecFunc(prefix+"grep", builtinGrep(v), mounts.FuncMeta{
		Description: "Search for patterns in files",
		Usage:       "grep [OPTIONS] PATTERN [FILE]...",
	})
	fs.AddExecFunc(prefix+"mount", builtinMount(v), mounts.FuncMeta{
		Description: "List mount points",
		Usage:       "mount",
	})
	fs.AddExecFunc(prefix+"bind", builtinBind(v), mounts.FuncMeta{
		Description: "Plan 9-style union bind",
		Usage:       "bind [-b|-a] source_path target_path",
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
	fs.AddExecFunc(prefix+"rmdir", builtinRmdir(v), mounts.FuncMeta{
		Description: "Remove empty directories",
		Usage:       "rmdir [-p] [--ignore-fail-on-non-empty] [-v] <directory>...",
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
	fs.AddExecFunc(prefix+"date", builtinDate(v), mounts.FuncMeta{
		Description: "Display the current date and time",
		Usage:       "date [+FORMAT]",
	})
	fs.AddExecFunc(prefix+"whoami", builtinWhoami(v), mounts.FuncMeta{
		Description: "Display the current user",
		Usage:       "whoami",
	})
	fs.AddExecFunc(prefix+"sleep", builtinSleep(v), mounts.FuncMeta{
		Description: "Delay for a specified time",
		Usage:       "sleep NUMBER[SUFFIX]",
	})
	fs.AddExecFunc(prefix+"true", builtinTrue(v), mounts.FuncMeta{
		Description: "Return success exit status",
		Usage:       "true",
	})
	fs.AddExecFunc(prefix+"false", builtinFalse(v), mounts.FuncMeta{
		Description: "Return failure exit status",
		Usage:       "false",
	})
	fs.AddExecFunc(prefix+"whereis", builtinWhereis(v), mounts.FuncMeta{
		Description: "Locate command files",
		Usage:       "whereis COMMAND...",
	})
	fs.AddExecFunc(prefix+"sed", builtinSed(v), mounts.FuncMeta{
		Description: "Stream editor for filtering and transforming text",
		Usage:       "sed [-n] -e SCRIPT [FILE]...",
	})
	fs.AddExecFunc(prefix+"touch", builtinTouch(v), mounts.FuncMeta{
		Description: "Update file timestamps or create empty files",
		Usage:       "touch <file>...",
	})
	fs.AddExecFunc(prefix+"wc", builtinWc(v), mounts.FuncMeta{
		Description: "Print newline, word, and byte counts",
		Usage:       "wc [-l|-w|-m|-c|-L] [FILE]...",
	})
	fs.AddExecFunc(prefix+"jsonq", builtinJsonq(v), mounts.FuncMeta{
		Description: "Query JSON data using gojsonq",
		Usage:       "jsonq [OPTIONS] [QUERY] [FILE]...",
	})
}
