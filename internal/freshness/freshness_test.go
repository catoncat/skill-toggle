package freshness

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/catoncat/skill-toggle/internal/lockfile"
)

func TestParseGitHubURL(t *testing.T) {
	cases := []struct {
		in    string
		owner string
		repo  string
		ok    bool
	}{
		{"https://github.com/vercel-labs/agent-skills.git", "vercel-labs", "agent-skills", true},
		{"https://github.com/vercel-labs/agent-skills", "vercel-labs", "agent-skills", true},
		{"https://gitlab.com/x/y", "", "", false},
		{"", "", "", false},
		{"not-a-url", "", "", false},
		{"https://github.com/onlyowner", "", "", false},
	}
	for _, c := range cases {
		owner, repo, err := parseGitHubURL(c.in)
		if c.ok {
			if err != nil {
				t.Errorf("%q: expected ok, got %v", c.in, err)
				continue
			}
			if owner != c.owner || repo != c.repo {
				t.Errorf("%q: expected (%s,%s), got (%s,%s)", c.in, c.owner, c.repo, owner, repo)
			}
		} else if err == nil {
			t.Errorf("%q: expected error", c.in)
		}
	}
}

func TestCheckUpToDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/vercel-labs/agent-skills/contents/skills" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"name":"cloudflare","type":"dir","sha":"abc123"},
			{"name":"web-design","type":"dir","sha":"def456"}
		]`))
	}))
	defer srv.Close()

	c := &Checker{
		HTTPClient: srv.Client(),
		APIBase:    srv.URL,
	}
	res, err := c.Check(&lockfile.Entry{
		SourceURL:       "https://github.com/vercel-labs/agent-skills.git",
		SkillPath:       "skills/cloudflare/SKILL.md",
		SkillFolderHash: "abc123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.UpToDate {
		t.Errorf("expected up-to-date, got remote=%s local=%s", res.RemoteSHA, res.LocalSHA)
	}
	if res.RemoteSHA != "abc123" {
		t.Errorf("expected remote sha abc123, got %s", res.RemoteSHA)
	}
}

func TestCheckOutOfDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"name":"cloudflare","type":"dir","sha":"newer-sha"}]`))
	}))
	defer srv.Close()

	c := &Checker{HTTPClient: srv.Client(), APIBase: srv.URL}
	res, err := c.Check(&lockfile.Entry{
		SourceURL:       "https://github.com/vercel-labs/agent-skills.git",
		SkillPath:       "skills/cloudflare/SKILL.md",
		SkillFolderHash: "abc123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.UpToDate {
		t.Error("expected out-of-date")
	}
	if res.RemoteSHA != "newer-sha" {
		t.Errorf("expected remote sha newer-sha, got %s", res.RemoteSHA)
	}
}

func TestCheckRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer srv.Close()

	c := &Checker{HTTPClient: srv.Client(), APIBase: srv.URL}
	_, err := c.Check(&lockfile.Entry{
		SourceURL:       "https://github.com/vercel-labs/agent-skills.git",
		SkillPath:       "skills/cloudflare/SKILL.md",
		SkillFolderHash: "abc123",
	})
	if err != ErrRateLimited {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

func TestCheckNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := &Checker{HTTPClient: srv.Client(), APIBase: srv.URL}
	_, err := c.Check(&lockfile.Entry{
		SourceURL:       "https://github.com/vercel-labs/agent-skills.git",
		SkillPath:       "skills/cloudflare/SKILL.md",
		SkillFolderHash: "abc123",
	})
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCheckSkillMissingInResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"name":"some-other","type":"dir","sha":"x"}]`))
	}))
	defer srv.Close()

	c := &Checker{HTTPClient: srv.Client(), APIBase: srv.URL}
	_, err := c.Check(&lockfile.Entry{
		SourceURL:       "https://github.com/vercel-labs/agent-skills.git",
		SkillPath:       "skills/cloudflare/SKILL.md",
		SkillFolderHash: "abc123",
	})
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound when skill name absent, got %v", err)
	}
}

func TestCheckUnsupportedSource(t *testing.T) {
	c := NewChecker()
	_, err := c.Check(&lockfile.Entry{
		SourceURL:       "https://gitlab.com/x/y.git",
		SkillPath:       "skills/cloudflare/SKILL.md",
		SkillFolderHash: "abc123",
	})
	if err != ErrUnsupported {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}
}

func TestCheckNilEntry(t *testing.T) {
	c := NewChecker()
	_, err := c.Check(nil)
	if err != ErrNoLockEntry {
		t.Fatalf("expected ErrNoLockEntry, got %v", err)
	}
}

func TestNewCheckerReadsTokenEnv(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "abc")
	c := NewChecker()
	if c.Token != "abc" {
		t.Errorf("expected GITHUB_TOKEN to seed token, got %q", c.Token)
	}
	t.Setenv("GH_TOKEN", "primary")
	c = NewChecker()
	if c.Token != "primary" {
		t.Errorf("GH_TOKEN should win over GITHUB_TOKEN, got %q", c.Token)
	}
}

func TestCheckSendsAuthorizationHeader(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("Authorization")
		w.Write([]byte(`[{"name":"cloudflare","type":"dir","sha":"abc"}]`))
	}))
	defer srv.Close()

	c := &Checker{HTTPClient: srv.Client(), APIBase: srv.URL, Token: "secret-token"}
	_, _ = c.Check(&lockfile.Entry{
		SourceURL:       "https://github.com/vercel-labs/agent-skills",
		SkillPath:       "skills/cloudflare/SKILL.md",
		SkillFolderHash: "x",
	})

	select {
	case h := <-got:
		if h != "Bearer secret-token" {
			t.Errorf("expected Bearer header, got %q", h)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not see request")
	}
}
