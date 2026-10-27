// Command openapi prints the control plane's OpenAPI document. The
// TypeScript client in packages/api-client is generated from its output.
package main

import (
	"encoding/json"
	"os"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/handlers"
)

func main() {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(handlers.Document()); err != nil {
		os.Exit(1)
	}
}
