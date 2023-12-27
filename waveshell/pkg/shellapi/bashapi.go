// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package shellapi

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/alessio/shellescape"
	"github.com/wavetermdev/waveterm/waveshell/pkg/packet"
	"github.com/wavetermdev/waveterm/waveshell/pkg/shellenv"
)

const BaseBashOpts = `set +m; set +H; shopt -s extglob`

const BashShellVersionCmdStr = `echo bash v${BASH_VERSINFO[0]}.${BASH_VERSINFO[1]}.${BASH_VERSINFO[2]}`

// do not use these directly, call GetLocalBashMajorVersion()
var localBashMajorVersionOnce = &sync.Once{}
var localBashMajorVersion = ""

var GetBashShellStateCmds = []string{
	BashShellVersionCmdStr + ";",
	`pwd;`,
	`declare -p $(compgen -A variable);`,
	`alias -p;`,
	`declare -f;`,
	GetGitBranchCmdStr + ";",
}

func GetBashShellStateCmd() string {
	return strings.Join(GetBashShellStateCmds, ` printf "\x00\x00";`)
}

func ExecGetLocalBashShellVersion() string {
	ctx, cancelFn := context.WithTimeout(context.Background(), GetStateTimeout)
	defer cancelFn()
	ecmd := exec.CommandContext(ctx, "bash", "-c", BashShellVersionCmdStr)
	out, err := ecmd.Output()
	if err != nil {
		return ""
	}
	versionStr := strings.TrimSpace(string(out))
	if strings.Index(versionStr, "bash ") == -1 {
		// invalid shell version (only bash is supported)
		return ""
	}
	return versionStr
}

func GetLocalBashMajorVersion() string {
	localBashMajorVersionOnce.Do(func() {
		fullVersion := ExecGetLocalBashShellVersion()
		localBashMajorVersion = packet.GetBashMajorVersion(fullVersion)
	})
	return localBashMajorVersion
}

func GetBashShellState() (*packet.ShellState, error) {
	ctx, cancelFn := context.WithTimeout(context.Background(), GetStateTimeout)
	defer cancelFn()
	cmdStr := BaseBashOpts + "; " + GetBashShellStateCmd()
	ecmd := exec.CommandContext(ctx, GetLocalBashPath(), "-l", "-i", "-c", cmdStr)
	outputBytes, err := RunSimpleCmdInPty(ecmd)
	if err != nil {
		return nil, err
	}
	return shellenv.ParseShellStateOutput(outputBytes, packet.ShellType_bash)
}

func GetLocalBashPath() string {
	if runtime.GOOS == "darwin" {
		macShell := GetMacUserShell()
		if strings.Index(macShell, "bash") != -1 {
			return shellescape.Quote(macShell)
		}
	}
	return "bash"
}
