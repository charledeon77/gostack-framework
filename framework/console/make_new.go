package console

import (
	"archive/zip"
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// NewCommand scaffolds a new GoStack project from the base GitHub repository.
type NewCommand struct{}

// Name returns the trigger string.
func (c *NewCommand) Name() string {
	return "new"
}

// Description returns the help text shown in lists.
func (c *NewCommand) Description() string {
	return "Scaffold a new GoStack project interactively (e.g. gost new myapp)"
}

// readInput displays a prompt and reads user input, defaulting to a fallback if input is empty.
func readInput(reader *bufio.Reader, prompt string, defaultValue string) string {
	fmt.Print(prompt)
	input, err := reader.ReadString('\n')
	if err != nil {
		return defaultValue
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue
	}
	return input
}

// Execute runs the command with standard string arguments.
func (c *NewCommand) Execute(args []string) error {
	projectName := ""
	noInteraction := false
	dbEngineArg := ""
	dbDriverArg := ""
	dbDSNArg := ""
	mongoURIArg := ""
	mongoDBArg := ""
	neo4jURIArg := ""
	neo4jUserArg := ""
	neo4jPassArg := ""
	cassandraHostsArg := ""
	cassandraKeyspaceArg := ""
	scaffoldAuthArg := ""

	for _, arg := range args {
		if arg == "--no-interaction" || arg == "-n" {
			noInteraction = true
		} else if strings.HasPrefix(arg, "--db-engine=") {
			dbEngineArg = strings.TrimPrefix(arg, "--db-engine=")
		} else if strings.HasPrefix(arg, "--db-driver=") {
			dbDriverArg = strings.TrimPrefix(arg, "--db-driver=")
		} else if strings.HasPrefix(arg, "--db-dsn=") {
			dbDSNArg = strings.TrimPrefix(arg, "--db-dsn=")
		} else if strings.HasPrefix(arg, "--mongo-uri=") {
			mongoURIArg = strings.TrimPrefix(arg, "--mongo-uri=")
		} else if strings.HasPrefix(arg, "--mongo-db=") {
			mongoDBArg = strings.TrimPrefix(arg, "--mongo-db=")
		} else if strings.HasPrefix(arg, "--neo4j-uri=") {
			neo4jURIArg = strings.TrimPrefix(arg, "--neo4j-uri=")
		} else if strings.HasPrefix(arg, "--neo4j-user=") {
			neo4jUserArg = strings.TrimPrefix(arg, "--neo4j-user=")
		} else if strings.HasPrefix(arg, "--neo4j-pass=") {
			neo4jPassArg = strings.TrimPrefix(arg, "--neo4j-pass=")
		} else if strings.HasPrefix(arg, "--cassandra-hosts=") {
			cassandraHostsArg = strings.TrimPrefix(arg, "--cassandra-hosts=")
		} else if strings.HasPrefix(arg, "--cassandra-keyspace=") {
			cassandraKeyspaceArg = strings.TrimPrefix(arg, "--cassandra-keyspace=")
		} else if strings.HasPrefix(arg, "--auth=") {
			scaffoldAuthArg = strings.TrimPrefix(arg, "--auth=")
		} else if !strings.HasPrefix(arg, "-") && projectName == "" {
			projectName = arg
		}
	}

	if projectName == "" {
		return fmt.Errorf("please specify a project name. Example: gost new myapp")
	}

	reader := bufio.NewReader(os.Stdin)

	// Clean CLI Welcome Screen
	fmt.Println("\033[1;36m┌────────────────────────────────────────────────────────┐\033[0m")
	fmt.Println("\033[1;36m│                 WELCOME TO GOSTACK                     │\033[0m")
	fmt.Println("\033[1;36m│      A Modern Fullstack Framework for Go               │\033[0m")
	fmt.Println("\033[1;36m└────────────────────────────────────────────────────────┘\033[0m")
	fmt.Println("\033[90m  Note: If you don't have a database server set up yet, press Enter to accept\033[0m")
	fmt.Println("\033[90m        the defaults. You can easily change them later in your .env file.\033[0m")
	fmt.Println()

	var dbType string // "sql", "mongodb", "neo4j", "cassandra"
	var dbDriver string // "mysql", "postgres", "sqlite"
	var dbDSN string
	var mongoURI, mongoDatabase string
	var neo4jURI, neo4jUsername, neo4jPassword string
	var cassandraHosts, cassandraKeyspace string
	var scaffoldAuth bool

	if noInteraction {
		if dbEngineArg == "" {
			dbEngineArg = "sql"
		}
		dbType = dbEngineArg

		switch dbType {
		case "mongodb":
			mongoURI = mongoURIArg
			if mongoURI == "" {
				mongoURI = "mongodb://127.0.0.1:27017"
			}
			mongoDatabase = mongoDBArg
			if mongoDatabase == "" {
				mongoDatabase = projectName
			}
		case "neo4j":
			neo4jURI = neo4jURIArg
			if neo4jURI == "" {
				neo4jURI = "neo4j://localhost:7687"
			}
			neo4jUsername = neo4jUserArg
			if neo4jUsername == "" {
				neo4jUsername = "neo4j"
			}
			neo4jPassword = neo4jPassArg
			if neo4jPassword == "" {
				neo4jPassword = "password"
			}
		case "cassandra":
			cassandraHosts = cassandraHostsArg
			if cassandraHosts == "" {
				cassandraHosts = "127.0.0.1:9042"
			}
			cassandraKeyspace = cassandraKeyspaceArg
			if cassandraKeyspace == "" {
				cassandraKeyspace = projectName
			}
		default:
			dbType = "sql"
			dbDriver = dbDriverArg
			if dbDriver == "" {
				dbDriver = "mysql"
			}
			dbDSN = dbDSNArg
			if dbDSN == "" {
				if dbDriver == "sqlite" {
					dbDSN = projectName + ".db"
				} else if dbDriver == "postgres" {
					dbDSN = "postgres://postgres:@127.0.0.1:5432/" + projectName + "?sslmode=disable"
				} else {
					dbDSN = "root:@tcp(127.0.0.1:3306)/" + projectName
				}
			}
		}

		if scaffoldAuthArg == "yes" || scaffoldAuthArg == "true" || scaffoldAuthArg == "y" {
			scaffoldAuth = true
		} else {
			scaffoldAuth = false
		}
	} else {
		// 1. Select Database Engine
		fmt.Println("\033[1;35m?\033[0m \033[1mSelect your database engine:\033[0m")
		fmt.Println("  1) Relational SQL (MySQL, PostgreSQL, CockroachDB, SQLite)")
		fmt.Println("  2) MongoDB (NoSQL Document Store)")
		fmt.Println("  3) Neo4j (Graph Database)")
		fmt.Println("  4) Cassandra (Wide-Column NoSQL)")
		engineChoice := readInput(reader, "  Enter selection (1-4, default: 1): ", "1")

		if engineChoice == "2" {
			dbType = "mongodb"
			fmt.Println("\n\033[1;36m⚙️ Configure MongoDB Connection:\033[0m")
			fmt.Println("  MongoDB is a fast, NoSQL Document Store. If you don't have one running, accept the defaults.")
			mongoURI = readInput(reader, "  Connection URI (default: mongodb://127.0.0.1:27017): ", "mongodb://127.0.0.1:27017")
			mongoDatabase = readInput(reader, "  Database name (default: "+projectName+"): ", projectName)
		} else if engineChoice == "3" {
			dbType = "neo4j"
			fmt.Println("\n\033[1;36m⚙️ Configure Neo4j Connection:\033[0m")
			fmt.Println("  Neo4j is a Graph Database. If you don't have one running, accept the defaults.")
			neo4jURI = readInput(reader, "  Connection URI (default: neo4j://localhost:7687): ", "neo4j://localhost:7687")
			neo4jUsername = readInput(reader, "  Username (default: neo4j): ", "neo4j")
			neo4jPassword = readInput(reader, "  Password (default: password): ", "password")
		} else if engineChoice == "4" {
			dbType = "cassandra"
			fmt.Println("\n\033[1;36m⚙️ Configure Cassandra Connection:\033[0m")
			fmt.Println("  Cassandra is a Wide-Column NoSQL Store. If you don't have one running, accept the defaults.")
			cassandraHosts = readInput(reader, "  Cluster Hosts (default: 127.0.0.1:9042): ", "127.0.0.1:9042")
			cassandraKeyspace = readInput(reader, "  Keyspace name (default: "+projectName+"): ", projectName)
		} else {
			dbType = "sql"
			fmt.Println("\n\033[1;35m?\033[0m \033[1mSelect your SQL dialect:\033[0m")
			fmt.Println("  1) MySQL")
			fmt.Println("  2) PostgreSQL")
			fmt.Println("  3) CockroachDB")
			fmt.Println("  4) SQLite (Local file-based SQL)")
			dialectChoice := readInput(reader, "  Enter selection (1-4, default: 1): ", "1")

			switch dialectChoice {
			case "2":
				dbDriver = "postgres"
				fmt.Println("\n\033[1;36m⚙️ Configure PostgreSQL Connection:\033[0m")
				host := readInput(reader, "  Host (default: 127.0.0.1): ", "127.0.0.1")
				port := readInput(reader, "  Port (default: 5432): ", "5432")
				dbname := readInput(reader, "  Database name (default: "+projectName+"): ", projectName)
				user := readInput(reader, "  Username (default: postgres): ", "postgres")
				pass := readInput(reader, "  Password (default: empty): ", "")
				dbDSN = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, dbname)
			case "3":
				dbDriver = "postgres" // CockroachDB is wire-compatible with PostgreSQL driver
				fmt.Println("\n\033[1;36m⚙️ Configure CockroachDB Connection:\033[0m")
				host := readInput(reader, "  Host (default: 127.0.0.1): ", "127.0.0.1")
				port := readInput(reader, "  Port (default: 26257): ", "26257")
				dbname := readInput(reader, "  Database name (default: "+projectName+"): ", projectName)
				user := readInput(reader, "  Username (default: root): ", "root")
				pass := readInput(reader, "  Password (default: empty): ", "")
				dbDSN = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, dbname)
			case "4":
				dbDriver = "sqlite"
				fmt.Println("\n\033[1;36m⚙️ Configure SQLite Connection:\033[0m")
				dbDSN = readInput(reader, "  Database file path (default: "+projectName+".db): ", projectName+".db")
			default:
				dbDriver = "mysql"
				fmt.Println("\n\033[1;36m⚙️ Configure MySQL Connection:\033[0m")
				host := readInput(reader, "  Host (default: 127.0.0.1): ", "127.0.0.1")
				port := readInput(reader, "  Port (default: 3306): ", "3306")
				dbname := readInput(reader, "  Database name (default: "+projectName+"): ", projectName)
				user := readInput(reader, "  Username (default: root): ", "root")
				pass := readInput(reader, "  Password (default: empty): ", "")
				dbDSN = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", user, pass, host, port, dbname)
			}
		}

		if dbType == "sql" {
			fmt.Println()
			fmt.Println("\033[1;36m🛡️ Guard Authentication:\033[0m")
			fmt.Println("  Guard is GoStack's built-in secure login system.")
			fmt.Println("  If enabled, the wizard will automatically generate:")
			fmt.Println("    • Customizable default User Registration & Login webpages (which you can fully edit later)")
			fmt.Println("    • Secure database tables for users and sessions")
			fmt.Println("    • Password hashing (Bcrypt) and session security middleware")
			fmt.Println()
			fmt.Println("  This gives you a fully working, secure login system out of the box.")
			fmt.Println()
			authChoice := readInput(reader, "\033[1;35m?\033[0m \033[1mWould you like to scaffold Guard Authentication? (y/N): \033[0m", "n")
			scaffoldAuth = strings.ToLower(authChoice) == "y" || strings.ToLower(authChoice) == "yes"
		}
	}

	fmt.Println()

	// 1. Download the Base Template ZIP natively
	fmt.Println("\033[1;32m📦 Downloading project template...\033[0m")
	if err := downloadAndUnzip(projectName); err != nil {
		return fmt.Errorf("failed to download project template: %w", err)
	}
	fmt.Println("\033[1;32m✔ Downloaded successfully!\033[0m")

	projectDir := filepath.Join(".", projectName)

	// 2. Git Init (optional helper to initialize local repository if git is installed)
	fmt.Println("\033[1;32m🌳 Initializing a new Git repository...\033[0m")
	gitInit := exec.Command("git", "init")
	gitInit.Dir = projectDir
	if err := gitInit.Run(); err != nil {
		fmt.Printf("⚠️ Note: Git is not installed or failed to initialize, skipping git setup: %v\n", err)
	} else {
		fmt.Println("\033[1;32m✔ Git repository initialized!\033[0m")
	}

	// 4. Environment Setup
	fmt.Println("\033[1;32m⚙️ Generating environment config (.env)...\033[0m")
	envExamplePath := filepath.Join(projectDir, ".env.example")
	envPath := filepath.Join(projectDir, ".env")
	envContent, err := os.ReadFile(envExamplePath)
	if err != nil {
		return fmt.Errorf("failed to read .env.example: %w", err)
	}

	// 5. App Key Generation
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("failed to generate random key: %w", err)
	}
	appKey := hex.EncodeToString(key)

	// Parse and replace environment variables in envContent dynamically based on database type
	lines := strings.Split(string(envContent), "\n")
	var newLines []string
	inDBSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "# ─── Database") || (strings.HasPrefix(trimmed, "#") && strings.Contains(trimmed, "Database")) {
			inDBSection = true
			newLines = append(newLines, line)
			switch dbType {
			case "sql":
				newLines = append(newLines, "DB_DRIVER="+dbDriver)
				newLines = append(newLines, "DB_DSN=\""+dbDSN+"\"")
			case "mongodb":
				newLines = append(newLines, "MONGO_URI=\""+mongoURI+"\"")
				newLines = append(newLines, "MONGO_DATABASE=\""+mongoDatabase+"\"")
			case "neo4j":
				newLines = append(newLines, "NEO4J_URI=\""+neo4jURI+"\"")
				newLines = append(newLines, "NEO4J_USERNAME=\""+neo4jUsername+"\"")
				newLines = append(newLines, "NEO4J_PASSWORD=\""+neo4jPassword+"\"")
			case "cassandra":
				newLines = append(newLines, "CASSANDRA_HOSTS=\""+cassandraHosts+"\"")
				newLines = append(newLines, "CASSANDRA_KEYSPACE=\""+cassandraKeyspace+"\"")
			}
			continue
		}

		if inDBSection {
			// Skip old relational config variables
			if strings.HasPrefix(trimmed, "DB_DRIVER=") ||
				strings.HasPrefix(trimmed, "DB_HOST=") ||
				strings.HasPrefix(trimmed, "DB_PORT=") ||
				strings.HasPrefix(trimmed, "DB_DATABASE=") ||
				strings.HasPrefix(trimmed, "DB_USERNAME=") ||
				strings.HasPrefix(trimmed, "DB_PASSWORD=") {
				continue
			}
			if strings.HasPrefix(trimmed, "#") && (strings.Contains(trimmed, "Session") || strings.Contains(trimmed, "───")) {
				inDBSection = false
			}
		}

		if strings.HasPrefix(trimmed, "APP_KEY=") {
			newLines = append(newLines, "APP_KEY="+appKey)
		} else if !inDBSection {
			newLines = append(newLines, line)
		}
	}
	envStr := strings.Join(newLines, "\n")

	if err := os.WriteFile(envPath, []byte(envStr), 0644); err != nil {
		return fmt.Errorf("failed to write .env file: %w", err)
	}
	fmt.Println("\033[1;32m✔ Configuration (.env) ready!\033[0m")

	// 6. Module Renaming
	fmt.Println("\033[1;32m📝 Renaming module imports to '" + projectName + "'...\033[0m")
	goModPath := filepath.Join(projectDir, "go.mod")
	modContent, err := os.ReadFile(goModPath)
	if err != nil {
		return fmt.Errorf("failed to read go.mod: %w", err)
	}

	modStr := string(modContent)
	linesMod := strings.Split(modStr, "\n")
	for i, line := range linesMod {
		if strings.HasPrefix(line, "module ") {
			linesMod[i] = "module " + projectName
			break
		}
	}

	if err := os.WriteFile(goModPath, []byte(strings.Join(linesMod, "\n")), 0644); err != nil {
		return fmt.Errorf("failed to write go.mod: %w", err)
	}

	// Rename imports across the project
	err = filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			newContent := strings.ReplaceAll(string(content), `"github.com/charledeon77/gostack-framework"`, `"`+projectName+`"`)
			newContent = strings.ReplaceAll(newContent, `"github.com/charledeon77/gostack-framework/`, `"`+projectName+`/`)
			if string(content) != newContent {
				os.WriteFile(path, []byte(newContent), 0644)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to rename module imports: %w", err)
	}
	fmt.Println("\033[1;32m✔ Module imports renamed!\033[0m")

	// Customize cmd/app/main.go database initialization
	mainGoPath := filepath.Join(projectDir, "cmd", "app", "main.go")
	mainContent, err := os.ReadFile(mainGoPath)
	if err == nil {
		mainStr := string(mainContent)
		var importInject string
		switch dbType {
		case "sql":
			switch dbDriver {
			case "mysql":
				importInject = "\n\t_ \"github.com/go-sql-driver/mysql\""
			case "postgres":
				importInject = "\n\t_ \"github.com/lib/pq\""
			case "sqlite":
				importInject = "\n\t_ \"modernc.org/sqlite\""
			}
		case "mongodb", "neo4j", "cassandra":
			importInject = "\n\t\"github.com/charledeon77/gostack-framework\""
		}
		mainStr = strings.Replace(mainStr, "import (", "import ("+importInject, 1)

		var dbSetupCode string
		switch dbType {
		case "sql":
			dbSetupCode = fmt.Sprintf(`	// 2. Initialize the Infrastructure Storage Layer (Database Adapter).
	dbDriver := foundation.Get("DB_DRIVER", "%s")
	dbDSN := foundation.Get("DB_DSN", "")
	if dbDSN == "" {
		dbDSN = "%s"
	}
	db, err := database.NewSQLAdapter(dbDriver, dbDSN)
	if err != nil {
		log.Printf("[GoStack App Warning] Database connection pool setup returned: %%v\n", err)
	}

	// 3. Initialize the Service Container and inject foundational dependencies.
	container := foundation.NewContainer()
	if db != nil {
		container.BindSingleton("db", func(c *foundation.Container) any {
			return db
		})
	}`, dbDriver, dbDSN)
		case "mongodb":
			dbSetupCode = `	// 2. Initialize the Infrastructure Storage Layer (GoMon MongoDB Client).
	mongoURI := foundation.Get("MONGO_URI", "mongodb://127.0.0.1:27017")
	mongoDB := foundation.Get("MONGO_DATABASE", "myapp")
	if err := gostack.InitMongo(mongoURI); err != nil {
		log.Printf("[GoStack App Warning] MongoDB client setup returned: %v\n", err)
	}

	// 3. Initialize the Service Container and inject foundational dependencies.
	container := foundation.NewContainer()
	if gostack.Mongo != nil {
		container.BindSingleton("mongo", func(c *foundation.Container) any {
			return gostack.Mongo
		})
		container.BindSingleton("mongo.db", func(c *foundation.Container) any {
			return gostack.Mongo.Database(mongoDB)
		})
	}`
		case "neo4j":
			dbSetupCode = `	// 2. Initialize the Infrastructure Storage Layer (Nexus Neo4j Driver).
	neo4jURI := foundation.Get("NEO4J_URI", "neo4j://localhost:7687")
	neo4jUser := foundation.Get("NEO4J_USERNAME", "neo4j")
	neo4jPass := foundation.Get("NEO4J_PASSWORD", "password")
	if err := gostack.InitNexus(neo4jURI, neo4jUser, neo4jPass); err != nil {
		log.Printf("[GoStack App Warning] Neo4j driver setup returned: %v\n", err)
	}

	// 3. Initialize the Service Container and inject foundational dependencies.
	container := foundation.NewContainer()
	if gostack.Neo4j != nil {
		container.BindSingleton("neo4j", func(c *foundation.Container) any {
			return gostack.Neo4j
		})
	}`
		case "cassandra":
			dbSetupCode = `	// 2. Initialize the Infrastructure Storage Layer (Aether Cassandra Session).
	cassandraHosts := foundation.Get("CASSANDRA_HOSTS", "127.0.0.1:9042")
	cassandraKeyspace := foundation.Get("CASSANDRA_KEYSPACE", "myapp")
	if err := gostack.InitAether(cassandraHosts, cassandraKeyspace); err != nil {
		log.Printf("[GoStack App Warning] Cassandra session setup returned: %v\n", err)
	}

	// 3. Initialize the Service Container and inject foundational dependencies.
	container := foundation.NewContainer()
	if gostack.Cassandra != nil {
		container.BindSingleton("cassandra", func(c *foundation.Container) any {
			return gostack.Cassandra
		})
	}`
		}

		targetBlock := `	// 2. Initialize the Infrastructure Storage Layer (Database Adapter).
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
	}`

		if strings.Contains(mainStr, targetBlock) {
			mainStr = strings.Replace(mainStr, targetBlock, dbSetupCode, 1)
		}
		os.WriteFile(mainGoPath, []byte(mainStr), 0644)
	}

	// Customize cmd/gostack/main.go database initialization
	gostackGoPath := filepath.Join(projectDir, "cmd", "gostack", "main.go")
	gostackContent, err := os.ReadFile(gostackGoPath)
	if err == nil {
		gostackStr := string(gostackContent)
		var importInject string
		switch dbType {
		case "sql":
			switch dbDriver {
			case "mysql":
				importInject = "\n\t_ \"github.com/go-sql-driver/mysql\""
			case "postgres":
				importInject = "\n\t_ \"github.com/lib/pq\""
			case "sqlite":
				importInject = "\n\t_ \"modernc.org/sqlite\""
			}
		}
		if importInject != "" {
			gostackStr = strings.Replace(gostackStr, "import (", "import ("+importInject, 1)
		}

		var dbSetupCode string
		switch dbType {
		case "sql":
			dbSetupCode = `// 2. Connect database if credentials are provided.
	if dsn != "" {
		if err := gostack.InitDatabase(driver, dsn); err != nil {
			log.Printf("[GoStack CLI Warning] Database connection failed: %v\n", err)
		}
	}`
		case "mongodb":
			dbSetupCode = `// 2. Connect database if credentials are provided.
	mongoURI := foundation.Get("MONGO_URI", "")
	if mongoURI != "" {
		if err := gostack.InitMongo(mongoURI); err != nil {
			log.Printf("[GoStack CLI Warning] MongoDB connection failed: %v\n", err)
		}
	}`
		case "neo4j":
			dbSetupCode = `// 2. Connect database if credentials are provided.
	neo4jURI := foundation.Get("NEO4J_URI", "")
	if neo4jURI != "" {
		neo4jUser := foundation.Get("NEO4J_USERNAME", "neo4j")
		neo4jPass := foundation.Get("NEO4J_PASSWORD", "password")
		if err := gostack.InitNexus(neo4jURI, neo4jUser, neo4jPass); err != nil {
			log.Printf("[GoStack CLI Warning] Neo4j connection failed: %v\n", err)
		}
	}`
		case "cassandra":
			dbSetupCode = `// 2. Connect database if credentials are provided.
	cassandraHosts := foundation.Get("CASSANDRA_HOSTS", "")
	if cassandraHosts != "" {
		cassandraKeyspace := foundation.Get("CASSANDRA_KEYSPACE", "myapp")
		if err := gostack.InitAether(cassandraHosts, cassandraKeyspace); err != nil {
			log.Printf("[GoStack CLI Warning] Cassandra connection failed: %v\n", err)
		}
	}`
		}

		targetBlock := `// 2. Connect database if credentials are provided.
	// Note: Generator commands (make:migration, make:controller) do not require a database connection,
	// so we log a warning instead of fatalling if connection configuration is missing or invalid.
	if dsn != "" {
		if err := gostack.InitDatabase(driver, dsn); err != nil {
			log.Printf("[GoStack CLI Warning] Database connection failed: %v\n", err)
		}
	}`

		if strings.Contains(gostackStr, targetBlock) {
			gostackStr = strings.Replace(gostackStr, targetBlock, dbSetupCode, 1)
		}
		os.WriteFile(gostackGoPath, []byte(gostackStr), 0644)
	}

	// 7. Prune unused database driver dependencies before resolving
	pruneUnusedDriverDeps(projectDir, dbType, dbDriver)

	// 8. Initial go mod tidy to resolve renamed imports
	fmt.Println("\033[1;32m Resolving module dependencies...\033[0m")
	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = projectDir
	tidyCmd.Stdout = os.Stdout
	tidyCmd.Stderr = os.Stderr
	if err := tidyCmd.Run(); err != nil {
		fmt.Println(" Warning: initial go mod tidy failed. You may need to run it manually.")
	}

	// 8. Scaffold Guard Authentication if selected
	if scaffoldAuth {
		fmt.Println("\033[1;32m🔒 Scaffolding Guard Authentication...\033[0m")
		authCmd := exec.Command("go", "run", "cmd/gostack/main.go", "make:auth")
		authCmd.Dir = projectDir
		authCmd.Stdout = os.Stdout
		authCmd.Stderr = os.Stderr
		if err := authCmd.Run(); err != nil {
			fmt.Printf("⚠️ Warning: Failed to scaffold authentication: %v\n", err)
		} else {
			fmt.Println("\033[1;32m✔ Authentication scaffolded successfully!\033[0m")
			
			// Run go mod tidy again to resolve any new imports added by make:auth
			tidyAuthCmd := exec.Command("go", "mod", "tidy")
			tidyAuthCmd.Dir = projectDir
			tidyAuthCmd.Stdout = os.Stdout
			tidyAuthCmd.Stderr = os.Stderr
			_ = tidyAuthCmd.Run()
		}
	}
	fmt.Println("\033[1;32m✔ Dependencies resolved!\033[0m")

	// 9. Initial Git Commit (always performed automatically)
	fmt.Println("\033[1;32m🌳 Making initial commit...\033[0m")
	gitAdd := exec.Command("git", "add", ".")
	gitAdd.Dir = projectDir
	_ = gitAdd.Run()

	gitCommit := exec.Command("git", "commit", "-m", "Initial commit from GoStack scaffolding")
	gitCommit.Dir = projectDir
	_ = gitCommit.Run()
	fmt.Println("\033[1;32m✔ Initial commit created!\033[0m")

	// Beautiful styled output
	fmt.Println()
	fmt.Println("\033[1;32m┌────────────────────────────────────────────────────────┐\033[0m")
	fmt.Println("\033[1;32m│ ⚡ GOSTACK PROJECT CREATED SUCCESSFULLY                │\033[0m")
	fmt.Println("\033[1;32m└────────────────────────────────────────────────────────┘\033[0m")
	fmt.Println()
	fmt.Printf("\033[1m📋 Scaffolding Summary:\033[0m\n")
	fmt.Printf("  • Project Name:    \033[1;36m%s\033[0m\n", projectName)
	
	switch dbType {
	case "sql":
		fmt.Printf("  • Database Engine: \033[1;36mRelational SQL (%s)\033[0m\n", dbDriver)
		fmt.Printf("  • Connection DSN:  \033[36m%s\033[0m\n", dbDSN)
	case "mongodb":
		fmt.Printf("  • Database Engine: \033[1;36mMongoDB (GoMon)\033[0m\n")
		fmt.Printf("  • Connection URI:  \033[36m%s\033[0m\n", mongoURI)
		fmt.Printf("  • Database Name:   \033[36m%s\033[0m\n", mongoDatabase)
	case "neo4j":
		fmt.Printf("  • Database Engine: \033[1;36mNeo4j (Nexus Graph)\033[0m\n")
		fmt.Printf("  • Connection URI:  \033[36m%s\033[0m\n", neo4jURI)
		fmt.Printf("  • Auth User:       \033[36m%s\033[0m\n", neo4jUsername)
	case "cassandra":
		fmt.Printf("  • Database Engine: \033[1;36mCassandra (Aether)\033[0m\n")
		fmt.Printf("  • Cluster Hosts:   \033[36m%s\033[0m\n", cassandraHosts)
		fmt.Printf("  • Keyspace:        \033[36m%s\033[0m\n", cassandraKeyspace)
	}

	if scaffoldAuth {
		fmt.Println("  • Guard Auth:      \033[1;32mScaffolded (Relational User Guard)\033[0m")
	} else {
		fmt.Println("  • Guard Auth:      \033[33mNot Scaffolded\033[0m")
	}
	fmt.Println("  • Application Key: \033[1;32mGenerated & Configured (.env)\033[0m")
	fmt.Println()

	fmt.Println("\033[1;32m🚀 Next Steps to Get Started:\033[0m")
	fmt.Printf("  1. Enter your project directory:\n")
	fmt.Printf("     \033[36mcd %s\033[0m\n\n", projectName)
	
	if dbType == "sql" {
		if scaffoldAuth {
			fmt.Println("  2. Run database migrations to create auth tables:")
			fmt.Println("     \033[36mgost migrate\033[0m")
			fmt.Println()
			fmt.Println("  3. Start the local web server:")
			fmt.Println("     \033[36mgost serve\033[0m")
		} else {
			fmt.Println("  2. Run database migrations:")
			fmt.Println("     \033[36mgost migrate\033[0m")
			fmt.Println()
			fmt.Println("  3. Start the local web server:")
			fmt.Println("     \033[36mgost serve\033[0m")
		}
	} else {
		fmt.Println("  2. Start the local web server:")
		fmt.Println("     \033[36mgost serve\033[0m")
	}
	fmt.Println()
	fmt.Println("  \033[1mAdditional Commands (run these from inside your project folder):\033[0m")
	fmt.Println("     - \033[36mgost make:model <Name>\033[0m")
	fmt.Println("       Creates a new Database Model. This is a Go struct representing a database table,")
	fmt.Println("       allowing you to easily read, write, and query data using Go code instead of writing raw SQL.")
	fmt.Println()
	fmt.Println("     - \033[36mgost make:controller <Name>\033[0m")
	fmt.Println("       Creates a new Request Controller. This is a Go file where you write the logic for")
	fmt.Println("       handling incoming web requests (e.g. loading a page, processing a form submit, or returning JSON).")
	fmt.Println()
	if dbType == "sql" {
		fmt.Println("     - \033[36mgost make:migration <Name>\033[0m")
		fmt.Println("       Creates a new Database Migration file. This lets you safely create, modify, or delete")
		fmt.Println("       database tables and columns using version-controlled Go code.")
		fmt.Println()
	}
	fmt.Println("     - \033[36mgost ui:preview\033[0m")
	fmt.Println("       Launches the Interactive UI Gallery. This opens a local web dashboard where you can")
	fmt.Println("       browse, preview, and copy code for over 100+ pre-built, responsive UI components.")
	fmt.Println()

	return nil
}

