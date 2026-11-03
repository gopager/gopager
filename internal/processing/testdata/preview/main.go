// Preview is an independently supplied pure WASI command fixture, not the processing entrypoint.
package main

import (
	"encoding/json"
	"io"
	"os"
)

type field struct {
	Kind  string          `json:"kind"`
	Value json.RawMessage `json:"value"`
}

func main() {
	var request struct {
		Input map[string]field `json:"input"`
	}
	if err := json.NewDecoder(io.LimitReader(os.Stdin, (1<<20)+1)).Decode(&request); err != nil {
		os.Exit(2)
	}
	if text, present := request.Input["text"]; present {
		var value string
		if err := json.Unmarshal(text.Value, &value); err != nil {
			os.Exit(2)
		}
		encoded, err := json.Marshal("PREVIEW:" + value)
		if err != nil {
			os.Exit(2)
		}
		request.Input["text"] = field{Kind: "string", Value: encoded}
	}
	if err := json.NewEncoder(os.Stdout).Encode(struct {
		SchemaVersion int              `json:"schemaVersion"`
		Fields        map[string]field `json:"fields"`
	}{SchemaVersion: 1, Fields: request.Input}); err != nil {
		os.Exit(2)
	}
}
