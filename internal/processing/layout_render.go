package processing

import (
	"context"
	"errors"
	"net/url"
	"strings"
)

// RenderedLayout is the bounded output of LayoutProgram.Render.
type RenderedLayout struct {
	// OutputKind is the layout's output kind: LayoutText, LayoutMarkdown or LayoutJSON.
	OutputKind string
	// Body is UTF-8 text, Markdown or a JSON array, at most MaxLayoutOutputBytes long.
	Body []byte
}

// layoutItem is one rendered unit before it is serialized for an output kind.
type layoutItem struct {
	kind     string
	text     string
	label    string
	title    string
	url      string
	value    *Value
	children []layoutItem
}

// heldSize is what the renderer holds for the item, whatever the output kind emits of it.
func (i layoutItem) heldSize() int {
	size := len(i.label) + len(i.title) + len(i.url) + len(i.text)
	if i.value != nil {
		size += len(i.value.Value)
	}
	return size
}

// size estimates the item's share of the output: JSON output carries a field's typed value and
// the texts as given; text and Markdown carry the formatted texts without control characters,
// never both. The serializer checks the exact length.
func (i layoutItem) size(json bool) int {
	if json {
		size := len(i.label) + len(i.title) + len(i.url)
		if i.value != nil {
			return size + len(i.value.Value)
		}
		return size + len(i.text)
	}
	return len(sanitizeLayoutText(i.label)) + len(sanitizeLayoutText(i.title)) + len(i.url) + len(sanitizeLayoutText(i.text))
}

// Render evaluates the layout against reader and returns the body for the program's output
// kind. Absent and null fields render nothing; rendering is deterministic and fails with a
// resource-limit failure as soon as the output would exceed MaxLayoutOutputBytes.
func (p *LayoutProgram) Render(ctx context.Context, reader FieldReader) (RenderedLayout, error) {
	if p == nil || ctx == nil || reader == nil {
		return RenderedLayout{}, invalid(errors.New("missing layout program/reader/context"))
	}
	r := &layoutRenderer{ctx: ctx, reader: reader, cache: make(map[string]*Value), json: p.outputKind == LayoutJSON}
	items, err := r.blocks(p.nodes)
	if err != nil {
		return RenderedLayout{}, err
	}
	body, err := serializeLayout(p.outputKind, items)
	if err != nil {
		return RenderedLayout{}, err
	}
	return RenderedLayout{OutputKind: p.outputKind, Body: body}, nil
}

type layoutRenderer struct {
	ctx    context.Context
	reader FieldReader
	cache  map[string]*Value
	size   int
	// json selects the JSON representation for the size estimate.
	json bool
	// held counts what the rendered items hold, with or without content, and cached the values
	// looked up for the render; together they are bounded so that no render grows without limit.
	// held is given back with a discarded fallback alternative, cached is not.
	held   int
	cached int
	// trying counts the fallback alternatives being tried. Their output without content is kept in
	// pending instead of size: it counts once the alternative is chosen, and is dropped with a
	// discarded alternative, which never fails the output bound.
	trying  int
	pending int
}

// maxHeldLayoutBytes bounds what one render may hold at a time.
const maxHeldLayoutBytes = 16 * MaxLayoutOutputBytes

func (r *layoutRenderer) charge(items []layoutItem) error {
	for _, item := range items {
		r.held += item.heldSize()
		if r.trying > 0 && !hasContent([]layoutItem{item}) {
			r.pending += item.size(r.json)
		} else {
			r.size += item.size(r.json)
		}
	}
	if r.size > MaxLayoutOutputBytes || r.held+r.cached > maxHeldLayoutBytes {
		return resource(errors.New("layout output boundary"))
	}
	return nil
}

// value returns the field's value, or nil when it is absent or an explicit null.
func (r *layoutRenderer) value(ref FieldRef, schema FieldSchema) (*Value, error) {
	if cached, ok := r.cache[ref.key()]; ok {
		return cached, nil
	}
	field, err := r.reader.LookupField(r.ctx, ref)
	if err != nil {
		return nil, err
	}
	var result *Value
	switch {
	case field.State == FieldAbsent && field.Value == nil:
	case field.State == FieldPresent && field.Value != nil:
		if _, err := field.Value.Native(schema); err != nil {
			return nil, err
		}
		if field.Value.Kind != NullValue {
			result = field.Value
		}
	default:
		return nil, invalid(errors.New("invalid layout field state"))
	}
	if result != nil {
		// Cached values live as long as the render, also those of a discarded fallback
		// alternative, so they are counted once and never given back.
		r.cached += len(result.Value)
		if r.cached+r.held > maxHeldLayoutBytes {
			return nil, resource(errors.New("layout value boundary"))
		}
	}
	r.cache[ref.key()] = result
	return result, nil
}

