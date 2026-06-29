// Package foundation (Citadel) serves as the core booting and structural bedrock of the 
// GoStack framework, managing low-level application lifecycles and diagnostics.
package foundation

// ServiceProvider establishes the unified structural lifecycle contract for all 
// modular packages, sub-systems, and feature drivers within the GoStack ecosystem.
//
// DESIGN PHILOSOPHY:
// In compliance with battle-tested architectural paradigms (like Laravel and Django),
// a Service Provider decouples module initialization from the global startup script. 
// Instead of cluttering main.go with database client initializations, router wiring, 
// and config parsing, each package provides an implementation of this contract.
//
// DECOUPLED LIFECYCLE PARADIGM:
// To completely eliminate dependency-ordering deadlock bugs (e.g., a router module 
// attempting to resolve an uninitialized database connector during startup), the 
// framework enforces a strict, two-step boot pipeline managed via Register() and Boot().
type ServiceProvider interface {
	// Register binds service factory recipes into the central Service Container.
	//
	// EXCLUSIVITY ARCHITECTURAL CRITERIA:
	// Providers are ONLY allowed to execute Container.BindSingleton() or Container.BindTransient()
	// within this method block. They are strictly forbidden from executing Resolve() or querying 
	// other services, because those companion dependencies might not have been registered yet.
	Register(c *Container)

	// Boot executes initialization, bootstrap, or configuration routines.
	//
	// EXCLUSIVITY ARCHITECTURAL CRITERIA:
	// This method fires ONLY after every single Service Provider in the application has completed 
	// its Register() step. Therefore, you are guaranteed that all service blueprints coexist 
	// inside the container warehouse. Here, a provider can safely run Resolve(), spawn background 
	// queue workers, or read active configuration variables.
	Boot(c *Container)
}

// Application serves as the absolute master core runtime manager for the GoStack framework.
// It encapsulates the root Service Container and coordinates the execution loop of providers.
type Application struct {
	container *Container
	providers []ServiceProvider
	isBooted  bool
}

// NewApplication instantiates a fresh framework coordinator bound to a new Service Container.
func NewApplication() *Application {
	return &Application{
		container: NewContainer(),
		providers: make([]ServiceProvider, 0),
		isBooted:  false,
	}
}

// Container returns the underlying Inversion of Control (IoC) instance managed by the application.
func (a *Application) Container() *Container {
	return a.container
}

// RegisterProvider appends a modular service provider to the framework's internal lifecycle stack.
// If the application has already entered its runtime phase (isBooted == true), the provider's 
// structural phases are instantly executed on the fly to support dynamic runtime hot-swaps.
func (a *Application) RegisterProvider(provider ServiceProvider) {
	a.providers = append(a.providers, provider)

	// If the application has already booted up, dynamically register and boot this hot-swapping module
	if a.isBooted {
		provider.Register(a.container)
		provider.Boot(a.container)
	}
}

// Boot executes the master two-step lifecycle processing loop across all registered providers.
// Once executed, the framework layer is fully initialized and open to accept incoming traffic.
func (a *Application) Boot() {
	if a.isBooted {
		return // Guard against accidental double-boot triggers re-initializing runtime state
	}

	// --- PHASE 1: UNIFIED REGISTRATION PASS ---
	// Loop over all modules and load their service recipes safely into the central container.
	for _, provider := range a.providers {
		provider.Register(a.container)
	}

	// --- PHASE 2: UNIFIED BOOTSTRAPPING PASS ---
	// Loop over all modules a second time to execute operational configurations safely.
	for _, provider := range a.providers {
		provider.Boot(a.container)
	}

	a.isBooted = true
}