package plugin

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var tmplRe = regexp.MustCompile(`\{\{\s*([^}|]+?)\s*(?:\|\s*([a-z]+)\s*)?\}\}`)

// Render expands {{path}} and {{path|filter}} using vars. Filters: url, raw,
// json, lower, upper. When escapeURL is true values are URL-escaped unless
// the "raw" filter is used.
func Render(tmpl string, vars map[string]any, escapeURL bool) string {
	return tmplRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		sub := tmplRe.FindStringSubmatch(m)
		v := Lookup(vars, sub[1])
		s := toString(v)
		filter := sub[2]
		switch filter {
		case "url":
			return url.QueryEscape(s)
		case "raw":
			return s
		case "json":
			b, _ := json.Marshal(v)
			return string(b)
		case "lower":
			s = strings.ToLower(s)
		case "upper":
			s = strings.ToUpper(s)
		}
		if escapeURL {
			return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
		}
		return s
	})
}

func toString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	case bool:
		return strconv.FormatBool(x)
	case map[string]any, []any:
		b, _ := json.Marshal(x)
		return string(b)
	}
	return fmt.Sprint(v)
}

var pathTok = regexp.MustCompile(`[^.\[\]]+|\[(\d+)\]`)

// Lookup walks "a.b[0].c" through maps and slices. "$" or "" is the root.
// Several paths may be joined with "||" to try alternatives.
func Lookup(root any, path string) any {
	for _, alt := range strings.Split(path, "||") {
		v := lookupOne(root, strings.TrimSpace(alt))
		if v != nil && v != "" {
			return v
		}
	}
	return nil
}

func lookupOne(root any, path string) any {
	if path == "" || path == "$" {
		return root
	}
	path = strings.TrimPrefix(strings.TrimPrefix(path, "$"), ".")
	if strings.HasPrefix(path, "'") && strings.HasSuffix(path, "'") { // literal
		return strings.Trim(path, "'")
	}
	cur := root
	for _, m := range pathTok.FindAllStringSubmatch(path, -1) {
		if cur == nil {
			return nil
		}
		if m[1] != "" {
			i, _ := strconv.Atoi(m[1])
			arr, ok := cur.([]any)
			if !ok || i >= len(arr) {
				return nil
			}
			cur = arr[i]
			continue
		}
		switch c := cur.(type) {
		case map[string]any:
			cur = c[m[0]]
		case map[string]string:
			cur = c[m[0]]
		case []any: // "items.0"
			i, err := strconv.Atoi(m[0])
			if err != nil || i >= len(c) {
				return nil
			}
			cur = c[i]
		default:
			return nil
		}
	}
	return cur
}

// mapValue reads a mapping expression: a template if it has {{ }}, else a path.
func mapValue(expr string, item any, vars map[string]any) any {
	if strings.Contains(expr, "{{") {
		scope := map[string]any{"item": item}
		for k, v := range vars {
			scope[k] = v
		}
		return Render(expr, scope, false)
	}
	return Lookup(item, expr)
}
