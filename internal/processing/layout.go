package processing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/icons"
	"github.com/gopager/gopager/internal/platform"
)

// LayoutSchemaVersion is the only supported value of Layout.SchemaVersion.
const LayoutSchemaVersion = 1

// The output kinds a Layout can render to.
const (
	// LayoutText renders plain text lines.
	LayoutText = "text"
	// LayoutMarkdown renders Markdown with every interpolated value escaped; it never emits HTML.
	LayoutMarkdown = "markdown"
	// LayoutJSON renders a JSON array of typed items.
	LayoutJSON = "json"
)

// MaxLayoutOutputBytes bounds one rendered layout body (64 KiB).
const MaxLayoutOutputBytes = 64 << 10

// Structural limits of a layout definition. Exceeding one is a resource-limit failure.
const (
	maxLayoutBlocks       = 256
	maxLayoutDepth        = 8
	maxLayoutLiteralBytes = 4096
	maxLayoutLabelBytes   = 256
	maxLayoutTemplate     = 2048
	maxLayoutURLFields    = 10
	maxFallbackAlts       = 8
)

// The block kinds of a Layout.
const (
	blockText        = "text"
	blockField       = "field"
	blockChip        = "chip"
	blockLink        = "link"
	blockSection     = "section"
	blockConditional = "conditional"
	blockFallback    = "fallback"
)

// Layout is a declarative arrangement of typed fields for one output kind. It is data, not a
// template: it has no expressions, loops or markup, and it never produces HTML.
type Layout struct {
	// SchemaVersion must equal LayoutSchemaVersion.
	SchemaVersion int `json:"schemaVersion"`
	// OutputKind is LayoutText, LayoutMarkdown or LayoutJSON.
	OutputKind string `json:"outputKind"`
	// Blocks is the non-empty, ordered top-level block list.
	Blocks []LayoutBlock `json:"blocks"`
}

// LayoutBlock is one element of a Layout. Kind selects the block type and determines which
// of the other properties are allowed; any property not belonging to the kind is rejected.
//
//   - text: Text.
//   - field, chip: Field, optional Label and optional Icon. A missing label or icon is taken
//     from the field's declaration when the declarations provide FieldPresentation.
//   - link: Label and URL.
//   - section: optional Title and Blocks.
//   - conditional: When, Then and optional Else.
//   - fallback: Blocks, the 2 to 8 alternatives; the first non-empty one is rendered.
type LayoutBlock struct {
	Kind   string        `json:"kind"`
	Text   string        `json:"text,omitempty"`
	Field  *FieldRef     `json:"field,omitempty"`
	Label  string        `json:"label,omitempty"`
	Icon   string        `json:"icon,omitempty"`
	Title  string        `json:"title,omitempty"`
	URL    *LayoutURL    `json:"url,omitempty"`
	When   *Predicate    `json:"when,omitempty"`
	Blocks []LayoutBlock `json:"blocks,omitempty"`
	Then   []LayoutBlock `json:"then,omitempty"`
	Else   []LayoutBlock `json:"else,omitempty"`
}

// LayoutURL builds a link target from a template and field values. The template holds the
// placeholders {0} to {9}, each indexing into Fields; every placeholder needs a field and
// every field must be used. At render time each value is formatted and escaped with
// net/url.PathEscape before substitution.
type LayoutURL struct {
	Template string     `json:"template"`
	Fields   []FieldRef `json:"fields"`
}

// LayoutRequest selects the layout a delivery needs: an explicitly configured layout, or the
// built-in default for the output kind when LayoutID is nil.
type LayoutRequest struct {
	LayoutID   *uuid.UUID
	OutputKind string
}

// ResolvedLayout is a layout together with its identity. ID uuid.Nil and Version 0 denote the
// built-in default layout.
type ResolvedLayout struct {
	ID      uuid.UUID
	Version int64
	Layout  Layout
}

