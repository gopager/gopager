package readstore

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/readmodel"
)

// suppressedTarget is true for a target with an active suppression. It is evaluated for the
// target alias tt of the enclosing query.
const suppressedTarget = `EXISTS (SELECT 1 FROM target_suppressions ts WHERE ts.transmission_id = tt.transmission_id
	AND ts.target_position = tt.position AND ts.removed_at IS NULL)`

// supports renders when Transmission t (with Network n) supports appearance mu: an unsuppressed
// assigned target of the appearance's Unit, or for a Unit-less appearance a targetless or
// unsuppressed-unassigned Transmission of a Network of the appearance's Agency.
func supports(t, n, mu string) string {
	return fmt.Sprintf(`((%[3]s.unit_id IS NOT NULL AND EXISTS (SELECT 1 FROM transmission_targets tt
			WHERE tt.transmission_id = %[1]s.id AND tt.resolution_status = 'assigned' AND tt.unit_id = %[3]s.unit_id
			AND NOT %[4]s))
		OR (%[3]s.unit_id IS NULL AND %[2]s.agency_id = %[3]s.agency_id AND (
			NOT EXISTS (SELECT 1 FROM transmission_targets tt WHERE tt.transmission_id = %[1]s.id)
			OR EXISTS (SELECT 1 FROM transmission_targets tt WHERE tt.transmission_id = %[1]s.id
				AND tt.resolution_status = 'unassigned' AND NOT %[4]s))))`, t, n, mu, suppressedTarget)
}

// textMatch renders the literal, Unicode case-folded substring match of the search term on the
// Message's canonical text or numeric Payload value.
func textMatch(term string) string {
	return `CASE WHEN m.identity_form IN ('text', 'numeric') THEN strpos(
		normalize(casefold(normalize(convert_from(m.identity_value, 'UTF8'), NFC) COLLATE pg_unicode_fast), NFC),
		normalize(casefold(normalize(` + term + `::text, NFC) COLLATE pg_unicode_fast), NFC)) > 0 ELSE false END`
}

// feedBuilder renders the single statement of a feed page or of a detail.
type feedBuilder struct {
	params *params
	elig   *eligibility
	query  readmodel.FeedQuery
	detail bool
	groups string
}

// groupsParam registers the caller's readable field groups on first use as a jsonb array, so
// that an unused parameter never leaves a gap in the numbering.
func (b *feedBuilder) groupsParam() string {
	if b.groups == "" {
		encoded, err := json.Marshal(append([]string{}, b.query.DerivedGroups...))
		if err != nil {
			encoded = []byte("[]")
		}
		b.groups = b.params.add(string(encoded)) + "::jsonb"
	}
	return b.groups
}

// appearanceColumns are the columns of the message_units alias mu.
func appearanceColumns(alias string) columns {
	return columns{agency: alias + ".agency_id", unit: alias + ".unit_id", anchor: alias + ".age_anchor"}
}

// eligible renders the eligibility of an appearance for the statement's mode.
func (b *feedBuilder) eligible(alias string) string {
	if b.detail {
		return b.elig.appearanceDetail(appearanceColumns(alias))
	}
	return b.elig.appearance(appearanceColumns(alias))
}

