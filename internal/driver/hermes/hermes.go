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

// Package hermes is the agentpack target driver for Hermes Agent.
// It installs skills into .agents/skills/ (local) or ~/.hermes/skills/
// (global), and merges hooks into ~/.hermes/config.yaml natively as YAML.
package hermes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/retr0h/agentpack/pkg/target"
	"gopkg.in/yaml.v3"
)

// Hermes is the target driver for Hermes Agent.
type Hermes struct {
	userHomeFunc func() (string, error)
	cwdFunc      func() (string, error)
	mkdirAllFunc func(string, os.FileMode) error
}

// New returns a production Hermes driver.
func New() *Hermes {
	return &Hermes{
		userHomeFunc: os.UserHomeDir,
		cwdFunc:      os.Getwd,
		mkdirAllFunc: os.MkdirAll,
	}
}

// Name returns the target identifier.
func (h *Hermes) Name() string { return "hermes-agent" }

// DisplayName returns the human-readable target name.
func (h *Hermes) DisplayName() string { return "Hermes Agent" }

// SupportedTypes returns the content types this driver can install.
func (h *Hermes) SupportedTypes() []string {
	return []string{"skill", "hook"}
}

// Detect returns true if the Hermes home directory exists.
func (h *Hermes) Detect() bool {
	home, err := h.userHomeFunc()
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(home, ".hermes"))

	return err == nil
}

// Install copies content from opts.SourceDir into the correct locations for
// Hermes Agent. When opts.Entries is non-empty the driver installs only the
// listed entries; otherwise it falls back to the legacy directory-walking
// behaviour. Returns the list of files written.
func (h *Hermes) Install(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(opts.Entries) > 0 {
		return h.installFromEntries(ctx, opts)
	}

	return h.installFromDirs(ctx, opts)
}

// installFromEntries installs only the content items listed in opts.Entries.
func (h *Hermes) installFromEntries(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	var allFiles []target.InstalledFile

	for _, entry := range opts.Entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		switch entry.Type {
		case "skill":
			written, err := h.installSkillEntry(ctx, opts, entry)
			if err != nil {
				return nil, err
			}

			allFiles = append(allFiles, written...)

		case "hook":
			configPath, err := h.hermesConfigPath()
			if err != nil {
				return nil, err
			}

			if err := h.installHooks(ctx, opts.SourceDir, configPath, opts.Name); err != nil {
				return nil, err
			}
		}
	}

	return allFiles, nil
}

// installFromDirs walks convention-named directories under opts.SourceDir
// and installs everything found. This is the legacy fallback when no manifest
// entries are provided. Skills and hooks are handled.
func (h *Hermes) installFromDirs(
	ctx context.Context,
	opts target.InstallOpts,
) ([]target.InstalledFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	baseDir, skillsDir, err := h.resolveDirs(opts)
	if err != nil {
		return nil, err
	}

	destDir := filepath.Join(skillsDir, opts.Name)

	if err := h.mkdirAllFunc(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir skills dir: %w", err)
	}

	skillsSrc := filepath.Join(opts.SourceDir, "skills")
	if err := copyTreeIfExists(ctx, skillsSrc, destDir); err != nil {
		return nil, fmt.Errorf("copy skills: %w", err)
	}

	files, err := enumerateFiles(ctx, destDir, baseDir)
	if err != nil {
		return nil, fmt.Errorf("enumerate installed files: %w", err)
	}

	configPath, configErr := h.hermesConfigPath()
	if configErr != nil {
		return nil, configErr
	}

	if err := h.installHooks(ctx, opts.SourceDir, configPath, opts.Name); err != nil {
		return nil, err
	}

	return files, nil
}

// installSkillEntry copies a single skill entry's tree into the skills
// directory.
func (h *Hermes) installSkillEntry(
	ctx context.Context,
	opts target.InstallOpts,
	entry target.ContentEntry,
) ([]target.InstalledFile, error) {
	baseDir, skillsDir, err := h.resolveDirs(opts)
	if err != nil {
		return nil, err
	}

	destDir := filepath.Join(skillsDir, entry.Name)

	if err := h.mkdirAllFunc(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir skills dir: %w", err)
	}

	if err := copyTreeIfExists(ctx, entry.Root, destDir); err != nil {
		return nil, fmt.Errorf("copy skills: %w", err)
	}

	return enumerateFiles(ctx, destDir, baseDir)
}

