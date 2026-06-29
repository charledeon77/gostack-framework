/*
Purpose:
This file implements the `make:migration` and `make:controller` CLI code generation commands.

Philosophy:
A framework should automate boilerplate generation. Generating code programmatically
using structured templates prevents typos, speeds up development, and maintains
cohesive coding standards. Templates are formatted using Go's official AST formatter.

Architecture:
Implements console.Command. Both commands use text/template + go/format to produce
syntactically valid Go source files.

Implementation:
- MakeMigrationCommand: scaffolds a self-registering migration file under database/migrations/.
- MakeControllerCommand: scaffolds an HTTP controller stub under internal/controller/.
*/
package console

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"github.com/charledeon77/gostack/framework/ui"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

// ─── make:migration ────────────────────────────────────────────────────────────

// MakeMigrationCommand scaffolds a new migration file under database/migrations/.
type MakeMigrationCommand struct{}

func (c *MakeMigrationCommand) Name() string        { return "make:migration" }
func (c *MakeMigrationCommand) Description() string {
	return "Scaffold a new database migration file (Usage: make:migration <name>)"
}

type migrationTemplateData struct {
	Version int64
	Table   string
}

const migrationStub = `/*
Purpose:
This file defines a database migration version {{.Version}} to modify the database schema.
Specifically, it handles the creation and destruction of the "{{.Table}}" table.
*/
package migrations

import (
	"github.com/charledeon77/gostack/framework/database"
)

func init() {
	database.Register({{.Version}},
		func(s *database.Builder) error {
			return s.Create("{{.Table}}", func(t *database.Table) {
				t.ID()
				// Define your table columns here (e.g. t.String("email").Unique())
				t.Timestamps()
			})
		},
		func(s *database.Builder) error {
			return s.Drop("{{.Table}}")
		},
	)
}
`

// Execute runs the migration scaffolding pipeline.
func (c *MakeMigrationCommand) Execute(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("make:migration failed: migration name argument is missing. Example: make:migration create_posts_table")
	}

	rawName := args[0]
	versionStr := time.Now().Format("20060102150405")
	var versionInt int64
	if _, err := fmt.Sscanf(versionStr, "%d", &versionInt); err != nil {
		return fmt.Errorf("failed to compile version timestamp: %w", err)
	}

	tableName := strings.TrimSuffix(strings.TrimPrefix(rawName, "create_"), "_table")
	fileName := fmt.Sprintf("%s_%s.go", versionStr, rawName)
	filePath := filepath.Join("database", "migrations", fileName)

	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	tmpl, err := template.New("migration").Parse(migrationStub)
	if err != nil {
		return fmt.Errorf("failed to parse migration template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, migrationTemplateData{Version: versionInt, Table: tableName}); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to format generated source: %w", err)
	}

	if err := os.WriteFile(filePath, formatted, 0644); err != nil {
		return fmt.Errorf("failed to write migration file: %w", err)
	}

	fmt.Printf("[GoStack CLI] Scaffolding migration complete: %s\n", filePath)
	return nil
}

// ─── make:controller ───────────────────────────────────────────────────────────

// MakeControllerCommand scaffolds a new HTTP controller stub under internal/controller/.
type MakeControllerCommand struct{}

func (c *MakeControllerCommand) Name() string        { return "make:controller" }
func (c *MakeControllerCommand) Description() string {
	return "Scaffold a new HTTP controller stub (Usage: make:controller <name>)"
}

type controllerTemplateData struct {
	StructName string
}

const controllerStub = `/*
Purpose:
This file defines the "{{.StructName}}" HTTP Controller.
*/
package controller

import (
	"github.com/charledeon77/gostack/framework/contract"
	"github.com/charledeon77/gostack/framework/http"
	netHTTP "net/http"
)

type {{.StructName}} struct {
	db contract.Database
}

func New{{.StructName}}(db contract.Database) *{{.StructName}} {
	return &{{.StructName}}{db: db}
}

func (c *{{.StructName}}) Index(ctx *http.Context) error {
	return ctx.JSON(netHTTP.StatusOK, map[string]string{
		"message": "Hello from {{.StructName}}!",
	})
}
`

// Execute runs the controller scaffolding pipeline.
func (c *MakeControllerCommand) Execute(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("make:controller failed: controller name argument is missing")
	}

	rawName := args[0]
	structName := toCamelCase(rawName)
	if !strings.HasSuffix(structName, "Controller") {
		structName += "Controller"
	}

	fileName := toSnakeCase(structName) + ".go"
	filePath := filepath.Join("internal", "controller", fileName)

	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	tmpl, err := template.New("controller").Parse(controllerStub)
	if err != nil {
		return fmt.Errorf("failed to parse controller template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, controllerTemplateData{StructName: structName}); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to format generated source: %w", err)
	}

	if err := os.WriteFile(filePath, formatted, 0644); err != nil {
		return fmt.Errorf("failed to write controller file: %w", err)
	}

	fmt.Printf("[GoStack CLI] Scaffolding controller complete: %s\n", filePath)
	return nil
}

