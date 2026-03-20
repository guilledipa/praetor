package gitops

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// Syncer handles periodic synchronization of a GitOps configuration repository
type Syncer struct {
	RepoURL    string
	Branch     string
	LocalPath  string
	Interval   time.Duration
	Logger     *slog.Logger
	cancelFunc context.CancelFunc
}

// NewSyncer creates a new GitOps syncer configuration
func NewSyncer(repoURL, branch, localPath string, interval time.Duration, logger *slog.Logger) *Syncer {
	if branch == "" {
		branch = "main"
	}
	return &Syncer{
		RepoURL:   repoURL,
		Branch:    branch,
		LocalPath: localPath,
		Interval:  interval,
		Logger:    logger,
	}
}

// Init performs the initial clone or pull to ensure local data is hydrated synchronously
func (s *Syncer) Init() error {
	s.Logger.Info("Initializing GitOps sync", "url", s.RepoURL, "branch", s.Branch, "path", s.LocalPath)

	err := os.MkdirAll(s.LocalPath, 0755)
	if err != nil {
		return fmt.Errorf("failed to create local path: %w", err)
	}

	repo, err := git.PlainOpen(s.LocalPath)
	if err != nil {
		if err == git.ErrRepositoryNotExists {
			s.Logger.Info("GitOps doing initial clone...")
			repo, err = git.PlainClone(s.LocalPath, false, &git.CloneOptions{
				URL:           s.RepoURL,
				ReferenceName: plumbing.NewBranchReferenceName(s.Branch),
				SingleBranch:  true,
				Progress:      os.Stdout,
			})
			if err != nil {
				return fmt.Errorf("initial clone failed: %w", err)
			}
			s.Logger.Info("Initial GitOps clone completed successfully")
			return nil
		}
		// Some other open error
		return fmt.Errorf("failed to open existing repo: %w", err)
	}

	s.Logger.Info("Repository already exists locally, pulling latest updates...")
	err = s.pullLatest(repo)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		s.Logger.Warn("Failed to pull latest during init, will continue with existing cache", "error", err)
	} else if err == git.NoErrAlreadyUpToDate {
		s.Logger.Info("GitOps repository is already up to date")
	} else {
		s.Logger.Info("GitOps repository pulled latest commits")
	}

	return nil
}

// Start begins the background sync loop
func (s *Syncer) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	s.cancelFunc = cancel

	ticker := time.NewTicker(s.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.Logger.Info("GitOps sync loop gracefully stopped")
			return
		case <-ticker.C:
			repo, err := git.PlainOpen(s.LocalPath)
			if err != nil {
				s.Logger.Error("GitOps sync loop failed to open local repository", "error", err)
				continue
			}

			err = s.pullLatest(repo)
			if err != nil && err != git.NoErrAlreadyUpToDate {
				s.Logger.Error("GitOps sync loop pull failed", "error", err)
			} else if err == nil {
				s.Logger.Info("GitOps catalog automatically updated from repository!")
			}
		}
	}
}

// Stop halts the background sync loop
func (s *Syncer) Stop() {
	if s.cancelFunc != nil {
		s.cancelFunc()
	}
}

func (s *Syncer) pullLatest(repo *git.Repository) error {
	worktree, err := repo.Worktree()
	if err != nil {
		return err
	}

	return worktree.Pull(&git.PullOptions{
		ReferenceName: plumbing.NewBranchReferenceName(s.Branch),
		SingleBranch:  true,
		Force:         true,
	})
}
