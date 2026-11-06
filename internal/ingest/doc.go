// Package ingest accepts Transmissions from authenticated Gateways and
// administers Gateway Network assignments and credentials.
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
// GatewayAdministration provides the authorized administrative operations
// (ConfigureNetworks, ReadCredentialStatus, IssueCredential, RevokeCredential);
// mutations are revision-guarded and the credential commands are idempotent
// through command receipts.
package ingest
