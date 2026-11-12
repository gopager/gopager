package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/persistence/migrations"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
)

// runBootstrap is a local-only, bounded initialization command without an HTTP listener.
func runBootstrap(ctx context.Context, args []string, getenv func(string) string, input io.Reader, output io.Writer) (result error) {
	flags := flag.NewFlagSet("gopager bootstrap", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	username := flags.String("username", "", "Initial local username")
	actions := flags.String("actions", "", "Explicit comma-separated initial actions; no wildcard")
	fields := flags.String("fields", "", "Explicit comma-separated initial readable fields")
	delegable := flags.Bool("delegable", false, "Permit delegation of these explicitly selected initial rights")
	mode := flags.String("migration-mode", string(platform.MigrationCheck), "Migration mode: check or apply")
	insecure := flags.Bool("allow-insecure-db", false, "Operator approval for unverified remote database TLS")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 ||
		(*mode != string(platform.MigrationCheck) && *mode != string(platform.MigrationApply)) {
		return platform.NewFailure(platform.CodeInvalidInput, err)
	}
	if _, err := identity.UsernameKey(*username); err != nil {
		return platform.NewFailure(platform.CodeInvalidInput, err)
	}
	permissions := identity.BootstrapPermissions{Actions: strings.Split(*actions, ","), Delegable: *delegable}
	if *fields != "" {
		permissions.Fields = strings.Split(*fields, ",")
	}
	password, err := readBootstrapPassword(input)
	if err != nil {
		return platform.NewFailure(platform.CodeInvalidInput, err)
	}
	store, insecureRemote, err := postgres.NewStore(getenv("GOPAGER_DATABASE_URL"), *insecure)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, store.Close()) }()
	if insecureRemote {
		slog.Warn("operator approved a remote database connection without certificate-verified TLS")
	}
	if err := store.Migrate(ctx, migrations.Files, platform.MigrationMode(*mode)); err != nil {
		return err
	}
	repository, err := identitystore.New(store)
	if err != nil {
		return err
	}
	facts, err := eventstore.New(store)
	if err != nil {
		return err
	}
	id, err := identity.Bootstrap(ctx, store, repository, store, facts, *username, password, permissions,
		identityOptions...)
	if err != nil {
		switch {
		case errors.Is(err, authz.ErrRevisionConflict):
			return platform.NewFailure(platform.CodeRevisionConflict, err)
		case errors.Is(err, identity.ErrInvalidInput), errors.Is(err, authz.ErrInvalidInput):
			return platform.NewFailure(platform.CodeInvalidInput, err)
		default:
			return err
		}
	}
	_, err = fmt.Fprintln(output, id.String())
	return err
}

// readBootstrapPassword accepts exact password bytes from a pipe/file, with one optional final LF delimiter.
// Interactive input is rejected rather than echoed by an unprotected terminal prompt.
func readBootstrapPassword(input io.Reader) (string, error) {
	if file, ok := input.(*os.File); ok {
		info, err := file.Stat()
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeCharDevice != 0 {
			return "", identity.ErrInvalidInput
		}
	}
	value, err := io.ReadAll(io.LimitReader(input, 1026))
	if err != nil {
		return "", err
	}
	if len(value) >= 1026 {
		return "", identity.ErrInvalidInput
	}
	password := strings.TrimSuffix(string(value), "\n")
	if err := identity.ValidatePassword(password); err != nil {
		return "", err
	}
	return password, nil
}
