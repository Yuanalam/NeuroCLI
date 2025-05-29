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

	// Prepare the prompt with more context and stricter instructions
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
4. Keep the subject line under 50 characters
5. Separate subject from body with a blank line
6. Wrap the body at 72 characters
7. Use the body to explain what and why, not how
8. IMPORTANT: The first line MUST match this exact format: ^(build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test)(\([a-z0-9\-]+\))?: [A-Z][^\n]{0,48}[^\s]

## Examples
fix: correct minor typos in code
feat(api): add user authentication endpoint
refactor(server): improve database connection handling

## Output Format
Provide ONLY the commit message, no additional text, explanations, or code blocks.`, 
		branchName, 
		strings.TrimSpace(string(statusOut)),
		diff)

	// Try up to 3 times to get a valid commit message
	maxAttempts := 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Get AI response
		response, err := askAI(prompt)
		if err != nil {
			return "", fmt.Errorf("failed to generate commit message: %v", err)
		}

		// Clean up the response
		message := cleanCommitMessage(response)

		// Validate the message format
		if isValidCommitMessage(message) {
			return message, nil
		}

		// If last attempt, return the error
		if attempt == maxAttempts {
			// Try to fix common issues before giving up
			fixedMessage := fixCommonCommitMessageIssues(message)
			if isValidCommitMessage(fixedMessage) {
				return fixedMessage, nil
			}
			return "", fmt.Errorf("failed to generate valid commit message after %d attempts. Last attempt: %s", maxAttempts, message)
		}

		// Try again with a more specific prompt
		prompt = fmt.Sprintf(`The previous commit message was rejected because it didn't follow the required format. 
Please generate a new commit message that follows these EXACT rules:

1. First line format: "type(scope): subject" (e.g., "feat(auth): add login button")
2. Valid types: build, chore, ci, docs, feat, fix, perf, refactor, revert, style, test
3. Subject must start with a capital letter and be in imperative mood
4. Subject must be 50 characters or less
5. Separate subject from body with a blank line
6. Wrap body at 72 characters

Here are the changes again:

%s`, diff)
	}

	return "", fmt.Errorf("unexpected error in AICommit")
}

// fixCommonCommitMessageIssues tries to fix common formatting issues in commit messages
func fixCommonCommitMessageIssues(message string) string {
	if message == "" {
		return message
	}

	// Split into lines
	lines := strings.Split(message, "\n")
	if len(lines) == 0 {
		return message
	}

	// Fix first line
	header := lines[0]
	
	// Remove any markdown code blocks
	header = strings.TrimSpace(header)
	header = strings.TrimPrefix(header, "`")
	header = strings.TrimSuffix(header, "`")

	// If header is too long, truncate it
	if len(header) > 50 && len(header) > 0 {
		header = header[:47] + "..."
	}

	// Ensure it starts with a valid type
	re := regexp.MustCompile(`^(build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test)(\([a-z0-9\-]+\))?: `)
	if !re.MatchString(header) {
		// Try to find the first colon and see if we can fix the type
		colonIndex := strings.Index(header, ":")
		if colonIndex > 0 {
			typePart := strings.TrimSpace(header[:colonIndex])
			descPart := strings.TrimSpace(header[colonIndex+1:])
			
			// Try to extract a valid type from the beginning
			typeRe := regexp.MustCompile(`^(build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test)`)
			if matches := typeRe.FindStringSubmatch(typePart); len(matches) > 0 {
				header = matches[0] + ": " + descPart
			} else {
				header = "fix: " + strings.TrimLeft(header, ": ")
			}
		} else {
			header = "fix: " + header
		}
	}

	// Capitalize first letter after type
	header = strings.TrimSpace(header)
	if len(header) > 0 {
		// Find the first letter after the type and scope
		afterType := ""
		if colonIndex := strings.Index(header, ":"); colonIndex > 0 && len(header) > colonIndex+1 {
			afterType = header[colonIndex+1:]
			header = header[:colonIndex+1] + " " + strings.ToUpper(string(afterType[0]))
			if len(afterType) > 1 {
				header += afterType[1:]
			}
		}
	}

	// Rebuild the message
	if len(lines) > 1 {
		return header + "\n" + strings.Join(lines[1:], "\n")
	}
	return header
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
	header := strings.TrimSpace(parts[0])

	// Check header format: type(scope): description
	headerRe := regexp.MustCompile(`^(build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test)(\([a-z0-9\-]+\))?: .+`)
	if !headerRe.MatchString(header) {
		return false
	}

	// Check header length (max 72 chars)
	if len(header) > 72 {
		return false
	}

	// Find the description start after the type/scope
	descStart := strings.Index(header, ": ") + 2
	if descStart < 2 || len(header) <= descStart {
		return false
	}

	// Capitalize the first letter of the description
	desc := header[descStart:]
	if len(desc) > 0 {
		desc = strings.ToUpper(string(desc[0])) + desc[1:]
		header = header[:descStart] + desc
		parts[0] = header // Update the header with capitalized description
	}

	// Check body line lengths if present
	if len(parts) > 1 {
		body := parts[1]
		// Ensure there's a blank line between header and body
		if !strings.HasPrefix(body, "\n") && !strings.HasPrefix(body, "\r\n") {
			// Add a blank line if missing
			parts[1] = "\n" + strings.TrimSpace(body)
		}

		// Check body line lengths (max 72 chars) and trim if needed
		var bodyLines []string
		for _, line := range strings.Split(parts[1], "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				if len(line) > 72 {
					line = line[:69] + "..."
				}
				bodyLines = append(bodyLines, line)
			}
		}
		parts[1] = strings.Join(bodyLines, "\n")
	}

	return true
}
