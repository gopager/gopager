package processing

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
)

// formatLayoutValue renders a typed value as display text: strings verbatim, integers and
// numbers as encoded, timestamps as RFC 3339 UTC, bytes as their canonical Base64, lists as
// their formatted elements joined with ", " and objects as the compact typed encoding.
func formatLayoutValue(v Value) (string, error) {
	switch v.Kind {
	case StringValue, IntegerValue, BytesValue:
		var text string
		if err := Decode(v.Value, &text); err != nil {
			return "", err
		}
		return text, nil
	case NumberValue:
		return string(bytes.TrimSpace(v.Value)), nil
	case BooleanValue:
		var boolean bool
		if err := Decode(v.Value, &boolean); err != nil {
			return "", err
		}
		if boolean {
			return "true", nil
		}
		return "false", nil
	case TimestampValue:
		var text string
		if err := Decode(v.Value, &text); err != nil {
			return "", err
		}
		instant, err := time.Parse(time.RFC3339Nano, text)
		if err != nil {
			return "", invalid(err)
		}
		return instant.UTC().Format(time.RFC3339Nano), nil
	case ListValue:
		var elements []Value
		if err := Decode(v.Value, &elements); err != nil {
			return "", err
		}
		formatted := make([]string, len(elements))
		for i, element := range elements {
			text, err := formatLayoutValue(element)
			if err != nil {
				return "", err
			}
			formatted[i] = text
		}
		return strings.Join(formatted, ", "), nil
	case ObjectValue:
		var compact bytes.Buffer
		if err := json.Compact(&compact, v.Value); err != nil {
			return "", invalid(err)
		}
		return compact.String(), nil
	case NullValue:
		return "null", nil
	}
	return "", invalid(errors.New("unsupported layout value kind"))
}

// sanitizeLayoutText drops carriage returns and other control characters, turns tabs into
// spaces and keeps newlines, so that interpolated text cannot smuggle terminal control
// sequences into an output.
func sanitizeLayoutText(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n':
			return r
		case r == '\t':
			return ' '
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, s)
}

const markdownMetacharacters = "\\`*_{}[]()#+-.!|<>~=&"

// escapeMarkdown backslash-escapes every Markdown metacharacter, including the angle
// brackets that would otherwise start raw HTML, `~` (strikethrough, fenced code), `=` (setext
// headings) and `&` (entity references).
func escapeMarkdown(s string) string {
	var escaped strings.Builder
	for _, r := range s {
		if strings.ContainsRune(markdownMetacharacters, r) {
			escaped.WriteByte('\\')
		}
		escaped.WriteRune(r)
	}
	return escaped.String()
}

// markdownURLEscaper encodes the characters that would end or escape a Markdown link destination
// and backslash-escapes `&`, which Markdown would otherwise read as the start of an entity
// reference such as `&copy;`; `\&` stays a literal `&`.
var markdownURLEscaper = strings.NewReplacer(" ", "%20", "(", "%28", ")", "%29", "<", "%3C", ">", "%3E", "\\", "%5C",
	"&", "\\&")

func serializeLayout(kind string, items []layoutItem) ([]byte, error) {
	if kind == LayoutJSON {
		return serializeLayoutJSON(items)
	}
	var lines []string
	size := 0
	var walk func([]layoutItem) error
	walk = func(items []layoutItem) error {
		for _, item := range items {
			for _, line := range itemLines(item, kind == LayoutMarkdown) {
				if len(lines) > 0 {
					size++ // the separating newline
				}
				size += len(line)
				if size > MaxLayoutOutputBytes {
					return resource(errors.New("layout output boundary"))
				}
				lines = append(lines, line)
			}
			if err := walk(item.children); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(items); err != nil {
		return nil, err
	}
	return []byte(strings.Join(lines, "\n")), nil
}

// itemLines returns the line an item itself contributes; a section contributes only its
// title line, its children are walked by the caller.
func itemLines(item layoutItem, markdown bool) []string {
	clean := func(s string) string {
		s = sanitizeLayoutText(s)
		if markdown {
			return escapeMarkdown(s)
		}
		return s
	}
	text, label := clean(item.text), clean(item.label)
	switch item.kind {
	case blockText:
		return []string{text}
	case blockField:
		switch {
		case label == "":
			return []string{text}
		case markdown:
			return []string{"**" + label + ":** " + text}
		}
		return []string{label + ": " + text}
	case blockChip:
		inner := text
		if label != "" {
			inner = label + ": " + text
		}
		if markdown {
			return []string{"\\[" + inner + "\\]"}
		}
		return []string{"[" + inner + "]"}
	case blockLink:
		if markdown {
			return []string{"[" + label + "](" + markdownURLEscaper.Replace(item.url) + ")"}
		}
		return []string{label + ": " + item.url}
	case blockSection:
		switch {
		case item.title == "":
			return nil
		case markdown:
			return []string{"**" + clean(item.title) + "**"}
		}
		return []string{clean(item.title)}
	}
	return nil
}

// jsonLayoutItem is the JSON output form of a layoutItem.
type jsonLayoutItem struct {
	Kind  string           `json:"kind"`
	Text  string           `json:"text,omitempty"`
	Label string           `json:"label,omitempty"`
	Title string           `json:"title,omitempty"`
	Value *Value           `json:"value,omitempty"`
	URL   string           `json:"url,omitempty"`
	Items []jsonLayoutItem `json:"items,omitempty"`
}

func toJSONItems(items []layoutItem) []jsonLayoutItem {
	converted := make([]jsonLayoutItem, 0, len(items))
	for _, item := range items {
		entry := jsonLayoutItem{Kind: item.kind, Label: item.label, Title: item.title, Value: item.value,
			URL: item.url, Items: toJSONItems(item.children)}
		if item.kind == blockText {
			entry.Text = item.text
		}
		if len(entry.Items) == 0 {
			entry.Items = nil
		}
		converted = append(converted, entry)
	}
	return converted
}

func serializeLayoutJSON(items []layoutItem) ([]byte, error) {
	body, err := json.Marshal(toJSONItems(items))
	if err != nil {
		return nil, invalid(err)
	}
	if len(body) > MaxLayoutOutputBytes {
		return nil, resource(errors.New("layout output boundary"))
	}
	return body, nil
}
