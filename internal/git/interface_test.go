package git_test

import (
	"github.com/colbymchenry/devpit/internal/beads"
	"github.com/colbymchenry/devpit/internal/git"
)

// Compile-time assertion: Git must satisfy BranchChecker.
var _ beads.BranchChecker = (*git.Git)(nil)
