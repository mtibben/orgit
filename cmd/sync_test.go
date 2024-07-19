package cmd

import "testing"

func TestHasAlreadyProcessedRepo(t *testing.T) {
	tidyAction := TidyAction{
		remoteRepos: []string{
			"gitlab.com/example/path1/path2",
		},
	}

	tests := []struct {
		filename string
		expected bool
	}{
		{"gitlab.com/example/path1/path2/path3", true},
		{"gitlab.com/example/path1/path2other", false},
		{"gitlab.com/example/path1/path2.git", false},
		{"gitlab.com/example/path1/path2", true},
		{"gitlab.com/example/path1.git", false},
		{"gitlab.com/example/path1", false},
		{"github.com/example", false},
		{"github.com", false},
	}

	for _, test := range tests {
		result := tidyAction.HasAlreadyProcessedRepo(test.filename)
		if result != test.expected {
			t.Errorf("HasAlreadyProcessedRepo(%s) returned %v, expected %v", test.filename, result, test.expected)
		}
	}
}
