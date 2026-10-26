package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
)

var errInvalidRoles = errors.New("invalid role list")

func main() {
	if err := run(); err != nil {
		slog.Error("GoPager exited", "code", platform.FailureCode(err))
		os.Exit(1)
	}
}

func run() error {
	config, err := parseConfig(os.Args[1:], os.Getenv)
	if err != nil {
		return err
	}

	store, insecureRemote, err := postgres.NewStore(config.DatabaseURL, config.AllowInsecureDBTLS)
	if err != nil {
		return err
	}
	if insecureRemote {
		slog.Warn("operator approved a remote database connection without certificate-verified TLS")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	handle, err := platform.Start(ctx, config, nil, store, nil)
	if err != nil {
		return errors.Join(err, store.Close())
	}
	slog.Info("platform started", "profile", config.Profile, "roles", config.Roles)
	<-ctx.Done()
	return handle.Shutdown(context.Background())
}

// parseConfig binds only the platform's explicit flags and secret environment input.
func parseConfig(args []string, getenv func(string) string) (platform.Config, error) {
	flags := flag.NewFlagSet("gopager", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	profile := flags.String("profile", "", "Full operating profile")
	rolesValue := flags.String("roles", "", "Comma-separated roles: api,worker")
	mode := flags.String("migration-mode", string(platform.MigrationCheck), "Migration mode: check or apply")
	address := flags.String("listen-address", "", "Explicit API listen address")
	allowInsecure := flags.Bool("allow-insecure-db", false, "Operator approval for unverified remote database TLS")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return platform.Config{}, platform.NewFailure(platform.CodeInvalidInput, err)
	}
	roles, err := parseRoles(*rolesValue)
	if err != nil {
		return platform.Config{}, platform.NewFailure(platform.CodeInvalidInput, err)
	}
	config := platform.Config{
		Profile: *profile, Roles: roles, MigrationMode: platform.MigrationMode(*mode),
		ListenAddress: *address, DatabaseURL: getenv("GOPAGER_DATABASE_URL"),
		AllowInsecureDBTLS: *allowInsecure,
	}
	return config, config.Validate()
}

func parseRoles(value string) ([]platform.Role, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	values := strings.Split(value, ",")
	roles := make([]platform.Role, 0, len(values))
	for _, value := range values {
		role := strings.TrimSpace(value)
		if role == "" {
			return nil, errInvalidRoles
		}
		roles = append(roles, platform.Role(role))
	}
	return roles, nil
}
