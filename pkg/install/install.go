// Copyright (c) 2026 John Dewey

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

// Package install orchestrates the agentpack install pipeline.
//
// Usage:
//
//	i := install.New()
//	result, err := i.Run(ctx, install.Options{
//	    Source:    "github.com/org/skills-repo",
//	    Selectors: []install.ContentSelector{{Type: "skill", Name: "review"}},
//	    OnStep:    func(s install.Step) { fmt.Println(s.Name, s.Detail) },
//	})
//
// Install supports git repos, local .agentpack archives, and HTTP/HTTPS URLs.
// Git sources are cloned, auto-packaged into a .agentpack archive, and
// installed. The archive is verified against its internal checksums before any
// files are written to disk.
package install

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/avfs/avfs/vfs/osfs"
	"gopkg.in/yaml.v3"

	"github.com/retr0h/agentpack/internal/archive"
	"github.com/retr0h/agentpack/internal/checksum"
	"github.com/retr0h/agentpack/internal/cli"
	"github.com/retr0h/agentpack/internal/configmerge"
	"github.com/retr0h/agentpack/internal/driver"
	"github.com/retr0h/agentpack/internal/fetcher"
	"github.com/retr0h/agentpack/internal/gitutil"
	"github.com/retr0h/agentpack/internal/lock"
	"github.com/retr0h/agentpack/internal/merge"
	"github.com/retr0h/agentpack/internal/metadata"
	"github.com/retr0h/agentpack/internal/packages"
	"github.com/retr0h/agentpack/internal/target"
	"github.com/retr0h/agentpack/pkg/registry"
	"github.com/retr0h/agentpack/pkg/safety"
)

// defaultResolveTargets maps requested target names to driver instances. When
// no names are given it returns all auto-detected targets.
func defaultResolveTargets(names []string) ([]target.Target, error) {
	if len(names) == 0 {
		return target.Detected(), nil
	}

	return target.Resolve(names)
}

// Installer orchestrates the agentpack install pipeline.
//
// Every collaborator that tests need to swap (target resolution, temp-file
// creation, registry I/O, the archives cache directory) is held as a field
// rather than a package-level var, so each Installer owns its own seams and
// parallel tests never race on shared mutable state.
type Installer struct {
	// resolveTargets maps requested target names to driver instances.
	resolveTargets func(names []string) ([]target.Target, error)

	// createTemp / mkdirTemp wrap os.CreateTemp / os.MkdirTemp so tests can
	// inject temp-creation failures.
	createTemp func(dir, pattern string) (*os.File, error)
	mkdirTemp  func(dir, pattern string) (string, error)

	// registrySave / registryLoad wrap registry.New().Save / .Load so tests can
	// avoid touching the real ~/.config/agentpack/packages/ directory.
	registrySave func(*registry.PackageManifest) error
	registryLoad func(string) (*registry.PackageManifest, error)

	// archivesDir resolves the archive cache directory
	// (~/.config/agentpack/archives), swappable so tests can redirect it.
	archivesDir func() (string, error)
}

// New returns a new Installer ready to run install pipelines.
func New() *Installer {
	return &Installer{
		resolveTargets: defaultResolveTargets,
		createTemp:     os.CreateTemp,
		mkdirTemp:      os.MkdirTemp,
		registrySave:   registry.New().Save,
		registryLoad:   registry.New().Load,
		archivesDir:    defaultArchivesDir,
	}
}

// Options configures an install run.
type Options struct {
	// Source is the local path or URL to the .agentpack archive.
	Source string

	// Dir is the root directory for installation (cwd for local, home for global).
	Dir string

	// Ref is the git ref (tag, branch, SHA) to check out. Passed from the
	// "@ref" portion of the source string per ADR-010.
	Ref string

	// Selectors restricts the install to specific content entries. Each
	// selector is a type/name pair (e.g. ContentSelector{Type: "skill",
	// Name: "review"}). When empty all content is installed. Per ADR-010.
	Selectors []ContentSelector

	// Skills restricts the install to named skills only. Deprecated: use
	// Selectors instead. Maintained for backward compatibility.
	Skills []string

	// Agents restricts the install to named agents only. Deprecated: use
	// Selectors instead. Maintained for backward compatibility.
	Agents []string

	// OriginalSource preserves the user-facing source URL when Source is
	// overwritten to point at a local archive during the build-first pipeline.
	OriginalSource string

	// OnStep is called in real-time as each pipeline phase completes.
	// When nil, steps are silently accumulated on the Result.
	OnStep func(Step)

	// TargetNames restricts the install to the named agent targets (e.g.
	// "claude-code", "cursor"). When nil or empty, all auto-detected targets
	// are used. Names are resolved against the registered target drivers.
	TargetNames []string

	// Global installs into each agent's global skills directory (under home)
	// instead of the project-local directory.
	Global bool

	// ContentCheck is called after reading metadata but before installing to
	// targets. It receives the content classification and may return an error
	// to abort the install. When nil the install proceeds unconditionally.
	ContentCheck func(*safety.Classification) error
}

