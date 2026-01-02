package binding

import (
    "github.com/lucasew/orvalho/pkg/device"
    gpuDevice "github.com/lucasew/orvalho/pkg/device/gpu"
    cameraDevice "github.com/lucasew/orvalho/pkg/device/camera"
    "modernc.org/quickjs"
)

// InjectDevices injects the `env.DEVICES` API.
func InjectDevices(vm *quickjs.VM, devices []device.Device) error {
    // 1. Create `env` if not exists
    _, err := vm.Eval(`if (!globalThis.env) globalThis.env = {};`, quickjs.EvalGlobal)
    if err != nil {
        return err
    }

    // 2. Prepare Go functions

    // list(type) -> []string (device IDs)
    err = vm.RegisterFunc("_go_devices_list", func(typeStr string) []any {
        var ids []any
        for _, d := range devices {
            if string(d.Type()) == typeStr {
                ids = append(ids, d.ID())
            }
        }
        return ids
    }, false)
    if err != nil { return err }

    // get(id) -> DeviceObject
    // We register specific functions for specific device types and return a JS object
    // wrapping them.

    // For simplicity, we can have a _go_device_dispatch(id, ...) dispatching to the right device.

    err = vm.RegisterFunc("_go_device_dispatch", func(id string, workgroups int) {
        for _, d := range devices {
            if d.ID() == id {
               if gpu, ok := d.(gpuDevice.Device); ok {
                   gpu.Dispatch(workgroups)
               }
            }
        }
    }, false)
    if err != nil { return err }

    err = vm.RegisterFunc("_go_device_capture", func(id string) int {
        for _, d := range devices {
            if d.ID() == id {
               if cam, ok := d.(cameraDevice.Device); ok {
                   id, _ := cam.Capture()
                   return id
               }
            }
        }
        return -1
    }, false)
    if err != nil { return err }

    // 3. JS Wrapper
    script := `
    globalThis.env.DEVICES = {
        list: function(type) {
            return _go_devices_list(type);
        },
        get: function(id) {
            return new Promise((resolve, reject) => {
                // Determine type from ID or lookup?
                // For this mock, we return an object with all methods,
                // and the go side filters valid calls.
                resolve({
                    dispatch: function(params) {
                        _go_device_dispatch(id, params.workgroups || 1);
                        return Promise.resolve();
                    },
                    capture: function(constraints) {
                        const streamId = _go_device_capture(id);
                        return Promise.resolve(streamId);
                    }
                });
            });
        }
    };
    `
    _, err = vm.Eval(script, quickjs.EvalGlobal)
    return err
}
