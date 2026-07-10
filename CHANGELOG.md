# GoStack Framework Changelog & Architectural Ledger

This ledger details the step-by-step history, architectural justifications, and feature listings of the **GoStack** battery-included web framework. It acts as a comprehensive reference for how each component operates, fits into the ecosystem, and connects to the broader application lifecycle.

### GoStack Subsystem Brand Registry
| Brand | Subsystem | Package |
| :--- | :--- | :--- |
| **Citadel** | Application Bootloader & Unified Kernel | `framework/foundation` |
| **Anchor** | IoC Service Container & Dependency Injection | `framework/foundation` |
| **Navigator** | HTTP Router | `framework/http` |
| **Bridge** | Middleware Pipeline (Onion Architecture) | `framework/http` |
| **Crafter** | Query Builder & Reflective Hydrator | `framework/database` |
| **Conflex** | ORM Relationship Mapper | `framework/database` |
| **Traveller** | Database Migration Engine | `framework/database/migrate` |
| **Grapher** | Schema Builder (fluent column definitions) | `framework/database` |
| **Guard** | Authentication (session-based) | `framework/auth` |
| **Mory** | Generic-typed In-Memory Cache | `framework/cache` |
| **Sequence** | Background Job Queue | `framework/worker/queue` |
| **Spark** | Pub/Sub Event Dispatcher | `framework/foundation/events` |
| **GoMail** | SMTP Mailer | `framework/mail` |
| **Vault** | File Storage (Local + S3) | `framework/storage` |
| **Planner** | Cron Task Scheduler | `framework/worker/schedule` |
| **SocialHub** | OAuth Social Login | `framework/http/socialhub` |
| **GoDash** | Admin Panel & Sequence Monitor | `framework/admin` |
| **Tempose** | AOT UI Component Compiler | `framework/ui` |
| **Glide** | Client-Side Reactive Runtime (`gs-*` directives) | `framework/ui/glide.go` |
| **Gost** | CLI Command Runner | `framework/console` |
| **Contract** | Interface Definitions (driver contracts) | `framework/contract` |
| **GoCon** | Environment Config Manager (`.env` parser) | `framework/foundation/config` |
| **Validator** | Request Validation Engine | `framework/http` |
| **GowSocket** | Real-Time WebSocket Hub | `framework/http` |
| **Transios** | Localization & Translation Engine | `framework/foundation/lang` |
| **GoMon** | MongoDB Integration (NoSQL Document Store) | `framework/database` |
| **Nexus** | Neo4j Graph Database Integration | `framework/database` |
| **Aether** | Cassandra Wide-Column Database Integration | `framework/database` |

## [v1.0.9] - Local Development Hot-Reload Command & EventSource Orchestrator
*Released on 10 July, 2026*

### Developer Experience & Subprocesses
*   **Dev Command (`gost dev`)**: Introduced a local development server runner that performs initial component compilation, builds the application to a temporary executable (`tmp_server`), and runs it.
*   **Zero-Dependency File Watcher**: Implemented a background scanner using standard library polling (every 500ms) to check for template (`templates/components/`) and Go source changes, avoiding external Cgo/fsnotify dependencies.
*   **Subprocess Protection**: Implemented strict process killing and temporary file deletion on Unix and Windows platforms, preventing locked port errors when restarting.

### Foundation & UI Subsystems
*   **Auto-Registered SSE Route**: Registered `GET /__gostack_reload` route in `foundation/kernel.go` automatically when `APP_ENV != "production"`.
*   **Auto-Injected Browser Script**: Injected an EventSource client reloader block inside `ui.WriteMasterAssetBlock` automatically during non-production builds. The browser client automatically detects server restarts and refreshes when the server is back online.

---

## [v1.0.8] - UI Component CSS Scoping Engine Fix & Encapsulation Restore
*Released in July 2026*

