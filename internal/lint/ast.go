package lint

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/errata-ai/vale/v2/internal/core"
	"github.com/errata-ai/vale/v2/internal/nlp"
	"golang.org/x/net/html"
)

// skipTags are tags that we don't want to lint.
var skipTags = []string{"script", "style", "pre", "figure", "noscript", "iframe"}

// skipClasses are classes that we don't want to lint:
//   - `problematic` is added by rst2html to processing errors which, in our
//     case, could be things like file-insertion URLs.
//   - `pre` is added by rst2html to code spans.
var skipClasses = []string{"problematic", "pre", "code"}
var inlineTags = []string{
	"b", "big", "i", "small", "abbr", "acronym", "cite", "dfn", "em", "kbd",
	"strong", "a", "br", "img", "span", "sub", "sup", "code", "tt", "del"}
var tagToScope = map[string]string{
	"th":         "text.table.header",
	"td":         "text.table.cell",
	"caption":    "text.table.caption",
	"li":         "text.list",
	"blockquote": "text.blockquote",
	"figcaption": "text.figure.caption",

	// NOTE: These shouldn't inherit from `text`
	// (or else they'll be linted twice.)
	"strong": "strong",
	"b":      "strong",
	"a":      "link",
	"em":     "emphasis",
	"i":      "emphasis",
	"code":   "code",
}

func (l Linter) lintHTMLTokens(f *core.File, raw []byte, offset int) error {
	var class, parentClass, attr string
	var inBlock, inline, skip, skipClass bool

	buf := bytes.NewBufferString("")

	// The user has specified a custom list of tags/classes to ignore.
	if len(l.Manager.Config.SkippedScopes) > 0 {
		skipTags = l.Manager.Config.SkippedScopes
	}
	if len(l.Manager.Config.IgnoredClasses) > 0 {
		skipClasses = append(skipClasses, l.Manager.Config.IgnoredClasses...)
	}

	skipped := []string{"tt", "code", "kbd"}
	if len(l.Manager.Config.IgnoredScopes) > 0 {
		skipped = l.Manager.Config.IgnoredScopes
	}

	walker := newWalker(f, raw, offset)
	for {
		tokt, tok, txt := walker.walk()

		walker.addCls(txt, tokt == html.StartTagToken)
		closed := walker.canClose()

		class = getAttribute(tok, "class")
		skipClass = checkClasses(class, skipClasses)

		blockSkip := skipClass && !core.StringInSlice(txt, inlineTags)
		if tokt == html.ErrorToken {
			break
		} else if tokt == html.StartTagToken && (core.StringInSlice(txt, skipTags) || blockSkip) {
			walker.setCls(txt, blockSkip)
			inBlock = true
			f.Metrics[txt]++
		} else if inBlock && (core.StringInSlice(txt, skipTags) || closed) {
			inBlock = false
			if closed {
				walker.close()
			}
		} else if tokt == html.StartTagToken {
			if (txt == "em" || txt == "b") && walker.lastTag(1) == "code" {
				// FIXME: See https://github.com/errata-ai/vale/issues/421
				txt = "code"
			}
			inline = core.StringInSlice(txt, inlineTags)
			skip = core.StringInSlice(txt, skipped)
			walker.addTag(txt)
		} else if tokt == html.EndTagToken && core.StringInSlice(txt, inlineTags) {
			walker.activeTag = ""
		} else if tokt == html.CommentToken {
			f.UpdateComments(txt)
		} else if tokt == html.TextToken {
			skip = skip || shouldBeSkipped(walker.tagHistory, f.NormedExt)
			if scope, match := tagToScope[walker.activeTag]; match {
				if core.StringInSlice(walker.activeTag, inlineTags) {
					// NOTE: We need to create a "temporary" context because
					// this text is actually linted twice: once as a 'link' and
					// once as part of the overall paragraph. See issue #105
					// for more info.
					tempCtx := updateContext(walker.context, walker.queue)
					ctxScope := getScope(walker.tagHistory, scope, f.RealExt)

					err := l.lintBlock(
						f,
						nlp.NewBlock(tempCtx, txt, ctxScope),
						walker.lines,
						0,
						true)

					if err != nil {
						return err
					}
					walker.activeTag = ""
				}
			}
			walker.append(txt)
			if !inBlock && txt != "" {
				skipClass = checkClasses(parentClass, skipClasses)
				txt, skip = clean(txt, attr, skip || skipClass, inline)
				buf.WriteString(txt)
			}
		}

		if tokt == html.EndTagToken && !core.StringInSlice(txt, inlineTags) {
			content := buf.String()
			if strings.TrimSpace(content) != "" {
				err := l.lintScope(f, &walker, content)
				if err != nil {
					return err
				}
			}
			walker.reset()
			buf.Reset()
		}

		attr = getAttribute(tok, "href")
		parentClass = getAttribute(tok, "class")

		walker.replaceToks(tok)
		if err := l.lintTags(f, walker, tok); err != nil {
			return err
		}
	}

	return l.lintSizedScopes(f)
}

