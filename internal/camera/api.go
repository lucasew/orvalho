package camera

import (
    "fmt"
    "github.com/lucasew/orvalho/internal/capability"
    "modernc.org/quickjs"
)

// InjectCamera injects the Camera API into the QuickJS context.
func InjectCamera(vm *quickjs.VM, caps capability.CapabilitySet) error {
    if !caps.AllowCamera {
        return nil
    }

    // Mock implementation for now, or use V4L2 via syscall
    err := vm.RegisterFunc("_go_camera_capture", func() int {
        fmt.Println("Camera capture requested")
        return 1 // Stream ID
    }, false)
    if err != nil { return err }

    script := `
    if (!globalThis.env) globalThis.env = {};
    globalThis.env.CAMERA = {
        capture: function(constraints) {
            const id = _go_camera_capture();
            return Promise.resolve(id);
        }
    };
    `
    _, err = vm.Eval(script, quickjs.EvalGlobal)
    return err
}