### UI Compiler Scoping & Performance Recovery
*   **Encapsulation Restoration**: Corrected the styling compiler scoping prefix from the non-existent `gostack-root [gs-component="componentName"]` to the wrapper attribute selector `[gs-component="componentName"]`. This resolves a critical mismatch that broke CSS scoping out-of-the-box.
*   **Production & UX Protection**: Solved the production rendering bug where all scoped component styles were discarded by browsers. This eliminates layout breakage and styled component failures across client viewports.
*   **Single-Binary & Asset Bundle Scalability**: Restored GoStack's O(1) single-binary performance philosophy. Developers no longer need to bypass the compiler by linking raw static files manually, reducing HTTP connection overhead, network latency, and preventing Flash of Unstyled Content (FOUC).
*   **Style Engine Matching Performance**: Replaced arbitrary tag lookups with attribute-based sibling-descendant matches (`[gs-component="name"] selector`), allowing browser engines to leverage fast internal CSS hash tables for O(1) selector lookup during paint steps.
*   **Automated Scoping Verification**: Added unit test coverage `TestAssetCompiler_ScopeCSS` to prevent regression.

---

## [v1.0.7] - Transios Internationalization Subsystem & Preview CLI Cleanup
*Released in July 2026*

### Internationalization (Transios)
*   **Request-Scoped Engine**: Integrated request-scoped language resolver middleware intercepting Accept-Language, query strings, sessions, and cookies.
*   **Template Interpolation**: Built AOT-compiled local translation functions (`trans`, `transChoice`, `transRaw`) directly inside component render contexts.
*   **Pluralization Ranges**: Supported plural range selector gates (e.g. `{0} no items|[1,10] a few items|[11,*] many items`).
*   **Missing Key Warnings**: Added development warning flags to log missing translation keys in the console, falling back to raw keys in production.

### Component Preview Cleanup
*   **Decoupled Preview Server**: Removed the local preview subcommand and dependencies to make the GoStack core framework completely standalone.

### Subsystem Registry & Branding
*   **Subsystem Brand Names**: Registered official brand definitions for **Anchor** (IoC container), **Conflex** (relationship mapper), **Validator** (validation middleware), and **GowSocket** (WebSockets).
*   **Interactive CLI Lookup (`gost lang:search`)**: Created a command utilizing the standard Unicode CLDR libraries and dynamic flag generators to search language codes and emojis directly in the console.

## [v1.0.6] - Unified Glide Reactivity Engine & Automatic Layout Injection
*Released in July 2026*

### Client-Side Reactivity Layer & Glide Engine
*   **Unified Reactive Runtime**: Merged competing runtimes and deprecated the simpler `GoStackRuntimeJS` class runtime in `framework/http/runtime.go` to avoid legacy code bloat. Exclusively standardise on the Proxy-based **Glide** engine in `framework/ui/glide.go`.
*   **Breaking Syntax Cleanup**: Broke backward compatibility to clean up code baggage. Directives are unified:
    *   **`gs-data`** (replaces `gs-state`) for state declarations.
    *   **`gs-click`** (replaces `gs-on:click`) for click events.
*   **Core Reactivity Fixes**:
    *   *Expression Sandbox*: Rewrote statements execution using `with(scope)` sandbox without strict mode to correctly update primitive values (like `count++`) directly on the reactive state.
    *   *Nested Model Bindings*: Supported dot-notation keys (e.g., `gs-model="form.email"`) to bind nested values correctly.
    *   *Deep Array Reactivity*: Trapped array methods (`push`, `pop`, `splice`, etc.) to trigger visual DOM updates automatically on array mutations.
    *   *Native Event Access*: Passed triggering browser event inside expressions as `$event` and `event`.
*   **Premium Svelte & Alpine-Inspired Features**:
    *   *`gs-init`*: Custom initialization code executed automatically on component hydration, with `$el` access.
    *   *`gs-persist`*: Automatic load/save of state values directly to and from browser `localStorage`.
    *   *`gs-intersect`*: IntersectionObserver-backed viewport detection for trigger logic (e.g. lazy-loading, infinite scrolling).
    *   *`gs-transition`*: Hardware-accelerated fade-and-scale animations applied automatically when toggling elements.
    *   *`gs-effect`*: Reactive side-effects that execute automatically whenever any dependency state changes.
    *   *`gs-ref` & `$refs`*: Direct DOM element references to bypass manual query selectors.
    *   *`$dispatch`*: Custom event dispatching to allow parent/child components to communicate easily.
*   **Global Modal Helpers**: Exposed global `window.GoStack.showModal(id)` and `window.GoStack.closeModal(id)` helpers natively.
*   **Component Updates**: Updated all framework default components (`counter`, `button`, `modal`) to use the new standard syntax.

