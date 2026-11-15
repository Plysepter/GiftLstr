package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// withTestServer temporarily points fetchClient at an httptest server so
// fetchPageMetadata can be exercised without real network access. Requests to
// any http(s) host are rewritten to hit the fake server, so tests can keep
// using realistic URLs while still being hermetic.
func withTestServer(t *testing.T, handler http.Handler) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	orig := fetchClient
	fetchClient = &http.Client{
		Transport: rewriteTransport{srv.URL},
		Timeout:   5 * time.Second,
	}
	t.Cleanup(func() { fetchClient = orig })
}

type rewriteTransport struct{ target string }

func (rt rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	u, err := url.Parse(rt.target)
	if err != nil {
		return nil, err
	}
	clone := r.Clone(r.Context())
	clone.URL.Scheme = u.Scheme
	clone.URL.Host = u.Host
	clone.URL.Path = strings.TrimSuffix(u.Path, "/") + r.URL.Path
	clone.Host = ""
	return http.DefaultTransport.RoundTrip(clone)
}

func TestFetchPageMetadataParsesOGCards(t *testing.T) {
	body := `<html><head>
		<meta property="og:title" content="Great Gift">
		<meta name="description" content="A wonderful widget.">
		<meta property="og:image" content="https://img.example.com/w.png">
	</head></html>`
	withTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(body))
	}))

	meta, err := fetchPageMetadata(context.Background(), "https://example.com/thing")
	if err != nil {
		t.Fatalf("fetchPageMetadata: %v", err)
	}
	if meta.Title != "Great Gift" {
		t.Errorf("title = %q, want Great Gift", meta.Title)
	}
	if meta.Description != "A wonderful widget." {
		t.Errorf("description = %q, want desc", meta.Description)
	}
	if meta.Image != "https://img.example.com/w.png" {
		t.Errorf("image = %q", meta.Image)
	}
	if meta.URL != "https://example.com/thing" {
		t.Errorf("url = %q", meta.URL)
	}
}