func toCamelCase(s string) string {
	parts := strings.Split(s, "_")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

func toSnakeCase(s string) string {
	var result []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		result = append(result, r)
	}
	return strings.ToLower(string(result))
}

// ─── make:auth ────────────────────────────────────────────────────────────────

// MakeAuthCommand scaffolds database tables, model logic, controller, and views
// for a production-ready authentication subsystem.
type MakeAuthCommand struct{}

func (c *MakeAuthCommand) Name() string        { return "make:auth" }
func (c *MakeAuthCommand) Description() string {
	return "Scaffold a complete authentication system (Users model, migrations, login/register UI views, controller)"
}

func (c *MakeAuthCommand) Execute(args []string) error {
	now := time.Now()
	versionStr := now.Format("20060102150405")
	var versionUsers int64
	if _, err := fmt.Sscanf(versionStr, "%d", &versionUsers); err != nil {
		return fmt.Errorf("failed to compile version timestamp: %w", err)
	}
	versionTokens := versionUsers + 1

	// 1. Generate migrations
	migrationData := map[string]int64{
		"users":       versionUsers,
		"user_tokens": versionTokens,
	}

	migrations := map[string]string{
		fmt.Sprintf("%d_create_users_table.go", versionUsers): `package migrations

import (
	"github.com/charledeon77/gostack/framework/database"
)

func init() {
	database.Register({{.users}},
		func(s *database.Builder) error {
			return s.Create("users", func(t *database.Table) {
				t.ID()
				t.String("email").Unique()
				t.String("password")
				t.Timestamps()
			})
		},
		func(s *database.Builder) error {
			return s.Drop("users")
		},
	)
}
`,
		fmt.Sprintf("%d_create_user_tokens_table.go", versionTokens): `package migrations

import (
	"github.com/charledeon77/gostack/framework/database"
)

func init() {
	database.Register({{.user_tokens}},
		func(s *database.Builder) error {
			return s.Create("user_tokens", func(t *database.Table) {
				t.ID()
				t.Integer("user_id")
				t.String("token").Unique()
				t.Timestamp("expires_at")
				t.Timestamps()
			})
		},
		func(s *database.Builder) error {
			return s.Drop("user_tokens")
		},
	)
}
`,
	}

	for fileName, tmplStr := range migrations {
		filePath := filepath.Join("database", "migrations", fileName)
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return err
		}
		tmpl, err := template.New("migration").Parse(tmplStr)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, migrationData); err != nil {
			return err
		}
		formatted, err := format.Source(buf.Bytes())
		if err != nil {
			return fmt.Errorf("failed to format %s: %w", fileName, err)
		}
		if err := os.WriteFile(filePath, formatted, 0644); err != nil {
			return err
		}
		fmt.Printf("[GoStack CLI] Scaffolding migration: %s\n", filePath)
	}

	// 2. Generate User and UserProvider models
	models := map[string]string{
		"user.go": `package model

import "time"

// User represents the persistent user account record.
type User struct {
	ID        int64     _BT_db:"id"_BT_
	Email     string    _BT_db:"email"_BT_
	Password  string    _BT_db:"password"_BT_
	CreatedAt time.Time _BT_db:"created_at"_BT_
	UpdatedAt time.Time _BT_db:"updated_at"_BT_
}

// GetID returns the primary key ID.
func (u *User) GetID() any {
	return u.ID
}

// GetEmail returns the login email address.
func (u *User) GetEmail() string {
	return u.Email
}

// GetPassword returns the hashed bcrypt password.
func (u *User) GetPassword() string {
	return u.Password
}
`,
		"user_provider.go": `package model

import (
	"github.com/charledeon77/gostack"
	"github.com/charledeon77/gostack/framework/auth"
	"github.com/charledeon77/gostack/framework/contract"
	"time"
)

// UserProvider handles database operations to retrieve users and tokens.
type UserProvider struct {
	hasher contract.Hasher
}

// NewUserProvider builds a fresh UserProvider instance.
func NewUserProvider(hasher contract.Hasher) *UserProvider {
	return &UserProvider{hasher: hasher}
}

// RetrieveByID retrieves a user struct by database primary key.
func (p *UserProvider) RetrieveByID(id any) (contract.Authenticatable, error) {
	var users []User
	err := gostack.Table("users").Where("id", "=", id).Get(&users)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, auth.ErrUserNotFound
	}
	return &users[0], nil
}

// RetrieveByCredentials resolves a user by credentials (like email/password or bearer tokens).
func (p *UserProvider) RetrieveByCredentials(credentials map[string]any) (contract.Authenticatable, error) {
	// 1. API Token Lookup
	if token, ok := credentials["token"].(string); ok && token != "" {
		var userTokens []struct {
			UserID    int64     _BT_db:"user_id"_BT_
			ExpiresAt time.Time _BT_db:"expires_at"_BT_
		}
		err := gostack.Table("user_tokens").Where("token", "=", token).Get(&userTokens)
		if err != nil {
			return nil, err
		}
		if len(userTokens) == 0 {
			return nil, auth.ErrInvalidToken
		}
		if time.Now().After(userTokens[0].ExpiresAt) {
			return nil, auth.ErrTokenExpired
		}
		return p.RetrieveByID(userTokens[0].UserID)
	}

	// 2. Email Lookup
	if email, ok := credentials["email"].(string); ok && email != "" {
		var users []User
		err := gostack.Table("users").Where("email", "=", email).Get(&users)
		if err != nil {
			return nil, err
		}
		if len(users) == 0 {
			return nil, auth.ErrUserNotFound
		}
		return &users[0], nil
	}

	return nil, auth.ErrUserNotFound
}

// ValidateCredentials verifies the user's password hash against the given credentials.
func (p *UserProvider) ValidateCredentials(user contract.Authenticatable, credentials map[string]any) bool {
	plain, ok := credentials["password"].(string)
	if !ok {
		return false
	}
	return p.hasher.Verify(plain, user.GetPassword())
}
`,
	}

	for fileName, content := range models {
		filePath := filepath.Join("internal", "model", fileName)
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return err
		}
		content = strings.ReplaceAll(content, "_BT_", "`")
		formatted, err := format.Source([]byte(content))
		if err != nil {
			return fmt.Errorf("failed to format %s: %w", fileName, err)
		}
		if err := os.WriteFile(filePath, formatted, 0644); err != nil {
			return err
		}
		fmt.Printf("[GoStack CLI] Scaffolding model: %s\n", filePath)
	}


	// 3. Generate AuthController
	controllerContent := `package controller

import (
	"crypto/rand"
	"encoding/hex"
	"github.com/charledeon77/gostack"
	"github.com/charledeon77/gostack/framework/auth"
	"github.com/charledeon77/gostack/framework/contract"
	"github.com/charledeon77/gostack/framework/http"
	"github.com/charledeon77/gostack/internal/model"
	netHTTP "net/http"
	"time"
)

// AuthController coordinates user logins, registration, and logouts.
type AuthController struct {
	db     contract.Database
	hasher contract.Hasher
}

// NewAuthController builds a new AuthController.
func NewAuthController(db contract.Database, hasher contract.Hasher) *AuthController {
	return &AuthController{db: db, hasher: hasher}
}

// ShowLogin renders the login UI page.
func (c *AuthController) ShowLogin(ctx *http.Context) error {
	csrfToken := ctx.Get("csrf_token")
	return ctx.Render("login", map[string]any{
		"csrf_token": csrfToken,
		"error":      "",
	})
}

// ShowRegister renders the registration page.
func (c *AuthController) ShowRegister(ctx *http.Context) error {
	csrfToken := ctx.Get("csrf_token")
	return ctx.Render("register", map[string]any{
		"csrf_token": csrfToken,
		"error":      "",
	})
}

// Login processes cookie-based session login requests.
func (c *AuthController) Login(ctx *http.Context) error {
	email := ctx.Post("email")
	password := ctx.Post("password")

	guard := gostack.Auth.Guard("session").(*auth.SessionGuard)
	credentials := map[string]any{"email": email, "password": password}

	if err := guard.Attempt(ctx.Writer, ctx.Request, credentials); err != nil {
		csrfToken := ctx.Get("csrf_token")
		return ctx.Render("login", map[string]any{
			"csrf_token": csrfToken,
			"error":      "Invalid email or password",
		})
	}

	ctx.Writer.Header().Set("Location", "/home")
	ctx.Writer.WriteHeader(netHTTP.StatusFound)
	return nil
}

// Register signs up a user by hashing their password and storing details.
func (c *AuthController) Register(ctx *http.Context) error {
	email := ctx.Post("email")
	password := ctx.Post("password")

	if email == "" || password == "" {
		csrfToken := ctx.Get("csrf_token")
		return ctx.Render("register", map[string]any{
			"csrf_token": csrfToken,
			"error":      "Email and password cannot be empty",
		})
	}

	hashed, err := c.hasher.Hash(password)
	if err != nil {
		csrfToken := ctx.Get("csrf_token")
		return ctx.Render("register", map[string]any{
			"csrf_token": csrfToken,
			"error":      "Failed to hash password",
		})
	}

	// Save to DB using QueryBuilder
	err = gostack.Table("users").Insert(map[string]any{
		"email":      email,
		"password":   hashed,
		"created_at": time.Now(),
		"updated_at": time.Now(),
	})
	if err != nil {
		csrfToken := ctx.Get("csrf_token")
		return ctx.Render("register", map[string]any{
			"csrf_token": csrfToken,
			"error":      "User already exists or database error",
		})
	}

	// Log them in immediately after registration
	guard := gostack.Auth.Guard("session").(*auth.SessionGuard)
	credentials := map[string]any{"email": email, "password": password}
	if err := guard.Attempt(ctx.Writer, ctx.Request, credentials); err != nil {
		csrfToken := ctx.Get("csrf_token")
		return ctx.Render("login", map[string]any{
			"csrf_token": csrfToken,
			"error":      "Registration successful, but login failed. Please sign in.",
		})
	}

	ctx.Writer.Header().Set("Location", "/home")
	ctx.Writer.WriteHeader(netHTTP.StatusFound)
	return nil
}

// Logout signs the user out of their session.
func (c *AuthController) Logout(ctx *http.Context) error {
	gostack.Auth.Logout(ctx.Writer, ctx.Request)
	ctx.Writer.Header().Set("Location", "/login")
	ctx.Writer.WriteHeader(netHTTP.StatusFound)
	return nil
}

// APILogin processes stateless login requests, returning an API Bearer token.
func (c *AuthController) APILogin(ctx *http.Context) error {
	email := ctx.Post("email")
	password := ctx.Post("password")

	var users []model.User
	err := gostack.Table("users").Where("email", "=", email).Get(&users)
	if err != nil || len(users) == 0 {
		return ctx.JSON(netHTTP.StatusUnauthorized, map[string]string{"error": "Invalid credentials"})
	}

	if !c.hasher.Verify(password, users[0].Password) {
		return ctx.JSON(netHTTP.StatusUnauthorized, map[string]string{"error": "Invalid credentials"})
	}

	// Seed secure random API token
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)
	expiresAt := time.Now().Add(24 * time.Hour) // Token valid for 24h

	err = gostack.Table("user_tokens").Insert(map[string]any{
		"user_id":    users[0].ID,
		"token":      token,
		"expires_at": expiresAt,
		"created_at": time.Now(),
		"updated_at": time.Now(),
	})
	if err != nil {
		return ctx.JSON(netHTTP.StatusInternalServerError, map[string]string{"error": "Failed to create token"})
	}

	return ctx.JSON(netHTTP.StatusOK, map[string]any{
		"token":      token,
		"expires_at": expiresAt.Format(time.RFC3339),
	})
}
`
	controllerPath := filepath.Join("internal", "controller", "auth_controller.go")
	formattedController, err := format.Source([]byte(controllerContent))
	if err != nil {
		return fmt.Errorf("failed to format auth_controller.go: %w", err)
	}
	if err := os.WriteFile(controllerPath, formattedController, 0644); err != nil {
		return err
	}
	fmt.Printf("[GoStack CLI] Scaffolding controller: %s\n", controllerPath)

	// 4. Generate login and register views (AOT components)
	components := map[string]map[string]string{
		"login": {
			"login.html": `<div class="auth-card">
	<h2>Login</h2>
	<form action="/login" method="POST">
		<input type="hidden" name="_token" value="{{.csrf_token}}" />
		<div class="form-group">
			<label>Email Address</label>
			<input type="email" name="email" required placeholder="name@example.com" />
		</div>
		<div class="form-group">
			<label>Password</label>
			<input type="password" name="password" required />
		</div>
		<div class="alert alert-danger">{{.error}}</div>
		<button type="submit" class="btn">Sign In</button>
	</form>
	<p class="auth-switch">Don't have an account? <a href="/register">Register</a></p>
</div>`,
			"login.css": `.auth-card {
	max-width: 400px;
	margin: 50px auto;
	padding: 30px;
	background: #ffffff;
	border-radius: 12px;
	box-shadow: 0 4px 20px rgba(0,0,0,0.08);
	font-family: 'Inter', sans-serif;
}
.auth-card h2 {
	margin-top: 0;
	margin-bottom: 24px;
	color: #1a1a1a;
	font-weight: 700;
	text-align: center;
}
.form-group {
	margin-bottom: 20px;
}
.form-group label {
	display: block;
	margin-bottom: 8px;
	font-size: 14px;
	font-weight: 500;
	color: #4a4a4a;
}
.form-group input {
	width: 100%;
	padding: 12px 16px;
	border: 1px solid #dcdcdc;
	border-radius: 8px;
	box-sizing: border-box;
	font-size: 15px;
	transition: border-color 0.2s;
}
.form-group input:focus {
	outline: none;
	border-color: #4f46e5;
}
.btn {
	display: block;
	width: 100%;
	padding: 14px;
	background: #4f46e5;
	color: #ffffff;
	border: none;
	border-radius: 8px;
	font-size: 16px;
	font-weight: 600;
	cursor: pointer;
	transition: background-color 0.2s;
}
.btn:hover {
	background: #4338ca;
}
.alert {
	padding: 12px;
	border-radius: 8px;
	margin-bottom: 20px;
	font-size: 14px;
}
.alert:empty {
	display: none;
}
.alert-danger {
	background: #fee2e2;
	color: #991b1b;
}
.auth-switch {
	margin-top: 24px;
	text-align: center;
	font-size: 14px;
	color: #6b7280;
}
.auth-switch a {
	color: #4f46e5;
	text-decoration: none;
	font-weight: 500;
}
.auth-switch a:hover {
	text-decoration: underline;
}`,
			"login.js": `// Login client scripts`,
		},
		"register": {
			"register.html": `<div class="auth-card">
	<h2>Register</h2>
	<form action="/register" method="POST">
		<input type="hidden" name="_token" value="{{.csrf_token}}" />
		<div class="form-group">
			<label>Email Address</label>
			<input type="email" name="email" required placeholder="name@example.com" />
		</div>
		<div class="form-group">
			<label>Password</label>
			<input type="password" name="password" required />
		</div>
		<div class="alert alert-danger">{{.error}}</div>
		<button type="submit" class="btn">Create Account</button>
	</form>
	<p class="auth-switch">Already have an account? <a href="/login">Login</a></p>
</div>`,
			"register.css": `.auth-card {
	max-width: 400px;
	margin: 50px auto;
	padding: 30px;
	background: #ffffff;
	border-radius: 12px;
	box-shadow: 0 4px 20px rgba(0,0,0,0.08);
	font-family: 'Inter', sans-serif;
}
.auth-card h2 {
	margin-top: 0;
	margin-bottom: 24px;
	color: #1a1a1a;
	font-weight: 700;
	text-align: center;
}
.form-group {
	margin-bottom: 20px;
}
.form-group label {
	display: block;
	margin-bottom: 8px;
	font-size: 14px;
	font-weight: 500;
	color: #4a4a4a;
}
.form-group input {
	width: 100%;
	padding: 12px 16px;
	border: 1px solid #dcdcdc;
	border-radius: 8px;
	box-sizing: border-box;
	font-size: 15px;
	transition: border-color 0.2s;
}
.form-group input:focus {
	outline: none;
	border-color: #4f46e5;
}
.btn {
	display: block;
	width: 100%;
	padding: 14px;
	background: #4f46e5;
	color: #ffffff;
	border: none;
	border-radius: 8px;
	font-size: 16px;
	font-weight: 600;
	cursor: pointer;
	transition: background-color 0.2s;
}
.btn:hover {
	background: #4338ca;
}
.alert {
	padding: 12px;
	border-radius: 8px;
	margin-bottom: 20px;
	font-size: 14px;
}
.alert:empty {
	display: none;
}
.alert-danger {
	background: #fee2e2;
	color: #991b1b;
}
.auth-switch {
	margin-top: 24px;
	text-align: center;
	font-size: 14px;
	color: #6b7280;
}
.auth-switch a {
	color: #4f46e5;
	text-decoration: none;
	font-weight: 500;
}
.auth-switch a:hover {
	text-decoration: underline;
}`,
			"register.js": `// Register client scripts`,
		},
	}

	for componentName, files := range components {
		componentDir := filepath.Join("templates", "components", componentName)
		if err := os.MkdirAll(componentDir, 0755); err != nil {
			return err
		}
		for name, content := range files {
			filePath := filepath.Join(componentDir, name)
			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				return err
			}
			fmt.Printf("[GoStack CLI] Scaffolding view: %s\n", filePath)
		}
	}

	// 5. Trigger Asset Compiler programmatically to recompile components
	compiler := ui.NewAssetCompiler(filepath.Join("templates", "components"), filepath.Join("cmd", "app"))
	if err := compiler.Run(); err != nil {
		return fmt.Errorf("failed to compile view assets: %w", err)
	}

	fmt.Println("[GoStack CLI] Scaffolding authentication subsystem completed successfully!")
	fmt.Println("[GoStack CLI] Next steps:")
	fmt.Println("  1. Import the generated user and userprovider models in main.go")
	fmt.Println("  2. Call gostack.InitAuth(\"session\", model.NewUserProvider(auth.NewBcryptHasher(10)))")
	fmt.Println("  3. Run your migrations: go run cmd/gostack/main.go migrate")
	return nil
}