// appearanceFilters renders the filters that must hold on the same appearance.
func (b *feedBuilder) appearanceFilters() string {
	filters := b.query.Filters
	var conditions []string
	if len(filters.Units) > 0 || filters.WithoutUnit {
		var alternatives []string
		if len(filters.Units) > 0 {
			alternatives = append(alternatives, "mu.unit_id = ANY("+b.params.uuids(filters.Units)+")")
		}
		if filters.WithoutUnit {
			alternatives = append(alternatives, "mu.unit_id IS NULL")
		}
		conditions = append(conditions, "("+strings.Join(alternatives, " OR ")+")")
	}
	if len(filters.Organizations) > 0 {
		conditions = append(conditions, "u.organization_id = ANY("+b.params.uuids(filters.Organizations)+")")
	}
	if len(filters.Agencies) > 0 {
		conditions = append(conditions, "mu.agency_id = ANY("+b.params.uuids(filters.Agencies)+")")
	}
	if len(filters.Networks) > 0 || len(filters.Protocols) > 0 {
		inner := []string{"t.message_id = mu.message_id", supports("t", "n", "mu")}
		if len(filters.Networks) > 0 {
			inner = append(inner, "t.network_id = ANY("+b.params.uuids(filters.Networks)+")")
		}
		if len(filters.Protocols) > 0 {
			inner = append(inner, "t.protocol = ANY("+b.params.add(filters.Protocols)+"::text[])")
		}
		conditions = append(conditions, "EXISTS (SELECT 1 FROM transmissions t JOIN networks n ON n.id = t.network_id WHERE "+
			strings.Join(inner, " AND ")+")")
	}
	if len(conditions) == 0 {
		return ""
	}
	return " AND " + strings.Join(conditions, " AND ")
}

// valueMatch renders when the typed value expression (an object with kind and value) equals the
// condition as a string, a decimal number or a boolean.
func (b *feedBuilder) valueMatch(value string, condition readmodel.DerivedCondition) string {
	matcher := condition.Matcher()
	alternatives := []string{"(" + value + "->>'kind' IN ('string', 'timestamp') AND " + value + "->'value' = to_jsonb(" +
		b.params.add(matcher.Text) + "::text))"}
	if matcher.Number != "" {
		alternatives = append(alternatives, "(CASE WHEN "+value+"->>'kind' IN ('integer', 'number') THEN ("+value+
			"->>'value')::numeric = "+b.params.add(matcher.Number)+"::numeric ELSE false END)")
	}
	if matcher.Boolean != "" {
		alternatives = append(alternatives, "("+value+"->>'kind' = 'boolean' AND "+value+"->'value' = "+
			b.params.add(matcher.Boolean)+"::jsonb)")
	}
	return "(" + strings.Join(alternatives, " OR ") + ")"
}

// targetVisibility renders when target tt of a Transmission is visible: assigned through an
// eligible Unit appearance, unassigned through an eligible Unit-less appearance, any other
// status through message.read and message.detail on the Network's Agency. appearance renders an
// existence test for an eligible appearance with the extra condition; agency, observed and
// accepted name the Transmission's Network Agency and times.
func (b *feedBuilder) targetVisibility(appearance func(condition string) string, agency, observed, accepted string) string {
	context := b.elig.context(columns{agency: agency, unit: "NULL::uuid", anchor: "LEAST(" + observed + ", " + accepted + ")"})
	return `(NOT ` + suppressedTarget + ` AND (
		(tt.resolution_status = 'assigned' AND ` + appearance("v.unit_id = tt.unit_id") + `)
		OR (tt.resolution_status = 'unassigned' AND ` + appearance("v.unit_id IS NULL AND v.agency_id = "+agency) + `)
		OR (tt.resolution_status IN ('pending', 'failed', 'unresolved') AND ` + context + `)))`
}

// latestDerived renders the latest published result of each Workflow among the Transmissions
// that support an eligible appearance of the Message m.id, looked up without the page CTEs.
func (b *feedBuilder) latestDerived() string {
	return `SELECT DISTINCT ON (c.workflow_id) c.workflow_id, r.field_groups, r.evaluation,
			dt.id AS transmission_id, dn.agency_id AS network_agency, dt.observed_at, dt.accepted_at, dt.message_id
		FROM message_units mu JOIN transmissions dt ON dt.message_id = mu.message_id
		JOIN networks dn ON dn.id = dt.network_id
		JOIN processing_candidates c ON c.transmission_id = dt.id
		JOIN processing_results r ON r.candidate_id = c.id AND r.published_at IS NOT NULL AND r.message_id = dt.message_id
		WHERE mu.message_id = m.id AND ` + b.eligible("mu") + ` AND ` + supports("dt", "dn", "mu") + `
		ORDER BY c.workflow_id, r.published_at DESC, r.id DESC`
}

