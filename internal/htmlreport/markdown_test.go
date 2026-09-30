package htmlreport

import (
	"slices"
	"strings"
	"testing"
)

// bodyHTML renders md and returns the output with its parsed hrefs; the output must pass the inert checks.
func bodyHTML(t *testing.T, md string) (out string, hrefs []string) {
	t.Helper()
	h, err := renderBody(md, headingShift)
	if err != nil {
		t.Fatal(err)
	}
	doc := parse(t, []byte("<!doctype html><title>t</title><body>"+string(h)))
	assertInert(t, doc, 0, 0)
	for _, a := range elements(doc, "a") {
		hrefs = append(hrefs, attr(a, "href"))
	}
	return string(h), hrefs
}

func TestBodyLinks(t *testing.T) {
	tests := []struct {
		name      string
		md        string
		wantHrefs []string
		wantText  string // must stay visible as text
	}{
		{"https", "[a](https://example.com/x?y=1&z=2)", []string{"https://example.com/x?y=1&z=2"}, "a"},
		{"http", "[a](http://example.com)", []string{"http://example.com"}, "a"},
		{"mailto", "[a](mailto:me@example.com)", []string{"mailto:me@example.com"}, "a"},
		{"relative", "[a](docs/a.md)", []string{"docs/a.md"}, "a"},
		{"fragment", "[a](#usage)", []string{"#usage"}, "a"},
		{"scheme relative", "[a](//example.com/x)", []string{"//example.com/x"}, "a"},
		{"autolink", "<https://example.com>", []string{"https://example.com"}, "https://example.com"},
		{"bare url", "see https://example.com/a", []string{"https://example.com/a"}, "see"},
		{"www", "see www.example.com", []string{"http://www.example.com"}, "www.example.com"},
		{"email", "mail me@example.com", []string{"mailto:me@example.com"}, "me@example.com"},

		{"javascript", "[a](javascript:alert(1))", nil, "a"},
		{"javascript mixed case", "[a](JaVaScRiPt:alert(1))", nil, "a"},
		{"javascript entity", "[a](&#106;avascript:alert(1))", nil, "a"},
		{"javascript hex entity", "[a](&#x6A;avascript:alert(1))", nil, "a"},
		{"javascript tab entity", "[a](java&#9;script:alert(1))", nil, "a"},
		{"javascript newline entity", "[a](java&#10;script:alert(1))", nil, "a"},
		{"javascript leading space", "[a]( javascript:alert(1))", nil, "a"},
		{"javascript angle", "[a](<javascript:alert(1)>)", nil, "a"},
		{"javascript escaped colon", `[a](javascript\:alert(1))`, nil, "a"},
		{"javascript autolink", "<javascript:alert(1)>", nil, "javascript:alert(1)"},
		{"javascript reference", "[a][r]\n\n[r]: javascript:alert(1)", nil, "a"},
		{"javascript in image link", "[![i](https://example.com/i.png)](javascript:alert(1))", nil, "i"},
		{"vbscript", "[a](vbscript:msgbox(1))", nil, "a"},
		{"data html", "[a](data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==)", nil, "a"},
		{"data image link", "[a](data:image/png;base64,AAAA)", nil, "a"},
		{"file", "[a](file:///etc/passwd)", nil, "a"},
		{"unknown scheme", "[a](ftp://example.com)", nil, "a"},
		{"custom scheme", "[a](slack://open)", nil, "a"},
		{"relative with colon", "[a](foo:bar)", nil, "a"},
		{"nested emphasis in unsafe link", "[**a** b](javascript:alert(1))", nil, "a b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, hrefs := bodyHTML(t, tt.md)
			if !slices.Equal(hrefs, tt.wantHrefs) {
				t.Errorf("hrefs = %q, want %q\n%s", hrefs, tt.wantHrefs, out)
			}
			if strings.Contains(strings.ToLower(out), "href=\"\"") {
				t.Errorf("output has an empty href:\n%s", out)
			}
			doc := parse(t, []byte(out))
			if !strings.Contains(textOf(doc), tt.wantText) {
				t.Errorf("text %q missing from:\n%s", tt.wantText, out)
			}
		})
	}
}

