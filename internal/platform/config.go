package platform

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// Role names a process role that Start composes into a running handle.
type Role string

// Supported process roles. RoleAPI serves the HTTP listener (health and
// application routes); RoleWorker runs the registered background Runners.
const (
	RoleAPI    Role = "api"
	RoleWorker Role = "worker"
)

// MigrationMode selects whether startup only verifies the database schema or
// may also apply pending migrations.
type MigrationMode string

// Migration modes. MigrationCheck (the default) refuses to start against a
// schema that is not exactly current; MigrationApply first applies pending
// migrations under the store's migration lock.
const (
	MigrationCheck MigrationMode = "check"
	MigrationApply MigrationMode = "apply"
)

// Config is the validated process configuration consumed by Start.
// DatabaseURL is private and never reported through Status.
type Config struct {
	Roles              []Role
	MigrationMode      MigrationMode
	DatabaseURL        string
	ListenAddress      string
	AllowInsecureDBTLS bool
}

// WithDefaults returns a copy of c with an empty MigrationMode replaced by
// MigrationCheck. Other fields are left unchanged.
func (c Config) WithDefaults() Config {
	if c.MigrationMode == "" {
		c.MigrationMode = MigrationCheck
	}
	return c
}

// Validate checks c after applying WithDefaults. It requires a known migration mode, a non-blank
// DatabaseURL and a non-empty, duplicate-free list of known roles; when RoleAPI is present ListenAddress must be an explicit
// host:port with a port in 1..65535. Every violation is a *Failure with
// CodeInvalidInput whose message does not reveal the offending value.
func (c Config) Validate() error {
	c = c.WithDefaults()
	if c.MigrationMode != MigrationCheck && c.MigrationMode != MigrationApply {
		return NewFailure(CodeInvalidInput, fmt.Errorf("unsupported migration mode"))
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return NewFailure(CodeInvalidInput, fmt.Errorf("missing database connection"))
	}
	if len(c.Roles) == 0 {
		return NewFailure(CodeInvalidInput, fmt.Errorf("roles are required"))
	}
	seen := make(map[Role]struct{}, len(c.Roles))
	for _, role := range c.Roles {
		if role != RoleAPI && role != RoleWorker {
			return NewFailure(CodeInvalidInput, fmt.Errorf("unknown role"))
		}
		if _, ok := seen[role]; ok {
			return NewFailure(CodeInvalidInput, fmt.Errorf("duplicate role"))
		}
		seen[role] = struct{}{}
	}
	if _, hasAPI := seen[RoleAPI]; hasAPI {
		if err := validateListenAddress(c.ListenAddress); err != nil {
			return NewFailure(CodeInvalidInput, err)
		}
	}
	return nil
}

func validateListenAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || port == "" {
		return fmt.Errorf("invalid API listen address")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("invalid API listen port")
	}
	return nil
}
