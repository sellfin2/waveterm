// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package sstore

import (
	"fmt"
	"log"
	"sync"

	"github.com/wavetermdev/waveterm/waveshell/pkg/packet"
	"github.com/wavetermdev/waveterm/waveshell/pkg/utilfn"
)

var MainBus *UpdateBus = MakeUpdateBus()

const PtyDataUpdateStr = "pty"
const ModelUpdateStr = "model"
const UpdateChSize = 100

type UpdatePacket interface {
	UpdateType() string
	Clean()
}

type PtyDataUpdate struct {
	ScreenId   string `json:"screenid,omitempty"`
	LineId     string `json:"lineid,omitempty"`
	RemoteId   string `json:"remoteid,omitempty"`
	PtyPos     int64  `json:"ptypos"`
	PtyData64  string `json:"ptydata64"`
	PtyDataLen int64  `json:"ptydatalen"`
}

func (*PtyDataUpdate) UpdateType() string {
	return PtyDataUpdateStr
}

func (pdu *PtyDataUpdate) Clean() {}

type ModelUpdate struct {
	Sessions                 []*SessionType                     `json:"sessions,omitempty"`
	ActiveSessionId          string                             `json:"activesessionid,omitempty"`
	Screens                  []*ScreenType                      `json:"screens,omitempty"`
	ScreenLines              *ScreenLinesType                   `json:"screenlines,omitempty"`
	Line                     *LineType                          `json:"line,omitempty"`
	Lines                    []*LineType                        `json:"lines,omitempty"`
	Cmd                      *CmdType                           `json:"cmd,omitempty"`
	CmdLine                  *utilfn.StrWithPos                 `json:"cmdline,omitempty"`
	Info                     *InfoMsgType                       `json:"info,omitempty"`
	ClearInfo                bool                               `json:"clearinfo,omitempty"`
	Remotes                  []RemoteRuntimeState               `json:"remotes,omitempty"`
	History                  *HistoryInfoType                   `json:"history,omitempty"`
	Interactive              bool                               `json:"interactive"`
	Connect                  bool                               `json:"connect,omitempty"`
	MainView                 string                             `json:"mainview,omitempty"`
	Bookmarks                []*BookmarkType                    `json:"bookmarks,omitempty"`
	SelectedBookmark         string                             `json:"selectedbookmark,omitempty"`
	HistoryViewData          *HistoryViewData                   `json:"historyviewdata,omitempty"`
	ClientData               *ClientData                        `json:"clientdata,omitempty"`
	RemoteView               *RemoteViewType                    `json:"remoteview,omitempty"`
	ScreenTombstones         []*ScreenTombstoneType             `json:"screentombstones,omitempty"`
	SessionTombstones        []*SessionTombstoneType            `json:"sessiontombstones,omitempty"`
	OpenAICmdInfoChat        []*packet.OpenAICmdInfoChatMessage `json:"openaicmdinfochat,omitempty"`
	AlertMessage             *AlertMessageType                  `json:"alertmessage,omitempty"`
	ScreenStatusIndicators   []*ScreenStatusIndicatorType       `json:"screenstatusindicators,omitempty"`
	ScreenNumRunningCommands []*ScreenNumRunningCommandsType    `json:"screennumrunningcommands,omitempty"`
}

func (*ModelUpdate) UpdateType() string {
	return ModelUpdateStr
}

func (update *ModelUpdate) Clean() {
	if update == nil {
		return
	}
	update.ClientData = update.ClientData.Clean()
}

func (update *ModelUpdate) UpdateScreen(newScreen *ScreenType) {
	if newScreen == nil {
		return
	}
	for idx, screen := range update.Screens {
		if screen.ScreenId == newScreen.ScreenId {
			update.Screens[idx] = newScreen
			return
		}
	}
	update.Screens = append(update.Screens, newScreen)
}

// only sets InfoError if InfoError is not already set
func (update *ModelUpdate) AddInfoError(errStr string) {
	if update.Info == nil {
		update.Info = &InfoMsgType{}
	}
	if update.Info.InfoError == "" {
		update.Info.InfoError = errStr
	}
}

type RemoteViewType struct {
	RemoteShowAll bool            `json:"remoteshowall,omitempty"`
	PtyRemoteId   string          `json:"ptyremoteid,omitempty"`
	RemoteEdit    *RemoteEditType `json:"remoteedit,omitempty"`
}

func InfoMsgUpdate(infoMsgFmt string, args ...interface{}) *ModelUpdate {
	msg := fmt.Sprintf(infoMsgFmt, args...)
	return &ModelUpdate{
		Info: &InfoMsgType{InfoMsg: msg},
	}
}

