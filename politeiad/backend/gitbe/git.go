package gitbe

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// gitError contains all the components of a git invocation.
type gitError struct {
	cmd    []string
	env    []string
	err    error
	stdout []string
	stderr []string
}

// Error satisfies the error interface.
func (e gitError) Error() string {
	return e.err.Error()
}

// log pretty prints a gitError.
func (e gitError) log() {
	var cmd string
	for _, v := range e.cmd {
		cmd += v + " "
	}
	log.Infof("Git command: %v", cmd)
	log.Infof("Git result : %v", e.err)
	s := "Git stdout :"
	for _, v := range e.stdout {
		log.Infof("%v %v", s, v)
		s = ""
	}
	s = "Git stderr :"
	for _, v := range e.stderr {
		log.Infof("%v %v", s, v)
		s = ""
	}
}

// git excutes the git command using the provided arguments.  If the path
// argument is set it'll be copied to the GIT_DIR environment variable.
func (g *gitBackEnd) git(path string, args ...string) ([]string, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("git requires arguments")
	}

	// Setup gitError
	ge := gitError{
		cmd:    make([]string, 0, len(args)+1),
		stdout: make([]string, 0, 128),
		stderr: make([]string, 0, 128),
	}

	ge.cmd = append(ge.cmd, g.gitPath)
	for _, v := range args {
		ge.cmd = append(ge.cmd, v)
	}

	if g.gitTrace {
		defer func() { ge.log() }()
	}

	cmd := exec.Command(g.gitPath, args...)

	// Determine if we need to set GIT_DIR
	if path != "" {
		cmd.Dir = path
	}

	// Make sure pipes are handled before we exit
	var wg sync.WaitGroup

	// Setup stdout
	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		ge.err = fmt.Errorf("stdout pipe: %v", err)
		return nil, ge
	}
	var stdoutError error
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(cmdReader)
		for scanner.Scan() {
			ge.stdout = append(ge.stdout, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			stdoutError = err
		}
	}()

	// Setup stderr
	cmdError, err := cmd.StderrPipe()
	if err != nil {
		ge.err = fmt.Errorf("stderr pipe: %v", err)
		return nil, ge
	}
	var stderrError error
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(cmdError)
		for scanner.Scan() {
			ge.stderr = append(ge.stderr, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			stderrError = err
		}
	}()

	// Actually launch git
	err = cmd.Start()
	if err != nil {
		ge.err = fmt.Errorf("cmd.Start: %v", err)
		return nil, ge
	}

	// Wait for pipes to finish reading.
	//
	// From the Go docs (https://golang.org/pkg/os/exec/#Cmd.StderrPipe):
	// Wait will close the pipe after seeing the command exit, so most
	// callers need not close the pipe themselves; however, an implication
	// is that it is incorrect to call Wait before all reads from the pipe
	// have completed.
	wg.Wait()

	// Finish up cmd.
	err = cmd.Wait()
	if err != nil {
		ge.err = fmt.Errorf("cmd.Wait: %v", err)
		return nil, ge
	}

	return ge.stdout, nil
}

// gitVersion returns the version of git.
func (g *gitBackEnd) gitVersion() (string, error) {
	out, err := g.git("", "version")
	if err != nil {
		return "", err
	}

	if len(out) != 1 {
		return "", fmt.Errorf("unexpected git output")
	}

	return out[0], nil
}

func (g *gitBackEnd) gitAdd(path, filename string) error {
	_, err := g.git(path, "add", filename)
	if err != nil {
		return err
	}

	return nil
}

func (g *gitBackEnd) gitCommit(path, message string) error {
	_, err := g.git(path, "commit", "-m", message)
	if err != nil {
		return err
	}

	return nil
}

func (g *gitBackEnd) gitCheckout(path, branch string) error {
	_, err := g.git(path, "checkout", branch)
	if err != nil {
		return err
	}

	return nil
}

func (g *gitBackEnd) gitBranchDelete(path, branch string) error {
	_, err := g.git(path, "branch", "-D", branch)
	if err != nil {
		return err
	}

	return nil
}