// downloadAndUnzip retrieves the main project template ZIP from GitHub over HTTPS,
// and extracts its contents natively in pure Go, bypassing the system Git command.
func downloadAndUnzip(projectName string) error {
	url := "https://github.com/charledeon77/gostack-framework/archive/refs/heads/main.zip"
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("network connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github repository returned status: %s", resp.Status)
	}

	// Save to a temporary file
	tempFile, err := os.CreateTemp("", "gostack-*.zip")
	if err != nil {
		return fmt.Errorf("failed to create local temporary file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	if _, err = io.Copy(tempFile, resp.Body); err != nil {
		return fmt.Errorf("failed to download repository template ZIP: %w", err)
	}

	if _, err = tempFile.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to read downloaded template ZIP: %w", err)
	}

	stat, err := tempFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to verify template ZIP integrity: %w", err)
	}

	reader, err := zip.NewReader(tempFile, stat.Size())
	if err != nil {
		return fmt.Errorf("failed to open template ZIP: %w", err)
	}

	for _, file := range reader.File {
		// Strip the top-level directory "gostack-main/" from GitHub's ZIP packaging
		parts := strings.Split(file.Name, "/")
		if len(parts) <= 1 {
			continue // skip the parent directory itself
		}

		subPath := filepath.Join(parts[1:]...)
		destPath := filepath.Join(projectName, subPath)

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, file.Mode()); err != nil {
				return fmt.Errorf("failed to create directory structure: %w", err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create parent directories: %w", err)
		}

		rc, err := file.Open()
		if err != nil {
			return fmt.Errorf("failed to read file inside ZIP: %w", err)
		}

		outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			rc.Close()
			return fmt.Errorf("failed to write local file: %w", err)
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return fmt.Errorf("failed to extract file: %w", err)
		}
	}

	return nil
}

