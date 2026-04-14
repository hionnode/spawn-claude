package cmd

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hionnode/spawn-claude/internal/alloy"
)

var collectorReloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Hot-reload config via POST /-/reload (no restart)",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, alloy.ReloadURL, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("POST %s: %w\n  hint: is the agent running? try `spawn-claude collector restart`", alloy.ReloadURL, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("POST %s: status %d: %s", alloy.ReloadURL, resp.StatusCode, strings.TrimSpace(string(body)))
		}
		fmt.Println("reloaded")
		return nil
	},
}

func init() {
	collectorCmd.AddCommand(collectorReloadCmd)
}