// ─── make:model ───────────────────────────────────────────────────────────────

/*
Purpose:
MakeModelCommand scaffolds a typed Go model struct under internal/model/.
It optionally generates the companion database migration in one step via the
--migration flag, matching the ergonomics of `php artisan make:model Post -m`.

Philosophy:
Model structs should be the single source of truth for a table's shape. By generating
them from the CLI we guarantee consistent db tag naming, a standard GetID() contract
implementation, and correct package placement — removing an entire category of copy-paste
errors that plague hand-written models.

Architecture:
Implements console.Command. Uses text/template + go/format to produce valid Go source.
If --migration is present it delegates to MakeMigrationCommand.Execute() to reuse
the exact same migration scaffolding pipeline.

Choice:
We derive the table name automatically from the struct name (e.g. Post → posts) following
the same convention as ActiveRecord/Eloquent, so developers never have to specify it
explicitly for the common case.

Implementation:
- Name(): returns "make:model".
- Execute(args): validates args, derives StructName/tableName/fileName,
  writes internal/model/<name>.go, optionally invokes MakeMigrationCommand.
*/

// MakeModelCommand scaffolds a new typed Go model struct under internal/model/.
type MakeModelCommand struct{}

// Name returns the CLI trigger string for this command.
func (c *MakeModelCommand) Name() string { return "make:model" }