// Step represents a completed pipeline phase for display.
type Step struct {
	Name   string `json:"name"`
	Detail string `json:"detail"`
}

// Result holds the outcome of a successful install.
type Result struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	// SHA is the short (display) git commit SHA. CommitSHA holds the full SHA.
	SHA string `json:"sha"`
	// CommitSHA is the full 40-char git commit SHA, used as the reproducible
	// pin in agentpack.lock. Empty for non-git sources.
	CommitSHA string `json:"commitSha,omitempty"`
	Source    string `json:"source"`
	Steps     []Step `json:"steps,omitempty"`
	// Dirs maps target display-name → installed directory.
	Dirs map[string]string `json:"dirs,omitempty"`
	// FileCounts maps target display-name → number of files installed.
	FileCounts map[string]int `json:"fileCounts,omitempty"`
	// ContentClassification holds the safety classification embedded in the
	// package metadata. Nil when the archive predates ADR-005.
	ContentClassification *safety.Classification `json:"contentClassification,omitempty"`
}

// Run installs from any source: .agentpack archive, git repo, or local path.
func (i *Installer) Run(ctx context.Context, opts Options) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	opts.Source = fetcher.ExpandShorthand(opts.Source)

	f, err := fetcher.New(opts.Source)
	if err != nil {
		return nil, fmt.Errorf("fetcher: %w", err)
	}

	// Git source: clone → install directly from the repo contents.
	if _, isGit := f.(*fetcher.GitFetcher); isGit {
		return i.runFromGit(ctx, opts, f)
	}

	return i.runFromArchive(ctx, opts, f)
}

