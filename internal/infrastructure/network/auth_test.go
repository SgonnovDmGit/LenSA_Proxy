package network

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
)

func TestBasicAuthenticatorDisabledAccepts(t *testing.T) {
	authenticator := NewBasicAuthenticator(proxy.Config{})
	if !authenticator.Authenticate(nil) {
		t.Fatal("Authenticate() rejected disabled authentication")
	}

	request := httptest.NewRequest(http.MethodConnect, "https://example.com", nil)
	request.Header.Set("Proxy-Authorization", "not valid")
	if !authenticator.Authenticate(request) {
		t.Fatal("Authenticate() rejected a request while authentication was disabled")
	}
}

func TestBasicAuthenticatorEnabled(t *testing.T) {
	valid := encodeTestBasicCredentials("user", "password")
	withColon := encodeTestBasicCredentials("user", "part:two")
	withoutPadding := strings.TrimRight(valid, "=")
	withNewline := valid[:4] + "\n" + valid[4:]

	tests := []struct {
		name        string
		credentials proxy.Credentials
		headerName  string
		values      []string
		want        bool
	}{
		{
			name:        "valid",
			credentials: proxy.Credentials{Username: "user", Password: "password"},
			values:      []string{"Basic " + valid},
			want:        true,
		},
		{
			name:        "case insensitive scheme and header",
			credentials: proxy.Credentials{Username: "user", Password: "password"},
			headerName:  "pRoXy-AuThOrIzAtIoN",
			values:      []string{"bAsIc " + valid},
			want:        true,
		},
		{
			name:        "password containing colon",
			credentials: proxy.Credentials{Username: "user", Password: "part:two"},
			values:      []string{"Basic " + withColon},
			want:        true,
		},
		{
			name:        "missing header",
			credentials: proxy.Credentials{Username: "user", Password: "password"},
		},
		{
			name:        "wrong scheme",
			credentials: proxy.Credentials{Username: "user", Password: "password"},
			values:      []string{"Bearer " + valid},
		},
		{
			name:        "scheme prefix",
			credentials: proxy.Credentials{Username: "user", Password: "password"},
			values:      []string{"BasicX " + valid},
		},
		{
			name:        "malformed separator",
			credentials: proxy.Credentials{Username: "user", Password: "password"},
			values:      []string{"Basic\t" + valid},
		},
		{
			name:        "malformed base64",
			credentials: proxy.Credentials{Username: "user", Password: "password"},
			values:      []string{"Basic %%%"},
		},
		{
			name:        "missing base64 padding",
			credentials: proxy.Credentials{Username: "user", Password: "password"},
			values:      []string{"Basic " + withoutPadding},
		},
		{
			name:        "base64 containing newline",
			credentials: proxy.Credentials{Username: "user", Password: "password"},
			values:      []string{"Basic " + withNewline},
		},
		{
			name:        "decoded value without colon",
			credentials: proxy.Credentials{Username: "user", Password: "password"},
			values:      []string{"Basic " + base64.StdEncoding.EncodeToString([]byte("userpassword"))},
		},
		{
			name:        "wrong username",
			credentials: proxy.Credentials{Username: "user", Password: "password"},
			values:      []string{"Basic " + encodeTestBasicCredentials("other", "password")},
		},
		{
			name:        "wrong password",
			credentials: proxy.Credentials{Username: "user", Password: "password"},
			values:      []string{"Basic " + encodeTestBasicCredentials("user", "other")},
		},
		{
			name:        "duplicate headers",
			credentials: proxy.Credentials{Username: "user", Password: "password"},
			values:      []string{"Basic " + valid, "Basic " + valid},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authenticator := NewBasicAuthenticator(proxy.Config{
				AuthEnabled: true,
				Credentials: tt.credentials,
			})
			request := httptest.NewRequest(http.MethodGet, "http://example.com/resource", nil)
			if len(tt.values) != 0 {
				headerName := tt.headerName
				if headerName == "" {
					headerName = "Proxy-Authorization"
				}
				request.Header[headerName] = append([]string(nil), tt.values...)
			}

			if got := authenticator.Authenticate(request); got != tt.want {
				t.Fatalf("Authenticate() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestBasicAuthenticatorEnabledRejectsNilRequest(t *testing.T) {
	authenticator := NewBasicAuthenticator(proxy.Config{
		AuthEnabled: true,
		Credentials: proxy.Credentials{Username: "user", Password: "password"},
	})
	if authenticator.Authenticate(nil) {
		t.Fatal("Authenticate() accepted a nil request")
	}
}

func TestNewProxyAuthenticationRequiredResponse(t *testing.T) {
	request := httptest.NewRequest(http.MethodConnect, "https://example.com", nil)
	request.Proto = "HTTP/1.0"
	request.ProtoMajor = 1
	request.ProtoMinor = 0

	response := NewProxyAuthenticationRequiredResponse(request)
	if response.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("StatusCode = %d, want %d", response.StatusCode, http.StatusProxyAuthRequired)
	}
	if response.Status != "407 Proxy Authentication Required" {
		t.Fatalf("Status = %q", response.Status)
	}
	if response.Proto != request.Proto || response.ProtoMajor != 1 || response.ProtoMinor != 0 {
		t.Fatalf("protocol = %q %d.%d", response.Proto, response.ProtoMajor, response.ProtoMinor)
	}
	if response.Request != request {
		t.Fatal("Request does not reference the challenged request")
	}
	if got := response.Header.Get("Proxy-Authenticate"); got != `Basic realm="LenSA Proxy"` {
		t.Fatalf("Proxy-Authenticate = %q", got)
	}
	if response.Body == nil {
		t.Fatal("Body is nil")
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("reading Body: %v", err)
	}
	if len(body) != 0 || response.ContentLength != 0 {
		t.Fatalf("body length = %d, ContentLength = %d", len(body), response.ContentLength)
	}

	var wire bytes.Buffer
	if err := response.Write(&wire); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	parsed, err := http.ReadResponse(bufio.NewReader(&wire), request)
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	defer parsed.Body.Close()
	if parsed.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("parsed StatusCode = %d", parsed.StatusCode)
	}
	if got := parsed.Header.Get("Proxy-Authenticate"); got != `Basic realm="LenSA Proxy"` {
		t.Fatalf("parsed Proxy-Authenticate = %q", got)
	}
}

func encodeTestBasicCredentials(username, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}
