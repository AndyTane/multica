package handler

import (
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var frontendDNSLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)
var frontendNumericTLD = regexp.MustCompile(`^(?:[0-9]+|0x[0-9a-f]+)$`)

// publicFrontendOrigin is a separate sharing contract, never an API/daemon URL.
// Reject credentials and URL suffixes rather than publishing operator secrets.
func publicFrontendOrigin() string {
	for _, raw := range []string{os.Getenv("MULTICA_APP_URL"), os.Getenv("FRONTEND_ORIGIN")} {
		if origin := validatedFrontendOrigin(raw); origin != "" {
			return origin
		}
	}
	return ""
}

func validatedFrontendOrigin(raw string) string {
	if strings.ContainsAny(raw, "\\\r\n\t") {
		return ""
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(raw, "?#") {
		return ""
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.Contains(host, "%") {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsUnspecified() {
			return ""
		}
	} else {
		labels := strings.Split(host, ".")
		// Browsers interpret numeric final labels as IPv4, including shorthand.
		if len(host) > 253 || frontendNumericTLD.MatchString(labels[len(labels)-1]) {
			return ""
		}
		for _, label := range labels {
			if len(label) > 63 || !frontendDNSLabel.MatchString(label) {
				return ""
			}
		}
	}
	port := u.Port()
	if strings.HasSuffix(u.Host, ":") {
		return ""
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return ""
		}
	}
	return u.Scheme + "://" + strings.ToLower(u.Host)
}
