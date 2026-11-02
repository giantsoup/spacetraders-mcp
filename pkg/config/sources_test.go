package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func cleanEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{"SPACETRADERS_API_TOKEN", "SPACETRADERS_TOKEN", "SPACETRADERS_TOKEN_COMMAND", "SPACETRADERS_TOKEN_FILE"} {
		t.Setenv(key, "")
	}
}

func helperCommand(t *testing.T, mode string) string {
	t.Helper()
	t.Setenv("MCP_TEST_SECRET_HELPER", "1")
	argv, err := json.Marshal([]string{os.Args[0], "-test.run=^TestSecretHelperProcess$", "--", mode})
	if err != nil {
		t.Fatal(err)
	}
	return string(argv)
}

func TestSecretHelperProcess(t *testing.T) {
	if os.Getenv("MCP_TEST_SECRET_HELPER") != "1" {
		return
	}
	switch os.Args[len(os.Args)-1] {
	case "ok":
		fmt.Print("helper-token\n")
	case "empty":
	case "fail":
		_, _ = fmt.Fprint(os.Stdout, "SECRET_STDOUT")
		_, _ = fmt.Fprint(os.Stderr, "SECRET_STDERR")
		os.Exit(1)
	case "large":
		fmt.Print(strings.Repeat("x", 2*maxTokenBytes))
	case "control":
		fmt.Print("SECRET\r\nInjected:value")
	case "sleep":
		time.Sleep(time.Hour)
	default:
		os.Exit(2)
	}
	os.Exit(0)
}

func TestCredentialSources(t *testing.T) {
	for _, test := range []struct {
		name, helper, file, env, alias, dotenv, want, source string
		warning                                              bool
	}{
		{"helper first", "ok", "file-token", "env-token", "alias-token", "SPACETRADERS_API_TOKEN=dotenv-token", "helper-token", "secret helper", false},
		{"helper failure falls back", "fail", "file-token", "", "", "", "file-token", "secret file", true},
		{"empty helper falls back", "empty", "file-token", "", "", "", "file-token", "secret file", false},
		{"oversized helper rejected", "large", "file-token", "", "", "", "file-token", "secret file", true},
		{"helper header injection rejected", "control", "file-token", "", "", "", "file-token", "secret file", true},
		{"file before env", "", "file-token\n", "env-token", "alias-token", "", "file-token", "secret file", false},
		{"missing file falls back", "", "MISSING", "env-token", "", "", "env-token", "SPACETRADERS_API_TOKEN", false},
		{"empty file falls back", "", "", "env-token", "", "", "env-token", "SPACETRADERS_API_TOKEN", false},
		{"invalid env falls back", "", "", "invalid\nSECRET", "alias-token", "", "alias-token", "SPACETRADERS_TOKEN", true},
		{"legacy env before alias", "", "", "env-token", "alias-token", "SPACETRADERS_API_TOKEN=dotenv-token", "env-token", "SPACETRADERS_API_TOKEN", false},
		{"alias before dotenv", "", "", "", "alias-token", "SPACETRADERS_API_TOKEN=dotenv-token", "alias-token", "SPACETRADERS_TOKEN", false},
		{"dotenv alias", "", "", "", "", "SPACETRADERS_TOKEN=dotenv-token", "dotenv-token", ".env", false},
		{"dotenv empty legacy key", "", "", "", "", "SPACETRADERS_API_TOKEN=\nSPACETRADERS_TOKEN=dotenv-token", "dotenv-token", ".env", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			cleanEnvironment(t)
			t.Chdir(t.TempDir())
			t.Setenv("SPACETRADERS_API_TOKEN", test.env)
			t.Setenv("SPACETRADERS_TOKEN", test.alias)
			options := Options{TokenFile: filepath.Join(t.TempDir(), "token")}
			if test.helper != "" {
				options.TokenCommand = helperCommand(t, test.helper)
			}
			if test.file != "MISSING" {
				if err := os.WriteFile(options.TokenFile, []byte(test.file), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if test.dotenv != "" {
				if err := os.WriteFile(".env", []byte(test.dotenv), 0600); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := Load(options)
			if err != nil || cfg.SpaceTradersAPIToken != test.want || cfg.TokenSource != test.source {
				t.Fatalf("unexpected credential resolution: %v, %v", cfg, err)
			}
			if (len(cfg.Warnings) > 0) != test.warning {
				t.Fatalf("unexpected warnings: %v", cfg.Warnings)
			}
			if strings.Contains(strings.Join(cfg.Warnings, " "), "SECRET") {
				t.Fatal("secret leaked into warnings")
			}
		})
	}
}

func TestSecretReferenceEnvironmentAndFlagOverride(t *testing.T) {
	cleanEnvironment(t)
	t.Chdir(t.TempDir())
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("file-token"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SPACETRADERS_TOKEN_COMMAND", helperCommand(t, "ok"))
	t.Setenv("SPACETRADERS_TOKEN_FILE", path)
	cfg, err := Load(Options{})
	if err != nil || cfg.TokenSource != "secret helper" {
		t.Fatalf("environment references not resolved: %v", err)
	}
	cfg, err = Load(Options{TokenCommand: helperCommand(t, "fail")})
	if err != nil || cfg.TokenSource != "secret file" {
		t.Fatalf("flag did not override environment command: %v", err)
	}
}

func TestDotEnvCannotExecuteHelper(t *testing.T) {
	cleanEnvironment(t)
	t.Chdir(t.TempDir())
	raw := "SPACETRADERS_TOKEN_COMMAND='" + helperCommand(t, "ok") + "'\nSPACETRADERS_TOKEN_FILE=/private/path/SECRET\n"
	if err := os.WriteFile(".env", []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if cfg, err := Load(Options{}); err == nil || cfg != nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("untrusted .env reference was used or exposed: %v", err)
	}
}

func TestTokenErrorsAndFormattingDoNotExposeSecrets(t *testing.T) {
	cleanEnvironment(t)
	t.Chdir(t.TempDir())
	for _, command := range []string{"SECRET_NOT_JSON", `["/missing/SECRET"]`, helperCommand(t, "fail")} {
		_, err := Load(Options{TokenCommand: command})
		if err == nil || strings.Contains(err.Error(), "SECRET") {
			t.Fatalf("missing or unsafe error: %v", err)
		}
	}
	cfg := Config{SpaceTradersAPIToken: "SECRET", TokenSource: "secret helper"}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fmt.Sprintf("%v %+v %#v %s", cfg, &cfg, cfg, raw), "SECRET") {
		t.Fatal("formatted config exposed secret")
	}
}

func TestCommandTimeoutAndLiteralArguments(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := readCommand(ctx, helperCommand(t, "sleep")); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("helper timeout not enforced: %v", err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	// A shell would expand this into a process; direct argv must preserve it.
	argv, err := json.Marshal([]string{"/usr/bin/printf", "%s", "$(touch should-not-exist)"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := readCommand(context.Background(), string(argv))
	if err != nil || raw != "$(touch should-not-exist)" {
		t.Fatalf("helper arguments were interpreted: %v", err)
	}
}

func TestSecretFileBoundaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", maxTokenBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readFile(path, true, maxTokenBytes); err == nil {
		t.Fatal("oversized file accepted")
	}
	if _, err := readFile(filepath.Dir(path), true, maxTokenBytes); err == nil {
		t.Fatal("directory accepted")
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := readFile(path, true, maxTokenBytes); err == nil {
			t.Fatal("publicly readable secret file accepted")
		}
	}
}