// LayoutResolver loads the layout a delivery renders with. The delivery side implements it;
// this package only defines the port.
type LayoutResolver interface {
	// ResolveLayout returns the layout selected by request, or a NOT_FOUND failure.
	ResolveLayout(ctx context.Context, tx platform.TransactionContext, request LayoutRequest) (ResolvedLayout, error)
}

// DecodeLayout strictly decodes a layout document, rejecting unknown properties, duplicate
// keys and trailing data, and then validates its structure and limits.
func DecodeLayout(raw []byte) (Layout, error) {
	var layout Layout
	if err := Decode(raw, &layout); err != nil {
		return Layout{}, err
	}
	if err := layout.validate(); err != nil {
		return Layout{}, err
	}
	return layout, nil
}

// property bits identify which optional LayoutBlock properties are populated.
type property uint16

const (
	propText property = 1 << iota
	propField
	propLabel
	propTitle
	propURL
	propWhen
	propBlocks
	propThen
	propElse
	propIcon
)

func (b LayoutBlock) properties() property {
	var set property
	for _, item := range []struct {
		present bool
		bit     property
	}{
		{b.Text != "", propText}, {b.Field != nil, propField}, {b.Label != "", propLabel},
		{b.Title != "", propTitle}, {b.URL != nil, propURL}, {b.When != nil, propWhen},
		{b.Blocks != nil, propBlocks}, {b.Then != nil, propThen}, {b.Else != nil, propElse},
		{b.Icon != "", propIcon},
	} {
		if item.present {
			set |= item.bit
		}
	}
	return set
}

func (l Layout) validate() error {
	if l.SchemaVersion != LayoutSchemaVersion {
		return invalid(fmt.Errorf("unsupported layout schema version %d", l.SchemaVersion))
	}
	switch l.OutputKind {
	case LayoutText, LayoutMarkdown, LayoutJSON:
	default:
		return invalid(errors.New("unknown layout output kind"))
	}
	if len(l.Blocks) == 0 {
		return invalid(errors.New("layout has no blocks"))
	}
	count := 0
	return validateLayoutBlocks(l.Blocks, 1, &count)
}

func validateLayoutBlocks(blocks []LayoutBlock, depth int, count *int) error {
	if depth > maxLayoutDepth {
		return resource(errors.New("layout nesting boundary"))
	}
	for _, block := range blocks {
		*count++
		if *count > maxLayoutBlocks {
			return resource(errors.New("layout block count boundary"))
		}
		if err := block.validate(depth, count); err != nil {
			return err
		}
	}
	return nil
}

func checkLayoutText(value string, limit int, required bool) error {
	switch {
	case len(value) > limit:
		return resource(errors.New("layout text boundary"))
	case required && value == "":
		return invalid(errors.New("layout text is empty"))
	case !utf8.ValidString(value) || strings.ContainsRune(value, '\x00'):
		return invalid(errors.New("layout text is not valid UTF-8"))
	}
	return nil
}

