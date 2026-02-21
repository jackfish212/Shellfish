package shell

import (
	"context"
	"io"
	"log/slog"
	"strings"

	"github.com/jackfish212/grasp/types"
)

func (s *Shell) loadProfileEnv() {
	ctx := context.Background()
	s.loadProfileDir(ctx, "/etc")
}

func (s *Shell) loadProfileDir(ctx context.Context, base string) {
	s.loadProfileFile(ctx, base+"/profile")

	entries, err := s.vos.List(ctx, base+"/profile.d", types.ListOpts{})
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir {
			continue
		}
		if strings.HasSuffix(entry.Name, ".sh") {
			s.loadProfileFile(ctx, base+"/profile.d/"+entry.Name)
		}
	}
}

func (s *Shell) loadProfileFile(ctx context.Context, path string) {
	rc, err := s.vos.Open(ctx, path)
	if err != nil {
		slog.Debug("shell: failed to open profile file", "path", path, "error", err)
		return
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		varName, varValue := parseExportLine(line)
		if varName != "" {
			slog.Debug("shell: loaded profile variable", "varName", varName, "varValue", varValue)
			s.Env.Set(varName, varValue)
		}
	}
}

func parseExportLine(line string) (string, string) {
	line = strings.TrimPrefix(line, "export ")
	eqIdx := strings.Index(line, "=")
	if eqIdx <= 0 {
		return "", ""
	}
	varName := line[:eqIdx]
	varValue := line[eqIdx+1:]
	varValue = strings.Trim(varValue, "\"'")
	return varName, varValue
}
