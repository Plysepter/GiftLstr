package main

import (
	"context"
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