// derivedFilters renders one condition per output name: some visible derived result of the
// Message has an output of that name equal to one of the names' values.
func (b *feedBuilder) derivedFilters() string {
	byName := make(map[string][]readmodel.DerivedCondition)
	var names []string
	for _, condition := range b.query.Filters.Derived {
		if _, seen := byName[condition.Name]; !seen {
			names = append(names, condition.Name)
		}
		byName[condition.Name] = append(byName[condition.Name], condition)
	}
	slices.Sort(names)
	var rendered []string
	for _, name := range names {
		parameter := b.params.add(name) + "::text"
		var topLevel, targeted []string
		for _, condition := range byName[name] {
			topLevel = append(topLevel, b.valueMatch("(l.evaluation->'fields'->"+parameter+"->'value')", condition))
			targeted = append(targeted, b.valueMatch("(tr->'fields'->"+parameter+"->'value')", condition))
		}
		appearance := func(condition string) string {
			return "EXISTS (SELECT 1 FROM message_units v WHERE v.message_id = l.message_id AND " + condition +
				" AND " + b.eligible("v") + ")"
		}
		visible := b.targetVisibility(appearance, "l.network_agency", "l.observed_at", "l.accepted_at")
		rendered = append(rendered, `EXISTS (SELECT 1 FROM (`+b.latestDerived()+`) l
			WHERE l.field_groups <@ `+b.groupsParam()+` AND (
				`+strings.Join(topLevel, " OR ")+`
				OR EXISTS (SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(l.evaluation->'targetResults') = 'array'
						THEN l.evaluation->'targetResults' ELSE '[]'::jsonb END) tr
					WHERE (`+strings.Join(targeted, " OR ")+`)
					AND (tr->>'targetPosition')::int IN (SELECT tt.position FROM transmission_targets tt
						WHERE tt.transmission_id = l.transmission_id AND `+visible+`))))`)
	}
	if len(rendered) == 0 {
		return ""
	}
	return " AND " + strings.Join(rendered, " AND ")
}

// pageCTE renders the selection of the page's Messages.
func (b *feedBuilder) pageCTE() string {
	filters := b.query.Filters
	conditions := []string{}
	limit := ""
	if b.query.IDs != nil {
		conditions = append(conditions, "m.id = ANY("+b.params.uuids(b.query.IDs)+")")
	} else {
		if filters.From != nil {
			conditions = append(conditions, "m.first_observed_at >= "+b.params.add(*filters.From))
		}
		if filters.Until != nil {
			conditions = append(conditions, "m.first_observed_at < "+b.params.add(*filters.Until))
		}
		if cursor := b.query.After; cursor != nil {
			at := b.params.add(cursor.Time.UTC())
			conditions = append(conditions, "(m.first_observed_at < "+at+" OR (m.first_observed_at = "+at+
				" AND m.id < "+b.params.add(cursor.ID)+"::uuid))")
		}
		limit = " LIMIT " + b.params.add(b.query.Limit+1)
	}
	if filters.Text != "" {
		conditions = append(conditions, textMatch(b.params.add(filters.Text)))
	}
	conditions = append(conditions, `EXISTS (SELECT 1 FROM message_units mu LEFT JOIN units u ON u.id = mu.unit_id
		WHERE mu.message_id = m.id AND `+b.eligible("mu")+b.appearanceFilters()+`)`)
	where := strings.Join(conditions, "\n\t\tAND ") + b.derivedFilters()
	return `page AS (
	SELECT m.id, m.first_observed_at FROM messages m
	WHERE ` + where + `
	ORDER BY m.first_observed_at DESC, m.id DESC` + limit + `)`
}