func TestBodyImages(t *testing.T) {
	tests := []struct {
		name string
		md   string
		want string // visible text
	}{
		{"remote", "![alt text](https://evil.test/track.png)", "alt text"},
		{"no alt", "![](https://evil.test/track.png)", "https://evil.test/track.png"},
		{"data uri", "![pic](data:image/png;base64,AAAA)", "pic"},
		{"javascript src", "![pic](javascript:alert(1))", "pic"},
		{"reference", "![pic][r]\n\n[r]: https://evil.test/p.png", "pic"},
		{"inside link", "[![pic](https://evil.test/p.png)](https://example.com)", "pic"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _ := bodyHTML(t, tt.md)
			if strings.Contains(out, "<img") {
				t.Errorf("output has an image:\n%s", out)
			}
			if !strings.Contains(textOf(parse(t, []byte(out))), tt.want) {
				t.Errorf("text %q missing from:\n%s", tt.want, out)
			}
		})
	}
}

func TestBodyRawHTML(t *testing.T) {
	tests := []struct {
		name string
		md   string
	}{
		{"script block", "<script>alert(1)</script>"},
		{"script inline", "text <script>alert(1)</script> text"},
		{"handler block", "<div onclick=alert(1)>hi</div>"},
		{"handler inline", "text <b onmouseover=alert(1)>b</b>"},
		{"iframe", "<iframe src=//evil.test></iframe>"},
		{"image", "<img src=x onerror=alert(1)>"},
		{"style", "<style>body{background:url(//evil.test)}</style>"},
		{"link tag", `<link rel="stylesheet" href="//evil.test/x.css">`},
		{"meta refresh", `<meta http-equiv="refresh" content="0;url=//evil.test">`},
		{"form", `<form action="//evil.test"><input name=a></form>`},
		{"anchor", `<a href="javascript:alert(1)">x</a>`},
		{"svg", `<svg onload=alert(1)></svg>`},
		{"comment breakout", `<!-- --><script>alert(1)</script>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, hrefs := bodyHTML(t, tt.md)
			if len(hrefs) != 0 {
				t.Errorf("hrefs = %q", hrefs)
			}
			lower := strings.ToLower(out)
			for _, bad := range []string{"<script", "<iframe", "<img", "<style", "<link", "<meta", "<form", "<svg", "onclick", "onmouseover", "onerror", "onload"} {
				if strings.Contains(lower, bad) {
					t.Errorf("output contains %q:\n%s", bad, out)
				}
			}
		})
	}
}

func TestBodyHeadingsShiftDown(t *testing.T) {
	out, _ := bodyHTML(t, "# a\n\n## b\n\n### c\n\n#### d\n\n##### e\n\n###### f\n\nSetext\n======\n")
	for _, want := range []string{"<h3>a</h3>", "<h4>b</h4>", "<h5>c</h5>", "<h6>d</h6>", "<h6>e</h6>", "<h6>f</h6>", "<h3>Setext</h3>"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "<h1") || strings.Contains(out, "<h2") {
		t.Errorf("body still has h1 or h2:\n%s", out)
	}
}

func TestBodyGFMAndCode(t *testing.T) {
	out, _ := bodyHTML(t, "| a | b |\n|---|---|\n| 1 | 2 |\n\n~~gone~~\n\n- [x] done\n- [ ] todo\n\n```go\nfmt.Println(\"<hi>\")\n```\n")
	for _, want := range []string{"<table>", "<del>gone</del>", `type="checkbox"`, "fmt.Println(&quot;&lt;hi&gt;&quot;)"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestBodyEmpty(t *testing.T) {
	out, _ := bodyHTML(t, "")
	if strings.TrimSpace(out) != "" {
		t.Errorf("empty body rendered %q", out)
	}
}

func TestSafeURL(t *testing.T) {
	tests := map[string]bool{
		"https://example.com":     true,
		"HTTP://EXAMPLE.COM":      true,
		"mailto:a@b.c":            true,
		"/abs/path":               true,
		"rel/path:with:colons":    true,
		"?q=a:b":                  true,
		"#frag:x":                 true,
		"":                        true,
		"javascript:alert(1)":     false,
		"JAVASCRIPT:alert(1)":     false,
		" javascript:alert(1)":    false,
		"java\tscript:alert(1)":   false,
		"java\nscript:alert(1)":   false,
		"\x01javascript:alert(1)": false,
		"vbscript:x":              false,
		"data:text/html,x":        false,
		"file:///x":               false,
		"ftp://x":                 false,
	}
	for in, want := range tests {
		if got := safeURL([]byte(in)); got != want {
			t.Errorf("safeURL(%q) = %v, want %v", in, got, want)
		}
	}
}
