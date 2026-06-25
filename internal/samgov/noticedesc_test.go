package samgov

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// fakeRT is a fake http.RoundTripper returning a canned response and capturing the URL.
type fakeRT struct {
	status int
	body   string
	gotURL string
}

func (f *fakeRT) RoundTrip(req *http.Request) (*http.Response, error) {
	f.gotURL = req.URL.String()
	return &http.Response{
		StatusCode: f.status,
		Body:       io.NopCloser(strings.NewReader(f.body)),
		Header:     make(http.Header),
	}, nil
}

func newTestResolver(t *testing.T, rt *fakeRT) *DescriptionResolver {
	t.Helper()
	r, err := NewDescriptionResolver("test-key")
	if err != nil {
		t.Fatalf("NewDescriptionResolver: %v", err)
	}
	return r.WithHTTPClient(&http.Client{Transport: rt})
}

func TestResolveJSONDescription(t *testing.T) {
	rt := &fakeRT{status: 200, body: `{"description":"<p>Zero Trust <b>Architecture</b> &amp; Continuous Monitoring required.</p>"}`}
	got, err := newTestResolver(t, rt).Resolve(context.Background(), "https://api.sam.gov/opportunities/v2/noticedesc?noticeid=abc")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := "Zero Trust Architecture & Continuous Monitoring required."
	if got != want {
		t.Errorf("Resolve = %q, want %q", got, want)
	}
	// api_key appended.
	if !strings.Contains(rt.gotURL, "api_key=test-key") {
		t.Errorf("request URL missing api_key: %s", rt.gotURL)
	}
}

func TestResolveRawHTMLFallback(t *testing.T) {
	// Not the {"description":...} JSON shape → treat the body as raw HTML.
	rt := &fakeRT{status: 200, body: "<html><body>Cloud Migration services</body></html>"}
	got, err := newTestResolver(t, rt).Resolve(context.Background(), "https://api.sam.gov/x?noticeid=1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "Cloud Migration services" {
		t.Errorf("raw fallback = %q", got)
	}
}

func TestResolveErrors(t *testing.T) {
	if _, err := newTestResolver(t, &fakeRT{}).Resolve(context.Background(), "   "); err == nil {
		t.Error("empty URL should error")
	}
	rt := &fakeRT{status: 429, body: "rate limited"}
	if _, err := newTestResolver(t, rt).Resolve(context.Background(), "https://api.sam.gov/x?noticeid=1"); err == nil {
		t.Error("non-200 should error")
	}
}

func TestResolveDoesNotLeakKeyOnError(t *testing.T) {
	rt := &fakeRT{status: 500, body: "boom"}
	_, err := newTestResolver(t, rt).Resolve(context.Background(), "https://api.sam.gov/x?noticeid=1")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "test-key") {
		t.Errorf("error leaked the API key: %v", err)
	}
}

func TestRequireAPIKey(t *testing.T) {
	if _, err := NewDescriptionResolver(""); err == nil {
		t.Error("empty key should error")
	}
}