func (i *Installer) runFromGit(
	ctx context.Context,
	opts Options,
	f fetcher.Fetcher,
) (*Result, error) {
	cloneDir, err := i.mkdirTemp("", "agentpack-git-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(cloneDir) }()

	gf, _ := f.(*fetcher.GitFetcher)

	cloneURL := opts.Source
	ref := opts.Ref

	sha, err := gf.FetchWithResult(ctx, opts.Source, cloneDir)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	emitStep(opts, Step{Name: "cloning", Detail: cloneURL})

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if !hasContentDirs(cloneDir) {
		return nil, fmt.Errorf(
			"repository has no installable content (no skills, commands, hooks, agents, mcp, or settings)",
		)
	}

	name := nameFromSource(opts.Source)

	// Use the ref as version when pinned, otherwise "latest".
	version := "latest"
	if ref != "" {
		version = ref
	}

	// Derive skill/agent filters from Selectors (ADR-010) or fall back to
	// the deprecated Skills/Agents fields.
	skillFilter := opts.Skills
	agentFilter := opts.Agents

	if len(opts.Selectors) > 0 {
		skillFilter = SelectorsToSkillFilter(opts.Selectors)
		agentFilter = SelectorsToAgentFilter(opts.Selectors)
	}

	archivePath, err := autoPackageWithVersion(
		ctx,
		cloneDir,
		name,
		sha,
		version,
		skillFilter,
		agentFilter,
	)
	if err != nil {
		return nil, fmt.Errorf("auto-package: %w", err)
	}
	defer func() { _ = os.Remove(archivePath) }()

	// Count files in the archive for the step detail.
	info, _ := os.Stat(archivePath)
	sizeStr := ""
	if info != nil {
		sizeStr = fmt.Sprintf("(%s)", cli.HumanSize(info.Size()))
	}
	emitStep(opts, Step{Name: "building package", Detail: sizeStr})

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// storeArchive caches the built archive (and its .sha256 sidecar) for
	// future reinstalls. A failure here is non-fatal — we install from the
	// freshly built temp archive instead — but it must not be silent, or an
	// operator has no way to know the cache (and its tamper-detection sidecar)
	// was never written.
	storedPath, err := i.storeArchive(ctx, archivePath, name, sha)
	if err != nil {
		storedPath = archivePath
		emitStep(opts, Step{
			Name:   "cache archive",
			Detail: fmt.Sprintf("skipped: %v", err),
		})
	}

	archiveOpts := opts
	archiveOpts.OriginalSource = opts.Source
	archiveOpts.Source = storedPath

	archiveFetcher, err := fetcher.New(storedPath)
	if err != nil {
		return nil, fmt.Errorf("fetcher for stored archive: %w", err)
	}

	result, err := i.runFromArchive(ctx, archiveOpts, archiveFetcher)
	if err != nil {
		return nil, err
	}

	result.Source = opts.Source

	return result, nil
}

func (i *Installer) runFromArchive(
	ctx context.Context,
	opts Options,
	f fetcher.Fetcher,
) (*Result, error) {
	tmpFile, err := i.createTemp("", "agentpack-install-*.agentpack")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}

	tmpArchive := tmpFile.Name()
	_ = tmpFile.Close()
	defer func() { _ = os.Remove(tmpArchive) }()

	if err := f.Fetch(ctx, opts.Source, tmpArchive); err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tmpDir, err := i.mkdirTemp("", "agentpack-install-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	verified, err := verifyArchiveSidecar(ctx, f, opts.Source, tmpArchive)
	if err != nil {
		return nil, err
	}
	if verified {
		emitStep(opts, Step{Name: "verify", Detail: "archive checksum verified"})
	}

	if err := archive.Extract(ctx, tmpArchive, tmpDir); err != nil {
		return nil, fmt.Errorf("extract: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// New YAML-based archives (ADR-009) omit checksums.txt. Legacy archives
	// include it and must still be verified. Detect the format by probing for
	// metadata.yaml — its presence signals the new format.
	newFormat := hasMetadataYAML(tmpDir)

	var verifyResults []checksum.Result

	if !newFormat {
		checksumFile, err := findChecksums(ctx, tmpDir)
		if err != nil {
			return nil, err
		}

		checksumEntries, err := checksum.ReadFile(checksumFile)
		if err != nil {
			return nil, fmt.Errorf("reading checksums: %w", err)
		}

		verifyResults, err = checksum.Verify(ctx, tmpDir, checksumEntries)
		if err != nil {
			return nil, fmt.Errorf("verify: %w", err)
		}

		for _, r := range verifyResults {
			if !r.OK {
				return nil, fmt.Errorf("checksum failed for %s: %s", r.Path, r.Err)
			}
		}
	}

	meta, err := findAndReadMetadata(ctx, tmpDir)
	if err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if opts.ContentCheck != nil && meta.Content != nil {
		if checkErr := opts.ContentCheck(meta.Content); checkErr != nil {
			return nil, checkErr
		}
	}

	if !hasContentDirs(tmpDir) {
		return nil, fmt.Errorf("package %s has no installable content", meta.Name)
	}

	if len(verifyResults) > 0 {
		emitStep(opts, Step{
			Name:   "verified checksums",
			Detail: fmt.Sprintf("%d/%d OK", len(verifyResults), len(verifyResults)),
		})
	}

	return i.installFromDir(ctx, opts, tmpDir, meta)
}

// verifyArchiveSidecar checks archivePath against a ".sha256" sidecar published
// next to source. ADR-009 makes that sidecar the integrity anchor for pre-built
// archives, verified before extraction. A missing sidecar is not an error so
// archives distributed without one still install (npm/Go-style optional
// integrity); a present sidecar that does not match aborts the install before
// any file is written. It returns true when a sidecar was found and matched.
//
// The sidecar is retrieved through the same fetcher as the archive, so the
// check works for every backend — a local "<path>.sha256", or an HTTP
// "<url>.sha256" alongside a remote archive. (A previous version read the
// sidecar with os.ReadFile on the raw source string, which silently no-oped
// for HTTP sources because a URL is not a local file.) archivePath must be a
// freshly fetched temp path distinct from source so the sidecar download
// cannot overwrite the source's own sidecar.
func verifyArchiveSidecar(
	ctx context.Context,
	f fetcher.Fetcher,
	source, archivePath string,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	sidecarPath := archivePath + ".sha256"
	if err := f.Fetch(ctx, source+".sha256", sidecarPath); err != nil {
		// No sidecar published (missing local file, HTTP 404, …). Distinguish a
		// genuine cancellation from an absent sidecar so a cancelled install
		// does not look like a clean skip.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return false, ctxErr
		}

		return false, nil
	}
	defer func() { _ = os.Remove(sidecarPath) }()

	// The sidecar was just fetched successfully, so a read failure here is a
	// real I/O error, not an absent sidecar — surface it rather than silently
	// skipping verification.
	data, err := os.ReadFile(sidecarPath)
	if err != nil {
		return false, fmt.Errorf("read fetched sidecar: %w", err)
	}

	expected := strings.TrimSpace(string(data))
	if expected == "" {
		return false, nil
	}

	actual, err := checksum.ComputeFile(ctx, osfs.NewWithNoIdm(), archivePath)
	if err != nil {
		return false, fmt.Errorf("hash archive: %w", err)
	}

	if !strings.EqualFold(actual, expected) {
		return false, fmt.Errorf(
			"archive SHA256 sidecar mismatch: expected %s, got %s", expected, actual,
		)
	}

	return true, nil
}

// knownGitHosts is the set of hosting domains whose first two path segments
// form an owner/repo pair.
var knownGitHosts = []string{
	"github.com",
	"gitlab.com",
	"bitbucket.org",
}

// nameFromSource extracts a namespaced owner/repo identifier from a source
// string. For git-hosted sources the two path segments immediately after the
// host are returned (e.g. "jeffallan/claude-skills"). For local paths only
// the base filename (without extension) is returned because there is no
// meaningful owner segment.
func nameFromSource(source string) string {
	s := source

	// Local absolute paths (or Windows-style) are identified by a leading
	// separator. Return just the base filename without extension.
	if filepath.IsAbs(s) {
		base := filepath.Base(s)
		if ext := filepath.Ext(base); ext != "" {
			base = base[:len(base)-len(ext)]
		}

		return base
	}

	// Strip #ref fragment (legacy).
	if idx := strings.LastIndex(s, "#"); idx >= 0 {
		s = s[:idx]
	}

	// Strip :selectors (ADR-010) — only when not part of a scheme (://).
	if idx := strings.Index(s, ":"); idx >= 0 {
		// Check if this colon is part of a "://" scheme.
		if idx+2 < len(s) && s[idx+1] == '/' && s[idx+2] == '/' {
			// scheme — strip scheme first, then look for selectors in remainder.
			remainder := s[idx+3:]
			if selIdx := strings.Index(remainder, ":"); selIdx >= 0 {
				s = s[:idx+3+selIdx]
			}
		} else {
			s = s[:idx]
		}
	}

	// Strip @ref version pin (ADR-010).
	if idx := strings.Index(s, "@"); idx >= 0 {
		s = s[:idx]
	}

	// Strip trailing slash.
	s = strings.TrimSuffix(s, "/")

	// Strip .git suffix.
	s = strings.TrimSuffix(s, ".git")

	// Strip scheme (https://, http://).
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}

	// Strip known git host prefix to obtain the remaining path segments.
	for _, host := range knownGitHosts {
		if after, ok := strings.CutPrefix(s, host+"/"); ok {
			s = after
			break
		}
	}

	// s is now one of:
	//   "owner/repo"          → return as-is
	//   "owner/repo/extra"    → return first two segments
	//   "just-a-name"         → no slash, bare name with no owner
	parts := strings.SplitN(s, "/", 3)
	if len(parts) >= 2 {
		return parts[0] + "/" + parts[1]
	}

	// Bare name (no owner, no host prefix): strip any file extension.
	base := parts[0]
	if ext := filepath.Ext(base); ext != "" {
		base = base[:len(base)-len(ext)]
	}

	return base
}

// UpdateManifests writes the installed package into agentpack-packages.yaml
// and agentpack.lock in the given directory.
func UpdateManifests(
	dir, source string,
	content, targets []string,
	ref string,
	result *Result,
) error {
	// Update agentpack-packages.yaml.
	pkgPath := filepath.Join(dir, "agentpack-packages.yaml")

	cfg, err := packages.Load(pkgPath)
	if err != nil {
		return fmt.Errorf("load packages: %w", err)
	}

	pkg := packages.BuildFromSource(result.Name, source, content, targets, ref)
	cfg.Add(pkg)

	if err := packages.Save(pkgPath, cfg); err != nil {
		return fmt.Errorf("save packages: %w", err)
	}

	// Update agentpack.lock.
	lockPath := filepath.Join(dir, "agentpack.lock")

	lf, err := lock.Load(lockPath)
	if err != nil {
		return fmt.Errorf("load lock: %w", err)
	}

	var lockedFiles []lock.LockedFile
	var lockedTargets []string

	if result.Dirs != nil {
		for _, tgtName := range result.Dirs {
			lockedTargets = append(lockedTargets, tgtName)
		}
	}

	// The lock pins the full commit SHA for reproducibility; the short SHA on
	// Result is for display only. Fall back to it for non-git sources.
	lockSHA := result.CommitSHA
	if lockSHA == "" {
		lockSHA = result.SHA
	}

	lp := lock.LockedPackage{
		Name:     result.Name,
		Source:   source,
		SHA:      lockSHA,
		Resolved: time.Now().UTC().Format(time.RFC3339),
		Content:  content,
		Targets:  lockedTargets,
		Files:    lockedFiles,
	}

	if ref != "" {
		lp.Ref = ref
	}

	lf.Set(lp)

	if err := lock.Save(lockPath, lf); err != nil {
		return fmt.Errorf("save lock: %w", err)
	}

	return nil
}

// RemoveManifests removes the named package from agentpack-packages.yaml,
// agentpack.lock, and the hooks section of .claude/settings.json. All
// operations are best-effort: a missing file or missing entry is not an error,
// because users may have installed a package without a managed yaml/lock.
//
// When selectors is non-empty, only matching content entries are removed from
// the package (partial removal). When empty the entire package is removed.
func RemoveManifests(dir, name string, selectors []ContentSelector) {
	pkgPath := filepath.Join(dir, "agentpack-packages.yaml")
	lockPath := filepath.Join(dir, "agentpack.lock")

	if len(selectors) > 0 {
		// Build set of "type/name" strings to remove.
		removeSet := make(map[string]bool, len(selectors))
		for _, s := range selectors {
			removeSet[s.Type+"/"+s.Name] = true
		}

		if cfg, loadErr := packages.Load(pkgPath); loadErr == nil {
			if p := cfg.Find(name); p != nil {
				remaining := make([]string, 0, len(p.Content))
				for _, c := range p.Content {
					if !removeSet[c] {
						remaining = append(remaining, c)
					}
				}

				p.Content = remaining
			}

			_ = packages.Save(pkgPath, cfg)
		}

		if lf, loadErr := lock.Load(lockPath); loadErr == nil {
			contentStrs := make([]string, len(selectors))
			for i, s := range selectors {
				contentStrs[i] = s.Type + "/" + s.Name
			}

			lf.RemoveContent(name, contentStrs)
			_ = lock.Save(lockPath, lf)
		}

		return
	}

	if cfg, loadErr := packages.Load(pkgPath); loadErr == nil {
		cfg.Remove(name)
		_ = packages.Save(pkgPath, cfg)
	}

	if lf, loadErr := lock.Load(lockPath); loadErr == nil {
		lf.Remove(name)
		_ = lock.Save(lockPath, lf)
	}

	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	_ = configmerge.RemoveHooks(settingsPath, name)
}

// ContentSelector represents a typed content selector in the form "type/name".
type ContentSelector struct {
	Type string // skill, command, hook, agent, mcp, config
	Name string // entry name
}

// ValidContentTypes lists the six recognized content types per ADR-009.
var ValidContentTypes = []string{"skill", "command", "hook", "agent", "mcp", "config"}

// SourceSpec holds the parsed components of a source string:
// "owner/repo@v2.0.0:skill/k8s:command/scan".
type SourceSpec struct {
	Source    string            // owner/repo or URL
	Ref       string            // git ref from @ (tag, branch, SHA)
	Selectors []ContentSelector // from : segments
}

// ParseSource parses "owner/repo@v2.0.0:skill/k8s:command/scan" into its
// components. The parsing order follows ADR-010:
//  1. Split on ":" -- first segment may contain "@", rest are selectors.
//  2. From the first segment, split on first "@" -- left is source, right is ref.
//  3. Each selector is validated as "type/name".
//
// A "://" scheme separator is not a selector boundary, so "https://" sources
// keep their scheme and any trailing ":type/name" selectors still split off.
func ParseSource(raw string) SourceSpec {
	parts := splitSelectors(raw)
	head := parts[0]

	var spec SourceSpec

	// Split the head on the first "@" to separate source from ref.
	if idx := strings.Index(head, "@"); idx > 0 {
		spec.Source = head[:idx]
		spec.Ref = head[idx+1:]
	} else {
		spec.Source = head
	}

	// Remaining parts are selectors in "type/name" form.
	for _, sel := range parts[1:] {
		if sel == "" {
			continue
		}

		slashIdx := strings.Index(sel, "/")
		if slashIdx < 0 {
			continue
		}

		spec.Selectors = append(spec.Selectors, ContentSelector{
			Type: sel[:slashIdx],
			Name: sel[slashIdx+1:],
		})
	}

	return spec
}

// splitSelectors splits a source string on ":" boundaries while treating a
// "://" scheme separator as part of the source, not a boundary. The first
// element is the "source@ref" head; the rest are "type/name" selectors.
func splitSelectors(raw string) []string {
	var segments []string

	start := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] != ':' {
			continue
		}

		// A "://" scheme separator belongs to the source URL — skip past it.
		if i+2 < len(raw) && raw[i+1] == '/' && raw[i+2] == '/' {
			i += 2
			continue
		}

		segments = append(segments, raw[start:i])
		start = i + 1
	}

	return append(segments, raw[start:])
}

