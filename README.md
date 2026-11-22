<!-- Logo -->
<p align="center">
  <img src="assets/images/gostack-3d-logo.png" alt="GoStack Logo" width="100%">
</p>

<!-- Badges -->
<p align="center">
  <img src="https://img.shields.io/badge/License-Apache%202.0-orange?logo=apache" />
  <img src="https://img.shields.io/badge/version-1.0.23-1FB249?logo=github" />
  <img src="https://img.shields.io/badge/build-passing-brightgreen?logo=githubactions" />
  <img src="https://img.shields.io/badge/tests-passing-brightgreen?logo=githubactions" />
  <img src="https://img.shields.io/badge/coverage-80%25-yellowgreen?logo=codecov" />
  <img src="https://img.shields.io/badge/Go%20Report-A%2B-8a2be2?logo=goreportcard" />
  <img src="https://img.shields.io/badge/docs-available-blue?logo=readthedocs" />
  <img src="https://img.shields.io/badge/Platform-Windows%20%7C%20Linux%20%7C%20macOS-lightgrey?logo=googlecast&logoColor=F4842D" />
  <a href="https://pkg.go.dev/github.com/charledeon77/gostack-framework"><img src="https://img.shields.io/badge/go.dev-reference-007d9c?logo=go" alt="Go Reference" /></a>
  <img src="https://img.shields.io/badge/PRs-welcome-brightgreen?logo=github" />
</p>

<p align="center">
  <b>GoStack</b> — <i>A modern fullstack web framework for scalable web development in Go.</i>
</p>

# 📝 The Philosophy & Vision of GoStack  
## The Problem GoStack Solves  

Modern Web development in Go is fragmented by design. A typical Go Web project looks like:

1. **Router** from a third-party library (`gin` or `gorilla/mux`)
2. **ORM** from a third-party library (`gorm` or `ent`)
3. **Query Builder** from a third-party library (`sqlx` or `sqlc`)
4. **Migration tool** from a third-party library (`golang-migrate` or `goose`)
5. **Authentication system** from a third-party library (`authcore` or `goth`)
6. **Social authentication** from a third-party library (`golang.org/x/oauth2`)
7. **Background job queue** from a third-party library (`machinery` or `neoq`)
8. **Cron scheduler** from a third-party library (`robfig/cron`)
9. **Event dispatcher** from a third-party library (`EventBus`)
10. **WebSocket library** from a third-party library (`gorilla/websocket` or `coder/websocket`)
11. **File storage abstraction** from a third-party library (`aws-sdk-go` or `go-cloud`)
12. **Caching layer** from a third-party library (`go-redis`)
13. **Mail delivery** from a third-party library (`gomail`)
14. **CLI tooling** from a third-party library (`cobra`)
15. **Configuration management** from a third-party library (`viper`)

And that's just the backend. If you need a modern frontend:

16. **Frontend** built entirely separately (`React`, `Vue` or `Svelte`) in a different language, with its own build tools (`vite`/`webpack`), its own routing, its own state management, its own ecosystem — and its own 6 million package dependency hell.

**The result:** Sixteen third-party ecosystems. Sixteen mental models. Sixteen failure points — just to build one application.
**The GoStack Answer:** One Language. One Binary. One Mental Model.

#
# The Disruptive Paradigm Shift: GOSTACK

**GoStack** is the first complete end-to-end framework solution for building **FullStack Web applications** in Go. It handles:

01.) ✅ **Kernel Bootloader** *(Citadel)* — Unified service registration and framework orchestration boot system.

02.) ✅ **Service Container** *(Anchor)* — Fast, lock-shielded, thread-safe Dependency Injection & IoC container.

03.) ✅ **HTTP Router** *(Navigator)* — Ultra-fast, zero-allocation routing with automated path parameter binding.

04.) ✅ **Middleware Pipeline** *(Bridge)* — Onion-style middleware pipeline for request filtering and interception.

05.) ✅ **Query Builder & Hydrator** *(Crafter)* — Active Record query builder that hydrates database results into Go structs.

06.) ✅ **ORM Relationships** *(Conflex)* — Native resolver mapping HasOne, HasMany, and BelongsTo database relations.

07.) ✅ **Database Migrations** *(Traveller)* — Transaction-shielded, versioned database schema migration runner.

08.) ✅ **Schema Builder** *(Grapher)* — Fluent programmatic definition for columns, indexes, and constraints.

