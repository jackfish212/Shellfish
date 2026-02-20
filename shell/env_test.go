package shell

import (
	"testing"
)

func TestNewShellEnv(t *testing.T) {
	env := NewShellEnv()
	if env == nil {
		t.Fatal("NewShellEnv returned nil")
	}

	// Check default values
	if env.Get("PATH") != "/bin" {
		t.Errorf("PATH = %q, want /bin", env.Get("PATH"))
	}
	if env.Get("PWD") != "/" {
		t.Errorf("PWD = %q, want /", env.Get("PWD"))
	}
	if env.Get("USER") != "root" {
		t.Errorf("USER = %q, want root", env.Get("USER"))
	}
	if env.Get("HOME") != "/" {
		t.Errorf("HOME = %q, want /", env.Get("HOME"))
	}
}

func TestShellEnvGetSet(t *testing.T) {
	env := NewShellEnv()

	// Test Set and Get
	env.Set("MYVAR", "myvalue")
	if env.Get("MYVAR") != "myvalue" {
		t.Errorf("Get(MYVAR) = %q, want myvalue", env.Get("MYVAR"))
	}

	// Test non-existent key
	if env.Get("NONEXISTENT") != "" {
		t.Errorf("Get(NONEXISTENT) = %q, want empty", env.Get("NONEXISTENT"))
	}

	// Test overwriting
	env.Set("MYVAR", "newvalue")
	if env.Get("MYVAR") != "newvalue" {
		t.Errorf("Get(MYVAR) after overwrite = %q, want newvalue", env.Get("MYVAR"))
	}
}

func TestShellEnvAll(t *testing.T) {
	env := NewShellEnv()
	env.Set("CUSTOM", "value")

	all := env.All()
	if all == nil {
		t.Fatal("All() returned nil")
	}

	// Check that default values are present
	if all["PATH"] != "/bin" {
		t.Errorf("All()[PATH] = %q, want /bin", all["PATH"])
	}
	if all["CUSTOM"] != "value" {
		t.Errorf("All()[CUSTOM] = %q, want value", all["CUSTOM"])
	}

	// Verify it's a copy (modifying shouldn't affect original)
	all["PATH"] = "modified"
	if env.Get("PATH") != "/bin" {
		t.Errorf("modifying All() affected original env")
	}
}

func TestIsAlnumOrUnderscore(t *testing.T) {
	tests := []struct {
		ch       byte
		expected bool
	}{
		{'a', true},
		{'z', true},
		{'A', true},
		{'Z', true},
		{'0', true},
		{'9', true},
		{'_', true},
		{'-', false},
		{'$', false},
		{' ', false},
		{'@', false},
		{'.', false},
	}

	for _, tt := range tests {
		t.Run(string(tt.ch), func(t *testing.T) {
			result := isAlnumOrUnderscore(tt.ch)
			if result != tt.expected {
				t.Errorf("isAlnumOrUnderscore('%c') = %v, want %v", tt.ch, result, tt.expected)
			}
		})
	}
}