// SelectorsToSkillFilter extracts skill names from selectors for backward
// compatibility with the skill-filter pipeline. Returns nil when no skill
// selectors are present.
func SelectorsToSkillFilter(selectors []ContentSelector) []string {
	var skills []string

	for _, s := range selectors {
		if s.Type == "skill" {
			skills = append(skills, s.Name)
		}
	}

	return skills
}

// SelectorsToAgentFilter extracts agent names from selectors for backward
// compatibility with the agent-filter pipeline. Returns nil when no agent
// selectors are present.
func SelectorsToAgentFilter(selectors []ContentSelector) []string {
	var agents []string

	for _, s := range selectors {
		if s.Type == "agent" {
			agents = append(agents, s.Name)
		}
	}

	return agents
}

// SelectorsToContent converts selectors to the "type/name" string format
// used in the packages.yaml content field.
func SelectorsToContent(selectors []ContentSelector) []string {
	if len(selectors) == 0 {
		return nil
	}

	content := make([]string, len(selectors))

	for i, s := range selectors {
		content[i] = s.Type + "/" + s.Name
	}

	return content
}

// emitStep calls opts.OnStep when the callback is set.
func emitStep(opts Options, s Step) {
	if opts.OnStep != nil {
		opts.OnStep(s)
	}
}