09.) ✅ **Authentication** *(Guard)* — Fully-baked session/token authentication with native CSRF protection.

10.) ✅ **In-Memory Cache** *(Mory)* — Generic-typed key-value caching layer with local and Redis memory adapters.

11.) ✅ **Background Queues** *(Sequence)* — Deferred job queue worker supporting delays, retries, and job chaining.

12.) ✅ **Event Dispatcher** *(Spark)* — Synchronous and asynchronous Pub/Sub event communication system.

13.) ✅ **SMTP HTML Mailer** *(GoMail)* — Rich HTML email formatting, template engines, and attachment delivery.

14.) ✅ **File Storage** *(Vault)* — Path-traversal secure storage engine supporting local and AWS S3 environments.

15.) ✅ **Notification Router** *(Alert)* — Decoupled, multi-channel alerting (Email, Database, SMS) dispatch engine.

16.) ✅ **Cron Task Scheduler** *(Planner)* — Native, in-app cron job scheduler replacing external system crontabs.

17.) ✅ **Social Logins** *(SocialHub)* — Plug-and-play OAuth2 login flow with Google, GitHub, and custom providers.

18.) ✅ **Admin Dashboard** *(GoDash)* — Auto-generated backoffice interface with real-time job and system monitors.

19.) ✅ **AOT UI Compiler** *(Tempose)* — Ahead-of-Time compiler translating HTML/CSS/JS components directly into Go source code.

20.) ✅ **Reactive Client Runtime** *(Glide)* — Zero-dependency DOM reactivity engine (Alpine/Svelte equivalent) running without Node.js.

21.) ✅ **CLI Assistant** *(Gost)* — Scaffolding runner for database models, migrations, and controller templates.

22.) ✅ **Driver Contracts** *(Contract)* — Interchangeable interface contracts decoupling framework drivers from core services.

23.) ✅ **Config Manager** *(GoCon)* — Clean, strongly-typed environment `.env` configuration file loader.

24.) ✅ **Request Validation** *(Validator)* — Declarative rule-based payload validation middleware for requests.

25.) ✅ **Real-Time WebSockets** *(GowSocket)* — Bi-directional, multi-group WebSocket communication hub.

26.) ✅ **Translation Engine** *(Transios)* — Localization manager supporting language dictionaries and formatting templates.

*(Note: In v1.0.12, database-paradigm specific engines GoMon, Nexus, and Aether were transitioned into official isolated packages in `gostack-extensions` to preserve core lightweight boundaries.)*

---

### 📦 Official Extensions (Isolated Packages)
To respect core design rationale and keep database/architecture boundaries perfectly clean, complex plug-and-play extensions and alternative database paradigms are managed in isolated packages:
*   🔑 **Multi-Factor Auth** *(MFA)* — RFC 6238 TOTP validation and QR code generation wrappers.
*   🛡️ **Role-Based Access Control** *(RBAC)* — Relational & NoSQL database-agnostic role permission middlewares and policy resolvers.
*   🍃 **MongoDB Document Store** *(GoMon)* — Active Record BSON hydration engine mapping MongoDB collections to Go models.
*   🕸️ **Neo4j Graph Database** *(Nexus)* — Native Cypher query mapper and relation graph nodes for Neo4j database instances.
*   ⚡ **Cassandra Wide-Column** *(Aether)* — Distributed CQL query mapper and table optimizer for Apache Cassandra database structures.

---

Everything in **One binary, One mental model, One language**.   

It's the equivalent of what **Laravel** gave **PHP**, what **Django** gave **Python**, and what **Rails** gave **Ruby** — instead of assembling an app from 10 independent packages, you get one coherent framework where every layer knows about every other layer by design.

**Go** developers have never had this — until now (June 2026).

# The Five (5) Core Pillars
## 1.) 🔋 Batteries Included:

**GoStack** ships with everything — routing, ORM, migrations, auth, caching, queues, WebSockets, scheduling, storage, events, admin dashboard, and a frontend compiler — all in one module, which means no abandonware risk, no integration hell, no context switching between Go and Node.js, no tracking ten different package versions, and no JavaScript frontend forced upon you, because everything is natively integrated, maintained as one coherent ecosystem, and compiled into a single binary, so you don't assemble a stack — you just build.

#
## 2.) 🔄 No Context Switching (Go-Native Fullstack)

