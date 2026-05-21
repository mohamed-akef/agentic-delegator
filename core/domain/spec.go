// core/domain/spec.go
package domain

// SourceType describes where a spec's content lives.
type SourceType string

const (
	SourceTypeInline SourceType = "inline" // Value is the spec text itself
	SourceTypePath   SourceType = "path"   // Value is a path inside the target repo
	SourceTypeURL    SourceType = "url"    // Value is an http(s) URL
)

// SpecSource is the value object passed to the runner. The skill classifies
// the user's input into one of the three types before submitting.
type SpecSource struct {
	Type  SourceType
	Value string
}

func (s SpecSource) Valid() bool {
	if s.Value == "" {
		return false
	}
	switch s.Type {
	case SourceTypeInline, SourceTypePath, SourceTypeURL:
		return true
	}
	return false
}