// resolveDirs returns (baseDir, skillsDir) based on whether the install is
// global or local.
func (h *Hermes) resolveDirs(opts target.InstallOpts) (string, string, error) {
	if opts.Global {
		home, err := h.userHomeFunc()
		if err != nil {
			return "", "", fmt.Errorf("home dir: %w", err)
		}

		return home, filepath.Join(home, ".hermes", "skills"), nil
	}

	dir := opts.Dir
	if dir == "" {
		cwd, err := h.cwdFunc()
		if err != nil {
			return "", "", fmt.Errorf("getwd: %w", err)
		}

		dir = cwd
	}

	return dir, filepath.Join(dir, ".agents", "skills"), nil
}

// hermesConfigPath returns the path to Hermes's global config file at
// ~/.hermes/config.yaml.
func (h *Hermes) hermesConfigPath() (string, error) {
	home, err := h.userHomeFunc()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}

	return filepath.Join(home, ".hermes", "config.yaml"), nil
}

// installHooks reads hooks/hooks.json from srcDir, parses the JSON hooks, and
// merges them into the "hooks" key of the Hermes YAML config at configPath.
// A "_plugin" field is injected into each hook entry for later removal. The
// config file is created when absent. Existing keys outside "hooks" are
// preserved.
func (h *Hermes) installHooks(
	_ context.Context,
	srcDir, configPath, pluginName string,
) error {
	hooksFile := filepath.Join(srcDir, "hooks", "hooks.json")
	if _, err := os.Stat(hooksFile); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(hooksFile)
	if err != nil {
		return fmt.Errorf("read hooks/hooks.json: %w", err)
	}

	var hooks map[string]any
	if err := json.Unmarshal(data, &hooks); err != nil {
		return fmt.Errorf("parse hooks/hooks.json: %w", err)
	}

	if err := mergeHooksYAML(configPath, pluginName, hooks); err != nil {
		return fmt.Errorf("merge hooks: %w", err)
	}

	return nil
}

// mergeHooksYAML reads the YAML config at path, merges hook entries under the
// "hooks" key, and writes back. Each entry is tagged with a "_plugin" field.
// The file and parent directories are created when absent.
func mergeHooksYAML(path, pluginName string, hooks map[string]any) error {
	cfg, err := readYAMLConfig(path)
	if err != nil {
		return err
	}

	hooksSection := getOrCreateYAMLMap(cfg, "hooks")

	for event, rawEntries := range hooks {
		entries, ok := rawEntries.([]any)
		if !ok {
			continue
		}

		existing := getOrCreateYAMLSlice(hooksSection, event)

		for _, rawEntry := range entries {
			entry, ok := rawEntry.(map[string]any)
			if !ok {
				continue
			}

			tagged := make(map[string]any, len(entry)+1)
			for k, v := range entry {
				tagged[k] = v
			}

			tagged["_plugin"] = pluginName
			existing = append(existing, tagged)
		}

		hooksSection[event] = existing
	}

	cfg["hooks"] = hooksSection

	return writeYAMLConfig(path, cfg)
}

// readYAMLConfig reads and unmarshals the YAML file at path into a map.
// When the file does not exist it returns an empty map.
func readYAMLConfig(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}

		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if doc == nil {
		doc = map[string]any{}
	}

	return doc, nil
}

// writeYAMLConfig marshals doc as YAML and writes it to path. Parent
// directories are created as needed.
func writeYAMLConfig(path string, doc map[string]any) error {
	data, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}

// getOrCreateYAMLMap returns the value at key as map[string]any, creating it
// when absent or when the existing value has an incompatible type.
func getOrCreateYAMLMap(doc map[string]any, key string) map[string]any {
	if v, ok := doc[key].(map[string]any); ok {
		return v
	}

	return map[string]any{}
}

// getOrCreateYAMLSlice returns the value at key in m as []any, creating it
// when absent or when the existing value has an incompatible type.
func getOrCreateYAMLSlice(m map[string]any, key string) []any {
	if v, ok := m[key].([]any); ok {
		return v
	}

	return []any{}
}

// List returns nil; Hermes does not store managed-plugin metadata.
func (h *Hermes) List() ([]target.InstalledPlugin, error) {
	return nil, nil
}

// enumerateFiles walks destDir and returns InstalledFile entries with paths
// relative to baseDir and SHA256 digests.
func enumerateFiles(ctx context.Context, destDir, baseDir string) ([]target.InstalledFile, error) {
	var files []target.InstalledFile

	err := filepath.WalkDir(destDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}

		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		rel, relErr := filepath.Rel(baseDir, path)
		if relErr != nil {
			return relErr
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}

		h := sha256.Sum256(data)
		files = append(files, target.InstalledFile{
			Path:   rel,
			SHA256: hex.EncodeToString(h[:]),
		})

		return nil
	})

	return files, err
}

func copyTreeIfExists(ctx context.Context, src string, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}

	return filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("rel path: %w", err)
		}

		tgt := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(tgt, 0o755)
		}

		return copyFile(path, tgt)
	})
}

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
