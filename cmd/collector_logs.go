package cmd

import (
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/hionnode/spawn-claude/internal/alloy"
)

var logsFollow bool

var collectorLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Print Alloy stderr.log (use -f to follow)",
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := alloy.ResolvePaths()
		if err != nil {
			return err
		}
		if logsFollow {
			bin, err := exec.LookPath("tail")
			if err != nil {
				return err
			}
			return syscall.Exec(bin, []string{"tail", "-f", paths.StderrLog}, os.Environ())
		}
		f, err := os.Open(paths.StderrLog)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(os.Stdout, f)
		return err
	},
}

func init() {
	collectorLogsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "follow log output (tail -f)")
	collectorCmd.AddCommand(collectorLogsCmd)
}
