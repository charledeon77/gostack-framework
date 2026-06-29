/*
Purpose:
This file is the global command-line installer for the GoStack framework.
It provides a single, lightweight binary — "gost" — that a developer installs
once on their machine and uses to scaffold new GoStack projects from anywhere.

Philosophy:
The pattern mirrors Laravel's global installer (`laravel new myapp`) and
Django's project creator (`django-admin startproject myapp`). A single global
binary is responsible only for project creation. Once a project exists, all
further commands (migrate, make:model, make:controller, etc.) are run through
the project-local CLI at `cmd/gostack/main.go`, which is compiled against the
project's exact dependency versions.

Installation:
  go install github.com/charledeon77/gostack-framework/cmd/gost@latest

Usage:
  gost new <project-name>           Interactive wizard (recommended)
  gost new <project-name> -n        Headless / CI mode with smart defaults
  gost new <project-name> --db-engine=sqlite --no-interaction
*/
package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"

	"github.com/charledeon77/gostack-framework/framework/console"
)

const FallbackVersion = "v1.0.0"

// getVersion returns the compiled Git tag version if installed via go install,
// or falls back to the hardcoded constant for local developer builds.
func getVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
	}
	return FallbackVersion
}

func main() {
	args := os.Args[1:]

	// If no arguments are provided, show a friendly usage hint.
	if len(args) == 0 {
		fmt.Println("\033[1;36mGoStack — A Modern Fullstack Framework for Go\033[0m")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  gost new <project-name>     Scaffold a new GoStack project")
		fmt.Println("  gost serve                  Start the GoStack web server")
		fmt.Println("  gost migrate                Run pending database migrations")
		fmt.Println("  gost rollback               Rollback the last database migration")
		fmt.Println("  gost make:model <name>      Generate a new database model")
		fmt.Println("  gost make:controller <name> Generate a new request controller")
		fmt.Println("  gost make:migration <name>  Generate a new migration schema")
		fmt.Println()
		fmt.Println("Getting started:")
		fmt.Println("  gost new myblog")
		fmt.Println("  cd myblog")
		fmt.Println("  gost serve")
		os.Exit(0)
	}

	commandName := args[0]

	// Handle version flags — works from anywhere, no project directory required.
	if commandName == "-v" || commandName == "--version" || commandName == "version" {
		fmt.Printf("\033[1;36mGoStack %s\033[0m\n", getVersion())
		fmt.Println("   A Modern Fullstack Framework for Go")
		fmt.Println()
		fmt.Printf("   Platform : %s/%s\n", runtime.GOOS, runtime.GOARCH)
		fmt.Printf("   Go       : %s\n", runtime.Version())
		fmt.Println("   Docs     : https://charledeon77.github.io/gostack-docs/")
		fmt.Println("   Source   : https://github.com/charledeon77/gostack-framework")
		os.Exit(0)
	}

	// "new" is the only command handled globally by the installer
	if commandName == "new" {
		kernel := console.NewKernel()
		kernel.Register(&console.NewCommand{})
		if err := kernel.Run(args); err != nil {
			fmt.Printf("\033[1;31m[GoStack Error]\033[0m %v\n", err)
			os.Exit(1)
		}
		return
	}

	// For all other commands, check if we are inside a GoStack project directory.
	// We check for the presence of the local CLI at cmd/gostack/main.go.
	if _, err := os.Stat("cmd/gostack/main.go"); os.IsNotExist(err) {
		fmt.Println("\033[1;31m[GoStack Error]\033[0m You must run this command inside a GoStack project directory.")
		fmt.Println()
		fmt.Println("  To start a new project:")
		fmt.Println("    gost new myapp")
		fmt.Println("    cd myapp")
		fmt.Println("    gost serve")
		os.Exit(1)
	}

	// Delegate the command to the local project-level CLI
	cmdArgs := append([]string{"run", "cmd/gostack/main.go"}, args...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			os.Exit(exitError.ExitCode())
		}
		os.Exit(1)
	}
}