// pruneUnusedDriverDeps removes unused database driver packages from go.mod
// before go mod tidy runs, ensuring developers only download the packages
// they actually need for their chosen database engine.
func pruneUnusedDriverDeps(projectDir, dbType, dbDriver string) {
	var toRemove []string

	if dbType == "sql" {
		// Remove SQL drivers that were not chosen
		if dbDriver != "mysql" {
			toRemove = append(toRemove, "github.com/go-sql-driver/mysql")
		}
		if dbDriver != "postgres" && dbDriver != "cockroach" {
			toRemove = append(toRemove, "github.com/lib/pq")
		}
		if dbDriver != "sqlite" {
			toRemove = append(toRemove, "modernc.org/sqlite")
		}
	} else {
		// User chose a non-SQL engine — remove all SQL drivers
		toRemove = append(toRemove, "github.com/go-sql-driver/mysql")
		toRemove = append(toRemove, "github.com/lib/pq")
		toRemove = append(toRemove, "modernc.org/sqlite")
	}

	if dbType != "mongodb" {
		toRemove = append(toRemove, "go.mongodb.org/mongo-driver")
	}
	if dbType != "neo4j" {
		toRemove = append(toRemove, "github.com/neo4j/neo4j-go-driver/v5")
	}
	if dbType != "cassandra" {
		toRemove = append(toRemove, "github.com/gocql/gocql")
	}

	for _, pkg := range toRemove {
		dropCmd := exec.Command("go", "mod", "edit", "-droprequire="+pkg)
		dropCmd.Dir = projectDir
		_ = dropCmd.Run() // Failures are non-fatal; go mod tidy will reconcile anyway
	}
}
