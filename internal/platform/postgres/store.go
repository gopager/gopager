package postgres

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"io/fs"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/gopager/gopager/internal/platform"
)

// Store is the PostgreSQL platform.Store. It is safe for concurrent use; its
// connection pool is limited to 10 open connections. After a successful Migrate it
// remembers the applied schema version so that every transaction can verify it
// with one statement instead of repeating the locked migration check.
type Store struct {
	db            *sql.DB
	mu            sync.RWMutex
	migrations    fs.FS
	registered    bool
	schemaVersion int64
	migrationGate chan struct{}
	migrationOnce sync.Once
}

// NewStore parses databaseURL (CodeInvalidInput on parse failure) and opens a
// lazily connecting pool. A host or fallback host that is neither loopback nor
// a Unix socket and does not verify the server certificate counts as an
// insecure remote; that fails with CodeInvalidInput unless allowInsecureRemote
// is set. The boolean result reports whether such an insecure remote was
// accepted, so the caller can warn about it.
func NewStore(databaseURL string, allowInsecureRemote bool) (*Store, bool, error) {
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, false, platform.NewFailure(platform.CodeInvalidInput, err)
	}
	insecureRemote := !isLocalHost(config.Host) && !tlsConfigVerifiesServer(config.TLSConfig)
	for _, fallback := range config.Fallbacks {
		if fallback == nil || (!isLocalHost(fallback.Host) && !tlsConfigVerifiesServer(fallback.TLSConfig)) {
			insecureRemote = true
		}
	}
	if insecureRemote && !allowInsecureRemote {
		return nil, false, platform.NewFailure(platform.CodeInvalidInput, fmt.Errorf("remote database requires certificate-verified TLS"))
	}
	db := stdlib.OpenDB(*config)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	return &Store{db: db}, insecureRemote, nil
}

// Ping verifies database connectivity, failing with CodeStorageUnavailable.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return platform.NewFailure(platform.CodeStorageUnavailable, err)
	}
	return nil
}

// CheckWriteAuthority fails with CodeAuthorityUnavailable unless the session is
// connected to a primary (not in recovery) whose default transaction mode is
// read-write.
func (s *Store) CheckWriteAuthority(ctx context.Context) error {
	var primary bool
	var readOnly bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT NOT pg_is_in_recovery(),
		       current_setting('transaction_read_only')::boolean
	`).Scan(&primary, &readOnly); err != nil {
		return platform.NewFailure(platform.CodeAuthorityUnavailable, err)
	}
	if !primary || readOnly {
		return platform.NewFailure(platform.CodeAuthorityUnavailable, fmt.Errorf("database is not a writable primary"))
	}
	return nil
}

// Close closes the pool, failing with CodeStorageUnavailable. The Store must not
// be used afterwards.
func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return platform.NewFailure(platform.CodeStorageUnavailable, err)
	}
	return nil
}

func isLocalHost(host string) bool {
	host = strings.Trim(host, "[]")
	if strings.HasPrefix(host, "/") || strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func tlsConfigVerifiesServer(config *tls.Config) bool {
	if config == nil {
		return false
	}
	return !config.InsecureSkipVerify ||
		config.VerifyConnection != nil ||
		config.VerifyPeerCertificate != nil
}
