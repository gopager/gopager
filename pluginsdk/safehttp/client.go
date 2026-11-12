// Package safehttp is an HTTP client for plugins that call destinations an administrator
// configures. It resolves the destination itself, refuses loopback, private, link-local,
// carrier-grade NAT, unspecified, multicast and cloud-metadata addresses, and dials the
// address it checked so a changing DNS answer cannot redirect the connection afterwards.
// It also refuses privileged ports other than 80 and 443, verifies TLS against the URL host,
// bounds redirects and response bodies, and reports whether a failed request was completely
// written, which tells a plugin whether the receiver may have acted.
package safehttp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"strconv"
	"sync/atomic"
	"time"
)

const (
	// DefaultTimeout bounds one Do call when Policy.Timeout is zero.
	DefaultTimeout = 10 * time.Second
	// DefaultMaxResponseBytes bounds the stored response body when Policy.MaxResponseBytes is zero.
	DefaultMaxResponseBytes int64 = 64 << 10
	// MaxRedirects is the number of redirects followed when Policy.FollowRedirects is set.
	MaxRedirects = 3
)

// Resolver looks up the addresses of a host name. *net.Resolver implements it.
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// Policy configures a Client. The zero value is the strictest setting.
type Policy struct {
	// AllowPrivate lifts the address and port rules; every other protection stays.
	AllowPrivate bool
	// FollowRedirects follows up to MaxRedirects redirects. Otherwise a redirect response
	// is returned as it is.
	FollowRedirects bool
	// SensitiveHeaders are removed together with Authorization when a redirect changes
	// scheme, host or port.
	SensitiveHeaders []string
	// Timeout bounds the whole call including the body read. Zero means DefaultTimeout.
	Timeout time.Duration
	// MaxResponseBytes bounds the returned body. Zero means DefaultMaxResponseBytes.
	MaxResponseBytes int64
	// Resolver looks up host names. Nil means net.DefaultResolver.
	Resolver Resolver
}

// Response is a completed HTTP exchange.
type Response struct {
	// StatusCode is the HTTP status code.
	StatusCode int
	// Header holds the response headers.
	Header http.Header
	// Body holds at most Policy.MaxResponseBytes bytes of the response body.
	Body []byte
	// Truncated is true when the server sent more than Body holds.
	Truncated bool
}

// Client sends requests under a Policy. It is safe for concurrent use.
type Client struct {
	policy    Policy
	transport *http.Transport
	// dial opens a connection to a checked address; tests replace it.
	dial func(ctx context.Context, network, address string) (net.Conn, error)
}

// NewClient returns a Client enforcing policy.
func NewClient(policy Policy) *Client {
	if policy.Timeout <= 0 {
		policy.Timeout = DefaultTimeout
	}
	if policy.MaxResponseBytes <= 0 {
		policy.MaxResponseBytes = DefaultMaxResponseBytes
	}
	if policy.Resolver == nil {
		policy.Resolver = net.DefaultResolver
	}
	dialer := &net.Dialer{Timeout: policy.Timeout}
	client := &Client{policy: policy, dial: dialer.DialContext}
	client.transport = &http.Transport{
		// No proxy: the checked address must be the one that is contacted.
		Proxy:               nil,
		DialContext:         client.dialChecked,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout: policy.Timeout,
		// A fresh, freshly checked connection per request; the plugin sends few requests.
		DisableKeepAlives: true,
	}
	return client
}

// Do sends req and returns the completed exchange. It returns a *DeniedError when the
// Policy refuses the destination or a redirect, and a *RequestError for transport failures.
// The caller keeps ownership of req; Do does not modify it.
func (c *Client) Do(req *http.Request) (*Response, error) {
	ctx, cancel := context.WithTimeout(req.Context(), c.policy.Timeout)
	defer cancel()

	var wrote atomic.Bool
	trace := &httptrace.ClientTrace{WroteRequest: func(info httptrace.WroteRequestInfo) {
		if info.Err == nil {
			wrote.Store(true)
		}
	}}
	ctx = httptrace.WithClientTrace(ctx, trace)

	current := req.Clone(ctx)
	for hops := 0; ; hops++ {
		if err := checkScheme(current.URL); err != nil {
			return nil, err
		}
		resp, err := c.roundTrip(current)
		if err != nil {
			return nil, c.classify(err, wrote.Load())
		}
		if !c.policy.FollowRedirects || !isRedirect(resp.StatusCode) || resp.Header.Get("Location") == "" {
			return c.readResponse(resp, wrote.Load())
		}
		next, err := c.nextHop(current, resp, hops)
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}
		current = next
	}
}

// roundTrip performs one request without following redirects.
func (c *Client) roundTrip(req *http.Request) (*http.Response, error) {
	return c.transport.RoundTrip(req)
}

