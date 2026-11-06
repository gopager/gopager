package identity

// settings collects the optional composition choices shared by Bootstrap and
// NewLoginService.
type settings struct {
	hasher PasswordHasher
}

// Option adjusts optional behaviour of Bootstrap and NewLoginService. Production
// composition passes none.
type Option func(*settings)

// WithPasswordHasher selects the password hasher. It exists so tests can use
// reduced Argon2id costs; the default is the production hasher.
func WithPasswordHasher(hasher PasswordHasher) Option {
	return func(s *settings) { s.hasher = hasher }
}

// newSettings applies options over the production defaults.
func newSettings(options []Option) settings {
	var s settings
	for _, option := range options {
		option(&s)
	}
	return s
}
