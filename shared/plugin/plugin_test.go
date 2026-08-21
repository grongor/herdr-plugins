package plugin

import (
	"errors"
	"testing"
)

func TestLoadReadsTheWorktree(t *testing.T) {
	t.Setenv(envContextJSON, `{
		"workspace_id": "w1P",
		"workspace_label": "storefront",
		"workspace_cwd": "/p/storefront-topic",
		"worktree": {
			"repo_key": "/p/storefront/.git",
			"repo_name": "storefront",
			"repo_root": "/p/storefront",
			"checkout_path": "/p/storefront-topic",
			"is_linked_worktree": true
		},
		"focused_pane_agent": "claude"
	}`)

	ctx, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if ctx.WorkspaceID != "w1P" {
		t.Errorf("WorkspaceID = %q, want w1P", ctx.WorkspaceID)
	}
	if ctx.Worktree == nil {
		t.Fatal("Worktree is nil")
	}

	want := Worktree{
		RepoKey:          "/p/storefront/.git",
		RepoName:         "storefront",
		RepoRoot:         "/p/storefront",
		CheckoutPath:     "/p/storefront-topic",
		IsLinkedWorktree: true,
	}
	if *ctx.Worktree != want {
		t.Fatalf("Worktree = %+v, want %+v", *ctx.Worktree, want)
	}
}

// TestLoadWithoutAWorktree pins that a workspace with no git behind it yields a nil Worktree, not a zero one.
func TestLoadWithoutAWorktree(t *testing.T) {
	t.Setenv(envContextJSON, `{"workspace_id": "w1", "workspace_label": "notes"}`)

	ctx, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ctx.Worktree != nil {
		t.Fatalf("Worktree = %+v, want nil", ctx.Worktree)
	}
}

func TestLoadOutsideHerdr(t *testing.T) {
	t.Setenv(envContextJSON, "")

	if _, err := Load(); !errors.Is(err, ErrNoContext) {
		t.Fatalf("Load() error = %v, want ErrNoContext", err)
	}
}

func TestLoadRejectsGarbage(t *testing.T) {
	t.Setenv(envContextJSON, "{not json")

	if _, err := Load(); err == nil {
		t.Fatal("Load accepted a context that is not JSON")
	}
}

// TestInPane pins the variable InPane reads; an action misdetected as a pane would hang in Fail with nobody watching.
func TestInPane(t *testing.T) {
	t.Setenv(envEntrypointID, "confirm")
	if !InPane() {
		t.Error("InPane() = false with an entrypoint id set")
	}

	t.Setenv(envEntrypointID, "")
	if InPane() {
		t.Error("InPane() = true with no entrypoint id")
	}
}
