package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// AIDiff explains git diff
func AIDiff() (string, error) {
	// Get git diff
	cmd := exec.Command("git", "diff", "--cached")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to get git diff: %v", err)
	}

	diff := out.String()
	if diff == "" {
		return "No changes to explain", nil
	}

	// Ask AI to explain the diff
	prompt := fmt.Sprintf(`Explain these code changes in a clear and concise way. Focus on what was added, removed, or modified:

%s`, diff)

	return askAI(prompt)
}

// AICommit generates a commit message from git diff
func AICommit() (string, error) {
	// Check if we're in a git repository
	if _, err := exec.Command("git", "rev-parse", "--is-inside-work-tree").Output(); err != nil {
		return "", fmt.Errorf("not a git repository")
	}

	// Get staged files
	statusCmd := exec.Command("git", "diff", "--cached", "--name-status")
	statusOut, err := statusCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get git status: %v", err)
	}

	if len(statusOut) == 0 {
		return "", fmt.Errorf("no staged changes to commit")
	}

	// Get git diff
	diffCmd := exec.Command("git", "diff", "--cached", "--unified=3")
	var diffOut bytes.Buffer
	diffCmd.Stdout = &diffOut

	if err := diffCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to get git diff: %v", err)
	}

	diff := diffOut.String()
	if diff == "" {
		return "", fmt.Errorf("no changes to commit")
	}

	// Get the current branch name
	branchCmd := exec.Command("git", "branch", "--show-current")
	branchOut, err := branchCmd.Output()
	branchName := ""
	if err == nil {
		branchName = strings.TrimSpace(string(branchOut))
	}

	// Prepare the prompt with more context
	prompt := fmt.Sprintf(`# Git Commit Message Generation

## Repository Context
- Branch: %s
- Staged Changes:
%s

## Changes
%s

## Task
Generate a conventional commit message following these rules:
1. Start with a type: build, chore, ci, docs, feat, fix, perf, refactor, revert, style, test
2. Optionally add a scope in parentheses after the type (e.g., feat(ui): )
3. Use the imperative mood ("add" not "added" or "adds")
4. Keep the subject line under 72 characters
5. Separate subject from body with a blank line
6. Wrap the body at 72 characters
7. Use the body to explain what and why, not how

## Output Format
Provide only the commit message, no additional text or code blocks.`, 
		branchName, 
		strings.TrimSpace(string(statusOut)),
		diff)

	// Get AI response
	response, err := askAI(prompt)
	if err != nil {
		return "", fmt.Errorf("failed to generate commit message: %v", err)
	}

	// Clean up the response
	message := cleanCommitMessage(response)

	// Validate the message format
	if !isValidCommitMessage(message) {
		return "", fmt.Errorf("generated commit message doesn't follow conventional commit format")
	}

	return message, nil
}

// cleanCommitMessage removes markdown code blocks and trims whitespace
func cleanCommitMessage(message string) string {
	// Remove markdown code blocks
	message = strings.TrimSpace(message)
	message = strings.TrimPrefix(message, "```")
	message = strings.TrimSuffix(message, "```")
	message = strings.TrimSpace(message)

	// Remove any line that's just a code block marker
	lines := strings.Split(message, "\n")
	var cleanLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "```" && trimmed != "" {
			cleanLines = append(cleanLines, line)
		}
	}

	return strings.Join(cleanLines, "\n")
}

// isValidCommitMessage checks if the message follows conventional commit format
func isValidCommitMessage(message string) bool {
	if message == "" {
		return false
	}

	// Split into header and body
	parts := strings.SplitN(message, "\n", 2)
	header := parts[0]

	// Check header format: type(scope): description
	re := regexp.MustCompile(`^(build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test)(\([a-z0-9\-]+\))?: .+`)
	if !re.MatchString(header) {
		return false
	}

	// Check header length (max 72 chars)
	if len(header) > 72 {
		return false
	}

	return true
}
