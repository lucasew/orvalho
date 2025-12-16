package capability

// CapabilitySet defines the permissions and limits for an actor.
type CapabilitySet struct {
	// HTTP access
	AllowFetch bool
	AllowedHosts []string

	// Hardware
	AllowGPU    bool
	AllowCamera bool

	// Resources
	MaxMemoryBytes int64
	MaxExecutionTimeMs int64
}

// DefaultCapabilities returns a safe default capability set.
func DefaultCapabilities() CapabilitySet {
	return CapabilitySet{
		AllowFetch: false,
		AllowGPU: false,
		AllowCamera: false,
		MaxMemoryBytes: 1024 * 1024 * 16, // 16MB
		MaxExecutionTimeMs: 1000, // 1 second
	}
}
