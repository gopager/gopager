package identity

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gopager/gopager/internal/authz"
	"golang.org/x/net/idna"
)

// HTTP names used by the browser session transport.
const (
	// SessionCookieName is the cookie carrying the session secret.
	SessionCookieName = "gopager_session"
	// CSRFHeaderName is the request header that must carry the session's CSRF
	// token on state-changing session requests.
	CSRFHeaderName = "X-CSRF-Token"
)

// HTTPTransport binds Cookie and Origin policy to the configured public origin.
// Upstream request transport and forwarding headers never select that policy.
type HTTPTransport struct {
	origin string
	secure bool
}

// NewHTTPTransport validates the deployment origin for either HTTP or HTTPS.
func NewHTTPTransport(publicOrigin string) (*HTTPTransport, error) {
	origin, secure, err := canonicalOrigin(publicOrigin, true)
	if err != nil {
		return nil, err
	}
	return &HTTPTransport{origin: origin, secure: secure}, nil
}

// SelectProof selects explicit Authorization before Cookies, without fallback on bad proof.
// Credential validity and current authority are subsequently checked by Resolver.
func (h *HTTPTransport) SelectProof(r *http.Request) (Proof, error) {
	if r == nil {
		return Proof{}, ErrInvalidInput
	}
	if values := r.Header.Values("Authorization"); len(values) != 0 {
		if len(values) != 1 {
			return Proof{}, ErrLoginRequired
		}
		scheme, secret, found := strings.Cut(values[0], " ")
		secret = strings.TrimLeft(secret, " ")
		if !found || !strings.EqualFold(scheme, "Bearer") {
			return Proof{}, ErrLoginRequired
		}
		if _, err := SecretHash(secret); err != nil {
			return Proof{}, err
		}
		return Proof{Kind: AccessKeyProof, Secret: secret}, nil
	}
	named := 0
	secret := ""
	for _, line := range r.Header.Values("Cookie") {
		for _, part := range strings.Split(line, ";") {
			name, value, found := strings.Cut(strings.TrimSpace(part), "=")
			if name == SessionCookieName {
				named++
				if !found {
					return Proof{}, ErrLoginRequired
				}
				secret = value
			}
		}
	}
	if named == 0 {
		return Proof{Kind: AnonymousProof}, nil
	}
	if named != 1 {
		return Proof{}, ErrLoginRequired
	}
	if _, err := SecretHash(secret); err != nil {
		return Proof{}, err
	}
	return Proof{Kind: SessionProof, Secret: secret}, nil
}

// CheckBrowserOrigin requires a single same-origin browser assertion.
// Login/Recovery routes use this before processing browser credential submissions.
func (h *HTTPTransport) CheckBrowserOrigin(r *http.Request) error {
	if r == nil {
		return ErrInvalidInput
	}
	values := r.Header.Values("Origin")
	if len(values) != 1 {
		return authz.ErrForbidden
	}
	origin, _, err := canonicalOrigin(values[0], false)
	if err != nil || origin != h.origin {
		return authz.ErrForbidden
	}
	return nil
}

// CheckStateChange enforces Session-bound CSRF and Origin on unsafe Cookie-authenticated methods.
// The authentication argument must come from the server's current Resolver result.
func (h *HTTPTransport) CheckStateChange(r *http.Request, authentication Authentication) error {
	if r == nil {
		return ErrInvalidInput
	}
	if authentication.Kind != AnonymousProof && authentication.Kind != SessionProof && authentication.Kind != AccessKeyProof {
		return ErrLoginRequired
	}
	if authentication.Kind != SessionProof || r.Method == http.MethodGet ||
		r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return nil
	}
	if err := h.CheckBrowserOrigin(r); err != nil {
		return err
	}
	values := r.Header.Values(CSRFHeaderName)
	if len(values) != 1 || authentication.CSRFToken == "" ||
		subtle.ConstantTimeCompare([]byte(values[0]), []byte(authentication.CSRFToken)) != 1 {
		return authz.ErrForbidden
	}
	return nil
}

// SessionCookie returns a host-only Session cookie, using public rather than upstream TLS.
func (h *HTTPTransport) SessionCookie(secret string, expiresAt time.Time) (*http.Cookie, error) {
	if _, err := SecretHash(secret); err != nil || expiresAt.IsZero() {
		return nil, ErrInvalidInput
	}
	return &http.Cookie{Name: SessionCookieName, Value: secret, Path: "/",
		Expires: expiresAt.UTC(), Secure: h.secure, HttpOnly: true, SameSite: http.SameSiteLaxMode}, nil
}

// ClearSessionCookie expires the same host-only cookie and retains its transport protections.
func (h *HTTPTransport) ClearSessionCookie() *http.Cookie {
	return &http.Cookie{Name: SessionCookieName, Path: "/", MaxAge: -1,
		Expires: time.Unix(1, 0).UTC(), Secure: h.secure, HttpOnly: true, SameSite: http.SameSiteLaxMode}
}

// canonicalOrigin compares browser origins without trusting request Host or forwarding headers.
func canonicalOrigin(value string, allowRoot bool) (string, bool, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Opaque != "" || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" ||
		strings.Contains(value, "#") || (parsed.Path != "" && (!allowRoot || parsed.Path != "/")) ||
		parsed.RawPath != "" || strings.HasSuffix(parsed.Host, ":") {
		return "", false, ErrInvalidInput
	}
	host := parsed.Hostname()
	if address, err := netip.ParseAddr(host); err == nil {
		if address.Zone() != "" {
			return "", false, ErrInvalidInput
		}
		host = address.String()
		if address.Is6() {
			host = "[" + host + "]"
		}
	} else {
		host, err = idna.Lookup.ToASCII(host)
		if err != nil || host == "" || strings.ContainsAny(host, ":[]") {
			return "", false, ErrInvalidInput
		}
		host = strings.ToLower(host)
	}
	port := parsed.Port()
	if port != "" {
		number, err := strconv.ParseUint(port, 10, 16)
		if err != nil {
			return "", false, ErrInvalidInput
		}
		port = strconv.FormatUint(number, 10)
		if (parsed.Scheme == "http" && port == "80") || (parsed.Scheme == "https" && port == "443") {
			port = ""
		}
	}
	if port != "" {
		host = net.JoinHostPort(strings.Trim(host, "[]"), port)
	}
	return parsed.Scheme + "://" + host, parsed.Scheme == "https", nil
}