### HTTP View Engine
*   **Automatic Layout Asset Injection**: Upgraded `Tempose.Render` in `framework/http/tempose.go` to automatically detect `</head>` in full-page renders and inject the compiled stylesheet and Glide runtime automatically right before it.

---

## [v1.0.5] - Schema Builder Raw SQL Migration Support
*Released in July 2026*

### Database migrations & DDL
*   **Raw SQL in Migrations**: Added the `Exec` and `Raw` methods on the schema `Builder` struct, enabling migration files to run custom transactional SQL queries like `ALTER TABLE` or `CREATE INDEX`.
*   **Verification Tests**: Covered compilation and query execution logic via `TestBuilderExecAndRaw`.

---

## [v1.0.4] - IoC Container Kernel, Redis Sessions & Multi-Channel Notifications
*Released in July 2026*

### IoC Container Core Integration
*   **Container-driven Kernel**: Refactored the core `NewKernel` to inject a service `Container` rather than a direct database adapter. Dependencies like `"db"` and `"tempose"` are now resolved dynamically.
*   **Entrypoints Update**: Bootstrapped `cmd/app/main.go` and `_examples/app/main.go` to pass the dependency container.

### Persistence & Session Store
*   **Redis-Backed Session Store**: Added `session_store_redis.go` implementing the `contract.SessionStore` interface to persist sessions securely in Redis.
*   **Tests**: Verified session loading, saving, and deletion using `miniredis` in `session_store_redis_test.go`.

### Multi-Channel Notification System
*   **Unified Notification Engine**: Introduced the `framework/notification/` package to decouple notification structures from delivery channels.
*   **Mail Channel**: Configured `MailChannel` for SMTP email alerts.
*   **Database Channel**: Configured `DatabaseChannel` to store notification logs inside a SQL-backed `notifications` table.

---

## [v1.0.3] - Security Hardening & Extensions Ecosystem
*Released in July 2026*

### Critical Security Hardening
*   **XSS Protection**: Integrated automatic HTML escaping in the Tempose compiler (`ui.Escape`) to sanitize all dynamic template rendering values by default.
*   **Admin Panel Auth**: Secured `/admin` routes with `LocalOnlyGuard` limiting access to localhost by default, with custom pluggable `AuthGuard` support for production.
*   **SQL Injection & Stability**: Enforced alphanumeric allowlists (`isSafeIdentifier`) on table/column name evaluations within the `IsUnique` validator. Added type assertion checks on standard SQL query results to prevent panics on non-standard drivers.
*   **CSRF Cookie Flags**: Configured secure cookie validation checks for enhanced browser-side token security.

### HTTP Routing & Middleware
*   **Trailing Slash Normalization**: Added `TrailingSlashRedirectMiddleware` to automatically redirect requests with trailing slashes, ensuring SEO consistency.
*   **405 Method Dispatcher Fix**: Resolved a suffix-matching bug on HTTP 405 checks to prevent router panics on long URLs.
*   **Auth Throttle rate-limiting**: Integrated brute-force and credential stuffing protections on authentication endpoints.

### Official Extensions Ecosystem
*   **Plug-and-play extensions isolation**: Isolated optional components to the `gostack-extensions` warehouse.
*   **MFA Extension (`mfa/v1.0.0`)**: Standard TOTP secret generation, base64 QR code URLs, and passcode verification.
*   **RBAC Extension (`rbac/v1.0.0`)**: Roles, permissions, route guards, and custom resolver callbacks (`SetRoleResolver`) for MongoDB and Cassandra engines.

---

## GoStack v1.0.2 — Enterprise Web Utilities & Hardening
*Released in July 2026*

GoStack v1.0.2 is a major upgrade that transitions the framework from a lightweight full-stack prototype into a robust, production-grade, and enterprise-ready application platform. This release introduces advanced database transactional control, recursive eager loading relationships, secure rate-limiting, timezone-aware scheduling, comprehensive MIME mail capabilities, and robust browser-native security hardening—all while strictly adhering to GoStack's zero-dependency standard library philosophy.

