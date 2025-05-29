// NeuroCLI - AI-powered command line assistant to automate dev workflows and shell commands
// Created: May 2025
// Repository: github.com/Ravsalt/neurocli
// Version: 1.0
// Author: Raven <github.com/Ravsalt>

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	apiURL = "https://text.pollinations.ai/openai"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

// Global configuration variables
var (
	// cfgFile stores the path to the configuration file
	cfgFile string

	// rootCmd represents the base command when called without any subcommands
	rootCmd = &cobra.Command{
		Use:   "neurocli",
		Short: "AI-powered command line assistant for developers",
		Long: `NeuroCLI is a versatile command-line tool that brings AI capabilities to your terminal.
It helps with various development tasks including code generation, git operations, and more.

Features:
  • Natural language interaction with AI
  • Code generation in multiple languages
  • Git integration (diffs, commits)
  • Interactive shell with history
  • Customizable prompts and settings

Examples:
  # Ask a question or get help
  neurocli "how do I sort a map in Go?"
  neurocli help

  # Generate code
  neurocli generate -l python -o script.py "function that reverses a string"
  neurocli generate -l go -o main.go "HTTP server with graceful shutdown"

  # Git operations
  git add .
  neurocli ai-diff    # Explain staged changes
  neurocli aicommit   # Generate commit message

  # Interactive mode
  neurocli shell      # Start interactive shell
  !ls -la             # Execute shell commands in interactive mode`,
		Version: "1.0.0",
	}
)

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.neurocli.yaml)")

	// Add commands
	rootCmd.AddCommand(newAskCmd())
	rootCmd.AddCommand(newGenerateCmd())
	rootCmd.AddCommand(newShellCmd())
	rootCmd.AddCommand(newAIDiffCmd())
	rootCmd.AddCommand(newAICommitCmd())

	// Set default command to handle natural language
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			pterm.Info.Println("Welcome to NeuroCLI! Here are some ways to use it:")
			fmt.Println()
			return cmd.Help()
		}

		// Check for execution command
		if strings.HasPrefix(args[0], "!") {
			return executeCommand(strings.TrimSpace(strings.TrimPrefix(args[0], "!")))
		}

		// Handle natural language query
		response, err := askAI(strings.Join(args, " "))
		if err != nil {
			return err
		}

		// Check if the response is a command to execute
		if strings.HasPrefix(response, "Command: ") {
			cmdStr := strings.TrimSpace(strings.TrimPrefix(response, "Command: "))
			pterm.Info.Println("Executing command:", cmdStr)
			return executeCommand(cmdStr)
		}

		// Otherwise, just print the response
		fmt.Println(response)
		return nil
	}
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			pterm.Error.Println("Error getting home directory:", err)
			return
		}

		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".neurocli")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		pterm.Info.Println("Using config file:", viper.ConfigFileUsed())
	}
}

func newAskCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ask [prompt]",
		Short: "Ask a question to the AI",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			prompt := strings.Join(args, " ")
			response, err := askAI(prompt)
			if err != nil {
				pterm.Error.Println("Error:", err)
				return
			}
			pterm.Info.Println("AI Response:")
			fmt.Println(response)
		},
	}
}

// genPrompt is the template for code generation requests
const genPrompt = `Generate clean, efficient, and well-documented code based on the following description:

"%s"

Requirements:
- Write in %s
- Include necessary imports and dependencies
- Add appropriate error handling
- Use clear and descriptive variable/function names
- Include basic documentation (docstrings/comments)
- Follow language-specific best practices
- Keep it simple and focused

Return only the code without any explanations.`

type genOptions struct {
	output   string
	language string
}

