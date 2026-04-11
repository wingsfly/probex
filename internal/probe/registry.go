package probe

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

type Registry struct {
	mu       sync.RWMutex
	probers  map[string]Prober
	metadata map[string]ProbeMetadata
}

func NewRegistry() *Registry {
	return &Registry{
		probers:  make(map[string]Prober),
		metadata: make(map[string]ProbeMetadata),
	}
}

// Register adds a prober to the registry. If the prober implements
// MetadataProber, its metadata is stored; otherwise minimal metadata
// is auto-generated.
func (r *Registry) Register(p Prober) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.probers[p.Name()] = p
	if mp, ok := p.(MetadataProber); ok {
		r.metadata[p.Name()] = mp.Metadata()
	} else {
		r.metadata[p.Name()] = ProbeMetadata{
			Name:            p.Name(),
			Kind:            ProbeKindBuiltin,
			Description:     p.Name() + " probe",
			ParameterSchema: EmptySchema,
		}
	}
}

// RegisterExternal registers an external probe that pushes results via API.
// No Prober is stored — external probes are not executed by the runner.
func (r *Registry) RegisterExternal(meta ProbeMetadata) {
	r.mu.Lock()
	defer r.mu.Unlock()
	meta.Kind = ProbeKindExternal
	r.metadata[meta.Name] = meta
}

// UnregisterExternal removes an external probe registration.
func (r *Registry) UnregisterExternal(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m, ok := r.metadata[name]; ok && m.Kind == ProbeKindExternal {
		delete(r.metadata, name)
	}
}

// Get returns a prober by name (for execution).
func (r *Registry) Get(name string) (Prober, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.probers[name]
	if !ok {
		return nil, fmt.Errorf("unknown probe type: %s", name)
	}
	return p, nil
}

// List returns all executable probe type names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.probers))
	for name := range r.probers {
		names = append(names, name)
	}
	return names
}

// GetMetadata returns metadata for a probe by name (any kind).
func (r *Registry) GetMetadata(name string) (ProbeMetadata, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.metadata[name]
	return m, ok
}

// ListMetadata returns metadata for all registered probes, sorted by name.
func (r *Registry) ListMetadata() []ProbeMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]ProbeMetadata, 0, len(r.metadata))
	for _, m := range r.metadata {
		result = append(result, m)
	}
	sort.Slice(result, func(i, j int) bool {
		// Order: builtin first, then script, then external; within kind, by name
		kindOrder := map[ProbeKind]int{ProbeKindBuiltin: 0, ProbeKindScript: 1, ProbeKindExternal: 2}
		if kindOrder[result[i].Kind] != kindOrder[result[j].Kind] {
			return kindOrder[result[i].Kind] < kindOrder[result[j].Kind]
		}
		return result[i].Name < result[j].Name
	})
	return result
}

// IsExternal checks if a probe name is registered as external.
func (r *Registry) IsExternal(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.metadata[name]
	return ok && m.Kind == ProbeKindExternal
}

// MarshalMetadataJSON is a helper that returns all metadata as a JSON array.
func (r *Registry) MarshalMetadataJSON() (json.RawMessage, error) {
	return json.Marshal(r.ListMetadata())
}