// Description returns the human-readable help text shown in the CLI command listing.
func (c *MakeModelCommand) Description() string {
	return "Scaffold a typed model struct (Usage: make:model <Name> [--migration])"
}

type modelTemplateData struct {
	StructName string
	TableName  string
}

const modelStub = `/*
Purpose:
This file defines the "{{.StructName}}" model, representing a single row of the
"{{.TableName}}" database table.
*/
package model

import "time"

// {{.StructName}} represents a record in the {{.TableName}} table.
type {{.StructName}} struct {
	ID        int64     ` + "`" + `db:"id"` + "`" + `
	CreatedAt time.Time ` + "`" + `db:"created_at"` + "`" + `
	UpdatedAt time.Time ` + "`" + `db:"updated_at"` + "`" + `
}

// GetID returns the primary key value, satisfying the contract.Authenticatable interface
// and GoStack's generic model conventions.
func (m *{{.StructName}}) GetID() any { return m.ID }
`

// Execute runs the model scaffolding pipeline.
func (c *MakeModelCommand) Execute(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("make:model failed: model name argument is missing. Example: make:model Post")
	}

	// Strip --migration flag before processing the name.
	withMigration := false
	filteredArgs := args[:0]
	for _, a := range args {
		if a == "--migration" {
			withMigration = true
		} else {
			filteredArgs = append(filteredArgs, a)
		}
	}
	if len(filteredArgs) < 1 {
		return fmt.Errorf("make:model failed: model name argument is missing")
	}

	rawName := filteredArgs[0]
	structName := toCamelCase(rawName)
	// Derive the snake_case plural table name (e.g. BlogPost → blog_posts).
	tableName := toSnakeCase(structName) + "s"

	fileName := toSnakeCase(structName) + ".go"
	filePath := filepath.Join("internal", "model", fileName)

	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	tmpl, err := template.New("model").Parse(modelStub)
	if err != nil {
		return fmt.Errorf("failed to parse model template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, modelTemplateData{StructName: structName, TableName: tableName}); err != nil {
		return fmt.Errorf("failed to execute model template: %w", err)
	}

	// Replace backtick placeholder before formatting.
	src := strings.ReplaceAll(buf.String(), "_BT_", "`")

	formatted, err := format.Source([]byte(src))
	if err != nil {
		return fmt.Errorf("failed to format generated source: %w", err)
	}

	if err := os.WriteFile(filePath, formatted, 0644); err != nil {
		return fmt.Errorf("failed to write model file: %w", err)
	}

	fmt.Printf("[GoStack CLI] Scaffolding model: %s\n", filePath)

	// Optionally scaffold the companion migration.
	if withMigration {
		migrationName := fmt.Sprintf("create_%s_table", tableName)
		migCmd := &MakeMigrationCommand{}
		if err := migCmd.Execute([]string{migrationName}); err != nil {
			return fmt.Errorf("failed to scaffold companion migration: %w", err)
		}
	}

	fmt.Printf("[GoStack CLI] Model %s scaffolded successfully!\n", structName)
	fmt.Printf("  File: %s\n", filePath)
	if withMigration {
		fmt.Printf("  Migration also generated — run: go run cmd/gostack/main.go migrate\n")
	}
	return nil
}