func newGenerateCmd() *cobra.Command {
	var opts genOptions

	cmd := &cobra.Command{
		Use:   "gen [description]",
		Short: "Generate clean, production-ready code",
		Long: `Quickly generate code snippets or entire files based on your description.
Supports multiple programming languages with sensible defaults.`,
		Example: `  # Generate a Python function
  neurocli gen -o script.py "function that reverses a string"
  
  # Generate Go code
  neurocli gen -l go -o main.go "simple HTTP server"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			description := strings.Join(args, " ")

			// Validate language
			opts.language = strings.ToLower(opts.language)
			switch opts.language {
			case "", "python", "py":
				opts.language = "Python"
			case "go", "golang":
				opts.language = "Go"
			case "js", "javascript":
				opts.language = "JavaScript"
			default:
				opts.language = strings.Title(opts.language)
			}

			pterm.Info.Printf("Generating %s code...\n", pterm.Cyan(opts.language))

			// Generate the prompt
			prompt := fmt.Sprintf(genPrompt, description, opts.language)

			// Get code from AI
			code, err := askAI(prompt)
			if err != nil {
				return fmt.Errorf("failed to generate code: %w", err)
			}

			// Clean up the response
			code = cleanCodeResponse(code)

			// Handle output
			if opts.output == "" {
				fmt.Println(code)
				return nil
			}

			// Ensure directory exists
			if err := os.MkdirAll(filepath.Dir(opts.output), 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

			// Write to file
			if err := os.WriteFile(opts.output, []byte(code), 0644); err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}

			pterm.Success.Printf("✓ Successfully generated %s code in %s\n",
				pterm.Cyan(opts.language),
				pterm.Green(opts.output))
			return nil
		},
	}

	// Flags
	cmd.Flags().StringVarP(&opts.output, "output", "o", "", "Output file (default: print to console)")
	cmd.Flags().StringVarP(&opts.language, "language", "l", "python", "Programming language (python, go, js, etc.)")

	// Register completions
	cmd.RegisterFlagCompletionFunc("language", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"python", "go", "javascript", "typescript", "rust", "ruby"}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

// cleanCodeResponse removes markdown code blocks and trims whitespace
func cleanCodeResponse(code string) string {
	code = strings.TrimSpace(code)

	// Remove markdown code blocks if present
	if strings.HasPrefix(code, "```") {
		lines := strings.Split(code, "\n")
		if len(lines) > 2 {
			code = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	return code
}

func askAI(prompt string) (string, error) {
	messages := []Message{
		{
			Role:    "system",
			Content: "You are NeuroCLI, an AI assistant specialized in command-line tools and code generation. Provide clear, concise, and technically accurate responses. Format code blocks with proper syntax highlighting and include only necessary explanations.",
		},
		{
			Role:    "user",
			Content: prompt,
		},
	}

	reqData := ChatRequest{
		Model:       "openai",
		Messages:    messages,
		Temperature: 0.7,
		MaxTokens:   2000,
	}

	reqBody, err := json.Marshal(reqData)
	if err != nil {
		return "", fmt.Errorf("error marshaling request: %v", err)
	}

	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("error decoding response: %v", err)
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "", fmt.Errorf("invalid response format")
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid choice format")
	}

	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid message format")
	}

	content, ok := message["content"].(string)
	if !ok {
		return "", fmt.Errorf("invalid content format")
	}

	return content, nil
}

func executeCommand(cmdStr string) error {
	var cmd *exec.Cmd

	// Use the appropriate shell based on the OS
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", cmdStr)
	} else {
		cmd = exec.Command("sh", "-c", cmdStr)
	}

	// Connect to standard streams
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the command
	return cmd.Run()
}

func newAIDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ai-diff",
		Short: "Explain git diff changes using AI",
		Run: func(cmd *cobra.Command, args []string) {
			explanation, err := AIDiff()
			if err != nil {
				pterm.Error.Println("Error:", err)
				return
			}
			pterm.Info.Println("AI Explanation of Changes:")
			fmt.Println(explanation)
		},
	}
}

func newAICommitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "aicommit",
		Short: "Generate a commit message from staged changes",
		Run: func(cmd *cobra.Command, args []string) {
			message, err := AICommit()
			if err != nil {
				pterm.Error.Println("Error:", err)
				return
			}
			pterm.Info.Println("Suggested commit message:")
			fmt.Println(message)
		},
	}
}

func newShellCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell",
		Short: "Start an interactive shell with enhanced features",
		Long: `Start an interactive shell with features like:
  - Command history
  - Tab completion
  - Syntax highlighting
  - Built-in commands (type 'help')
  - AI integration
  - Shell command execution with '!'
`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := handleShell(); err != nil {
				pterm.Error.Println("Shell error:", err)
			}
		},
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		pterm.Error.Println(err)
		os.Exit(1)
	}
}
