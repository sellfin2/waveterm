package sstore

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path"

	"github.com/google/uuid"
	"github.com/scripthaus-dev/mshell/pkg/base"
	"github.com/scripthaus-dev/mshell/pkg/cirfile"
	"github.com/scripthaus-dev/sh2-server/pkg/scbase"
)

func CreateCmdPtyFile(ctx context.Context, sessionId string, cmdId string, maxSize int64) error {
	ptyOutFileName, err := scbase.PtyOutFile(sessionId, cmdId)
	if err != nil {
		return err
	}
	f, err := cirfile.CreateCirFile(ptyOutFileName, maxSize)
	if err != nil {
		return err
	}
	return f.Close()
}

func AppendToCmdPtyBlob(ctx context.Context, sessionId string, cmdId string, data []byte, pos int64) (*PtyDataUpdate, error) {
	if pos < 0 {
		return nil, fmt.Errorf("invalid seek pos '%d' in AppendToCmdPtyBlob", pos)
	}
	ptyOutFileName, err := scbase.PtyOutFile(sessionId, cmdId)
	if err != nil {
		return nil, err
	}
	f, err := cirfile.OpenCirFile(ptyOutFileName)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	err = f.WriteAt(ctx, data, pos)
	if err != nil {
		return nil, err
	}
	data64 := base64.StdEncoding.EncodeToString(data)
	update := &PtyDataUpdate{
		SessionId:  sessionId,
		CmdId:      cmdId,
		PtyPos:     pos,
		PtyData64:  data64,
		PtyDataLen: int64(len(data)),
	}
	return update, nil
}

// returns (offset, data, err)
func ReadFullPtyOutFile(ctx context.Context, sessionId string, cmdId string) (int64, []byte, error) {
	ptyOutFileName, err := scbase.PtyOutFile(sessionId, cmdId)
	if err != nil {
		return 0, nil, err
	}
	f, err := cirfile.OpenCirFile(ptyOutFileName)
	if err != nil {
		return 0, nil, err
	}
	defer f.Close()
	return f.ReadAll(ctx)
}

type SessionDiskSizeType struct {
	NumFiles   int
	TotalSize  int64
	ErrorCount int
}

func directorySize(dirName string) (SessionDiskSizeType, error) {
	var rtn SessionDiskSizeType
	entries, err := os.ReadDir(dirName)
	if err != nil {
		return rtn, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			rtn.ErrorCount++
			continue
		}
		finfo, err := entry.Info()
		if err != nil {
			rtn.ErrorCount++
			continue
		}
		rtn.NumFiles++
		rtn.TotalSize += finfo.Size()
	}
	return rtn, nil
}

func SessionDiskSize(sessionId string) (SessionDiskSizeType, error) {
	sessionDir, err := base.EnsureSessionDir(sessionId)
	if err != nil {
		return SessionDiskSizeType{}, err
	}
	return directorySize(sessionDir)
}

func FullSessionDiskSize() (map[string]SessionDiskSizeType, error) {
	sdir := base.GetSessionsDir()
	entries, err := os.ReadDir(sdir)
	if err != nil {
		return nil, err
	}
	rtn := make(map[string]SessionDiskSizeType)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		_, err = uuid.Parse(name)
		if err != nil {
			continue
		}
		diskSize, err := directorySize(path.Join(sdir, name))
		if err != nil {
			continue
		}
		rtn[name] = diskSize
	}
	return rtn, nil
}

func DeletePtyOutFile(ctx context.Context, sessionId string, cmdId string) error {
	ptyOutFileName, err := scbase.PtyOutFile(sessionId, cmdId)
	if err != nil {
		return err
	}
	return os.Remove(ptyOutFileName)
}

func DeleteSessionDir(ctx context.Context, sessionId string) error {
	sessionDir, err := base.EnsureSessionDir(sessionId)
	if err != nil {
		return fmt.Errorf("error getting sessiondir: %w", err)
	}
	return os.RemoveAll(sessionDir)
}