// typeToDir maps a metadata entry type back to the conventional directory
// name used in agentpack archives.
var typeToDir = map[string]string{
	"skill":   "skills",
	"command": "commands",
	"hook":    "hooks",
	"agent":   "agents",
	"mcp":     "mcp",
	"config":  "settings",
}

// buildContentEntries builds a []target.ContentEntry list from metadata. When
// the metadata includes explicit entries (ADR-009 format) those are used;
// otherwise the source directory is scanned for known content directories to
// maintain backward compatibility with legacy archives.
func buildContentEntries(meta *metadata.Metadata, sourceDir string) []target.ContentEntry {
	if len(meta.Entries) == 0 {
		return buildContentEntriesFromDirs(sourceDir)
	}

	entries := make([]target.ContentEntry, 0, len(meta.Entries))
	for _, e := range meta.Entries {
		entries = append(entries, target.ContentEntry{
			Name: e.Name,
			Type: e.Type,
			Root: resolveEntryRoot(e.Name, e.Type),
		})
	}

	return entries
}

// buildContentEntriesFromDirs discovers content entries by scanning the known
// content directories. This provides backward compatibility for archives that
// predate the entries field in metadata.
func buildContentEntriesFromDirs(sourceDir string) []target.ContentEntry {
	var entries []target.ContentEntry

	for _, dir := range []string{"skills", "agents"} {
		contentPath := filepath.Join(sourceDir, dir)

		subdirs, err := os.ReadDir(contentPath)
		if err != nil {
			continue
		}

		for _, d := range subdirs {
			if !d.IsDir() {
				continue
			}

			entries = append(entries, target.ContentEntry{
				Name: d.Name(),
				Type: dirToType[dir],
				Root: filepath.Join(dir, d.Name()),
			})
		}
	}

	for _, dir := range []string{"commands", "hooks", "mcp", "settings"} {
		contentPath := filepath.Join(sourceDir, dir)
		if _, err := os.Stat(contentPath); err != nil {
			continue
		}

		entries = append(entries, target.ContentEntry{
			Name: dir,
			Type: dirToType[dir],
			Root: dir,
		})
	}

	return entries
}