func (l Linter) lintScope(f *core.File, state *walker, txt string) error {
	for _, tag := range state.tagHistory {
		scope, match := tagToScope[tag]
		if (match && !core.StringInSlice(tag, inlineTags)) || heading.MatchString(tag) {
			if scope == "text.blockquote" || scope == "text.list" {
				f.Summary.WriteString(txt + "\n\n")
			}

			if !match {
				scope = "text.heading." + tag
			}
			f.Metrics[strings.TrimPrefix(scope, "text.")] += 1

			txt = strings.TrimLeft(txt, " ")
			b := state.block(txt, scope+f.RealExt)

			return l.lintBlock(f, b, state.lines, 0, false)
		}
	}

	f.Summary.WriteString(txt + "\n\n")

	b := state.block(txt, "txt")
	return l.lintProse(f, b, state.lines)
}

func (l Linter) lintSizedScopes(f *core.File) error {
	f.ResetComments()

	// Run all rules with `scope: summary`
	//
	// TODO: is this the most efficient place to assign tagging?
	summary := nlp.NewLinedBlock(f.Content, f.Summary.String(),
		"summary"+f.RealExt, 0, &f.NLP)

	// Run all rules with `scope: raw`
	//
	// NOTE: We need to use `f.Lines` (instead of `f.Content`) to ensure that
	// we don't include any markup preprocessing.
	//
	// See #248, #306.
	raw := nlp.NewBlock("", strings.Join(f.Lines, ""), "raw"+f.RealExt)

	for _, blk := range []nlp.Block{summary, raw} {
		err := l.lintBlock(f, blk, len(f.Lines), 0, true)
		if err != nil {
			return err
		}
	}

	return nil
}

func (l Linter) lintTags(f *core.File, state walker, tok html.Token) error {
	ignored := core.StringInSlice("alt", l.Manager.Config.SkippedScopes)
	if tok.Data == "img" {
		for _, a := range tok.Attr {
			if a.Key == "alt" && !ignored {
				err := l.lintBlock(
					f,
					state.block(a.Val, "text.attr."+a.Key), state.lines, 0, false)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func checkClasses(attr string, ignore []string) bool {
	for _, class := range strings.Split(attr, " ") {
		if core.StringInSlice(class, ignore) {
			return true
		}
	}
	return false
}

// HACK: We need to look for inserted `spans` within `tt` tags.
//
// See https://github.com/errata-ai/vale/v2/issues/140.
func shouldBeSkipped(tagHistory []string, ext string) bool {
	if ext == ".rst" {
		n := len(tagHistory)
		for i := n - 1; i >= 0; i-- {
			if tagHistory[i] == "span" {
				continue
			}
			return tagHistory[i] == "tt" && i+1 != n
		}
	}
	return false
}

func clean(txt, attr string, skip, inline bool) (string, bool) {
	punct := []string{".", "?", "!", ",", ":", ";"}
	first, _ := utf8.DecodeRuneInString(txt)
	starter := core.StringInSlice(string(first), punct) && !skip
	if skip || attr == txt || inline {
		txt, _ = core.Substitute(txt, txt, '*')
	}

	if skip || attr == txt {
		skip = false
	}

	if inline && !starter {
		txt = " " + txt
	}
	return txt, skip
}

func getAttribute(tok html.Token, key string) string {
	for _, attr := range tok.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func updateContext(ctx string, queue []string) string {
	for _, s := range queue {
		ctx = updateCtx(ctx, s, html.TextToken)
	}
	return ctx
}

func updateCtx(ctx, txt string, tokt html.TokenType) string {
	var found bool
	if (tokt == html.TextToken || tokt == html.CommentToken) && txt != "" {
		for _, s := range strings.Split(txt, "\n") {
			ctx, found = core.Substitute(ctx, s, '@')
			if !found {
				for _, w := range strings.Fields(s) {
					ctx, _ = core.Substitute(ctx, w, '@')
				}
			}
		}
	}
	return ctx
}

func getScope(tags []string, new, ext string) string {
	root := tags[0]
	if root == "ul" {
		root = "li"
	} else if root == "p" {
		root = ""
	}

	scope, match := tagToScope[root]
	if !match && heading.MatchString(root) {
		scope = "text.heading." + root
	}

	ctx := strings.Join([]string{scope, new}, ".")
	return strings.TrimPrefix(ctx, ".") + ext
}
