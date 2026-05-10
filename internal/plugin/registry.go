package plugin

import "sync"

// Registry stores loaded plugins and provides lookup by name and capability.
type Registry struct {
	mu       sync.RWMutex
	plugins  map[string]Plugin
}

// NewRegistry creates an empty plugin registry.
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]Plugin),
	}
}

// Register adds a plugin to the registry. Returns an error if a plugin with
// the same name is already registered.
func (r *Registry) Register(p Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.plugins[p.Name()]; ok {
		return &PluginError{Name: p.Name(), Op: "register", Reason: "already registered"}
	}

	r.plugins[p.Name()] = p
	return nil
}

// Get returns a plugin by name. The second return value indicates whether
// the plugin was found.
func (r *Registry) Get(name string) (Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[name]
	return p, ok
}

// List returns all currently registered plugins.
func (r *Registry) List() []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Plugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		result = append(result, p)
	}
	return result
}

// ListByProvides returns all plugins that declare they provide the given
// extension point.
func (r *Registry) ListByProvides(provides Provides) []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Plugin
	for _, p := range r.plugins {
		for _, pv := range p.Provides() {
			if pv == provides {
				result = append(result, p)
				break
			}
		}
	}
	return result
}

// Remove unregisters a plugin by name. Returns false if the plugin was not
// found.
func (r *Registry) Remove(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.plugins[name]; !ok {
		return false
	}
	delete(r.plugins, name)
	return true
}

// Len returns the number of registered plugins.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.plugins)
}

// PluginError is a structured error for plugin operations.
type PluginError struct {
	Name   string
	Op     string
	Reason string
}

func (e *PluginError) Error() string {
	return "plugin " + e.Name + ": " + e.Op + ": " + e.Reason
}