// classify turns a transport error into a DeniedError or a RequestError.
func (c *Client) classify(err error, written bool) error {
	if IsDenied(err) {
		return err
	}
	return &RequestError{Err: err, Written: written}
}

// readResponse reads the bounded body of resp and closes it.
func (c *Client) readResponse(resp *http.Response, written bool) (*Response, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.policy.MaxResponseBytes+1))
	if err != nil {
		return nil, &RequestError{Err: err, Written: written}
	}
	result := &Response{StatusCode: resp.StatusCode, Header: resp.Header}
	if int64(len(body)) > c.policy.MaxResponseBytes {
		body, result.Truncated = body[:c.policy.MaxResponseBytes], true
	}
	result.Body = body
	return result, nil
}

// nextHop builds the request that follows the redirect response resp of current, or refuses it.
func (c *Client) nextHop(current *http.Request, resp *http.Response, hops int) (*http.Request, error) {
	if hops >= MaxRedirects {
		return nil, &DeniedError{Reason: "too many redirects"}
	}
	target, err := current.URL.Parse(resp.Header.Get("Location"))
	if err != nil {
		return nil, &DeniedError{Reason: "invalid redirect location"}
	}
	if current.URL.Scheme == "https" && target.Scheme != "https" {
		return nil, &DeniedError{Reason: "redirect from https to http"}
	}
	next := current.Clone(current.Context())
	next.URL = target
	next.Host = ""
	if err := rewriteForRedirect(next, resp.StatusCode); err != nil {
		return nil, err
	}
	if sameAuthority(current.URL, target) {
		return next, nil
	}
	next.Header.Del("Authorization")
	for _, name := range c.policy.SensitiveHeaders {
		next.Header.Del(name)
	}
	return next, nil
}

// rewriteForRedirect applies the method and body rules of the redirect status to req.
func rewriteForRedirect(req *http.Request, status int) error {
	switch {
	case status == http.StatusSeeOther && req.Method != http.MethodHead,
		(status == http.StatusMovedPermanently || status == http.StatusFound) && req.Method == http.MethodPost:
		req.Method = http.MethodGet
		req.Body, req.GetBody, req.ContentLength = nil, nil, 0
		req.Header.Del("Content-Type")
		req.Header.Del("Content-Length")
	case req.Body != nil && req.Body != http.NoBody:
		if req.GetBody == nil {
			return &DeniedError{Reason: "redirect cannot replay the request body"}
		}
		body, err := req.GetBody()
		if err != nil {
			return &DeniedError{Reason: "redirect cannot replay the request body"}
		}
		req.Body = body
	}
	return nil
}

// isRedirect reports whether status is a redirect that carries a Location.
func isRedirect(status int) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	}
	return false
}

// sameAuthority reports whether a and b share scheme, host and port.
func sameAuthority(a, b *url.URL) bool {
	return a.Scheme == b.Scheme && a.Hostname() == b.Hostname() && effectivePort(a) == effectivePort(b)
}

// effectivePort returns the explicit port of u or the default of its scheme.
func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

// checkScheme refuses everything except http and https URLs with a host.
func checkScheme(u *url.URL) error {
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return &DeniedError{Reason: "unsupported URL"}
	}
	return nil
}

// dialChecked resolves the host of address, refuses it when any resolved address or the
// port is forbidden, and dials one of the checked addresses.
func (c *Client) dialChecked(ctx context.Context, network, address string) (net.Conn, error) {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return nil, &DeniedError{Reason: "invalid address"}
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return nil, &DeniedError{Reason: "invalid port"}
	}
	if !c.policy.AllowPrivate && deniedPort(port) {
		return nil, &DeniedError{Reason: "port not allowed"}
	}
	addresses, err := c.resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	var dialErr error
	for _, ip := range addresses {
		conn, err := c.dial(ctx, network, net.JoinHostPort(ip.String(), portText))
		if err == nil {
			return conn, nil
		}
		dialErr = errors.Join(dialErr, err)
	}
	return nil, dialErr
}

// resolve returns the addresses of host after checking every one of them against the Policy.
func (c *Client) resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	var addresses []netip.Addr
	if literal, err := netip.ParseAddr(host); err == nil {
		addresses = []netip.Addr{literal}
	} else {
		found, err := c.policy.Resolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolve host: %w", err)
		}
		for _, entry := range found {
			if ip, ok := netip.AddrFromSlice(entry.IP); ok {
				addresses = append(addresses, ip)
			}
		}
	}
	if len(addresses) == 0 {
		return nil, errors.New("host has no addresses")
	}
	if c.policy.AllowPrivate {
		return addresses, nil
	}
	for _, ip := range addresses {
		if reason := forbiddenReason(ip); reason != "" {
			return nil, &DeniedError{Reason: reason}
		}
	}
	return addresses, nil
}
