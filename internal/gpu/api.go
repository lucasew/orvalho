package gpu

import (
    "github.com/lucasew/orvalho/internal/capability"
    "modernc.org/quickjs"
    "fmt"
)

// InjectGPU injects the GPU API into the QuickJS context.
// It mocks the functionality if the GPU library is not available.
func InjectGPU(vm *quickjs.VM, caps capability.CapabilitySet) error {
    if !caps.AllowGPU {
        return nil
    }

    // Attempt to load GPU manager (will fail if lib missing)
    manager, err := NewGPUManager()
    isMock := err != nil
    if isMock {
        fmt.Println("GPU library not found, using mock implementation")
    }

    // dispatch
    err = vm.RegisterFunc("_go_gpu_dispatch", func(workgroups int) {
        if isMock {
            fmt.Printf("Mock GPU Dispatch: %d workgroups\n", workgroups)
        } else {
             // Real implementation
             _ = manager // usage
        }
    }, false)
    if err != nil { return err }

    // createBuffer
    err = vm.RegisterFunc("_go_gpu_createBuffer", func(size int) int {
        if isMock {
            fmt.Printf("Mock GPU CreateBuffer: %d bytes\n", size)
            return 123 // Mock ID
        } else {
            return 0
        }
    }, false)
    if err != nil { return err }

    // JS Wrapper
    script := `
    if (!globalThis.env) globalThis.env = {};
    globalThis.env.GPU = {
        dispatch: function(workgroups) {
            _go_gpu_dispatch(workgroups);
            return Promise.resolve();
        },
        createBuffer: function(size) {
            return _go_gpu_createBuffer(size);
        }
    };
    `
    _, err = vm.Eval(script, quickjs.EvalGlobal)
    return err
}
