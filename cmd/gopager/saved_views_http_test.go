package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
)

// credentials are the transport proofs of one test caller.
type credentials struct {
	bearer string
	cookie string
	csrf   string
}

// newSavedViewFixture composes the API with the given search limit (0 selects the default).
func newSavedViewFixture(t *testing.T, searchAttempts int) *queryFixture {
	t.Helper()
	store := newManagementStore(t)
	backend := httptest.NewUnstartedServer(nil)
	t.Cleanup(backend.Close)
	origin := "http://" + backend.Listener.Addr().String()
	shared := mustComposeCore(t, store)
	shared.searchAttempts = searchAttempts
	handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), shared, nil)
	if err != nil {
		t.Fatal(err)
	}
	backend.Config.Handler = handler
	backend.Start()
	f := &queryFixture{t: t, store: store, origin: origin, client: backend.Client(), gateway: uuid.New(),
		network: uuid.New(), agency: seedAgency(t, store)}
	f.exec(`INSERT INTO gateways (id, name) VALUES ($1, 'gw')`, f.gateway)
	f.exec(`INSERT INTO networks (id, name, protocol, agency_id) VALUES ($1, 'Net', 'pocsag', $2)`, f.network, f.agency)
	return f
}

// grantAccount stores grants and field permissions for an existing account.
func (f *queryFixture) grantAccount(principal uuid.UUID, actions []string, scope authz.Scope, fields []string) {
	f.t.Helper()
	scopeJSON, _ := json.Marshal(scope)
	actionsJSON, _ := json.Marshal(actions)
	fieldsJSON, _ := json.Marshal(fields)
	f.exec(`INSERT INTO identity_grants (id, subject_id, subject_kind, actions, scope) VALUES ($1, $2, 'principal', $3::jsonb, $4::jsonb)`,
		uuid.New(), principal, actionsJSON, scopeJSON)
	f.exec(`INSERT INTO identity_field_permissions (id, subject_id, subject_kind, field_names) VALUES ($1, $2, 'principal', $3::jsonb)`,
		uuid.New(), principal, fieldsJSON)
}

// human seeds a human account with grants and returns its Principal.
func (f *queryFixture) human(actions []string, scope authz.Scope, fields []string) uuid.UUID {
	f.t.Helper()
	principal := uuid.New()
	f.exec(`INSERT INTO identity_accounts (id, kind, state, created_at) VALUES ($1, 'human', 'active', now())`, principal)
	f.grantAccount(principal, actions, scope, fields)
	return principal
}

// session logs a seeded human account in and returns the cookie proof.
func (f *queryFixture) session(principal uuid.UUID) credentials {
	f.t.Helper()
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		f.t.Fatal(err)
	}
	secret := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(secret))
	csrf := uuid.NewString()
	f.exec(`INSERT INTO identity_credentials (id, account_id, kind, secret_hash, csrf_token, created_at, last_used_at, expires_at)
		VALUES ($1, $2, 'session', $3, $4, now(), now(), now() + interval '1 day')`, uuid.New(), principal, hash[:], csrf)
	return credentials{cookie: identity.SessionCookieName + "=" + secret, csrf: csrf}
}

// key seeds an access key of the account restricted to the actions and fields given.
func (f *queryFixture) key(principal uuid.UUID, actions, fields []string) credentials {
	f.t.Helper()
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		f.t.Fatal(err)
	}
	secret := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(secret))
	restrictions, _ := json.Marshal(authz.Restrictions{Actions: actions, Scopes: []authz.Scope{{Kind: "all"}}, Fields: fields})
	f.exec(`INSERT INTO identity_credentials (id, account_id, kind, secret_hash, restrictions, created_at, last_used_at)
		VALUES ($1, $2, 'accessKey', $3, $4::jsonb, now(), now())`, uuid.New(), principal, hash[:], restrictions)
	return credentials{bearer: "Bearer " + secret}
}

