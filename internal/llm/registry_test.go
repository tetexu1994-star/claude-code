package llm

import (
	"context"
	"errors"
	"sort"
	"testing"
)

// mockProvider implements Provider for testing.
type mockProvider struct {
	name       string
	available  bool
	chatErr    error
	chatResp   *ChatResponse
	modelsList []string
	modelsErr  error
}

func (m *mockProvider) Name() string                              { return m.name }
func (m *mockProvider) IsAvailable() bool                         { return m.available }
func (m *mockProvider) Chat(_ context.Context, _ ChatRequest) (*ChatResponse, error) {
	return m.chatResp, m.chatErr
}
func (m *mockProvider) ChatStream(_ context.Context, _ ChatRequest) (<-chan Chunk, error) {
	return nil, nil
}
func (m *mockProvider) Models() ([]string, error) {
	return m.modelsList, m.modelsErr
}

func mockFactory(p Provider) ProviderFactory {
	return func(_ ProviderConfig) (Provider, error) { return p, nil }
}

func mockFactoryErr(err error) ProviderFactory {
	return func(_ ProviderConfig) (Provider, error) { return nil, err }
}

func newRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
		configs:   make(map[string]ProviderConfig),
		status:    make(map[string]*ProviderStatus),
	}
}

func TestGlobalRegistry_Singleton(t *testing.T) {
	r1 := GlobalRegistry()
	r2 := GlobalRegistry()
	if r1 != r2 {
		t.Error("GlobalRegistry() should return the same instance")
	}
}

func TestRegister(t *testing.T) {
	r := newRegistry()
	p := &mockProvider{name: "test", available: true}

	err := r.Register("test", mockFactory(p))
	if err != nil {
		t.Fatalf("Register() error: %v", err)
	}

	if len(r.providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(r.providers))
	}
	if status, ok := r.status["test"]; !ok || status.Name != "test" {
		t.Error("status not created for registered provider")
	}
}

func TestRegister_FactoryError(t *testing.T) {
	r := newRegistry()
	wantErr := errors.New("factory failed")

	err := r.Register("bad", mockFactoryErr(wantErr))
	if err == nil {
		t.Fatal("Register() should return error when factory fails")
	}
}

func TestGet(t *testing.T) {
	r := newRegistry()
	p := &mockProvider{name: "test", available: true}
	r.Register("test", mockFactory(p))

	got, ok := r.Get("test")
	if !ok {
		t.Fatal("Get() should find registered provider")
	}
	if got.Name() != "test" {
		t.Errorf("Get() name = %s, want test", got.Name())
	}

	_, ok = r.Get("missing")
	if ok {
		t.Error("Get() should not find unregistered provider")
	}
}

func TestList(t *testing.T) {
	r := newRegistry()
	r.Register("b", mockFactory(&mockProvider{name: "b", available: true}))
	r.Register("a", mockFactory(&mockProvider{name: "a", available: true}))

	names := r.List()
	if len(names) != 2 {
		t.Fatalf("List() len = %d, want 2", len(names))
	}
	if names[0] != "a" || names[1] != "b" {
		t.Errorf("List() = %v, want sorted [a b]", names)
	}
}

func TestListByPriority(t *testing.T) {
	r := newRegistry()
	r.Register("low", mockFactory(&mockProvider{name: "low", available: true}), ProviderConfig{Priority: 200})
	r.Register("high", mockFactory(&mockProvider{name: "high", available: true}), ProviderConfig{Priority: 10})

	names := r.ListByPriority()
	if len(names) != 2 {
		t.Fatalf("ListByPriority() len = %d, want 2", len(names))
	}
	if names[0] != "high" {
		t.Errorf("ListByPriority()[0] = %s, want high", names[0])
	}
	if names[1] != "low" {
		t.Errorf("ListByPriority()[1] = %s, want low", names[1])
	}
}

