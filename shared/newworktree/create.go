package newworktree

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/grongor/herdr-plugins/shared/git"
	"github.com/grongor/herdr-plugins/shared/herdrcli"
)

// Creator turns a request into a checkout and opens it as a workspace.
type Creator struct {
	// Seed lists paths copied from the main checkout into the new one, relative to the checkout root.
	Seed []string
}

// Create uses git rather than Herdr's `worktree create` because Herdr's call is asynchronous server-side and returns
// before the directory exists, leaving nothing to seed. The "herdr" layout (empty Path) has Herdr choose the path,
// so it goes to Herdr whole and is not seeded; the dialog says so.
func (c Creator) Create(req Request) error {
	if req.Path == "" {
		_, err := herdrcli.CreateWorktree(req.Repo.CommonDir, req.Branch, req.Base, req.Path)

		return err
	}

	if req.Branch == "" {
		return errors.New("a branch name is required to create a checkout at a path")
	}

	dir := req.Repo.Anchor
	if dir == "" {
		dir = req.Repo.CommonDir
	}

	if err := git.AddWorktree(dir, req.Path, req.Branch, req.Base); err != nil {
		return err
	}

	// A seeding failure is reported but does not undo the checkout, which is correct without the seed.
	seedErr := c.seed(req.Repo.Anchor, req.Path)

	if _, err := herdrcli.OpenCheckout(req.Repo.CommonDir, req.Path); err != nil {
		return err
	}

	return seedErr
}

// seed never overwrites: a path already in the new checkout is one git put there.
func (c Creator) seed(from, to string) error {
	if from == "" || len(c.Seed) == 0 {
		return nil
	}

	var failures []error
	for _, rel := range c.Seed {
		if !filepath.IsLocal(rel) {
			failures = append(failures, fmt.Errorf("seed %q is not a path inside the checkout", rel))

			continue
		}

		if err := copyPath(filepath.Join(from, rel), filepath.Join(to, rel)); err != nil {
			failures = append(failures, err)
		}
	}

	if len(failures) == 0 {
		return nil
	}

	return fmt.Errorf("seeding the checkout: %w", errors.Join(failures...))
}

// copyPath treats a missing source as success: one seed list serves every repository.
func copyPath(from, to string) error {
	info, err := os.Lstat(from)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	}

	if _, err := os.Lstat(to); err == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}

	if info.IsDir() {
		return os.CopyFS(to, os.DirFS(from))
	}

	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}

	return os.WriteFile(to, data, info.Mode().Perm())
}
