package util

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// NewGitExec creates a new gitExec object
func NewGitExec(repositoryPath, remoteAddr string) (g *GitExec, err error) {
	if repositoryPath == "" {
		repositoryPath, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("could not get the current dir (%s)", err)
		}
	}
	return &GitExec{
		repositoryPath: repositoryPath,
		remoteAddr:     remoteAddr,
	}, nil
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

	gitRemoteAddr := fmt.Sprintf("%s/%s/%s", g.remoteAddr, namespace, deployName)
	cmd = exec.Command("git", "remote", "add", GitRemoteName, gitRemoteAddr)
	cmd.Dir = g.repositoryPath
	return cmd.Run()
}

// GetTopLevelRepository returns the top level basename of a git repository
func (g *GitExec) GetTopLevelRepository() (repository string, err error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = g.repositoryPath
	output, err := cmd.Output()
	if err != nil {
		return "", errors.New("missing 'git' command or this isn't a git repository")
	}
	splittedPath := strings.Split(string(output[:len(output)-1]), "/") // TODO: Windows implementation
	return splittedPath[len(splittedPath)-1], nil
}
