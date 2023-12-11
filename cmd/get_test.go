package cmd

import (
	"testing"
)

func TestGetArgs(t *testing.T) {
	tableTests := []struct {
		args                   string
		expectedGitUrl         string
		expectedCommitOrBranch string
		expectedDir            string
		expectedError          error
		mockCwd                string
	}{
		{"github.com/user/project@commit", "ssh://github.com/user/project", "commit", "/path/to/dir", nil, ""},
		{"gitlab.com/user/project@commit", "ssh://gitlab.com/user/project", "commit", "/home/user/Developer/src/gitlab.com/user/project", nil, ""},
		{"project", "https://gitlab.com/vistaprint-org/project.git", "", "/home/user/Developer/src/gitlab.com/vistaprint-org/project", nil, "/home/user/Developer/src/gitlab.com/vistaprint-org"},
		{"project@commit", "https://gitlab.com/vistaprint-org/project.git", "commit", "/home/user/Developer/src/gitlab.com/vistaprint-org/project", nil, ""},
		{"/project@commit", "https://gitlab.com/project.git", "commit", "/home/user/Developer/src/gitlab.com/project", nil, ""},
	}

	for i, tt := range tableTests {
		// if tt.mockCwd != "" {
		// 	osGetwd = func() (string, error) { return tt.mockCwd, nil }
		// } else {
		// 	osGetwd = func() (string, error) { return "/home/user", nil }
		// }
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
