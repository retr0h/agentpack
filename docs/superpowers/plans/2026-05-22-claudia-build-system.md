# Claudia Build System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `claudia build` and `claudia verify` commands that package Claude Code plugins into checksummed `.claudia` tarballs matching the native marketplace directory structure.

**Architecture:** Five internal packages (`manifest`, `metadata`, `checksum`, `archive`, `plugin`) each handle one responsibility. The `cmd/build.go` orchestrates the build pipeline by calling them in sequence. `cmd/verify.go` orchestrates the verify pipeline. All packages are already scaffolded as empty Go files with license headers.

**Tech Stack:** Go 1.25, cobra, yaml.v3, crypto/sha256, archive/tar, compress/gzip (stdlib)

**Spec:** `docs/superpowers/specs/2026-05-22-claudia-build-system-design.md`

**Testing:** Table-driven tests for every public function. Run `go test ./...` after every implementation step. Run `go vet ./...` before every commit.

**License header:** Every `.go` file in this project starts with the MIT license header. Copy the exact header from any existing file (e.g., `cmd/root.go` lines 1-19). All code blocks below omit the header for brevity — you MUST include it when writing files.

---

## File Map

| File | Responsibility |
|------|---------------|
| `internal/manifest/manifest.go` | Types (`Manifest`, `Plugin`, `Entry`, `Author`, `MCPEntry`), `Load()`, `Normalize()`, `Entry.UnmarshalYAML` |
| `internal/manifest/manifest_test.go` | Table-driven tests for Load, Normalize, Entry unmarshaling |
| `internal/manifest/resolve.go` | `ResolveEntries()` — expand globs and src/dest mappings into concrete file pairs |
| `internal/manifest/resolve_test.go` | Table-driven tests for glob expansion, src/dest, errors |
| `internal/metadata/metadata.go` | Types (`Metadata`), `Capture()` — run git commands, collect build info |
| `internal/metadata/metadata_test.go` | Table-driven tests for git present, git absent, detached HEAD |
| `internal/checksum/checksum.go` | Types (`Entry`), `ComputeAll()`, `WriteFile()`, `ReadFile()`, `Verify()` |
| `internal/checksum/checksum_test.go` | Table-driven tests for compute, write, read, verify pass/fail |
| `internal/plugin/plugin.go` | `GenerateMarketplace()`, `GeneratePlugin()` — produce JSON structs |
| `internal/plugin/plugin_test.go` | Table-driven tests for JSON generation with all/minimal fields |
| `internal/archive/archive.go` | `Create()` — write .tar.gz from a file map; `Extract()` — extract to temp dir |
| `internal/archive/archive_test.go` | Table-driven tests for round-trip, path traversal, symlinks |
| `cmd/build.go` | Orchestrate full build pipeline, CLI arg handling |
| `cmd/verify.go` | Orchestrate verify pipeline, CLI arg handling |

---

### Task 1: Manifest Types and Loading

**Files:**
- Modify: `internal/manifest/manifest.go`
- Create: `internal/manifest/manifest_test.go`

- [ ] **Step 1: Write the failing tests for Load and types**

```go
package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "single plugin manifest",
			yaml: `
name: my-plugin
version: 1.0.0
description: "Test plugin"
author:
  name: "Test Author"
  email: "test@example.com"
