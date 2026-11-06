// Command is a supplied pure plugin fixture for the installed-invocation wire boundary.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"

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

func textField(value string) field {
	encoded, _ := json.Marshal(value)
	return field{Kind: "string", Value: encoded}
}

func handle(ctx context.Context, data []byte) ([]byte, error) {
	var request struct {
		InstanceID string           `json:"instanceId"`
		Mode       string           `json:"mode"`
		Input      map[string]field `json:"input"`
	}
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, err
	}
	if request.Mode == "preview" {
		// Preview never runs the live behaviours below (including their failures and effects).
		if value, present := request.Input["text"]; present {
			var text string
			if err := json.Unmarshal(value.Value, &text); err != nil {
				return nil, err
			}
			request.Input["text"] = textField("PREVIEW:" + text)
		}
		return json.Marshal(response{SchemaVersion: 2, Fields: request.Input})
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
		request.Input["text"] = textField(os.Getenv("GOPAGER_TEST_CORE_SECRET"))
	case "pid":
		request.Input["text"] = textField(strconv.Itoa(os.Getpid()))
	case "instance":
		request.Input["text"] = textField(request.InstanceID)
	}
	if ctx.Err() != nil {
		return nil, errors.New("cancelled")
	}
	return json.Marshal(response{SchemaVersion: 2, Fields: request.Input})
}

func main() {
	pluginsdk.Serve(pluginsdk.HandlerFunc(handle))
}
