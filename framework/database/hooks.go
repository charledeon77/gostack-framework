package database

// Purpose: To provide Model Lifecycle Hooks for GoStack's Crafter (Query Builder) layer.
// Philosophy: Database models should have the ability to intercept the lifecycle of their own
// persistence — before they are written to disk, and after. This enables powerful patterns like
// automatic password hashing before a User is saved, timestamp mutation, cascading soft-deletes,
// and audit logging after creation — all without bloating controller logic.
// Architecture:
// We use Go interfaces to define optional hook contracts. The QueryBuilder checks if the model
// passed to Insert/Update/Delete implements any of these interfaces via a type assertion,
// and fires the hook if present. Zero-cost for models that don't implement the interfaces.
// Choice:
// We chose opt-in interfaces over struct embedding because they impose zero overhead on models
// that don't need hooks, they are explicit and self-documenting, and they compose cleanly with
// existing reflection-based hydration. No magic, no code generation required.
// Implementation:
// Models implement hooks by defining methods with specific signatures:
//
//	func (u *User) BeforeSave() error    { u.UpdatedAt = time.Now(); return nil }
//	func (u *User) AfterCreate() error   { sendWelcomeEmail(u); return nil }

// BeforeSaver is implemented by models that need to run logic before any INSERT or UPDATE.
type BeforeSaver interface {
	BeforeSave() error
}

// AfterCreator is implemented by models that need to run logic after a successful INSERT.
type AfterCreator interface {
	AfterCreate() error
}

// BeforeDeleter is implemented by models that need to run logic before a DELETE.
type BeforeDeleter interface {
	BeforeDelete() error
}

// AfterDeleter is implemented by models that need to run logic after a successful DELETE.
type AfterDeleter interface {
	AfterDelete() error
}
