// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmdrunner

import (
	"fmt"
	"os"
	"testing"

	"github.com/wavetermdev/waveterm/wavesrv/pkg/rtnstate"
)

func xTestParseAliases(t *testing.T) {
	m, err := rtnstate.ParseAliases(`
alias cdg='cd work/gopath/src/github.com/sawka'
alias s='scripthaus'
alias x='ls;ls"'
alias foo="bar \"hello\""
alias x=y
`)
	if err != nil {
		fmt.Printf("err: %v\n", err)
		return
	}
	fmt.Printf("m: %#v\n", m)
}

func xTestParseFuncs(t *testing.T) {
	file, err := os.ReadFile("./linux-decls.txt")
	if err != nil {
		t.Fatalf("error reading linux-decls: %v", err)
	}
	m, err := rtnstate.ParseFuncs(string(file))
	if err != nil {
		t.Fatalf("error parsing funcs: %v", err)
	}
	for key, val := range m {
		fmt.Printf("func: %s %d\n", key, len(val))
	}
}

func testRSC(t *testing.T, cmd string, expected bool) {
	rtn := IsReturnStateCommand(cmd)
	if rtn != expected {
		t.Errorf("cmd [%s], rtn=%v, expected=%v", cmd, rtn, expected)
	}
}

func TestIsReturnStateCommand(t *testing.T) {
	testRSC(t, "FOO=1", true)
	testRSC(t, "FOO=1 X=2", true)
	testRSC(t, "ls", false)
	testRSC(t, "export X", true)
	testRSC(t, "export X=1", true)
	testRSC(t, "declare -x FOO=1", true)
	testRSC(t, "source ./test", true)
	testRSC(t, "unset FOO BAR", true)
	testRSC(t, "FOO=1; ls", true)
	testRSC(t, ". ./test", true)
	testRSC(t, "{ FOO=6; }", true)
	testRSC(t, "cd foo && ls -l", true)
	testRSC(t, "ls -l && ./foo || git checkout main", true)
	testRSC(t, "./foo || ./bar", false)
	testRSC(t, ". foo.sh", true)
	testRSC(t, "cd work; conda activate myenv", true)
	testRSC(t, "asdf foo", true)
}
