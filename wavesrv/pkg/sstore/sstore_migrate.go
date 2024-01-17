// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package sstore

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/wavetermdev/waveterm/waveshell/pkg/packet"
	"github.com/wavetermdev/waveterm/wavesrv/pkg/scbase"
)

const MigrationChunkSize = 10

type cmdMigration13Type struct {
	SessionId string
	ScreenId  string
	CmdId     string
}

type cmdMigration20Type struct {
	ScreenId string
	LineId   string
	CmdId    string
}

func getSliceChunk[T any](slice []T, chunkSize int) ([]T, []T) {
	if chunkSize >= len(slice) {
		return slice, nil
	}
	return slice[0:chunkSize], slice[chunkSize:]
}

// we're going to mark any invalid basestate versions as "invalid"
// so we can give a better error message for the FE and prompt a reset
func RunMigration30() error {
	ctx := context.Background()
	startTime := time.Now()
	updateCount := 0
	txErr := WithTx(ctx, func(tx *TxWrap) error {
		query := `SELECT riid FROM remote_instance`
		riidArr := tx.SelectStrings(query)
		for _, riid := range riidArr {
			query = `SELECT version FROM state_base WHERE basehash = (SELECT statebasehash FROM remote_instance WHERE riid = ?)`
			version := tx.GetString(query, riid)
			_, _, err := packet.ParseShellStateVersion(version)
			if err == nil {
				continue
			}
			// deal with bad versions by marking festate with an invalidshellstate flag
			query = `UPDATE remote_instance SET festate = json_set(festate, '$.invalidshellstate', '1') WHERE riid = ?`
			tx.Exec(query, riid)
			updateCount++
		}
		return nil
	})
	if txErr != nil {
		return fmt.Errorf("error running remote-instance v30 migration: %w", txErr)
	}
	log.Printf("[db] remote-instance v30 migration done: %v (%d bad versions)\n", time.Since(startTime), updateCount)
	return nil
}

func RunMigration20() error {
	ctx := context.Background()
	startTime := time.Now()
	var migrations []cmdMigration20Type
	txErr := WithTx(ctx, func(tx *TxWrap) error {
		tx.Select(&migrations, `SELECT * FROM cmd_migrate20`)
		return nil
	})
	if txErr != nil {
		return fmt.Errorf("trying to get cmd20 migrations: %w", txErr)
	}
	log.Printf("[db] got %d cmd-line migrations\n", len(migrations))
	for len(migrations) > 0 {
		var mchunk []cmdMigration20Type
		mchunk, migrations = getSliceChunk(migrations, MigrationChunkSize)
		err := processMigration20Chunk(ctx, mchunk)
		if err != nil {
			return fmt.Errorf("cmd migration failed on chunk: %w", err)
		}
	}
	log.Printf("[db] cmd line migration done: %v\n", time.Since(startTime))
	return nil
}

func processMigration20Chunk(ctx context.Context, mchunk []cmdMigration20Type) error {
	for _, mig := range mchunk {
		newFile, err := scbase.PtyOutFile(mig.ScreenId, mig.LineId)
		if err != nil {
			log.Printf("ptyoutfile(lineid) error: %v\n", err)
			continue
		}
		oldFile, err := scbase.PtyOutFile(mig.ScreenId, mig.CmdId)
		if err != nil {
			log.Printf("ptyoutfile(cmdid) error: %v\n", err)
			continue
		}
		err = os.Rename(oldFile, newFile)
		if err != nil {
			log.Printf("error renaming %s => %s: %v\n", oldFile, newFile, err)
			continue
		}
	}
	txErr := WithTx(ctx, func(tx *TxWrap) error {
		for _, mig := range mchunk {
			query := `DELETE FROM cmd_migrate20 WHERE cmdid = ?`
			tx.Exec(query, mig.CmdId)
		}
		return nil
	})
	if txErr != nil {
		return txErr
	}
	return nil
}

func RunMigration13() error {
	ctx := context.Background()
	startTime := time.Now()
	var migrations []cmdMigration13Type
	txErr := WithTx(ctx, func(tx *TxWrap) error {
		tx.Select(&migrations, `SELECT * FROM cmd_migrate`)
		return nil
	})
	if txErr != nil {
		return fmt.Errorf("trying to get cmd13 migrations: %w", txErr)
	}
	log.Printf("[db] got %d cmd-screen migrations\n", len(migrations))
	for len(migrations) > 0 {
		var mchunk []cmdMigration13Type
		mchunk, migrations = getSliceChunk(migrations, MigrationChunkSize)
		err := processMigration13Chunk(ctx, mchunk)
		if err != nil {
			return fmt.Errorf("cmd migration failed on chunk: %w", err)
		}
	}
	err := os.RemoveAll(scbase.GetSessionsDir())
	if err != nil {
		return fmt.Errorf("cannot remove old sessions dir %s: %w\n", scbase.GetSessionsDir(), err)
	}
	txErr = WithTx(ctx, func(tx *TxWrap) error {
		query := `UPDATE client SET cmdstoretype = 'screen'`
		tx.Exec(query)
		return nil
	})
	if txErr != nil {
		return fmt.Errorf("cannot change client cmdstoretype: %w", err)
	}
	log.Printf("[db] cmd screen migration done: %v\n", time.Since(startTime))
	return nil
}

func processMigration13Chunk(ctx context.Context, mchunk []cmdMigration13Type) error {
	for _, mig := range mchunk {
		newFile, err := scbase.PtyOutFile(mig.ScreenId, mig.CmdId)
		if err != nil {
			log.Printf("ptyoutfile error: %v\n", err)
			continue
		}
		oldFile, err := scbase.PtyOutFile_Sessions(mig.SessionId, mig.CmdId)
		if err != nil {
			log.Printf("ptyoutfile_sessions error: %v\n", err)
			continue
		}
		err = os.Rename(oldFile, newFile)
		if err != nil {
			log.Printf("error renaming %s => %s: %v\n", oldFile, newFile, err)
			continue
		}
	}
	txErr := WithTx(ctx, func(tx *TxWrap) error {
		for _, mig := range mchunk {
			query := `DELETE FROM cmd_migrate WHERE cmdid = ?`
			tx.Exec(query, mig.CmdId)
		}
		return nil
	})
	if txErr != nil {
		return txErr
	}
	return nil
}
