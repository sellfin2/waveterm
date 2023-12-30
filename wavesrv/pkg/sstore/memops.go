// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// in-memory storage for waveterm server
package sstore

import (
	"log"
	"sync"

	"github.com/wavetermdev/waveterm/waveshell/pkg/packet"
	"github.com/wavetermdev/waveterm/wavesrv/pkg/utilfn"
)

// global lock for all memory operations
// memory ops are very fast, so this is not a bottleneck
var MemLock *sync.Mutex = &sync.Mutex{}
var ScreenMemStore map[string]*ScreenMemState = make(map[string]*ScreenMemState) // map of screenid -> ScreenMemState

const (
	ScreenIndicator_None    = ""
	ScreenIndicator_Error   = "error"
	ScreenIndicator_Success = "success"
	ScreenIndicator_Output  = "output"
)

var screenIndicatorLevels map[string]int = map[string]int{
	ScreenIndicator_None:    0,
	ScreenIndicator_Output:  1,
	ScreenIndicator_Success: 2,
	ScreenIndicator_Error:   3,
}

func dumpScreenMemStore() {
	MemLock.Lock()
	defer MemLock.Unlock()
	for k, v := range ScreenMemStore {
		log.Printf("  ScreenMemStore[%s] = %+v\n", k, v)
	}
}

// returns true if i1 > i2
func isIndicatorGreater(i1 string, i2 string) bool {
	return screenIndicatorLevels[i1] > screenIndicatorLevels[i2]
}

type OpenAICmdInfoChatStore struct {
	MessageCount int                                `json:"messagecount"`
	Messages     []*packet.OpenAICmdInfoChatMessage `json:"messages"`
}

type ScreenMemState struct {
	NumRunningCommands int                     `json:"numrunningcommands,omitempty"`
	IndicatorType      string                  `json:"indicatortype,omitempty"`
	CmdInputText       utilfn.StrWithPos       `json:"cmdinputtext,omitempty"`
	CmdInputSeqNum     int                     `json:"cmdinputseqnum,omitempty"`
	AICmdInfoChat      *OpenAICmdInfoChatStore `json:"aicmdinfochat,omitempty"`
}

func ScreenMemInitCmdInfoChat(screenId string) {
	ScreenMemStore[screenId].AICmdInfoChat = &OpenAICmdInfoChatStore{MessageCount: 0, Messages: []*packet.OpenAICmdInfoChatMessage{}}
}

func ScreenMemAddCmdInfoChatMessage(screenId string, msg *packet.OpenAICmdInfoChatMessage) {
	if ScreenMemStore[screenId].AICmdInfoChat == nil {
		log.Printf("AICmdInfoChat is null, creating")
		ScreenMemInitCmdInfoChat(screenId)
	}

	CmdInfoChat := ScreenMemStore[screenId].AICmdInfoChat
	CmdInfoChat.Messages = append(CmdInfoChat.Messages, msg)
	CmdInfoChat.MessageCount++
}

func ScreenMemGetCmdInfoMessageCount(screenId string) int {
	if ScreenMemStore[screenId].AICmdInfoChat == nil {
		log.Printf("AICmdInfoChat is null, creating")
		ScreenMemInitCmdInfoChat(screenId)
	}
	return ScreenMemStore[screenId].AICmdInfoChat.MessageCount
}

func ScreenMemGetCmdInfoChat(screenId string) *OpenAICmdInfoChatStore {
	if ScreenMemStore[screenId].AICmdInfoChat == nil {
		log.Printf("AICmdInfoChat is null, creating")
		ScreenMemInitCmdInfoChat(screenId)
	}
	return ScreenMemStore[screenId].AICmdInfoChat
}

func ScreenMemUpdateCmdInfoChatMessage(screenId string, messageID int, msg *packet.OpenAICmdInfoChatMessage) {
	if ScreenMemStore[screenId].AICmdInfoChat == nil {
		log.Printf("AICmdInfoChat is null, creating")
		ScreenMemInitCmdInfoChat(screenId)
	}
	CmdInfoChat := ScreenMemStore[screenId].AICmdInfoChat
	if messageID >= 0 && messageID < len(CmdInfoChat.Messages) {
		log.Printf("len of message: %d", len(CmdInfoChat.Messages))
		CmdInfoChat.Messages[messageID] = msg
		log.Printf("len of message: %d", len(CmdInfoChat.Messages))
	} else {
		log.Printf("Message Id out of range: %d", messageID)
	}
}

func ScreenMemSetCmdInputText(screenId string, sp utilfn.StrWithPos, seqNum int) {
	MemLock.Lock()
	defer MemLock.Unlock()
	if ScreenMemStore[screenId] == nil {
		ScreenMemStore[screenId] = &ScreenMemState{}
	}
	if seqNum <= ScreenMemStore[screenId].CmdInputSeqNum {
		return
	}
	ScreenMemStore[screenId].CmdInputText = sp
	ScreenMemStore[screenId].CmdInputSeqNum = seqNum
}

func ScreenMemSetNumRunningCommands(screenId string, num int) {
	MemLock.Lock()
	defer MemLock.Unlock()
	if ScreenMemStore[screenId] == nil {
		ScreenMemStore[screenId] = &ScreenMemState{}
	}
	ScreenMemStore[screenId].NumRunningCommands = num
}

func ScreenMemCombineIndicator(screenId string, indicator string) {
	MemLock.Lock()
	defer MemLock.Unlock()
	if ScreenMemStore[screenId] == nil {
		ScreenMemStore[screenId] = &ScreenMemState{}
	}
	if isIndicatorGreater(indicator, ScreenMemStore[screenId].IndicatorType) {
		ScreenMemStore[screenId].IndicatorType = indicator
	}
}

// safe because we return a copy
func GetScreenMemState(screenId string) *ScreenMemState {
	MemLock.Lock()
	defer MemLock.Unlock()
	ptr := ScreenMemStore[screenId]
	if ptr == nil {
		return nil
	}
	rtn := *ptr
	return &rtn
}