The most radical part of GoStack: the frontend is Go. Components are `.html`, `.css`, and `.js` files that **Tempose** compiles into Go string literals at build time — no separate dev server, no proxy configuration, no CORS headaches.

**Client-side reactivity** is provided by **Glide**, a micro frontend engine inspired by Alpine.js, embedded directly in your binary. With Glide's `gs-*` directive system, you add interactivity using declarative attributes right in your HTML — `gs-click`, `gs-model`, `gs-show`, `gs-for`, and more. No `useState`, no `useEffect`, no virtual DOM, no build step. Just HTML with superpowers.

The result is a fullstack application with no Node.js, no `npm`, no `package.json`, no `node_modules`, no Webpack, no Vite, no Babel — just `go build`.   

**One language. One binary. No separate frontend toolchain**.

#
## 3.) 📘 Easy to Learn and Use (One Mental Model)

When you learn a new framework, you're not just learning syntax — you're learning a way of thinking. Most frameworks teach you ten different ways of thinking: one for data, one for background tasks, one for real-time updates, one for configuration. Every new feature demands a new mental context switch.

GoStack doesn't do this — everything in GoStack works the way you'd naturally expect it to work. The pattern you learn on day one is the same pattern you use every time forward. There are no hidden layers where the framework suddenly behaves differently because you've crossed some invisible complexity threshold.

With GoStack, there's just one simple pattern that never changes, no matter how big your idea gets.

#
## 4.) 🔒 Secure by Default

Most frameworks assume you'll remember to lock the door after you build the house. GoStack assumes you won't — so it locks every door for you, before you even lay the first brick.

With GoStack, security isn't an afterthought you scramble to add at the end of a project. It's baked into the foundation from the very first line. Every request is questioned. Every file access is contained. Every database query is protected unless you explicitly say otherwise. Every user action needs permission. Nothing is trusted by accident.

This means you can build with speed and sleep with peace of mind. Beginners don't have to become security experts just to launch their first app. Investors don't have to worry about the hidden cost of a breach. And developers don't have to spend their nights wondering if they forgot to sanitize that one input.

GoStack doesn't wait for you to make a mistake — it prevents the mistake from ever happening. Security isn't a feature you add later. It's how the framework works, baked in and always on.

## 5. ⚡ Developer Experience as a First-Class Feature

For years, developers in other ecosystems have enjoyed something Go never had: a complete, integrated framework where every piece works with every other piece out of the box. Rails has it. Django has it. Laravel has it.

Go developers have built amazing things too — but not because of the tooling. Despite it. Go developers have built amazing things too — but not because of the tooling. Despite it. Go developers have always had to build integration themselves, piece by piece, and then maintain it forever. A router here. An ORM there. A queue library over here. A WebSocket package somewhere else. Ten different ecosystems. Ten different futures. Ten chances for any one of them to fall out of sync with your needs.

GoStack delivers what Go has always deserved: a complete, integrated framework designed from the ground up to be **Future-Proof by Default**. No hunting for which router works with which ORM. No wondering if your WebSocket library will still be maintained next year. No constant context switching between packages with different philosophies.

The result is simple: one stack, one way of working, one future. No assembly required. No fragmentation. No guesswork.

Other ecosystems have enjoyed this foundation for years. GoStack brings it to the Go ecosystem, to finally remove those headaches and uncertainties.

GoStack handles the heavy lifting and the behind-the-scenes complexity. While you handle your ambition and concentrate on your core business objectives.

# The Core Rationale in One Sentence
GoStack exists because Go is an exceptional language held back by ecosystem fragmentation — and the cure is a single, opinionated, compile-safe framework that lets you build the server, the database layer, and the UI without ever leaving Go.

It's positioned as the answer to: "Why would I use Laravel/Rails/Django if I want Go's performance?" The answer GoStack gives is: "You wouldn't have to choose anymore."


---

## ⚡ The Death of "Glue Code" (15 Packages vs. The GoStack Way)

If you ask how to build a complete fullstack application in Go using standard open-source libraries, you are usually handed a fragmented stack of 15+ independent packages. 

Instead of writing your product, you spend the first week writing "glue code" to make these libraries talk to each other, dealing with dependency version conflicts, and managing complex configurations.

Here is how **GoStack** consolidates the entire fullstack developer experience:

