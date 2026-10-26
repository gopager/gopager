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

type Store struct {
	db            *sql.DB
	mu            sync.RWMutex
	migrations    fs.FS
	registered    bool
	migrationGate chan struct{}
	migrationOnce sync.Once
}

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

func (s *Store) Ping(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return platform.NewFailure(platform.CodeStorageUnavailable, err)
	}
	return nil
}

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
