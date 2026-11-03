// Command is a supplied pure Go/WASI fixture for the installed-command wire boundary.
package main

import (
	"encoding/json"
	"io"
	"os"
)

func main() {
	var request struct {
		Input map[string]struct {
			Kind  string          `json:"kind"`
			Value json.RawMessage `json:"value"`
		} `json:"input"`
	}
	if err := json.NewDecoder(io.LimitReader(os.Stdin, (1<<20)+1)).Decode(&request); err != nil {
		os.Exit(2)
	}
	mode := ""
	if field, ok := request.Input["mode"]; ok {
		if err := json.Unmarshal(field.Value, &mode); err != nil {
			os.Exit(2)
		}
	}
	switch mode {
	case "mixed":
		if _, err := io.WriteString(os.Stdout, `{"schemaVersion":1,"fields":{},"failure":"evaluation-failed"}`); err != nil {
			os.Exit(2)
		}
	case "private-failure":
		if _, err := io.WriteString(os.Stdout, `{"schemaVersion":1,"failure":"private source text"}`); err != nil {
			os.Exit(2)
		}
	case "fail":
		if _, err := io.WriteString(os.Stdout, `{"schemaVersion":1,"failure":"evaluation-failed"}`); err != nil {
			os.Exit(2)
		}
	case "loop":
		for {
		}
	default:
		response := struct {
			SchemaVersion int `json:"schemaVersion"`
			Fields        map[string]struct {
				Kind  string          `json:"kind"`
				Value json.RawMessage `json:"value"`
			} `json:"fields"`
		}{SchemaVersion: 1, Fields: request.Input}
		if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
			os.Exit(2)
		}
	}
}
