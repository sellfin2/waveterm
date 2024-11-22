// Copyright 2024, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package windowservice

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/wavetermdev/waveterm/pkg/eventbus"
	"github.com/wavetermdev/waveterm/pkg/tsgen/tsgenmeta"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wcore"
	"github.com/wavetermdev/waveterm/pkg/wlayout"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

const DefaultTimeout = 2 * time.Second

type WindowService struct{}

func (svc *WindowService) GetWindow(windowId string) (*waveobj.Window, error) {
	ctx, cancelFn := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancelFn()
	window, err := wstore.DBGet[*waveobj.Window](ctx, windowId)
	if err != nil {
		return nil, fmt.Errorf("error getting window: %w", err)
	}
	return window, nil
}

func (svc *WindowService) MakeWindow(ctx context.Context) (*waveobj.Window, error) {
	log.Println("MakeWindow")
	window, err := wcore.CreateWindow(ctx, nil, "")
	if err != nil {
		log.Printf("error creating window: %v\n", err)
		return nil, err
	}
	log.Printf("New window: %v\n", window)
	ws, err := wcore.GetWorkspace(ctx, window.WorkspaceId)
	if err != nil {
		log.Printf("error getting workspace: %v\n", err)
		return nil, err
	}
	log.Printf("New workspace: %v\n", ws)
	err = wlayout.BootstrapNewWorkspaceLayout(ctx, ws)
	if err != nil {
		return window, err
	}
	return window, nil
}

func (svc *WindowService) SetWindowPosAndSize_Meta() tsgenmeta.MethodMeta {
	return tsgenmeta.MethodMeta{
		Desc:     "set window position and size",
		ArgNames: []string{"windowId", "pos", "size"},
	}
}

func (ws *WindowService) SetWindowPosAndSize(ctx context.Context, windowId string, pos *waveobj.Point, size *waveobj.WinSize) (waveobj.UpdatesRtnType, error) {
	if pos == nil && size == nil {
		return nil, nil
	}
	ctx = waveobj.ContextWithUpdates(ctx)
	win, err := wstore.DBMustGet[*waveobj.Window](ctx, windowId)
	if err != nil {
		return nil, err
	}
	if pos != nil {
		win.Pos = *pos
	}
	if size != nil {
		win.WinSize = *size
	}
	win.IsNew = false
	err = wstore.DBUpdate(ctx, win)
	if err != nil {
		return nil, err
	}
	return waveobj.ContextGetUpdatesRtn(ctx), nil
}

func (svc *WindowService) MoveBlockToNewWindow_Meta() tsgenmeta.MethodMeta {
	return tsgenmeta.MethodMeta{
		Desc:     "move block to new window",
		ArgNames: []string{"ctx", "currentTabId", "blockId"},
	}
}

func (svc *WindowService) MoveBlockToNewWindow(ctx context.Context, currentTabId string, blockId string) (waveobj.UpdatesRtnType, error) {
	log.Printf("MoveBlockToNewWindow(%s, %s)", currentTabId, blockId)
	ctx = waveobj.ContextWithUpdates(ctx)
	tab, err := wstore.DBMustGet[*waveobj.Tab](ctx, currentTabId)
	if err != nil {
		return nil, fmt.Errorf("error getting tab: %w", err)
	}
	log.Printf("tab.BlockIds[%s]: %v", tab.OID, tab.BlockIds)
	var foundBlock bool
	for _, tabBlockId := range tab.BlockIds {
		if tabBlockId == blockId {
			foundBlock = true
			break
		}
	}
	if !foundBlock {
		return nil, fmt.Errorf("block not found in current tab")
	}
	newWindow, err := wcore.CreateWindow(ctx, nil, "")
	if err != nil {
		return nil, fmt.Errorf("error creating window: %w", err)
	}
	ws, err := wcore.GetWorkspace(ctx, newWindow.WorkspaceId)
	if err != nil {
		return nil, fmt.Errorf("error getting workspace: %w", err)
	}
	err = wstore.MoveBlockToTab(ctx, currentTabId, ws.ActiveTabId, blockId)
	if err != nil {
		return nil, fmt.Errorf("error moving block to tab: %w", err)
	}
	eventbus.SendEventToElectron(eventbus.WSEventType{
		EventType: eventbus.WSEvent_ElectronNewWindow,
		Data:      newWindow.OID,
	})
	windowCreated := eventbus.BusyWaitForWindowId(newWindow.OID, 2*time.Second)
	if !windowCreated {
		return nil, fmt.Errorf("new window not created")
	}
	wlayout.QueueLayoutActionForTab(ctx, currentTabId, waveobj.LayoutActionData{
		ActionType: wlayout.LayoutActionDataType_Remove,
		BlockId:    blockId,
	})
	wlayout.QueueLayoutActionForTab(ctx, ws.ActiveTabId, waveobj.LayoutActionData{
		ActionType: wlayout.LayoutActionDataType_Insert,
		BlockId:    blockId,
		Focused:    true,
	})
	return waveobj.ContextGetUpdatesRtn(ctx), nil
}

func (svc *WindowService) SwitchWorkspace_Meta() tsgenmeta.MethodMeta {
	return tsgenmeta.MethodMeta{
		ArgNames: []string{"windowId", "workspaceId"},
	}
}

func (svc *WindowService) SwitchWorkspace(ctx context.Context, windowId string, workspaceId string) (*waveobj.Workspace, error) {
	ctx = waveobj.ContextWithUpdates(ctx)
	return wcore.SwitchWorkspace(ctx, windowId, workspaceId)
}

func (svc *WindowService) CloseWindow_Meta() tsgenmeta.MethodMeta {
	return tsgenmeta.MethodMeta{
		ArgNames: []string{"windowId", "fromElectron"},
	}
}

func (svc *WindowService) CloseWindow(ctx context.Context, windowId string, fromElectron bool) error {
	ctx = waveobj.ContextWithUpdates(ctx)
	return wcore.CloseWindow(ctx, windowId, fromElectron)
}
