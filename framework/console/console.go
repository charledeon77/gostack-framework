/*
Purpose:
This file defines the Gost CLI console execution subsystem for the GoStack framework.
It provides the Command interface and the central console Kernel that registers,
dispatches, and executes CLI commands.

Philosophy:
We believe CLI engines in compiled languages should remain simple, modular, and fast.
By avoiding bloated runtime parsing libraries, we keep the framework's build speeds
instantaneous and ensure the console runner remains compile-time verifiable.

Architecture:
This console subsystem serves as the entrypoint for CLI operations. The console Kernel
manages a static map of registered Commands. It acts as the routing multiplexer for
incoming OS terminal arguments.

Choice:
We chose a direct subcommand-to-handler router over feature-heavy external libraries
(like Cobra or Urfave/CLI) to maintain a zero-dependency, minimal binary footprint and
maximize compilation speed.

Implementation:
- Command: An interface that commands must implement to expose their Name, Description, and Execute logic.
- Kernel: The central execution coordinator.
  - NewKernel(): constructor.
  - Register(cmd Command): Registers command structs.
  - Run(args []string): Extracts the subcommand, checks for help commands, and dispatches to the correct handler.
  - printHelp(): Standard help instructions detailing available commands.
*/
package console

import (
	"fmt"
)

// Command defines the structural rules that all CLI command handlers must implement.
type Command interface {
	// Name returns the trigger string (e.g. "migrate", "make:controller").
	Name() string

	// Description returns the help text shown in lists.
	Description() string

	// Execute runs the command with standard string arguments.
	Execute(args []string) error
}

// Kernel coordinates command registration and execution.
type Kernel struct {
	commands map[string]Command
}

// NewKernel builds a fresh console Kernel instance.
func NewKernel() *Kernel {
	return &Kernel{
		commands: make(map[string]Command),
	}
}

// Register adds a Command to the console registry.
func (k *Kernel) Register(cmd Command) {
	if cmd == nil {
		panic("console: cannot register nil command")
	}
	k.commands[cmd.Name()] = cmd
}

// Run executes the matched command based on terminal arguments.
func (k *Kernel) Run(args []string) error {
	if len(args) < 1 {
		k.PrintHelp()
		return nil
	}

	subcommand := args[0]
	if subcommand == "--help" || subcommand == "-h" || subcommand == "help" {
		k.PrintHelp()
		return nil
	}

	if subcommand == "--version" || subcommand == "-v" || subcommand == "version" {
		fmt.Println("GoStack Framework v1.0.0")
		return nil
	}

	cmd, exists := k.commands[subcommand]
	if !exists {
		k.PrintHelp()
		return fmt.Errorf("console: unknown command '%s'", subcommand)
	}

	return cmd.Execute(args[1:])
}

// PrintHelp prints the usage instructions and list of registered commands.
func (k *Kernel) PrintHelp() {
	fmt.Println("GoStack CLI Command Runner")
	fmt.Println("\nUsage:")
	fmt.Println("  go run cmd/gostack/main.go <command> [arguments]")
	fmt.Println("\nAvailable Commands:")

	for _, cmd := range k.commands {
		fmt.Printf("  %-20s %s\n", cmd.Name(), cmd.Description())
	}
	fmt.Println()
}
