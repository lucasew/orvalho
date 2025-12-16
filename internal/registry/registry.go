package registry

import (
    "sync"
    "fmt"
)

type ActorRef interface {
    SendMessage(msg any) error
    GetId() string
}

type Registry struct {
    actors map[string]ActorRef
    lock   sync.RWMutex
}

var GlobalRegistry = &Registry{
    actors: make(map[string]ActorRef),
}

func (r *Registry) Register(actor ActorRef) {
    r.lock.Lock()
    defer r.lock.Unlock()
    r.actors[actor.GetId()] = actor
}

func (r *Registry) Unregister(id string) {
    r.lock.Lock()
    defer r.lock.Unlock()
    delete(r.actors, id)
}

func (r *Registry) Get(id string) (ActorRef, bool) {
    r.lock.RLock()
    defer r.lock.RUnlock()
    a, ok := r.actors[id]
    return a, ok
}

// SendMessage sends a message to an actor by ID
func SendMessage(targetId string, msg any) error {
    actor, ok := GlobalRegistry.Get(targetId)
    if !ok {
        return fmt.Errorf("actor not found: %s", targetId)
    }
    return actor.SendMessage(msg)
}
