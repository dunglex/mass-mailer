package mailer

import (
	htemplate "html/template"
	"io"
	"regexp"
	"strings"
	ttemplate "text/template"
)

// templateKeywords lists the identifiers that Go's text/template parser
// treats specially (control-flow keywords and built-in functions). Bare
// placeholders whose action body is one of these are left untouched by the
// backward-compatibility shim below, so authors can still write genuine
// actions such as {{if .vip}}...{{end}} or {{range .items}}...{{end}}.
var templateKeywords = map[string]bool{
	"if": true, "else": true, "end": true, "range": true, "with": true,
	"template": true, "define": true, "block": true, "break": true, "continue": true,
	"nil": true, "true": true, "false": true, "and": true, "or": true, "not": true,
	"len": true, "index": true, "slice": true, "printf": true, "print": true,
	"println": true, "call": true, "html": true, "js": true, "urlquery": true,
	"eq": true, "ne": true, "lt": true, "le": true, "gt": true, "ge": true,
}

// actionRe matches a single {{ ... }} template action, including multi-line
// ones (comments can legitimately span lines).
var actionRe = regexp.MustCompile(`(?s)\{\{(.*?)\}\}`)

// isIdentifier reports whether s is a valid Go identifier: letters, digits,
// or underscores, and not starting with a digit.
func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			continue
		case r >= '0' && r <= '9':
			if i == 0 {
				return false
			}
			continue
		default:
			return false
		}
	}
	return true
}

// preprocessTemplate rewrites bare placeholders like {{name}} to {{.name}}
// for backward compatibility with older stored batches, starter templates,
// and README examples, which all predate real Go templating. An action is
// rewritten only when its body is a single bare identifier that is not a
// template keyword or builtin and does not already start with "." or "$".
// Everything else -- conditionals, loops, function calls, already-dotted or
// dollar-prefixed expressions, and comments -- is left untouched.
func preprocessTemplate(src string) string {
	return actionRe.ReplaceAllStringFunc(src, func(match string) string {
		inner := match[2 : len(match)-2]
		trimmed := strings.TrimSpace(inner)
		if strings.HasPrefix(trimmed, "/*") {
			return match
		}
		fields := strings.Fields(trimmed)
		if len(fields) != 1 {
			return match
		}
		token := fields[0]
		if strings.HasPrefix(token, ".") || strings.HasPrefix(token, "$") {
			return match
		}
		if !isIdentifier(token) {
			return match
		}
		if templateKeywords[token] {
			return match
		}
		return "{{." + token + "}}"
	})
}

// compile parses a batch's subject and body once, applying the bare
// placeholder compatibility shim first. The subject is parsed with
// text/template so recipient values are never HTML-escaped; the body is
// parsed with html/template so it gets proper contextual auto-escaping.
// Both templates use missingkey=zero so a recipient missing a CSV column
// renders as empty text rather than the literal string "<no value>".
func compile(b Batch) (*ttemplate.Template, *htemplate.Template, error) {
	subjectTmpl, err := ttemplate.New("subject").Option("missingkey=zero").Parse(preprocessTemplate(b.Subject))
	if err != nil {
		return nil, nil, err
	}
	bodyTmpl, err := htemplate.New("body").Option("missingkey=zero").Parse(preprocessTemplate(b.Body))
	if err != nil {
		return nil, nil, err
	}
	return subjectTmpl, bodyTmpl, nil
}

// executor is satisfied by both *text/template.Template and
// *html/template.Template, letting render work with either.
type executor interface {
	Execute(w io.Writer, data any) error
}

// render executes a compiled template against a receiver's data and returns
// the result as a string.
func render(t executor, receiver Receiver) (string, error) {
	var buf strings.Builder
	if err := t.Execute(&buf, receiver); err != nil {
		return "", err
	}
	return buf.String(), nil
}
