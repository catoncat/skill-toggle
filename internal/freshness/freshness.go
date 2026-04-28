// Package freshness checks whether a single managed skill's upstream folder
// SHA differs from its lockfile-recorded SHA — i.e. "is there a new version
// available?" — without running `npx skills update`.
//
// We could not rely on the npx CLI here because vercel-labs/skills has no
// dry-run / --check / --json flag (see
// docs/superpowers/specs/2026-04-27-tui-redesign-aggregated-lazygit.md and
// the agent investigation it cites). Their internal `fetchSkillFolderHash`
// function hits the GitHub Tree/Contents API per skill and compares the
// folder sha to the lockfile's `skillFolderHash`. We replicate the
// minimum-cost version of that for one skill at a time, on user demand
// (the TUI's `F` key), so a 78-skill home doesn't burn through GitHub's
// 60 req/h anonymous quota on every launch.
//
// Out of scope: caching across runs, batching, retries, anything other
// than github.com sources. Non-github sources surface as ErrUnsupported.
package freshness

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/catoncat/skill-toggle/internal/lockfile"
)

// Sentinel errors so the caller can render distinct UI for each.
var (
	ErrRateLimited = errors.New("github rate limited")
	ErrNotFound    = errors.New("skill folder not found upstream")
	ErrUnsupported = errors.New("source url not supported (only github.com)")
	ErrNoLockEntry = errors.New("skill is not in any lockfile (unmanaged)")
)

// Result is what one Check returns: enough information to render a status
// line and decide whether to surface an "update available" hint.
type Result struct {
	LocalSHA  string
	RemoteSHA string
	UpToDate  bool
	CheckedAt time.Time
}

// Checker holds the HTTP client and optional GitHub token. Reuse one Checker
// across calls so connection pooling / token detection happen once.
type Checker struct {
	HTTPClient *http.Client
	Token      string
	UserAgent  string

	// APIBase lets tests redirect to a httptest server. Defaults to
	// "https://api.github.com" when empty.
	APIBase string
}

// NewChecker reads GH_TOKEN / GITHUB_TOKEN from the environment and returns
// a default-configured Checker. Callers that want to override individual
// fields can do so on the returned struct before calling Check.
func NewChecker() *Checker {
	token := firstNonEmpty(os.Getenv("GH_TOKEN"), os.Getenv("GITHUB_TOKEN"))
	return &Checker{
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		Token:      token,
		UserAgent:  "skill-toggle (https://github.com/catoncat/skill-toggle)",
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// Check resolves the upstream folder SHA for a managed skill and compares
// it against the lockfile's SkillFolderHash. The returned Result has
// CheckedAt set even on failure so callers can record when the last
// attempt happened.
func (c *Checker) Check(entry *lockfile.Entry) (Result, error) {
	now := time.Now()
	if entry == nil || entry.SkillFolderHash == "" {
		return Result{CheckedAt: now}, ErrNoLockEntry
	}

	owner, repo, err := parseGitHubURL(entry.SourceURL)
	if err != nil {
		return Result{LocalSHA: entry.SkillFolderHash, CheckedAt: now}, err
	}

	folder := path.Dir(entry.SkillPath)
	if folder == "." || folder == "/" {
		// Lockfile pointed at a top-level SKILL.md without an owning folder
		// — we can't ask GitHub for "the parent of nothing". Surface as
		// unsupported so the UI shows a useful message instead of a 404.
		return Result{LocalSHA: entry.SkillFolderHash, CheckedAt: now}, ErrUnsupported
	}

	remote, err := c.fetchFolderSHA(owner, repo, folder)
	if err != nil {
		return Result{LocalSHA: entry.SkillFolderHash, CheckedAt: now}, err
	}
	return Result{
		LocalSHA:  entry.SkillFolderHash,
		RemoteSHA: remote,
		UpToDate:  remote == entry.SkillFolderHash,
		CheckedAt: now,
	}, nil
}

// fetchFolderSHA resolves the GitHub tree SHA for one folder by asking the
// Contents API for the folder's *parent* and picking the matching child.
// This is the cheapest path that works at any depth: contents-of-folder
// returns the children's blob SHAs, not the folder's tree SHA, but
// contents-of-parent returns one entry per child where `type == "dir"`
// gives us exactly the tree SHA we want, in one round trip.
//
// Default branch is implicit (no ?ref=…) so we don't need a second request
// to discover it. GitHub's contents endpoint resolves to the repo's
// default_branch when ref is omitted.
func (c *Checker) fetchFolderSHA(owner, repo, folder string) (string, error) {
	parent := path.Dir(folder)
	target := path.Base(folder)

	apiURL := c.apiBase() + "/repos/" + owner + "/" + repo + "/contents/" + url.PathEscape(parent)
	if parent == "." || parent == "/" {
		apiURL = c.apiBase() + "/repos/" + owner + "/" + repo + "/contents"
	}

	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("github api: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through
	case http.StatusForbidden:
		// GitHub returns 403 with X-RateLimit-Remaining: 0 when the quota
		// is exhausted. Distinguish that from an auth-only 403 by header.
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			return "", ErrRateLimited
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("github 403: %s", strings.TrimSpace(string(body)))
	case http.StatusNotFound:
		return "", ErrNotFound
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("github %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var entries []contentsEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	for _, e := range entries {
		if e.Name == target && e.Type == "dir" {
			return e.SHA, nil
		}
	}
	return "", ErrNotFound
}

func (c *Checker) apiBase() string {
	if c.APIBase != "" {
		return strings.TrimRight(c.APIBase, "/")
	}
	return "https://api.github.com"
}

type contentsEntry struct {
	Name string `json:"name"`
	Type string `json:"type"` // "file" | "dir" | "symlink" | "submodule"
	SHA  string `json:"sha"`
}

// parseGitHubURL extracts owner/repo from the lockfile's `sourceUrl`. We
// accept the two shapes vercel-labs/skills writes:
//
//	https://github.com/vercel-labs/agent-skills.git
//	https://github.com/vercel-labs/agent-skills
//
// Anything else returns ErrUnsupported.
func parseGitHubURL(raw string) (owner, repo string, err error) {
	if raw == "" {
		return "", "", ErrUnsupported
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", ErrUnsupported
	}
	if !strings.EqualFold(u.Host, "github.com") {
		return "", "", ErrUnsupported
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", "", ErrUnsupported
	}
	owner = parts[0]
	repo = strings.TrimSuffix(parts[1], ".git")
	if owner == "" || repo == "" {
		return "", "", ErrUnsupported
	}
	return owner, repo, nil
}