// derivedColumn renders, for the page Message p, the latest result per Workflow if its stored
// field groups are all readable.
func (b *feedBuilder) derivedColumn() string {
	if len(b.query.DerivedGroups) == 0 {
		return `'[]'::jsonb`
	}
	appearance := func(condition string) string {
		return "EXISTS (SELECT 1 FROM app v WHERE v.message_id = l.message_id AND " + condition + ")"
	}
	visible := b.targetVisibility(appearance, "l.network_agency", "l.observed_at", "l.accepted_at")
	return `COALESCE((SELECT jsonb_agg(jsonb_build_object('workflowId', l.workflow_id, 'resultId', l.result_id,
			'evaluation', l.evaluation, 'visible', ARRAY(
				SELECT tt.position FROM transmission_targets tt WHERE tt.transmission_id = l.transmission_id AND ` + visible + `))
			ORDER BY l.workflow_id, l.result_id)
		FROM (SELECT DISTINCT ON (c.workflow_id) c.workflow_id, r.id AS result_id, r.field_groups,
				r.evaluation, s.id AS transmission_id, s.message_id, s.network_agency, s.observed_at, s.accepted_at
			FROM sup s JOIN processing_candidates c ON c.transmission_id = s.id
			JOIN processing_results r ON r.candidate_id = c.id AND r.published_at IS NOT NULL AND r.message_id = s.message_id
			WHERE s.message_id = p.id
			ORDER BY c.workflow_id, r.published_at DESC, r.id DESC) l
		WHERE l.field_groups <@ ` + b.groupsParam() + `), '[]'::jsonb)`
}

// transmissionsColumn renders the supporting Transmissions of a detail with their targets.
func (b *feedBuilder) transmissionsColumn() string {
	if !b.detail {
		return `'[]'::jsonb`
	}
	appearance := func(condition string) string {
		return "EXISTS (SELECT 1 FROM app v WHERE v.message_id = s.message_id AND " + condition + ")"
	}
	visible := b.targetVisibility(appearance, "s.network_agency", "s.observed_at", "s.accepted_at")
	return `COALESCE((SELECT jsonb_agg(jsonb_build_object('id', s.id, 'acceptedAt', s.accepted_at,
			'observedAt', s.observed_at, 'processingStatus', s.processing_status, 'networkId', s.network_id,
			'networkName', s.network_name, 'networkAgencyId', s.network_agency, 'networkUnresolved', s.network_unresolved,
			'protocol', s.protocol, 'incomplete', s.incomplete, 'payload', s.payload, 'quality', s.quality,
			'targets', COALESCE((SELECT jsonb_agg(jsonb_build_object('position', tt.position,
					'resolution', tt.resolution_status, 'unitId', tt.unit_id,
					'encrypted', tt.resolution_status = 'assigned' AND COALESCE(rule.encrypted, false),
					'addressNumber', tt.address_number, 'addressText', tt.address_text, 'function', tt.function)
					ORDER BY tt.position)
				FROM transmission_targets tt LEFT JOIN address_rules rule ON rule.id = tt.matched_rule_id
				WHERE tt.transmission_id = s.id AND ` + visible + `), '[]'::jsonb))
			ORDER BY s.incomplete, s.accepted_at, s.id)
		FROM sup s WHERE s.message_id = p.id), '[]'::jsonb)`
}