| Feature Capability | The Fragmented Way (15 Libraries) | The GoStack Way (Unified Built-ins) |
| :--- | :--- | :--- |
| **HTTP Routing** | `gin-gonic/gin` or `chi` | **Navigator** — Built-in, high-performance router |
| **Database ORM** | `gorm.io/gorm` | **Crafter** & **Conflex** — Active Record & Hydrator |
| **Migrations** | `pressly/goose` or `golang-migrate` | **Traveller** — Database version schema runner |
| **UI Templates** | `a-h/templ` | **Tempose** — AOT component compiler |
| **Session Auth** | `authcore` or custom cookie sessions | **Guard** — Session & Token Auth + CSRF protection |
| **Social Login** | `golang.org/x/oauth2` | **SocialHub** — Multi-provider OAuth login flow |
| **Job Queue** | `neoq` or `asynq` | **Sequence** — Delayed background worker queues |
| **Cron Scheduling** | `robfig/cron/v3` | **Planner** — Background task scheduler |
| **Event Dispatcher** | `EventBus` | **Spark** — Pub/Sub event dispatcher |
| **WebSockets** | `coder/websocket` | **GowSocket** — Bi-directional WebSocket hub |
| **File Storage** | `gocloud.dev` (blob) | **Vault** — Local & AWS S3 secure storage sandbox |
| **Caching / Cache** | `go-redis/v9` | **Mory** — Generic-typed key-value memory cache |
| **SMTP Mailing** | `gomail` | **GoMail** — SMTP HTML mailer |
| **CLI App Runner** | `spf13/cobra` | **Gost** — Interactive console command assistant |
| **Config Loader** | `spf13/viper` | **GoCon** — Dynamic `.env` environment parser |
| **User Alerting** | Custom mail & DB notification logic | **Alert** — Multi-channel notification router |

By consolidating these 16 libraries into **one binary, one programming language, and one cohesive mental model**, GoStack frees you from configuration hell. **Zero glue code. Zero package conflicts. Just pure product engineering.**

---

## 🗺️ The GoStack Subsystem Registry

Every major capability in GoStack is a fully tested, co-developed, and branded subsystem:

| # | Branded Name | Subsystem Capability | Description |
| :--- | :--- | :--- | :--- |
| 1 | **Citadel** | Application Bootloader & Unified Kernel | Coordinates GoStack's application lifecycle by executing registration and boot phases across all modules. It guarantees that critical resources (like database pools and routers) are fully initialized before the framework starts serving requests. |
| 2 | **Anchor** | IoC Service Container & Dependency Injection | A thread-safe, lock-shielded Inversion of Control storage container. By resolving service blueprints on demand, it completely eliminates dependency-ordering bugs and compile-time code generation bloat. |
| 3 | **Navigator** | HTTP Router | A high-performance, parameter-binding HTTP routing tree. It parses URL patterns, matches incoming paths, and passes request scopes directly to target handlers with zero dynamic runtime allocations. |
| 4 | **Bridge** | Middleware Pipeline (Onion Architecture) | Implements a sequential onion-style middleware pipeline. It intercepts requests to perform cross-cutting tasks (like CSRF protection, CORS handling, or sessions) before letting target handlers execute, supporting early-exit abort flows. |
| 5 | **Crafter** | Query Builder & Reflective Hydrator | Dynamically generates clean, optimized SQL and database calls. It utilizes a runtime reflection cache to safely translate raw database records into strongly-typed Go entities, shielding developer code from raw SQL boilerplate. |
| 6 | **Conflex** | ORM Relationship Mapper | Resolves relational model schemas (`HasOne`, `HasMany`, `BelongsTo`). It tracks and loads child connections, caching retrieved structures to eliminate costly N+1 query execution bottlenecks. |
| 7 | **Traveller** | Database Migration Engine | Performs transaction-shielded, versioned database schema updates. It maps applied migrations inside system ledgers to keep local development environments and production servers perfectly in sync. |
| 8 | **Grapher** | Schema Builder (fluent column definitions) | Provides a fluent, programmatic builder interface for setting up database columns, indexes, and constraints. It translates Go definitions into native DDL statements, avoiding raw schema script errors. |
| 9 | **Guard** | Authentication (session-based) | Manages secure user session cookies, token issuance, and CSRF token generation. It acts as the secure entry guard, shielding internal routes from unauthenticated actors. |
| 10 | **Mory** | Generic-typed In-Memory Cache | A high-speed, generic-typed key-value caching layer. It stores heavily accessed data models in local memory to speed up response times and reduce database querying overhead. |
| 11 | **Sequence** | Background Job Queue | Manages background queue workers to run deferred tasks (like report compiles or API calls). It supports retries, custom delays, and job chains, ensuring the main HTTP request thread remains fast. |
| 12 | **Spark** | Pub/Sub Event Dispatcher | Dispatches synchronous and asynchronous events across application modules. It allows developers to completely decouple actions (e.g. "UserRegistered") from side-effects (e.g. "SendWelcomeEmail"). |
| 13 | **GoMail** | SMTP Mailer | A zero-dependency SMTP client built on standard Go packages. It handles HTML mail formatting, attachment rendering, and template loading, enabling professional email delivery. |
| 14 | **Vault** | File Storage (Local + S3) | Provides a secure, path-traversal shielded file storage gateway. It allows developers to swap local folder storage for cloud storage (like AWS S3) transparently by modifying a single environment variable. |
| 15 | **Alert** | Multi-Channel Notification System | Routes notifications across multiple delivery channels (Email, Database, SMS). It decouples notification messages from delivery platforms using a unified, fluent dispatcher structure. |
| 16 | **Planner** | Cron Task Scheduler | A background schedule scheduler that runs cron-style tasks. It eliminates the need for external system-level crontabs, letting developers configure recurrent jobs inside compile-safe Go code. |
| 17 | **SocialHub** | OAuth Social Login | Handles third-party OAuth2 authorization flows (like Google or GitHub). It validates secure callbacks, retrieves user profile metadata, and integrates credentials with the Guard subsystem. |
| 18 | **GoDash** | Admin Panel & Sequence Monitor | Provides an auto-generated admin interface and background queue monitoring screen. It allows developers to visualize active tasks, search database records, and debug jobs out of the box. |
| 19 | **Tempose** | AOT UI Component Compiler | Compiles HTML component templates, styles, and scripts into static Go code. It completely eliminates disk IO overhead at runtime, allowing templates to render at Go's compiled speed. |
| 20 | **Glide** | Client-Side Reactive Runtime | A zero-dependency reactive directive engine served directly inside page bundles. It parses dynamic HTML attributes (like `gs-model`, `gs-show`) to manage reactivity, spa router morphs, and transitions without Node.js toolchains. |
| 21 | **Gost** | CLI Command Runner | Powers the command-line interface. It registers custom console scripts, executes database migration scripts, and generates scaffolded components using Go's exact dependency tree. |
| 22 | **Contract** | Interface Definitions (driver contracts) | Defines the core interface contracts that decouple framework drivers from your application logic. It ensures that system implementations (like cache backends) remain fully interchangeable. |
| 23 | **GoCon** | Environment Config Manager (`.env` parser) | Scans, parses, and loads `.env` variables into application state at boot time. It converts configuration keys into strongly-typed structures, protecting startup routines from missing keys. |
| 24 | **Validator** | Request Validation Engine | Evaluates incoming request JSON payloads or form data against declared rules. It acts as the gateway validation layer, mapping error details onto contexts before reaching controller logic. |
| 25 | **GowSocket** | Real-Time WebSocket Hub | Orchestrates bi-directional, persistent WebSocket client connections. It manages socket groups, processes message frames, and runs real-time pushes with native Go channels. |
| 26 | **Transios** | Localization & Translation Engine | Dynamically formats dates, currencies, and localized strings based on user languages. It reads translation directories at boot time, mapping language templates for client renderers. |
| 27 | **GoMon** | MongoDB Integration (Official Extension) | Provides native NoSQL document store mappings. Extends Crafter's hydration engine to parse BSON objects, enabling schema-less database operations in `gostack-extensions/gomon`. |
| 28 | **Nexus** | Neo4j Graph Database Integration (Official Extension) | Bridges GoStack with Neo4j graph nodes. Supports execution of Cypher query patterns, mapping complex relational graphs in `gostack-extensions/nexus`. |
| 29 | **Aether** | Cassandra Wide-Column Integration (Official Extension) | Distributed CQL query mapper and table optimizer for Apache Cassandra database structures in `gostack-extensions/aether`. |

---

## 📦 Official Extensions

GoStack supports optional plug-and-play extensions that are fully isolated from the core framework package to maintain clean database and architecture boundaries:

*   **MFA (TOTP Multi-Factor Authentication):** Standard RFC 6238 TOTP verification and QR code generation.
    ```bash
    go get github.com/charledeon77/gostack-extensions/mfa
    ```
