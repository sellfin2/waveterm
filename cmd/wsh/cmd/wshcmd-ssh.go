// Copyright 2024, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
)

var identityFiles []string

var sshCmd = &cobra.Command{
	Use:     "ssh",
	Short:   "connect this terminal to a remote host",
	Args:    cobra.ExactArgs(1),
	Run:     sshRun,
	PreRunE: preRunSetupRpcClient,
}

func init() {
	sshCmd.Flags().StringArrayVarP(&identityFiles, "identity_file", "i", []string{}, "add an identity file for publickey authentication")
	rootCmd.AddCommand(sshCmd)
}

func sshRun(cmd *cobra.Command, args []string) {
	sshArg := args[0]
	blockId := RpcContext.BlockId
	if blockId == "" {
		WriteStderr("[error] cannot determine blockid (not in JWT)\n")
		return
	}
	// first, make a connection independent of the block
	connOpts := wshrpc.SshKeywords{
		HostName:     sshArg,
		IdentityFile: identityFiles,
	}
	wshclient.ConnConnectCommand(RpcClient, &connOpts, nil)

	// now, with that made, it will be straightforward to connect
	data := wshrpc.CommandSetMetaData{
		ORef: waveobj.MakeORef(waveobj.OType_Block, blockId),
		Meta: map[string]any{
			waveobj.MetaKey_Connection: sshArg,
		},
	}
	err := wshclient.SetMetaCommand(RpcClient, data, nil)
	if err != nil {
		WriteStderr("[error] setting switching connection: %v\n", err)
		return
	}
	WriteStderr("switched connection to %q\n", sshArg)
}
