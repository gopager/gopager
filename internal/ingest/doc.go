// Package ingest accepts Transmissions from authenticated Gateways, derives
// Gateway state, and administers Networks and Gateways.
//
// Service.Submit authenticates the Gateway bearer credential, validates the
// Submission, resolves the target Network (declared identifier, single
// protocol-compatible Network, unique address-rule match, or the highest
// Gateway Network priority among the matching or all Networks), matches targets and stores the Transmission, all in one
// transaction using the database clock. Submissions are idempotent per
// Gateway and Submission ID: a retry with the same fingerprint returns the
// original Receipt, a different fingerprint fails with ErrSubmissionConflict.
// Rejections with a public code (invalid input, resource limit, idempotency
// conflict) by an authenticated Gateway are recorded durably in a separate
// transaction. Service.ReportState records Gateway state reports.
//
// NextGatewayState defines the state a contact leads to; every effective state
// change is recorded as one GatewayTransition with one gateway.state-changed fact.
// ContactMonitor is the worker-role runner that sets silent Gateways
// contact-lost against the database clock.
//
// NetworkAdministration provides the Network lifecycle and GatewayAdministration
// the Gateway lifecycle, diagnostics, history, Network assignments and
// credentials. Mutations are revision-guarded, creating commands are idempotent
// through command receipts. An object on which the caller holds no right is not
// found; one on which it holds another right but not the requested one is
// forbidden. Credential and diagnostics operations also accept the single-Gateway
// delegation actions gateway.credential.rotate and gateway.diagnostics.read.
package ingest
