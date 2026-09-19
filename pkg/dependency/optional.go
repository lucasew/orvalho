package dependency

import (
	"os"
	"runtime"
	"strings"
)

// keepOptional reports whether an optional Dependency should be installed
// (INV-18): packument cpu lists wasm32, or os/cpu/libc match this host.
func keepOptional(cpu, osName, libc []string) bool {
	return platformOK(cpu, osName, libc, npmCPU(runtime.GOARCH), npmOS(runtime.GOOS), hostLibc())
}

func platformOK(cpu, osName, libc []string, hostCPU, hostOS, hostLibc string) bool {
	if hasFold(cpu, "wasm32") {
		return true
	}
	return matchList(osName, hostOS) && matchList(cpu, hostCPU) && matchList(libc, hostLibc)
}

func matchList(list []string, host string) bool {
	if len(list) == 0 {
		return true
	}
	sawPositive := false
	for _, v := range list {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "" {
			continue
		}
		if strings.HasPrefix(v, "!") {
			if strings.TrimPrefix(v, "!") == host {
				return false
			}
			continue
		}
		sawPositive = true
		if v == host {
			return true
		}
	}
	return !sawPositive
}

func hasFold(list []string, want string) bool {
	for _, v := range list {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

func npmCPU(arch string) string {
	switch arch {
	case "amd64":
		return "x64"
	case "386":
		return "ia32"
	default:
		return arch
	}
}

func npmOS(goos string) string {
	if goos == "windows" {
		return "win32"
	}
	return goos
}

func hostLibc() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	}
	if _, err := os.Stat("/lib/ld-musl-" + arch + ".so.1"); err == nil {
		return "musl"
	}
	return "glibc"
}