license: MIT
skills:
  - skills/*.md
`,
		},
		{
			name: "multi plugin manifest",
			yaml: `
author:
  name: "Shared Author"
  email: "shared@example.com"
license: MIT
plugins:
  - name: plugin-a
    version: 1.0.0
    description: "Plugin A"
    skills:
      - skills/*.md
  - name: plugin-b
    version: 2.0.0
    description: "Plugin B"
    commands:
      - commands/*.md
`,
		},
		{
			name:    "missing name in single plugin",
			yaml:    "version: 1.0.0\ndescription: test\n",
			wantErr: "manifest requires a name",
		},
		{
			name:    "missing version in single plugin",
			yaml:    "name: test\ndescription: test\n",
			wantErr: "manifest requires a version",
		},
		{
			name:    "missing description in single plugin",
			yaml:    "name: test\nversion: 1.0.0\n",
			wantErr: "manifest requires a description",
		},
		{
			name: "both name and plugins is an error",
			yaml: `
name: conflict
version: 1.0.0
description: "Conflict"
plugins:
  - name: sub
    version: 1.0.0
    description: "Sub"
`,
			wantErr: "manifest has both top-level 'name' and 'plugins'; use one or the other",
		},
		{
			name:    "empty plugins list",
			yaml:    "plugins: []\n",
			wantErr: "no plugins defined in claudia.yaml",
		},
		{
			name: "plugin missing name",
			yaml: `
plugins:
  - version: 1.0.0
    description: "No name"
`,
			wantErr: "plugin 0: requires a name",
		},
		{
			name: "plugin missing version",
			yaml: `
plugins:
  - name: test
    description: "No version"
`,
			wantErr: "plugin 'test': requires a version",
		},
		{
			name: "plugin missing description",
			yaml: `
plugins:
  - name: test
    version: 1.0.0
`,
			wantErr: "plugin 'test': requires a description",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			err := os.WriteFile(filepath.Join(dir, "claudia.yaml"), []byte(tt.yaml), 0o644)
			if err != nil {
				t.Fatal(err)
			}

			m, err := Load(dir)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !containsStr(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if m == nil {
				t.Fatal("expected non-nil manifest")
			}
		})
	}
}

func TestLoadFileNotFound(t *testing.T) {
	_, err := Load(t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing claudia.yaml")
	}
	if !containsStr(err.Error(), "claudia.yaml not found") {
		t.Fatalf("expected 'not found' error, got %q", err.Error())
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/manifest/ -v`
Expected: FAIL — `Load` is not defined

- [ ] **Step 3: Implement types and Load function**

Replace the contents of `internal/manifest/manifest.go` (after the license header):

```go
// Package manifest handles claudia.yaml parsing and validation.
package manifest

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Manifest struct {
	Author   Author `yaml:"author"`
	License  string `yaml:"license"`
	Homepage string `yaml:"homepage"`

	Name        string     `yaml:"name"`
	Version     string     `yaml:"version"`
	Description string     `yaml:"description"`
	Keywords    []string   `yaml:"keywords"`
	Category    string     `yaml:"category"`
	Skills      []Entry    `yaml:"skills"`
	Commands    []Entry    `yaml:"commands"`
	Hooks       []Entry    `yaml:"hooks"`
	Agents      []Entry    `yaml:"agents"`
	MCP         []MCPEntry `yaml:"mcp"`
	Binaries    []Entry    `yaml:"binaries"`
	Settings    []Entry    `yaml:"settings"`

	Plugins []Plugin `yaml:"plugins"`
}

type Plugin struct {
	Name        string     `yaml:"name"`
	Version     string     `yaml:"version"`
	Description string     `yaml:"description"`
	Author      Author     `yaml:"author"`
	License     string     `yaml:"license"`
	Homepage    string     `yaml:"homepage"`
	Keywords    []string   `yaml:"keywords"`
	Category    string     `yaml:"category"`
	Skills      []Entry    `yaml:"skills"`
	Commands    []Entry    `yaml:"commands"`
	Hooks       []Entry    `yaml:"hooks"`
	Agents      []Entry    `yaml:"agents"`
	MCP         []MCPEntry `yaml:"mcp"`
	Binaries    []Entry    `yaml:"binaries"`
	Settings    []Entry    `yaml:"settings"`
}

type Author struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
}

type MCPEntry struct {
	Type      string   `yaml:"type"`
	Src       string   `yaml:"src"`
	Config    string   `yaml:"config"`
	Package   string   `yaml:"package"`
	Platforms []string `yaml:"platforms"`
}

type Entry struct {
	Glob string `yaml:"-"`
	Src  string `yaml:"src"`
	Dest string `yaml:"dest"`
}

func (e *Entry) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		e.Glob = value.Value
		return nil
	}
	type raw Entry
	return value.Decode((*raw)(e))
}

func Load(dir string) (*Manifest, error) {
	path := filepath.Join(dir, "claudia.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("claudia.yaml not found in %s", dir)
		}
		return nil, fmt.Errorf("reading claudia.yaml: %w", err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing claudia.yaml: %w", err)
	}

	if err := m.validate(); err != nil {
		return nil, err
	}

	return &m, nil
}

func (m *Manifest) validate() error {
	hasSingle := m.Name != ""
	hasMulti := len(m.Plugins) > 0

	if hasSingle && hasMulti {
		return fmt.Errorf("manifest has both top-level 'name' and 'plugins'; use one or the other")
	}

	if hasSingle {
		return m.validateSingle()
	}

	if m.Plugins != nil && len(m.Plugins) == 0 {
		return fmt.Errorf("no plugins defined in claudia.yaml")
	}

	if !hasSingle && !hasMulti {
		return fmt.Errorf("manifest requires a name or a plugins list")
	}

	for i, p := range m.Plugins {
		if p.Name == "" {
			return fmt.Errorf("plugin %d: requires a name", i)
		}
		if p.Version == "" {
			return fmt.Errorf("plugin '%s': requires a version", p.Name)
		}
		if p.Description == "" {
			return fmt.Errorf("plugin '%s': requires a description", p.Name)
		}
	}
	return nil
}

func (m *Manifest) validateSingle() error {
	if m.Name == "" {
		return fmt.Errorf("manifest requires a name")
	}
	if m.Version == "" {
		return fmt.Errorf("manifest requires a version")
	}
	if m.Description == "" {
		return fmt.Errorf("manifest requires a description")
	}
	return nil
}

// Normalize converts a single-plugin manifest into the multi-plugin form
// and applies shared field inheritance. After Normalize, callers always
// work with m.Plugins.
func Normalize(m *Manifest) []Plugin {
	if len(m.Plugins) > 0 {
		for i := range m.Plugins {
			inheritShared(m, &m.Plugins[i])
		}
		return m.Plugins
	}

	p := Plugin{
		Name:        m.Name,
		Version:     m.Version,
		Description: m.Description,
		Author:      m.Author,
		License:     m.License,
		Homepage:    m.Homepage,
		Keywords:    m.Keywords,
		Category:    m.Category,
		Skills:      m.Skills,
		Commands:    m.Commands,
		Hooks:       m.Hooks,
		Agents:      m.Agents,
		MCP:         m.MCP,
		Binaries:    m.Binaries,
		Settings:    m.Settings,
	}
	return []Plugin{p}
}

func inheritShared(m *Manifest, p *Plugin) {
	if p.Author.Name == "" && p.Author.Email == "" {
		p.Author = m.Author
	}
	if p.License == "" {
		p.License = m.License
	}
	if p.Homepage == "" {
		p.Homepage = m.Homepage
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/manifest/ -v`
Expected: PASS

- [ ] **Step 5: Write tests for Normalize and Entry unmarshaling**

Add to `internal/manifest/manifest_test.go`:

```go
func TestNormalize(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		wantCount  int
		wantName   string
		wantAuthor string
	}{
		{
			name: "single plugin normalizes to one-element slice",
			yaml: `
name: solo
version: 1.0.0
description: "Solo plugin"
author:
  name: "Solo Author"
`,
			wantCount:  1,
			wantName:   "solo",
			wantAuthor: "Solo Author",
		},
		{
			name: "multi plugin preserves all plugins",
			yaml: `
author:
  name: "Shared"
plugins:
  - name: a
    version: 1.0.0
    description: "A"
  - name: b
    version: 2.0.0
    description: "B"
`,
			wantCount:  2,
			wantName:   "a",
			wantAuthor: "Shared",
		},
		{
			name: "plugin overrides shared author",
			yaml: `
author:
  name: "Shared"
plugins:
  - name: a
    version: 1.0.0
    description: "A"
    author:
      name: "Override"
`,
			wantCount:  1,
			wantName:   "a",
			wantAuthor: "Override",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "claudia.yaml"), []byte(tt.yaml), 0o644); err != nil {
				t.Fatal(err)
			}

			m, err := Load(dir)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			plugins := Normalize(m)
			if len(plugins) != tt.wantCount {
				t.Fatalf("expected %d plugins, got %d", tt.wantCount, len(plugins))
			}
			if plugins[0].Name != tt.wantName {
				t.Fatalf("expected name %q, got %q", tt.wantName, plugins[0].Name)
			}
			if plugins[0].Author.Name != tt.wantAuthor {
				t.Fatalf("expected author %q, got %q", tt.wantAuthor, plugins[0].Author.Name)
			}
		})
	}
}

func TestEntryUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		wantGlob string
		wantSrc  string
		wantDest string
	}{
		{
			name:     "bare string becomes glob",
			yaml:     "skills:\n  - skills/*.md\n",
			wantGlob: "skills/*.md",
		},
		{
			name:     "src dest object",
			yaml:     "skills:\n  - src: prompts/*.md\n    dest: skills/\n",
			wantSrc:  "prompts/*.md",
			wantDest: "skills/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m struct {
				Skills []Entry `yaml:"skills"`
			}
			if err := yaml.Unmarshal([]byte(tt.yaml), &m); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			if len(m.Skills) != 1 {
				t.Fatalf("expected 1 entry, got %d", len(m.Skills))
			}
			e := m.Skills[0]
			if tt.wantGlob != "" && e.Glob != tt.wantGlob {
				t.Fatalf("expected glob %q, got %q", tt.wantGlob, e.Glob)
			}
			if tt.wantSrc != "" && e.Src != tt.wantSrc {
				t.Fatalf("expected src %q, got %q", tt.wantSrc, e.Src)
			}
			if tt.wantDest != "" && e.Dest != tt.wantDest {
				t.Fatalf("expected dest %q, got %q", tt.wantDest, e.Dest)
			}
		})
	}
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/manifest/ -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/manifest/
git commit -m "feat(manifest): add types, Load, Normalize, Entry unmarshaling"
```

---

### Task 2: Entry Resolution (Globs and Source/Dest Mappings)

**Files:**
- Create: `internal/manifest/resolve.go`
- Create: `internal/manifest/resolve_test.go`

- [ ] **Step 1: Write the failing tests for ResolveEntries**

Create `internal/manifest/resolve_test.go`:

```go
package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveEntries(t *testing.T) {
	tests := []struct {
		name      string
		entries   []Entry
		files     map[string]string // path -> content to create
		wantCount int
		wantErr   string
	}{
		{
			name:    "glob matches files",
			entries: []Entry{{Glob: "skills/*.md"}},
			files: map[string]string{
				"skills/one.md": "# One",
				"skills/two.md": "# Two",
			},
			wantCount: 2,
		},
		{
			name:    "glob matches no files",
			entries: []Entry{{Glob: "skills/*.md"}},
			files:   map[string]string{},
			wantErr: "pattern 'skills/*.md' matched no files",
		},
		{
			name:    "src/dest with directory dest",
			entries: []Entry{{Src: "prompts/review.md", Dest: "skills/"}},
			files: map[string]string{
				"prompts/review.md": "# Review",
			},
			wantCount: 1,
		},
		{
			name:    "src/dest with file dest (rename)",
			entries: []Entry{{Src: "prompts/review.md", Dest: "skills/code-review.md"}},
			files: map[string]string{
				"prompts/review.md": "# Review",
			},
			wantCount: 1,
		},
		{
			name:    "src/dest with glob src",
			entries: []Entry{{Src: "prompts/*.md", Dest: "skills/"}},
			files: map[string]string{
				"prompts/one.md": "# One",
				"prompts/two.md": "# Two",
			},
			wantCount: 2,
		},
		{
			name:    "src file not found",
			entries: []Entry{{Src: "missing.md", Dest: "skills/"}},
			files:   map[string]string{},
			wantErr: "pattern 'missing.md' matched no files",
		},
		{
			name:    "empty entries returns empty",
			entries: []Entry{},
			files:   map[string]string{},
		},
		{
			name:    "nil entries returns empty",
			entries: nil,
			files:   map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for path, content := range tt.files {
				full := filepath.Join(dir, path)
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			pairs, err := ResolveEntries(dir, tt.entries)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !containsStr(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(pairs) != tt.wantCount {
				t.Fatalf("expected %d pairs, got %d", tt.wantCount, len(pairs))
			}
		})
	}
}

func TestResolveEntriesDestPaths(t *testing.T) {
	tests := []struct {
		name     string
		entry    Entry
		files    map[string]string
		wantDest string
	}{
		{
			name:     "glob preserves relative path",
			entry:    Entry{Glob: "skills/review.md"},
			files:    map[string]string{"skills/review.md": "x"},
			wantDest: "skills/review.md",
		},
		{
			name:     "src/dest dir preserves filename",
			entry:    Entry{Src: "prompts/review.md", Dest: "skills/"},
			files:    map[string]string{"prompts/review.md": "x"},
			wantDest: "skills/review.md",
		},
		{
			name:     "src/dest file renames",
			entry:    Entry{Src: "prompts/review.md", Dest: "skills/renamed.md"},
			files:    map[string]string{"prompts/review.md": "x"},
			wantDest: "skills/renamed.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for path, content := range tt.files {
				full := filepath.Join(dir, path)
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			pairs, err := ResolveEntries(dir, []Entry{tt.entry})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(pairs) != 1 {
				t.Fatalf("expected 1 pair, got %d", len(pairs))
			}
			if pairs[0].Dest != tt.wantDest {
				t.Fatalf("expected dest %q, got %q", tt.wantDest, pairs[0].Dest)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/manifest/ -v -run Resolve`
Expected: FAIL — `ResolveEntries` and `FilePair` not defined

- [ ] **Step 3: Implement ResolveEntries**

Create `internal/manifest/resolve.go`:

```go
// Package manifest handles claudia.yaml parsing and validation.
package manifest

import (
	"fmt"
	"path/filepath"
	"strings"
)

type FilePair struct {
	Src  string
	Dest string
}

func ResolveEntries(baseDir string, entries []Entry) ([]FilePair, error) {
	var result []FilePair

	for _, e := range entries {
		pairs, err := resolveEntry(baseDir, e)
		if err != nil {
			return nil, err
		}
		result = append(result, pairs...)
	}

	return result, nil
}

func resolveEntry(baseDir string, e Entry) ([]FilePair, error) {
	pattern := e.Glob
	if pattern == "" {
		pattern = e.Src
	}
	if pattern == "" {
		return nil, fmt.Errorf("entry has neither glob nor src")
	}

	matches, err := filepath.Glob(filepath.Join(baseDir, pattern))
	if err != nil {
		return nil, fmt.Errorf("invalid glob %q: %w", pattern, err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("pattern '%s' matched no files", pattern)
	}

	var pairs []FilePair
	for _, abs := range matches {
		rel, err := filepath.Rel(baseDir, abs)
		if err != nil {
			return nil, fmt.Errorf("computing relative path: %w", err)
		}

		dest := computeDest(e, rel)
		pairs = append(pairs, FilePair{Src: abs, Dest: dest})
	}

	return pairs, nil
}

func computeDest(e Entry, relSrc string) string {
	if e.Glob != "" {
		return relSrc
	}

	if strings.HasSuffix(e.Dest, "/") {
		return e.Dest + filepath.Base(relSrc)
	}

	return e.Dest
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/manifest/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/manifest/resolve.go internal/manifest/resolve_test.go
git commit -m "feat(manifest): add ResolveEntries for glob and src/dest mapping"
```

---

### Task 3: Git Metadata Capture

**Files:**
- Modify: `internal/metadata/metadata.go`
- Create: `internal/metadata/metadata_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/metadata/metadata_test.go`:

```go
package metadata

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCapture(t *testing.T) {
	tests := []struct {
		name       string
		setupRepo  bool
		detached   bool
		wantErr    string
		wantBranch string
	}{
		{
			name:       "captures from git repo",
			setupRepo:  true,
			wantBranch: "main",
		},
		{
			name:    "fails outside git repo",
			wantErr: "not a git repository",
		},
		{
			name:       "detached HEAD",
			setupRepo:  true,
			detached:   true,
			wantBranch: "HEAD",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			if tt.setupRepo {
				initTestRepo(t, dir)
				if tt.detached {
					run(t, dir, "git", "checkout", "--detach")
				}
			}

			meta, err := Capture(dir, "test-plugin", "1.0.0")
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !containsStr(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if meta.Name != "test-plugin" {
				t.Fatalf("expected name 'test-plugin', got %q", meta.Name)
			}
			if meta.Version != "1.0.0" {
				t.Fatalf("expected version '1.0.0', got %q", meta.Version)
			}
			if meta.GitCommitSha == "" {
				t.Fatal("expected non-empty git SHA")
			}
			if len(meta.GitCommitSha) != 40 {
				t.Fatalf("expected 40-char SHA, got %d chars", len(meta.GitCommitSha))
			}
			if meta.GitBranch != tt.wantBranch {
				t.Fatalf("expected branch %q, got %q", tt.wantBranch, meta.GitBranch)
			}
			if meta.BuildTimestamp == "" {
				t.Fatal("expected non-empty timestamp")
			}
			if meta.Platform == "" {
				t.Fatal("expected non-empty platform")
			}
		})
	}
}

func initTestRepo(t *testing.T, dir string) {
	t.Helper()
	run(t, dir, "git", "init")
	run(t, dir, "git", "checkout", "-b", "main")
	f := filepath.Join(dir, "README.md")
	if err := os.WriteFile(f, []byte("# test"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "init")
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %s\n%s", name, args, err, out)
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/metadata/ -v`
Expected: FAIL — `Capture` not defined

- [ ] **Step 3: Implement Capture**

Replace `internal/metadata/metadata.go` contents (after license header):

```go
// Package metadata captures git SHA, version, and timestamp for archives.
package metadata

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type Metadata struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	GitCommitSha   string `json:"gitCommitSha"`
	GitBranch      string `json:"gitBranch"`
	BuildTimestamp string `json:"buildTimestamp"`
	BuilderVersion string `json:"builderVersion"`
	Platform       string `json:"platform"`
}

func Capture(dir string, name string, version string) (*Metadata, error) {
	sha, err := gitCmd(dir, "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("not a git repository; claudia build requires git metadata: %w", err)
	}

	branch, err := gitCmd(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("detecting git branch: %w", err)
	}

	return &Metadata{
		Name:           name,
		Version:        version,
		GitCommitSha:   sha,
		GitBranch:      branch,
		BuildTimestamp: time.Now().UTC().Format(time.RFC3339),
		BuilderVersion: "dev",
		Platform:       runtime.GOOS + "-" + runtime.GOARCH,
	}, nil
}

func gitCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/metadata/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/metadata/
git commit -m "feat(metadata): add Capture for git SHA, branch, timestamp"
```

---

### Task 4: Per-File Checksumming

**Files:**
- Modify: `internal/checksum/checksum.go`
- Create: `internal/checksum/checksum_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/checksum/checksum_test.go`:

```go
package checksum

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputeFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantLen int
	}{
		{
			name:    "non-empty file",
			content: "hello world",
			wantLen: 64,
		},
		{
			name:    "empty file",
			content: "",
			wantLen: 64,
		},
		{
			name:    "binary-like content",
			content: "\x00\x01\x02\x03",
			wantLen: 64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := filepath.Join(t.TempDir(), "test")
			if err := os.WriteFile(f, []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			hash, err := ComputeFile(f)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(hash) != tt.wantLen {
				t.Fatalf("expected hash length %d, got %d", tt.wantLen, len(hash))
			}
		})
	}
}

func TestComputeFileMissing(t *testing.T) {
	_, err := ComputeFile("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestWriteAndReadFile(t *testing.T) {
	dir := t.TempDir()

	entries := []Entry{
		{Hash: "aabbccdd", Path: "file1.txt"},
		{Hash: "11223344", Path: "dir/file2.txt"},
	}

	path := filepath.Join(dir, "checksums.txt")
	if err := WriteFile(path, entries); err != nil {
		t.Fatalf("write error: %v", err)
	}

	got, err := ReadFile(path)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if len(got) != len(entries) {
		t.Fatalf("expected %d entries, got %d", len(entries), len(got))
	}
	for i, e := range got {
		if e.Hash != entries[i].Hash {
			t.Fatalf("entry %d: expected hash %q, got %q", i, entries[i].Hash, e.Hash)
		}
		if e.Path != entries[i].Path {
			t.Fatalf("entry %d: expected path %q, got %q", i, entries[i].Path, e.Path)
		}
	}
}

func TestVerify(t *testing.T) {
	dir := t.TempDir()
	content := "verify me"
	fpath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(fpath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	hash, err := ComputeFile(fpath)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		entries []Entry
		wantErr bool
	}{
		{
			name:    "matching checksum passes",
			entries: []Entry{{Hash: hash, Path: "test.txt"}},
		},
		{
			name:    "mismatched checksum fails",
			entries: []Entry{{Hash: "0000000000000000000000000000000000000000000000000000000000000000", Path: "test.txt"}},
			wantErr: true,
		},
		{
			name:    "missing file fails",
			entries: []Entry{{Hash: hash, Path: "missing.txt"}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := Verify(dir, tt.entries)
			if tt.wantErr {
				hasFailure := false
				for _, r := range results {
					if !r.OK {
						hasFailure = true
						break
					}
				}
				if err == nil && !hasFailure {
					t.Fatal("expected failure but all passed")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, r := range results {
				if !r.OK {
					t.Fatalf("expected %s to pass", r.Path)
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/checksum/ -v`
Expected: FAIL — types and functions not defined

- [ ] **Step 3: Implement checksum package**

Replace `internal/checksum/checksum.go` contents (after license header):

```go
// Package checksum handles per-file SHA256 checksumming and verification.
package checksum

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Entry struct {
	Hash string
	Path string
}

type Result struct {
	Path string
	OK   bool
	Err  string
}

func ComputeFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hashing %s: %w", path, err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func WriteFile(path string, entries []Entry) error {
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "%s  %s\n", e.Hash, e.Path)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func ReadFile(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading checksums: %w", err)
	}

	var entries []Entry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed checksum line: %q", line)
		}
		entries = append(entries, Entry{Hash: parts[0], Path: parts[1]})
	}

	return entries, nil
}

func Verify(baseDir string, entries []Entry) ([]Result, error) {
	var results []Result

	for _, e := range entries {
		abs := filepath.Join(baseDir, e.Path)
		actual, err := ComputeFile(abs)
		if err != nil {
			results = append(results, Result{Path: e.Path, OK: false, Err: err.Error()})
			continue
		}
		if actual != e.Hash {
			results = append(results, Result{
				Path: e.Path,
				OK:   false,
				Err:  fmt.Sprintf("expected %s, got %s", e.Hash, actual),
			})
			continue
		}
		results = append(results, Result{Path: e.Path, OK: true})
	}

	return results, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/checksum/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/checksum/
git commit -m "feat(checksum): add ComputeFile, WriteFile, ReadFile, Verify"
```

---

### Task 5: Plugin Descriptor Generation

**Files:**
- Modify: `internal/plugin/plugin.go`
- Create: `internal/plugin/plugin_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/plugin/plugin_test.go`:

```go
package plugin

import (
	"encoding/json"
	"testing"

	"github.com/retr0h/claudia/internal/manifest"
)

func TestGenerateMarketplace(t *testing.T) {
	tests := []struct {
		name       string
		plugin     manifest.Plugin
		wantName   string
		wantOwner  string
		wantSource string
	}{
		{
			name: "full fields",
			plugin: manifest.Plugin{
				Name:        "my-plugin",
				Version:     "1.0.0",
				Description: "Test plugin",
				Author:      manifest.Author{Name: "Test", Email: "test@test.com"},
				License:     "MIT",
				Keywords:    []string{"test"},
				Category:    "dev",
			},
			wantName:   "my-plugin",
			wantOwner:  "Test",
			wantSource: "./",
		},
		{
			name: "minimal fields",
			plugin: manifest.Plugin{
				Name:        "minimal",
				Version:     "0.1.0",
				Description: "Minimal",
			},
			wantName:  "minimal",
			wantOwner: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := GenerateMarketplace(tt.plugin)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var m map[string]interface{}
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}

			if m["name"] != tt.wantName {
				t.Fatalf("expected name %q, got %v", tt.wantName, m["name"])
			}

			plugins := m["plugins"].([]interface{})
			if len(plugins) != 1 {
				t.Fatalf("expected 1 plugin entry, got %d", len(plugins))
			}

			p0 := plugins[0].(map[string]interface{})
			if p0["source"] != "./" {
				t.Fatalf("expected source './', got %v", p0["source"])
			}
		})
	}
}

func TestGeneratePlugin(t *testing.T) {
	tests := []struct {
		name         string
		plugin       manifest.Plugin
		commandPaths []string
		wantCommands int
	}{
		{
			name: "with commands",
			plugin: manifest.Plugin{
				Name:        "my-plugin",
				Version:     "1.0.0",
				Description: "Test",
			},
			commandPaths: []string{"./commands/init.md", "./commands/run.md"},
			wantCommands: 2,
		},
		{
			name: "without commands",
			plugin: manifest.Plugin{
				Name:        "my-plugin",
				Version:     "1.0.0",
				Description: "Test",
			},
			commandPaths: nil,
			wantCommands: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := GeneratePlugin(tt.plugin, tt.commandPaths)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var m map[string]interface{}
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}

			if m["name"] != tt.plugin.Name {
				t.Fatalf("expected name %q, got %v", tt.plugin.Name, m["name"])
			}

			cmds, hasCmds := m["commands"]
			if tt.wantCommands == 0 {
				if hasCmds {
					t.Fatal("expected no commands field")
				}
				return
			}
			cmdSlice := cmds.([]interface{})
			if len(cmdSlice) != tt.wantCommands {
				t.Fatalf("expected %d commands, got %d", tt.wantCommands, len(cmdSlice))
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/plugin/ -v`
Expected: FAIL — functions not defined

- [ ] **Step 3: Implement plugin descriptor generation**

Replace `internal/plugin/plugin.go` contents (after license header):

```go
// Package plugin generates Claude Code plugin directory structures.
package plugin

import (
	"encoding/json"

	"github.com/retr0h/claudia/internal/manifest"
)

type marketplaceJSON struct {
	Name     string            `json:"name"`
	Owner    *ownerJSON        `json:"owner,omitempty"`
	Metadata metadataJSON      `json:"metadata"`
	Plugins  []pluginEntryJSON `json:"plugins"`
}

type ownerJSON struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

type metadataJSON struct {
	Description string `json:"description"`
	Version     string `json:"version"`
}

type pluginEntryJSON struct {
	Name        string   `json:"name"`
	Source      string   `json:"source"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Author      *ownerJSON `json:"author,omitempty"`
	License     string   `json:"license,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
	Category    string   `json:"category,omitempty"`
}

type pluginJSON struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Author      *ownerJSON `json:"author,omitempty"`
	License     string   `json:"license,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
	Commands    []string `json:"commands,omitempty"`
}

func GenerateMarketplace(p manifest.Plugin) ([]byte, error) {
	m := marketplaceJSON{
		Name:     p.Name,
		Metadata: metadataJSON{Description: p.Description, Version: p.Version},
		Plugins: []pluginEntryJSON{
			{
				Name:        p.Name,
				Source:      "./",
				Description: p.Description,
				Version:     p.Version,
				License:     p.License,
				Homepage:    p.Homepage,
				Keywords:    p.Keywords,
				Category:    p.Category,
			},
		},
	}

	if p.Author.Name != "" || p.Author.Email != "" {
		owner := &ownerJSON{Name: p.Author.Name, Email: p.Author.Email}
		m.Owner = owner
		m.Plugins[0].Author = owner
	}

	return json.MarshalIndent(m, "", "  ")
}

func GeneratePlugin(p manifest.Plugin, commandPaths []string) ([]byte, error) {
	pj := pluginJSON{
		Name:        p.Name,
		Version:     p.Version,
		Description: p.Description,
		License:     p.License,
		Homepage:    p.Homepage,
		Keywords:    p.Keywords,
		Commands:    commandPaths,
	}

	if p.Author.Name != "" || p.Author.Email != "" {
		pj.Author = &ownerJSON{Name: p.Author.Name, Email: p.Author.Email}
	}

	if len(pj.Commands) == 0 {
		pj.Commands = nil
	}

	return json.MarshalIndent(pj, "", "  ")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/plugin/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/plugin/
git commit -m "feat(plugin): add GenerateMarketplace and GeneratePlugin"
```

---

### Task 6: Archive Creation and Extraction

**Files:**
- Modify: `internal/archive/archive.go`
- Create: `internal/archive/archive_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/archive/archive_test.go`:

```go
package archive

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateAndExtract(t *testing.T) {
	srcDir := t.TempDir()
	files := map[string]string{
		"marketplaces/test/.claude-plugin/plugin.json": `{"name":"test"}`,
		"marketplaces/test/skills/one.md":              "# One",
		"marketplaces/test/skills/two.md":              "# Two",
	}

	for path, content := range files {
		full := filepath.Join(srcDir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var entries []FileEntry
	for path := range files {
		entries = append(entries, FileEntry{
			SrcPath:     filepath.Join(srcDir, path),
			ArchivePath: path,
		})
	}

	outDir := t.TempDir()
	archivePath := filepath.Join(outDir, "test-1.0.0.claudia")

	if err := Create(archivePath, entries); err != nil {
		t.Fatalf("create error: %v", err)
	}

	info, err := os.Stat(archivePath)
	if err != nil {
		t.Fatalf("archive not created: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("archive is empty")
	}

	extractDir, err := Extract(archivePath)
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}
	defer func() { _ = os.RemoveAll(extractDir) }()

	for path, content := range files {
		data, err := os.ReadFile(filepath.Join(extractDir, path))
		if err != nil {
			t.Fatalf("missing extracted file %s: %v", path, err)
		}
		if string(data) != content {
			t.Fatalf("content mismatch for %s: got %q", path, string(data))
		}
	}
}

func TestCreateEmptyEntries(t *testing.T) {
	out := filepath.Join(t.TempDir(), "empty.claudia")
	err := Create(out, nil)
	if err == nil {
		t.Fatal("expected error for empty entries")
	}
}

func TestExtractMissingFile(t *testing.T) {
	_, err := Extract("/nonexistent/archive.claudia")
	if err == nil {
		t.Fatal("expected error for missing archive")
	}
}

func TestCreatePathTraversalRejected(t *testing.T) {
	entries := []FileEntry{
		{SrcPath: "/dev/null", ArchivePath: "../../../etc/passwd"},
	}
	out := filepath.Join(t.TempDir(), "evil.claudia")
	err := Create(out, entries)
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/archive/ -v`
Expected: FAIL — types and functions not defined

- [ ] **Step 3: Implement archive package**

Replace `internal/archive/archive.go` contents (after license header):

```go
// Package archive handles creation and extraction of .claudia tarballs.
package archive

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type FileEntry struct {
	SrcPath     string
	ArchivePath string
}

func Create(archivePath string, entries []FileEntry) error {
	if len(entries) == 0 {
		return fmt.Errorf("no files to archive")
	}

	for _, e := range entries {
		if strings.Contains(e.ArchivePath, "..") {
			return fmt.Errorf("path traversal rejected: %s", e.ArchivePath)
		}
	}

	f, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("creating archive: %w", err)
	}
	defer func() { _ = f.Close() }()

	gw := gzip.NewWriter(f)
	defer func() { _ = gw.Close() }()

	tw := tar.NewWriter(gw)
	defer func() { _ = tw.Close() }()

	for _, e := range entries {
		if err := addToTar(tw, e); err != nil {
			return fmt.Errorf("adding %s: %w", e.ArchivePath, err)
		}
	}

	return nil
}

func addToTar(tw *tar.Writer, e FileEntry) error {
	info, err := os.Lstat(e.SrcPath)
	if err != nil {
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("symlinks not supported: %s", e.SrcPath)
	}

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = e.ArchivePath

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	if info.IsDir() {
		return nil
	}

	f, err := os.Open(e.SrcPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	_, err = io.Copy(tw, f)
	return err
}

func Extract(archivePath string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("opening archive: %w", err)
	}
	defer func() { _ = f.Close() }()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("gzip reader: %w", err)
	}
	defer func() { _ = gr.Close() }()

	dir, err := os.MkdirTemp("", "claudia-extract-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = os.RemoveAll(dir)
			return "", fmt.Errorf("reading tar: %w", err)
		}

		clean := filepath.Clean(header.Name)
		if strings.Contains(clean, "..") {
			_ = os.RemoveAll(dir)
			return "", fmt.Errorf("path traversal in archive: %s", header.Name)
		}

		target := filepath.Join(dir, clean)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				_ = os.RemoveAll(dir)
				return "", err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				_ = os.RemoveAll(dir)
				return "", err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				_ = os.RemoveAll(dir)
				return "", err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				_ = os.RemoveAll(dir)
				return "", err
			}
			_ = out.Close()
		default:
			_ = os.RemoveAll(dir)
			return "", fmt.Errorf("unsupported tar entry type %d for %s", header.Typeflag, header.Name)
		}
	}

	return dir, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/archive/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/archive/
git commit -m "feat(archive): add Create and Extract for .claudia tarballs"
```

---

### Task 7: Build Command — Full Pipeline

**Files:**
- Modify: `cmd/build.go`

- [ ] **Step 1: Implement the build pipeline**

Replace `cmd/build.go` contents (after license header):

```go
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/retr0h/claudia/internal/archive"
	"github.com/retr0h/claudia/internal/checksum"
	"github.com/retr0h/claudia/internal/manifest"
	"github.com/retr0h/claudia/internal/metadata"
	"github.com/retr0h/claudia/internal/plugin"
)

var buildCmd = &cobra.Command{
	Use:   "build [plugin-names...]",
	Short: "Build a .claudia archive from a claudia.yaml manifest",
	RunE:  runBuild,
}

func init() {
	rootCmd.AddCommand(buildCmd)
}

func runBuild(_ *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	m, err := manifest.Load(cwd)
	if err != nil {
		return err
	}

	plugins := manifest.Normalize(m)

	if len(args) > 0 {
		plugins, err = filterPlugins(plugins, args)
		if err != nil {
			return err
		}
	}

	built := 0
	for _, p := range plugins {
		if err := buildPlugin(cwd, p); err != nil {
			return fmt.Errorf("building %s: %w", p.Name, err)
		}
		built++
	}

	if built > 1 {
		fmt.Printf("\n%d archives built\n", built)
	}

	return nil
}

func filterPlugins(plugins []manifest.Plugin, names []string) ([]manifest.Plugin, error) {
	index := make(map[string]manifest.Plugin, len(plugins))
	for _, p := range plugins {
		index[p.Name] = p
	}

	var result []manifest.Plugin
	for _, name := range names {
		p, ok := index[name]
		if !ok {
			return nil, fmt.Errorf("plugin '%s' not found in claudia.yaml", name)
		}
		result = append(result, p)
	}
	return result, nil
}

func buildPlugin(cwd string, p manifest.Plugin) error {
	meta, err := metadata.Capture(cwd, p.Name, p.Version)
	if err != nil {
		return err
	}

	shortSha := meta.GitCommitSha
	if len(shortSha) > 7 {
		shortSha = shortSha[:7]
	}
	fmt.Printf("claudia: building %s v%s (%s)\n\n", p.Name, p.Version, shortSha)

	prefix := filepath.Join("marketplaces", p.Name)
	staging := filepath.Join(os.TempDir(), fmt.Sprintf("claudia-stage-%s-%s", p.Name, p.Version))
	_ = os.RemoveAll(staging)
	defer func() { _ = os.RemoveAll(staging) }()

	var archiveEntries []archive.FileEntry
	var checksumEntries []checksum.Entry

	type section struct {
		name    string
		entries []manifest.Entry
		destDir string
	}

	sections := []section{
		{"skills", p.Skills, "skills"},
		{"commands", p.Commands, "commands"},
		{"hooks", p.Hooks, "hooks"},
		{"agents", p.Agents, "agents"},
		{"binaries", p.Binaries, "bin"},
		{"settings", p.Settings, "settings"},
	}

	for _, s := range sections {
		if len(s.entries) == 0 {
			continue
		}
		pairs, err := manifest.ResolveEntries(cwd, s.entries)
		if err != nil {
			return err
		}
		fmt.Printf("  %-14s%d files\n", s.name+"/", len(pairs))
		for _, fp := range pairs {
			archPath := filepath.Join(prefix, fp.Dest)
			archiveEntries = append(archiveEntries, archive.FileEntry{
				SrcPath:     fp.Src,
				ArchivePath: archPath,
			})
		}
	}

	for _, mcp := range p.MCP {
		switch mcp.Type {
		case "binary":
			srcPath := filepath.Join(cwd, mcp.Src)
			if _, err := os.Stat(srcPath); err != nil {
				return fmt.Errorf("file not found: %s", mcp.Src)
			}
			archPath := filepath.Join(prefix, "mcp", filepath.Base(mcp.Src))
			archiveEntries = append(archiveEntries, archive.FileEntry{
				SrcPath:     srcPath,
				ArchivePath: archPath,
			})
			platforms := "all"
			if len(mcp.Platforms) > 0 {
				platforms = fmt.Sprintf("%v", mcp.Platforms)
			}
			fmt.Printf("  %-14s1 binary (%s)\n", "mcp/binary", platforms)
		case "remote":
			srcPath := filepath.Join(cwd, mcp.Config)
			if _, err := os.Stat(srcPath); err != nil {
				return fmt.Errorf("file not found: %s", mcp.Config)
			}
			archPath := filepath.Join(prefix, "mcp", filepath.Base(mcp.Config))
			archiveEntries = append(archiveEntries, archive.FileEntry{
				SrcPath:     srcPath,
				ArchivePath: archPath,
			})
			fmt.Printf("  %-14s1 config\n", "mcp/remote")
		case "ux":
			uxConfig := map[string]string{"package": mcp.Package}
			uxData, err := json.MarshalIndent(uxConfig, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling ux config: %w", err)
			}
			uxPath := filepath.Join(staging, "ux.json")
			if err := os.MkdirAll(filepath.Dir(uxPath), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(uxPath, uxData, 0o644); err != nil {
				return err
			}
			archiveEntries = append(archiveEntries, archive.FileEntry{
				SrcPath:     uxPath,
				ArchivePath: filepath.Join(prefix, "mcp", "ux.json"),
			})
			fmt.Printf("  %-14s1 package (%s)\n", "mcp/ux", mcp.Package)
		default:
			return fmt.Errorf("unknown mcp type '%s'; expected binary, remote, or ux", mcp.Type)
		}
	}

	var commandPaths []string
	if len(p.Commands) > 0 {
		cmdPairs, err := manifest.ResolveEntries(cwd, p.Commands)
		if err != nil {
			return err
		}
		for _, fp := range cmdPairs {
			commandPaths = append(commandPaths, "./"+fp.Dest)
		}
	}

	marketplaceData, err := plugin.GenerateMarketplace(p)
	if err != nil {
		return fmt.Errorf("generating marketplace.json: %w", err)
	}
	pluginData, err := plugin.GeneratePlugin(p, commandPaths)
	if err != nil {
		return fmt.Errorf("generating plugin.json: %w", err)
	}

	generatedFiles := map[string][]byte{
		filepath.Join(prefix, ".claude-plugin", "marketplace.json"): marketplaceData,
		filepath.Join(prefix, ".claude-plugin", "plugin.json"):      pluginData,
	}

	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}

	manifestData, err := os.ReadFile(filepath.Join(cwd, "claudia.yaml"))
	if err != nil {
		return fmt.Errorf("reading claudia.yaml: %w", err)
	}

	generatedFiles[filepath.Join(prefix, ".claudia", "metadata.json")] = metaData
	generatedFiles[filepath.Join(prefix, ".claudia", "claudia.yaml")] = manifestData

	for archPath, data := range generatedFiles {
		tmpFile := filepath.Join(staging, archPath)
		if err := os.MkdirAll(filepath.Dir(tmpFile), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(tmpFile, data, 0o644); err != nil {
			return err
		}
		archiveEntries = append(archiveEntries, archive.FileEntry{
			SrcPath:     tmpFile,
			ArchivePath: archPath,
		})
	}

	for _, ae := range archiveEntries {
		hash, err := checksum.ComputeFile(ae.SrcPath)
		if err != nil {
			return fmt.Errorf("checksumming %s: %w", ae.ArchivePath, err)
		}
		checksumEntries = append(checksumEntries, checksum.Entry{
			Hash: hash,
			Path: ae.ArchivePath,
		})
	}

	checksumPath := filepath.Join(staging, prefix, ".claudia", "checksums.txt")
	if err := os.MkdirAll(filepath.Dir(checksumPath), 0o755); err != nil {
		return err
	}
	if err := checksum.WriteFile(checksumPath, checksumEntries); err != nil {
		return fmt.Errorf("writing checksums: %w", err)
	}
	archiveEntries = append(archiveEntries, archive.FileEntry{
		SrcPath:     checksumPath,
		ArchivePath: filepath.Join(prefix, ".claudia", "checksums.txt"),
	})

	archiveName := fmt.Sprintf("%s-%s.claudia", p.Name, p.Version)
	if err := archive.Create(archiveName, archiveEntries); err != nil {
		return fmt.Errorf("creating archive: %w", err)
	}

	info, err := os.Stat(archiveName)
	if err != nil {
		return fmt.Errorf("stat archive: %w", err)
	}

	archiveHash, err := checksum.ComputeFile(archiveName)
	if err != nil {
		return fmt.Errorf("checksumming archive: %w", err)
	}

	fmt.Printf("\n  checksummed %d files (SHA256)\n", len(checksumEntries))
	fmt.Printf("\n  %s  (%s)\n", archiveName, formatSize(info.Size()))
	fmt.Printf("  sha256: %s\n\n", archiveHash)

	return nil
}

func formatSize(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
	)
	switch {
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
```

- [ ] **Step 2: Build and smoke test**

Run:
```bash
go build -o claudia . && go vet ./...
```
Expected: clean build

- [ ] **Step 3: Create a test plugin and run claudia build**

```bash
mkdir -p /tmp/test-plugin/skills /tmp/test-plugin/commands
echo "# Skill One" > /tmp/test-plugin/skills/one.md
echo "# Skill Two" > /tmp/test-plugin/skills/two.md
echo "# Init Command" > /tmp/test-plugin/commands/init.md

cd /tmp/test-plugin
git init && git add . && git commit -m "init"

cat > claudia.yaml << 'EOF'
name: test-plugin
version: 0.1.0
description: "A test plugin"
author:
  name: "Test Author"
  email: "test@test.com"
license: MIT
skills:
  - skills/*.md
commands:
  - commands/*.md
EOF

/path/to/claudia build
```

Expected: produces `test-plugin-0.1.0.claudia` with summary output

- [ ] **Step 4: Verify the archive contents are correct**

```bash
tar tzf test-plugin-0.1.0.claudia | sort
```

Expected output should show `marketplaces/test-plugin/` prefix on all files.

- [ ] **Step 5: Commit**

```bash
git add cmd/build.go
git commit -m "feat(cli): implement claudia build pipeline"
```

---

### Task 8: Verify Command — Full Pipeline

**Files:**
- Modify: `cmd/verify.go`

- [ ] **Step 1: Implement the verify pipeline**

Replace `cmd/verify.go` contents (after license header):

```go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/retr0h/claudia/internal/archive"
	"github.com/retr0h/claudia/internal/checksum"
)

var verifyCmd = &cobra.Command{
	Use:   "verify <archive>",
	Short: "Verify checksums of a .claudia archive",
	Args:  cobra.ExactArgs(1),
	RunE:  runVerify,
}

func init() {
	rootCmd.AddCommand(verifyCmd)
}

func runVerify(_ *cobra.Command, args []string) error {
	archivePath := args[0]

	if _, err := os.Stat(archivePath); err != nil {
		return fmt.Errorf("archive not found: %s", archivePath)
	}

	fmt.Printf("claudia: verifying %s\n\n", filepath.Base(archivePath))

	extractDir, err := archive.Extract(archivePath)
	if err != nil {
		return fmt.Errorf("extracting archive: %w", err)
	}
	defer func() { _ = os.RemoveAll(extractDir) }()

	checksumFiles, err := findChecksumFiles(extractDir)
	if err != nil {
		return err
	}

	if len(checksumFiles) == 0 {
		return fmt.Errorf("no checksums.txt found in archive")
	}

	allPassed := true
	totalFiles := 0

	for _, csFile := range checksumFiles {
		entries, err := checksum.ReadFile(csFile)
		if err != nil {
			return fmt.Errorf("reading checksums: %w", err)
		}

		results, err := checksum.Verify(extractDir, entries)
		if err != nil {
			return fmt.Errorf("verifying checksums: %w", err)
		}

		for _, r := range results {
			totalFiles++
			if r.OK {
				fmt.Printf("  %-50s OK\n", r.Path)
			} else {
				fmt.Printf("  %-50s FAIL  %s\n", r.Path, r.Err)
				allPassed = false
			}
		}
	}

	fmt.Printf("\n  %d/%d files verified\n", totalFiles, totalFiles)

	if !allPassed {
		return fmt.Errorf("checksum verification failed")
	}

	return nil
}

func findChecksumFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, "checksums.txt") &&
			strings.Contains(path, ".claudia") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
```

- [ ] **Step 2: Build and smoke test**

Run:
```bash
go build -o claudia . && go vet ./...
```
Expected: clean build

- [ ] **Step 3: Verify the test archive from Task 7**

```bash
./claudia verify test-plugin-0.1.0.claudia
```

Expected: all files show OK, exit 0

- [ ] **Step 4: Commit**

```bash
git add cmd/verify.go
git commit -m "feat(cli): implement claudia verify pipeline"
```

---

### Task 9: Full Integration Test

**Files:**
- Create: `internal/archive/integration_test.go`

- [ ] **Step 1: Write an end-to-end build+verify test**

Create `internal/archive/integration_test.go`:

```go
package archive_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildAndVerifyIntegration(t *testing.T) {
	claudia := buildClaudia(t)

	dir := t.TempDir()
	initTestRepo(t, dir)

	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "commands"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "one.md"), []byte("# Skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "commands", "init.md"), []byte("# Init"), 0o644); err != nil {
		t.Fatal(err)
	}

	manifest := `
name: integration-test
version: 0.1.0
description: "Integration test plugin"
author:
  name: "Test"
skills:
  - skills/*.md
commands:
  - commands/*.md
`
	if err := os.WriteFile(filepath.Join(dir, "claudia.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	run(t, dir, claudia, "build")

	archivePath := filepath.Join(dir, "integration-test-0.1.0.claudia")
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("archive not created: %v", err)
	}

	run(t, dir, claudia, "verify", archivePath)

	extractDir := t.TempDir()
	run(t, extractDir, "tar", "xzf", archivePath, "-C", extractDir)

	pluginJSON := filepath.Join(extractDir, "marketplaces", "integration-test", ".claude-plugin", "plugin.json")
	data, err := os.ReadFile(pluginJSON)
	if err != nil {
		t.Fatalf("plugin.json not found: %v", err)
	}

	var pj map[string]interface{}
	if err := json.Unmarshal(data, &pj); err != nil {
		t.Fatalf("invalid plugin.json: %v", err)
	}
	if pj["name"] != "integration-test" {
		t.Fatalf("expected name 'integration-test', got %v", pj["name"])
	}

	skillPath := filepath.Join(extractDir, "marketplaces", "integration-test", "skills", "one.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("skill file not extracted: %v", err)
	}
}

func buildClaudia(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "claudia")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("building claudia: %s\n%s", err, out)
	}
	return bin
}

func repoRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("finding repo root: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func initTestRepo(t *testing.T, dir string) {
	t.Helper()
	run(t, dir, "git", "init")
	run(t, dir, "git", "checkout", "-b", "main")

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmds := [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "init"},
	}
	for _, c := range cmds {
		run(t, dir, c[0], c[1:]...)
	}
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %s\n%s", name, args, err, out)
	}
}
```

- [ ] **Step 2: Run the integration test**

Run: `go test ./internal/archive/ -v -run Integration`
Expected: PASS

- [ ] **Step 3: Run all tests**

Run: `go test ./... -v`
Expected: all PASS

- [ ] **Step 4: Run vet**

Run: `go vet ./...`
Expected: clean

- [ ] **Step 5: Commit**

```bash
git add internal/archive/integration_test.go
git commit -m "test(archive): add build+verify integration test"
```

---

### Task 10: Final Validation

- [ ] **Step 1: Build the binary**

Run: `go build -o claudia .`
Expected: clean build

- [ ] **Step 2: Run all tests**

Run: `go test ./... -v`
Expected: all PASS

- [ ] **Step 3: Run vet and check formatting**

Run: `go vet ./... && gofmt -l .`
Expected: clean, no files listed by gofmt

- [ ] **Step 4: Test CLI help**

Run: `./claudia --help && ./claudia build --help && ./claudia verify --help`
Expected: help output shows correct usage for all commands

- [ ] **Step 5: Commit any final fixes if needed**
