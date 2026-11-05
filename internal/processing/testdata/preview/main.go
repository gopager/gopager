// Preview is an independently supplied pure plugin fixture, not the processing entrypoint (D-131).
package main

import (
	"context"
	"encoding/json"

	"github.com/gopager/gopager/pluginsdk"
)

type field struct {
	Kind  string          `json:"kind"`
	Value json.RawMessage `json:"value"`
}

func handle(_ context.Context, data []byte) ([]byte, error) {
	var request struct {
		Input map[string]field `json:"input"`
	}
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, err
	}
	if text, present := request.Input["text"]; present {
		var value string
		if err := json.Unmarshal(text.Value, &value); err != nil {
			return nil, err
		}
		encoded, err := json.Marshal("PREVIEW:" + value)
		if err != nil {
			return nil, err
		}
		request.Input["text"] = field{Kind: "string", Value: encoded}
	}
	return json.Marshal(struct {
		SchemaVersion int              `json:"schemaVersion"`
		Fields        map[string]field `json:"fields"`
	}{SchemaVersion: 2, Fields: request.Input})
}

func main() {
	pluginsdk.Serve(pluginsdk.HandlerFunc(handle))
}
