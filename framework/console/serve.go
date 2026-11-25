package console

import (
	"fmt"
	"os"
	"os/exec"
)

/*
Purpose:
This file implements the `gost serve` command for the GoStack framework.

Philosophy:
New developers should not need to remember internal file paths like
`go run cmd/app/main.go`. A single short command — `gost serve` — is enough.
This matches the experience of `php artisan serve` in Laravel and
`python manage.py runserver` in Django.

Architecture:
ServeCommand is intentionally minimal. It delegates entirely to the Go
toolchain by running `go run cmd/app/main.go` as a subprocess, passing
through all stdout/stderr output and OS signals so the developer sees
live server logs exactly as they would running the command directly.

Usage:
  gost serve              Start on default port (8080, from .env APP_PORT)
*/

// ServeCommand starts the GoStack application web server.
type ServeCommand struct{}

// Name returns the trigger string.
func (c *ServeCommand) Name() string {
	return "serve"
}

// Description returns the help text shown in command lists.
func (c *ServeCommand) Description() string {
	return "Start the GoStack web server (equivalent of: go run cmd/app/main.go)"
}

// Execute runs the GoStack application server.
func (c *ServeCommand) Execute(args []string) error {
	// Verify that cmd/app/main.go exists in the current working directory.
	// This guards against the user running `gost serve` outside a GoStack project.
	if _, err := os.Stat("cmd/app/main.go"); os.IsNotExist(err) {
		return fmt.Errorf(
			"no GoStack project found in the current directory.\n\n" +
				"  Make sure you are inside your project folder:\n" +
				"    cd <your-project>\n" +
				"    gost serve",
		)
	}

	fmt.Println("\033[1;36m⚡ GoStack\033[0m — Starting web server...")
	fmt.Println("\033[90m  Press Ctrl+C to stop.\033[0m")
	fmt.Println()

	cmd := exec.Command("go", "run", "./cmd/app")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}
