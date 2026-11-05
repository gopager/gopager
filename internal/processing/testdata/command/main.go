// Command is a supplied pure plugin fixture for the installed-invocation wire boundary (D-131).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/gopager/gopager/pluginsdk"
)

type field struct {
	Kind  string          `json:"kind"`
	Value json.RawMessage `json:"value"`
}

type response struct {
	SchemaVersion int              `json:"schemaVersion"`
	Fields        map[string]field `json:"fields,omitempty"`
	Failure       string           `json:"failure,omitempty"`
}

func text(value string) field {
	encoded, _ := json.Marshal(value)
	return field{Kind: "string", Value: encoded}
}

func handle(ctx context.Context, data []byte) ([]byte, error) {
	var request struct {
		InstanceID string           `json:"instanceId"`
		Input      map[string]field `json:"input"`
	}
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, err
	}
	mode := ""
	if value, ok := request.Input["mode"]; ok {
		if err := json.Unmarshal(value.Value, &mode); err != nil {
			return nil, err
		}
	}
	switch mode {
	case "mixed":
		return []byte(`{"schemaVersion":2,"fields":{},"failure":"evaluation-failed"}`), nil
	case "private-failure":
		return []byte(`{"schemaVersion":2,"failure":"private source text"}`), nil
	case "fail":
		return []byte(`{"schemaVersion":2,"failure":"evaluation-failed"}`), nil
	case "crash":
		os.Exit(3)
	case "loop":
		// Deliberately ignores ctx: the Core must kill the process at its deadline.
		for {
		}
	case "env":
		request.Input["text"] = text(os.Getenv("GOPAGER_TEST_CORE_SECRET"))
	case "instance":
		request.Input["text"] = text(request.InstanceID)
	}
	if ctx.Err() != nil {
		return nil, errors.New("cancelled")
	}
	return json.Marshal(response{SchemaVersion: 2, Fields: request.Input})
}

func main() {
	pluginsdk.Serve(pluginsdk.HandlerFunc(handle))
}
