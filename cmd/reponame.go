package cmd

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

type FilePath string

// RepoName is the full name of a repository in the format "provider.tld/path"
type RepoName struct {
	Host string
	Path string
}

func (r RepoName) String() string {
	return path.Join(r.Host, r.Path)
}

func (r RepoName) LocalPathAbsolute() string {
	return filepath.Join(getWorkspaceDir(), r.Host, r.Path)
}

func (r RepoName) GitUrl() string {
	return fmt.Sprintf("https://%s.git", r.String())
}

// ParseRepoName parses a raw repository name into a RepoName struct
// the rawName can be in the format of:
//   - https://provider-tld/path.git
//   - https://provider-tld/path
//   - provider-tld/path
var cloneUrlRe = regexp.MustCompile(`https?://(?P<host>[^/]+\.[^/]+)/(?P<path>.+)\.git`)
var webUrlRe = regexp.MustCompile(`https?://(?P<host>[^/]+\.[^/]+)/(?P<path>.+)`)
var nakedUrlRe = regexp.MustCompile(`(?P<host>[^/]+\.[^/]+)/(?P<path>.+)`)

func ParseRepoName(rawName string) (RepoName, error) {
	matches := cloneUrlRe.FindStringSubmatch(rawName)
	if len(matches) == 3 {
		return RepoName{
			Host: matches[1],
			Path: strings.Trim(path.Clean(matches[2]), "/"),
		}, nil
	}

	matches = webUrlRe.FindStringSubmatch(rawName)
	if len(matches) == 3 {
		return RepoName{
			Host: matches[1],
			Path: strings.Trim(path.Clean(matches[2]), "/"),
		}, nil
	}

	matches = nakedUrlRe.FindStringSubmatch(rawName)
	if len(matches) == 3 {
		return RepoName{
			Host: matches[1],
			Path: strings.Trim(path.Clean(matches[2]), "/"),
		}, nil
	}

	return RepoName{}, fmt.Errorf("invalid repository name: %s", rawName)
}

func MustParseRepoName(rawName string) RepoName {
	r, err := ParseRepoName(rawName)
	if err != nil {
		panic(err)
	}
	return r
}
