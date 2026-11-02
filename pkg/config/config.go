// Package config resolves gameplay credentials without exposing secret values.
package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const maxTokenBytes = 16 << 10

// Options contains secret references. TokenCommand is a JSON argv array,
// executed directly without a shell, only when explicitly configured.
type Options struct {
	TokenCommand string
	TokenFile    string
}

type Config struct {
	SpaceTradersAPIToken string `json:"-"`
	TokenSource          string
	Warnings             []string
}

func (c Config) String() string   { return "SpaceTraders credential [redacted], source=" + c.TokenSource }
func (c Config) GoString() string { return c.String() }

// Load uses the first usable source once at startup. It never retries API
// requests or searches unrelated files and password-manager entries.
func Load(options Options) (*Config, error) {
	command := options.TokenCommand
	if command == "" {
		command = os.Getenv("SPACETRADERS_TOKEN_COMMAND")
	}
	file := options.TokenFile
	if file == "" {
		file = os.Getenv("SPACETRADERS_TOKEN_FILE")
	}
	type source struct {
		name string
		read func() (string, error)
	}
	sources := []source{}
	if command != "" {
		sources = append(sources, source{"secret helper", func() (string, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return readCommand(ctx, command)
		}})
	}
	if file != "" {
		sources = append(sources, source{"secret file", func() (string, error) {
			return readFile(file, true, maxTokenBytes)
		}})
	}
	for _, key := range []string{"SPACETRADERS_API_TOKEN", "SPACETRADERS_TOKEN"} {
		sources = append(sources, source{key, func() (string, error) { return os.Getenv(key), nil }})
	}
	// Working-directory .env files cannot authorize helper execution or reads
	// from arbitrary paths. Only the two legacy token keys are read here.
	sources = append(sources, source{".env", readDotEnv})
	warnings := []string{}
	for _, candidate := range sources {
		raw, err := candidate.read()
		if err != nil {
			warnings = append(warnings, candidate.name+": "+err.Error())
			continue
		}
		token, err := validateToken(raw)
		if err != nil {
			warnings = append(warnings, candidate.name+": "+err.Error())
			continue
		}
		if token != "" {
			return &Config{SpaceTradersAPIToken: token, TokenSource: candidate.name, Warnings: warnings}, nil
		}
	}
	message := "no usable SpaceTraders agent token; configure --token-command, --token-file, SPACETRADERS_API_TOKEN, or SPACETRADERS_TOKEN"
	if len(warnings) > 0 {
		message += "; " + strings.Join(warnings, "; ")
	}
	return nil, errors.New(message)
}

func validateToken(raw string) (string, error) {
	// Files and helpers may end with a newline. Internal whitespace and control
	// characters cannot enter an Authorization header.
	token := strings.TrimSpace(raw)
	if len(token) > maxTokenBytes {
		return "", errors.New("token exceeds 16 KiB")
	}
	for _, char := range token {
		if char <= 32 || char >= 127 {
			return "", errors.New("token must contain only visible ASCII without whitespace")
		}
	}
	return token, nil
}

func readFile(path string, private bool, limit int) (string, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("not a readable regular file")
	}
	if private && runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return "", errors.New("requires owner-only permissions, such as 0600")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", errors.New("cannot open file")
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, int64(limit+1)))
	if err != nil || len(data) > limit {
		return "", errors.New("cannot read file within size limit")
	}
	return string(data), nil
}

func readDotEnv() (string, error) {
	raw, err := readFile(".env", false, 64<<10)
	if err != nil || raw == "" {
		return "", err
	}
	v := viper.New()
	v.SetConfigType("env")
	if v.ReadConfig(strings.NewReader(raw)) != nil {
		return "", errors.New("cannot parse token configuration")
	}
	for _, key := range []string{"SPACETRADERS_API_TOKEN", "SPACETRADERS_TOKEN"} {
		token, err := validateToken(v.GetString(key))
		if err != nil {
			return "", err
		}
		if token != "" {
			return token, nil
		}
	}
	return "", nil
}

type secretOutput struct{ buffer bytes.Buffer }

func (b *secretOutput) Write(p []byte) (int, error) {
	if b.buffer.Len()+len(p) > maxTokenBytes {
		return 0, errors.New("secret helper output exceeds 16 KiB")
	}
	return b.buffer.Write(p)
}

func readCommand(ctx context.Context, raw string) (string, error) {
	var argv []string
	if len(raw) > 32<<10 || json.Unmarshal([]byte(raw), &argv) != nil || len(argv) == 0 || argv[0] == "" {
		return "", errors.New("expected a JSON array containing an executable and its arguments")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.WaitDelay = 250 * time.Millisecond
	var output secretOutput
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	if cmd.Run() != nil {
		if ctx.Err() != nil {
			return "", errors.New("timed out or cancelled")
		}
		// Underlying exec errors, argv, stdout and stderr may contain secrets.
		return "", errors.New("failed or exceeded output limit")
	}
	return output.buffer.String(), nil
}
