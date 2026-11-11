package readmodel

import (
	"context"
	"net/url"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
)

// Page size bounds of the feed.
const (
	// DefaultPageSize is the page size when the request names none.
	DefaultPageSize = 50
	// MaxPageSize is the largest page size.
	MaxPageSize = 200
)

// FeedRequest is a parsed feed request.
type FeedRequest struct {
	Filters   Filters
	PageSize  int
	PageToken string
}

// ParseFeedRequest reads the feed parameters: the filters plus pageSize (1-200, default 50)
// and pageToken. Failures are authz.ErrInvalidInput.
func ParseFeedRequest(values url.Values) (FeedRequest, error) {
	filters, err := ParseFilters(values, "pageSize", "pageToken")
	if err != nil {
		return FeedRequest{}, err
	}
	request := FeedRequest{Filters: filters, PageSize: DefaultPageSize, PageToken: values.Get("pageToken")}
	if values.Has("pageSize") {
		size, err := strconv.Atoi(values.Get("pageSize"))
		if err != nil || size < 1 || size > MaxPageSize {
			return FeedRequest{}, authz.ErrInvalidInput
		}
		request.PageSize = size
	}
	if values.Has("pageToken") && request.PageToken == "" {
		return FeedRequest{}, authz.ErrInvalidInput
	}
	return request, nil
}

// Service answers Message reads for an authenticated caller, the guest included. Create it
// with NewService; it is safe for concurrent use.
type Service struct {
	transactions identity.Transactions
	resolver     identity.AuthorityResolver
	clock        platform.Clock
	store        Store
	keys         TokenKeys
	registry     FieldRegistry

	mu       sync.Mutex
	tokenKey []byte
}

// NewService composes the read side. It fails with authz.ErrInvalidInput on a missing dependency.
func NewService(transactions identity.Transactions, resolver identity.AuthorityResolver, clock platform.Clock,
	store Store, keys TokenKeys, registry FieldRegistry) (*Service, error) {
	if transactions == nil || resolver == nil || clock == nil || store == nil || keys == nil || registry == nil {
		return nil, authz.ErrInvalidInput
	}
	return &Service{transactions: transactions, resolver: resolver, clock: clock, store: store, keys: keys,
		registry: registry}, nil
}

// access is everything one read needs to know about its caller.
type access struct {
	caller Caller
	// revision is the access revision of the resolved authority; a change means grants or
	// restrictions changed since an earlier read.
	revision int64
	rows     authz.MessageAccess
	viewer   viewer
}

// run resolves the caller's authority and runs operation in the same transaction.
func (s *Service) run(ctx context.Context, proof identity.Proof,
	operation func(tx platform.TransactionContext, access access, now time.Time) error) error {
	var authentication identity.Authentication
	remember := func(value identity.Authentication) error { authentication = value; return nil }
	return identity.WithAuthority(ctx, s.transactions, s.clock, s.resolver, proof, false, remember,
		func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
			current, err := newAccess(authority, now)
			if err != nil {
				return err
			}
			if !current.rows.Read.Granted() {
				return authz.ErrForbidden
			}
			current.caller = NewCaller(authentication)
			return operation(tx, current, now)
		})
}

// newAccess derives row eligibility, readable field groups and Network reach from an already
// resolved authority. It does not require message.read; callers decide what a missing grant means.
func newAccess(authority authz.Authority, now time.Time) (access, error) {
	rows, err := authz.NewMessageAccess(authority, now)
	if err != nil {
		return access{}, err
	}
	groups, err := authz.ReadableFieldGroups(authority)
	if err != nil {
		return access{}, err
	}
	reach, err := authz.ReachableAgencies(authority, "network.read", now)
	if err != nil {
		reach = authz.AgencyReach{}
	}
	current := access{revision: authority.Revision, rows: rows,
		viewer: viewer{groups: make(map[string]bool, len(groups)), networkReach: reach}}
	for _, group := range groups {
		current.viewer.groups[group] = true
	}
	return current, nil
}

// requireGroups fails with authz.ErrForbidden when a needed field group is unreadable.
func (a access) requireGroups(groups []string) error {
	for _, group := range groups {
		if !a.viewer.can(group) {
			return authz.ErrForbidden
		}
	}
	return nil
}

// derivedGroups returns the field groups the caller may read results of, or nil when the caller
// cannot read derived results at all. The store shows a result only when all of its stored
// groups are among them.
func (a access) derivedGroups() []string {
	if !a.viewer.can("derived") {
		return nil
	}
	groups := make([]string, 0, len(a.viewer.groups))
	for group := range a.viewer.groups {
		groups = append(groups, group)
	}
	slices.Sort(groups)
	return groups
}

// key returns the page token secret, remembered after the first successful load. On first use
// the secret is created in its own transaction, committed before it is remembered: created
// inside a read that later fails, it would roll back while this process kept signing with it,
// and every other process would create a different one. It runs before the caller's authority
// transaction so it never waits for a second pool connection while holding one.
func (s *Service) key(ctx context.Context) ([]byte, error) {
	s.mu.Lock()
	key := s.tokenKey
	s.mu.Unlock()
	if key != nil {
		return key, nil
	}
	err := s.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		var err error
		key, err = s.keys.TokenKey(tx.Context(), tx)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.tokenKey = key
	s.mu.Unlock()
	return key, nil
}