func (g *gitBackEnd) gitPull(path string, fastForward bool) error {
	var err error
	if fastForward {
		_, err = g.git(path, "pull", "--ff-only", "--rebase")
	} else {
		_, err = g.git(path, "pull")
	}
	if err != nil {
		return err
	}

	return nil
}

func (g *gitBackEnd) gitRebase(path, branch string) error {
	_, err := g.git(path, "rebase", branch)
	if err != nil {
		return err
	}

	return nil
}

func (g *gitBackEnd) gitPush(path, remote, branch string, upstream bool) error {
	var err error
	if upstream {
		_, err = g.git(path, "push", "--set-upstream", remote, branch)
	} else {
		_, err = g.git(path, "push")
	}
	if err != nil {
		return err
	}

	return nil
}

func (g *gitBackEnd) gitNewBranch(path, branch string) error {
	_, err := g.git(path, "checkout", "-b", branch)
	if err != nil {
		return err
	}

	return nil
}

func (g *gitBackEnd) gitLastDigest(path string) ([]byte, error) {
	out, err := g.git(path, "log", "--pretty=oneline", "-n 1")
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("invalid git output")
	}

	// Returned data is "<digest> <commit message>"
	ds := strings.SplitN(out[0], " ", 2)
	if len(ds) == 0 {
		return nil, fmt.Errorf("invalid log")
	}

	d, err := hex.DecodeString(ds[0])
	if err != nil {
		return nil, err
	}

	if len(d) != sha1.Size {
		return nil, fmt.Errorf("invalid sha1 size")
	}

	return d, nil
}

func (g *gitBackEnd) gitLog(path string) ([]string, error) {
	out, err := g.git(path, "log")
	if err != nil {
		return nil, err
	}

	return out, nil
}

func (g *gitBackEnd) gitFsck(path string) ([]string, error) {
	out, err := g.git(path, "fsck", "--full", "--strict")
	if err != nil {
		return nil, err
	}

	return out, nil
}

// gitClone clones a git repository.  This functions exits without an error
// if the directory is already a git repo.
func (g *gitBackEnd) gitClone(from, to string) error {
	_, err := os.Stat(filepath.Join(from, ".git"))
	if os.IsNotExist(err) {
		return fmt.Errorf("source repo does not exist")
	}
	_, err = os.Stat(filepath.Join(to, ".git"))
	if !os.IsNotExist(err) {
		return err
	}

	log.Infof("Cloning git repo %v to %v", from, to)
	_, err = g.git("", "clone", from, to)
	if err != nil {
		return err
	}

	return nil
}

// gitInit initializes a new repository.  If the repository exists
// it does not reinit it; it reutns failure instead.
func (g *gitBackEnd) gitInit(path string) (string, error) {
	out, err := g.git("", "init", path)
	if err != nil {
		return "", err
	}

	if len(out) != 1 {
		return "", fmt.Errorf("unexpected git output")
	}

	return out[0], nil
}

// gitInitRepo initializes a directory as a git repo.  This functions exits
// without an error if the directory is already a git repo.  The git repo is
// initialized with a .gitignore file so that a) have a master and b) always
// ignore the lock file.
func (g *gitBackEnd) gitInitRepo(path string) error {
	_, err := os.Stat(filepath.Join(path, ".git"))
	if !os.IsNotExist(err) {
		return err
	}

	// Containing directory
	err = os.MkdirAll(path, 0700)
	if err != nil {
		return err
	}
	log.Infof("Initializing git repo: %v", path)

	// Initialize git repo
	_, err = g.gitInit(path)
	if err != nil {
		return err
	}

	// Add .gitignore with the lock file name.
	// This makes the repo ready to go and we'll always use this as the
	// initial commit.
	err = ioutil.WriteFile(filepath.Join(path, ".gitignore"),
		[]byte(LockFilename+"\n"), 0664)
	if err != nil {
		return err
	}
	err = g.gitAdd(path, ".gitignore")
	if err != nil {
		return err
	}
	err = g.gitCommit(path, "Add .gitignore")
	if err != nil {
		return err
	}

	return nil
}
