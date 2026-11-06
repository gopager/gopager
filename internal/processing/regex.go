package processing

import (
	"context"
	"errors"
	"io"
	"regexp"
	"regexp/syntax"
	"strings"
	"time"
	"unicode/utf8"
)

// Core regex admission bounds. Go's regexp runs in linear time, but its NFA keeps one capture
// slice per program instruction, so memory grows with instructions x capture slots. Both are known
// before matching, so oversized programs are rejected deterministically instead of being sandboxed.
const (
	maxRegexPatternBytes = 4096
	maxRegexTextBytes    = 1 << 20
	maxRegexInsts        = 20_000
	maxRegexNFABytes     = 32 << 20
	regexCallBudget      = time.Second
	regexCancelInterval  = 16
)

// RegexCapture is one named capture group of a match. Index is the group's position in
// RegexResult.Names; Text is nil when the group did not participate in the match.
type RegexCapture struct {
	Name  string  `json:"name"`
	Index int     `json:"index"`
	Text  *string `json:"text,omitempty"`
}

// RegexResult reports the pattern's group names (Names[0] is the whole match and unnamed
// groups are "") and, when text was supplied, whether it matched and the named captures.
// Unnamed groups are never reported as captures.
type RegexResult struct {
	Matched  bool           `json:"matched"`
	Captures []RegexCapture `json:"captures,omitempty"`
	Names    []string       `json:"names"`
}

// RegexEvaluator inspects (text == nil) or matches a Core extraction pattern.
type RegexEvaluator interface {
	Regex(ctx context.Context, pattern string, text *string) (RegexResult, error)
}

// CoreRegex is the in-process, admission-bounded Core regex adapter.
type CoreRegex struct{}

var _ RegexEvaluator = CoreRegex{}

// Regex admits the pattern against the instruction and NFA-memory bounds, then matches with
// cooperative cancellation; a cancelled or expired call is a failure, never a no-match.
func (CoreRegex) Regex(ctx context.Context, pattern string, text *string) (RegexResult, error) {
	if ctx == nil {
		return RegexResult{}, invalid(errors.New("missing regex context"))
	}
	if err := ctx.Err(); err != nil {
		return RegexResult{}, err
	}
	if len(pattern) > maxRegexPatternBytes || (text != nil && len(*text) > maxRegexTextBytes) {
		return RegexResult{}, resource(errors.New("regex input boundary"))
	}
	if !utf8.ValidString(pattern) || (text != nil && !utf8.ValidString(*text)) {
		return RegexResult{}, invalid(errors.New("regex data is not UTF-8"))
	}
	compiled, err := admitRegex(pattern)
	if err != nil {
		return RegexResult{}, err
	}
	result := RegexResult{Names: compiled.SubexpNames()}
	if text == nil {
		return result, nil
	}
	ctx, cancel := context.WithTimeout(ctx, regexCallBudget)
	defer cancel()
	reader := &cancellableReader{ctx: ctx, source: strings.NewReader(*text)}
	indexes := compiled.FindReaderSubmatchIndex(reader)
	// A cancelled reader looks like end of input to regexp; report it as failure.
	if err := ctx.Err(); err != nil {
		return RegexResult{}, err
	}
	result.Matched = indexes != nil
	if !result.Matched {
		return result, nil
	}
	for i, name := range result.Names {
		if i == 0 || name == "" {
			continue
		}
		value := RegexCapture{Name: name, Index: i}
		if indexes[i*2] >= 0 {
			captured := (*text)[indexes[i*2]:indexes[i*2+1]]
			value.Text = &captured
		}
		result.Captures = append(result.Captures, value)
	}
	return result, nil
}

// admitRegex bounds compilation memory with a parse-tree estimate, then checks the exact program.
func admitRegex(pattern string) (*regexp.Regexp, error) {
	parsed, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return nil, invalid(errors.New("Core regex rejected its pattern"))
	}
	slots := int64(2 * (parsed.MaxCap() + 1))
	if !regexWithinBounds(estimateRegexInsts(parsed), slots) {
		return nil, resource(errors.New("regex program boundary"))
	}
	program, err := syntax.Compile(parsed.Simplify())
	if err != nil {
		return nil, invalid(errors.New("Core regex rejected its pattern"))
	}
	if !regexWithinBounds(int64(len(program.Inst)), slots) {
		return nil, resource(errors.New("regex program boundary"))
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, invalid(errors.New("Core regex rejected its pattern"))
	}
	return compiled, nil
}

func regexWithinBounds(insts, slots int64) bool {
	// Two NFA queues, each holding up to one thread per instruction with a slots-sized int slice.
	return insts <= maxRegexInsts && insts*slots*8*2 <= maxRegexNFABytes
}

// estimateRegexInsts over-approximates the compiled program size, saturating above the bound.
func estimateRegexInsts(re *syntax.Regexp) int64 {
	const ceiling = maxRegexInsts + 1
	add := func(a, b int64) int64 { return min(a+b, ceiling) }
	mul := func(a, b int64) int64 {
		if a == 0 || b == 0 {
			return 0
		}
		if a > ceiling/b {
			return ceiling
		}
		return min(a*b, ceiling)
	}
	switch re.Op {
	case syntax.OpLiteral:
		return int64(max(len(re.Rune), 1))
	case syntax.OpCapture:
		return add(2, estimateRegexInsts(re.Sub[0]))
	case syntax.OpStar, syntax.OpPlus, syntax.OpQuest:
		return add(2, estimateRegexInsts(re.Sub[0]))
	case syntax.OpRepeat:
		copies := int64(re.Max)
		if copies < 0 {
			copies = int64(re.Min) + 1
		}
		return mul(max(copies, 1), add(2, estimateRegexInsts(re.Sub[0])))
	case syntax.OpConcat, syntax.OpAlternate:
		total := int64(len(re.Sub))
		for _, sub := range re.Sub {
			total = add(total, estimateRegexInsts(sub))
		}
		return total
	default:
		return 2
	}
}

// cancellableReader checks its context every regexCancelInterval runes so regexp stops reading
// after cancellation. The interval is small because one rune of a near-limit pattern can cost
// milliseconds of NFA work; a coarse interval let a call overrun its budget by up to half a second.
type cancellableReader struct {
	ctx    context.Context
	source io.RuneReader
	count  int
}

func (r *cancellableReader) ReadRune() (rune, int, error) {
	r.count++
	if r.count%regexCancelInterval == 0 && r.ctx.Err() != nil {
		return 0, 0, io.EOF
	}
	return r.source.ReadRune()
}