*   **RBAC (Role-Based Access Control):** Dynamic roles, permissions, HTTP middlewares, and database-agnostic resolvers (supporting both relational SQL databases and NoSQL/Cassandra).
    ```bash
    go get github.com/charledeon77/gostack-extensions/rbac
    ```
*   **GoMon (MongoDB Document Store):** BSON hydration and NoSQL Active Record queries for MongoDB.
    ```bash
    go get github.com/charledeon77/gostack-extensions/gomon
    ```
*   **Nexus (Neo4j Graph Database):** Cypher query builders and graph relation mapping for Neo4j.
    ```bash
    go get github.com/charledeon77/gostack-extensions/nexus
    ```
*   **Aether (Cassandra Wide-Column Store):** Distributed CQL queries and wide-column optimizations for Apache Cassandra.
    ```bash
    go get github.com/charledeon77/gostack-extensions/aether
    ```

---

## ✨ The GoStack Architectural Manifesto & Developer Joy Guidelines

GoStack was designed with a simple belief: **building web apps should feel like play, not puzzle assembly.** These principles ensure GoStack remains the most enjoyable, robust, and lightning-fast fullstack environment in the Go ecosystem.

### 🚀 1. The Zero-Dependency Principle (Compile in under 2 Seconds)
*   **The Rationale:** GoStack avoids the library creep that slows down projects. By writing native, optimized standard library wrappers instead of importing heavy third-party packages, we preserve Go's famous compilation speeds.
*   **The Developer Joy:** Your entire app compiles in the blink of an eye. You get tiny, single-file distribution binaries with zero runtime dependencies.
*   **The Guideline:** Before running `go get` for a utility, try standard library primitives first. Keep the codebase clean, lean, and self-contained.

### 🎨 2. No Node.js. No npm. No Webpack. (Tempose AOT Compilation)
*   **The Rationale:** Frontend templates (HTML, CSS, JS) compile directly into Go code by **Tempose** during your build. 
*   **The Developer Joy:** You get a fullstack app with no `node_modules` bloating your drive, no Webpack configuration files to debug, and no separate Node.js dev server. Just raw, lightning-fast Go runtime rendering.
*   **The Guideline:** Run `gostack compile` (or launch `gost dev` for real-time hot-reloading) to let the AOT compiler build your frontend assets directly into the Go source files.

### 🧩 3. Write Once, Run Anywhere (The Context & Contract Design)
*   **The Rationale:** Dynamic application operations use **Contract** interfaces (`framework/contract`), meaning they never bind directly to a specific backend database or driver.
*   **The Developer Joy:** Need to switch from local disk storage to AWS S3? Or from SQLite to PostgreSQL? Just change a line in your `.env`. The codebase remains untouched and fully operational.
*   **The Guideline:** Always accept contract interfaces in your services rather than concrete structs. Let the container handle the bindings.

### ⚡ 4. Streamlined HTTP Operations (The Context Wrapper Pattern)
*   **The Rationale:** Middleware, controllers, and interceptors share a unified `*http.Context` wrapper instead of raw primitives.
*   **The Developer Joy:** Retrieving path params, validating JSON inputs, rendering templates, or setting secure cookies becomes a single, clean method call. No redundant boilerplate code.
*   **The Guideline:** Utilize the context helpers to pass payload states cleanly through the **Bridge** onion middleware pipeline.

### 🎨 5. Style Freedom by Default (`gs-css`)
*   **The Rationale:** Tempose's baseline default styling engine only acts on HTML elements that explicitly carry the `gs-css` tag attribute.
*   **The Developer Joy:** The framework never hijacks your custom layouts or overrides your Tailwind/Vanilla CSS rules. You stay in full creative control of your design system from day one.
*   **The Guideline:** Include `gs-css` on components where you want GoStack's built-in styles, and leave it off where you want absolute creative freedom.

---
*GoStack — The speed and performance of Go, the interconnected ecosystem of Laravel, the simplicity and reactivity of  AlpineJS, with the complete powers of React, Svelte and Vue combined. One binary. One language. Zero headaches.*

---

## 🛡️ Production Ready Assurance

Over a rigorous release cadence (*culminating in current version **`v1.0.23`** released 22 November 2026*), **GoStack** has matured into an enterprise-grade, hardened, and battle-tested fullstack framework.