// resolveEntryRoot maps an entry name and type to the relative directory path
// within the archive. For skills and agents the entry name is a subdirectory
// under the type directory; for other types the root is the type directory
// itself.
func resolveEntryRoot(name, entryType string) string {
	dir := typeToDir[entryType]

	if entryType == "skill" || entryType == "agent" {
		return filepath.Join(dir, name)
	}

	return dir
}

func (i *Installer) installFromDir(
	ctx context.Context,
	opts Options,
	sourceDir string,
	meta *metadata.Metadata,
) (*Result, error) {
	targets, err := i.resolveTargets(opts.TargetNames)
	if err != nil {
		return nil, err
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("no agent targets detected — nothing to install to")
	}

	dir := opts.Dir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("getwd: %w", err)
		}
	}

	installOpts := target.InstallOpts{
		Name:    meta.Name,
		Version: meta.Version,
		Meta:    meta,
		Dir:     dir,
		Global:  opts.Global,
	}

	allEntries := buildContentEntries(meta, sourceDir)

	dirs := make(map[string]string)
	fileCounts := make(map[string]int)

	var allFiles []registry.InstalledFile

	for _, tgt := range targets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		srcDir, err := i.copyToTemp(ctx, sourceDir)
		if err != nil {
			return nil, fmt.Errorf("prepare source for %s: %w", tgt.Name(), err)
		}

		installOpts.SourceDir = srcDir

		// Filter entries to types this driver supports.
		supported := make(map[string]bool, len(tgt.SupportedTypes()))
		for _, t := range tgt.SupportedTypes() {
			supported[t] = true
		}

		var filtered []target.ContentEntry
		for _, e := range allEntries {
			if supported[e.Type] {
				filtered = append(filtered, target.ContentEntry{
					Name: e.Name,
					Type: e.Type,
					Root: filepath.Join(srcDir, e.Root),
				})
			}
		}

		installOpts.Entries = filtered

		emitStep(opts, Step{Name: "installing to", Detail: tgt.DisplayName()})

		installed, installErr := tgt.Install(ctx, installOpts)
		if installErr != nil {
			_ = os.RemoveAll(srcDir)

			return nil, fmt.Errorf("install to %s: %w", tgt.Name(), installErr)
		}

		for _, f := range installed {
			allFiles = append(allFiles, registry.InstalledFile{
				Path:   f.Path,
				SHA256: f.SHA256,
				Target: tgt.Name(),
				Dir:    dir,
			})
		}

		fileCounts[tgt.DisplayName()] = len(installed)

		_ = os.RemoveAll(srcDir)

		dirs[tgt.DisplayName()] = tgt.Name()
	}

	// Save registry manifest.
	scope := registry.ScopeLocal
	if opts.Global {
		scope = registry.ScopeGlobal
	}

	if len(allFiles) == 0 {
		return nil, fmt.Errorf("package %s has no installable content", meta.Name)
	}

	manifest := &registry.PackageManifest{
		Name:            meta.Name,
		Source:          registrySource(opts),
		SHA:             meta.GitCommitSHA,
		Version:         meta.Version,
		Installed:       time.Now().UTC().Format(time.RFC3339),
		Scope:           scope,
		SelectedContent: SelectorsToContent(opts.Selectors),
		SelectedSkills:  opts.Skills,
		Files:           allFiles,
	}

	if existing, loadErr := i.registryLoad(meta.Name); loadErr == nil && existing != nil {
		manifest.Files = mergeFiles(existing.Files, manifest.Files)
		manifest.SelectedContent = merge.Strings(existing.SelectedContent, manifest.SelectedContent)
		manifest.SelectedSkills = merge.Strings(existing.SelectedSkills, manifest.SelectedSkills)
	}

	if saveErr := i.registrySave(manifest); saveErr != nil {
		return nil, fmt.Errorf("save registry manifest: %w", saveErr)
	}

	return &Result{
		Name:                  meta.Name,
		Version:               meta.Version,
		SHA:                   gitutil.ShortSHA(meta.GitCommitSHA),
		CommitSHA:             meta.GitCommitSHA,
		Dirs:                  dirs,
		FileCounts:            fileCounts,
		ContentClassification: meta.Content,
	}, nil
}

