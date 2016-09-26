package util

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// NewGitExec creates a new gitExec object
func NewGitExec(repositoryPath, remoteAddr string) *GitExec {
	return &GitExec{
		repositoryPath: repositoryPath,
		remoteAddr:     remoteAddr,
	}
}

// GitExec interact with the 'git' command to retrieve and add info
type GitExec struct {
	repositoryPath string
	remoteAddr     string
}

// GetRepositoryPath returns the repositoryPath
func (g *GitExec) GetRepositoryPath() string {
	return g.repositoryPath
}

// AddRemote adds a new git remote
func (g *GitExec) AddRemote(namespace, deployName string) error {
	cmd := exec.Command("git", "remote", "rm", GitRemoteName)
	cmd.Dir = g.repositoryPath
	cmd.Run()

	addr := filepath.Join(g.remoteAddr, deployName)
	cmd = exec.Command("git", "remote", "add", GitRemoteName, addr)
	cmd.Dir = g.repositoryPath
	return cmd.Run()
}

// TopLevelRepository sets the RepositoryPath with the current directory if it's empty
// and also returns the top level basename of a git repository
func (g *GitExec) TopLevelRepository() (repository string, err error) {
	if g.repositoryPath == "" {
		g.repositoryPath, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("could not get the current dir (%s)", err)
		}
	}
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = g.repositoryPath
	output, err := cmd.Output()
	if err != nil {
		return "", errors.New("missing 'git' command or this isn't a git repository")
	}
	splittedPath := strings.Split(string(output[:len(output)-1]), "/") // TODO: Windows implementation
	return splittedPath[len(splittedPath)-1], nil
}