// keyCaller is a technical account with a restricted key; it returns its proof and Principal.
func (f *queryFixture) keyCaller(actions []string, scope authz.Scope, fields []string) (credentials, uuid.UUID) {
	bearer, principal := f.caller(actions, scope, fields)
	return credentials{bearer: bearer}, principal
}

// guestGrant gives the guest account grants.
func (f *queryFixture) guestGrant(actions []string, scope authz.Scope, fields []string) {
	f.t.Helper()
	var guest uuid.UUID
	err := f.store.WithinTransaction(f.t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT id FROM identity_accounts WHERE kind = 'guest'`).Scan(&guest)
	})
	if err != nil {
		f.t.Fatal(err)
	}
	f.grantAccount(guest, actions, scope, fields)
}

// scalar runs a query that returns one integer.
func (f *queryFixture) scalar(statement string, args ...any) int {
	f.t.Helper()
	var value int
	err := f.store.WithinTransaction(f.t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		err = executor.QueryRowContext(tx.Context(), statement, args...).Scan(&value)
		if err == sql.ErrNoRows {
			value = -1
			return nil
		}
		return err
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return value
}

// call sends a request with the proofs and returns the status, headers and body.
func (f *queryFixture) call(who credentials, method, path, body string) (int, http.Header, []byte) {
	f.t.Helper()
	request, err := http.NewRequestWithContext(f.t.Context(), method, f.origin+path, strings.NewReader(body))
	if err != nil {
		f.t.Fatal(err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet {
		request.Header.Set("Origin", f.origin)
	}
	if who.bearer != "" {
		request.Header.Set("Authorization", who.bearer)
	}
	if who.cookie != "" {
		request.Header.Set("Cookie", who.cookie)
		request.Header.Set(identity.CSRFHeaderName, who.csrf)
	}
	response, err := f.client.Do(request)
	if err != nil {
		f.t.Fatal(err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		f.t.Fatal(err)
	}
	return response.StatusCode, response.Header, payload
}

// viewBody is the saved view response envelope.
type viewBody struct {
	Data struct {
		ID                 uuid.UUID       `json:"id"`
		Revision           string          `json:"revision"`
		Ownership          string          `json:"ownership"`
		Name               string          `json:"name"`
		Definition         json.RawMessage `json:"definition"`
		UnavailableFilters []string        `json:"unavailableFilters"`
	} `json:"data"`
	Code string `json:"code"`
}

// view sends a saved view request and expects the status.
func (f *queryFixture) view(who credentials, method, path, body string, status int) viewBody {
	f.t.Helper()
	got, _, payload := f.call(who, method, path, body)
	if got != status {
		f.t.Fatalf("%s %s = %d, want %d: %s", method, path, got, status, payload)
	}
	var parsed viewBody
	if err := json.Unmarshal(payload, &parsed); err != nil {
		f.t.Fatal(err)
	}
	return parsed
}

// viewList reads GET /saved-views and returns the names and total.
func (f *queryFixture) viewList(who credentials, query string) ([]string, int) {
	f.t.Helper()
	status, _, payload := f.call(who, http.MethodGet, "/api/v1/saved-views"+query, "")
	if status != http.StatusOK {
		f.t.Fatalf("GET /saved-views = %d: %s", status, payload)
	}
	var parsed struct {
		Data struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		f.t.Fatal(err)
	}
	names := []string{}
	for _, item := range parsed.Data.Items {
		names = append(names, item.Name)
	}
	return names, parsed.Data.Total
}

func createBody(command uuid.UUID, ownership, name, definition string) string {
	return fmt.Sprintf(`{"schemaVersion":1,"commandId":%q,"ownership":%q,"name":%q,"definition":%s}`,
		command, ownership, name, definition)
}

func updateBody(revision, name, definition string) string {
	return fmt.Sprintf(`{"schemaVersion":1,"expectedRevision":%q,"name":%q,"definition":%s}`, revision, name, definition)
}

func deleteBody(revision string) string {
	return fmt.Sprintf(`{"schemaVersion":1,"expectedRevision":%q}`, revision)
}

const emptyDefinition = `{"schemaVersion":1,"filters":{}}`

// TestSavedViewsPersonalPrivacy covers F-103, revision conflicts and command replay.
func TestSavedViewsPersonalPrivacy(t *testing.T) {
	t.Parallel()
	f := newSavedViewFixture(t, 0)
	read := []string{"message.read"}
	all := authz.Scope{Kind: "all"}
	fields := []string{"payload", "unit"}
	u1, _ := f.keyCaller(read, all, fields)
	u2, _ := f.keyCaller(read, all, fields)
	manager, _ := f.keyCaller([]string{"saved-view.manage-global"}, all, fields)
	f.guestGrant(read, all, fields)
	guest := credentials{}

	command := uuid.New()
	definition := `{"schemaVersion":1,"filters":{"text":"Brand","protocols":["pocsag"]}}`
	created := f.view(u1, http.MethodPost, "/api/v1/saved-views", createBody(command, "personal", "Mine", definition), http.StatusCreated)
	if created.Data.Revision != "1" || created.Data.Ownership != "personal" || len(created.Data.UnavailableFilters) != 0 {
		t.Fatalf("created = %+v", created.Data)
	}
	path := "/api/v1/saved-views/" + created.Data.ID.String()

	// owner lists and reads.
	{
		if names, total := f.viewList(u1, ""); len(names) != 1 || names[0] != "Mine" || total != 1 {
			t.Fatalf("list = %v/%d", names, total)
		}
		if got := f.view(u1, http.MethodGet, path, "", http.StatusOK); got.Data.Name != "Mine" {
			t.Fatalf("read = %+v", got.Data)
		}
	}
	// others neither list nor reach it.
	{
		for name, who := range map[string]credentials{"other reader": u2, "global manager": manager, "guest": guest} {
			if names, total := f.viewList(who, ""); len(names) != 0 || total != 0 {
				t.Errorf("%s list = %v/%d", name, names, total)
			}
		}
		for _, who := range []credentials{u2, manager} {
			f.view(who, http.MethodGet, path, "", http.StatusNotFound)
			f.view(who, http.MethodPut, path, updateBody("1", "Hijack", emptyDefinition), http.StatusNotFound)
			f.view(who, http.MethodDelete, path, deleteBody("1"), http.StatusNotFound)
		}
		f.view(guest, http.MethodGet, path, "", http.StatusNotFound)
	}
	// the guest cannot create.
	{
		got := f.view(guest, http.MethodPost, "/api/v1/saved-views", createBody(uuid.New(), "personal", "G", emptyDefinition), http.StatusUnauthorized)
		if got.Code != "LOGIN_REQUIRED" {
			t.Fatalf("code = %q", got.Code)
		}
	}
	// a caller without message.read cannot create a personal view.
	{
		got := f.view(manager, http.MethodPost, "/api/v1/saved-views", createBody(uuid.New(), "personal", "M", emptyDefinition), http.StatusForbidden)
		if got.Code != "FORBIDDEN" {
			t.Fatalf("code = %q", got.Code)
		}
	}
	// command replay returns the same view.
	{
		replay := f.view(u1, http.MethodPost, "/api/v1/saved-views", createBody(command, "personal", "Mine", definition), http.StatusCreated)
		if replay.Data.ID != created.Data.ID || replay.Data.Revision != "1" {
			t.Fatalf("replay = %+v", replay.Data)
		}
		if got := f.scalar(`SELECT count(*) FROM saved_views`); got != 1 {
			t.Fatalf("stored views = %d", got)
		}
		conflict := f.view(u1, http.MethodPost, "/api/v1/saved-views", createBody(command, "personal", "Other", definition), http.StatusConflict)
		if conflict.Code != "IDEMPOTENCY_CONFLICT" {
			t.Fatalf("code = %q", conflict.Code)
		}
	}
	// update, stale revision and delete.
	{
		updated := f.view(u1, http.MethodPut, path, updateBody("1", "Renamed", emptyDefinition), http.StatusOK)
		if updated.Data.Revision != "2" || updated.Data.Name != "Renamed" {
			t.Fatalf("updated = %+v", updated.Data)
		}
		if got := f.view(u1, http.MethodPut, path, updateBody("1", "Stale", emptyDefinition), http.StatusConflict); got.Code != "REVISION_CONFLICT" {
			t.Fatalf("code = %q", got.Code)
		}
		if got := f.view(u1, http.MethodDelete, path, deleteBody("1"), http.StatusConflict); got.Code != "REVISION_CONFLICT" {
			t.Fatalf("code = %q", got.Code)
		}
		same := f.view(u1, http.MethodPut, path, updateBody("2", "Renamed", emptyDefinition), http.StatusOK)
		if same.Data.Revision != "2" {
			t.Fatalf("an unchanged update bumped the revision: %+v", same.Data)
		}
		f.view(u1, http.MethodDelete, path, deleteBody("2"), http.StatusOK)
		f.view(u1, http.MethodGet, path, "", http.StatusNotFound)
	}
	// invalid input.
	{
		for name, body := range map[string]string{
			"unknown filter field": createBody(uuid.New(), "personal", "x", `{"schemaVersion":1,"filters":{"gateway":["`+uuid.NewString()+`"]}}`),
			"blank name":           createBody(uuid.New(), "personal", "  ", emptyDefinition),
			"unknown ownership":    createBody(uuid.New(), "shared", "x", emptyDefinition),
			"nil command":          createBody(uuid.Nil, "personal", "x", emptyDefinition),
			"unknown protocol":     createBody(uuid.New(), "personal", "x", `{"schemaVersion":1,"filters":{"protocols":["morse"]}}`),
			"missing definition":   `{"schemaVersion":1,"commandId":"` + uuid.NewString() + `","ownership":"personal","name":"x"}`,
		} {
			if got := f.view(u1, http.MethodPost, "/api/v1/saved-views", body, http.StatusBadRequest); got.Code != "INVALID_INPUT" {
				t.Errorf("%s: code %q", name, got.Code)
			}
		}
		other := f.view(u1, http.MethodPost, "/api/v1/saved-views", createBody(uuid.New(), "personal", "P", emptyDefinition), http.StatusCreated)
		put := "/api/v1/saved-views/" + other.Data.ID.String()
		f.view(u1, http.MethodPut, put, `{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","expectedRevision":"1","name":"x","definition":`+emptyDefinition+`}`, http.StatusBadRequest)
		f.view(u1, http.MethodGet, "/api/v1/saved-views?bogus=1", "", http.StatusBadRequest)
	}
}

// TestSavedViewsGlobalDelegation covers F-104.
func TestSavedViewsGlobalDelegation(t *testing.T) {
	t.Parallel()
	f := newSavedViewFixture(t, 0)
	agencyA := f.agency
	agencyB := seedAgency(t, f.store)
	fields := []string{"payload", "unit"}
	read := []string{"message.read"}
	unitA := f.unit("UA", nil)
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	messageA := f.message(base, "Brand A", unitA)
	f.agency = agencyB
	unitB := f.unit("UB", nil)
	f.message(base.Add(time.Minute), "Brand B", unitB)
	f.agency = agencyA

	manager, _ := f.keyCaller([]string{"saved-view.manage-global"}, authz.Scope{Kind: "all"}, fields)
	reader, _ := f.keyCaller(read, authz.Scope{Kind: "agency", References: []string{agencyA.String()}}, fields)
	outsider, _ := f.keyCaller([]string{"agency.read"}, authz.Scope{Kind: "all"}, fields)

	created := f.view(manager, http.MethodPost, "/api/v1/saved-views",
		createBody(uuid.New(), "global", "Brand", `{"schemaVersion":1,"filters":{"text":"Brand"}}`), http.StatusCreated)
	path := "/api/v1/saved-views/" + created.Data.ID.String()

	if names, total := f.viewList(reader, ""); len(names) != 1 || total != 1 {
		t.Fatalf("reader list = %v/%d", names, total)
	}
	f.view(reader, http.MethodGet, path, "", http.StatusOK)
	if got := f.view(reader, http.MethodPut, path, updateBody("1", "x", emptyDefinition), http.StatusForbidden); got.Code != "FORBIDDEN" {
		t.Fatalf("code = %q", got.Code)
	}
	f.view(reader, http.MethodDelete, path, deleteBody("1"), http.StatusForbidden)
	if _, total := f.viewList(outsider, ""); total != 0 {
		t.Fatalf("outsider total = %d", total)
	}
	f.view(outsider, http.MethodGet, path, "", http.StatusNotFound)
	f.view(outsider, http.MethodPut, path, updateBody("1", "x", emptyDefinition), http.StatusNotFound)

	// Running the view's filters yields only the reader's rights; the manager has no Message access.
	page := f.feed(reader.bearer, "?text=Brand", http.StatusOK)
	if got := itemIDs(page.Data.Items); len(got) != 1 || got[0] != messageA.String() {
		t.Fatalf("reader feed = %v", got)
	}
	f.feed(manager.bearer, "?text=Brand", http.StatusForbidden)

	// A manager without message.read maintains and reads the view, still without Message access.
	updated := f.view(manager, http.MethodPut, path, updateBody("1", "Brand 2", emptyDefinition), http.StatusOK)
	if updated.Data.Revision != "2" {
		t.Fatalf("updated = %+v", updated.Data)
	}
	f.view(manager, http.MethodGet, path, "", http.StatusOK)
	f.view(manager, http.MethodDelete, path, deleteBody("2"), http.StatusOK)
	f.view(reader, http.MethodGet, path, "", http.StatusNotFound)
}

// TestSavedViewsUnavailableFilters covers F-105.
func TestSavedViewsUnavailableFilters(t *testing.T) {
	t.Parallel()
	f := newSavedViewFixture(t, 0)
	all := authz.Scope{Kind: "all"}
	read := []string{"message.read"}
	unit := f.unit("U1", nil)
	f.message(time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond), "Brand", unit)
	p, _ := f.keyCaller(read, all, []string{"payload"})
	q, _ := f.keyCaller(read, all, []string{"payload", "unit"})
	author, _ := f.keyCaller([]string{"saved-view.manage-global"}, all, []string{"payload", "unit"})
	payloadAuthor, _ := f.keyCaller([]string{"saved-view.manage-global"}, all, []string{"payload"})

	definition := `{"schemaVersion":1,"filters":{"units":["` + unit.String() + `"],"text":"Brand"}}`
	created := f.view(author, http.MethodPost, "/api/v1/saved-views", createBody(uuid.New(), "global", "G", definition), http.StatusCreated)
	path := "/api/v1/saved-views/" + created.Data.ID.String()

	if got := f.view(p, http.MethodGet, path, "", http.StatusOK); len(got.Data.UnavailableFilters) != 1 || got.Data.UnavailableFilters[0] != "unit" {
		t.Fatalf("P unavailable = %v", got.Data.UnavailableFilters)
	}
	if got := f.view(q, http.MethodGet, path, "", http.StatusOK); got.Data.UnavailableFilters == nil || len(got.Data.UnavailableFilters) != 0 {
		t.Fatalf("Q unavailable = %v", got.Data.UnavailableFilters)
	}
	f.feed(p.bearer, "?unit="+unit.String(), http.StatusForbidden)
	f.feed(p.bearer, "?text=Brand", http.StatusOK)

	before := f.scalar(`SELECT count(*) FROM saved_views`)
	for _, who := range []credentials{payloadAuthor, p} {
		ownership := "global"
		if who == p {
			ownership = "personal"
		}
		if got := f.view(who, http.MethodPost, "/api/v1/saved-views", createBody(uuid.New(), ownership, "Bad", definition), http.StatusForbidden); got.Code != "FORBIDDEN" {
			t.Fatalf("code = %q", got.Code)
		}
	}
	if after := f.scalar(`SELECT count(*) FROM saved_views`); after != before {
		t.Fatalf("a forbidden save stored %d view(s)", after-before)
	}
	f.view(payloadAuthor, http.MethodPut, path, updateBody("1", "G", definition), http.StatusForbidden)
}

