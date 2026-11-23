// Package endpoint turns the address a user pastes (an Elasticsearch URL,
// a Kibana URL or an Elastic Cloud ID) into the Elasticsearch base URL.
package endpoint

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Elastic Cloud host names end with one of these suffixes. On these hosts the
// second DNS label is "es" for Elasticsearch and "kb" for Kibana.
var cloudSuffixes = []string{".elastic.cloud", ".elastic-cloud.com", ".found.io"}

// Resolve returns the Elasticsearch base URL for input, without a trailing slash.
// The note is non-empty when Resolve changed the address in a way that the user
// should be told about, such as converting a Kibana URL.
func Resolve(input string) (esURL, note string, err error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", "", errors.New("no address given")
	}
	if !strings.Contains(s, "://") {
		if u, ok := fromCloudID(s); ok {
			return u, "decoded the Elastic Cloud ID", nil
		}
		s = "https://" + s
	}

	u, err := url.Parse(s)
	if err != nil {
		return "", "", fmt.Errorf("%q is not a valid URL: %w", input, err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", "", fmt.Errorf("%q must use http or https", input)
	}
	if u.Hostname() == "" {
		return "", "", fmt.Errorf("%q has no host name", input)
	}

	if isCloud(u.Hostname()) {
		labels := strings.Split(u.Hostname(), ".")
		if labels[1] == "kb" {
			labels[1] = "es"
			setHostname(u, strings.Join(labels, "."))
			note = "converted the Kibana address to the Elasticsearch address"
		}
		// Elastic Cloud serves Elasticsearch at the root, so a pasted
		// Kibana page path such as /app/discover is dropped.
		u.Path, u.RawPath = "", ""
	}
	u.User, u.RawQuery, u.Fragment = nil, "", ""
	return strings.TrimRight(u.String(), "/"), note, nil
}

// KibanaURL returns the Kibana base URL that belongs to an Elastic Cloud
// Elasticsearch URL, or "" when esURL is not an Elastic Cloud address.
func KibanaURL(esURL string) string {
	u, err := url.Parse(esURL)
	if err != nil || !isCloud(u.Hostname()) {
		return ""
	}
	labels := strings.Split(u.Hostname(), ".")
	if labels[1] != "es" {
		return ""
	}
	labels[1] = "kb"
	setHostname(u, strings.Join(labels, "."))
	u.Path, u.RawPath = "", ""
	return u.String()
}

func isCloud(host string) bool {
	host = strings.ToLower(host)
	for _, suffix := range cloudSuffixes {
		if strings.HasSuffix(host, suffix) && strings.Count(host, ".") >= 3 {
			return true
		}
	}
	return false
}

func setHostname(u *url.URL, host string) {
	if port := u.Port(); port != "" {
		u.Host = net.JoinHostPort(host, port)
		return
	}
	u.Host = host
}

// fromCloudID decodes an Elastic Cloud ID. The format is
// "<name>:<base64 of host$es-id$kibana-id>", and the Elasticsearch
// address is https://<es-id>.<host>.
func fromCloudID(id string) (string, bool) {
	i := strings.LastIndex(id, ":")
	if i < 0 {
		return "", false
	}
	encoded := id[i+1:]
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		if data, err = base64.RawStdEncoding.DecodeString(encoded); err != nil {
			return "", false
		}
	}
	parts := strings.Split(string(data), "$")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	host, port := parts[0], ""
	if h, p, err := net.SplitHostPort(parts[0]); err == nil {
		host, port = h, p
	}
	addr := parts[1] + "." + host
	if port != "" && port != "443" {
		addr = net.JoinHostPort(addr, port)
	}
	return "https://" + addr, true
}