// copyToTemp makes a fresh copy of src into a new temp directory and returns
// the path to the new directory.
func (i *Installer) copyToTemp(ctx context.Context, src string) (string, error) {
	dst, err := i.mkdirTemp("", "agentpack-target-*")
	if err != nil {
		return "", fmt.Errorf("create target temp dir: %w", err)
	}

	// Delegate to the shared driver helper so the extracted-archive copy honors
	// the same symlink guard every driver uses — a symlink that survived
	// extraction is skipped rather than followed to a file outside the tree.
	if copyErr := driver.CopyTreeIfExists(ctx, src, dst); copyErr != nil {
		_ = os.RemoveAll(dst)

		return "", fmt.Errorf("copy to target dir: %w", copyErr)
	}

	return dst, nil
}

// findChecksums locates the checksums.txt file inside the extracted archive.
// The generic archive layout places it at .agentpack/checksums.txt.
func findChecksums(ctx context.Context, dir string) (string, error) {
	var found string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		if !d.IsDir() && d.Name() == "checksums.txt" && strings.Contains(path, ".agentpack") {
			found = path

			return filepath.SkipAll
		}

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("searching for checksums.txt: %w", err)
	}

	if found == "" {
		return "", fmt.Errorf("checksums.txt not found in archive")
	}

	return found, nil
}