func TestFetchPageMetadataFallsBackToTitleTag(t *testing.T) {
	withTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>Fallback Title</title></head><body></body></html>`))
	}))

	meta, err := fetchPageMetadata(context.Background(), "https://example.com/")
	if err != nil {
		t.Fatalf("fetchPageMetadata: %v", err)
	}
	if meta.Title != "Fallback Title" {
		t.Errorf("title = %q, want Fallback Title", meta.Title)
	}
}

func TestFetchPageMetadataRejectsNonHTML(t *testing.T) {
	withTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4"))
	}))

	_, err := fetchPageMetadata(context.Background(), "https://example.com/doc.pdf")
	if err == nil || !strings.Contains(err.Error(), "does not return html") {
		t.Errorf("err = %v, want html mismatch error", err)
	}
}

func TestFetchPageMetadataRejectsNon200(t *testing.T) {
	withTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := fetchPageMetadata(context.Background(), "https://example.com/missing")
	if err == nil || !strings.Contains(err.Error(), "status 404") {
		t.Errorf("err = %v, want 404 error", err)
	}
}

func TestIsPublicIP(t *testing.T) {
	public := []string{"8.8.8.8", "1.1.1.1"}
	for _, s := range public {
		if !isPublicIP(net.ParseIP(s)) {
			t.Errorf("%s should be public", s)
		}
	}
	private := []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "169.254.0.1", "0.0.0.0", "::1", "fe80::1"}
	for _, s := range private {
		if isPublicIP(net.ParseIP(s)) {
			t.Errorf("%s should NOT be public", s)
		}
	}
}

func TestResolvePublicAddrsLiteralIP(t *testing.T) {
	public, err := resolvePublicAddrs(context.Background(), "8.8.8.8")
	if err != nil || len(public) != 1 {
		t.Fatalf("public literal: %v ok=%v", public, err)
	}
	private, err := resolvePublicAddrs(context.Background(), "127.0.0.1")
	if err != nil || len(private) != 0 {
		t.Fatalf("private literal should yield no addresses, got %v err=%v", private, err)
	}
}

func TestSSRFDialerRejectsPrivateHost(t *testing.T) {
	d := ssrfDialer{}
	// Loopback must be refused at dial time, closing any DNS-rebinding gap.
	if _, err := d.DialContext(context.Background(), "tcp", "127.0.0.1:80"); err == nil {
		t.Error("expected dial of loopback to be rejected")
	}
	if _, err := d.DialContext(context.Background(), "tcp", "127.0.0.1:9"); err == nil {
		t.Error("expected dial of private (10.0.0.1) to be rejected")
	}
	// A literal private address must also be refused.
	if _, err := d.DialContext(context.Background(), "tcp", "10.0.0.1:80"); err == nil {
		t.Error("expected dial of private literal to be rejected")
	}
}

func TestSSRFDialerRejectsHostnameResolvingToPrivate(t *testing.T) {
	// Point a hostname at loopback via the system resolver is hard to force in
	// a unit test, so instead verify that a hostname that resolves (whatever it
	// may be) is dialed by its validated numeric IP, i.e. we never hand an
	// unvalidated hostname to the dialer.
	d := ssrfDialer{}
	// "localhost" resolves to loopback; DialContext must refuse before connecting.
	if _, err := d.DialContext(context.Background(), "tcp", "localhost:80"); err == nil {
		t.Error("expected dial of localhost (loopback) to be rejected")
	}
}

func TestFetchPageMetadataOGTakesPrecedenceOverTitle(t *testing.T) {
	body := `<html><head>
		<title>Fallback Title</title>
		<meta property="og:title" content="OG Title">
	</head></html>`
	withTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(body))
	}))

	meta, err := fetchPageMetadata(context.Background(), "https://example.com/")
	if err != nil {
		t.Fatalf("fetchPageMetadata: %v", err)
	}
	if meta.Title != "OG Title" {
		t.Errorf("title = %q, want OG Title (Open Graph should win)", meta.Title)
	}
}

func TestNormalizeImageURL(t *testing.T) {
	tests := []struct {
		name  string
		image string
		page  string
		want  string
	}{
		{"already https", "https://img.example.com/w.png", "https://example.com/thing", "https://img.example.com/w.png"},
		{"http upgraded", "http://sonofoak.com/cdn/shop/files/x.jpg?v=1", "https://example.com/thing", "https://sonofoak.com/cdn/shop/files/x.jpg?v=1"},
		{"relative resolved", "/cdn/shop/files/x.jpg", "https://shop.example.com/products/x", "https://shop.example.com/cdn/shop/files/x.jpg"},
		{"relative resolved from http page", "images/x.jpg", "http://example.com/a/b", "https://example.com/a/images/x.jpg"},
		{"protocol relative", "//img.example.com/x.png", "https://example.com/", "https://img.example.com/x.png"},
		{"empty", "", "https://example.com/", ""},
		{"non-http scheme", "data:image/png;base64,AAAA", "https://example.com/", ""},
		{"javascript scheme", "javascript:alert(1)", "https://example.com/", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeImageURL(tc.image, tc.page); got != tc.want {
				t.Errorf("normalizeImageURL(%q, %q) = %q, want %q", tc.image, tc.page, got, tc.want)
			}
		})
	}
}

func TestHandleMetadataNormalizesCachedImageURL(t *testing.T) {
	raw := "https://8.8.8.8/cached-preview"
	if _, err := db.Exec(`INSERT INTO metadata_cache (url, title, description, image, fetched_at) VALUES (?, ?, ?, ?, datetime('now'))
		ON CONFLICT(url) DO UPDATE SET title=excluded.title, description=excluded.description, image=excluded.image, fetched_at=excluded.fetched_at`,
		raw, "Cached", "Cached desc", "http://img.example.com/a.jpg"); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/metadata?url="+url.QueryEscape(raw), nil)
	rec := httptest.NewRecorder()
	handleMetadata(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got pageMetadata
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if want := "https://img.example.com/a.jpg"; got.Image != want {
		t.Errorf("image = %q, want %q", got.Image, want)
	}
}

func TestAmazonHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"www.amazon.ca", true},
		{"amazon.com", true},
		{"amazon.co.uk", true},
		{"smile.amazon.de", true},
		{"notamazon.com", false},
		{"amazon.com.evil.example", false},
		{"example.com", false},
	}
	for _, tc := range tests {
		if got := amazonHost(tc.host); got != tc.want {
			t.Errorf("amazonHost(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestAmazonASIN(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://www.amazon.ca/Starfinder-RPG-Player-Core-Pocket/dp/1640787097/ref=sr_1_1", "1640787097"},
		{"https://www.amazon.com/gp/product/B0ABC12345", "B0ABC12345"},
		{"https://www.amazon.ca/gp/aw/d/B0ABC12345?th=1", "B0ABC12345"},
		{"https://www.amazon.ca/dp/B0ABC12345", "B0ABC12345"},
		{"https://example.com/dp/B0ABC12345", ""},
		{"https://www.amazon.ca/s?k=starfinder", ""},
	}
	for _, tc := range tests {
		u, err := url.Parse(tc.url)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.url, err)
		}
		if got := amazonASIN(u); got != tc.want {
			t.Errorf("amazonASIN(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestAmazonSlugTitle(t *testing.T) {
	u, _ := url.Parse("https://www.amazon.ca/Starfinder-RPG-Player-Core-Pocket/dp/1640787097/ref=sr_1_1")
	if got, want := amazonSlugTitle(u, "1640787097"), "Starfinder RPG Player Core Pocket"; got != want {
		t.Errorf("amazonSlugTitle = %q, want %q", got, want)
	}
	u, _ = url.Parse("https://www.amazon.ca/dp/1640787097")
	if got := amazonSlugTitle(u, "1640787097"); got != "" {
		t.Errorf("slug from bare /dp/ = %q, want empty", got)
	}
	u, _ = url.Parse("https://www.amazon.com/gp/product/B0ABC12345")
	if got := amazonSlugTitle(u, "B0ABC12345"); got != "" {
		t.Errorf("slug from /gp/product/ = %q, want empty", got)
	}
}

func TestFetchPageMetadataAugmentsAmazonCaptcha(t *testing.T) {
	withTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/isbn/") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"title":"Starfinder Player Core Pocket Edition (S2)"}`)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<html><head><title>Amazon.ca</title></head><body>Click the button below to continue shopping</body></html>`)
	}))

	raw := "https://www.amazon.ca/Starfinder-RPG-Player-Core-Pocket/dp/1640787097/ref=sr_1_1"
	meta, err := fetchPageMetadata(context.Background(), raw)
	if err != nil {
		t.Fatalf("fetchPageMetadata: %v", err)
	}
	if want := "Starfinder Player Core Pocket Edition (S2)"; meta.Title != want {
		t.Errorf("title = %q, want %q", meta.Title, want)
	}
	if want := amazonImageURL("1640787097"); meta.Image != want {
		t.Errorf("image = %q, want %q", meta.Image, want)
	}
}

func TestFetchPageMetadataAmazonFallsBackToSlug(t *testing.T) {
	withTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/isbn/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<html><head><title>Amazon.ca</title></head></html>`)
	}))

	raw := "https://www.amazon.ca/Starfinder-RPG-Player-Core-Pocket/dp/1640787097"
	meta, err := fetchPageMetadata(context.Background(), raw)
	if err != nil {
		t.Fatalf("fetchPageMetadata: %v", err)
	}
	if want := "Starfinder RPG Player Core Pocket"; meta.Title != want {
		t.Errorf("title = %q, want %q", meta.Title, want)
	}
}