### Database & ORM (Crafter & SQL Adapter)
*   **Nested Transaction Support (SQL Savepoints)**: The global `Transaction(db, fn)` wrapper now natively supports nested transaction closures. Passing an active transaction (`contract.Tx`) automatically triggers SQL `SAVEPOINT`, `RELEASE SAVEPOINT`, and `ROLLBACK TO SAVEPOINT` commands.
*   **Database Connection Pooling**: Added connection pool limit controls (`SetMaxOpenConns`, `SetMaxIdleConns`, and `SetConnMaxLifetime`) to `contract.ConnectionPooler` and implemented them in the SQL adapter.
*   **Advanced Recursive Eager Loading**: Supports deep recursive relationship loading via `With("Relation.NestedRelation")` using type-agnostic key matching.
*   **New Eager Relationships**: Fully implemented `HasOne`, `ManyToMany`, and `HasManyThrough` relationship loaders.

### Security, Routing & Middleware (Navigator & Guard)
*   **Brute-Force Auth Rate Limiter (AuthThrottle)**: Added a sliding-window rate-limiting middleware for auth endpoints using a composite Client IP + Credential key. Returns standard headers (`Retry-After`, `X-RateLimit-*`) and `429 Too Many Requests`.
*   **Route Conflict Detection**: The HTTP router now scans and prints warning logs at bootstrap if there are duplicate method + path registrations.
*   **Native DOMParser HTML Sanitization**: Upgraded the client-side Glide `gs-html` engine to use the browser-native `DOMParser` API instead of fragile regular expressions, safely stripping `<script>` tags, inline event attributes (`on*`), and `javascript:` URLs.

### UI Engine & AOT Compiler (Tempose & Glide)
*   **Stateful Filter Parser**: Rewrote the template pipe filter compiler to safely parse pipes (`|`) and commas (`,`) even when they are enclosed inside quotes or parentheses (e.g. `{{ .Text | replace("|", "-") }}`).
*   **Standard Filters**: Added support for `date`, `truncate`, `slugify`, `plural`, `upper`, and `lower` filters.
*   **Named Component Slots**: Added Svelte-style component slot support (`<slot name="...">` and `slot="..."`), translated to Blade-style `@yield` / `@section` blocks at compile-time.
*   **SSE Hot-Reload Watcher**: Added a standard library-based file change watcher using folder polling and Server-Sent Events (SSE) to auto-reload browsers during development.

### Timezone-Aware Cron Scheduler (Planner)
*   **Timezone Support**: Task schedules can now be mapped to specific IANA locations (e.g. `.Timezone("Europe/London")`).
*   **Overlap Prevention**: Added `.WithoutOverlapping()` utilizing atomic state markers to skip executions if a previous job run is still active.
*   **Launch-on-Boot**: Added `.RunOnBoot()` to execute scheduled tasks once immediately when the scheduler starts.

### MIME-Compliant Mailer & i18n Pluralization (GoMail & Lang)
*   **Attachments & Advanced Envelopes**: Extended GoMail to support BCC, Reply-To, binary attachments, and automatic construction of multipart MIME formats with RFC 2045 compliant base64 line wrapping. Includes a fluent `MessageBuilder`.
*   **Translator Choice Pluralization (TransChoice)**: Implemented localized pluralization templates with exact matches and range guards (e.g., `{0} No items|{1} One item|[2,*] :count items`).

### CLI Code Scaffolding (Gost)
*   **New CLI Generators**: Added 4 new generators to the CLI:
    *   `gost make:event <Name>`
    *   `gost make:job <Name>`
    *   `gost make:policy <Name>`
    *   `gost make:provider <Name>`
*   **Automated Formatting**: Stubs are automatically run through `gofmt` to verify syntax and ensure perfect coding standards.

---

## [v1.0.0] - Initial Framework Release
*Released in June 2026*

