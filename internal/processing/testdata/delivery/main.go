// Command delivery is a supplied Delivery plugin fixture for the host tests. Its behaviour is
// selected by the attempt body and it speaks the Delivery wire format by hand, independent of the
// SDK's Delivery helpers.
package main

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gopager/gopager/pluginsdk"
)

type request struct {
	Kind             string          `json:"kind"`
	InvocationID     string          `json:"invocationId"`
	InstanceID       string          `json:"instanceId"`
	InstanceRevision string          `json:"instanceRevision"`
	ArtifactDigest   string          `json:"artifactDigest"`
	CapabilityID     string          `json:"capabilityId"`
	Deadline         string          `json:"deadline"`
	Configuration    json.RawMessage `json:"configuration"`
	SecretVersions   json.RawMessage `json:"secretVersions"`
	Attempt          json.RawMessage `json:"attempt"`
}

type attempt struct {
	Body string `json:"body"`
}

// answer renders a response with an optional receipt and retry hint.
func answer(status, code string, retryAfterSeconds int, receipt any) ([]byte, error) {
	body := map[string]any{"status": status, "code": code}
	if retryAfterSeconds > 0 {
		body["retryAfterSeconds"] = retryAfterSeconds
	}
	if receipt != nil {
		body["receipt"] = receipt
	}
	return json.Marshal(map[string]any{"schemaVersion": 2, "attempt": body})
}

// summarize reports the shape of the received configuration without the full Secret values: the
// title, and for each Secret its length and first characters.
func summarize(configuration json.RawMessage) map[string]any {
	var typed struct {
		Value map[string]struct {
			Value string `json:"value"`
		} `json:"value"`
	}
	if err := json.Unmarshal(configuration, &typed); err != nil {
		return map[string]any{"error": "unreadable configuration"}
	}
	summary := map[string]any{"title": typed.Value["title"].Value}
	for _, name := range []string{"apiToken", "userKey"} {
		member, present := typed.Value[name]
		if !present {
			summary[name] = nil
			continue
		}
		head := member.Value
		if len(head) > 6 {
			head = head[:6]
		}
		summary[name] = map[string]any{"length": len(member.Value), "head": head}
	}
	return summary
}

func handle(_ context.Context, data []byte) ([]byte, error) {
	var in request
	if err := json.Unmarshal(data, &in); err != nil {
		return nil, err
	}
	var current attempt
	if err := json.Unmarshal(in.Attempt, &current); err != nil {
		return nil, err
	}
	switch current.Body {
	case "accept":
		return answer("accepted", "ok", 0, map[string]bool{"ok": true})
	case "transient":
		return answer("transient", "provider-busy", 30, nil)
	case "permanent":
		return answer("permanent", "rejected", 0, nil)
	case "uncertain":
		return answer("uncertain", "provider-timeout", 0, nil)
	case "echo-config":
		return answer("accepted", "ok", 0, in.Configuration)
	case "config-summary":
		return answer("accepted", "ok", 0, summarize(in.Configuration))
	case "echo-request":
		return answer("accepted", "ok", 0, map[string]any{"kind": in.Kind, "invocationId": in.InvocationID,
			"instanceId": in.InstanceID, "instanceRevision": in.InstanceRevision, "artifactDigest": in.ArtifactDigest,
			"capabilityId": in.CapabilityID, "deadline": in.Deadline, "secretVersions": in.SecretVersions,
			"attempt": in.Attempt})
	case "pid":
		return answer("accepted", "ok", 0, map[string]int{"pid": os.Getpid()})
	case "bad-code":
		return answer("accepted", "Not A Valid Code!", 0, nil)
	case "huge":
		return answer("accepted", "ok", 0, strings.Repeat("x", 300<<10))
	case "hang":
		// Deliberately ignores the context: the Core must kill the process at its deadline.
		time.Sleep(time.Hour)
	case "crash":
		os.Exit(3)
	case "slow":
		time.Sleep(300 * time.Millisecond)
		return answer("accepted", "ok", 0, nil)
	}
	return answer("permanent", "unknown-body-"+strconv.Itoa(len(current.Body)), 0, nil)
}

func main() {
	pluginsdk.Serve(pluginsdk.HandlerFunc(handle))
}