Below is an overview of GoStack's production architecture, security engineering, and extension ecosystem.

---

### 1. Robust Core Architecture & Subsystem Maturity
Every major framework layer is co-developed, branded, and unified via the dynamic **Citadel** container bootstrapper and the **Anchor** Dependency Injection engine:

*   **Navigator** (HTTP Router) and **Bridge** (Middleware Pipeline) handle request orchestration with built-in trailing slash normalization, route conflict detection, and zero-allocation path parameter binding.
*   **Crafter** (ORM / Query Builder), **Conflex** (Relations Mapper), and **Grapher** (Schema Builder) automate database access across SQL dialects (PostgreSQL, MySQL, CockroachDB, SQLite) with deterministic parameter binding and savepoint transaction nesting.
*   [**Glide**](./GLIDE.md) (*Reactive Runtime*) and **Tempose** (AOT UI Compiler) compile component templates, styles, and client reactivity directly into Go source code, completely removing Node.js/npm dependencies from production binaries.

### 2. Crucial Production-Grade Hardening & Security Features
GoStack features an extensive, defense-in-depth security and stability foundation (*detailed in* **`CHANGELOG.md`**):

*   **Vault Zero-Memory Object Streaming (`v1.0.23`)**:
    *   **`PutStream(path, io.Reader)`**: Streams file uploads directly to local disk or S3-compatible object storage (AWS S3, Backblaze B2, Cloudflare R2, MinIO) in 32 KB chunks via `io.Copy`. Server RAM cost for a 50 MB upload is ~32 KB (the active buffer), not 50 MB.
    *   **`GetStream(path) io.ReadCloser`**: Returns a live read stream — `*os.File` for local storage, `s3.GetObjectOutput.Body` for S3 — allowing response writers to pipe bytes directly to the client with zero intermediate heap allocation.
    *   **Path-Traversal Sandbox on Streaming Endpoints**: `PutStream` and `GetStream` on `LocalStorage` route through the same `resolvePath()` sandbox that guards all blocking operations; traversal payloads are rejected before any I/O is performed.
    *   **Backward-Compatible Interface Extension**: `contract.Storage` now defines six methods; existing `Put`, `Get`, `Delete`, and `Exists` calls are untouched across all drivers.
*   **Tempose Layout Architecture & Scoped Fragment Isolation (`v1.0.22`)**:
    *   **Automatic Fragment Badging**: Automatically wraps page fragments inserted at layout content slots (`@yield('content')`, `<slot/>`) in `<div gs-component="name">`, guaranteeing page-specific CSS matches with surgical precision while keeping outer document doctypes at byte index 0 (100% Standards Mode).
    *   **Layout-Transparent Scope (`display: contents`)**: Injected `[gs-component] { display: contents; }` into `GoStackCoreCSS` so isolation wrappers never disrupt flexbox, CSS grid, or flow geometry.
    *   **Compile-Time Document Tag Guard**: Rejects child templates authoring `<!DOCTYPE>`, `<html>`, `<head>`, or `<body>` inside `@extends` sections, preventing corrupt multi-document nesting at build time.
    *   **Standalone Document Badging**: Automatically badges `<body>` tags in standalone full documents so component styles bind without breaking Standards Mode.
*   **Enterprise Security & Multi-Dialect Hardening (`v1.0.21`)**:
    *   **GoDash Admin CSRF Armor**: Intercepts all administrative mutations (`/admin/:model`, `/admin/:model/:id/delete`, sequence monitor actions) with `SameSite=Lax` HttpOnly companion cookies and constant-time token validation (`crypto/subtle.ConstantTimeCompare`).
    *   **Multi-Dialect Schema Differ**: Dialect-native type mapping for PostgreSQL (`BOOLEAN`, `DOUBLE PRECISION`, `TIMESTAMPTZ`, `BYTEA`), MySQL, and SQLite.
    *   **Postgres Schema Isolation & Transactional Rollbacks**: Appends `AND table_schema = current_schema()` to introspection queries and executes `ApplyDiff()` inside transactions with atomic rollback on failure.
    *   **QueryBuilder Pagination Ceiling**: Enforces `DefaultMaxPaginationPerPage = 100` to prevent memory-exhaustion Denial-of-Service attacks.
    *   **GoCon Quoted Hash Preservation**: Preserves `#` in complex passwords, DSNs, and secret keys inside `.env` files.