### Core Subsystems & CLI Scaffolding
*   **Global CLI Tool (`cmd/gost`)**: Created a dedicated, lightweight entry point compiled and installed once globally using Go's package manager (`go install`).
*   **Smart Wrapper Delegation**: Programmed the global `gost` binary to act as a pass-through proxy. Running `gost serve`, `gost migrate`, or any generator command from inside a project directory dynamically forwards the request to the project-local compiler.
*   **Pure Go Template Downloader**: Downloads scaffolding templates over secure HTTPS ZIP archives and extracts natively in Go, eliminating external Git client dependencies for end-users.
*   **Package Resolution Fix (`framework/console/serve.go`)**: Ensured `gost serve` compiles the full package directory via `go run ./cmd/app` so generated component registrations (`RegisterComponents`) resolve cleanly.
*   **Smarter Dependency Pruning (`pruneUnusedDriverDeps`)**: Automatically removes unselected database drivers from `go.mod` during project setup to keep application dependency trees minimal.
*   **Guard Authentication Wizard & Generator**: Integrated interactive setup prompts and automated scaffolding for login/register views, user models, session middleware, and database migrations.
*   **Root Documentation**: Included full getting-started guides in the repository root for complete codebase context awareness.

---

## [Planned Milestone] - Roadmap: Core Web Utilities
*Proposed for Upcoming Releases*

### 1. API Rate Limiting (Throttling Middleware)
*   **Purpose**: Protect web application endpoints and APIs from denial-of-service (DoS) attacks and brute-force attempts.
*   **Design**: Implement a memory-backed rate limiter middleware inside `framework/http` using a sliding window algorithm. It will allow developers to restrict requests based on client IP or authenticated session, returning `429 Too Many Requests` when the threshold is crossed.

### 2. Multi-Language Support (Localization / i18n)
*   **Purpose**: Enable developers to localize application text and response messages for global users.
*   **Design**: Introduce a localization manager that loads key-value translation files (YAML/JSON) and dynamically selects languages based on the HTTP `Accept-Language` header or user preferences.

---

## [Milestone 11] - Multi-Database Architecture & CLI Scaffolding
*Added in June 2026*

### Features & Implementation
*   **GoMon (`framework/database/mongodb.go`)**: MongoDB integration subsystem, exposing native `*mongo.Client` as `gostack.Mongo`.
*   **Nexus (`framework/database/neo4j.go`)**: Neo4j graph database integration subsystem, exposing native `neo4j.DriverWithContext` as `gostack.Neo4j`.
*   **Aether (`framework/database/cassandra.go`)**: Cassandra wide-column database integration subsystem, exposing native `*gocql.Session` as `gostack.Cassandra`.
*   **Relational SQL Dialect Expansion**: SQLite (powered by pure-Go `modernc.org/sqlite` for zero-CGO setup) and CockroachDB support integrated natively into **Crafter** (Querying), **Traveller** (Migrations), and **Grapher** (Schema Compilation).
*   **Interactive Database Scaffolding Selection**: Rewrote `make_new.go` to provide an interactive multi-database scaffolding wizard. It automatically injects connection code, DSNs, and package dependencies into `go.mod`, `main.go`, and `.env` based on user selection.

### Architectural Decisions (The "Why")
*   **Purpose**: Enable developers to build any application architecture imaginable—whether relational SQL, NoSQL document-based, distributed wide-column, or highly connected graph databases.
*   **Philosophy**: Modular self-containment. GoStack avoids compile-time dependencies on database client packages (like MongoDB, Neo4j, or Cassandra) unless the developer explicitly imports them, keeping binaries lightweight.

---

## [Milestone 10] - Gost: CLI Scaffolding (`gostack new`)
*Added in June 2026*

### Features & Implementation
*   **`gostack new <project-name>`** (Gost): Built-in CLI scaffolding command (`framework/console/make_new.go`). Clones the GoStack base repository, cleans Git history, dynamically copies `.env.example` to `.env`, generates a secure random 32-byte `APP_KEY`, renames `go.mod` to the new project namespace, and automatically updates all internal imports.

### Architectural Decisions (The "Why")
*   **Purpose**: Enable developers to initialize a brand new, fully configured GoStack project with zero friction.
*   **Philosophy**: Developer ergonomics. A framework should be instantly usable. Manually cloning, renaming modules, and setting up environment files violates the "batteries included" promise.
*   **Choice**: Automated the repository cloning and string replacement entirely within Go, meaning developers only need the **Gost** CLI installed globally to spawn infinite new projects.

---

## [Milestone 9] - Tempose: Semantic UI Engine & Glide Naming
*Added in June 2026*