func TestFetchPageMetadataKeepsAmazonOGWhenPresent(t *testing.T) {
	withTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<html><head>
			<meta property="og:title" content="Amazon Basics HDMI Cable">
			<meta property="og:image" content="https://img.example.com/cable.jpg">
		</head></html>`)
	}))

	raw := "https://www.amazon.ca/Amazon-Basics-HDMI-Cable/dp/B0ABC12345"
	meta, err := fetchPageMetadata(context.Background(), raw)
	if err != nil {
		t.Fatalf("fetchPageMetadata: %v", err)
	}
	if want := "Amazon Basics HDMI Cable"; meta.Title != want {
		t.Errorf("title = %q, want %q", meta.Title, want)
	}
	if want := "https://img.example.com/cable.jpg"; meta.Image != want {
		t.Errorf("image = %q, want %q", meta.Image, want)
	}
}

func TestCachedMetadataFixesAmazonPlaceholder(t *testing.T) {
	raw := "https://www.amazon.ca/Some-Gift-Item/dp/B0ABC12345"
	u, _ := url.Parse(raw)
	meta := cachedMetadata(context.Background(), raw, u, "Amazon.ca", "", "")
	if want := "Some Gift Item"; meta.Title != want {
		t.Errorf("title = %q, want %q", meta.Title, want)
	}
	if want := amazonImageURL("B0ABC12345"); meta.Image != want {
		t.Errorf("image = %q, want %q", meta.Image, want)
	}
}

func TestFetchPageMetadataNormalizesImageURL(t *testing.T) {
	body := `<html><head>
		<meta property="og:image" content="http://sonofoak.com/cdn/shop/files/black_friday.jpg?v=1">
	</head></html>`
	withTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(body))
	}))

	meta, err := fetchPageMetadata(context.Background(), "https://example.com/thing")
	if err != nil {
		t.Fatalf("fetchPageMetadata: %v", err)
	}
	want := "https://sonofoak.com/cdn/shop/files/black_friday.jpg?v=1"
	if meta.Image != want {
		t.Errorf("image = %q, want %q", meta.Image, want)
	}
}
