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
//	    Source: "github.com/org/skills-repo",
//	    Skills: []string{"review"},    // optional: only install named skills
//	    OnStep: func(s install.Step) { fmt.Println(s.Name, s.Detail) },
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
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/retr0h/agentpack/internal/archive"
	"github.com/retr0h/agentpack/internal/checksum"
	"github.com/retr0h/agentpack/internal/fetcher"
	"github.com/retr0h/agentpack/internal/metadata"
	"github.com/retr0h/agentpack/internal/safety"
	"github.com/retr0h/agentpack/pkg/registry"
	"github.com/retr0h/agentpack/pkg/target"
)

// Swappable OS functions for testing.
var (
	// osCreateTemp is a swappable wrapper so tests can inject temp-file
	// creation failures.
	osCreateTemp = os.CreateTemp
	osMkdirTemp  = os.MkdirTemp

	// registrySave is a swappable wrapper around registry.New().Save so tests
	// can prevent writes to the real ~/.config/agentpack/packages/ directory.
	registrySave = registry.New().Save

	// registryLoad is a swappable wrapper around registry.New().Load so tests
	// can inject a pre-existing manifest without touching the real registry.
	registryLoad = registry.New().Load
)

// Installer orchestrates the agentpack install pipeline.
type Installer struct{}

// New returns a new Installer ready to run install pipelines.
func New() *Installer { return &Installer{} }

// Options configures an install run.
type Options struct {
	// Source is the local path or URL to the .agentpack archive.
	Source string

	// Dir is the root directory for installation (cwd for local, home for global).
	Dir string

	// Skills restricts the install to named skills only. Each value is matched
	// against the skill subdirectory name (e.g. "review" matches
	// skills/review/SKILL.md). When empty all skills are installed.
	Skills []string

	// Agents restricts the install to named agents only. When empty all agents
	// are installed.
	Agents []string

	// OriginalSource preserves the user-facing source URL when Source is
	// overwritten to point at a local archive during the build-first pipeline.
	OriginalSource string

	// OnStep is called in real-time as each pipeline phase completes.
	// When nil, steps are silently accumulated on the Result.
	OnStep func(Step)

	// Targets is the list of agent targets to install into. When nil or empty
	// the global target registry is consulted and only detected targets are
	// used.
	Targets []target.Target

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
	SHA     string `json:"sha"`
	Source  string `json:"source"`
	Steps   []Step `json:"steps,omitempty"`
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
		return runFromGit(ctx, opts, f)
	}

	return runFromArchive(ctx, opts, f)
}

