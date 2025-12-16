package runtime

import (
	"context"
	"fmt"
	"sync"

	"modernc.org/quickjs"
	"github.com/lucasew/orvalho/internal/capability"
    "github.com/lucasew/orvalho/internal/gpu"
    "github.com/lucasew/orvalho/internal/camera"
    "github.com/lucasew/orvalho/internal/registry"
)

type ActorId string

// Actor represents a running actor instance.
type Actor struct {
	Id           ActorId
	vm           *quickjs.VM
	capabilities capability.CapabilitySet

	eventChan chan Event
	closeChan chan struct{}
	wg        sync.WaitGroup

	// Timer heap management
	timers       map[int]interface{} // *time.Timer or *time.Ticker
	nextTimerId  int
	timersLock   sync.Mutex
}

type Event interface {
    Process(*Actor) error
}

type EvalEvent struct {
    Script string
}

func (e EvalEvent) Process(a *Actor) error {
    _, err := a.vm.Eval(e.Script, quickjs.EvalGlobal)
    return err
}

func NewActor(id ActorId, caps capability.CapabilitySet) (*Actor, error) {
	vm, err := quickjs.NewVM()
	if err != nil {
		return nil, err
	}

    // Set memory limit
    if caps.MaxMemoryBytes > 0 {
        vm.SetMemoryLimit(uintptr(caps.MaxMemoryBytes))
    }

	actor := &Actor{
		Id:           id,
		vm:           vm,
		capabilities: caps,
		eventChan:    make(chan Event, 100),
		closeChan:    make(chan struct{}),
		timers:       make(map[int]interface{}),
        nextTimerId:  1,
	}

	// Inject capabilities
	if err := actor.injectEnv(); err != nil {
		vm.Close()
		return nil, err
	}

	return actor, nil
}

func (a *Actor) Start(ctx context.Context) {
	a.wg.Add(1)
	go a.loop(ctx)
}

func (a *Actor) Shutdown() {
	close(a.closeChan)
	a.wg.Wait()
	a.vm.Close()
}

func (a *Actor) loop(ctx context.Context) {
    fmt.Printf("Actor %s loop started\n", a.Id)
	defer a.wg.Done()

	for {
		select {
		case <-ctx.Done():
            fmt.Println("Context done")
			return
		case <-a.closeChan:
            fmt.Println("Close chan")
			return
		case evt := <-a.eventChan:
			// Execute event
            fmt.Printf("Processing event in actor %s\n", a.Id)
			err := evt.Process(a)
            if err != nil {
                fmt.Printf("Error processing event: %v\n", err)
            }
		}
	}
}

func (a *Actor) Send(evt Event) error {
    fmt.Printf("Sending event to %s\n", a.Id)
    select {
    case a.eventChan <- evt:
        fmt.Println("Event sent")
        return nil
    default:
        // Drop event or handle backpressure
        return fmt.Errorf("actor %s event channel full", a.Id)
    }
}

func (a *Actor) injectEnv() error {
    // Inject Polyfills
    fmt.Println("Injecting polyfills...")
    _, err := a.vm.Eval(Polyfills, quickjs.EvalGlobal)
    if err != nil {
        return fmt.Errorf("failed to inject polyfills: %w", err)
    }

    // Register Bridge Functions
    fmt.Println("Registering bridge...")
    if err := a.registerBridge(); err != nil {
        return fmt.Errorf("failed to register bridge: %w", err)
    }

    // Inject GPU
    if err := gpu.InjectGPU(a.vm, a.capabilities); err != nil {
        return fmt.Errorf("failed to inject GPU: %w", err)
    }

    // Inject Camera
    if err := camera.InjectCamera(a.vm, a.capabilities); err != nil {
        return fmt.Errorf("failed to inject Camera: %w", err)
    }

    // Register Actor in Registry
    registry.GlobalRegistry.Register(a)

    return nil
}

// Implement Registry Interface
func (a *Actor) GetId() string {
    return string(a.Id)
}

func (a *Actor) SendMessage(msg any) error {
    // Wrap message in Event and send
    // We need a specific event type for messages
    // For now, let's just log it or eval a callback
    fmt.Printf("Actor %s received message: %v\n", a.Id, msg)
    return nil
}

// Since Send matches the interface Send(msg any) error?
// No, Send(evt Event) vs Send(msg any).
// Registry expects Send(msg any).
// But Actor.Send takes Event.
// We need to implement the interface correctly.