### Features & Implementation
*   **Opt-in Semantic Core CSS (`framework/ui/core_css.go`)** (Tempose): A pre-styled, Pico-inspired semantic CSS base injected into the `gostack_components_gen.go` master asset block.
*   **`[gs-css]` Attribute Selector**: Opt-in styling engine. Elements and containers only inherit GoStack core defaults if the `gs-css` HTML attribute is present.
*   **Glide Named**: The client-side `gs-*` directive runtime engine (`framework/http/runtime.go`) is officially branded **Glide**.
*   **Documentation Correction**: Removed misleading "Hot-Reloading" claim from the README and removed old, incorrect brand names (SparkORM, Glide as old alias).

### Architectural Decisions (The "Why")
*   **Purpose**: Provide 100+ components with a stunning, zero-dependency baseline so they do not require Node.js or Tailwind.
*   **Philosophy**: Developer control and the "Zero Dependency" rule. The `gs-css` opt-in strategy ensures the framework doesn't fight custom designs while still providing "Batteries Included" pre-styled defaults.
*   **Choice**: Baked the semantic base directly into **Tempose** (`framework/ui`) over scaffolding it into the developer's workspace.

---

## [Milestone 8] - Sequence, Spark, Vault, WebSockets, Mory
*Added in June 2026*

### Features & Implementation
*   **Sequence (`framework/queue`)**: Memory-backed async job processing system with background worker pools.
*   **Spark (`framework/events`)**: Concurrent-safe Pub/Sub event network supporting synchronous and asynchronous listener execution.
*   **Vault (`framework/storage`)**: Local disk storage driver with directory traversal protection and standardized Put/Get/Delete interfaces.
*   **WebSockets (`framework/http/websockets.go`)**: Real-time bi-directional communication hub.
*   **Mory (`framework/cache`)**: High-performance, zero-dependency in-memory cache featuring a Type-Safe Generic API wrapper (`Get[T any]`).

### Architectural Decisions (The "Why")
*   **Purpose**: Elevate GoStack from a basic web framework to a fully enterprise-ready, batteries-included toolkit.
*   **Philosophy**: Provide these tools natively. Use Go 1.18+ Generics strategically in **Mory** to maximize type safety without cluttering the underlying **Contract** driver interfaces.
*   **Choice**: Zero-dependency standard library only. **Sequence**, **Spark**, **Vault**, and **Mory** are built from scratch using Go concurrency primitives (`sync.RWMutex`, `sync.WaitGroup`, channels).

### Ecosystem Integration & The Bigger Picture
*   **Lifecycle Boundary**: **Sequence** and **Spark** operate in background goroutines parallel to HTTP requests. **Vault** and **Mory** integrate directly into controller logic.
*   **Integration with Contract**: Every subsystem binds to a strict **Contract** interface in `framework/contract/contracts.go`, ensuring default implementations can be swapped for distributed variants (Redis, AWS S3, RabbitMQ) without changing business logic.

## [Milestone 7] - Citadel: Service Driver Registry (`framework/foundation`)
*Added in Prior Releases*

### Features & Implementation
*   **Thread-Safe Constructor Registry (Citadel `driver_registry.go`)**: Implements `Register(name, DriverFunc)` and `GetDriver(name)` guarded by a `sync.RWMutex`.
*   **Dynamic Constructor Signature (`DriverFunc`)**: Defines a generic constructor footprint `func(dsn string) (interface{}, error)` allowing database adapters to be resolved by name configuration.

### Architectural Decisions (The "Why")
*   **Purpose**: Decouple the core **Citadel** codebase from specific SQL drivers so that driver registration remains pluggable.
*   **Philosophy**: Modular self-containment. GoStack aims to be lightweight, compile fast, and avoid importing unused third-party dependencies.
*   **Choice**: A thread-safe constructor map mapping strings to constructor functions over hardcoding drivers inside the SQL adapter.

### Ecosystem Integration & The Bigger Picture
*   **Lifecycle Boundary**: Runs during package `init()`. When the developer imports their chosen driver adapter, it registers its connection constructor with the central **Citadel** registry.
*   **Integration with Contract**: Ensures **Crafter** and **Traveller** are provided with a valid database adapter resolving the correct SQL dialect.

---

## [Milestone 6] - Traveller & Grapher: Database Migration Engine
*Added in June 2026*

