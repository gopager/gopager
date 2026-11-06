// Command gopager runs the GoPager server.
//
// Usage:
//
//	gopager -profile full -roles api,worker -listen-address HOST:PORT [flags]
//	gopager bootstrap -username NAME -actions A,B,... [flags] < password-file
//
// Server mode composes the requested roles on one PostgreSQL primary:
//
//   - api serves the HTTP listener: /health/live, /health/ready and the
//     /api/v1 application routes (authentication, management, Gateway intake
//     and administration, Processing).
//   - worker runs the background Processing dispatcher and the assignment
//     correction runner.
//
// Server flags: -profile (must be "full"), -roles (comma-separated, required),
// -listen-address (required with the api role), -migration-mode (check, the
// default, refuses to start unless the schema is exactly current; apply runs
// pending migrations first), -public-origin (the HTTP(S) origin used for
// cookie and Origin checks; required with the api role),
// -processing-artifact-root (directory of operator-provisioned plugin
// executables; falls back to $GOPAGER_PROCESSING_ARTIFACT_ROOT),
// -login-attempts-per-account and -login-attempts-per-instance (per-minute
// login limits, positive) and -allow-insecure-db (accept a remote database
// without certificate-verified TLS). Positional arguments are rejected.
//
// The database connection string is read only from $GOPAGER_DATABASE_URL so
// that it never appears in process listings. The process runs until SIGINT or
// SIGTERM and then shuts down gracefully.
//
// The bootstrap subcommand creates the initial local account of a fresh
// installation without starting a listener; it fails with REVISION_CONFLICT
// when the installation is already initialized. It takes -username, -actions (explicit comma-separated actions,
// no wildcards), -fields (readable fields), -delegable, -migration-mode and
// -allow-insecure-db. The password is read verbatim from standard input, which
// must be a pipe or file, not a terminal (one trailing newline is stripped).
// On success it prints the new account's ID to standard output.
//
// On failure gopager logs only the platform error code and exits with status 1.
package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	units "github.com/gopager/gopager/internal/Units"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/identity"
	ingestdomain "github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/persistence/ent/assignmentstore"
	"github.com/gopager/gopager/internal/persistence/ent/commandstore"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/persistence/ent/ingeststore"
	"github.com/gopager/gopager/internal/persistence/ent/unitstore"
	"github.com/gopager/gopager/internal/persistence/migrations"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing/worker"
)

var errInvalidRoles = errors.New("invalid role list")

type startupConfig struct {
	platform.Config
	PublicOrigin           string
	ProcessingArtifactRoot string
	LoginLimits            identity.LoginRateLimits
	ProcessingDispatch     worker.DispatchConfig
}

func main() {
	if err := run(); err != nil {
		slog.Error("GoPager exited", "code", platform.FailureCode(err))
		os.Exit(1)
	}
}

func run() (result error) {
	if len(os.Args) > 1 && os.Args[1] == "bootstrap" {
		ctx, cancel := context.WithTimeout(context.Background(), platform.DefaultCallTimeout)
		defer cancel()
		return runBootstrap(ctx, os.Args[2:], os.Getenv, os.Stdin, os.Stdout)
	}
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
	var runners []platform.Runner
	var processingRuntime *processingAPIRuntime
	if slices.Contains(config.Roles, platform.RoleAPI) || slices.Contains(config.Roles, platform.RoleWorker) {
		processingRuntime, err = composeProcessingAPIRuntime(ctx, store, config.ProcessingArtifactRoot)
		if err != nil {
			return errors.Join(err, store.Close())
		}
		defer func() {
			result = errors.Join(result, processingRuntime.Close(context.Background()))
		}()
	}
	if slices.Contains(config.Roles, platform.RoleWorker) {
		runtime, err := composeProcessingWorker(ctx, store, processingRuntime.workerSource,
			processingRuntime.workerPublisher, config.ProcessingDispatch)
		if err != nil {
			return errors.Join(err, store.Close())
		}
		defer func() {
			result = errors.Join(result, runtime.Close(context.Background()))
		}()
		runners = append(runners, runtime.dispatcher)
		correctionRunner, err := composeCorrectionRunner(store, processingRuntime.correctionReadmission)
		if err != nil {
			return errors.Join(err, store.Close())
		}
		runners = append(runners, correctionRunner)
	}
	var api http.Handler
	if slices.Contains(config.Roles, platform.RoleAPI) {
		api, err = composeApplicationAPI(config.PublicOrigin, config.LoginLimits, store, nil, processingRuntime.routes)
		if err != nil {
			return errors.Join(err, store.Close())
		}
	}
	handle, err := platform.StartWithAPI(ctx, config.Config, migrations.Files, store, runners, api)
	if err != nil {
		return errors.Join(err, store.Close())
	}
	slog.Info("platform started", "profile", config.Profile, "roles", config.Roles)
	<-ctx.Done()
	return handle.Shutdown(context.Background())
}

