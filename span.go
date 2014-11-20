package apptrace

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// A SpanID refers to a single span.
type SpanID struct {
	// Trace is the root ID of the tree that contains all of the spans
	// related to this one.
	Trace ID

	// Span is an ID that probabilistically uniquely identifies this
	// span.
	Span ID

	// Parent is the ID of the parent span, if any.
	Parent ID
}

var (
	// ErrBadSpanID is returned when the span ID cannot be parsed.
	ErrBadSpanID = errors.New("bad span ID")
)

// String returns the SpanID as a slash-separated, set of hex-encoded
// parameters (root, ID, parent). If the SpanID has no parent, that value is
// elided.
func (id SpanID) String() string {
	if id.Parent == 0 {
		return fmt.Sprintf("%s%s%s", id.Trace, SpanIDDelimiter, id.Span)
	}
	return fmt.Sprintf(
		"%s%s%s%s%s",
		id.Trace,
		SpanIDDelimiter,
		id.Span,
		SpanIDDelimiter,
		id.Parent,
	)
}

// Format formats according to a format specifier and returns the
// resulting string. The receiver's string representation is the first
// argument.
func (id SpanID) Format(s string, args ...interface{}) string {
	args = append([]interface{}{id.String()}, args...)
	return fmt.Sprintf(s, args...)
}

// IsRoot returns whether id is the root ID of a trace.
func (id SpanID) IsRoot() bool {
	return id.Parent == 0
}

// NewRootSpanID generates a new span ID for a root span. This should
// only be used to generate entries for spans caused exclusively by
// spans which are outside of your system as a whole (e.g., a root
// span for the first time you see a user request).
func NewRootSpanID() SpanID {
	return SpanID{
		Trace: generateID(),
		Span:  generateID(),
	}
}

// NewSpanID returns a new ID for an span which is the child of the
// given parent ID. This should be used to track causal relationships
// between spans.
func NewSpanID(parent SpanID) SpanID {
	return SpanID{
		Trace:  parent.Trace,
		Span:   generateID(),
		Parent: parent.Span,
	}
}

const (
	// SpanIDDelimiter is the delimiter used to concatenate an
	// SpanID's components.
	SpanIDDelimiter = "/"
)

// ParseSpanID parses the given string as a slash-separated set of parameters.
func ParseSpanID(s string) (*SpanID, error) {
	parts := strings.Split(s, SpanIDDelimiter)
	if len(parts) != 2 && len(parts) != 3 {
		return nil, ErrBadSpanID
	}
	root, err := ParseID(parts[0])
	if err != nil {
		return nil, ErrBadSpanID
	}
	id, err := ParseID(parts[1])
	if err != nil {
		return nil, ErrBadSpanID
	}
	var parent ID
	if len(parts) == 3 {
		i, err := ParseID(parts[2])
		if err != nil {
			return nil, ErrBadSpanID
		}
		parent = i
	}
	return &SpanID{
		Trace:  root,
		Span:   id,
		Parent: parent,
	}, nil
}

// Span is a span ID and its annotations.
type Span struct {
	// ID probabilistically uniquely identifies this span.
	ID SpanID

	Annotations
}

// String returns the Span as a formatted string.
func (s *Span) String() string {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(b)
}

// Name returns a span's name if it has a name annotation, and ""
// otherwise.
func (s *Span) Name() string {
	for _, a := range s.Annotations {
		if a.Key == nameKey {
			return string(a.Value)
		}
	}
	return ""
}

const (
	// Special annotation keys.
	nameKey = "name"
)

// Annotations is a list of annotations (on a span).
type Annotations []Annotation

// An Annotation is an arbitrary key-value property on a span.
type Annotation struct {
	// Key is the annotation's key.
	Key string

	// Value is the annotation's value, which may be either human or
	// machine readable, depending on the schema of the event that
	// generated it.
	Value []byte
}