### Features & Implementation
*   **Grapher — Fluent Schema Column Builder (`framework/schema/table.go`)**: Type-safe columns builder supporting `ID()`, `String(name)`, `Integer(name)`, `Boolean(name)`, `Text(name)`, and `Timestamps()`.
*   **Grapher — Driver-Aware Dialect Compiler**: Translates column definitions into MySQL and PostgreSQL DDL syntax dynamically.
*   **Grapher — Schema DDL Runner (`framework/schema/builder.go`)**: Executes compiled DDL statements (`Create` and `Drop`) atomically inside an active transaction.
*   **Traveller — Registry & Migrator Runner (`framework/migrate/migrator.go`)**: Self-registration, transactional runner (`Run()`), and rollback runner (`Rollback()`).

### Architectural Decisions (The "Why")
*   **Purpose**: Protect database schema mutations from compilation bugs and run-time mismatches by representing schemas directly as Go code.
*   **Philosophy**: Developer ergonomics and compile-time safety via **Grapher**. No raw SQL files or YAML configurations.
*   **Choice**: Go function registration with a **Grapher** Schema Builder wrapper over external config files.

### Ecosystem Integration & The Bigger Picture
*   **Lifecycle Boundary**: **Traveller** executes during the framework boot phase, immediately after **Citadel** service resolution and **GoCon** config compilation, but before **Navigator** binds to socket ports.

---

## [Milestone 5] - Crafter: Query Builder & Reflective Hydrator
*Added in Prior Releases*

### Features & Implementation
*   **Crafter Query Builder (`builder.go`)**: Fluent SQL generator supporting `Where` chaining.
*   **Driver Parameterizer**: Detects database driver state and parameterizes SQL using MySQL `?` or PostgreSQL `$1, $2` placeholders, preventing SQL injection.
*   **Crafter Reflective Hydration Engine (`hydrator.go`)**: Reflection-based mapping utility (`Hydrate`) that matches database records with struct field tags (`db:"fieldname"`).

### Architectural Decisions (The "Why")
*   **Purpose**: Abstract raw SQL query syntax from the application layer, returning populated domain models directly.
*   **Philosophy**: Secure-by-default, sql-injection proof, type-safe mapping via **Crafter**.
*   **Choice**: Structured mapping tags (`db:"name"`) let us use native structs rather than arbitrary maps.

### Ecosystem Integration & The Bigger Picture
*   **Lifecycle Boundary**: Serves requests during standard controller execution.
*   **Integration with Contract**: Utilizes the injected `contract.Database` instance from **Citadel** to run query commands.

---

## [Milestone 4] - Glide: Client-Side Local Reactivity
*Added in Prior Releases*

### Features & Implementation
*   **Glide Reactive Engine (`framework/http/runtime.go`)**: A micro-JS reactivity framework loaded dynamically through the layout `GoStackRuntimeJS`.
*   **Directive Evaluators**: Parses and intercepts client DOM tags:
    *   `gs-state`: Sets local state components within the browser.
    *   `gs-click` & `gs-on:click`: Triggers event handling routines locally.
    *   `gs-text`: Performs reactive text substitutions.
    *   `gs-show`: Manages element visibility.
*   **Zero Server Roundtrips**: Performs visual updates on state changes immediately in the user's browser.

### Architectural Decisions (The "Why")
*   **Purpose**: Provide an interactive React/Alpine-style interface inside compiled HTML templates.
*   **Philosophy**: Reduce latency and backend overhead by performing UI rendering changes on the client side via **Glide**.
*   **Choice**: Using simple HTML attribute parsing (`gs-*` directives) removes the need for large SPA compilation tools (Vite, Webpack).

### Ecosystem Integration & The Bigger Picture
*   **Lifecycle Boundary**: **Glide** executes inside the browser DOM after **Tempose** has rendered the templates server-side.
*   **Front-Backend Symbiosis**: **Tempose** builds it. **Glide** runs it.

---

## [Milestone 3] - Tempose: Component & Template Compiler
*Added in Prior Releases*