// Feed returns one page of the Messages the caller may read, newest first. Filters that need a
// field group the caller cannot read fail with authz.ErrForbidden; a page token of other
// filters, another caller or a tampered one fails with authz.ErrInvalidInput.
func (s *Service) Feed(ctx context.Context, proof identity.Proof, request FeedRequest) (FeedPage, error) {
	key, err := s.key(ctx)
	if err != nil {
		return FeedPage{}, err
	}
	var page FeedPage
	err = s.run(ctx, proof, func(tx platform.TransactionContext, current access, now time.Time) error {
		selection, err := s.prepare(tx.Context(), tx, current, request.Filters)
		if err != nil {
			return err
		}
		var after *Cursor
		if request.PageToken != "" {
			cursor, err := DecodePageToken(key, request.PageToken, request.Filters, current.caller)
			if err != nil {
				return err
			}
			after = &cursor
		}
		result, err := s.store.Feed(tx.Context(), tx, FeedQuery{Access: current.rows, Filters: request.Filters,
			Selection: selection, After: after, Limit: request.PageSize, DerivedGroups: current.derivedGroups()})
		if err != nil {
			return err
		}
		page = FeedPage{Items: make([]FeedItem, 0, len(result.Items)),
			Position: Position{TxID: result.TxID, Seq: result.Seq, Issued: now}.Sign(key)}
		for _, record := range result.Items {
			page.Items = append(page.Items, current.viewer.feedItem(record, false))
		}
		if result.HasMore && len(result.Items) > 0 {
			last := result.Items[len(result.Items)-1]
			page.NextPageToken = EncodePageToken(key, Cursor{Time: last.Time, ID: last.MessageID}, request.Filters, current.caller)
		}
		return nil
	})
	return page, err
}

// Evaluate returns the feed items of exactly the listed Messages that match the filters, which
// are applied without their time bounds, in feed order. It serves live updates: the client
// places a returned item in its range, and a listed Message that is absent no longer matches.
func (s *Service) Evaluate(ctx context.Context, proof identity.Proof, filters Filters, ids []uuid.UUID) ([]FeedItem, error) {
	var items []FeedItem
	err := s.run(ctx, proof, func(tx platform.TransactionContext, current access, _ time.Time) error {
		selection, err := s.prepare(ctx, tx, current, filters.WithoutTime())
		if err != nil {
			return err
		}
		items, err = s.evaluate(ctx, tx, current, filters, selection, ids)
		return err
	})
	return items, err
}

// evaluate returns the feed items of the listed Messages that match the filters without their
// time bounds, in feed order, within the caller's transaction. selection is the compiled
// predicate of the filters.
func (s *Service) evaluate(ctx context.Context, tx platform.TransactionContext, current access, filters Filters,
	selection *fields.Filter, ids []uuid.UUID) ([]FeedItem, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	result, err := s.store.Feed(tx.Context(), tx, FeedQuery{Access: current.rows, Filters: filters.WithoutTime(),
		Selection: selection, IDs: slices.Clone(ids), DerivedGroups: current.derivedGroups()})
	if err != nil {
		return nil, err
	}
	items := make([]FeedItem, 0, len(result.Items))
	for _, record := range result.Items {
		items = append(items, current.viewer.feedItem(record, false))
	}
	return items, nil
}

// EligibleMessages returns those of the listed Messages that have an appearance the caller may
// read, regardless of any filter.
func (s *Service) EligibleMessages(ctx context.Context, proof identity.Proof, ids []uuid.UUID) ([]uuid.UUID, error) {
	var eligible []uuid.UUID
	err := s.run(ctx, proof, func(tx platform.TransactionContext, current access, _ time.Time) error {
		if len(ids) == 0 {
			return nil
		}
		var err error
		eligible, err = s.store.Eligible(tx.Context(), tx, current.rows, slices.Clone(ids))
		return err
	})
	return eligible, err
}

// Detail returns one Message with its supporting Transmissions. A Message without an
// appearance that is eligible for detail is authz.ErrNotFound.
func (s *Service) Detail(ctx context.Context, proof identity.Proof, id uuid.UUID) (DetailItem, error) {
	var item DetailItem
	err := s.run(ctx, proof, func(tx platform.TransactionContext, current access, _ time.Time) error {
		if !current.rows.Detail.Granted() {
			return authz.ErrForbidden
		}
		record, err := s.store.Detail(tx.Context(), tx, DetailQuery{Access: current.rows, MessageID: id,
			DerivedGroups: current.derivedGroups()})
		if err != nil {
			return err
		}
		item = current.viewer.detailItem(record)
		return nil
	})
	return item, err
}

// FilterOptions returns the objects the caller may filter by, limited to what the caller's
// message.read scopes reach and to the families whose field group is readable. It carries no
// counts and no values derived from Messages.
func (s *Service) FilterOptions(ctx context.Context, proof identity.Proof) (OptionsItem, error) {
	var item OptionsItem
	err := s.run(ctx, proof, func(tx platform.TransactionContext, current access, _ time.Time) error {
		record, err := s.store.Options(tx.Context(), tx, OptionsQuery{Access: current.rows,
			Units: current.viewer.can("unit"), Organizations: current.viewer.can("unit"),
			Agencies: current.viewer.can("unit"), Networks: current.viewer.can("gateway")})
		if err != nil {
			return err
		}
		item = current.viewer.optionsItem(record)
		return nil
	})
	return item, err
}
