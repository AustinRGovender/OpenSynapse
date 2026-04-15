package crawler

import "testing"

func TestIsSameOrigin(t *testing.T) {
	tests := []struct {
		base, candidate string
		want            bool
	}{
		{"https://example.com/a", "https://example.com/b", true},
		{"https://example.com", "https://example.com:443/path", false}, // port mismatch in string form
		{"https://example.com", "https://other.com", false},
		{"https://example.com", "http://example.com", false},
		{"https://Example.COM/a", "https://example.com/b", true},
		{"https://example.com", "", false},
		{"", "https://example.com", false},
	}

	for _, tt := range tests {
		got := IsSameOrigin(tt.base, tt.candidate)
		if got != tt.want {
			t.Errorf("IsSameOrigin(%q, %q) = %v, want %v", tt.base, tt.candidate, got, tt.want)
		}
	}
}

func TestIsBlocked(t *testing.T) {
	blocklist := []string{"/logout", "/delete", "/admin"}

	tests := []struct {
		url  string
		want bool
	}{
		{"https://example.com/logout", true},
		{"https://example.com/api/delete/user", true},
		{"https://example.com/admin/settings", true},
		{"https://example.com/home", false},
		{"https://example.com/api/users", false},
		{"https://example.com/", false},
	}

	for _, tt := range tests {
		got := IsBlocked(tt.url, blocklist)
		if got != tt.want {
			t.Errorf("IsBlocked(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestIsBlockedEmptyBlocklist(t *testing.T) {
	if IsBlocked("https://example.com/anything", nil) {
		t.Error("empty blocklist should not block anything")
	}
	if IsBlocked("https://example.com/anything", []string{}) {
		t.Error("empty blocklist should not block anything")
	}
	if IsBlocked("https://example.com/anything", []string{"", "  "}) {
		t.Error("whitespace-only blocklist entries should not block anything")
	}
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		base, href string
		want       string
	}{
		{"https://example.com/page", "/about", "https://example.com/about"},
		{"https://example.com/page", "https://other.com/x", "https://other.com/x"},
		{"https://example.com/page", "sub/page", "https://example.com/sub/page"},
		{"https://example.com/page", "#anchor", "https://example.com/page"},
		{"https://example.com/page", "", ""},
		{"https://example.com/page", "javascript:void(0)", ""},
		{"https://example.com/page", "mailto:a@b.com", ""},
	}

	for _, tt := range tests {
		got := NormalizeURL(tt.base, tt.href)
		if got != tt.want {
			t.Errorf("NormalizeURL(%q, %q) = %q, want %q", tt.base, tt.href, got, tt.want)
		}
	}
}

func TestShouldVisit(t *testing.T) {
	cfg := CrawlConfig{
		EntryURL:   "https://example.com",
		SameOrigin: true,
		Blocklist:  []string{"/logout"},
		Limit:      100,
	}

	tests := []struct {
		candidate string
		visited   int
		want      bool
	}{
		{"https://example.com/page", 0, true},
		{"https://other.com/page", 0, false},   // cross-origin
		{"https://example.com/logout", 0, false}, // blocked
		{"https://example.com/page", 100, false},  // at limit
		{"", 0, false},                            // empty
	}

	for _, tt := range tests {
		got := ShouldVisit(cfg, cfg.EntryURL, tt.candidate, tt.visited)
		if got != tt.want {
			t.Errorf("ShouldVisit(%q, visited=%d) = %v, want %v", tt.candidate, tt.visited, got, tt.want)
		}
	}
}

func TestShouldVisitNoSameOrigin(t *testing.T) {
	cfg := CrawlConfig{
		SameOrigin: false,
		Limit:      100,
	}

	if !ShouldVisit(cfg, "https://example.com", "https://other.com/page", 0) {
		t.Error("with SameOrigin=false, cross-origin URLs should be allowed")
	}
}
