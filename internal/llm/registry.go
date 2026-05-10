package llm

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/tetexu/tlaude-code/internal/logging"
)

type ProviderStatus struct {
	Name      string
	Available bool
	Priority  int
	Latency   time.Duration
	LastCheck time.Time
}

type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
	configs   map[string]ProviderConfig
	status    map[string]*ProviderStatus
}

var (
	globalRegistry *Registry
	registryOnce   sync.Once
)

func GlobalRegistry() *Registry {
	registryOnce.Do(func() {
		globalRegistry = &Registry{
			providers: make(map[string]Provider),
			configs:   make(map[string]ProviderConfig),
			status:    make(map[string]*ProviderStatus),
		}
	})
	return globalRegistry
}

func (r *Registry) Register(name string, factory ProviderFactory, config ...ProviderConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cfg := DefaultConfig()
	if len(config) > 0 {
		cfg = config[0]
	}

	provider, err := factory(cfg)
	if err != nil {
		return fmt.Errorf("failed to create provider %s: %w", name, err)
	}

	r.providers[name] = provider
	r.configs[name] = cfg
	r.status[name] = &ProviderStatus{
		Name:      name,
		Available: false,
		Priority:  cfg.Priority,
		LastCheck: time.Now(),
	}

	logging.Info("provider registered", "name", name, "priority", cfg.Priority)
	return nil
}

func (r *Registry) Get(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *Registry) ListByPriority() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	type entry struct {
		name     string
		priority int
	}
	entries := make([]entry, 0, len(r.providers))
	for name := range r.providers {
		entries = append(entries, entry{name: name, priority: r.configs[name].Priority})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].priority < entries[j].priority
	})
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.name
	}
	return names
}

func (r *Registry) GetConfig(name string) (ProviderConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg, ok := r.configs[name]
	return cfg, ok
}

func (r *Registry) GetStatus(name string) (*ProviderStatus, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.status[name]
	return s, ok
}

func (r *Registry) AllStatus() []ProviderStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	statuses := make([]ProviderStatus, 0, len(r.status))
	for _, s := range r.status {
		statuses = append(statuses, *s)
	}
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Priority < statuses[j].Priority
	})
	return statuses
}

func (r *Registry) ProbeAll(ctx context.Context) {
	r.mu.RLock()
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	r.mu.RUnlock()
	var wg sync.WaitGroup
	for _, name := range names {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			r.probeOne(ctx, n)
		}(name)
	}
	wg.Wait()
}

func (r *Registry) probeOne(ctx context.Context, name string) {
	r.mu.RLock()
	provider, ok := r.providers[name]
	r.mu.RUnlock()
	if !ok {
		return
	}
	start := time.Now()
	available := provider.IsAvailable()
	r.mu.Lock()
	if r.status[name] == nil {
		r.status[name] = &ProviderStatus{Name: name}
	}
	r.status[name].Available = available
	r.status[name].Latency = time.Since(start)
	r.status[name].LastCheck = time.Now()
	r.mu.Unlock()
	if available {
		logging.Debug("provider probe ok", "name", name, "latency", time.Since(start))
	} else {
		logging.Warn("provider probe failed", "name", name)
	}
}

func (r *Registry) SelectAvailable(preferred ...string) (Provider, string, error) {
	if len(preferred) > 0 && preferred[0] != "" {
		for _, name := range preferred {
			if p, ok := r.Get(name); ok {
				r.mu.RLock()
				status := r.status[name]
				r.mu.RUnlock()
				if status != nil && status.Available {
					return p, name, nil
				}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				r.probeOne(ctx, name)
				cancel()
				if s, _ := r.GetStatus(name); s != nil && s.Available {
					return p, name, nil
				}
			}
		}
	}
	byPriority := r.ListByPriority()
	for _, name := range byPriority {
		p, ok := r.Get(name)
		if !ok {
			continue
		}
		r.mu.RLock()
		status := r.status[name]
		r.mu.RUnlock()
		if status != nil && status.Available {
			return p, name, nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		r.probeOne(ctx, name)
		cancel()
		if s, _ := r.GetStatus(name); s != nil && s.Available {
			return p, name, nil
		}
	}
	return nil, "", fmt.Errorf("no available provider found")
}

func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.providers, name)
	delete(r.configs, name)
	delete(r.status, name)
	logging.Info("provider unregistered", "name", name)
}