// TestSavedViewsPaging covers ordering by name and the offset page.
func TestSavedViewsPaging(t *testing.T) {
	t.Parallel()
	f := newSavedViewFixture(t, 0)
	user, _ := f.keyCaller([]string{"message.read"}, authz.Scope{Kind: "all"}, []string{"payload"})
	for _, name := range []string{"b", "c", "a"} {
		f.view(user, http.MethodPost, "/api/v1/saved-views", createBody(uuid.New(), "personal", name, emptyDefinition), http.StatusCreated)
	}
	if names, total := f.viewList(user, "?offset=1&limit=2"); strings.Join(names, ",") != "b,c" || total != 3 {
		t.Fatalf("page = %v/%d", names, total)
	}
	if names, _ := f.viewList(user, ""); strings.Join(names, ",") != "a,b,c" {
		t.Fatalf("order = %v", names)
	}
}

// waitForFreshWindow avoids a window boundary inside a test that counts requests.
func waitForFreshWindow(t *testing.T) {
	t.Helper()
	if left := 60 - time.Now().UTC().Second(); left < 15 {
		time.Sleep(time.Duration(left+1) * time.Second)
	}
}

// TestSearchAdmission covers F-108 with three attempts per minute.
func TestSearchAdmission(t *testing.T) {
	t.Parallel()
	f := newSavedViewFixture(t, 3)
	read := []string{"message.read"}
	all := authz.Scope{Kind: "all"}
	fields := []string{"payload"}
	p1 := f.human(read, all, fields)
	p2 := f.human(read, all, fields)
	p1First, p1Second, p2Session := f.session(p1), f.session(p1), f.session(p2)
	p1Key := f.key(p1, read, fields)
	f.guestGrant(read, all, fields)
	guest := credentials{}

	waitForFreshWindow(t)
	feed := func(who credentials) (int, http.Header, []byte) {
		return f.call(who, http.MethodGet, "/api/v1/messages", "")
	}
	for i := 0; i < 3; i++ {
		if status, _, body := feed(p1First); status != http.StatusOK {
			t.Fatalf("P1 request %d = %d: %s", i+1, status, body)
		}
	}
	for i := 0; i < 3; i++ {
		status, header, body := feed(p1Second)
		if status != http.StatusTooManyRequests {
			t.Fatalf("P1 over the limit (second session) = %d: %s", status, body)
		}
		var parsed struct {
			Code              string `json:"code"`
			Retryable         bool   `json:"retryable"`
			RetryAfterSeconds int    `json:"retryAfterSeconds"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			t.Fatal(err)
		}
		retry, err := strconv.Atoi(header.Get("Retry-After"))
		if err != nil || retry < 1 || retry > 60 || parsed.RetryAfterSeconds != retry || parsed.Code != "RATE_LIMITED" || !parsed.Retryable {
			t.Fatalf("429 = %q %+v", header.Get("Retry-After"), parsed)
		}
		left := 60 - time.Now().UTC().Second()
		if retry < left-3 || retry > left+3 {
			t.Fatalf("Retry-After = %d, %d seconds left in the window", retry, left)
		}
	}
	if status, _, body := feed(p2Session); status != http.StatusOK {
		t.Fatalf("P2 = %d: %s", status, body)
	}
	if status, _, body := feed(p1Key); status != http.StatusOK {
		t.Fatalf("P1 key = %d: %s", status, body)
	}
	for i := 0; i < 3; i++ {
		if status, _, body := feed(guest); status != http.StatusOK {
			t.Fatalf("guest request %d = %d: %s", i+1, status, body)
		}
	}
	if status, header, _ := feed(guest); status != http.StatusTooManyRequests || header.Get("Retry-After") == "" {
		t.Fatalf("guest over the limit = %d", status)
	}
	bucket := sha256.Sum256([]byte("principal:" + p1.String()))
	if got := f.scalar(`SELECT count FROM search_admissions WHERE bucket = $1`, bucket[:]); got != 3 {
		t.Fatalf("P1 count = %d; rejected requests must not be counted", got)
	}
	if got := f.scalar(`SELECT count(*) FROM search_admissions`); got != 4 {
		t.Fatalf("buckets = %d, want P1, P2, key and guest", got)
	}
}

// TestSavedViewsStoreTheFilterPredicate checks that a predicate is compiled when saved, that its
// field groups make the view unavailable for readers without them, and that it round-trips.
func TestSavedViewsStoreTheFilterPredicate(t *testing.T) {
	t.Parallel()
	f := newSavedViewFixture(t, 0)
	all := authz.Scope{Kind: "all"}
	reader, _ := f.keyCaller([]string{"message.read"}, all, []string{"payload"})
	noPayload, _ := f.keyCaller([]string{"message.read"}, all, []string{"gateway"})
	author, _ := f.keyCaller([]string{"saved-view.manage-global"}, all, []string{"payload"})

	predicate := `{"op":"eq","field":{"namespace":"message","path":["protocol"]},"value":{"kind":"string","value":"pocsag"}}`
	definition := `{"schemaVersion":1,"filters":{"filter":` + predicate + `}}`
	created := f.view(author, http.MethodPost, "/api/v1/saved-views", createBody(uuid.New(), "global", "G", definition), http.StatusCreated)
	path := "/api/v1/saved-views/" + created.Data.ID.String()
	if got := f.view(reader, http.MethodGet, path, "", http.StatusOK); len(got.Data.UnavailableFilters) != 0 {
		t.Fatalf("reader unavailable = %v", got.Data.UnavailableFilters)
	}
	got := f.view(noPayload, http.MethodGet, path, "", http.StatusOK)
	if len(got.Data.UnavailableFilters) != 1 || got.Data.UnavailableFilters[0] != "filter" {
		t.Fatalf("noPayload unavailable = %v", got.Data.UnavailableFilters)
	}
	if !strings.Contains(string(got.Data.Definition), `"filter":{"op":"eq"`) {
		t.Fatalf("definition = %s", got.Data.Definition)
	}
	unknown := `{"schemaVersion":1,"filters":{"filter":{"op":"present","field":{"namespace":"attributes","path":["nope","x"]},"value":{"kind":"boolean","value":true}}}}`
	if bad := f.view(author, http.MethodPost, "/api/v1/saved-views", createBody(uuid.New(), "global", "Bad", unknown), http.StatusBadRequest); bad.Code != "INVALID_INPUT" {
		t.Fatalf("code = %q", bad.Code)
	}
	needsUnit := `{"schemaVersion":1,"filters":{"filter":{"op":"present","field":{"namespace":"message","path":["unitNames"]},"value":{"kind":"boolean","value":true}}}}`
	f.view(author, http.MethodPost, "/api/v1/saved-views", createBody(uuid.New(), "global", "Forbidden", needsUnit), http.StatusForbidden)
}
