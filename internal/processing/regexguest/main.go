// Command regexguest is the supplied isolated Core regex implementation, not a plugin.
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"unicode/utf8"
)

type request struct {
	Mode    string `json:"mode"`
	Pattern string `json:"pattern"`
	Text    string `json:"text,omitempty"`
}

type capture struct {
	Name  string  `json:"name"`
	Index int     `json:"index"`
	Text  *string `json:"text,omitempty"`
}

type response struct {
	Matched  bool      `json:"matched"`
	Captures []capture `json:"captures,omitempty"`
	Names    []string  `json:"names"`
	Failure  string    `json:"failure,omitempty"`
}

func main() {
	data, err := io.ReadAll(io.LimitReader(os.Stdin, (1<<20)+1))
	if err != nil || len(data) > 1<<20 || !utf8.Valid(data) {
		emit(response{Failure: "processing.regex-input-invalid"})
		return
	}
	var input request
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		emit(response{Failure: "processing.regex-input-invalid"})
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || (input.Mode != "inspect" && input.Mode != "match") ||
		len(input.Pattern) > 4096 || !utf8.ValidString(input.Pattern) || !utf8.ValidString(input.Text) {
		emit(response{Failure: "processing.regex-input-invalid"})
		return
	}
	compiled, err := regexp.Compile(input.Pattern)
	if err != nil {
		emit(response{Failure: "processing.regex-invalid"})
		return
	}
	output := response{Names: compiled.SubexpNames()}
	if input.Mode == "inspect" {
		emit(output)
		return
	}
	indexes := compiled.FindStringSubmatchIndex(input.Text)
	output.Matched = indexes != nil
	if output.Matched {
		for i, name := range output.Names {
			if i == 0 || name == "" {
				continue
			}
			value := capture{Name: name, Index: i}
			if indexes[i*2] >= 0 {
				text := input.Text[indexes[i*2]:indexes[i*2+1]]
				value.Text = &text
			}
			output.Captures = append(output.Captures, value)
		}
	}
	emit(output)
}

func emit(output response) {
	if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
		os.Exit(2)
	}
}
