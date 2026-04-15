package crawler

import (
	"context"
	"fmt"
	"sync"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

// CrawlConfig holds configuration for a crawl session.
type CrawlConfig struct {
	EntryURL   string
	Auth       db.CrawlAuthConfig
	Depth      int
	SameOrigin bool
	Blocklist  []string
	Limit      int
}

// ProgressCallback is called by engines to report incremental progress.
type ProgressCallback func(progress db.CrawlProgress, graph db.CrawlGraph, requests []db.CapturedRequest)

// CrawlEngine defines the interface that all crawl engines must implement.
type CrawlEngine interface {
	Name() string
	Crawl(ctx context.Context, cfg CrawlConfig, onProgress ProgressCallback) (db.CrawlGraph, []db.CapturedRequest, error)
}

var (
	enginesMu sync.RWMutex
	engines   = map[string]CrawlEngine{}
)

// RegisterEngine adds an engine to the global registry.
func RegisterEngine(e CrawlEngine) {
	enginesMu.Lock()
	defer enginesMu.Unlock()
	engines[e.Name()] = e
}

// GetEngine returns a registered engine by name.
func GetEngine(name string) (CrawlEngine, error) {
	enginesMu.RLock()
	defer enginesMu.RUnlock()
	e, ok := engines[name]
	if !ok {
		return nil, fmt.Errorf("unknown crawl engine: %q", name)
	}
	return e, nil
}

// AvailableEngines returns the names of all registered engines.
func AvailableEngines() []string {
	enginesMu.RLock()
	defer enginesMu.RUnlock()
	names := make([]string, 0, len(engines))
	for name := range engines {
		names = append(names, name)
	}
	return names
}

func init() {
	RegisterEngine(&RodEngine{})
	RegisterEngine(&CollyEngine{})
	RegisterEngine(&ZAPEngine{})
}