func (r *layoutRenderer) blocks(nodes []layoutNode) ([]layoutItem, error) {
	var items []layoutItem
	for _, node := range nodes {
		if err := r.ctx.Err(); err != nil {
			return nil, err
		}
		produced, err := r.block(node)
		if err != nil {
			return nil, err
		}
		items = append(items, produced...)
	}
	return items, nil
}

func (r *layoutRenderer) block(node layoutNode) ([]layoutItem, error) {
	switch node.kind {
	case blockText:
		return r.emit(layoutItem{kind: blockText, text: node.text})
	case blockField, blockChip:
		value, err := r.value(node.field, node.schema)
		if err != nil || value == nil {
			return nil, err
		}
		formatted, err := formatLayoutValue(*value)
		if err != nil {
			return nil, err
		}
		return r.emit(layoutItem{kind: node.kind, label: node.label, text: formatted, value: value})
	case blockLink:
		target, err := r.link(node)
		if err != nil || target == "" {
			return nil, err
		}
		return r.emit(layoutItem{kind: blockLink, label: node.label, url: target})
	case blockSection:
		children, err := r.blocks(node.children)
		if err != nil || len(children) == 0 {
			return nil, err
		}
		return r.emit(layoutItem{kind: blockSection, title: node.title, children: children})
	case blockConditional:
		matched, err := node.when.Evaluate(r.ctx, r.reader)
		if err != nil {
			return nil, err
		}
		if matched {
			return r.blocks(node.then)
		}
		return r.blocks(node.otherwise)
	default: // blockFallback
		for _, alternative := range node.children {
			before, held, pending := r.size, r.held, r.pending
			r.trying++
			items, err := r.block(alternative)
			r.trying--
			if err != nil {
				return nil, err
			}
			if hasContent(items) {
				if r.trying == 0 {
					// The chosen alternative's output without content is now part of the body.
					r.size, r.pending = r.size+r.pending-pending, pending
					if r.size > MaxLayoutOutputBytes {
						return nil, resource(errors.New("layout output boundary"))
					}
				}
				return items, nil
			}
			r.size, r.held, r.pending = before, held, pending
		}
		return nil, nil
	}
}

// hasContent reports whether rendered items show anything: a label, title, URL or text that is
// not empty once control characters are removed, somewhere. A present but empty (or
// control-only), unlabelled value is no content, in every output kind.
func hasContent(items []layoutItem) bool {
	for _, item := range items {
		if sanitizeLayoutText(item.label) != "" || sanitizeLayoutText(item.title) != "" || item.url != "" ||
			sanitizeLayoutText(item.text) != "" || hasContent(item.children) {
			return true
		}
	}
	return false
}

// emit charges a single item against the output budget. Children of a section were charged
// when they were produced, so only the section's own text is added here.
func (r *layoutRenderer) emit(item layoutItem) ([]layoutItem, error) {
	own := item
	own.children = nil
	if err := r.charge([]layoutItem{own}); err != nil {
		return nil, err
	}
	return []layoutItem{item}, nil
}

// link substitutes the escaped field values into the template and returns the canonical URL,
// or "" when a field is absent or the result is not an http, https or mailto URL.
func (r *layoutRenderer) link(node layoutNode) (string, error) {
	var target strings.Builder
	for _, part := range node.url {
		if part.index < 0 {
			target.WriteString(part.literal)
			continue
		}
		value, err := r.value(node.urlField[part.index], node.urlTypes[part.index])
		if err != nil || value == nil {
			return "", err
		}
		formatted, err := formatLayoutValue(*value)
		if err != nil {
			return "", err
		}
		target.WriteString(escapeURLValue(formatted))
		if target.Len() > MaxLayoutOutputBytes {
			return "", resource(errors.New("layout output boundary"))
		}
	}
	parsed, err := url.Parse(target.String())
	if err != nil {
		return "", nil
	}
	switch parsed.Scheme {
	case "http", "https":
		if parsed.Host == "" {
			return "", nil
		}
	case "mailto":
	default:
		return "", nil
	}
	return parsed.String(), nil
}

// escapeURLValue percent-encodes a field value for any position of a link template: everything
// except the RFC 3986 unreserved characters and "@" is encoded, so a value can never add a path
// segment, query parameter (&, =), plus-as-space or fragment of its own. "@" stays literal for
// readable mailto addresses.
func escapeURLValue(value string) string {
	const hex = "0123456789ABCDEF"
	var escaped strings.Builder
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
			c == '-' || c == '.' || c == '_' || c == '~' || c == '@' {
			escaped.WriteByte(c)
			continue
		}
		escaped.WriteByte('%')
		escaped.WriteByte(hex[c>>4])
		escaped.WriteByte(hex[c&15])
	}
	return escaped.String()
}
