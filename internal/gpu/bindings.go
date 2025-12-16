package gpu

import (
    "github.com/ebitengine/purego"
    "fmt"
    "runtime"
)

// WGPU Types (Simplified)
type WGPUInstance uintptr
type WGPUAdapter uintptr
type WGPUDevice uintptr
type WGPUQueue uintptr
type WGPUBuffer uintptr

type WGPUInstanceDescriptor struct {
    NextInChain *WGPUChainedStruct
}

type WGPUChainedStruct struct {
    Next  *WGPUChainedStruct
    SType int32
}

var (
    libwgpu uintptr

    // Bindings
    wgpuCreateInstance func(*WGPUInstanceDescriptor) WGPUInstance
    wgpuInstanceRequestAdapter func(WGPUInstance, *WGPURequestAdapterOptions, uintptr, uintptr)
    // ... add more as needed
)

type WGPURequestAdapterOptions struct {
    NextInChain *WGPUChainedStruct
    CompatibleSurface uintptr
    PowerPreference int32
    ForceFallbackAdapter bool
}

// Load loads the wgpu-native library.
func Load() error {
    libName := "libwgpu_native.so"
    if runtime.GOOS == "darwin" {
        libName = "libwgpu_native.dylib"
    } else if runtime.GOOS == "windows" {
        libName = "wgpu_native.dll"
    }

    lib, err := purego.Dlopen(libName, purego.RTLD_NOW|purego.RTLD_GLOBAL)
    if err != nil {
        return fmt.Errorf("failed to load wgpu-native: %w", err)
    }
    libwgpu = lib

    purego.RegisterLibFunc(&wgpuCreateInstance, lib, "wgpuCreateInstance")
    purego.RegisterLibFunc(&wgpuInstanceRequestAdapter, lib, "wgpuInstanceRequestAdapter")

    return nil
}

// GPUManager manages GPU resources for the runtime.
type GPUManager struct {
    Instance WGPUInstance
}

func NewGPUManager() (*GPUManager, error) {
    if libwgpu == 0 {
        if err := Load(); err != nil {
            return nil, err
        }
    }

    instance := wgpuCreateInstance(nil)
    if instance == 0 {
        return nil, fmt.Errorf("failed to create WGPU instance")
    }

    return &GPUManager{
        Instance: instance,
    }, nil
}
