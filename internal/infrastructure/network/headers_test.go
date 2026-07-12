package network

import (
	"net/http"
	"strings"
	"testing"
)

func TestSanitizeHeadersRemovesEveryHopByHopHeader(t *testing.T) {
	hopByHopNames := []string{
		"Connection",
		"Proxy-Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"TE",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	}

	header := make(http.Header)
	for _, name := range hopByHopNames {
		header[alternatingHeaderCaseForTest(name)] = []string{"remove"}
	}
	header.Set("Authorization", "retain")
	header.Set("Content-Type", "application/json")
	header.Set("Upgrade-Insecure-Requests", "1")
	header.Set("X-End-To-End", "retain")

	SanitizeHeaders(header)

	for _, name := range hopByHopNames {
		if containsHeaderForTest(header, name) {
			t.Errorf("header %q was not removed", name)
		}
	}
	retained := map[string]string{
		"Authorization":             "retain",
		"Content-Type":              "application/json",
		"Upgrade-Insecure-Requests": "1",
		"X-End-To-End":              "retain",
	}
	for name, want := range retained {
		if got := header.Get(name); got != want {
			t.Errorf("header %q = %q, want %q", name, got, want)
		}
	}
}

func TestSanitizeHeadersRemovesConnectionTokens(t *testing.T) {
	header := http.Header{
		"cOnNeCtIoN": []string{" X-First, x-SECOND ", "X-Third,,"},
		"x-FiRsT":    []string{"remove"},
		"X-Second":   []string{"remove"},
		"x-THIRD":    []string{"remove"},
	}
	header.Set("X-End-To-End", "retain")
	header.Set("Cache-Control", "no-cache")

	SanitizeHeaders(header)

	for _, name := range []string{"Connection", "X-First", "X-Second", "X-Third"} {
		if containsHeaderForTest(header, name) {
			t.Errorf("header %q was not removed", name)
		}
	}
	if got := header.Get("X-End-To-End"); got != "retain" {
		t.Fatalf("X-End-To-End = %q", got)
	}
	if got := header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q", got)
	}
}

func TestSanitizeHeadersAcceptsNilHeader(t *testing.T) {
	SanitizeHeaders(nil)
}

func containsHeaderForTest(header http.Header, name string) bool {
	for headerName := range header {
		if strings.EqualFold(headerName, name) {
			return true
		}
	}
	return false
}

func alternatingHeaderCaseForTest(name string) string {
	var result strings.Builder
	result.Grow(len(name))
	upper := false
	for _, character := range name {
		if upper {
			result.WriteString(strings.ToUpper(string(character)))
		} else {
			result.WriteString(strings.ToLower(string(character)))
		}
		if character != '-' {
			upper = !upper
		}
	}
	return result.String()
}
