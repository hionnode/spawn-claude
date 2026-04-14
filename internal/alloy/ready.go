package alloy

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

func WaitReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	var lastErr error
	for {
		if time.Now().After(deadline) {
			if lastErr != nil {
				return fmt.Errorf("alloy not ready at %s after %s: %w", ReadyURL, timeout, lastErr)
			}
			return fmt.Errorf("alloy not ready at %s after %s", ReadyURL, timeout)
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ReadyURL, nil)
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

func CheckReady(ctx context.Context) (bool, int, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ReadyURL, nil)
	resp, err := client.Do(req)
	if err != nil {
		return false, 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK, resp.StatusCode, nil
}
