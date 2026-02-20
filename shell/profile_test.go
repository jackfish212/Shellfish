package shell

import (
	"testing"
)

func TestParseExportLine(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantName  string
		wantValue string
	}{
		{
			name:      "simple export",
			line:      "export FOO=bar",
			wantName:  "FOO",
			wantValue: "bar",
		},
		{
			name:      "export with double quotes",
			line:      `export FOO="bar baz"`,
			wantName:  "FOO",
			wantValue: "bar baz",
		},
		{
			name:      "export with single quotes",
			line:      `export FOO='bar baz'`,
			wantName:  "FOO",
			wantValue: "bar baz",
		},
		{
			name:      "without export keyword",
			line:      "FOO=bar",
			wantName:  "FOO",
			wantValue: "bar",
		},
		{
			name:      "empty value",
			line:      "FOO=",
			wantName:  "FOO",
			wantValue: "",
		},
		{
			name:      "no equals sign",
			line:      "FOO",
			wantName:  "",
			wantValue: "",
		},
		{
			name:      "empty line",
			line:      "",
			wantName:  "",
			wantValue: "",
		},
		{
			name:      "equals at start",
			line:      "=value",
			wantName:  "",
			wantValue: "",
		},
		{
			name:      "value with equals",
			line:      "FOO=bar=baz",
			wantName:  "FOO",
			wantValue: "bar=baz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, value := parseExportLine(tt.line)
			if name != tt.wantName {
				t.Errorf("parseExportLine(%q) name = %q, want %q", tt.line, name, tt.wantName)
			}
			if value != tt.wantValue {
				t.Errorf("parseExportLine(%q) value = %q, want %q", tt.line, value, tt.wantValue)
			}
		})
	}
}