func TestGetConfig(t *testing.T) {
	r := newRegistry()
	cfg := ProviderConfig{Priority: 42, BaseURL: "https://example.com"}
	r.Register("test", mockFactory(&mockProvider{name: "test", available: true}), cfg)

	got, ok := r.GetConfig("test")
	if !ok {
		t.Fatal("GetConfig() should find config")
	}
	if got.Priority != 42 {
		t.Errorf("GetConfig() priority = %d, want 42", got.Priority)
	}
	if got.BaseURL != "https://example.com" {
		t.Errorf("GetConfig() baseURL = %s, want https://example.com", got.BaseURL)
	}

	_, ok = r.GetConfig("missing")
	if ok {
		t.Error("GetConfig() should not find missing config")
	}
}

func TestAllStatus(t *testing.T) {
	r := newRegistry()
	r.Register("a", mockFactory(&mockProvider{name: "a", available: true}), ProviderConfig{Priority: 10})
	r.Register("b", mockFactory(&mockProvider{name: "b", available: false}), ProviderConfig{Priority: 20})

	statuses := r.AllStatus()
	if len(statuses) != 2 {
		t.Fatalf("AllStatus() len = %d, want 2", len(statuses))
	}

	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Priority < statuses[j].Priority
	})
	if statuses[0].Name != "a" {
		t.Errorf("AllStatus()[0].Name = %s, want a", statuses[0].Name)
	}
}

func TestSelectAvailable_PreferredFound(t *testing.T) {
	r := newRegistry()
	r.Register("a", mockFactory(&mockProvider{name: "a", available: true}), ProviderConfig{Priority: 10})
	r.Register("b", mockFactory(&mockProvider{name: "b", available: true}), ProviderConfig{Priority: 20})

	p, name, err := r.SelectAvailable("b")
	if err != nil {
		t.Fatalf("SelectAvailable() error: %v", err)
	}
	if name != "b" {
		t.Errorf("SelectAvailable() name = %s, want b (preferred)", name)
	}
	if p.Name() != "b" {
		t.Errorf("SelectAvailable() provider name = %s, want b", p.Name())
	}
}

func TestSelectAvailable_ByPriority(t *testing.T) {
	r := newRegistry()
	r.Register("low", mockFactory(&mockProvider{name: "low", available: false}), ProviderConfig{Priority: 200})
	r.Register("high", mockFactory(&mockProvider{name: "high", available: true}), ProviderConfig{Priority: 10})

	p, name, err := r.SelectAvailable()
	if err != nil {
		t.Fatalf("SelectAvailable() error: %v", err)
	}
	if name != "high" {
		t.Errorf("SelectAvailable() name = %s, want high", name)
	}
	if p.Name() != "high" {
		t.Errorf("SelectAvailable() provider name = %s, want high", p.Name())
	}
}

func TestSelectAvailable_NoneAvailable(t *testing.T) {
	r := newRegistry()
	r.Register("a", mockFactory(&mockProvider{name: "a", available: false}), ProviderConfig{Priority: 10})

	_, _, err := r.SelectAvailable()
	if err == nil {
		t.Error("SelectAvailable() should return error when no provider is available")
	}
}

func TestUnregister(t *testing.T) {
	r := newRegistry()
	r.Register("test", mockFactory(&mockProvider{name: "test", available: true}))
	r.Unregister("test")

	if _, ok := r.Get("test"); ok {
		t.Error("Get() should not find unregistered provider")
	}
	if _, ok := r.GetConfig("test"); ok {
		t.Error("GetConfig() should not find unregistered config")
	}
	if _, ok := r.GetStatus("test"); ok {
		t.Error("GetStatus() should not find unregistered status")
	}
}

func TestProbeAll(t *testing.T) {
	r := newRegistry()
	r.Register("up", mockFactory(&mockProvider{name: "up", available: true}), ProviderConfig{Priority: 10})
	r.Register("down", mockFactory(&mockProvider{name: "down", available: false}), ProviderConfig{Priority: 20})

	r.ProbeAll(context.Background())

	upStatus, ok := r.GetStatus("up")
	if !ok {
		t.Fatal("GetStatus() should find up")
	}
	if !upStatus.Available {
		t.Error("up should be available after probe")
	}

	downStatus, ok := r.GetStatus("down")
	if !ok {
		t.Fatal("GetStatus() should find down")
	}
	if downStatus.Available {
		t.Error("down should not be available after probe")
	}
}
