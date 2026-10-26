package platform

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

type Role string

const (
	RoleAPI    Role = "api"
	RoleWorker Role = "worker"
)

type MigrationMode string

const (
	MigrationCheck MigrationMode = "check"
	MigrationApply MigrationMode = "apply"
)

type Config struct {
	Profile            string
	Roles              []Role
	MigrationMode      MigrationMode
	DatabaseURL        string
	ListenAddress      string
	AllowInsecureDBTLS bool
}

func (c Config) WithDefaults() Config {
	if c.MigrationMode == "" {
		c.MigrationMode = MigrationCheck
	}
	return c
}

func (c Config) Validate() error {
	c = c.WithDefaults()
	if c.Profile != "full" {
		return NewFailure(CodeInvalidInput, fmt.Errorf("unsupported profile"))
	}
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