func composeCorrectionRunner(store *postgres.Store, readmission assignments.ProcessingReadmission,
) (*assignments.CorrectionRunner, error) {
	identityRepository, err := identitystore.New(store)
	if err != nil {
		return nil, err
	}
	unitRepository, err := unitstore.New(store)
	if err != nil {
		return nil, err
	}
	assignmentRepository, err := assignmentstore.New(store)
	if err != nil {
		return nil, err
	}
	receipts, err := commandstore.New(store)
	if err != nil {
		return nil, err
	}
	facts, err := eventstore.New(store)
	if err != nil {
		return nil, err
	}
	intakeRepository, err := ingeststore.New(store, facts, facts)
	if err != nil {
		return nil, err
	}
	resolver, err := identity.NewResolver(identityRepository, store, applicationScopeOwners{
		identity: identityRepository, units: unitRepository,
	})
	if err != nil {
		return nil, err
	}
	matcher, err := assignments.NewService(store, resolver, store, assignmentRepository,
		unitRepository, intakeRepository, receipts, facts, facts)
	if err != nil {
		return nil, err
	}
	return assignments.NewCorrectionRunner(store, store, matcher, assignmentRepository, facts, readmission)
}

// parseConfig binds only the platform's explicit flags and secret environment input.
func parseConfig(args []string, getenv func(string) string) (startupConfig, error) {
	flags := flag.NewFlagSet("gopager", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	profile := flags.String("profile", "", "Full operating profile")
	rolesValue := flags.String("roles", "", "Comma-separated roles: api,worker")
	mode := flags.String("migration-mode", string(platform.MigrationCheck), "Migration mode: check or apply")
	address := flags.String("listen-address", "", "Explicit API listen address")
	publicOrigin := flags.String("public-origin", "", "Public HTTP(S) origin for application Cookie and Origin policy")
	processingArtifactRoot := flags.String("processing-artifact-root", "", "Optional operator-provisioned plugin directory (or GOPAGER_PROCESSING_ARTIFACT_ROOT)")
	defaultLimits := identity.DefaultLoginRateLimits()
	loginAccount := flags.Int("login-attempts-per-account", defaultLimits.AccountAttempts, "Per-minute login attempt limit per normalized username")
	loginInstance := flags.Int("login-attempts-per-instance", defaultLimits.InstanceAttempts, "Per-minute instance login attempt limit")
	allowInsecure := flags.Bool("allow-insecure-db", false, "Operator approval for unverified remote database TLS")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return startupConfig{}, platform.NewFailure(platform.CodeInvalidInput, err)
	}
	artifactRootSet := false
	flags.Visit(func(value *flag.Flag) {
		if value.Name == "processing-artifact-root" {
			artifactRootSet = true
		}
	})
	if artifactRootSet && *processingArtifactRoot == "" {
		return startupConfig{}, platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid Processing artifact root"))
	}
	// Container deployments may name the mounted plugin directory through the environment.
	if !artifactRootSet {
		*processingArtifactRoot = getenv("GOPAGER_PROCESSING_ARTIFACT_ROOT")
	}
	roles, err := parseRoles(*rolesValue)
	if err != nil {
		return startupConfig{}, platform.NewFailure(platform.CodeInvalidInput, err)
	}
	config := platform.Config{
		Profile: *profile, Roles: roles, MigrationMode: platform.MigrationMode(*mode),
		ListenAddress: *address, DatabaseURL: getenv("GOPAGER_DATABASE_URL"),
		AllowInsecureDBTLS: *allowInsecure,
	}
	if err := config.Validate(); err != nil {
		return startupConfig{}, err
	}
	if slices.Contains(roles, platform.RoleAPI) || *publicOrigin != "" {
		if _, err := identity.NewHTTPTransport(*publicOrigin); err != nil {
			return startupConfig{}, platform.NewFailure(platform.CodeInvalidInput, err)
		}
	}
	if *loginAccount <= 0 || *loginInstance <= 0 {
		return startupConfig{}, platform.NewFailure(platform.CodeInvalidInput, identity.ErrInvalidInput)
	}
	if *processingArtifactRoot != "" {
		if strings.TrimSpace(*processingArtifactRoot) != *processingArtifactRoot {
			return startupConfig{}, platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid Processing artifact root"))
		}
		root, err := os.OpenRoot(*processingArtifactRoot)
		if err != nil {
			return startupConfig{}, platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid Processing artifact root"))
		}
		if err := root.Close(); err != nil {
			return startupConfig{}, platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid Processing artifact root"))
		}
	}
	return startupConfig{Config: config, PublicOrigin: *publicOrigin,
		ProcessingArtifactRoot: *processingArtifactRoot,
		LoginLimits:            identity.LoginRateLimits{AccountAttempts: *loginAccount, InstanceAttempts: *loginInstance},
		ProcessingDispatch:     (worker.DispatchConfig{}).WithDefaults()}, nil
}