func (b LayoutBlock) validate(depth int, count *int) error {
	var allowed property
	switch b.Kind {
	case blockText:
		allowed = propText
	case blockField, blockChip:
		allowed = propField | propLabel | propIcon
	case blockLink:
		allowed = propLabel | propURL
	case blockSection:
		allowed = propTitle | propBlocks
	case blockConditional:
		allowed = propWhen | propThen | propElse
	case blockFallback:
		allowed = propBlocks
	default:
		return invalid(errors.New("unknown layout block kind"))
	}
	if b.properties()&^allowed != 0 {
		return invalid(fmt.Errorf("property not allowed on %s block", b.Kind))
	}
	if err := checkLayoutText(b.Label, maxLayoutLabelBytes, b.Kind == blockLink); err != nil {
		return err
	}
	if err := checkLayoutText(b.Title, maxLayoutLabelBytes, false); err != nil {
		return err
	}
	if b.Icon != "" && !icons.Valid(b.Icon) {
		return invalid(errors.New("invalid layout icon"))
	}
	switch b.Kind {
	case blockText:
		return checkLayoutText(b.Text, maxLayoutLiteralBytes, true)
	case blockField, blockChip:
		if b.Field == nil {
			return invalid(errors.New("field block requires a field"))
		}
		return b.Field.validateShape()
	case blockLink:
		return b.URL.validate()
	case blockSection:
		if len(b.Blocks) == 0 {
			return invalid(errors.New("section requires blocks"))
		}
		return validateLayoutBlocks(b.Blocks, depth+1, count)
	case blockConditional:
		if b.When == nil || len(b.Then) == 0 {
			return invalid(errors.New("conditional requires when and then"))
		}
		if err := validateLayoutBlocks(b.Then, depth+1, count); err != nil {
			return err
		}
		return validateLayoutBlocks(b.Else, depth+1, count)
	default: // blockFallback
		if len(b.Blocks) < 2 || len(b.Blocks) > maxFallbackAlts {
			return invalid(errors.New("fallback requires 2 to 8 alternatives"))
		}
		return validateLayoutBlocks(b.Blocks, depth+1, count)
	}
}

func (u *LayoutURL) validate() error {
	if u == nil {
		return invalid(errors.New("link requires a url"))
	}
	if len(u.Fields) > maxLayoutURLFields {
		return resource(errors.New("link url field boundary"))
	}
	if err := checkLayoutText(u.Template, maxLayoutTemplate, true); err != nil {
		return err
	}
	used := make([]bool, len(u.Fields))
	for _, part := range splitTemplate(u.Template) {
		if part.index < 0 {
			if part.invalid {
				return invalid(errors.New("malformed link url placeholder"))
			}
			continue
		}
		if part.index >= len(u.Fields) {
			return invalid(errors.New("link url placeholder has no field"))
		}
		used[part.index] = true
	}
	for _, ok := range used {
		if !ok {
			return invalid(errors.New("link url field is unused"))
		}
	}
	for _, field := range u.Fields {
		if err := field.validateShape(); err != nil {
			return err
		}
	}
	return nil
}

// templatePart is a literal run (index < 0) or a placeholder reference. A literal run with
// invalid set holds a brace that is not part of a placeholder.
type templatePart struct {
	literal string
	index   int
	invalid bool
}

func splitTemplate(template string) []templatePart {
	var parts []templatePart
	start := 0
	for i := 0; i < len(template); i++ {
		switch template[i] {
		case '{':
			if i+2 < len(template) && template[i+1] >= '0' && template[i+1] <= '9' && template[i+2] == '}' {
				parts = append(parts, templatePart{literal: template[start:i], index: -1},
					templatePart{index: int(template[i+1] - '0')})
				i += 2
				start = i + 1
				continue
			}
			return append(parts, templatePart{index: -1, invalid: true})
		case '}':
			return append(parts, templatePart{index: -1, invalid: true})
		}
	}
	return append(parts, templatePart{literal: template[start:], index: -1})
}

// layoutNode is the compiled form of one LayoutBlock.
type layoutNode struct {
	kind      string
	text      string
	label     string
	icon      string
	title     string
	field     FieldRef
	schema    FieldSchema
	url       []templatePart
	urlField  []FieldRef
	urlTypes  []FieldSchema
	when      *PredicateProgram
	children  []layoutNode
	then      []layoutNode
	otherwise []layoutNode
}

// LayoutProgram is a validated, immutable layout bound to the caller's field declarations.
// It is safe for concurrent use.
type LayoutProgram struct {
	outputKind string
	nodes      []layoutNode
	fields     []FieldRef
}

