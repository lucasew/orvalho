package ula

import (
	"fmt"
	"sync"
)

// AllocationKey identifies a logical actor within a mesh device.
// Mesh is opaque owner/mesh context (e.g. manager public id).
// Actor is a stable install/actor identity chosen by the manager.
type AllocationKey struct {
	Mesh   string
	Device uint16
	Actor  string
}

func (k AllocationKey) validate() error {
	if k.Mesh == "" {
		return fmt.Errorf("ula: mesh context is required")
	}
	if k.Actor == "" {
		return fmt.Errorf("ula: actor identity is required")
	}
	return nil
}

// deviceScope keys allocations under one mesh device /64.
type deviceScope struct {
	Mesh   string
	Device uint16
}

// Store persists actor → interface-id mappings. Address bytes are not stored;
// they are pure functions of mesh context, device id, and interface id.
//
// ClaimNext must be atomic with respect to other writers for the same
// mesh+device so concurrent allocators cannot assign the same iid.
type Store interface {
	// Get returns the recorded interface id for key, if any.
	Get(key AllocationKey) (iid uint64, ok bool, err error)

	// ClaimNext returns the existing iid for key, or records and returns the
	// smallest free iid >= min under the key's mesh+device.
	ClaimNext(key AllocationKey, min uint64) (iid uint64, err error)

	// Put records iid for key. Returns an error if key already maps to a
	// different iid, or if iid is already used by another actor under the
	// same mesh+device.
	Put(key AllocationKey, iid uint64) error

	// ListIIDs returns every interface id allocated under mesh+device.
	ListIIDs(mesh string, device uint16) ([]uint64, error)
}

// MemoryStore is an in-memory Store suitable for tests and single-process use.
// All methods are safe for concurrent use.
type MemoryStore struct {
	mu sync.Mutex
	// actor -> iid
	byActor map[AllocationKey]uint64
	// device scope -> iid -> actor id
	byIID map[deviceScope]map[uint64]string
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		byActor: make(map[AllocationKey]uint64),
		byIID:   make(map[deviceScope]map[uint64]string),
	}
}

// Get implements Store.
func (s *MemoryStore) Get(key AllocationKey) (uint64, bool, error) {
	if err := key.validate(); err != nil {
		return 0, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	iid, ok := s.byActor[key]
	return iid, ok, nil
}

// ClaimNext implements Store.
func (s *MemoryStore) ClaimNext(key AllocationKey, min uint64) (uint64, error) {
	if err := key.validate(); err != nil {
		return 0, err
	}
	if min < MinActorIID {
		return 0, fmt.Errorf("ula: min interface id %d is below MinActorIID (%d)", min, MinActorIID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.byActor[key]; ok {
		return existing, nil
	}

	scope := deviceScope{Mesh: key.Mesh, Device: key.Device}
	iids := s.byIID[scope]
	if iids == nil {
		iids = make(map[uint64]string)
		s.byIID[scope] = iids
	}

	var next uint64 = min
	for {
		if _, busy := iids[next]; !busy {
			break
		}
		if next == ^uint64(0) {
			return 0, fmt.Errorf("ula: no free interface ids under mesh %q device %d", key.Mesh, key.Device)
		}
		next++
	}

	s.byActor[key] = next
	iids[next] = key.Actor
	return next, nil
}

// Put implements Store.
func (s *MemoryStore) Put(key AllocationKey, iid uint64) error {
	if err := key.validate(); err != nil {
		return err
	}
	if iid < MinActorIID {
		return fmt.Errorf("ula: cannot record reserved interface id %d", iid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.byActor[key]; ok {
		if existing != iid {
			return fmt.Errorf("ula: actor %q already has interface id %d", key.Actor, existing)
		}
		return nil // idempotent
	}

	scope := deviceScope{Mesh: key.Mesh, Device: key.Device}
	iids := s.byIID[scope]
	if iids == nil {
		iids = make(map[uint64]string)
		s.byIID[scope] = iids
	}
	if owner, taken := iids[iid]; taken {
		return fmt.Errorf("ula: interface id %d already used by actor %q on device %d", iid, owner, key.Device)
	}

	s.byActor[key] = iid
	iids[iid] = key.Actor
	return nil
}

// ListIIDs implements Store.
func (s *MemoryStore) ListIIDs(mesh string, device uint16) ([]uint64, error) {
	if mesh == "" {
		return nil, fmt.Errorf("ula: mesh context is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	scope := deviceScope{Mesh: mesh, Device: device}
	iids := s.byIID[scope]
	out := make([]uint64, 0, len(iids))
	for iid := range iids {
		out = append(out, iid)
	}
	return out, nil
}
