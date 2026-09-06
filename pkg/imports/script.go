package imports

// Script is CommonJS source. The isolate evaluates it with require,
// module, and exports. File is the path inside an injected tree, if any.
type Script struct {
	Source string
	File   string
}