// CompileLayout validates the layout structure and limits, resolves every field reference
// against declarations and compiles every predicate. Unknown field references fail with the
// declarations' error, wrapped as INVALID_INPUT when it is not already a platform failure.
func CompileLayout(ctx context.Context, layout Layout, declarations Declarations) (*LayoutProgram, error) {
	if ctx == nil || declarations == nil {
		return nil, invalid(errors.New("missing layout declarations/context"))
	}
	if err := layout.validate(); err != nil {
		return nil, err
	}
	compiler := &layoutCompiler{ctx: ctx, declarations: declarations, seen: make(map[string]bool)}
	nodes, err := compiler.compile(layout.Blocks)
	if err != nil {
		return nil, err
	}
	return &LayoutProgram{outputKind: layout.OutputKind, nodes: nodes, fields: compiler.fields}, nil
}

type layoutCompiler struct {
	ctx          context.Context
	declarations Declarations
	fields       []FieldRef
	seen         map[string]bool
}

func (c *layoutCompiler) record(ref FieldRef) {
	if c.seen[ref.key()] {
		return
	}
	c.seen[ref.key()] = true
	c.fields = append(c.fields, FieldRef{Namespace: ref.Namespace, Path: append([]string(nil), ref.Path...)})
}

func (c *layoutCompiler) schema(ref FieldRef) (FieldSchema, error) {
	if err := c.ctx.Err(); err != nil {
		return FieldSchema{}, err
	}
	schema, err := c.declarations.FieldSchema(ref)
	if err != nil {
		var failure *platform.Failure
		if errors.As(err, &failure) {
			return FieldSchema{}, err
		}
		return FieldSchema{}, invalid(err)
	}
	if err := schema.Validate(); err != nil {
		return FieldSchema{}, err
	}
	c.record(ref)
	return cloneSchema(schema), nil
}

// presentation returns the label and icon of a field block: the block's own, else the
// declaration's.
func (c *layoutCompiler) presentation(ref FieldRef, label, icon string) (string, string) {
	if label != "" && icon != "" {
		return label, icon
	}
	source, ok := c.declarations.(FieldPresentation)
	if !ok {
		return label, icon
	}
	declaredLabel, declaredIcon, ok := source.FieldPresentation(ref)
	if !ok {
		return label, icon
	}
	if label == "" {
		label = declaredLabel
	}
	if icon == "" {
		icon = declaredIcon
	}
	return label, icon
}

func (c *layoutCompiler) compile(blocks []LayoutBlock) ([]layoutNode, error) {
	nodes := make([]layoutNode, 0, len(blocks))
	for _, block := range blocks {
		node := layoutNode{kind: block.Kind, text: block.Text, label: block.Label, title: block.Title}
		var err error
		switch block.Kind {
		case blockField, blockChip:
			node.field = *block.Field
			if node.schema, err = c.schema(*block.Field); err == nil {
				node.label, node.icon = c.presentation(*block.Field, block.Label, block.Icon)
			}
		case blockLink:
			node.url = splitTemplate(block.URL.Template)
			node.urlField = block.URL.Fields
			for _, ref := range block.URL.Fields {
				var schema FieldSchema
				if schema, err = c.schema(ref); err != nil {
					break
				}
				node.urlTypes = append(node.urlTypes, schema)
			}
		case blockSection, blockFallback:
			node.children, err = c.compile(block.Blocks)
		case blockConditional:
			if node.when, err = CompilePredicate(c.ctx, *block.When, c.declarations); err != nil {
				break
			}
			for _, ref := range node.when.Fields() {
				c.record(ref)
			}
			if node.then, err = c.compile(block.Then); err == nil {
				node.otherwise, err = c.compile(block.Else)
			}
		}
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

// OutputKind returns the output kind the program renders.
func (p *LayoutProgram) OutputKind() string { return p.outputKind }

// Fields returns every field the layout reads through blocks, link URLs and predicates,
// without duplicates, in first-use order. The result is a copy the caller may modify.
func (p *LayoutProgram) Fields() []FieldRef {
	references := make([]FieldRef, len(p.fields))
	for i, field := range p.fields {
		references[i] = FieldRef{Namespace: field.Namespace, Path: append([]string(nil), field.Path...)}
	}
	return references
}
