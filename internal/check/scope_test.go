package check

import (
	"testing"

	"github.com/errata-ai/vale/v2/internal/core"
)

type scopeCase struct {
	scope []string
	pos   []string
	neg   []string
}

var scopeCases = []scopeCase{
	{
		[]string{`~blockquote & ~code`},
		[]string{`text.list.strong.md`},
		[]string{`text.blockquote.code.md`, `text.blockquote.md`},
	},
	{
		[]string{`~strong`},
		[]string{`text.list.md`, `text.blockquote.md`},
		[]string{},
	},
}

func TestScopes(t *testing.T) {
	for _, c := range scopeCases {
		s := NewScope(c.scope)
		for _, p := range c.pos {
			if !s.Matches(p) {
				t.Errorf("expected `true`, got `false`")
			}
		}

		for _, n := range c.neg {
			if s.Matches(n) {
				t.Errorf("expected `false`, got `true`")
			}
		}
	}
}

func TestSelectors(t *testing.T) {
	s1 := Selector{Value: []string{"text.comment.line.py"}}
	s2 := Selector{Value: []string{"text.comment"}}
	// s3 := Selector{Value: "text.comment.line.rb"}

	sec := []string{"text", "comment", "line", "py"}
	if !core.AllStringsInSlice(sec, s1.Sections()) {
		t.Errorf("expected = %v, got = %v", sec, s1.Sections())
	}

	if s2.Has("py") {
		t.Errorf("expected `false`, got `true`")
	}

	for _, part := range s1.Sections() {
		if !s1.Has(part) {
			t.Errorf("expected `true`, got `false`")
		}
	}
}
