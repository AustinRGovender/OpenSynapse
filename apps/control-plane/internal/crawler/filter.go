package crawler

import (
	"net/url"
	"strings"
)

// IsSameOrigin returns true if candidate has the same scheme+host as base.
func IsSameOrigin(base, candidate string) bool {
	bu, err := url.Parse(base)
	if err != nil {
		return false
	}
	cu, err := url.Parse(candidate)
	if err != nil {
		return false
	}
	return strings.EqualFold(bu.Scheme, cu.Scheme) && strings.EqualFold(bu.Host, cu.Host)
}

// IsBlocked returns true if the URL path matches any pattern in the blocklist.
func IsBlocked(rawURL string, blocklist []string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	for _, pattern := range blocklist {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if strings.Contains(u.Path, pattern) {
			return true
		}
	}
	return false
}

// NormalizeURL resolves a possibly-relative href against a base URL and strips the fragment.
func NormalizeURL(base, href string) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") {
		return ""
	}

	bu, err := url.Parse(base)
	if err != nil {
		return ""
	}
	hu, err := url.Parse(href)
	if err != nil {
		return ""
	}
	resolved := bu.ResolveReference(hu)
	resolved.Fragment = ""
	return resolved.String()
}

// ShouldVisit checks all filter conditions: same-origin, blocklist, and limit.
func ShouldVisit(cfg CrawlConfig, baseURL, candidate string, visited int) bool {
	if candidate == "" {
		return false
	}
	if cfg.SameOrigin && !IsSameOrigin(baseURL, candidate) {
		return false
	}
	if IsBlocked(candidate, cfg.Blocklist) {
		return false
	}
	if cfg.Limit > 0 && visited >= cfg.Limit {
		return false
	}
	return true
}
