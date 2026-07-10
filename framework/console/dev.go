package console

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/charledeon77/gostack-framework/framework/ui"
)

// DevCommand manages the hot-reloading development cycle for templates and Go files.
type DevCommand struct{}

// Name returns the trigger string.
func (c *DevCommand) Name() string {
	return "dev"
}

// Description returns help text shown in command lists.
func (c *DevCommand) Description() string {
	return "Start the GoStack web server with hot reload (recompiles components and restarts on code changes)"
}

// Execute runs the GoStack app dev watcher environment.
func (c *DevCommand) Execute(args []string) error {
	// 1. Verify we are inside a GoStack project directory
	if _, err := os.Stat("cmd/app/main.go"); os.IsNotExist(err) {
		return fmt.Errorf(
			"no GoStack project found in the current directory.\n\n" +
				"  Make sure you are inside your project folder:\n" +
				"    cd <your-project>\n" +
				"    gost dev",
		)
	}

	fmt.Println("\033[1;36m⚡ GoStack Dev Server\033[0m — Starting hot-reload mode...")
	fmt.Println("\033[90m  Press Ctrl+C to stop.\033[0m")
	fmt.Println()

	// Dynamically name temporary binary based on OS
	binaryName := "tmp_server"
	if filepath.Separator == '\\' {
		binaryName += ".exe"
	}

	// Clean up any stale binary from previous runs
	_ = os.Remove(binaryName)

	// Channel to intercept termination signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Instantiate the component AssetCompiler
	compiler := ui.NewAssetCompiler("templates/components", "cmd/app")
	log.Println("[GoStack Dev] Running initial component compilation...")
	if err := compiler.Run(); err != nil {
		log.Printf("[GoStack Dev] Initial component compilation warning: %v", err)
	}

	var cmd *exec.Cmd

	// Helper function to build and boot/restart the web server
	startServer := func() error {
		// Terminate existing server process first
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}

		log.Println("[GoStack Dev] Compiling application...")
		buildCmd := exec.Command("go", "build", "-o", binaryName, "./cmd/app")
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("build failed: %w", err)
		}

		log.Println("[GoStack Dev] Starting web server subprocess...")
		cmd = exec.Command("./" + binaryName)
		cmd.Env = append(os.Environ(), "APP_ENV=local")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("failed to start server process: %w", err)
		}
		return nil
	}

	// Trigger initial boot
	if err := startServer(); err != nil {
		log.Printf("[GoStack Dev] Initial server start error: %v", err)
	}

	// Defer clean up of subprocess and built binary on command exit
	defer func() {
		if cmd != nil && cmd.Process != nil {
			log.Println("[GoStack Dev] Stopping web server subprocess...")
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
		log.Println("[GoStack Dev] Cleaning up temporary files...")
		_ = os.Remove(binaryName)
	}()

	// Polling trackers
	templateSnap := make(map[string]time.Time)
	goSnap := make(map[string]time.Time)

	// Helper: recursively snapshot component asset mod times
	snapTemplates := func() map[string]time.Time {
		snap := make(map[string]time.Time)
		_ = filepath.Walk("templates/components", func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			snap[path] = info.ModTime()
			return nil
		})
		return snap
	}

	// Helper: recursively snapshot Go source file mod times
	snapGo := func() map[string]time.Time {
		snap := make(map[string]time.Time)
		_ = filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				name := info.Name()
				if name == ".git" || name == "vendor" || name == "node_modules" || name == ".agents" || name == ".gemini" {
					return filepath.SkipDir
				}
				return nil
			}
			// Exclude the temp binary itself and any built executable files
			if info.Name() == binaryName {
				return nil
			}
			if strings.HasSuffix(path, ".go") {
				snap[path] = info.ModTime()
			}
			return nil
		})
		return snap
	}

	// Prime initial state snapshots
	templateSnap = snapTemplates()
	goSnap = snapGo()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-sigs:
			fmt.Println("\n[GoStack Dev] Terminating dev session cleanly...")
			return nil

		case <-ticker.C:
			// 1. Scan templates directory
			currTemplates := snapTemplates()
			templateChanged := false
			if len(currTemplates) != len(templateSnap) {
				templateChanged = true
			} else {
				for path, modTime := range currTemplates {
					if templateSnap[path] != modTime {
						templateChanged = true
						break
					}
				}
			}

			if templateChanged {
				templateSnap = currTemplates
				log.Println("[GoStack Dev] Component templates changed — recompiling...")
				if err := compiler.Run(); err != nil {
					log.Printf("[GoStack Dev] Asset compile failure: %v", err)
				}
			}

			// 2. Scan Go source files
			currGo := snapGo()
			goChanged := false
			if len(currGo) != len(goSnap) {
				goChanged = true
			} else {
				for path, modTime := range currGo {
					if goSnap[path] != modTime {
						goChanged = true
						break
					}
				}
			}

			if goChanged {
				goSnap = currGo
				log.Println("[GoStack Dev] Go source code changed — restarting web server...")
				if err := startServer(); err != nil {
					log.Printf("[GoStack Dev] Rebuild and restart failure: %v", err)
				}
			}
		}
	}
}
