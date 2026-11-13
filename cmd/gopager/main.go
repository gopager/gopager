// Command gopager runs the GoPager server.
//
// Usage:
//
//	gopager -roles api,worker -listen-address HOST:PORT [flags]
//	gopager bootstrap -username NAME -actions A,B,... [flags] < password-file
//
// Server mode composes the requested roles on one PostgreSQL primary:
//
//   - api serves the HTTP listener: /health/live, /health/ready and the
//     /api/v1 application routes (authentication, management, Gateway intake
//     and administration, Processing).
//   - worker runs the background Processing dispatcher, the assignment
//     correction runner and the Gateway contact-loss monitor.
//
// Server flags: -roles (comma-separated, required),
// -listen-address (required with the api role), -migration-mode (check, the
// default, refuses to start unless the schema is exactly current; apply runs
// pending migrations first), -public-origin (the HTTP(S) origin used for
// cookie and Origin checks; required with the api role),
// -processing-concurrency (default 2) and -processing-poll-interval (default 5s)
// bound parallel Processing evaluation and the fallback poll for due work;
// -event-poll-interval (default 5s) is the fallback poll of event consumers;
// -live-replay-window (default 24h, positive) is how old a position may be when a live
// stream resumes from it;
// -processing-artifact-root (directory of operator-provisioned plugin
// executables; falls back to $GOPAGER_PROCESSING_ARTIFACT_ROOT),
// -login-attempts-per-account and -login-attempts-per-instance (per-minute
// login limits, positive), -search-attempts-per-minute (default 60, at least 1), -gateway-heartbeat-interval (default 60s),
// -gateway-contact-timeout (default 180s, at least the heartbeat interval) and
// -gateway-contact-scan-interval (default 15s, at most the contact timeout) for
// the worker's Gateway contact-loss monitor, -delivery-sends-per-minute (default 60) and
// -delivery-target-sends-per-minute (default 600) for Delivery sending admission per subscriber
// and per target instance, and -allow-insecure-db (accept a
// remote database without certificate-verified TLS). Positional arguments are
// rejected.
//
// The Secret store master key is read from $GOPAGER_SECRET_KEY_FILE (a file holding the Base64 key;
// wins) or $GOPAGER_SECRET_KEY, and $GOPAGER_SECRET_FILE_ROOT names the directory that file: Secret
// references are relative to.
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
	"time"

	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/httpapi"
	"github.com/gopager/gopager/internal/identity"
	ingestdomain "github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/persistence/migrations"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing/worker"
	"github.com/gopager/gopager/internal/readmodel"
)

var errInvalidRoles = errors.New("invalid role list")

type startupConfig struct {
	platform.Config
	PublicOrigin           string
	ProcessingArtifactRoot string
	// PluginSecrets holds the master key and Secret file root; it redacts the key.
	PluginSecrets      pluginSecretSettings
	LoginLimits        identity.LoginRateLimits
	SearchAttempts     int
	ProcessingDispatch worker.DispatchConfig
	EventPollInterval  time.Duration
	LiveReplayWindow   time.Duration
	GatewayContact     ingestdomain.ContactConfig
	Delivery           deliveryConfig
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
	// eventRuntime owns the wake hub that the live stream subscribes to.
	var eventRuntime *eventsRuntime
	var shared *core
	if slices.Contains(config.Roles, platform.RoleAPI) || slices.Contains(config.Roles, platform.RoleWorker) {
		shared, err = composeCore(store)
		if err != nil {
			return errors.Join(err, store.Close())
		}
		shared.deliveryConfig = config.Delivery
		processingRuntime, err = composeProcessingAPIRuntimeWith(ctx, shared,
			processingRuntimeSettings{ArtifactRoot: config.ProcessingArtifactRoot, Secrets: config.PluginSecrets})
		if err != nil {
			return errors.Join(err, store.Close())
		}
		defer func() {
			result = errors.Join(result, processingRuntime.Close(context.Background()))
		}()
		// The Delivery side is composed after the plugin host, so its Subscriptions, fan-out and
		// runner see the real plugin targets from their first use.
		if err := processingRuntime.bindDelivery(shared); err != nil {
			return errors.Join(err, store.Close())
		}
		eventRuntime, err = composeEventsRuntime(shared, slices.Contains(config.Roles, platform.RoleWorker), config.EventPollInterval)
		if err != nil {
			return errors.Join(err, store.Close())
		}
		shared.live = &liveConfig{wake: eventRuntime.hub, replayWindow: config.LiveReplayWindow}
		runners = append(runners, eventRuntime.runners...)
		stopEventHub := eventRuntime.startStandaloneHub(ctx)
		defer func() {
			result = errors.Join(result, stopEventHub())
		}()
	}
	if slices.Contains(config.Roles, platform.RoleWorker) {
		runtime, err := composeProcessingWorker(ctx, shared, processingRuntime.workerSource,
			processingRuntime.workerPublisher, config.ProcessingDispatch)
		if err != nil {
			return errors.Join(err, store.Close())
		}
		defer func() {
			result = errors.Join(result, runtime.Close(context.Background()))
		}()
		runners = append(runners, runtime.dispatcher)
		correctionRunner, err := composeCorrectionRunner(shared, processingRuntime.correctionReadmission)
		if err != nil {
			return errors.Join(err, store.Close())
		}
		runners = append(runners, correctionRunner)
		reprocessRunner, err := composeReprocessRunner(shared, processingRuntime.readmission())
		if err != nil {
			return errors.Join(err, store.Close())
		}
		runners = append(runners, reprocessRunner)
		contactMonitor, err := ingestdomain.NewContactMonitor(shared.store, shared.store, shared.intake, config.GatewayContact)
		if err != nil {
			return errors.Join(err, store.Close())
		}
		runners = append(runners, contactMonitor)
	}
	var api http.Handler
	if slices.Contains(config.Roles, platform.RoleAPI) {
		shared.searchAttempts = config.SearchAttempts
		api, err = composeApplicationAPI(config.PublicOrigin, config.LoginLimits, shared, nil, processingRuntime.routes)
		if err != nil {
			return errors.Join(err, store.Close())
		}
	}
	handle, err := platform.StartWithAPI(ctx, config.Config, migrations.Files, store, runners, api)
	if err != nil {
		return errors.Join(err, store.Close())
	}
	slog.Info("platform started", "roles", config.Roles)
	<-ctx.Done()
	return handle.Shutdown(context.Background())
}

