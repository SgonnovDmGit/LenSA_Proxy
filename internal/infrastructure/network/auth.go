package network

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/SgonnovDmGit/LenSA_Proxy/internal/domain/proxy"
)

const proxyAuthenticationChallenge = `Basic realm="LenSA Proxy"`

type BasicAuthenticator struct {
	enabled        bool
	usernameDigest [sha256.Size]byte
	passwordDigest [sha256.Size]byte
}

func NewBasicAuthenticator(config proxy.Config) BasicAuthenticator {
	authenticator := BasicAuthenticator{enabled: config.AuthEnabled}
	if !config.AuthEnabled {
		return authenticator
	}

	authenticator.usernameDigest = sha256.Sum256([]byte(config.Credentials.Username))
	authenticator.passwordDigest = sha256.Sum256([]byte(config.Credentials.Password))
	return authenticator
}

func (a BasicAuthenticator) Authenticate(request *http.Request) bool {
	if !a.enabled {
		return true
	}
	if request == nil {
		return false
	}

	values := headerValuesEqualFold(request.Header, "Proxy-Authorization")
	if len(values) != 1 {
		return false
	}

	scheme, encoded, found := strings.Cut(values[0], " ")
	if !found || !strings.EqualFold(scheme, "Basic") {
		return false
	}

	decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if len(decoded) != 0 {
		defer clear(decoded)
	}
	if err != nil || base64.StdEncoding.EncodeToString(decoded) != encoded {
		return false
	}

	username, password, found := bytes.Cut(decoded, []byte{':'})
	if !found {
		return false
	}

	usernameDigest := sha256.Sum256(username)
	passwordDigest := sha256.Sum256(password)
	usernameMatches := subtle.ConstantTimeCompare(usernameDigest[:], a.usernameDigest[:])
	passwordMatches := subtle.ConstantTimeCompare(passwordDigest[:], a.passwordDigest[:])
	return usernameMatches&passwordMatches == 1
}

func NewProxyAuthenticationRequiredResponse(request *http.Request) *http.Response {
	proto := "HTTP/1.1"
	protoMajor := 1
	protoMinor := 1
	if request != nil && request.Proto != "" {
		proto = request.Proto
		protoMajor = request.ProtoMajor
		protoMinor = request.ProtoMinor
	}

	return &http.Response{
		Status:        "407 Proxy Authentication Required",
		StatusCode:    http.StatusProxyAuthRequired,
		Proto:         proto,
		ProtoMajor:    protoMajor,
		ProtoMinor:    protoMinor,
		Header:        http.Header{"Proxy-Authenticate": {proxyAuthenticationChallenge}},
		Body:          http.NoBody,
		ContentLength: 0,
		Request:       request,
	}
}

func headerValuesEqualFold(header http.Header, name string) []string {
	var values []string
	for headerName, currentValues := range header {
		if strings.EqualFold(headerName, name) {
			values = append(values, currentValues...)
		}
	}
	return values
}
