package builtins

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/jackfish212/grasp/mounts"
)

func builtinUname() mounts.ExecFunc {
	return func(ctx context.Context, args []string, _ io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader(`uname — print system information
Usage: uname [OPTION]...
Options:
  -a, --all            print all information
  -s, --kernel-name    print the kernel name
  -n, --nodename       print the network node hostname
  -r, --kernel-release print the kernel release
  -v, --kernel-version print the kernel version
  -m, --machine        print the machine hardware name
`)), nil
		}

		info := unameInfo{
			SysName:  "AgentFS",
			NodeName: "grasp",
			Release:  Version,
			Version:  runtime.Version(),
			Machine:  runtime.GOARCH,
		}

		showAll := hasFlag(args, "-a", "--all")
		showSysName := hasFlag(args, "-s", "--kernel-name")
		showNodeName := hasFlag(args, "-n", "--nodename")
		showRelease := hasFlag(args, "-r", "--kernel-release")
		showVersion := hasFlag(args, "-v", "--kernel-version")
		showMachine := hasFlag(args, "-m", "--machine")

		noFlags := !showAll && !showSysName && !showNodeName && !showRelease && !showVersion && !showMachine
		if noFlags {
			showSysName = true
		}

		var parts []string
		if showAll {
			parts = []string{info.SysName, info.NodeName, info.Release, info.Version, info.Machine}
		} else {
			if showSysName {
				parts = append(parts, info.SysName)
			}
			if showNodeName {
				parts = append(parts, info.NodeName)
			}
			if showRelease {
				parts = append(parts, info.Release)
			}
			if showVersion {
				parts = append(parts, info.Version)
			}
			if showMachine {
				parts = append(parts, info.Machine)
			}
		}

		return io.NopCloser(strings.NewReader(fmt.Sprintf("%s\n", strings.Join(parts, " ")))), nil
	}
}

type unameInfo struct {
	SysName  string
	NodeName string
	Release  string
	Version  string
	Machine  string
}

// Version can be set via ldflags at build time.
var Version = "dev"