// statement renders the whole statement: the snapshot cutoff, the page, the eligible
// appearances and supporting Transmissions of the page, and one row per page Message.
func (b *feedBuilder) statement() string {
	page := b.pageCTE()
	return `WITH snapshot AS (
	SELECT pg_snapshot_xmin(pg_current_snapshot())::text::bigint - 1 AS tx_id,
		COALESCE((SELECT max(seq) FROM domain_events), 0) AS seq),
` + page + `,
app AS (
	SELECT mu.message_id, mu.unit_id, mu.agency_id, mu.first_observed_at, mu.age_anchor, mu.correction_status,
		u.name AS unit_name, u.organization_id
	FROM page p JOIN message_units mu ON mu.message_id = p.id LEFT JOIN units u ON u.id = mu.unit_id
	WHERE ` + b.eligible("mu") + `),
sup AS (
	SELECT t.id, t.message_id, t.accepted_at, t.observed_at, t.incomplete, t.processing_status, t.network_unresolved,
		t.network_id, t.protocol, t.payload::text AS payload, t.quality_metrics::text AS quality,
		n.agency_id AS network_agency, n.name AS network_name
	FROM page p JOIN transmissions t ON t.message_id = p.id JOIN networks n ON n.id = t.network_id
	WHERE EXISTS (SELECT 1 FROM app a WHERE a.message_id = t.message_id AND ` + supports("t", "n", "a") + `)),
result_rows AS (
	SELECT p.id, p.first_observed_at,
		(SELECT CASE WHEN bool_or(s.processing_status = 'pending') THEN 'pending'
			WHEN bool_or(s.processing_status = 'failed') THEN 'failed'
			WHEN bool_or(s.processing_status = 'completed') THEN 'completed' ELSE 'none' END
			FROM sup s WHERE s.message_id = p.id) AS processing_status,
		COALESCE((SELECT bool_or(a.correction_status = 'correcting') FROM app a WHERE a.message_id = p.id), false) AS correcting,
		r.payload, r.protocol, r.incomplete,
		COALESCE((SELECT jsonb_agg(jsonb_build_object('unitId', a.unit_id, 'name', a.unit_name, 'agencyId', a.agency_id,
				'organizationId', a.organization_id, 'firstObservedAt', a.first_observed_at,
				'encrypted', EXISTS (SELECT 1 FROM sup s2 JOIN transmission_targets tt ON tt.transmission_id = s2.id
					JOIN address_rules rule ON rule.id = tt.matched_rule_id
					WHERE s2.message_id = a.message_id AND tt.resolution_status = 'assigned' AND tt.unit_id = a.unit_id
					AND rule.encrypted AND NOT ` + suppressedTarget + `)) ORDER BY a.unit_name, a.unit_id)
			FROM app a WHERE a.message_id = p.id AND a.unit_id IS NOT NULL), '[]'::jsonb) AS units,
		COALESCE((SELECT jsonb_agg(jsonb_build_object('agencyId', a.agency_id, 'firstObservedAt', a.first_observed_at)
				ORDER BY a.agency_id)
			FROM app a WHERE a.message_id = p.id AND a.unit_id IS NULL), '[]'::jsonb) AS without_unit,
		COALESCE((SELECT jsonb_agg(DISTINCT s.network_agency) FROM sup s
			WHERE s.message_id = p.id AND s.network_unresolved), '[]'::jsonb) AS unresolved,
		` + b.derivedColumn() + ` AS derived,
		` + b.transmissionsColumn() + ` AS transmissions
	FROM page p LEFT JOIN LATERAL (SELECT s.payload, s.protocol, s.incomplete FROM sup s WHERE s.message_id = p.id
		ORDER BY s.incomplete, s.accepted_at, s.id LIMIT 1) r ON true)
SELECT s.tx_id, s.seq, r.id, r.first_observed_at, r.processing_status, r.correcting, r.payload, r.protocol, r.incomplete,
	r.units, r.without_unit, r.unresolved, r.derived, r.transmissions
FROM snapshot s LEFT JOIN result_rows r ON true
ORDER BY r.first_observed_at DESC NULLS LAST, r.id DESC`
}

// Feed implements readmodel.Store.
func (s *Store) Feed(ctx context.Context, tx platform.TransactionContext, query readmodel.FeedQuery) (readmodel.FeedResult, error) {
	result, _, err := s.run(ctx, tx, query, false)
	return result, err
}

// Detail implements readmodel.Store.
func (s *Store) Detail(ctx context.Context, tx platform.TransactionContext, query readmodel.DetailQuery) (readmodel.DetailRecord, error) {
	result, details, err := s.run(ctx, tx, readmodel.FeedQuery{Access: query.Access, IDs: []uuid.UUID{query.MessageID},
		DerivedGroups: query.DerivedGroups}, true)
	if err != nil {
		return readmodel.DetailRecord{}, err
	}
	if len(result.Items) != 1 {
		return readmodel.DetailRecord{}, authz.ErrNotFound
	}
	return readmodel.DetailRecord{FeedRecord: result.Items[0], Transmissions: details[0]}, nil
}

