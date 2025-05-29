package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/peterh/liner"
	"github.com/pterm/pterm"
)

// _---~~(~~-_.
//     _{        )   )
//   ,   ) -~~- ( ,-' )_
//  (  `-,_..`., )-- '_,)
// ( ` _)  (  -~( -_ `,  }
// (_-  _  ~_-~~~~`,  ,' )
//   `~ -^(    __;-,((()))
//         ~~~~ {_ -_(())
//                `\  }
//                  { }      Neurocli

type ShellCommand struct {
	Name        string
	Description string
	Handler     func([]string) error
}

var (
	shellCommands []ShellCommand
	historyFile   string
)

func init() {
	home, _ := os.UserHomeDir()
	historyFile = filepath.Join(home, ".neurocli_history")

	shellCommands = []ShellCommand{
		{
			Name:        "help",
			Description: "Show this help message",
			Handler:     handleHelp,
		},
		{
			Name:        "exit",
			Description: "Exit the shell",
			Handler:     handleExit,
		},
		{
			Name:        "clear",
			Description: "Clear the screen",
			Handler:     handleClear,
		},
		{
			Name:        "cd",
			Description: "Change directory",
			Handler:     handleChangeDir,
		},
	}
}

// newShell creates a new liner instance with configuration
func newShell() *liner.State {
	line := liner.NewLiner()
	line.SetTabCompletionStyle(liner.TabCircular)
	line.SetCtrlCAborts(true)

	// Set up command completion
	var commands []string
	for _, cmd := range shellCommands {
		commands = append(commands, cmd.Name)
	}

	line.SetCompleter(func(line string) (c []string) {
		for _, cmd := range commands {
			if strings.HasPrefix(cmd, strings.ToLower(line)) {
				c = append(c, cmd)
			}
		}
		return
	})

	// Load history
	if f, err := os.Open(historyFile); err == nil {
		line.ReadHistory(f)
		f.Close()
	}

	return line
}

// saveHistory saves the command history to a file
func saveHistory(line *liner.State) {
	if f, err := os.Create(historyFile); err == nil {
		line.WriteHistory(f)
		f.Close()
	}
}

// getPrompt returns a simple, reliable prompt string
func getPrompt() string {
	return "> "
}

func handleShell() error {
	line := newShell()
	defer line.Close()

	// Save history on exit
	defer saveHistory(line)

	// Save limited history on exit
	defer func() {
		if f, err := os.Create(historyFile); err == nil {
			defer f.Close()
			// WriteHistory will write the current history to the writer
			line.WriteHistory(f)
		}
	}()

	fmt.Println("NeuroCLI Shell - Type 'help' for commands, 'exit' to quit")

	for {
		input, err := line.Prompt(getPrompt())
		if err != nil {
			if err == liner.ErrPromptAborted {
				fmt.Println("^C")
				continue
			}
			return err
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		line.AppendHistory(input)

		parts := strings.Fields(input)
		if len(parts) == 0 {
			continue
		}

		cmd := strings.ToLower(parts[0])
		args := parts[1:]

		// Handle built-in commands
		if handleBuiltInCommand(cmd, args) {
			continue
		}

		// Handle shell commands (prefixed with '!')
		if handleShellCommand(input) {
			continue
		}

		// Handle as AI query
		response, err := askAI(input)
		if err != nil {
			pterm.Error.Println("Error:", err)
			continue
		}

		// If AI response is a command to execute
		if strings.HasPrefix(response, "Command: ") {
			cmdStr := strings.TrimSpace(strings.TrimPrefix(response, "Command: "))
			if !isValidCommand(cmdStr) {
				pterm.Error.Println("Invalid or potentially unsafe command.")
				continue
			}
			pterm.Info.Println("Executing command:", cmdStr)
			if err := executeCommand(cmdStr); err != nil {
				pterm.Error.Println("Command failed:", err)
			}
			continue
		}

		// Print AI response with code block formatting if present
		if strings.Contains(response, "```") {
			parts := strings.Split(response, "```")
			for i, part := range parts {
				if i%2 == 1 { // Code block
					fmt.Println("\n--- CODE ---")
					fmt.Println(part)
					fmt.Println("------------")
					fmt.Println()
				} else {
					fmt.Print(part)
				}
			}
		} else {
			fmt.Println(response)
		}
	}
}

// handleBuiltInCommand encapsulates handling of built-in shell commands.
func handleBuiltInCommand(cmd string, args []string) bool {
	for _, shellCmd := range shellCommands {
		if shellCmd.Name == cmd {
			if err := shellCmd.Handler(args); err != nil {
				pterm.Error.Println(err)
			}
			return true
		}
	}
	return false
}

// handleShellCommand encapsulates handling of shell commands (prefixed with '!').
func handleShellCommand(input string) bool {
	if strings.HasPrefix(input, "!") {
		cmdStr := strings.TrimSpace(input[1:])
		if !isValidCommand(cmdStr) {
			pterm.Error.Println("Invalid or potentially unsafe command.")
			return true
		}
		if err := executeCommand(cmdStr); err != nil {
			pterm.Error.Println("Command failed:", err)
		}
		return true
	}
	return false
}

// isValidCommand checks if a command is safe to execute
func isValidCommand(cmd string) bool {
	// Define a list of allowed commands
	allowedCommands := []string{
		"ls", "pwd", "echo", "cat", "grep", "find", "ps",
		"top", "df", "du", "date", "whoami", "uname",
	}

	// Split the command into parts
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return false
	}

	// Check if the command is in the allowed list
	for _, allowed := range allowedCommands {
		if parts[0] == allowed {
			return true
		}
	}

	return false
}

// Command handlers
func handleHelp(args []string) error {
	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("63"))).
		Headers("COMMAND", "DESCRIPTION")

	for _, cmd := range shellCommands {
		t.Row(cmd.Name, cmd.Description)
	}

	// Add AI commands
	t.Row("!command", "Execute a shell command")
	t.Row("query", "Ask a question to the AI")

	fmt.Println(t.Render())
	return nil
}

func handleExit(args []string) error {
	pterm.Info.Println("Goodbye!")
	os.Exit(0)
	return nil
}

func handleClear(args []string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "cls")
	} else {
		cmd = exec.Command("clear")
	}
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

func handleChangeDir(args []string) error {
	if len(args) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		return os.Chdir(home)
	}
	return os.Chdir(args[0])
}
