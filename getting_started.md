# Getting Started with GoStack

GoStack is a full-stack web framework for Go. It is batteries-included, meaning everything you need to build a complete web application — routing, database access, authentication, queuing, caching, mail, file storage, and more — is built-in and ready to use the moment you scaffold a new project.

If you have just finished learning Go and want to build a real application, this guide walks you through every step from installation to your first working webpage.

---

## Prerequisites

You only need one thing installed before you can use GoStack:

**Go (Version 1.22 or newer)**
Download and run the installer from [go.dev/download](https://go.dev/download). The installer configures everything automatically for your operating system.

That is it. You do not need Git, Docker, or any database server to get started.

---

## Step 1 — Install the GoStack CLI

GoStack ships with a global command-line tool called **`gost`**. You install it once on your machine and use it everywhere.

Open your terminal and run:

```bash
go install github.com/charledeon77/gostack/cmd/gost@latest
```

Go will download, compile, and install the `gost` binary on your machine automatically. This may take a minute the first time.

Once it finishes, verify the installation worked:

```bash
gost
```

You should see the GoStack command list printed to your terminal. If your terminal says `gost: command not found`, see the [Troubleshooting](#troubleshooting) section at the bottom of this page.

---

## Step 2 — Create a New Project

Run the following command, replacing `myapp` with whatever you want to name your project:

```bash
gost new myapp
```

This launches the **interactive project wizard**. The wizard asks you a few simple questions to configure your project. For each question, you can press **Enter** to accept the recommended default.

### Wizard Walkthrough

**Question 1 — Database Engine**
```
? Select your database engine:
  1) Relational SQL (MySQL, PostgreSQL, CockroachDB, SQLite)
  2) MongoDB (NoSQL Document Store)
  3) Neo4j (Graph Database)
  4) Cassandra (Wide-Column NoSQL)

  Enter selection (1-4, default: 1):
```
Press **Enter** to select Relational SQL. This is the most common choice for web applications.

---

**Question 2 — SQL Dialect**
```
? Select your SQL dialect:
  1) MySQL
  2) PostgreSQL
  3) CockroachDB
  4) SQLite

  Enter selection (1-4, default: 4):
```
Press **Enter** to select SQLite. SQLite stores your entire database in a single local file inside your project folder. It requires no installation, no server, and no configuration. It is the right choice for local development.

> When you are ready to move your application to production, you simply change two lines in your `.env` file to point to MySQL or PostgreSQL. You do not need to rewrite any of your application code.

---

**Question 3 — Guard Authentication**
```
? Would you like to scaffold Guard Authentication? (y/N):
```
Type **`y`** and press Enter if you want GoStack to automatically generate a complete user authentication system (registration, login, logout, password hashing, and session management). Type **`N`** or press Enter to skip it and add it manually later.

---

### What Happens Behind the Scenes

After you answer the wizard questions, GoStack automatically:

1. Downloads the project template from GitHub directly over HTTPS (no Git required)
2. Generates a secure 32-character `APP_KEY` and writes it to your `.env` file
3. Configures your chosen database connection in `.env`
4. Sets up your Go module with your project name
5. Runs `go mod tidy` to download all dependencies
6. Initializes a fresh local Git repository (if Git is installed)
7. Prints a detailed summary of everything that was configured

---

## Step 3 — Start Your Application

Move into your new project folder:

```bash
cd myapp
```

Start the local development server:

```bash
gost serve
```

You will see GoStack print its startup log, including the port it is listening on. Open your browser and navigate to:

```
http://localhost:8080
```

Your GoStack application is live.

---

## Step 4 — Build Your First Webpage

Every webpage in a GoStack application is built from two parts:

- A **Controller** — a Go file containing the logic that runs when someone visits a URL
- A **Route** — a line of code that maps a URL path to a controller method

### Create a Controller

Use the `gost` CLI to generate a new controller:

```bash
gost make:controller WelcomeController
```

This creates a new file at `internal/controller/welcome_controller.go`. Open that file in your editor. You will see an empty controller struct with a placeholder method. Replace the contents with the following:

```go
package controller

import "github.com/charledeon77/gostack/framework/http"

type WelcomeController struct{}

func NewWelcomeController() *WelcomeController {
    return &WelcomeController{}
}

// Show handles GET requests to /welcome
func (c *WelcomeController) Show(ctx *http.Context) error {
    return ctx.HTML(http.StatusOK, `
        <!DOCTYPE html>
        <html lang="en">
        <head>
            <meta charset="UTF-8">
            <title>My GoStack App</title>
        </head>
        <body>
            <h1>Hello! This is my first GoStack page.</h1>
            <p>Built with GoStack — the full-stack Go framework.</p>
        </body>
        </html>
    `)
}
```

### Register the Route

Open `cmd/app/main.go`. This is the entry point of your application. Scroll down until you find the routing registration block (around Step 5 and Step 6 in the file comments). Add your new controller:

```go
// Step 5 — Instantiate Controllers
welcomeCtrl := controller.NewWelcomeController()

// Step 6 — Register Routes
router.Get("/welcome", welcomeCtrl.Show)
```

### Visit Your New Page

Make sure `gost serve` is still running (restart it if needed), then open:

```
http://localhost:8080/welcome
```

Your first GoStack webpage is live.

---

## Step 5 — Run Your Database Migrations

If you chose a SQL database and scaffolded Guard Authentication, GoStack has prepared the database migration files for the users and sessions tables. Apply them now:

```bash
gost migrate
```

GoStack will connect to your SQLite database file, create all the necessary tables, and confirm each migration. Your application is now ready to register and authenticate real users.

---

## Summary of All `gost` Commands

Once inside your project directory, you have access to the full GoStack CLI:

| Command | What It Does |
|---|---|
| `gost serve` | Start the local development web server |
| `gost migrate` | Run all pending database migrations |
| `gost rollback` | Roll back the last database migration |
| `gost make:model <Name>` | Generate a new database model |
| `gost make:controller <Name>` | Generate a new controller |
| `gost make:migration <Name>` | Generate a new migration schema file |
| `gost ui:preview` | Launch the interactive UI component gallery |

---

## Switching to a Production Database

When you are ready to move your application from local development to production, switching databases requires no code changes whatsoever.

Open the `.env` file in your project root and update these two values:

**For PostgreSQL:**
```env
DB_DRIVER=postgres
DB_DSN=postgres://username:password@127.0.0.1:5432/myapp_db?sslmode=disable
```

**For MySQL:**
```env
DB_DRIVER=mysql
DB_DSN=username:password@tcp(127.0.0.1:3306)/myapp_db
```

**For CockroachDB:**
```env
DB_DRIVER=cockroach
DB_DSN=postgres://username:password@127.0.0.1:26257/myapp_db?sslmode=disable
```

Restart the server with `gost serve` and GoStack will connect to your new database automatically.

---

## Troubleshooting

### `gost: command not found`

This occasionally happens after a fresh Go installation. Try the following steps in order:

**Step 1 — Close your terminal completely and open a brand new one.**
This is the fix for most people. Your terminal needs to be restarted to recognise newly installed tools. Once you have reopened it, run `gost` again.

**Step 2 — If it still does not work**, it means your Go installation was not set up completely on your machine. Visit [go.dev/doc/install](https://go.dev/doc/install) and follow the "Test your installation" instructions for your operating system to fix your Go setup. Once Go is working correctly, run the install command again:
```bash
go install github.com/charledeon77/gostack/cmd/gost@latest
```

---

### `failed to download project template`

This means `gost new` could not reach GitHub. Check that your internet connection is active and try again.

---

### Port 8080 is already in use

Another application on your machine is using port 8080. You can change GoStack's port by editing the `.env` file in your project root:
```env
APP_PORT=9000
```
Then restart the server with `gost serve`.
