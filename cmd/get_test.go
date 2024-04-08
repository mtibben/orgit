package cmd

import (
	"os"
	"testing"
)

func TestGetArgs(t *testing.T) {
	os.Setenv("GITORG_WORKSPACE", "/home/user/gitorg")

	tableTests := []struct {
		args                   string
		expectedGitUrl         string
		expectedCommitOrBranch string
		expectedDir            string
		expectedError          error
	}{
		{"github.com/user/project@commit", "https://github.com/user/project.git", "commit", "/home/user/gitorg/github.com/user/project", nil},
		{"github.com/user/project", "https://github.com/user/project.git", "", "/home/user/gitorg/github.com/user/project", nil},
		{"github.com/org/group/project", "https://github.com/org/group/project.git", "", "/home/user/gitorg/github.com/org/group/project", nil},
		{"github.com/org/group/project/", "https://github.com/org/group/project.git", "", "/home/user/gitorg/github.com/org/group/project", nil},
	}

	for i, tt := range tableTests {
		osUserHomeDir = func() (string, error) { return "/home/user", nil }

		projectUrl, commitOrBranch, err := parseArgsForGetCmd(tt.args)
		if err != nil {
			t.Errorf("Test %d: Expected err to be nil, got %s", i, err.Error())
		}

		dir := getLocalDir(projectUrl)

		if projectUrl.String() != tt.expectedGitUrl {
			t.Errorf("Test %d: Expected projectUrl to be %s, got %s", i, tt.expectedGitUrl, projectUrl.String())
		}
		if commitOrBranch != tt.expectedCommitOrBranch {
			t.Errorf("Test %d: Expected commitOrBranch to be %s, got %s", i, tt.expectedCommitOrBranch, commitOrBranch)
		}
		if dir != tt.expectedDir {
			t.Errorf("Test %d: Expected dir to be %s, got %s", i, tt.expectedDir, dir)
		}

	}
}
