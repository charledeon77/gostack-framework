package admin

// Purpose: To provide a Django-style, auto-generated administrative control panel (GoDash) for GoStack.
// Philosophy: A developer should be able to register any database model with `admin.Register(&User{})`
// and immediately get a fully functional, secure CRUD interface in their browser at `/admin`.
// No code generation. No templates to maintain. Pure Go reflection at the service layer, and
// embedded HTML at the view layer. The Admin panel is an introspection engine, not a code generator.
// Architecture:
// - Registry: A global map of model names to AdminEntry descriptors.
// - AdminEntry: Holds the model prototype, table name, and any optional configuration.
// - Routes: The HTTP handlers are in controller.go and are registered onto the app's router
//   by calling `admin.Mount(router)`.
// Choice:
// We embed the HTML views directly as Go string constants (similar to how Tempose works) to
// keep the panel completely self-contained. No external template files, no separate asset server.
// The panel is mounted at `/admin` by default and is protected by a configurable guard check.
// Implementation:
// - Register(model, opts...): Adds a model to the global registry.
// - All(): Returns all registered entries.
// - Find(name): Retrieves a specific entry by model name.

import (
	"github.com/charledeon77/gostack/framework/contract"
	"reflect"
	"strings"
	"sync"
)

// AdminEntry describes a model registered with the admin panel.
type AdminEntry struct {
	ModelType  reflect.Type
	TableName  string
	Columns    []ColumnMeta
	Label      string // Human-readable plural name, e.g. "Users"
}

// ColumnMeta describes a single column/field for admin display.
type ColumnMeta struct {
	Name    string // db tag / column name
	GoField string // struct field name
	Type    string // "string", "int", "bool", etc.
}

// Registry is the global admin model registry.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]*AdminEntry
}

// global is the package-level singleton registry.
var global = &Registry{
	entries: make(map[string]*AdminEntry),
}

// Register adds a model to the global admin panel registry.
// The model argument should be a pointer to a struct (e.g. &model.User{}).
func Register(model any) {
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	name := t.Name()
	tableName := strings.ToLower(name) + "s"
	// Support custom TableName()
	if tn, ok := model.(interface{ TableName() string }); ok {
		tableName = tn.TableName()
	}

	var cols []ColumnMeta
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		dbTag := f.Tag.Get("db")
		if dbTag == "" || dbTag == "-" || f.Tag.Get("rel") != "" {
			continue
		}
		cols = append(cols, ColumnMeta{
			Name:    dbTag,
			GoField: f.Name,
			Type:    f.Type.Kind().String(),
		})
	}

	entry := &AdminEntry{
		ModelType: t,
		TableName: tableName,
		Columns:   cols,
		Label:     name + "s",
	}

	global.mu.Lock()
	global.entries[strings.ToLower(name)] = entry
	global.mu.Unlock()
}

// All returns all registered admin entries.
func All() map[string]*AdminEntry {
	global.mu.RLock()
	defer global.mu.RUnlock()
	return global.entries
}

// Find retrieves an admin entry by its lowercase model name.
func Find(name string) (*AdminEntry, bool) {
	global.mu.RLock()
	defer global.mu.RUnlock()
	e, ok := global.entries[strings.ToLower(name)]
	return e, ok
}

// Queue holds the registered queue interface to be monitored by the admin panel.
var Queue contract.Queue

// SetQueue registers the active queue instance for dashboard inspection.
func SetQueue(q contract.Queue) {
	Queue = q
}
