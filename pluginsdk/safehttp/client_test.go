package safehttp

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// mapResolver answers from a fixed host-to-address table.
type mapResolver map[string][]string

func (m mapResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	texts, ok := m[host]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	var out []net.IPAddr
	for _, text := range texts {
		out = append(out, net.IPAddr{IP: net.ParseIP(text)})
	}
	return out, nil
}

// redirectingClient returns a client whose dials all go to target, so names that resolve to
// public addresses reach a local server while every policy check still runs.
func redirectingClient(policy Policy, target string) *Client {
	client := NewClient(policy)
	client.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, target)
	}
	return client
}

func get(t *testing.T, client *Client, url string) (*Response, error) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	return client.Do(req)
}

func TestDeniedDestinationsMakeNoConnection(t *testing.T) {
	t.Parallel()
	var connections atomic.Int32
	resolver := mapResolver{
		"meta.example":   {"169.254.169.254"},
		"loop.example":   {"127.0.0.1"},
		"lan.example":    {"10.0.0.1"},
		"ula.example":    {"fd00:ec2::254"},
		"mapped.example": {"::ffff:127.0.0.1"},
		"cgnat.example":  {"100.64.0.9"},
		"mixed.example":  {"93.184.216.34", "10.0.0.1"},
	}
	client := NewClient(Policy{Resolver: resolver})
	client.dial = func(context.Context, string, string) (net.Conn, error) {
		connections.Add(1)
		return nil, errors.New("must not dial")
	}
	urls := []string{
		"http://meta.example/", "http://loop.example/", "http://lan.example/", "http://ula.example/",
		"http://mapped.example/", "http://cgnat.example/", "http://mixed.example/",
		"http://127.0.0.1/", "http://[::1]/", "http://169.254.169.254/latest/meta-data",
		"http://93.184.216.34:22/", "http://93.184.216.34:1023/", "https://meta.example/",
		"ftp://93.184.216.34/", "http:///nohost",
	}
	for _, url := range urls {
		_, err := get(t, client, url)
		var denied *DeniedError
		if !errors.As(err, &denied) {
			t.Errorf("%s: want DeniedError, got %v", url, err)
		}
		if Written(err) {
			t.Errorf("%s: a denied request is not written", url)
		}
	}
	if connections.Load() != 0 {
		t.Fatalf("denied destinations opened %d connections", connections.Load())
	}
}

func TestAllowedPublicDestinationsAndPorts(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer server.Close()
	client := redirectingClient(Policy{Resolver: mapResolver{"pub.example": {"93.184.216.34", "2606:4700:4700::1111"}}}, server.Listener.Addr().String())
	for _, url := range []string{"http://pub.example/", "http://pub.example:80/", "http://pub.example:8080/", "http://93.184.216.34/"} {
		resp, err := get(t, client, url)
		if err != nil || string(resp.Body) != "ok" {
			t.Errorf("%s: %v %v", url, resp, err)
		}
	}
}

func TestAllowPrivateLiftsAddressAndPortRules(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	client := NewClient(Policy{AllowPrivate: true})
	resp, err := get(t, client, server.URL)
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("loopback with AllowPrivate: %v %v", resp, err)
	}
	if _, err := get(t, NewClient(Policy{}), server.URL); !IsDenied(err) {
		t.Fatalf("loopback without AllowPrivate: %v", err)
	}
	// Only the address and port rules are lifted: other schemes stay refused.
	if _, err := get(t, client, "ftp://127.0.0.1/"); !IsDenied(err) {
		t.Fatalf("ftp must stay refused: %v", err)
	}
}

func TestRedirectsAreNotFollowedByDefault(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	}))
	defer server.Close()
	resp, err := get(t, NewClient(Policy{AllowPrivate: true}), server.URL)
	if err != nil || resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/elsewhere" || hits.Load() != 1 {
		t.Fatalf("got %v %v hits=%d", resp, err, hits.Load())
	}
}

func TestFollowedRedirectDropsCredentialsOnAuthorityChange(t *testing.T) {
	t.Parallel()
	var gotAuth, gotSignature, gotOther string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotSignature, gotOther = r.Header.Get("Authorization"), r.Header.Get("X-Signature"), r.Header.Get("X-Other")
	}))
	defer target.Close()
	var sameAuth string
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cross":
			http.Redirect(w, r, target.URL+"/landing", http.StatusFound)
		case "/same":
			http.Redirect(w, r, "/landing", http.StatusFound)
		default:
			sameAuth = r.Header.Get("Authorization")
		}
	}))
	defer origin.Close()
	client := NewClient(Policy{AllowPrivate: true, FollowRedirects: true, SensitiveHeaders: []string{"x-signature"}})
	do := func(path string) {
		req, _ := http.NewRequest(http.MethodGet, origin.URL+path, nil)
		req.Header.Set("Authorization", "Bearer s3cret")
		req.Header.Set("X-Signature", "sig")
		req.Header.Set("X-Other", "keep")
		if _, err := client.Do(req); err != nil {
			t.Fatal(err)
		}
	}
	do("/cross")
	if gotAuth != "" || gotSignature != "" || gotOther != "keep" {
		t.Errorf("cross-authority redirect kept credentials: %q %q %q", gotAuth, gotSignature, gotOther)
	}
	do("/same")
	if sameAuth != "Bearer s3cret" {
		t.Errorf("same-authority redirect lost Authorization: %q", sameAuth)
	}
}

