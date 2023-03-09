package check

import (
	"strings"

	"github.com/errata-ai/vale/v2/internal/core"
)

// A Selector represents a named section of text.
type Selector struct {
	Value   []string // e.g., text.comment.line.py
	Negated bool
}

type Scope struct {
	Selectors map[string][]Selector
	Sized     bool
}

func NewScope(value []string) Scope {
	scope := map[string][]Selector{}

	for _, v := range value {
		selectors := []Selector{}
		for _, part := range strings.Split(v, "&") {
			selectors = append(selectors, NewSelector(strings.Split(part, ".")))
		}
		scope[v] = selectors
	}

	sized := false
	if len(value) == 1 {
		sized = value[0] == "raw" || value[0] == "summary" || value[0] == "text"
	}

	return Scope{Selectors: scope, Sized: sized}
}

// Macthes the scope `s` matches `s2`.
func (s Scope) Matches(blk string) bool {
	candidate := NewSelector([]string{blk})
	if candidate.isSized() {
		// TODO: ensure scope == candidate
		return s.Sized
	}

	for _, sel := range s.Selectors {
		for _, part := range sel {
			matched := candidate.Contains(part)
			if (!part.Negated && !matched) || (part.Negated && matched) {
				return false
			}
		}
	}

	return true
}

func NewSelector(value []string) Selector {
	negated := false

	parts := []string{}
	for i, m := range value {
		m = strings.TrimSpace(m)
		if i == 0 && strings.HasPrefix(m, "~") {
			m = strings.TrimPrefix(m, "~")
			negated = true
		}
		parts = append(parts, m)
	}

	return Selector{Value: parts, Negated: negated}
}

// Sections splits a Selector into its parts -- e.g., text.comment.line.py ->
// []string{"text", "comment", "line", "py"}.
func (s *Selector) Sections() []string {
	parts := []string{}
	for _, m := range s.Value {
		parts = append(parts, strings.Split(m, ".")...)
	}
	return parts
}

// Contains determines if all if sel's sections are in s.
func (s *Selector) Contains(sel Selector) bool {
	return core.AllStringsInSlice(sel.Sections(), s.Sections())
}

// ContainsString determines if all if sel's sections are in s.
func (s *Selector) ContainsString(scope []string) bool {
	for _, option := range scope {
		sel := Selector{Value: []string{option}}
		if !s.Contains(sel) {
			return false
		}
	}
	return true
}

// Equal determines if sel == s.
func (s *Selector) Equal(sel Selector) bool {
	if len(s.Value) == len(sel.Value) {
		for i, v := range s.Value {
			if sel.Value[i] != v {
				return false
			}
		}
		return true
	}
	return false
}

// Has determines if s has a part equal to scope.
func (s *Selector) Has(scope string) bool {
	return core.StringInSlice(scope, s.Sections())
}

func (s *Selector) isSized() bool {
	parts := s.Sections()
	return (s.Has("raw") || s.Has("summary") || s.Has("text")) && len(parts) == 2
}