// composeCorrectionRunner binds the historical correction runner to the shared matcher and to the
// intake store's Gateway mappings for Network re-resolution.
func composeCorrectionRunner(c *core, readmission assignments.ProcessingReadmission,
) (*assignments.CorrectionRunner, error) {
	networks, err := ingestdomain.NewNetworkReResolver(c.intake, c.rules)
	if err != nil {
		return nil, err
	}
	return assignments.NewCorrectionRunner(c.store, c.store, c.rules, networks, c.assignments, c.facts, readmission)
}

// parseConfig binds only the platform's explicit flags and secret environment input.
func parseConfig(args []string, getenv func(string) string) (startupConfig, error) {
	flags := flag.NewFlagSet("gopager", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	rolesValue := flags.String("roles", "", "Comma-separated roles: api,worker")
	mode := flags.String("migration-mode", string(platform.MigrationCheck), "Migration mode: check or apply")
	address := flags.String("listen-address", "", "Explicit API listen address")
	publicOrigin := flags.String("public-origin", "", "Public HTTP(S) origin for application Cookie and Origin policy")
	processingArtifactRoot := flags.String("processing-artifact-root", "", "Optional operator-provisioned plugin directory (or GOPAGER_PROCESSING_ARTIFACT_ROOT)")
	defaultLimits := identity.DefaultLoginRateLimits()
	loginAccount := flags.Int("login-attempts-per-account", defaultLimits.AccountAttempts, "Per-minute login attempt limit per normalized username")
	loginSource := flags.Int("login-attempts-per-source", defaultLimits.SourceAttempts, "Per-minute login attempt limit per client source (remote address host)")
	loginInstance := flags.Int("login-attempts-per-instance", defaultLimits.InstanceAttempts, "Per-minute instance login attempt limit")
	searchAttempts := flags.Int("search-attempts-per-minute", readmodel.DefaultSearchAttempts, "Per-minute Message search limit per access key, Principal or guest client source (at least 1)")
	processingConcurrency := flags.Int("processing-concurrency", worker.DefaultDispatchConcurrency, "Maximum Processing candidates evaluated in parallel")
	processingPoll := flags.Duration("processing-poll-interval", worker.DefaultDispatchPollInterval, "Fallback interval for discovering due Processing work (wake-ups arrive through notifications)")
	eventPoll := flags.Duration("event-poll-interval", events.DefaultPollInterval, "Fallback interval at which event consumers look for new facts (wake-ups arrive through notifications)")
	liveReplayWindow := flags.Duration("live-replay-window", readmodel.DefaultLiveReplayWindow, "How old a live stream position may be when a stream resumes from it (positive); keep domain events at least this long")
	heartbeatInterval := flags.Duration("gateway-heartbeat-interval", ingestdomain.DefaultHeartbeatInterval, "Interval in which Gateways are expected to send a heartbeat")
	contactTimeout := flags.Duration("gateway-contact-timeout", ingestdomain.DefaultContactTimeout, "Silence after which an active or degraded Gateway is contact-lost (at least the heartbeat interval)")
	contactScan := flags.Duration("gateway-contact-scan-interval", ingestdomain.DefaultContactScan, "Interval of the worker's Gateway contact-loss scan (at most the contact timeout)")
	deliverySends := flags.Int("delivery-sends-per-minute", delivery.DefaultSubscriberSendsPerMinute, "Per-minute Delivery sends and send requests per subscriber (at least 1)")
	deliveryTargetSends := flags.Int("delivery-target-sends-per-minute", delivery.DefaultTargetSendsPerMinute, "Per-minute Delivery sends per target instance; a smaller limit of the target lowers it (at least 1)")
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
		Roles: roles, MigrationMode: platform.MigrationMode(*mode),
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
	if *loginAccount <= 0 || *loginSource <= 0 || *loginInstance <= 0 {
		return startupConfig{}, platform.NewFailure(platform.CodeInvalidInput, identity.ErrInvalidInput)
	}
	if *searchAttempts < 1 {
		return startupConfig{}, platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid search attempt limit"))
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
	secrets, err := parsePluginSecrets(getenv)
	if err != nil {
		return startupConfig{}, err
	}
	// Not WithDefaults: an explicit zero must be rejected, not silently replaced.
	dispatch := worker.DispatchConfig{Concurrency: *processingConcurrency, PollInterval: *processingPoll}
	if err := dispatch.Validate(); err != nil {
		return startupConfig{}, err
	}
	if *eventPoll <= 0 {
		return startupConfig{}, platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid event poll interval"))
	}
	if *liveReplayWindow <= 0 {
		return startupConfig{}, platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid live replay window"))
	}
	contact := ingestdomain.ContactConfig{HeartbeatInterval: *heartbeatInterval, Timeout: *contactTimeout,
		ScanInterval: *contactScan}
	if err := contact.Validate(); err != nil {
		return startupConfig{}, err
	}
	if *deliverySends < 1 || *deliveryTargetSends < 1 {
		return startupConfig{}, platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid Delivery sending limit"))
	}
	deliveries := deliveryConfig{limits: delivery.AdmissionLimits{Subscriber: *deliverySends, Target: *deliveryTargetSends}}
	return startupConfig{Config: config, PublicOrigin: *publicOrigin,
		ProcessingArtifactRoot: *processingArtifactRoot, PluginSecrets: secrets, SearchAttempts: *searchAttempts,
		LoginLimits:        identity.LoginRateLimits{AccountAttempts: *loginAccount, SourceAttempts: *loginSource, InstanceAttempts: *loginInstance},
		ProcessingDispatch: dispatch, EventPollInterval: *eventPoll, LiveReplayWindow: *liveReplayWindow, GatewayContact: contact, Delivery: deliveries}, nil
}

// composeApplicationAPI binds the shared services to the HTTP listener. A non-nil networkOwner
// replaces the production Network owner for address-rule management (tests only); production
// passes nil and uses the shared rule service.
func composeApplicationAPI(publicOrigin string, limits identity.LoginRateLimits, c *core, networkOwner assignments.NetworkOwner,
	processingRoutes ...httpapi.ProcessingRoutes) (http.Handler, error) {
	if len(processingRoutes) > 1 {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("multiple Processing API route sets"))
	}
	transport, err := identity.NewHTTPTransport(publicOrigin)
	if err != nil {
		return nil, err
	}
	login, err := identity.NewLoginService(c.store, c.resolver, c.identity, c.facts, limits, identityOptions...)
	if err != nil {
		return nil, err
	}
	authHandler, err := identity.NewHTTPHandler(transport, login)
	if err != nil {
		return nil, err
	}
	ruleService := c.rules
	if networkOwner != nil {
		if ruleService, err = c.newRuleService(networkOwner); err != nil {
			return nil, err
		}
	}
	intakeService, err := ingestdomain.NewService(c.store, c.store, c.intake, ruleService)
	if err != nil {
		return nil, err
	}
	gatewayAdministration, err := ingestdomain.NewGatewayAdministration(c.store, c.resolver, c.store,
		c.intake, c.intake, c.receipts, c.facts, c.assignments)
	if err != nil {
		return nil, err
	}
	networkAdministration, err := ingestdomain.NewNetworkAdministration(c.store, c.resolver, c.store, c.intake, c.receipts)
	if err != nil {
		return nil, err
	}
	preauthenticator, err := identity.NewPreauthenticator(c.store, c.resolver)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/auth/", authHandler)
	httpapi.RegisterManagementRoutes(mux, transport, preauthenticator, c.unitService, ruleService)
	if err := registerQueryRoutes(mux, transport, preauthenticator, c); err != nil {
		return nil, err
	}
	if err := registerLiveRoutes(mux, transport, preauthenticator, c); err != nil {
		return nil, err
	}
	if err := registerDeliveryRoutes(mux, transport, preauthenticator, c); err != nil {
		return nil, err
	}
	if err := registerAttributeRoutes(mux, transport, preauthenticator, c); err != nil {
		return nil, err
	}
	if err := registerPresentationRoutes(mux, transport, preauthenticator, c); err != nil {
		return nil, err
	}
	httpapi.RegisterIngestRoutes(mux, intakeService)
	httpapi.RegisterGatewayManagementRoutes(mux, transport, preauthenticator, gatewayAdministration)
	httpapi.RegisterNetworkManagementRoutes(mux, transport, preauthenticator, networkAdministration)
	if len(processingRoutes) == 1 {
		httpapi.RegisterProcessingRoutes(mux, transport, preauthenticator, processingRoutes[0])
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