func TestFollowedRedirectLimitAndRevalidation(t *testing.T) {
	t.Parallel()
	var hops atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops.Add(1)
		http.Redirect(w, r, "/again", http.StatusFound)
	}))
	defer server.Close()
	client := NewClient(Policy{AllowPrivate: true, FollowRedirects: true})
	_, err := get(t, client, server.URL)
	if !IsDenied(err) || hops.Load() != MaxRedirects+1 {
		t.Fatalf("want denial after %d hops, got %v after %d", MaxRedirects, err, hops.Load())
	}

	// Every hop is revalidated: a public first hop redirecting to the metadata address is refused.
	meta := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest", http.StatusFound)
	}))
	defer meta.Close()
	strict := redirectingClient(Policy{FollowRedirects: true, Resolver: mapResolver{"pub.example": {"93.184.216.34"}}}, meta.Listener.Addr().String())
	if _, err := get(t, strict, "http://pub.example/"); !IsDenied(err) {
		t.Fatalf("redirect to metadata address: %v", err)
	}
}

func TestRedirectFromHTTPSToHTTPIsRefused(t *testing.T) {
	t.Parallel()
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer plain.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL, http.StatusFound)
	}))
	defer secure.Close()
	client := NewClient(Policy{AllowPrivate: true, FollowRedirects: true})
	client.transport.TLSClientConfig = secure.Client().Transport.(*http.Transport).TLSClientConfig
	_, err := get(t, client, secure.URL)
	var denied *DeniedError
	if !errors.As(err, &denied) || !strings.Contains(denied.Reason, "https to http") {
		t.Fatalf("got %v", err)
	}
}

func TestTLSIsVerifiedAgainstTheURLHost(t *testing.T) {
	t.Parallel()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer secure.Close()
	// The default roots do not trust the test CA: the handshake must fail, not be skipped.
	_, err := get(t, NewClient(Policy{AllowPrivate: true}), secure.URL)
	var requestErr *RequestError
	if !errors.As(err, &requestErr) || Written(err) {
		t.Fatalf("untrusted certificate: %v", err)
	}
	client := NewClient(Policy{AllowPrivate: true})
	client.transport.TLSClientConfig = secure.Client().Transport.(*http.Transport).TLSClientConfig
	if _, err := get(t, client, secure.URL); err != nil {
		t.Fatalf("trusted certificate: %v", err)
	}
	// A certificate for another name is refused even when its CA is trusted.
	config := secure.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	config.MinVersion = tls.VersionTLS12
	client.transport.TLSClientConfig = config
	wrong := redirectingClient(Policy{Resolver: mapResolver{"other.example": {"93.184.216.34"}}}, secure.Listener.Addr().String())
	wrong.transport.TLSClientConfig = config
	if _, err := get(t, wrong, "https://other.example/"); err == nil {
		t.Fatal("certificate for another host name was accepted")
	}
}

func TestBodyIsBounded(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", 100)))
	}))
	defer server.Close()
	resp, err := get(t, NewClient(Policy{AllowPrivate: true, MaxResponseBytes: 10}), server.URL)
	if err != nil || len(resp.Body) != 10 || !resp.Truncated {
		t.Fatalf("got %v %v", resp, err)
	}
	resp, err = get(t, NewClient(Policy{AllowPrivate: true, MaxResponseBytes: 100}), server.URL)
	if err != nil || len(resp.Body) != 100 || resp.Truncated {
		t.Fatalf("exact fit: %v %v", resp, err)
	}
}

func TestWrittenClassification(t *testing.T) {
	t.Parallel()
	// A server that reads the whole request and then closes without answering.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			buf := make([]byte, 4096)
			_ = conn.SetReadDeadline(time.Now().Add(time.Second))
			_, _ = conn.Read(buf)
			_ = conn.Close()
		}
	}()
	client := NewClient(Policy{AllowPrivate: true})
	req, _ := http.NewRequest(http.MethodPost, "http://"+listener.Addr().String(), strings.NewReader("payload"))
	_, err = client.Do(req)
	if err == nil || !Written(err) || IsDenied(err) {
		t.Fatalf("closed after request: written=%v err=%v", Written(err), err)
	}

	// Nothing listens: the request is never written.
	closed, _ := net.Listen("tcp", "127.0.0.1:0")
	address := closed.Addr().String()
	_ = closed.Close()
	req, _ = http.NewRequest(http.MethodPost, "http://"+address, strings.NewReader("payload"))
	_, err = client.Do(req)
	if err == nil || Written(err) || IsDenied(err) {
		t.Fatalf("connection refused: written=%v err=%v", Written(err), err)
	}

	// An unresolvable name is not written either.
	_, err = get(t, NewClient(Policy{Resolver: mapResolver{}}), "http://missing.example/")
	if err == nil || Written(err) || IsDenied(err) {
		t.Fatalf("dns failure: written=%v err=%v", Written(err), err)
	}
}

func TestTimeoutAfterWriteIsWritten(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		<-release
	}))
	defer server.Close()
	defer close(release)
	req, _ := http.NewRequest(http.MethodPost, server.URL, strings.NewReader("x"))
	_, err := NewClient(Policy{AllowPrivate: true, Timeout: 200 * time.Millisecond}).Do(req)
	if err == nil || !Written(err) {
		t.Fatalf("timeout after write: written=%v err=%v", Written(err), err)
	}
}

func TestSeeOtherRedirectBecomesGetWithoutBody(t *testing.T) {
	t.Parallel()
	var method, body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/end", http.StatusSeeOther)
			return
		}
		method = r.Method
		buf := make([]byte, 16)
		n, _ := r.Body.Read(buf)
		body = string(buf[:n])
	}))
	defer server.Close()
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/start", strings.NewReader("data"))
	if _, err := NewClient(Policy{AllowPrivate: true, FollowRedirects: true}).Do(req); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodGet || body != "" {
		t.Fatalf("got %s %q", method, body)
	}
}
