//go:build !darwin && !linux && !freebsd

package taskquality

import "os/exec"

func configureCLIProcess(cmd *exec.Cmd) {}

func finishCLIProcess(cmd *exec.Cmd) {}