*   **Standards Mode Doctype & Compile-Time Validation (`v1.0.20`)**: Guarantees `<!DOCTYPE html>` at byte index 0 to eliminate browser Quirks Mode, accompanied by a compile-time directive nesting validation stack tracking open `@if` and `@foreach` tags with file and line numbers.
*   **Robust CSS Scoping & Animation Preservation (`v1.0.19`)**: Strips multi-line comments cleanly without corrupting adjacent selectors, preserves `@keyframes` animation steps (`from`, `to`, `0%`), and prefixes comma-separated multi-target selectors.
*   **Lean Core & Pure CSP Compliance (`v1.0.18`)**: Eradicated the 470 KB embedded base64 font from core, delivering 100% strict Content-Security-Policy compliance with zero inline styles and zero-FOUC static `[gs-cloak]` styling.
*   **Browser-Cached Static Asset Bundles & `router.Static` (`v1.0.17`)**: Compiles component styles and scripts into fingerprinted static bundles (`/assets/css/gostack.css`, `/assets/js/gostack.js`) with immutable browser caching headers and built-in path-traversal / Zip-Slip protected file serving.
*   **Client Reactivity & DOM Morphing (`v1.0.10`)**: Upgraded Glide to a Proxy-based reactivity engine supporting SPA routing, client-side hydration, and browser-native `DOMParser` sanitization.
*   **Nested Transactions (SQL Savepoints) & Connection Pooling (`v1.0.2`)**: Natively supports nested transactions via standard SQL `SAVEPOINT` protocols, together with full connection pool limit management (`MaxOpenConns`, `MaxIdleConns`, `ConnMaxLifetime`).
*   **API Security & Throttling (`v1.0.2` / `v1.0.3`)**: Employs an `AuthThrottle` rate-limiting middleware on auth endpoints (sliding-window composite keys) and alphanumeric identifier validation check allowlists to prevent SQL injection.
*   **Reliable Cron Scheduler (`v1.0.2`)**: Implements timezone-aware schedules with `.WithoutOverlapping()` atomic locks to ensure long-running background tasks never trigger concurrent, degrading runs.
*   **Transios i18n Localization (`v1.0.7`)**: Serves translation keys under zero disk I/O latency, preloading default locale data directly inside binaries.

### 3. Mature Isolated Extension Ecosystem
To respect core design rationale and keep database/architecture boundaries perfectly clean, complex plug-and-play extensions and specialized storage engines are managed in isolated packages under `gostack-extensions`:

*   🔑 **Multi-Factor Auth (MFA)**: RFC 6238 TOTP validation and QR code generation (`gostack-extensions/mfa`).
*   🛡️ **Role-Based Access Control (RBAC)**: Dynamic roles, permissions, HTTP middlewares, and database-agnostic resolvers supporting both relational SQL databases and NoSQL/Cassandra (`gostack-extensions/rbac`).
*   🍃 **MongoDB Document Store (GoMon)**: BSON hydration and NoSQL Active Record queries (`gostack-extensions/gomon`).
*   🕸️ **Neo4j Graph Database (Nexus)**: Cypher query builders and graph relation mapping (`gostack-extensions/nexus`).
*   ⚡ **Cassandra Wide-Column (Aether)**: Distributed CQL queries and wide-column optimizations (`gostack-extensions/aether`).

> [!IMPORTANT]
> ***Why Extensions Remain Isolated***
>
> *GoStack natively supports multiple database paradigms—including SQL (PostgreSQL, MySQL, SQLite) and NoSQL (MongoDB, Neo4j, Cassandra). Keeping specialized database engines and features like RBAC as isolated extensions allows developers to include only the database drivers their architecture actually requires, eliminating bloated binaries, cross-dependency pollution, and forced relational schemas.*

### 4. 100% Green Test Suite
All test suites across the framework compile, build, and pass cleanly:

*   **Core Framework**: Passed across all 23 packages under race condition detection (`go test -race -count=1 ./framework/...`), `go vet`, and `go build`.
*   **Official Extensions**: Passed and verified in the `gostack-extensions` repositories.

### Summary
With zero external Node.js/npm runtime dependencies, built-in enterprise security protections (CSRF armor, XSS filtering, Rate-Limiting, SQL Injection guards), robust database transaction layers, fingerprinted static asset delivery, and isolated extensible packages, GoStack is fully prepared to back **Mission-Critical, High-Scale Applications**.
