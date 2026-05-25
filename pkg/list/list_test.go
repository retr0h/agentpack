package list_test

import (
	"testing"

	"github.com/retr0h/agentpack/pkg/list"
	"github.com/retr0h/agentpack/pkg/registry"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T)
		wantCount int
	}{
		{
			name: "returns entries from registry",
			setup: func(t *testing.T) {
				t.Helper()
				_ = registry.Save(&registry.PackageManifest{
					Name:    "list-test-pkg",
					Source:  "github.com/org/test",
					Version: "v1.0.0",
					SHA:     "abc1234567890",
					Files: []registry.InstalledFile{
						{Path: ".claude/skills/x/SKILL.md", Target: "claude-code"},
					},
				})
				t.Cleanup(func() { _ = registry.Remove("list-test-pkg") })
			},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)

			entries, err := list.Run()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			found := false
			for _, e := range entries {
				if e.Name == "list-test-pkg" {
					found = true
					if e.Source != "github.com/org/test" {
						t.Errorf("Source = %q", e.Source)
					}
				}
			}

			if tt.wantCount > 0 && !found {
				t.Error("list-test-pkg not found")
			}
		})
	}
}
