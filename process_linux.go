// +build linux

package wrapper

import (
	"os"
	"os/exec"
	"syscall"
)

func setCmdParams(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: os.Kill,
	}

}
