package cmd

import (
	"testing"
)

func TestParseRepoName(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    RepoName
		expectError bool
	}{
		{
			name:        "Valid URL with https and .git",
			input:       "https://example.com/path/to/repo.git",
			expected:    RepoName{Host: "example.com", Path: "path/to/repo"},
			expectError: false,
		},
		{
			name:        "Valid URL with https without .git",
			input:       "https://example.com/path/to/repo",
			expected:    RepoName{Host: "example.com", Path: "path/to/repo"},
			expectError: false,
		},
		{
			name:        "Valid URL without https and .git",
			input:       "example.com/path/to/repo",
			expected:    RepoName{Host: "example.com", Path: "path/to/repo"},
			expectError: false,
		},
		{
			name:        "Valid URL with trailing slash",
			input:       "example.com/path/to/repo/",
			expected:    RepoName{Host: "example.com", Path: "path/to/repo"},
			expectError: false,
		},
		{
			name:        "Valid URL without https and with .git",
			input:       "example.com/path/to/repo.git",
			expected:    RepoName{Host: "example.com", Path: "path/to/repo.git"},
			expectError: false,
		},
		{
			name:        "Valid URL with extra slashes",
			input:       "example.com//path//to///repo",
			expected:    RepoName{Host: "example.com", Path: "path/to/repo"},
			expectError: false,
		},
		{
			name:        "Invalid URL",
			input:       "not a url",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseRepoName(tt.input)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected an error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Did not expect an error but got one: %v", err)
				}
				if result.Host != tt.expected.Host || result.Path != tt.expected.Path {
					t.Errorf("Expected %v, got host %v path %v", tt.expected, result.Host, result.Path)
				}
			}
		})
	}
}
