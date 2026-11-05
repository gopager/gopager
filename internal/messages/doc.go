// Package messages groups accepted Transmissions into Messages.
//
// Decide is a pure function: two Transmissions belong to the same Message when
// their Payload identities are exactly equal and their observation times lie
// within the policy's inclusive whole-second window. A Transmission matching
// several existing Messages merges them into the one with the lowest UUID.
// Decide performs no I/O; callers must load candidates and apply the Decision
// within one transaction.
package messages
