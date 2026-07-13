package network

import (
	"net/http"
	"strings"
)

var hopByHopHeaderNames = [...]string{
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

func SanitizeHeaders(header http.Header) {
	remove := make(map[string]struct{}, len(hopByHopHeaderNames))
	for _, name := range hopByHopHeaderNames {
		remove[strings.ToLower(name)] = struct{}{}
	}

	for name, values := range header {
		if !strings.EqualFold(name, "Connection") {
			continue
		}
		for _, value := range values {
			for _, token := range strings.Split(value, ",") {
				token = strings.TrimSpace(token)
				if token != "" {
					remove[strings.ToLower(token)] = struct{}{}
				}
			}
		}
	}

	for name := range header {
		if _, found := remove[strings.ToLower(name)]; found {
			delete(header, name)
		}
	}
}
