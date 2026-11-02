// Command healthcheck exits 0 when the API on this host answers its health
// probe. The runtime image has no shell or curl, so container healthchecks
// run this instead.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

// probeURL is fixed: the probe only ever asks the API in its own container.
const probeURL = "http://127.0.0.1:5000/api/v1/health"

func main() {
	if err := check(); err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		os.Exit(1)
	}
}

func check() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("get %s: %w", probeURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %s: status %d", probeURL, resp.StatusCode)
	}
	return nil
}