// run executes the statement and decodes its rows.
func (s *Store) run(ctx context.Context, tx platform.TransactionContext, query readmodel.FeedQuery,
	detail bool) (readmodel.FeedResult, [][]readmodel.TransmissionRecord, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return readmodel.FeedResult{}, nil, err
	}
	p := &params{}
	builder := &feedBuilder{params: p, elig: newEligibility(p, query.Access), query: query, detail: detail}
	rows, err := executor.QueryContext(ctx, builder.statement(), p.values...)
	if err != nil {
		return readmodel.FeedResult{}, nil, err
	}
	defer func() { _ = rows.Close() }()
	var result readmodel.FeedResult
	var details [][]readmodel.TransmissionRecord
	for rows.Next() {
		var row feedRow
		if err := rows.Scan(&result.TxID, &result.Seq, &row.id, &row.time, &row.processing, &row.correcting, &row.payload,
			&row.protocol, &row.incomplete, &row.units, &row.withoutUnit, &row.unresolved, &row.derived,
			&row.transmissions); err != nil {
			return readmodel.FeedResult{}, nil, err
		}
		if row.id == nil {
			continue
		}
		record, transmissions, err := row.record()
		if err != nil {
			return readmodel.FeedResult{}, nil, err
		}
		result.Items = append(result.Items, record)
		details = append(details, transmissions)
	}
	if err := rows.Err(); err != nil {
		return readmodel.FeedResult{}, nil, err
	}
	if query.IDs == nil && len(result.Items) > query.Limit {
		result.HasMore = true
		result.Items, details = result.Items[:query.Limit], details[:query.Limit]
	}
	return result, details, nil
}

// feedRow is one scanned row of the statement.
type feedRow struct {
	id            *uuid.UUID
	time          *time.Time
	processing    *string
	correcting    *bool
	payload       *string
	protocol      *string
	incomplete    *bool
	units         []byte
	withoutUnit   []byte
	unresolved    []byte
	derived       []byte
	transmissions []byte
}

// record decodes the JSON aggregates of a row.
func (r feedRow) record() (readmodel.FeedRecord, []readmodel.TransmissionRecord, error) {
	record := readmodel.FeedRecord{MessageID: *r.id, Time: r.time.UTC(), ProcessingStatus: "none", CorrectionStatus: "current"}
	if r.processing != nil {
		record.ProcessingStatus = *r.processing
	}
	if r.correcting != nil && *r.correcting {
		record.CorrectionStatus = "correcting"
	}
	if r.payload != nil && r.protocol != nil && r.incomplete != nil {
		record.Representative = &readmodel.PayloadRecord{Payload: json.RawMessage(*r.payload), Protocol: *r.protocol,
			Incomplete: *r.incomplete}
	}
	var units []struct {
		UnitID          uuid.UUID  `json:"unitId"`
		Name            string     `json:"name"`
		AgencyID        uuid.UUID  `json:"agencyId"`
		OrganizationID  *uuid.UUID `json:"organizationId"`
		FirstObservedAt time.Time  `json:"firstObservedAt"`
		Encrypted       bool       `json:"encrypted"`
	}
	var withoutUnit []struct {
		AgencyID        uuid.UUID `json:"agencyId"`
		FirstObservedAt time.Time `json:"firstObservedAt"`
	}
	var derived []struct {
		WorkflowID uuid.UUID       `json:"workflowId"`
		ResultID   uuid.UUID       `json:"resultId"`
		Evaluation json.RawMessage `json:"evaluation"`
		Visible    []int           `json:"visible"`
	}
	for _, decode := range []struct {
		data   []byte
		target any
	}{{r.units, &units}, {r.withoutUnit, &withoutUnit}, {r.unresolved, &record.UnresolvedAgencies}, {r.derived, &derived}} {
		if err := json.Unmarshal(decode.data, decode.target); err != nil {
			return readmodel.FeedRecord{}, nil, err
		}
	}
	for _, unit := range units {
		record.Units = append(record.Units, readmodel.UnitRecord{UnitID: unit.UnitID, Name: unit.Name, AgencyID: unit.AgencyID,
			OrganizationID: unit.OrganizationID, Encrypted: unit.Encrypted, FirstObservedAt: unit.FirstObservedAt.UTC()})
	}
	for _, entry := range withoutUnit {
		record.WithoutUnit = append(record.WithoutUnit, readmodel.WithoutUnitRecord{AgencyID: entry.AgencyID,
			FirstObservedAt: entry.FirstObservedAt.UTC()})
	}
	for _, entry := range derived {
		record.Derived = append(record.Derived, readmodel.DerivedRecord{WorkflowID: entry.WorkflowID, ResultID: entry.ResultID,
			Evaluation: entry.Evaluation, VisibleTargets: entry.Visible})
	}
	transmissions, err := decodeTransmissions(r.transmissions)
	return record, transmissions, err
}