### Features & Implementation
*   **Tempose CSS Scoper (`framework/ui/compiler.go`)**: Automatically isolates component CSS definitions during build time by wrapping selectors inside `gostack-root [gs-component="componentName"]`.
*   **Tempose Layout Component Compiler**: Scans component directories, compiling `.html` templates, styles, and script assets into static rendering instructions stored inside `cmd/app/gostack_components_gen.go`.
*   **Reflection Evaluator (`framework/ui/registry.go`)**: Inspects runtime data shapes, extracting model parameters and bindings inside compiled templates dynamically.

### Architectural Decisions (The "Why")
*   **Purpose**: Enable reusable, encapsulated frontend components (styles, markup, script) inside native Go views.
*   **Philosophy**: Cohesive, modular views. Encapsulation prevents global CSS style leakage across components.
*   **Choice**: Ahead-of-Time (AOT) compilation to raw Go string writes was chosen over runtime file parsing to avoid disk I/O latency in production.

### Ecosystem Integration & The Bigger Picture
*   **Lifecycle Boundary**: **Tempose** runs at development/compile build time. The compiled rendering functions are registered at server launch.
*   **Integration with Navigator**: When a route handler invokes `ctx.HTML()`, **Navigator** triggers **Tempose** compiled component structures in memory.

---

## [Milestone 2] - Navigator, Bridge & Context
*Added in Prior Releases*

### Features & Implementation
*   **Navigator (`framework/http/router.go`)**: Fast HTTP multiplexer supporting verb registration (GET, POST), route groupings, prefixes, and group-scoped middlewares.
*   **Bridge — Request Onion Context (`context.go`)**: Custom context struct wrapping `http.ResponseWriter` and `*http.Request`. Provides `.JSON()`, `.HTML()`, parameter queries, and a `.Next()` execution chain.
*   **Bridge — Standard Middleware Stack (`middleware.go`)**: Core CORS, Recovery, and Logger layers.

### Architectural Decisions (The "Why")
*   **Purpose**: Manage request orchestration and routing efficiently via **Navigator**.
*   **Philosophy**: Clean pipeline interceptors. **Bridge** wraps the response pipeline in an Onion context that simplifies writing custom middlewares.
*   **Choice**: Middleware wrappers returning `error` instead of managing raw write bounds allows unified error handling.

### Ecosystem Integration & The Bigger Picture
*   **Lifecycle Boundary**: **Navigator** operates as the runtime entry point for incoming client requests.
*   **Integration with Citadel**: Middlewares and routes resolve core adapters dynamically from the **Citadel** container context.

---

## [Milestone 1] - Citadel: DI Container & Bootstrap
*Added in Prior Releases*

### Features & Implementation
*   **Citadel DI Container (`container.go`)**: Fast, thread-safe dependency injection registry.
*   **Lifecycle Scopes**:
    *   `BindSingleton`: Instantiates the struct pointer once and caches it.
    *   `BindTransient`: Materializes a new memory reference on every request.
*   **Concurrency Guard**: Shielded by a standard `sync.RWMutex` to prevent multi-threaded map corruptions.
*   **Bootstrap Kernel (`application.go`, `kernel.go`)**: Manages service provider registration, lifecycle configurations, and **GoCon** environment configuration loaders.

### Architectural Decisions (The "Why")
*   **Purpose**: Wire, resolve, and manage the lifecycle of database engines, loggers, and business controllers via **Citadel**.
*   **Philosophy**: Inversion of control. Loose coupling via **Contract** allows adapters to be swapped seamlessly.
*   **Choice**: A reflection-free factory registration approach keeps dependency resolution fast and compile-time safe.

### Ecosystem Integration & The Bigger Picture
*   **Lifecycle Boundary**: **Citadel** runs first when the Go application launches (`main.go`). It acts as the registry backbone for all other subsystems.
*   **Application Coordinator**: Integrates **GoCon**, database adapters, **Tempose** template registries, and **Navigator** route definitions.

---

## Middleware & Validation Libraries

### `framework/validation` (Bridge Layer)
*   **Cached Struct Validator (`middleware.go`)**: Validates request parameters by type-asserting inputs to a `Validator` interface. Uses a `sync.Map` type cache to bypass reflection overhead on subsequent HTTP validation calls. Short-circuits with `422 Unprocessable Entity` on validation failure.
*   **Integration with Bridge Pipeline**: Intercepts requests before they reach the controller, decoding payloads and checking validations. This prevents invalid records from ever reaching **Crafter** query builders or **Vault** storage layers.
