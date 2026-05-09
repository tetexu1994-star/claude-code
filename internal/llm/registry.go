package llm

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
)

// ProviderStatus 提供者状态
type ProviderStatus struct {
	Name      string
	Available bool
	Priority  int
	Latency   time.Duration
	LastCheck time.Time
}

// Registry 是全局 Provider 注册中心
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

// GlobalRegistry 返回全局注册中心单例
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

// Register 注册一个 Provider
func (r *Registry) Register(name string, provider Provider, config ...ProviderConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.providers[name] = provider
	if len(config) > 0 {
		r.configs[name] = config[0]
	} else {
		r.configs[name] = DefaultConfig()
	}
	r.status[name] = &ProviderStatus{
		Name:      name,
		Available: true,
		Priority:  r.configs[name].Priority,
		LastCheck: time.Now(),
	}

	log.Printf("[Registry] Provider registered: %s (priority: %d)", name, r.configs[name].Priority)
}

// Get 获取指定名称的 Provider
func (r *Registry) Get(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[name]
	return p, ok
}

// List 返回所有已注册的 Provider 名称列表
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

// ListByPriority 按优先级返回 Provider 名称列表（优先级高的在前）
func (r *Registry) ListByPriority() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	type entry struct {
		name     string
		priority int
	}
	entries := make([]entry, 0, len(r.providers))
	for name := range r.providers {
		cfg := r.configs[name]
		entries = append(entries, entry{name: name, priority: cfg.Priority})
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

// GetConfig 获取 Provider 的配置
func (r *Registry) GetConfig(name string) (ProviderConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cfg, ok := r.configs[name]
	return cfg, ok
}

// GetStatus 获取 Provider 的当前状态
func (r *Registry) GetStatus(name string) (*ProviderStatus, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s, ok := r.status[name]
	return s, ok
}

// AllStatus 返回所有 Provider 的状态
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

// ProbeAll 探测所有已注册 Provider 的可用性
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

// ProbeOne 探测单个 Provider 的可用性
func (r *Registry) ProbeOne(ctx context.Context, name string) {
	r.probeOne(ctx, name)
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
		log.Printf("[Registry] Probe OK: %s (latency: %v)", name, time.Since(start))
	} else {
		log.Printf("[Registry] Probe FAIL: %s", name)
	}
}

// SelectAvailable 选择一个当前可用的 Provider，按优先级排序
func (r *Registry) SelectAvailable(preferred ...string) (Provider, string, error) {
	// 如果指定了优先选择的 Provider
	if len(preferred) > 0 {
		for _, name := range preferred {
			if p, ok := r.Get(name); ok {
				r.mu.RLock()
				status := r.status[name]
				r.mu.RUnlock()
				if status != nil && status.Available {
					return p, name, nil
				}
				// 尝试探测一次
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				r.probeOne(ctx, name)
				cancel()
				if s, _ := r.GetStatus(name); s != nil && s.Available {
					return p, name, nil
				}
			}
		}
	}

	// 按优先级遍历，找第一个可用的
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

		// 快速探测
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		r.probeOne(ctx, name)
		cancel()
		if s, _ := r.GetStatus(name); s != nil && s.Available {
			return p, name, nil
		}
	}

	return nil, "", fmt.Errorf("no available provider found")
}

// Unregister 注销一个 Provider
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.providers, name)
	delete(r.configs, name)
	delete(r.status, name)
	log.Printf("[Registry] Provider unregistered: %s", name)
}