// ─── make:request ────────────────────────────────────────────────────────────

// MakeRequestCommand scaffolds a new request validation struct under internal/request/.
type MakeRequestCommand struct{}

func (c *MakeRequestCommand) Name() string        { return "make:request" }
func (c *MakeRequestCommand) Description() string {
	return "Scaffold a new request validation struct (Usage: make:request <name>)"
}

type requestTemplateData struct {
	StructName string
}

const requestStub = `/*
Purpose:
This file defines the "{{.StructName}}" request validation struct.
*/
package request

import (
	"github.com/charledeon77/gostack/framework/http"
)

type {{.StructName}} struct {
	// Add request fields here (with json tags)
	// Example:
	// Email    string ` + "`" + `json:"email"` + "`" + `
	// Password string ` + "`" + `json:"password"` + "`" + `
}

// Validate executes validation rules against the request payload.
func (r *{{.StructName}}) Validate() map[string]string {
	return http.Rules(r, http.RuleSet{
		// Define field rules here
		// Example:
		// "Email":    {http.Required, http.IsEmail},
		// "Password": {http.Required, http.MinLength(8)},
	})
}
`

// Execute runs the request scaffolding pipeline.
func (c *MakeRequestCommand) Execute(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("make:request failed: request name argument is missing. Example: make:request StoreUserRequest")
	}

	rawName := args[0]
	structName := toCamelCase(rawName)
	if !strings.HasSuffix(structName, "Request") {
		structName += "Request"
	}

	fileName := toSnakeCase(structName) + ".go"
	filePath := filepath.Join("internal", "request", fileName)

	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	tmpl, err := template.New("request").Parse(requestStub)
	if err != nil {
		return fmt.Errorf("failed to parse request template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, requestTemplateData{StructName: structName}); err != nil {
		return fmt.Errorf("failed to execute request template: %w", err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to format generated source: %w", err)
	}

	if err := os.WriteFile(filePath, formatted, 0644); err != nil {
		return fmt.Errorf("failed to write request file: %w", err)
	}

	fmt.Printf("[GoStack CLI] Scaffolding request complete: %s\n", filePath)
	return nil
}

// ─── make:middleware ──────────────────────────────────────────────────────────

// MakeMiddlewareCommand scaffolds a new HTTP middleware interceptor under internal/middleware/.
type MakeMiddlewareCommand struct{}

func (c *MakeMiddlewareCommand) Name() string        { return "make:middleware" }
func (c *MakeMiddlewareCommand) Description() string {
	return "Scaffold a new HTTP middleware interceptor (Usage: make:middleware <name>)"
}

type middlewareTemplateData struct {
	FuncName string
}

const middlewareStub = `/*
Purpose:
This file defines the "{{.FuncName}}" HTTP middleware interceptor.
*/
package middleware

import (
	"github.com/charledeon77/gostack/framework/http"
)

// {{.FuncName}} intercepts incoming HTTP requests.
func {{.FuncName}}(ctx *http.Context, next http.NextHandler) error {
	// Execute pre-request logic here (before calling the downstream handler)
	
	err := next(ctx)
	
	// Execute post-request logic here (after the downstream handler finishes)
	
	return err
}
`

// Execute runs the middleware scaffolding pipeline.
func (c *MakeMiddlewareCommand) Execute(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("make:middleware failed: middleware name argument is missing. Example: make:middleware log_request")
	}

	rawName := args[0]
	funcName := toCamelCase(rawName)
	if !strings.HasSuffix(funcName, "Middleware") {
		funcName += "Middleware"
	}

	fileName := toSnakeCase(funcName) + ".go"
	filePath := filepath.Join("internal", "middleware", fileName)

	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	tmpl, err := template.New("middleware").Parse(middlewareStub)
	if err != nil {
		return fmt.Errorf("failed to parse middleware template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, middlewareTemplateData{FuncName: funcName}); err != nil {
		return fmt.Errorf("failed to execute middleware template: %w", err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to format generated source: %w", err)
	}

	if err := os.WriteFile(filePath, formatted, 0644); err != nil {
		return fmt.Errorf("failed to write middleware file: %w", err)
	}

	fmt.Printf("[GoStack CLI] Scaffolding middleware complete: %s\n", filePath)
	return nil
}

// ─── make:mail ───────────────────────────────────────────────────────────────

// MakeMailCommand scaffolds a new custom email class under internal/mail/.
type MakeMailCommand struct{}

func (c *MakeMailCommand) Name() string        { return "make:mail" }
func (c *MakeMailCommand) Description() string {
	return "Scaffold a new mail message class (Usage: make:mail <name>)"
}

type mailTemplateData struct {
	StructName string
}

const mailStub = `/*
Purpose:
This file defines the "{{.StructName}}" mail constructor.
*/
package mail

import (
	"github.com/charledeon77/gostack/framework/mail"
)

// {{.StructName}} represents a mailable message layout.
type {{.StructName}} struct {
	// Add mail payload/context fields here
	RecipientName string
}

// New{{.StructName}} creates a new instance of {{.StructName}}.
func New{{.StructName}}(recipientName string) *{{.StructName}} {
	return &{{.StructName}}{RecipientName: recipientName}
}

// Build compiles the {{.StructName}} layout into a deliverable mail.Message.
func (m *{{.StructName}}) Build() mail.Message {
	return mail.Message{
		Subject: "Notification from GoStack",
		Body:    "Hello " + m.RecipientName + ",\n\nThis is a notification email sent from GoStack.",
		IsHTML:  false,
	}
}
`

// Execute runs the mail scaffolding pipeline.
func (c *MakeMailCommand) Execute(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("make:mail failed: mail class name argument is missing. Example: make:mail OrderShipped")
	}

	rawName := args[0]
	structName := toCamelCase(rawName)
	if !strings.HasSuffix(structName, "Mail") {
		structName += "Mail"
	}

	fileName := toSnakeCase(structName) + ".go"
	filePath := filepath.Join("internal", "mail", fileName)

	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	tmpl, err := template.New("mail").Parse(mailStub)
	if err != nil {
		return fmt.Errorf("failed to parse mail template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, mailTemplateData{StructName: structName}); err != nil {
		return fmt.Errorf("failed to execute mail template: %w", err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to format generated source: %w", err)
	}

	if err := os.WriteFile(filePath, formatted, 0644); err != nil {
		return fmt.Errorf("failed to write mail file: %w", err)
	}

	fmt.Printf("[GoStack CLI] Scaffolding mail class complete: %s\n", filePath)
	return nil
}

// ─── make:seeder ─────────────────────────────────────────────────────────────

// MakeSeederCommand scaffolds a new database table seeder under database/seeders/.
type MakeSeederCommand struct{}

func (c *MakeSeederCommand) Name() string        { return "make:seeder" }
func (c *MakeSeederCommand) Description() string {
	return "Scaffold a new database table seeder (Usage: make:seeder <name>)"
}

type seederTemplateData struct {
	StructName string
}

const seederStub = `/*
Purpose:
This file defines the "{{.StructName}}" seeder.
*/
package seeders

import (
	"github.com/charledeon77/gostack/framework/database"
)

type {{.StructName}} struct{}

func init() {
	database.RegisterSeeder("{{.StructName}}", &{{.StructName}}{})
}

// Run executes the database seed operation.
func (s *{{.StructName}}) Run() error {
	// Seed your database table here using gostack.Table()
	// Example:
	// return gostack.Table("users").Insert(map[string]any{
	// 	"email":      "user@example.com",
	// 	"password":   "hashed_password",
	// 	"created_at": time.Now(),
	// 	"updated_at": time.Now(),
	// })
	return nil
}
`

// Execute runs the seeder scaffolding pipeline.
func (c *MakeSeederCommand) Execute(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("make:seeder failed: seeder name argument is missing. Example: make:seeder UserSeeder")
	}

	rawName := args[0]
	structName := toCamelCase(rawName)
	if !strings.HasSuffix(structName, "Seeder") {
		structName += "Seeder"
	}

	fileName := toSnakeCase(structName) + ".go"
	filePath := filepath.Join("database", "seeders", fileName)

	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	tmpl, err := template.New("seeder").Parse(seederStub)
	if err != nil {
		return fmt.Errorf("failed to parse seeder template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, seederTemplateData{StructName: structName}); err != nil {
		return fmt.Errorf("failed to execute seeder template: %w", err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to format generated source: %w", err)
	}

	if err := os.WriteFile(filePath, formatted, 0644); err != nil {
		return fmt.Errorf("failed to write seeder file: %w", err)
	}

	fmt.Printf("[GoStack CLI] Scaffolding seeder complete: %s\n", filePath)
	return nil
}

// ─── add ───────────────────────────────────────────────────────────────────

// AddComponentCommand downloads UI components from GitHub registry.
type AddComponentCommand struct{}

func (c *AddComponentCommand) Name() string {
	return "add"
}

func (c *AddComponentCommand) Description() string {
	return "Add a UI component from the GoStack repository (Usage: add <component-name>)"
}

type componentInfo struct {
	Description  string   `json:"description"`
	Dependencies []string `json:"dependencies"`
	Files        []string `json:"files"`
}

type componentRegistry struct {
	Name       string                   `json:"name"`
	Version    string                   `json:"version"`
	Components map[string]componentInfo `json:"components"`
}

// Execute runs the component downloader and compilation.
func (c *AddComponentCommand) Execute(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("add failed: component name argument is missing. Example: gost add button")
	}

	targetComp := strings.ToLower(args[0])

	fmt.Printf("[GoStack CLI] Fetching remote registry.json...\n")
	registryURL := "https://raw.githubusercontent.com/Charledeon77/gostack-components/main/registry.json"
	resp, err := http.Get(registryURL)
	if err != nil {
		return fmt.Errorf("failed to fetch component registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch registry: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read registry response: %w", err)
	}

	var reg componentRegistry
	if err := json.Unmarshal(body, &reg); err != nil {
		return fmt.Errorf("failed to parse component registry JSON: %w", err)
	}

	// Resolve dependencies
	var queue []string
	visited := make(map[string]bool)

	var resolveDeps func(name string) error
	resolveDeps = func(name string) error {
		if visited[name] {
			return nil
		}
		comp, exists := reg.Components[name]
		if !exists {
			return fmt.Errorf("component '%s' not found in remote registry", name)
		}
		for _, dep := range comp.Dependencies {
			if err := resolveDeps(dep); err != nil {
				return err
			}
		}
		visited[name] = true
		queue = append(queue, name)
		return nil
	}

	if err := resolveDeps(targetComp); err != nil {
		return err
	}

	fmt.Printf("[GoStack CLI] Installing components: %s\n", strings.Join(queue, ", "))

	for _, name := range queue {
		comp := reg.Components[name]
		compDir := filepath.Join("templates", "components", name)
		if err := os.MkdirAll(compDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", compDir, err)
		}

		for _, file := range comp.Files {
			fileURL := "https://raw.githubusercontent.com/Charledeon77/gostack-components/main/" + file
			targetPath := filepath.Join(compDir, filepath.Base(file))

			fmt.Printf("[GoStack CLI] Downloading %s -> %s...\n", fileURL, targetPath)
			fResp, err := http.Get(fileURL)
			if err != nil {
				return fmt.Errorf("failed to download file %s: %w", file, err)
			}
			defer fResp.Body.Close()

			if fResp.StatusCode != http.StatusOK {
				return fmt.Errorf("failed to download %s: HTTP %d", file, fResp.StatusCode)
			}

			out, err := os.Create(targetPath)
			if err != nil {
				return fmt.Errorf("failed to create local file %s: %w", targetPath, err)
			}
			defer out.Close()

			if _, err := io.Copy(out, fResp.Body); err != nil {
				return fmt.Errorf("failed to write local file %s: %w", targetPath, err)
			}
		}
	}

	fmt.Println("[GoStack CLI] Compiling component assets...")
	compiler := ui.NewAssetCompiler(filepath.Join("templates", "components"), filepath.Join("cmd", "app"))
	if err := compiler.Run(); err != nil {
		return fmt.Errorf("asset compilation failed: %w", err)
	}

	fmt.Printf("[GoStack CLI] Component '%s' and dependencies successfully added and compiled!\n", targetComp)
	return nil
}