type HistoryViewData struct {
	Items         []*HistoryItemType `json:"items"`
	Offset        int                `json:"offset"`
	RawOffset     int                `json:"rawoffset"`
	NextRawOffset int                `json:"nextrawoffset"`
	HasMore       bool               `json:"hasmore"`
	Lines         []*LineType        `json:"lines"`
	Cmds          []*CmdType         `json:"cmds"`
}

type RemoteEditType struct {
	RemoteEdit  bool   `json:"remoteedit"`
	RemoteId    string `json:"remoteid,omitempty"`
	ErrorStr    string `json:"errorstr,omitempty"`
	InfoStr     string `json:"infostr,omitempty"`
	KeyStr      string `json:"keystr,omitempty"`
	HasPassword bool   `json:"haspassword,omitempty"`
}

type AlertMessageType struct {
	Title    string `json:"title,omitempty"`
	Message  string `json:"message"`
	Confirm  bool   `json:"confirm,omitempty"`
	Markdown bool   `json:"markdown,omitempty"`
}

type InfoMsgType struct {
	InfoTitle     string   `json:"infotitle"`
	InfoError     string   `json:"infoerror,omitempty"`
	InfoMsg       string   `json:"infomsg,omitempty"`
	InfoMsgHtml   bool     `json:"infomsghtml,omitempty"`
	WebShareLink  bool     `json:"websharelink,omitempty"`
	InfoComps     []string `json:"infocomps,omitempty"`
	InfoCompsMore bool     `json:"infocompssmore,omitempty"`
	InfoLines     []string `json:"infolines,omitempty"`
	TimeoutMs     int64    `json:"timeoutms,omitempty"`
}

type HistoryInfoType struct {
	HistoryType string             `json:"historytype"`
	SessionId   string             `json:"sessionid,omitempty"`
	ScreenId    string             `json:"screenid,omitempty"`
	Items       []*HistoryItemType `json:"items"`
	Show        bool               `json:"show"`
}

type UpdateChannel struct {
	ScreenId string
	ClientId string
	Ch       chan interface{}
}

func (uch UpdateChannel) Match(screenId string) bool {
	if screenId == "" {
		return true
	}
	return screenId == uch.ScreenId
}

type UpdateBus struct {
	Lock     *sync.Mutex
	Channels map[string]UpdateChannel
}

func MakeUpdateBus() *UpdateBus {
	return &UpdateBus{
		Lock:     &sync.Mutex{},
		Channels: make(map[string]UpdateChannel),
	}
}

// always returns a new channel
func (bus *UpdateBus) RegisterChannel(clientId string, screenId string) chan interface{} {
	bus.Lock.Lock()
	defer bus.Lock.Unlock()
	uch, found := bus.Channels[clientId]
	if found {
		close(uch.Ch)
		uch.ScreenId = screenId
		uch.Ch = make(chan interface{}, UpdateChSize)
	} else {
		uch = UpdateChannel{
			ClientId: clientId,
			ScreenId: screenId,
			Ch:       make(chan interface{}, UpdateChSize),
		}
	}
	bus.Channels[clientId] = uch
	return uch.Ch
}

func (bus *UpdateBus) UnregisterChannel(clientId string) {
	bus.Lock.Lock()
	defer bus.Lock.Unlock()
	uch, found := bus.Channels[clientId]
	if found {
		close(uch.Ch)
		delete(bus.Channels, clientId)
	}
}

func (bus *UpdateBus) SendUpdate(update UpdatePacket) {
	if update == nil {
		return
	}
	update.Clean()
	bus.Lock.Lock()
	defer bus.Lock.Unlock()
	for _, uch := range bus.Channels {
		select {
		case uch.Ch <- update:

		default:
			log.Printf("[error] dropped update on updatebus uch clientid=%s\n", uch.ClientId)
		}
	}
}

func (bus *UpdateBus) SendScreenUpdate(screenId string, update UpdatePacket) {
	if update == nil {
		return
	}
	update.Clean()
	bus.Lock.Lock()
	defer bus.Lock.Unlock()
	for _, uch := range bus.Channels {
		if uch.Match(screenId) {
			select {
			case uch.Ch <- update:

			default:
				log.Printf("[error] dropped update on updatebus uch clientid=%s\n", uch.ClientId)
			}
		}
	}
}

func MakeSessionsUpdateForRemote(sessionId string, ri *RemoteInstance) []*SessionType {
	return []*SessionType{
		{
			SessionId: sessionId,
			Remotes:   []*RemoteInstance{ri},
		},
	}
}

type BookmarksViewType struct {
	Bookmarks []*BookmarkType `json:"bookmarks"`
}

type ScreenStatusIndicatorType struct {
	ScreenId string               `json:"screenid"`
	Status   StatusIndicatorLevel `json:"status"`
}

type ScreenNumRunningCommandsType struct {
	ScreenId string `json:"screenid"`
	Num      int    `json:"num"`
}
