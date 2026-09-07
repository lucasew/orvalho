package bundle

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

// TransformCJS downlevels source to CommonJS ES2015 via the esbuild Go API.
func TransformCJS(source, file string) (string, error) {
	loader := api.LoaderJS
	switch strings.ToLower(filepath.Ext(file)) {
	case ".ts", ".mts", ".cts":
		loader = api.LoaderTS
	}
	result := api.Transform(source, api.TransformOptions{
		Loader:     loader,
		Sourcefile: file,
		Format:     api.FormatCommonJS,
		Target:     api.ES2015,
		Platform:   api.PlatformNeutral,
	})
	if len(result.Errors) > 0 {
		return "", fmt.Errorf("bundle: transform %s: %s", file, result.Errors[0].Text)
	}
	return string(result.Code), nil
}
