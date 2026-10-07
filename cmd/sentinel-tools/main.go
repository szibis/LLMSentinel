// sentinel-tools provides native Go client integrations, lab management and CI helpers.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/szibis/claude-escalate/internal/clientcapture"
	"github.com/szibis/claude-escalate/internal/clientcontrol"
	"github.com/szibis/claude-escalate/internal/lab"
	"github.com/szibis/claude-escalate/internal/labdashboard"
	"github.com/szibis/claude-escalate/internal/labstatus"
	"github.com/szibis/claude-escalate/internal/qwensmoke"
	"github.com/szibis/claude-escalate/internal/release"
	"github.com/szibis/claude-escalate/internal/taskquality"
)

func run(args []string, in io.Reader, out, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "help" {
		fmt.Fprintln(out, "Usage: sentinel-tools <capture|control|statusline|dashboard|lab|runner|ci-lab|role-check|smoke|quality|cli-quality|release> [options]")
		return 0
	}
	var entry func([]string, io.Reader, io.Writer, io.Writer) int
	switch args[0] {
	case "capture":
		entry = clientcapture.Run
	case "control":
		entry = clientcontrol.Run
	case "statusline":
		entry = labstatus.Run
	case "dashboard":
		entry = labdashboard.Run
	case "lab":
		entry = lab.Run
	case "runner":
		entry = lab.RunRunner
	case "ci-lab":
		entry = lab.RunCI
	case "role-check":
		entry = qwensmoke.RunRoles
	case "smoke":
		entry = qwensmoke.Run
	case "release":
		entry = release.Run
	case "quality":
		entry = taskquality.Run
	case "cli-quality":
		entry = taskquality.RunCLI
	default:
		fmt.Fprintln(stderr, "unknown sentinel-tools command:", args[0])
		return 2
	}
	return entry(args[1:], in, out, stderr)
}

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