// composeApplicationAPI binds domain services to the existing primary store and HTTP listener.
func composeApplicationAPI(publicOrigin string, limits identity.LoginRateLimits, store *postgres.Store, networkOwner assignments.NetworkOwner,
	processingRoutes ...processingAPIRoutes) (http.Handler, error) {
	if len(processingRoutes) > 1 {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("multiple Processing API route sets"))
	}
	transport, err := identity.NewHTTPTransport(publicOrigin)
	if err != nil {
		return nil, err
	}
	repository, err := identitystore.New(store)
	if err != nil {
		return nil, err
	}
	unitRepository, err := unitstore.New(store)
	if err != nil {
		return nil, err
	}
	ruleRepository, err := assignmentstore.New(store)
	if err != nil {
		return nil, err
	}
	receipts, err := commandstore.New(store)
	if err != nil {
		return nil, err
	}
	facts, err := eventstore.New(store)
	if err != nil {
		return nil, err
	}
	intakeRepository, err := ingeststore.New(store, facts, facts)
	if err != nil {
		return nil, err
	}
	if networkOwner == nil {
		networkOwner = intakeRepository
	}
	resolver, err := identity.NewResolver(repository, store, applicationScopeOwners{
		identity: repository, units: unitRepository,
	})
	if err != nil {
		return nil, err
	}
	login, err := identity.NewLoginService(store, resolver, repository, facts, limits)
	if err != nil {
		return nil, err
	}
	authHandler, err := identity.NewHTTPHandler(transport, login)
	if err != nil {
		return nil, err
	}
	unitService, err := units.NewService(store, resolver, store, unitRepository, ruleRepository, receipts, facts)
	if err != nil {
		return nil, err
	}
	ruleService, err := assignments.NewService(store, resolver, store, ruleRepository, unitRepository,
		networkOwner, receipts, facts, facts)
	if err != nil {
		return nil, err
	}
	intakeService, err := ingestdomain.NewService(store, store, intakeRepository, ruleService)
	if err != nil {
		return nil, err
	}
	gatewayAdministration, err := ingestdomain.NewGatewayAdministration(store, resolver, store,
		intakeRepository, receipts, facts)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/auth/", authHandler)
	registerManagementRoutes(mux, transport, unitService, ruleService)
	registerIngestRoutes(mux, intakeService)
	registerGatewayManagementRoutes(mux, transport, gatewayAdministration)
	if len(processingRoutes) == 1 {
		registerProcessingRoutes(mux, transport, processingRoutes[0])
	}
	return mux, nil
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