// decodeTransmissions decodes the supporting Transmissions of a detail row.
func decodeTransmissions(data []byte) ([]readmodel.TransmissionRecord, error) {
	var entries []struct {
		ID                uuid.UUID `json:"id"`
		AcceptedAt        time.Time `json:"acceptedAt"`
		ObservedAt        time.Time `json:"observedAt"`
		ProcessingStatus  string    `json:"processingStatus"`
		NetworkID         uuid.UUID `json:"networkId"`
		NetworkName       string    `json:"networkName"`
		NetworkAgencyID   uuid.UUID `json:"networkAgencyId"`
		NetworkUnresolved bool      `json:"networkUnresolved"`
		Protocol          string    `json:"protocol"`
		Incomplete        bool      `json:"incomplete"`
		Payload           string    `json:"payload"`
		Quality           *string   `json:"quality"`
		Targets           []struct {
			Position      int        `json:"position"`
			Resolution    string     `json:"resolution"`
			UnitID        *uuid.UUID `json:"unitId"`
			Encrypted     bool       `json:"encrypted"`
			AddressNumber *int64     `json:"addressNumber"`
			AddressText   *string    `json:"addressText"`
			Function      *int       `json:"function"`
		} `json:"targets"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	var out []readmodel.TransmissionRecord
	for _, entry := range entries {
		record := readmodel.TransmissionRecord{ID: entry.ID, AcceptedAt: entry.AcceptedAt.UTC(), ObservedAt: entry.ObservedAt.UTC(),
			ProcessingStatus: entry.ProcessingStatus, NetworkID: entry.NetworkID, NetworkName: entry.NetworkName,
			NetworkAgencyID: entry.NetworkAgencyID, NetworkUnresolved: entry.NetworkUnresolved,
			PayloadRecord: readmodel.PayloadRecord{Payload: json.RawMessage(entry.Payload), Protocol: entry.Protocol,
				Incomplete: entry.Incomplete}}
		if entry.Quality != nil {
			record.QualityMetrics = json.RawMessage(*entry.Quality)
		}
		for _, target := range entry.Targets {
			record.Targets = append(record.Targets, readmodel.TargetRecord{Position: target.Position, Resolution: target.Resolution,
				UnitID: target.UnitID, Encrypted: target.Encrypted, AddressNumber: target.AddressNumber,
				AddressText: target.AddressText, Function: target.Function})
		}
		out = append(out, record)
	}
	return out, nil
}

// Eligible implements readmodel.Store.
func (s *Store) Eligible(ctx context.Context, tx platform.TransactionContext, access authz.MessageAccess, ids []uuid.UUID) ([]uuid.UUID, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	p := &params{}
	elig := newEligibility(p, access)
	statement := `SELECT m.id FROM messages m WHERE m.id = ANY(` + p.uuids(ids) + `) AND EXISTS (
		SELECT 1 FROM message_units mu WHERE mu.message_id = m.id AND ` + elig.appearance(appearanceColumns("mu")) + `)
		ORDER BY m.first_observed_at DESC, m.id DESC`
	rows, err := executor.QueryContext(ctx, statement, p.values...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var found []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		found = append(found, id)
	}
	return found, rows.Err()
}
