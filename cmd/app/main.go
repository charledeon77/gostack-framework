// Package main serves as the primary operational entry point for the GoStack framework application.
// It is strictly responsible for manual dependency graph construction, coordinating the lifecycle 
// of storage adapters, service registries, routing infrastructures, and presentation controllers, 
// before finally booting up the centralized framework Kernel.
package main

import (
	"fmt"
	"github.com/charledeon77/gostack-framework/framework/contract"
	"github.com/charledeon77/gostack-framework/framework/database"
	"github.com/charledeon77/gostack-framework/framework/foundation"
	"github.com/charledeon77/gostack-framework/framework/http"
	"github.com/charledeon77/gostack-framework/internal/controller"
	"io"
	"log"
)

// main coordinates and orchestrates the critical startup sequence of the GoStack application.
func main() {
	// 1. Initialize the View Engine (Tempose).
	temposeEngine := http.NewTempose()

	// Register all compiled component views, styles, and scripts.
	RegisterComponents(temposeEngine)

	temposeEngine.Register("landing", func(w io.Writer, data any, t http.ViewTranslator) error {
		_, err := io.WriteString(w, `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>GoStack UI — Premium Demo</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Outfit:wght@400;500;600;700;800&family=Plus+Jakarta+Sans:wght@400;500;600;700&display=swap" rel="stylesheet">
    <style>
        :root {
            --bg-dark: #060608;
            --bg-surface: #0e0e12;
            --border-color: rgba(255, 255, 255, 0.08);
            --text-primary: #f8fafc;
            --text-secondary: #94a3b8;
            --accent: #6366f1;
            --font-sans: 'Plus Jakarta Sans', sans-serif;
            --font-display: 'Outfit', sans-serif;
        }
        body {
            margin: 0;
            background: var(--bg-dark);
            color: var(--text-primary);
            font-family: var(--font-sans);
            display: flex;
            flex-direction: column;
            min-height: 100vh;
        }
        /* Hero Section */
        .hero {
            position: relative;
            padding: 100px 24px;
            text-align: center;
            background: radial-gradient(circle at top, rgba(99, 102, 241, 0.15) 0%, transparent 60%);
            border-bottom: 1px solid var(--border-color);
        }
        .hero-title {
            font-family: var(--font-display);
            font-size: 48px;
            font-weight: 800;
            letter-spacing: -1.5px;
            margin: 0 0 16px 0;
            background: linear-gradient(135deg, #fff 30%, #a5b4fc 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        .hero-desc {
            font-size: 18px;
            color: var(--text-secondary);
            max-width: 600px;
            margin: 0 auto;
            line-height: 1.6;
        }
        /* Test Section */
        .test-section {
            padding: 80px 24px;
            max-width: 600px;
            margin: 0 auto;
            width: 100%;
            box-sizing: border-box;
        }
        .section-title {
            font-family: var(--font-display);
            font-size: 28px;
            font-weight: 700;
            letter-spacing: -0.5px;
            margin: 0 0 12px 0;
            text-align: center;
        }
        .section-desc {
            color: var(--text-secondary);
            font-size: 15px;
            text-align: center;
            margin: 0 0 40px 0;
            line-height: 1.5;
        }
        /* Center Accordion Component */
        .component-container {
            display: flex;
            justify-content: center;
            width: 100%;
        }
    </style>
</head>
<body class="gostack-root">
    <!-- Hero Section -->
    <header class="hero">
        <h1 class="hero-title">Experience GoStack UI</h1>
        <p class="hero-desc">An elegant fullstack orchestration engine for Go paired with a pre-compiled, reactive component library.</p>
    </header>

    <!-- Test Section -->
    <main class="test-section">
        <h2 class="section-title">Test Section</h2>
        <p class="section-desc">Below is the Accordion UI component dynamically pulled, scoped, and rendered.</p>
        
        <div class="component-container">
`)
		if err != nil {
			return err
		}

		if err := temposeEngine.Render(w, "accordion", nil, t); err != nil {
			return err
		}

		_, err = io.WriteString(w, `
        </div>
    </main>
</body>
</html>`)
		return err
	})

	// 2. Initialize the Infrastructure Storage Layer (Database Adapter).
	dbDriver := foundation.Get("DB_DRIVER", "mysql")
	dbDSN := foundation.Get("DB_DSN", "")
	if dbDSN == "" {
		// Provide a default string to avoid crashing during dry builds
		dbDSN = "root:password@tcp(localhost:3306)/gostack"
	}
	db, err := database.NewSQLAdapter(dbDriver, dbDSN)
	if err != nil {
		log.Printf("[GoStack App Warning] Database connection pool setup returned: %v\n", err)
	}

	// 3. Initialize the Service Container and inject foundational dependencies.
	container := foundation.NewContainer()
	if db != nil {
		container.BindSingleton("db", func(c *foundation.Container) any {
			return db
		})
	}
	container.BindSingleton("tempose", func(c *foundation.Container) any {
		return temposeEngine
	})

	// 4. Initialize the HTTP Routing Infrastructure.
	router := http.NewRouter()

	// 5. Instantiate the Home Presentation Controller.
	var dbInstance contract.Database
	if db != nil {
		rawDB, err := container.Resolve("db")
		if err == nil {
			dbInstance = rawDB.(contract.Database)
		}
	}
	home := controller.NewHomeController(dbInstance)

	// 6. Register application endpoints.
	router.Get("/", home.Index, http.Logger)
	router.Get("/users", home.Users, http.Logger)

	// 7. Orchestrate Framework Orchestration & Boot up the Application Kernel.
	kernel := foundation.NewKernel(container, router)
	port := ":" + foundation.Get("APP_PORT", "8080")
	fmt.Printf("GoStack fullstack engine starting up securely on port %s...\n", port)

	if err := kernel.Run(port); err != nil {
		log.Fatalf("Critical Error: Core framework server crashed or failed to bind to socket address: %v", err)
	}
}