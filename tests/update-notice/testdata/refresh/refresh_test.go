// Package harness exercises the bounded single-page release refresh against a
// local httptest server. It is copied into a dot-prefixed directory inside the
// main module and run by the acceptance suite.
package harness

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"unidoc-pdf-debugger/internal/updatecheck"
)

type release struct {
	tag        string
	draft      bool
	prerelease bool
}

// releasesServer serves one page of releases at the GitHub path and counts
// every request it receives. The Link header always advertises a next page.
func releasesServer(t *testing.T, page []release) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/repos/unidoc/pdfdebug/releases" {
			http.NotFound(w, r)
			return
		}
		var body []map[string]any
		for _, rel := range page {
			body = append(body, map[string]any{
				"tag_name":   rel.tag,
				"name":       rel.tag,
				"draft":      rel.draft,
				"prerelease": rel.prerelease,
				"assets":     []any{},
			})
		}
		if body == nil {
			body = []map[string]any{}
		}
		w.Header().Set("Link", "<"+srv.URL+"/repos/unidoc/pdfdebug/releases?per_page=30&page=2>; rel=\"next\"")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func checkerFor(srv *httptest.Server) *updatecheck.Checker {
	return &updatecheck.Checker{BaseURL: srv.URL, Client: srv.Client()}
}

func sameVersion(got, want string) bool {
	return strings.TrimPrefix(got, "v") == strings.TrimPrefix(want, "v")
}

func TestLatestStableMakesExactlyOneRequest(t *testing.T) {
	var gotQuery string
	var hits atomic.Int64
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		gotQuery = r.URL.RawQuery
		w.Header().Set("Link", "<"+srv.URL+"/repos/unidoc/pdfdebug/releases?per_page=30&page=2>; rel=\"next\"")
		_ = json.NewEncoder(w).Encode([]map[string]any{{"tag_name": "v0.5.0", "draft": false, "prerelease": false}})
	}))
	defer srv.Close()

	got, err := checkerFor(srv).LatestStable(context.Background())
	if err != nil {
		t.Fatalf("LatestStable: %v", err)
	}
	if !sameVersion(got, "v0.5.0") {
		t.Errorf("LatestStable = %q, want v0.5.0", got)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("server saw %d requests, want exactly 1 (rel=next must not be followed)", n)
	}
	if !strings.Contains(gotQuery, "per_page=30") {
		t.Errorf("request query = %q, want per_page=30", gotQuery)
	}
}

func TestLatestStablePicksHighestStableTagInAnyOrder(t *testing.T) {
	srv, _ := releasesServer(t, []release{
		{tag: "v0.4.1"},
		{tag: "v0.9.0", draft: true},
		{tag: "v0.8.0", prerelease: true},
		{tag: "v0.7.0-rc1"},
		{tag: "nightly"},
		{tag: "v0.6.0"},
		{tag: "0.5.2"},
	})
	got, err := checkerFor(srv).LatestStable(context.Background())
	if err != nil {
		t.Fatalf("LatestStable: %v", err)
	}
	if !sameVersion(got, "v0.6.0") {
		t.Errorf("LatestStable = %q, want v0.6.0", got)
	}
}

func TestLatestStableSkipsPrereleaseTagWithoutTheFlag(t *testing.T) {
	srv, _ := releasesServer(t, []release{{tag: "v0.6.0-rc1"}, {tag: "v0.5.0"}})
	got, err := checkerFor(srv).LatestStable(context.Background())
	if err != nil {
		t.Fatalf("LatestStable: %v", err)
	}
	if !sameVersion(got, "v0.5.0") {
		t.Errorf("LatestStable = %q, want v0.5.0 (an unflagged -rc1 tag is still a prerelease)", got)
	}
}

func TestLatestStableEmptyWhenNothingQualifies(t *testing.T) {
	for name, page := range map[string][]release{
		"empty page": nil,
		"only unqualified": {
			{tag: "v1.0.0", draft: true},
			{tag: "v2.0.0", prerelease: true},
			{tag: "v3.0.0-beta.1"},
			{tag: "nightly"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv, _ := releasesServer(t, page)
			got, err := checkerFor(srv).LatestStable(context.Background())
			if err != nil {
				t.Fatalf("LatestStable: %v", err)
			}
			if got != "" {
				t.Errorf("LatestStable = %q, want an empty string", got)
			}
		})
	}
}

func TestLatestStableErrorsOnNonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer srv.Close()
	if got, err := checkerFor(srv).LatestStable(context.Background()); err == nil {
		t.Errorf("LatestStable = %q, nil on a 403; want an error", got)
	}
}

func TestLatestStableHonoursTheCallersDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := checkerFor(srv).LatestStable(ctx)
	elapsed := time.Since(start)
	if err == nil {
		t.Error("a server slower than the deadline must produce an error")
	}
	if elapsed > 2500*time.Millisecond {
		t.Errorf("LatestStable returned after %v; the 1.5s deadline must bound it", elapsed)
	}
}
