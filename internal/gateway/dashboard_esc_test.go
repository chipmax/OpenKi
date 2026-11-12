package gateway

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// esc() is the dashboard's only guard on values that land inside HTML
// attributes — a provider's baseUrl or a model id read out of its /v1/models is
// input from outside the gateway. Quotes were not escaped, so a `"` could close
// the attribute and run script in the admin page.
func TestDashboardEscNeutralisesAttributeDelimiters(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not available")
	}
	dir := t.TempDir()
	harness := filepath.Join(dir, "esc.mjs")
	body := escSource(t) + "\nprocess.stdout.write(esc(process.argv[2]));\n"
	if err := os.WriteFile(harness, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, harness, `a" onerror="alert(1) b'<c>&`).CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	want := `a&#34; onerror=&#34;alert(1) b&#39;&lt;c&gt;&amp;`
	if got := string(out); got != want {
		t.Errorf("esc(%q) = %q, want %q", `a" onerror="alert(1) b'<c>&`, got, want)
	}
}

// escSource pulls the dashboard's own esc() out of the embedded page, so the
// test runs the shipped function rather than a copy of it.
func escSource(t *testing.T) string {
	t.Helper()
	return dashboardFunc(t, "function esc(")
}

// dashboardFunc returns the one-line function in the page whose definition
// starts at marker.
func dashboardFunc(t *testing.T, marker string) string {
	t.Helper()
	for _, line := range strings.Split(string(dashboardHTML), "\n") {
		if i := strings.Index(line, marker); i >= 0 {
			return strings.TrimSpace(line[i:])
		}
	}
	t.Fatalf("the dashboard no longer defines %s", marker)
	return ""
}

// dashboardFuncBlock returns a multi-line function, brace-matched so a nested
// if/else does not truncate it. It understands the string and comment forms
// used by the dashboard; it is test scaffolding, not a JavaScript parser.
func dashboardFuncBlock(t *testing.T, marker string) string {
	t.Helper()
	src := string(dashboardHTML)
	start := strings.Index(src, marker)
	if start < 0 {
		t.Fatalf("the dashboard no longer defines %s", marker)
	}
	open := strings.Index(src[start:], "{")
	if open < 0 {
		t.Fatalf("%s has no body", marker)
	}
	open += start
	depth := 0
	quote := byte(0)
	lineComment, blockComment := false, false
	for i := open; i < len(src); i++ {
		c := src[i]
		if lineComment {
			if c == '\n' {
				lineComment = false
			}
			continue
		}
		if blockComment {
			if c == '*' && i+1 < len(src) && src[i+1] == '/' {
				blockComment = false
				i++
			}
			continue
		}
		if quote != 0 {
			if c == '\\' {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
		case '/':
			if i+1 < len(src) && src[i+1] == '/' {
				lineComment = true
				i++
			} else if i+1 < len(src) && src[i+1] == '*' {
				blockComment = true
				i++
			}
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return strings.TrimSpace(src[start : i+1])
			}
		}
	}
	t.Fatalf("%s has an unbalanced body", marker)
	return ""
}

// CodeArts identity fields come from config/token responses. The card used to
// concatenate them straight into body.innerHTML, so a nickname from an
// imported config could run script in the admin origin.
func TestDashboardCodeartsCardEscapesIdentityFields(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not available")
	}
	dir := t.TempDir()
	harness := filepath.Join(dir, "codearts.mjs")
	body := escSource(t) + "\n" + dashboardFuncBlock(t, "async function renderCodeartsCard(") + `
const wallStatus = {};
const state = { textContent: "" };
const body = { set innerHTML(v) { globalThis.captured = v; }, get innerHTML() { return ""; } };
const els = {
  "#ca-state": state, "#ca-body": body,
  "#ca-logout": { style: {} }, "#ca-login": { style: {} },
};
const $ = (sel) => els[sel] || { style: {} };
const setToggle = () => {};
const fetch = async () => ({ json: async () => ({
  loggedIn: true, enabled: true, models: 2, canRefresh: true, expiresAtMs: 0,
  nickname: '<img src=x onerror=alert(1)>', uid: "u1",
  lastCheckinDate: "<b>2026-11-12</b>",
}) });
await renderCodeartsCard();
process.stdout.write(globalThis.captured || "");
`
	if err := os.WriteFile(harness, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, harness).CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, out)
	}
	html := string(out)
	if strings.Contains(html, "<img src=x") || strings.Contains(html, "<b>2026") {
		t.Fatalf("CodeArts identity reached innerHTML unescaped: %s", html)
	}
	if !strings.Contains(html, "&lt;img src=x onerror=alert(1)&gt;") ||
		!strings.Contains(html, "&lt;b&gt;2026-11-12&lt;/b&gt;") {
		t.Fatalf("CodeArts identity was not escaped as expected: %s", html)
	}
}

// 详情 links take their target from the announcement feed and the model
// catalog, and a feed is not ours — escaping the quotes still leaves
// `javascript:` as a working href.
func TestDashboardSafeHrefKeepsOnlyWebSchemes(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not available")
	}
	dir := t.TempDir()
	harness := filepath.Join(dir, "href.mjs")
	body := escSource(t) + "\n" + dashboardFunc(t, "function safeHref(") +
		"\nprocess.stdout.write(esc(safeHref(process.argv[2])));\n"
	if err := os.WriteFile(harness, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ in, want string }{
		{"https://example.com/a?b=1&c=2", "https://example.com/a?b=1&amp;c=2"},
		{"http://127.0.0.1:8787/v1", "http://127.0.0.1:8787/v1"},
		{"javascript:alert(1)", "#"},
		{"JavaScript:alert(1)", "#"},
		{"  javascript:alert(1)", "#"},
		{"data:text/html;base64,PHNjcmlwdD4=", "#"},
		{"", "#"},
		{"https:/example.com", "#"},
	} {
		out, err := exec.Command(node, harness, tc.in).CombinedOutput()
		if err != nil {
			t.Fatalf("node failed on %q: %v\n%s", tc.in, err, out)
		}
		if got := string(out); got != tc.want {
			t.Errorf("safeHref(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
