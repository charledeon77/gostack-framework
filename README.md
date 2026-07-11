<!-- Logo -->
<p align="center">
  <img src="assets/images/gostack-3d-logo.png" alt="GoStack Logo" width="100%">
</p>

<!-- Badges -->
<p align="center">
  <img src="https://img.shields.io/badge/License-Apache%202.0-orange?logo=apache" />
  <img src="https://img.shields.io/badge/version-1.0.0-1FB249?logo=github" />
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

**GoStack** is the first complete end-to-end framework solution for building **FullStack Web applications** in Go. It handles the:

1.) ✅ **Server & Container** *(Citadel & Anchor)* — unified bootstrapping kernel and IoC service container.

2.) ✅ **Middleware** *(Navigator)* — pipeline-aware routing engine with onion-style middleware.

3.) ✅ **ORM & Relations** *(Crafter & Conflex)* — compile-time safe Active Record ORM with model hooks, hydration, and relationship mapping.

4.) ✅ **Schema Builder** *(Grapher)* — declarative fluent schema builder for database definitions.

5.) ✅ **Migrations** *(Traveller)* — auto-schema diffing migration runner with transaction support.

6.) ✅ **Authentication** *(Guard)* — session/token auth with CSRF and policy-based RBAC.

7.) ✅ **Caching** *(Mory)* — strongly-typed generic caching adapter.

8.) ✅ **Queue** *(Sequence)* — background worker with retries, delays, job chains, and batches.

9.) ✅ **Events** *(Spark)* — sync and async Pub/Sub event dispatcher.

10.) ✅ **Mail** *(GoMail)* — rich SMTP HTML mailer built on the standard library.

11.) ✅ **Storage** *(Vault)* — traversal-secure local and S3 storage sandbox.

12.) ✅ **Scheduler** *(Planner)* — native cron-style task scheduler running in background.

13.) ✅ **Admin Dashboard** *(GoDash)* — auto-generated admin panel with live queue monitoring.

14.) ✅ **Frontend Compiler** *(Tempose)* — AOT compiler turning HTML/CSS/JS into Go code.

15.) ✅ **Client Reactivity** *(Glide)* — zero-dependency reactive directive engine.

16.) ✅ **CLI** *(Gost)* — interactive scaffolding for migrations, models, and controllers.

17.) ✅ **Config** *(GoCon)* — environment-based configuration management.

18.) ✅ **Validation** *(Validator)* — rule-based request payload validator middleware.

19.) ✅ **Real-Time WebSockets** *(GowSocket)* — bi-directional WebSocket communication hub.

20.) ✅ **Internationalization** *(Transios)* — dynamic localization and translation engine.

21.) ✅ **Everything** else in-between, and the glue that connects them — all within the **Go** ecosystem.

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

## 🗺️ The GoStack Subsystem Registry

Every major capability in GoStack has a branded name:

| Branded Name | What It Is | Package |
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

---

## 🛠️ Architecture Invariants & Developer Guidelines

### 1. The Zero-Dependency Principle
*   **Invariant:** Keep third-party dependencies to an absolute minimum.
*   **Rationale:** GoStack prioritizes high compilation speeds and tiny binary footprints. Rather than importing heavy CLI or routing libraries, GoStack relies on native standard library wrappers (`net/http`, `database/sql`).
*   **Guideline:** Avoid running `go get` for minor utilities. Write clean, native Go helpers first.

### 2. Ahead-of-Time (AOT) Frontend Compilation (Tempose)
*   **Invariant:** Component templates (HTML, CSS, JS) are compiled directly into Go source code by **Tempose**.
*   **Rationale:** GoStack does not parse template files at runtime or rely on Node.js. Scoped component styles are isolated using custom attribute selectors at build-time.
*   **Guideline:** Run `gostack compile` to update `gostack_components_gen.go`. Do not manually edit generated files.

### 3. Modular Self-Containment via Contract
*   **Invariant:** Application code binds to **Contract** interfaces (`framework/contract`), not concrete drivers.
*   **Rationale:** Decoupling adapters from core packages ensures drivers (MySQL, PostgreSQL) can be swapped by modifying the environment DSN without altering application logic.

### 4. Pluggable Self-Registration
*   **Invariant:** Components, drivers, and migrations register dynamically via `init()` blocks.
*   **Rationale:** Centralized configuration lists are avoided. Packages self-register at runtime upon import.

### 5. Local Command Execution via Gost
*   **Invariant:** Execute CLI commands locally using `go run cmd/gostack/main.go <command>` or the global `gost` binary.
*   **Rationale:** Running **Gost** locally ensures compilers, code generators, and **Traveller** migrations are compiled with the active project's exact dependency tree.

### 6. The Context Wrapper Pattern (Bridge)
*   **Invariant:** Interceptors and handlers use `*http.Context` rather than raw `http.ResponseWriter`/`*http.Request`.
*   **Rationale:** Wrapping the HTTP primitives simplifies payload decoding, routing arguments, and state-sharing across the **Bridge** middleware pipeline.

### 7. The `gs-css` Opt-In Styling Rule (Tempose)
*   **Invariant:** Tempose's Core Base CSS only applies to elements that carry the `gs-css` HTML attribute.
*   **Rationale:** Opt-in styling gives developers 100% control. The framework never overrides custom designs without explicit permission.
