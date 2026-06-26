package config

import (
	"os"
	"testing"
)

func TestParseDefaults(t *testing.T) {
	cfg, sub, subArgs, err := Parse([]string{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Host != "0.0.0.0" {
		t.Fatalf("default host = %q, want 0.0.0.0", cfg.Host)
	}
	if cfg.Port != 8083 {
		t.Fatalf("default port = %d, want 8083", cfg.Port)
	}
	if cfg.DataDir != "./data" {
		t.Fatalf("default data-dir = %q, want ./data", cfg.DataDir)
	}
	if sub != "" {
		t.Fatalf("default subcommand = %q, want empty", sub)
	}
	if len(subArgs) != 0 {
		t.Fatalf("default subArgs = %v, want empty", subArgs)
	}
}

func TestParseFlags(t *testing.T) {
	cfg, _, _, err := Parse([]string{"--host", "127.0.0.1", "--port", "9090", "--data-dir", "/tmp/test"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Host != "127.0.0.1" {
		t.Fatalf("host = %q, want 127.0.0.1", cfg.Host)
	}
	if cfg.Port != 9090 {
		t.Fatalf("port = %d, want 9090", cfg.Port)
	}
	if cfg.DataDir != "/tmp/test" {
		t.Fatalf("data-dir = %q, want /tmp/test", cfg.DataDir)
	}
}

func TestParseSubcommand(t *testing.T) {
	_, sub, subArgs, err := Parse([]string{"set-credentials"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if sub != "set-credentials" {
		t.Fatalf("sub = %q, want set-credentials", sub)
	}
	if len(subArgs) != 0 {
		t.Fatalf("subArgs = %v, want empty", subArgs)
	}
}

func TestParseImportSubcommand(t *testing.T) {
	_, sub, subArgs, err := Parse([]string{"import", "/path/to/file.json"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if sub != "import" {
		t.Fatalf("sub = %q, want import", sub)
	}
	if len(subArgs) != 1 || subArgs[0] != "/path/to/file.json" {
		t.Fatalf("subArgs = %v, want [/path/to/file.json]", subArgs)
	}
}

func TestParseFlagsBeforeSubcommand(t *testing.T) {
	cfg, sub, subArgs, err := Parse([]string{"--data-dir", "/custom", "import", "file.json"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.DataDir != "/custom" {
		t.Fatalf("data-dir = %q, want /custom", cfg.DataDir)
	}
	if sub != "import" {
		t.Fatalf("sub = %q, want import", sub)
	}
	if len(subArgs) != 1 || subArgs[0] != "file.json" {
		t.Fatalf("subArgs = %v, want [file.json]", subArgs)
	}
}

func TestEnvOverrides(t *testing.T) {
	os.Setenv("COST_BOARD_HOST", "1.2.3.4")
	os.Setenv("COST_BOARD_PORT", "3000")
	os.Setenv("COST_BOARD_DATA_DIR", "/env-data")
	defer func() {
		os.Unsetenv("COST_BOARD_HOST")
		os.Unsetenv("COST_BOARD_PORT")
		os.Unsetenv("COST_BOARD_DATA_DIR")
	}()

	cfg, _, _, err := Parse([]string{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Host != "1.2.3.4" {
		t.Fatalf("host = %q, want 1.2.3.4", cfg.Host)
	}
	if cfg.Port != 3000 {
		t.Fatalf("port = %d, want 3000", cfg.Port)
	}
	if cfg.DataDir != "/env-data" {
		t.Fatalf("data-dir = %q, want /env-data", cfg.DataDir)
	}
}

func TestEnvOrIntInvalid(t *testing.T) {
	os.Setenv("COST_BOARD_PORT", "not-a-number")
	defer os.Unsetenv("COST_BOARD_PORT")

	_, _, _, err := Parse([]string{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// should fall back to default
}

func TestEnvOrEmptyStringFallsBack(t *testing.T) {
	os.Setenv("COST_BOARD_HOST", "")
	defer os.Unsetenv("COST_BOARD_HOST")

	cfg, _, _, err := Parse([]string{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Host != "0.0.0.0" {
		t.Fatalf("empty env should fall back to default, got %q", cfg.Host)
	}
}