// hasMetadataYAML returns true when the extracted archive contains a
// .agentpack/metadata.yaml file, indicating the new ADR-009 format.
func hasMetadataYAML(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".agentpack", "metadata.yaml"))

	return err == nil
}

// findAndReadMetadata locates and parses archive metadata. It prefers
// .agentpack/metadata.yaml (new format) and falls back to
// .agentpack/metadata.json (legacy format) for backward compatibility.
func findAndReadMetadata(ctx context.Context, dir string) (*metadata.Metadata, error) {
	var yamlPath, jsonPath string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		if d.IsDir() || !strings.Contains(path, ".agentpack") {
			return nil
		}

		switch d.Name() {
		case "metadata.yaml":
			yamlPath = path
		case "metadata.json":
			jsonPath = path
		}

		if yamlPath != "" && jsonPath != "" {
			return filepath.SkipAll
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("searching for metadata.json: %w", err)
	}

	// Prefer YAML (new format), fall back to JSON (legacy).
	if yamlPath != "" {
		data, err := os.ReadFile(yamlPath)
		if err != nil {
			return nil, fmt.Errorf("read metadata.yaml: %w", err)
		}

		var meta metadata.Metadata
		if err := yaml.Unmarshal(data, &meta); err != nil {
			return nil, fmt.Errorf("parse metadata.yaml: %w", err)
		}

		return &meta, nil
	}

	if jsonPath != "" {
		data, err := os.ReadFile(jsonPath)
		if err != nil {
			return nil, fmt.Errorf("read metadata.json: %w", err)
		}

		var meta metadata.Metadata
		if err := json.Unmarshal(data, &meta); err != nil {
			return nil, fmt.Errorf("parse metadata.json: %w", err)
		}

		return &meta, nil
	}

	return nil, fmt.Errorf("metadata.json not found in archive")
}

func hasContentDirs(dir string) bool {
	for _, name := range []string{"skills", "commands", "agents", "hooks", "mcp", "settings"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}

	return false
}

func registrySource(opts Options) string {
	if opts.OriginalSource != "" {
		return opts.OriginalSource
	}
	return opts.Source
}

func mergeFiles(existing, incoming []registry.InstalledFile) []registry.InstalledFile {
	type key struct {
		Path   string
		Target string
	}

	seen := make(map[key]int, len(existing))
	merged := make([]registry.InstalledFile, len(existing))
	copy(merged, existing)

	for i, f := range merged {
		seen[key{f.Path, f.Target}] = i
	}

	for _, f := range incoming {
		k := key{f.Path, f.Target}
		if idx, ok := seen[k]; ok {
			merged[idx] = f
		} else {
			seen[k] = len(merged)
			merged = append(merged, f)
		}
	}

	return merged
}