func runFromGit(ctx context.Context, opts Options, f fetcher.Fetcher) (*Result, error) {
	cloneDir, err := osMkdirTemp("", "agentpack-git-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(cloneDir) }()

	gf, _ := f.(*fetcher.GitFetcher)

	cloneURL := opts.Source
	ref := ""
	if idx := strings.LastIndex(cloneURL, "#"); idx >= 0 {
		ref = cloneURL[idx+1:]
		cloneURL = cloneURL[:idx]
	}

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

	archivePath, err := autoPackageWithVersion(
		ctx,
		cloneDir,
		name,
		sha,
		version,
		opts.Skills,
		opts.Agents,
	)
	if err != nil {
		return nil, fmt.Errorf("auto-package: %w", err)
	}
	defer func() { _ = os.Remove(archivePath) }()

	// Count files in the archive for the step detail.
	info, _ := os.Stat(archivePath)
	sizeStr := ""
	if info != nil {
		sizeStr = fmt.Sprintf("(%s)", humanSize(info.Size()))
	}
	emitStep(opts, Step{Name: "building package", Detail: sizeStr})

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	storedPath, err := storeArchive(archivePath, name, sha)
	if err != nil {
		storedPath = archivePath
	}

	archiveOpts := opts
	archiveOpts.OriginalSource = opts.Source
	archiveOpts.Source = storedPath

	archiveFetcher, err := fetcher.New(storedPath)
	if err != nil {
		return nil, fmt.Errorf("fetcher for stored archive: %w", err)
	}

	result, err := runFromArchive(ctx, archiveOpts, archiveFetcher)
	if err != nil {
		return nil, err
	}

	result.Source = opts.Source

	return result, nil
}

func runFromArchive(ctx context.Context, opts Options, f fetcher.Fetcher) (*Result, error) {
	tmpFile, err := osCreateTemp("", "agentpack-install-*.agentpack")
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

	tmpDir, err := osMkdirTemp("", "agentpack-install-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

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
		checksumFile, err := findChecksums(tmpDir)
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

	meta, err := findAndReadMetadata(tmpDir)
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

	return installFromDir(ctx, opts, tmpDir, meta)
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

	// Strip #ref fragment.
	if idx := strings.LastIndex(s, "#"); idx >= 0 {
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

// emitStep calls opts.OnStep when the callback is set.
func emitStep(opts Options, s Step) {
	if opts.OnStep != nil {
		opts.OnStep(s)
	}
}

func humanSize(bytes int64) string {
	const kb = 1024
	if bytes < kb {
		return fmt.Sprintf("%d B", bytes)
	}

	return fmt.Sprintf("%d KB", bytes/kb)
}

func installFromDir(
	ctx context.Context,
	opts Options,
	sourceDir string,
	meta *metadata.Metadata,
) (*Result, error) {
	targets := opts.Targets
	if len(targets) == 0 {
		targets = target.Detected()
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

	dirs := make(map[string]string)
	fileCounts := make(map[string]int)

	var allFiles []registry.InstalledFile

	for _, tgt := range targets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		srcDir, err := copyToTemp(ctx, sourceDir)
		if err != nil {
			return nil, fmt.Errorf("prepare source for %s: %w", tgt.Name(), err)
		}

		installOpts.SourceDir = srcDir

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
		Name:           meta.Name,
		Source:         registrySource(opts),
		SHA:            meta.GitCommitSHA,
		Version:        meta.Version,
		Installed:      time.Now().UTC().Format(time.RFC3339),
		Scope:          scope,
		SelectedSkills: opts.Skills,
		Files:          allFiles,
	}

	if existing, loadErr := registryLoad(meta.Name); loadErr == nil && existing != nil {
		manifest.Files = mergeFiles(existing.Files, manifest.Files)
		manifest.SelectedSkills = mergeStrings(existing.SelectedSkills, manifest.SelectedSkills)
	}

	if saveErr := registrySave(manifest); saveErr != nil {
		return nil, fmt.Errorf("save registry manifest: %w", saveErr)
	}

	return &Result{
		Name:                  meta.Name,
		Version:               meta.Version,
		SHA:                   shortSHA(meta.GitCommitSHA),
		Dirs:                  dirs,
		FileCounts:            fileCounts,
		ContentClassification: meta.Content,
	}, nil
}

// copyToTemp makes a fresh copy of src into a new temp directory and returns
// the path to the new directory.
func copyToTemp(ctx context.Context, src string) (string, error) {
	dst, err := osMkdirTemp("", "agentpack-target-*")
	if err != nil {
		return "", fmt.Errorf("create target temp dir: %w", err)
	}

	if copyErr := copyDir(ctx, src, dst); copyErr != nil {
		_ = os.RemoveAll(dst)

		return "", fmt.Errorf("copy to target dir: %w", copyErr)
	}

	return dst, nil
}

// findChecksums locates the checksums.txt file inside the extracted archive.
// The generic archive layout places it at .agentpack/checksums.txt.
func findChecksums(dir string) (string, error) {
	var found string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
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
	matches, _ := filepath.Glob(filepath.Join(dir, "**", ".agentpack", "metadata.yaml"))
	if len(matches) > 0 {
		return true
	}

	// Also check the top-level .agentpack directory directly.
	_, err := os.Stat(filepath.Join(dir, ".agentpack", "metadata.yaml"))

	return err == nil
}

// findAndReadMetadata locates and parses archive metadata. It prefers
// .agentpack/metadata.yaml (new format) and falls back to
// .agentpack/metadata.json (legacy format) for backward compatibility.
func findAndReadMetadata(dir string) (*metadata.Metadata, error) {
	var yamlPath, jsonPath string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
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

// copyDir recursively copies src to dst.
func copyDir(ctx context.Context, src string, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("rel path: %w", err)
		}

		tgtPath := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(tgtPath, 0o755)
		}

		return copyFile(path, tgtPath)
	})
}

// copyFile copies a single file from src to dst.
func copyFile(src string, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}

	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}

	if err := os.WriteFile(dst, data, info.Mode()); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}

	return nil
}

// shortSHA returns the first 7 characters of a git commit SHA.
func shortSHA(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}

	return sha
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

func mergeStrings(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	for _, s := range a {
		seen[s] = true
	}
	for _, s := range b {
		seen[s] = true
	}

	result := make([]string, 0, len(seen))
	for s := range seen {
		result = append(result, s)
	}

	slices.Sort(result)

	return result
}
