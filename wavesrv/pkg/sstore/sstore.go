// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package sstore

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"database/sql/driver"
	"fmt"
	"log"
	"os"
	"os/user"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/sawka/txwrap"
	"github.com/wavetermdev/waveterm/waveshell/pkg/base"
	"github.com/wavetermdev/waveterm/waveshell/pkg/packet"
	"github.com/wavetermdev/waveterm/waveshell/pkg/shellenv"
	"github.com/wavetermdev/waveterm/wavesrv/pkg/dbutil"
	"github.com/wavetermdev/waveterm/wavesrv/pkg/scbase"

	_ "github.com/mattn/go-sqlite3"
)

const LineNoHeight = -1
const DBFileName = "waveterm.db"
const DBWALFileName = "waveterm.db-wal"
const DBFileNameBackup = "backup.waveterm.db"
const DBWALFileNameBackup = "backup.waveterm.db-wal"
const MaxWebShareLineCount = 50
const MaxWebShareScreenCount = 3
const MaxLineStateSize = 4 * 1024 // 4k for now, can raise if needed

const DefaultSessionName = "default"
const LocalRemoteAlias = "local"

const DefaultCwd = "~"
const APITokenSentinel = "--apitoken--"

// defined here and not in packet.go since this value should never
// be passed to waveshell (it should always get resolved prior to sending a run packet)
const ShellTypePref_Detect = "detect"

const (
	LineTypeCmd    = "cmd"
	LineTypeText   = "text"
	LineTypeOpenAI = "openai"
)

const (
	LineState_Source   = "prompt:source"
	LineState_File     = "prompt:file"
	LineState_Template = "template"
	LineState_Mode     = "mode"
	LineState_Lang     = "lang"
)

const (
	MainViewSession   = "session"
	MainViewBookmarks = "bookmarks"
	MainViewHistory   = "history"
)

const (
	CmdStatusRunning  = "running"
	CmdStatusDetached = "detached"
	CmdStatusError    = "error"
	CmdStatusDone     = "done"
	CmdStatusHangup   = "hangup"
	CmdStatusUnknown  = "unknown" // used for history items where we don't have a status
)

const (
	CmdRendererOpenAI = "openai"
)

const (
	OpenAIRoleSystem    = "system"
	OpenAIRoleUser      = "user"
	OpenAIRoleAssistant = "assistant"
)

const (
	RemoteAuthTypeNone        = "none"
	RemoteAuthTypePassword    = "password"
	RemoteAuthTypeKey         = "key"
	RemoteAuthTypeKeyPassword = "key+password"
)

const (
	SSHConfigSrcTypeManual = "waveterm-manual"
	SSHConfigSrcTypeImport = "sshconfig-import"
)

const (
	ShareModeLocal = "local"
	ShareModeWeb   = "web"
)

const (
	ConnectModeStartup = "startup"
	ConnectModeAuto    = "auto"
	ConnectModeManual  = "manual"
)

const (
	RemoteTypeSsh    = "ssh"
	RemoteTypeOpenAI = "openai"
)

const (
	ScreenFocusInput = "input"
	ScreenFocusCmd   = "cmd"
)

const (
	CmdStoreTypeSession = "session"
	CmdStoreTypeScreen  = "screen"
)

const (
	UpdateType_ScreenNew          = "screen:new"
	UpdateType_ScreenDel          = "screen:del"
	UpdateType_ScreenSelectedLine = "screen:selectedline"
	UpdateType_ScreenName         = "screen:sharename"
	UpdateType_LineNew            = "line:new"
	UpdateType_LineDel            = "line:del"
	UpdateType_LineRenderer       = "line:renderer"
	UpdateType_LineContentHeight  = "line:contentheight"
	UpdateType_LineState          = "line:state"
	UpdateType_CmdStatus          = "cmd:status"
	UpdateType_CmdTermOpts        = "cmd:termopts"
	UpdateType_CmdExitCode        = "cmd:exitcode"
	UpdateType_CmdDurationMs      = "cmd:durationms"
	UpdateType_CmdRtnState        = "cmd:rtnstate"
	UpdateType_PtyPos             = "pty:pos"
)

const MaxTzNameLen = 50

var globalDBLock = &sync.Mutex{}
var globalDB *sqlx.DB
var globalDBErr error

func lineIdFromCK(ck base.CommandKey) string {
	return ck.GetCmdId()
}

func GetDBName() string {
	scHome := scbase.GetWaveHomeDir()
	return path.Join(scHome, DBFileName)
}

func GetDBWALName() string {
	scHome := scbase.GetWaveHomeDir()
	return path.Join(scHome, DBWALFileName)
}

func GetDBBackupName() string {
	scHome := scbase.GetWaveHomeDir()
	return path.Join(scHome, DBFileNameBackup)
}

func GetDBWALBackupName() string {
	scHome := scbase.GetWaveHomeDir()
	return path.Join(scHome, DBWALFileNameBackup)
}

func IsValidConnectMode(mode string) bool {
	return mode == ConnectModeStartup || mode == ConnectModeAuto || mode == ConnectModeManual
}

func GetDB(ctx context.Context) (*sqlx.DB, error) {
	if txwrap.IsTxWrapContext(ctx) {
		return nil, fmt.Errorf("cannot call GetDB from within a running transaction")
	}
	globalDBLock.Lock()
	defer globalDBLock.Unlock()
	if globalDB == nil && globalDBErr == nil {
		dbName := GetDBName()
		globalDB, globalDBErr = sqlx.Open("sqlite3", fmt.Sprintf("file:%s?cache=shared&mode=rwc&_journal_mode=WAL&_busy_timeout=5000", dbName))
		if globalDBErr != nil {
			globalDBErr = fmt.Errorf("opening db[%s]: %w", dbName, globalDBErr)
			log.Printf("[db] error: %v\n", globalDBErr)
		} else {
			log.Printf("[db] successfully opened db %s\n", dbName)
		}
	}
	return globalDB, globalDBErr
}

func CloseDB() {
	globalDBLock.Lock()
	defer globalDBLock.Unlock()
	if globalDB == nil {
		return
	}
	err := globalDB.Close()
	if err != nil {
		log.Printf("[db] error closing database: %v\n", err)
	}
	globalDB = nil
}

type CmdPtr struct {
	ScreenId string
	LineId   string
}

type ClientWinSizeType struct {
	Width      int  `json:"width"`
	Height     int  `json:"height"`
	Top        int  `json:"top"`
	Left       int  `json:"left"`
	FullScreen bool `json:"fullscreen,omitempty"`
}

type ActivityUpdate struct {
	FgMinutes        int
	ActiveMinutes    int
	OpenMinutes      int
	NumCommands      int
	ClickShared      int
	HistoryView      int
	BookmarksView    int
	NumConns         int
	WebShareLimit    int
	ReinitBashErrors int
	ReinitZshErrors  int
	BuildTime        string
}

type ActivityType struct {
	Day           string        `json:"day"`
	Uploaded      bool          `json:"-"`
	TData         TelemetryData `json:"tdata"`
	TzName        string        `json:"tzname"`
	TzOffset      int           `json:"tzoffset"`
	ClientVersion string        `json:"clientversion"`
	ClientArch    string        `json:"clientarch"`
	BuildTime     string        `json:"buildtime"`
	DefaultShell  string        `json:"defaultshell"`
	OSRelease     string        `json:"osrelease"`
}

type TelemetryData struct {
	NumCommands      int `json:"numcommands"`
	ActiveMinutes    int `json:"activeminutes"`
	FgMinutes        int `json:"fgminutes"`
	OpenMinutes      int `json:"openminutes"`
	ClickShared      int `json:"clickshared,omitempty"`
	HistoryView      int `json:"historyview,omitempty"`
	BookmarksView    int `json:"bookmarksview,omitempty"`
	NumConns         int `json:"numconns"`
	WebShareLimit    int `json:"websharelimit,omitempty"`
	ReinitBashErrors int `json:"reinitbasherrors,omitempty"`
	ReinitZshErrors  int `json:"reinitzsherrors,omitempty"`
}

func (tdata TelemetryData) Value() (driver.Value, error) {
	return quickValueJson(tdata)
}

func (tdata *TelemetryData) Scan(val interface{}) error {
	return quickScanJson(tdata, val)
}

type SidebarValueType struct {
	Collapsed bool `json:"collapsed"`
	Width     int  `json:"width"`
}

type ClientOptsType struct {
	NoTelemetry    bool              `json:"notelemetry,omitempty"`
	NoReleaseCheck bool              `json:"noreleasecheck,omitempty"`
	AcceptedTos    int64             `json:"acceptedtos,omitempty"`
	ConfirmFlags   map[string]bool   `json:"confirmflags,omitempty"`
	MainSidebar    *SidebarValueType `json:"mainsidebar,omitempty"`
}

type FeOptsType struct {
	TermFontSize int `json:"termfontsize,omitempty"`
}

type ReleaseInfoType struct {
	LatestVersion string `json:"latestversion,omitempty"`
}

type ClientData struct {
	ClientId            string            `json:"clientid"`
	UserId              string            `json:"userid"`
	UserPrivateKeyBytes []byte            `json:"-"`
	UserPublicKeyBytes  []byte            `json:"-"`
	UserPrivateKey      *ecdsa.PrivateKey `json:"-" dbmap:"-"`
	UserPublicKey       *ecdsa.PublicKey  `json:"-" dbmap:"-"`
	ActiveSessionId     string            `json:"activesessionid"`
	WinSize             ClientWinSizeType `json:"winsize"`
	ClientOpts          ClientOptsType    `json:"clientopts"`
	FeOpts              FeOptsType        `json:"feopts"`
	CmdStoreType        string            `json:"cmdstoretype"`
	DBVersion           int               `json:"dbversion" dbmap:"-"`
	OpenAIOpts          *OpenAIOptsType   `json:"openaiopts,omitempty" dbmap:"openaiopts"`
	ReleaseInfo         ReleaseInfoType   `json:"releaseinfo"`
}

func (ClientData) UseDBMap() {}

func (cdata *ClientData) Clean() *ClientData {
	if cdata == nil {
		return nil
	}
	rtn := *cdata
	if rtn.OpenAIOpts != nil {
		rtn.OpenAIOpts = &OpenAIOptsType{
			Model:      cdata.OpenAIOpts.Model,
			MaxTokens:  cdata.OpenAIOpts.MaxTokens,
			MaxChoices: cdata.OpenAIOpts.MaxChoices,
			// omit API Token
		}
		if cdata.OpenAIOpts.APIToken != "" {
			rtn.OpenAIOpts.APIToken = APITokenSentinel
		}
	}
	return &rtn
}

type SessionType struct {
	SessionId      string            `json:"sessionid"`
	Name           string            `json:"name"`
	SessionIdx     int64             `json:"sessionidx"`
	ActiveScreenId string            `json:"activescreenid"`
	ShareMode      string            `json:"sharemode"`
	NotifyNum      int64             `json:"notifynum"`
	Archived       bool              `json:"archived,omitempty"`
	ArchivedTs     int64             `json:"archivedts,omitempty"`
	Remotes        []*RemoteInstance `json:"remotes"`

	// only for updates
	Remove bool `json:"remove,omitempty"`
	Full   bool `json:"full,omitempty"`
}

type SessionTombstoneType struct {
	SessionId string `json:"sessionid"`
	Name      string `json:"name"`
	DeletedTs int64  `json:"deletedts"`
}

func (SessionTombstoneType) UseDBMap() {}

type SessionStatsType struct {
	SessionId          string              `json:"sessionid"`
	NumScreens         int                 `json:"numscreens"`
	NumArchivedScreens int                 `json:"numarchivedscreens"`
	NumLines           int                 `json:"numlines"`
	NumCmds            int                 `json:"numcmds"`
	DiskStats          SessionDiskSizeType `json:"diskstats"`
}

var RemoteNameRe = regexp.MustCompile("^\\*?[a-zA-Z0-9_-]+$")

type RemotePtrType struct {
	OwnerId  string `json:"ownerid"`
	RemoteId string `json:"remoteid"`
	Name     string `json:"name"`
}

func (r RemotePtrType) IsSessionScope() bool {
	return strings.HasPrefix(r.Name, "*")
}

func (rptr *RemotePtrType) GetDisplayName(baseDisplayName string) string {
	name := baseDisplayName
	if rptr == nil {
		return name
	}
	if rptr.Name != "" {
		name = name + ":" + rptr.Name
	}
	if rptr.OwnerId != "" {
		name = "@" + rptr.OwnerId + ":" + name
	}
	return name
}

func (r RemotePtrType) Validate() error {
	if r.OwnerId != "" {
		if _, err := uuid.Parse(r.OwnerId); err != nil {
			return fmt.Errorf("invalid ownerid format: %v", err)
		}
	}
	if r.RemoteId != "" {
		if _, err := uuid.Parse(r.RemoteId); err != nil {
			return fmt.Errorf("invalid remoteid format: %v", err)
		}
	}
	if r.Name != "" {
		ok := RemoteNameRe.MatchString(r.Name)
		if !ok {
			return fmt.Errorf("invalid remote name")
		}
	}
	return nil
}

func (r RemotePtrType) MakeFullRemoteRef() string {
	if r.RemoteId == "" {
		return ""
	}
	if r.OwnerId == "" && r.Name == "" {
		return r.RemoteId
	}
	if r.OwnerId != "" && r.Name == "" {
		return fmt.Sprintf("@%s:%s", r.OwnerId, r.RemoteId)
	}
	if r.OwnerId == "" && r.Name != "" {
		return fmt.Sprintf("%s:%s", r.RemoteId, r.Name)
	}
	return fmt.Sprintf("@%s:%s:%s", r.OwnerId, r.RemoteId, r.Name)
}

func (h *HistoryItemType) ToMap() map[string]interface{} {
	rtn := make(map[string]interface{})
	rtn["historyid"] = h.HistoryId
	rtn["ts"] = h.Ts
	rtn["userid"] = h.UserId
	rtn["sessionid"] = h.SessionId
	rtn["screenid"] = h.ScreenId
	rtn["lineid"] = h.LineId
	rtn["linenum"] = h.LineNum
	rtn["haderror"] = h.HadError
	rtn["cmdstr"] = h.CmdStr
	rtn["remoteownerid"] = h.Remote.OwnerId
	rtn["remoteid"] = h.Remote.RemoteId
	rtn["remotename"] = h.Remote.Name
	rtn["ismetacmd"] = h.IsMetaCmd
	rtn["exitcode"] = h.ExitCode
	rtn["durationms"] = h.DurationMs
	rtn["festate"] = quickJson(h.FeState)
	rtn["tags"] = quickJson(h.Tags)
	rtn["status"] = h.Status
	return rtn
}

func (h *HistoryItemType) FromMap(m map[string]interface{}) bool {
	quickSetStr(&h.HistoryId, m, "historyid")
	quickSetInt64(&h.Ts, m, "ts")
	quickSetStr(&h.UserId, m, "userid")
	quickSetStr(&h.SessionId, m, "sessionid")
	quickSetStr(&h.ScreenId, m, "screenid")
	quickSetStr(&h.LineId, m, "lineid")
	quickSetBool(&h.HadError, m, "haderror")
	quickSetStr(&h.CmdStr, m, "cmdstr")
	quickSetStr(&h.Remote.OwnerId, m, "remoteownerid")
	quickSetStr(&h.Remote.RemoteId, m, "remoteid")
	quickSetStr(&h.Remote.Name, m, "remotename")
	quickSetBool(&h.IsMetaCmd, m, "ismetacmd")
	quickSetStr(&h.HistoryNum, m, "historynum")
	quickSetInt64(&h.LineNum, m, "linenum")
	dbutil.QuickSetNullableInt64(&h.ExitCode, m, "exitcode")
	dbutil.QuickSetNullableInt64(&h.DurationMs, m, "durationms")
	quickSetJson(&h.FeState, m, "festate")
	quickSetJson(&h.Tags, m, "tags")
	quickSetStr(&h.Status, m, "status")
	return true
}

type ScreenOptsType struct {
	TabColor string `json:"tabcolor,omitempty"`
	TabIcon  string `json:"tabicon,omitempty"`
	PTerm    string `json:"pterm,omitempty"`
}

type ScreenLinesType struct {
	ScreenId string      `json:"screenid"`
	Lines    []*LineType `json:"lines" dbmap:"-"`
	Cmds     []*CmdType  `json:"cmds" dbmap:"-"`
}

func (ScreenLinesType) UseDBMap() {}

type ScreenWebShareOpts struct {
	ShareName string `json:"sharename"`
	ViewKey   string `json:"viewkey"`
}

type ScreenCreateOpts struct {
	BaseScreenId string
	CopyRemote   bool
	CopyCwd      bool
	CopyEnv      bool
}

func (sco ScreenCreateOpts) HasCopy() bool {
	return sco.CopyRemote || sco.CopyCwd || sco.CopyEnv
}

type ScreenSidebarOptsType struct {
	Open  bool   `json:"open,omitempty"`
	Width string `json:"width,omitempty"`

	// this used to be more complicated (sections with types).  simplified for this release
	SidebarLineId string `json:"sidebarlineid,omitempty"`
}

type ScreenViewOptsType struct {
	Sidebar *ScreenSidebarOptsType `json:"sidebar,omitempty"`
}

type ScreenType struct {
	SessionId      string              `json:"sessionid"`
	ScreenId       string              `json:"screenid"`
	Name           string              `json:"name"`
	ScreenIdx      int64               `json:"screenidx"`
	ScreenOpts     ScreenOptsType      `json:"screenopts"`
	ScreenViewOpts ScreenViewOptsType  `json:"screenviewopts"`
	OwnerId        string              `json:"ownerid"`
	ShareMode      string              `json:"sharemode"`
	WebShareOpts   *ScreenWebShareOpts `json:"webshareopts,omitempty"`
	CurRemote      RemotePtrType       `json:"curremote"`
	NextLineNum    int64               `json:"nextlinenum"`
	SelectedLine   int64               `json:"selectedline"`
	Anchor         ScreenAnchorType    `json:"anchor"`
	FocusType      string              `json:"focustype"`
	Archived       bool                `json:"archived,omitempty"`
	ArchivedTs     int64               `json:"archivedts,omitempty"`

	// only for updates
	Full            bool   `json:"full,omitempty"`
	Remove          bool   `json:"remove,omitempty"`
	StatusIndicator string `json:"statusindicator,omitempty"`
}

func (s *ScreenType) ToMap() map[string]interface{} {
	rtn := make(map[string]interface{})
	rtn["sessionid"] = s.SessionId
	rtn["screenid"] = s.ScreenId
	rtn["name"] = s.Name
	rtn["screenidx"] = s.ScreenIdx
	rtn["screenopts"] = quickJson(s.ScreenOpts)
	rtn["screenviewopts"] = quickJson(s.ScreenViewOpts)
	rtn["ownerid"] = s.OwnerId
	rtn["sharemode"] = s.ShareMode
	rtn["webshareopts"] = quickNullableJson(s.WebShareOpts)
	rtn["curremoteownerid"] = s.CurRemote.OwnerId
	rtn["curremoteid"] = s.CurRemote.RemoteId
	rtn["curremotename"] = s.CurRemote.Name
	rtn["nextlinenum"] = s.NextLineNum
	rtn["selectedline"] = s.SelectedLine
	rtn["anchor"] = quickJson(s.Anchor)
	rtn["focustype"] = s.FocusType
	rtn["archived"] = s.Archived
	rtn["archivedts"] = s.ArchivedTs
	return rtn
}

func (s *ScreenType) FromMap(m map[string]interface{}) bool {
	quickSetStr(&s.SessionId, m, "sessionid")
	quickSetStr(&s.ScreenId, m, "screenid")
	quickSetStr(&s.Name, m, "name")
	quickSetInt64(&s.ScreenIdx, m, "screenidx")
	quickSetJson(&s.ScreenOpts, m, "screenopts")
	quickSetJson(&s.ScreenViewOpts, m, "screenviewopts")
	quickSetStr(&s.OwnerId, m, "ownerid")
	quickSetStr(&s.ShareMode, m, "sharemode")
	quickSetNullableJson(&s.WebShareOpts, m, "webshareopts")
	quickSetStr(&s.CurRemote.OwnerId, m, "curremoteownerid")
	quickSetStr(&s.CurRemote.RemoteId, m, "curremoteid")
	quickSetStr(&s.CurRemote.Name, m, "curremotename")
	quickSetInt64(&s.NextLineNum, m, "nextlinenum")
	quickSetInt64(&s.SelectedLine, m, "selectedline")
	quickSetJson(&s.Anchor, m, "anchor")
	quickSetStr(&s.FocusType, m, "focustype")
	quickSetBool(&s.Archived, m, "archived")
	quickSetInt64(&s.ArchivedTs, m, "archivedts")
	return true
}

type ScreenTombstoneType struct {
	ScreenId   string         `json:"screenid"`
	SessionId  string         `json:"sessionid"`
	Name       string         `json:"name"`
	DeletedTs  int64          `json:"deletedts"`
	ScreenOpts ScreenOptsType `json:"screenopts"`
}

func (ScreenTombstoneType) UseDBMap() {}

const (
	LayoutFull = "full"
)

type LayoutType struct {
	Type   string `json:"type"`
	Parent string `json:"parent,omitempty"`
	ZIndex int64  `json:"zindex,omitempty"`
	Float  bool   `json:"float,omitempty"`
	Top    string `json:"top,omitempty"`
	Bottom string `json:"bottom,omitempty"`
	Left   string `json:"left,omitempty"`
	Right  string `json:"right,omitempty"`
	Width  string `json:"width,omitempty"`
	Height string `json:"height,omitempty"`
}

func (l *LayoutType) Scan(val interface{}) error {
	return quickScanJson(l, val)
}

func (l LayoutType) Value() (driver.Value, error) {
	return quickValueJson(l)
}

type ScreenAnchorType struct {
	AnchorLine   int `json:"anchorline,omitempty"`
	AnchorOffset int `json:"anchoroffset,omitempty"`
}

type HistoryItemType struct {
	HistoryId  string          `json:"historyid"`
	Ts         int64           `json:"ts"`
	UserId     string          `json:"userid"`
	SessionId  string          `json:"sessionid"`
	ScreenId   string          `json:"screenid"`
	LineId     string          `json:"lineid"`
	HadError   bool            `json:"haderror"`
	CmdStr     string          `json:"cmdstr"`
	Remote     RemotePtrType   `json:"remote"`
	IsMetaCmd  bool            `json:"ismetacmd"`
	ExitCode   *int64          `json:"exitcode,omitempty"`
	DurationMs *int64          `json:"durationms,omitempty"`
	FeState    FeStateType     `json:"festate,omitempty"`
	Tags       map[string]bool `json:"tags,omitempty"`
	LineNum    int64           `json:"linenum" dbmap:"-"`
	Status     string          `json:"status"`

	// only for updates
	Remove bool `json:"remove" dbmap:"-"`

	// transient (string because of different history orderings)
	HistoryNum string `json:"historynum" dbmap:"-"`
}

type HistoryQueryOpts struct {
	Offset     int
	MaxItems   int
	FromTs     int64
	SearchText string
	SessionId  string
	RemoteId   string
	ScreenId   string
	NoMeta     bool
	RawOffset  int
	FilterFn   func(*HistoryItemType) bool
}

type HistoryQueryResult struct {
	MaxItems      int
	Items         []*HistoryItemType
	Offset        int // the offset shown to user
	RawOffset     int // internal offset
	HasMore       bool
	NextRawOffset int // internal offset used by pager for next query

	prevItems int // holds number of items skipped by RawOffset
}

type TermOpts struct {
	Rows       int64 `json:"rows"`
	Cols       int64 `json:"cols"`
	FlexRows   bool  `json:"flexrows,omitempty"`
	MaxPtySize int64 `json:"maxptysize,omitempty"`
}

func (opts *TermOpts) Scan(val interface{}) error {
	return quickScanJson(opts, val)
}

func (opts TermOpts) Value() (driver.Value, error) {
	return quickValueJson(opts)
}

type ShellStatePtr struct {
	BaseHash    string
	DiffHashArr []string
}

func (ssptr *ShellStatePtr) IsEmpty() bool {
	if ssptr == nil || ssptr.BaseHash == "" {
		return true
	}
	return false
}

type RemoteInstance struct {
	RIId             string            `json:"riid"`
	Name             string            `json:"name"`
	SessionId        string            `json:"sessionid"`
	ScreenId         string            `json:"screenid"`
	RemoteOwnerId    string            `json:"remoteownerid"`
	RemoteId         string            `json:"remoteid"`
	FeState          map[string]string `json:"festate"`
	ShellType        string            `json:"shelltype"`
	StateBaseHash    string            `json:"-"`
	StateDiffHashArr []string          `json:"-"`

	// only for updates
	Remove bool `json:"remove,omitempty"`
}

type StateBase struct {
	BaseHash string
	Version  string
	Ts       int64
	Data     []byte
}

type StateDiff struct {
	DiffHash    string
	Ts          int64
	BaseHash    string
	DiffHashArr []string
	Data        []byte
}

func (sd *StateDiff) FromMap(m map[string]interface{}) bool {
	quickSetStr(&sd.DiffHash, m, "diffhash")
	quickSetInt64(&sd.Ts, m, "ts")
	quickSetStr(&sd.BaseHash, m, "basehash")
	quickSetJsonArr(&sd.DiffHashArr, m, "diffhasharr")
	quickSetBytes(&sd.Data, m, "data")
	return true
}

func (sd *StateDiff) ToMap() map[string]interface{} {
	rtn := make(map[string]interface{})
	rtn["diffhash"] = sd.DiffHash
	rtn["ts"] = sd.Ts
	rtn["basehash"] = sd.BaseHash
	rtn["diffhasharr"] = quickJsonArr(sd.DiffHashArr)
	rtn["data"] = sd.Data
	return rtn
}

func FeStateFromShellState(state *packet.ShellState) map[string]string {
	if state == nil {
		return nil
	}
	rtn := make(map[string]string)
	rtn["cwd"] = state.Cwd
	envMap := shellenv.EnvMapFromState(state)
	if envMap["VIRTUAL_ENV"] != "" {
		rtn["VIRTUAL_ENV"] = envMap["VIRTUAL_ENV"]
	}
	for key, val := range envMap {
		if strings.HasPrefix(key, "PROMPTVAR_") && envMap[key] != "" {
			rtn[key] = val
		}
	}
	_, _, err := packet.ParseShellStateVersion(state.Version)
	if err != nil {
		rtn["invalidstate"] = "1"
	}
	return rtn
}

func (ri *RemoteInstance) FromMap(m map[string]interface{}) bool {
	quickSetStr(&ri.RIId, m, "riid")
	quickSetStr(&ri.Name, m, "name")
	quickSetStr(&ri.SessionId, m, "sessionid")
	quickSetStr(&ri.ScreenId, m, "screenid")
	quickSetStr(&ri.RemoteOwnerId, m, "remoteownerid")
	quickSetStr(&ri.RemoteId, m, "remoteid")
	quickSetJson(&ri.FeState, m, "festate")
	quickSetStr(&ri.StateBaseHash, m, "statebasehash")
	quickSetJsonArr(&ri.StateDiffHashArr, m, "statediffhasharr")
	quickSetStr(&ri.ShellType, m, "shelltype")
	return true
}

func (ri *RemoteInstance) ToMap() map[string]interface{} {
	rtn := make(map[string]interface{})
	rtn["riid"] = ri.RIId
	rtn["name"] = ri.Name
	rtn["sessionid"] = ri.SessionId
	rtn["screenid"] = ri.ScreenId
	rtn["remoteownerid"] = ri.RemoteOwnerId
	rtn["remoteid"] = ri.RemoteId
	rtn["festate"] = quickJson(ri.FeState)
	rtn["statebasehash"] = ri.StateBaseHash
	rtn["statediffhasharr"] = quickJsonArr(ri.StateDiffHashArr)
	rtn["shelltype"] = ri.ShellType
	return rtn
}

type ScreenUpdateType struct {
	UpdateId   int64  `json:"updateid"`
	ScreenId   string `json:"screenid"`
	LineId     string `json:"lineid"`
	UpdateType string `json:"updatetype"`
	UpdateTs   int64  `json:"updatets"`
}

func (ScreenUpdateType) UseDBMap() {}

type LineType struct {
	ScreenId      string         `json:"screenid"`
	UserId        string         `json:"userid"`
	LineId        string         `json:"lineid"`
	Ts            int64          `json:"ts"`
	LineNum       int64          `json:"linenum"`
	LineNumTemp   bool           `json:"linenumtemp,omitempty"`
	LineLocal     bool           `json:"linelocal"`
	LineType      string         `json:"linetype"`
	LineState     map[string]any `json:"linestate"`
	Renderer      string         `json:"renderer,omitempty"`
	Text          string         `json:"text,omitempty"`
	Ephemeral     bool           `json:"ephemeral,omitempty"`
	ContentHeight int64          `json:"contentheight,omitempty"`
	Star          bool           `json:"star,omitempty"`
	Archived      bool           `json:"archived,omitempty"`
	Remove        bool           `json:"remove,omitempty"`
}

func (LineType) UseDBMap() {}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type OpenAIChoiceType struct {
	Text         string `json:"text"`
	Index        int    `json:"index"`
	FinishReason string `json:"finish_reason"`
}

type OpenAIResponse struct {
	Model   string             `json:"model"`
	Created int64              `json:"created"`
	Usage   *OpenAIUsage       `json:"usage,omitempty"`
	Choices []OpenAIChoiceType `json:"choices,omitempty"`
}

type PlaybookType struct {
	PlaybookId   string   `json:"playbookid"`
	PlaybookName string   `json:"playbookname"`
	Description  string   `json:"description"`
	EntryIds     []string `json:"entryids"`

	// this is not persisted to DB, just for transport to FE
	Entries []*PlaybookEntry `json:"entries"`
}

func (p *PlaybookType) ToMap() map[string]interface{} {
	rtn := make(map[string]interface{})
	rtn["playbookid"] = p.PlaybookId
	rtn["playbookname"] = p.PlaybookName
	rtn["description"] = p.Description
	rtn["entryids"] = quickJsonArr(p.EntryIds)
	return rtn
}

func (p *PlaybookType) FromMap(m map[string]interface{}) bool {
	quickSetStr(&p.PlaybookId, m, "playbookid")
	quickSetStr(&p.PlaybookName, m, "playbookname")
	quickSetStr(&p.Description, m, "description")
	quickSetJsonArr(&p.Entries, m, "entries")
	return true
}

// reorders p.Entries to match p.EntryIds
func (p *PlaybookType) OrderEntries() {
	if len(p.Entries) == 0 {
		return
	}
	m := make(map[string]*PlaybookEntry)
	for _, entry := range p.Entries {
		m[entry.EntryId] = entry
	}
	newList := make([]*PlaybookEntry, 0, len(p.EntryIds))
	for _, entryId := range p.EntryIds {
		entry := m[entryId]
		if entry != nil {
			newList = append(newList, entry)
		}
	}
	p.Entries = newList
}

// removes from p.EntryIds (not from p.Entries)
func (p *PlaybookType) RemoveEntry(entryIdToRemove string) {
	if len(p.EntryIds) == 0 {
		return
	}
	newList := make([]string, 0, len(p.EntryIds)-1)
	for _, entryId := range p.EntryIds {
		if entryId == entryIdToRemove {
			continue
		}
		newList = append(newList, entryId)
	}
	p.EntryIds = newList
}

type PlaybookEntry struct {
	PlaybookId  string `json:"playbookid"`
	EntryId     string `json:"entryid"`
	Alias       string `json:"alias"`
	CmdStr      string `json:"cmdstr"`
	UpdatedTs   int64  `json:"updatedts"`
	CreatedTs   int64  `json:"createdts"`
	Description string `json:"description"`
	Remove      bool   `json:"remove,omitempty"`
}

type BookmarkType struct {
	BookmarkId  string   `json:"bookmarkid"`
	CreatedTs   int64    `json:"createdts"`
	CmdStr      string   `json:"cmdstr"`
	Alias       string   `json:"alias,omitempty"`
	Tags        []string `json:"tags"`
	Description string   `json:"description"`
	OrderIdx    int64    `json:"orderidx"`
	Remove      bool     `json:"remove,omitempty"`
}

func (bm *BookmarkType) GetSimpleKey() string {
	return bm.BookmarkId
}

func (bm *BookmarkType) ToMap() map[string]interface{} {
	rtn := make(map[string]interface{})
	rtn["bookmarkid"] = bm.BookmarkId
	rtn["createdts"] = bm.CreatedTs
	rtn["cmdstr"] = bm.CmdStr
	rtn["alias"] = bm.Alias
	rtn["description"] = bm.Description
	rtn["tags"] = quickJsonArr(bm.Tags)
	return rtn
}

func (bm *BookmarkType) FromMap(m map[string]interface{}) bool {
	quickSetStr(&bm.BookmarkId, m, "bookmarkid")
	quickSetInt64(&bm.CreatedTs, m, "createdts")
	quickSetStr(&bm.Alias, m, "alias")
	quickSetStr(&bm.CmdStr, m, "cmdstr")
	quickSetStr(&bm.Description, m, "description")
	quickSetJsonArr(&bm.Tags, m, "tags")
	return true
}

type ResolveItem struct {
	Name   string
	Num    int
	Id     string
	Hidden bool
}

type SSHOpts struct {
	Local       bool   `json:"local,omitempty"`
	IsSudo      bool   `json:"issudo,omitempty"`
	SSHHost     string `json:"sshhost"`
	SSHUser     string `json:"sshuser"`
	SSHOptsStr  string `json:"sshopts,omitempty"`
	SSHIdentity string `json:"sshidentity,omitempty"`
	SSHPort     int    `json:"sshport,omitempty"`
	SSHPassword string `json:"sshpassword,omitempty"`
}

func (opts SSHOpts) GetAuthType() string {
	if opts.SSHPassword != "" && opts.SSHIdentity != "" {
		return RemoteAuthTypeKeyPassword
	}
	if opts.SSHIdentity != "" {
		return RemoteAuthTypeKey
	}
	if opts.SSHPassword != "" {
		return RemoteAuthTypePassword
	}
	return RemoteAuthTypeNone
}

type RemoteOptsType struct {
	Color string `json:"color"`
}

type OpenAIOptsType struct {
	Model      string `json:"model"`
	APIToken   string `json:"apitoken"`
	BaseURL    string `json:"baseurl,omitempty"`
	MaxTokens  int    `json:"maxtokens,omitempty"`
	MaxChoices int    `json:"maxchoices,omitempty"`
}

const (
	RemoteStatus_Connected    = "connected"
	RemoteStatus_Connecting   = "connecting"
	RemoteStatus_Disconnected = "disconnected"
	RemoteStatus_Error        = "error"
)

type RemoteRuntimeState struct {
	RemoteType          string            `json:"remotetype"`
	RemoteId            string            `json:"remoteid"`
	RemoteAlias         string            `json:"remotealias,omitempty"`
	RemoteCanonicalName string            `json:"remotecanonicalname"`
	RemoteVars          map[string]string `json:"remotevars"`
	DefaultFeState      map[string]string `json:"defaultfestate"`
	Status              string            `json:"status"`
	ConnectTimeout      int               `json:"connecttimeout,omitempty"`
	ErrorStr            string            `json:"errorstr,omitempty"`
	InstallStatus       string            `json:"installstatus"`
	InstallErrorStr     string            `json:"installerrorstr,omitempty"`
	NeedsMShellUpgrade  bool              `json:"needsmshellupgrade,omitempty"`
	NoInitPk            bool              `json:"noinitpk,omitempty"`
	AuthType            string            `json:"authtype,omitempty"`
	ConnectMode         string            `json:"connectmode"`
	AutoInstall         bool              `json:"autoinstall"`
	Archived            bool              `json:"archived,omitempty"`
	RemoteIdx           int64             `json:"remoteidx"`
	SSHConfigSrc        string            `json:"sshconfigsrc"`
	UName               string            `json:"uname"`
	MShellVersion       string            `json:"mshellversion"`
	WaitingForPassword  bool              `json:"waitingforpassword,omitempty"`
	Local               bool              `json:"local,omitempty"`
	RemoteOpts          *RemoteOptsType   `json:"remoteopts,omitempty"`
	CanComplete         bool              `json:"cancomplete,omitempty"`
	ActiveShells        []string          `json:"activeshells,omitempty"`
	ShellPref           string            `json:"shellpref,omitempty"`
	DefaultShellType    string            `json:"defaultshelltype,omitempty"`
}

func (state RemoteRuntimeState) IsConnected() bool {
	return state.Status == RemoteStatus_Connected
}

func (state RemoteRuntimeState) GetBaseDisplayName() string {
	if state.RemoteAlias != "" {
		return state.RemoteAlias
	}
	return state.RemoteCanonicalName
}

func (state RemoteRuntimeState) GetDisplayName(rptr *RemotePtrType) string {
	baseDisplayName := state.GetBaseDisplayName()
	if rptr == nil {
		return baseDisplayName
	}
	return rptr.GetDisplayName(baseDisplayName)
}

func (state RemoteRuntimeState) ExpandHomeDir(pathStr string) (string, error) {
	if pathStr != "~" && !strings.HasPrefix(pathStr, "~/") {
		return pathStr, nil
	}
	homeDir := state.RemoteVars["home"]
	if homeDir == "" {
		return "", fmt.Errorf("remote does not have HOME set, cannot do ~ expansion")
	}
	if pathStr == "~" {
		return homeDir, nil
	}
	return path.Join(homeDir, pathStr[2:]), nil
}

type RemoteType struct {
	RemoteId            string          `json:"remoteid"`
	RemoteType          string          `json:"remotetype"`
	RemoteAlias         string          `json:"remotealias"`
	RemoteCanonicalName string          `json:"remotecanonicalname"`
	RemoteOpts          *RemoteOptsType `json:"remoteopts"`
	LastConnectTs       int64           `json:"lastconnectts"`
	RemoteIdx           int64           `json:"remoteidx"`
	Archived            bool            `json:"archived"`

	// SSH fields
	Local        bool              `json:"local"`
	RemoteUser   string            `json:"remoteuser"`
	RemoteHost   string            `json:"remotehost"`
	ConnectMode  string            `json:"connectmode"`
	AutoInstall  bool              `json:"autoinstall"`
	SSHOpts      *SSHOpts          `json:"sshopts"`
	StateVars    map[string]string `json:"statevars"`
	SSHConfigSrc string            `json:"sshconfigsrc"`
	ShellPref    string            `json:"shellpref"` // bash, zsh, or detect

	// OpenAI fields (unused)
	OpenAIOpts *OpenAIOptsType `json:"openaiopts,omitempty"`
}

func (r *RemoteType) IsSudo() bool {
	return r.SSHOpts != nil && r.SSHOpts.IsSudo
}

func (r *RemoteType) GetName() string {
	if r.RemoteAlias != "" {
		return r.RemoteAlias
	}
	return r.RemoteCanonicalName
}

type CmdType struct {
	ScreenId     string              `json:"screenid"`
	LineId       string              `json:"lineid"`
	Remote       RemotePtrType       `json:"remote"`
	CmdStr       string              `json:"cmdstr"`
	RawCmdStr    string              `json:"rawcmdstr"`
	FeState      map[string]string   `json:"festate"`
	StatePtr     ShellStatePtr       `json:"state"`
	TermOpts     TermOpts            `json:"termopts"`
	OrigTermOpts TermOpts            `json:"origtermopts"`
	Status       string              `json:"status"`
	CmdPid       int                 `json:"cmdpid"`
	RemotePid    int                 `json:"remotepid"`
	RestartTs    int64               `json:"restartts,omitempty"`
	DoneTs       int64               `json:"donets"`
	ExitCode     int                 `json:"exitcode"`
	DurationMs   int                 `json:"durationms"`
	RunOut       []packet.PacketType `json:"runout,omitempty"`
	RtnState     bool                `json:"rtnstate,omitempty"`
	RtnStatePtr  ShellStatePtr       `json:"rtnstateptr,omitempty"`
	Remove       bool                `json:"remove,omitempty"`    // not persisted to DB
	Restarted    bool                `json:"restarted,omitempty"` // not persisted to DB
}

func (r *RemoteType) ToMap() map[string]interface{} {
	rtn := make(map[string]interface{})
	rtn["remoteid"] = r.RemoteId
	rtn["remotetype"] = r.RemoteType
	rtn["remotealias"] = r.RemoteAlias
	rtn["remotecanonicalname"] = r.RemoteCanonicalName
	rtn["remoteuser"] = r.RemoteUser
	rtn["remotehost"] = r.RemoteHost
	rtn["connectmode"] = r.ConnectMode
	rtn["autoinstall"] = r.AutoInstall
	rtn["sshopts"] = quickJson(r.SSHOpts)
	rtn["remoteopts"] = quickJson(r.RemoteOpts)
	rtn["lastconnectts"] = r.LastConnectTs
	rtn["archived"] = r.Archived
	rtn["remoteidx"] = r.RemoteIdx
	rtn["local"] = r.Local
	rtn["statevars"] = quickJson(r.StateVars)
	rtn["sshconfigsrc"] = r.SSHConfigSrc
	rtn["openaiopts"] = quickJson(r.OpenAIOpts)
	rtn["shellpref"] = r.ShellPref
	return rtn
}

func (r *RemoteType) FromMap(m map[string]interface{}) bool {
	quickSetStr(&r.RemoteId, m, "remoteid")
	quickSetStr(&r.RemoteType, m, "remotetype")
	quickSetStr(&r.RemoteAlias, m, "remotealias")
	quickSetStr(&r.RemoteCanonicalName, m, "remotecanonicalname")
	quickSetStr(&r.RemoteUser, m, "remoteuser")
	quickSetStr(&r.RemoteHost, m, "remotehost")
	quickSetStr(&r.ConnectMode, m, "connectmode")
	quickSetBool(&r.AutoInstall, m, "autoinstall")
	quickSetJson(&r.SSHOpts, m, "sshopts")
	quickSetJson(&r.RemoteOpts, m, "remoteopts")
	quickSetInt64(&r.LastConnectTs, m, "lastconnectts")
	quickSetBool(&r.Archived, m, "archived")
	quickSetInt64(&r.RemoteIdx, m, "remoteidx")
	quickSetBool(&r.Local, m, "local")
	quickSetJson(&r.StateVars, m, "statevars")
	quickSetStr(&r.SSHConfigSrc, m, "sshconfigsrc")
	quickSetJson(&r.OpenAIOpts, m, "openaiopts")
	quickSetStr(&r.ShellPref, m, "shellpref")
	return true
}

func (cmd *CmdType) ToMap() map[string]interface{} {
	rtn := make(map[string]interface{})
	rtn["screenid"] = cmd.ScreenId
	rtn["lineid"] = cmd.LineId
	rtn["remoteownerid"] = cmd.Remote.OwnerId
	rtn["remoteid"] = cmd.Remote.RemoteId
	rtn["remotename"] = cmd.Remote.Name
	rtn["cmdstr"] = cmd.CmdStr
	rtn["rawcmdstr"] = cmd.RawCmdStr
	rtn["festate"] = quickJson(cmd.FeState)
	rtn["statebasehash"] = cmd.StatePtr.BaseHash
	rtn["statediffhasharr"] = quickJsonArr(cmd.StatePtr.DiffHashArr)
	rtn["termopts"] = quickJson(cmd.TermOpts)
	rtn["origtermopts"] = quickJson(cmd.OrigTermOpts)
	rtn["status"] = cmd.Status
	rtn["cmdpid"] = cmd.CmdPid
	rtn["remotepid"] = cmd.RemotePid
	rtn["restartts"] = cmd.RestartTs
	rtn["donets"] = cmd.DoneTs
	rtn["exitcode"] = cmd.ExitCode
	rtn["durationms"] = cmd.DurationMs
	rtn["runout"] = quickJson(cmd.RunOut)
	rtn["rtnstate"] = cmd.RtnState
	rtn["rtnbasehash"] = cmd.RtnStatePtr.BaseHash
	rtn["rtndiffhasharr"] = quickJsonArr(cmd.RtnStatePtr.DiffHashArr)
	return rtn
}

func (cmd *CmdType) FromMap(m map[string]interface{}) bool {
	quickSetStr(&cmd.ScreenId, m, "screenid")
	quickSetStr(&cmd.LineId, m, "lineid")
	quickSetStr(&cmd.Remote.OwnerId, m, "remoteownerid")
	quickSetStr(&cmd.Remote.RemoteId, m, "remoteid")
	quickSetStr(&cmd.Remote.Name, m, "remotename")
	quickSetStr(&cmd.CmdStr, m, "cmdstr")
	quickSetStr(&cmd.RawCmdStr, m, "rawcmdstr")
	quickSetJson(&cmd.FeState, m, "festate")
	quickSetStr(&cmd.StatePtr.BaseHash, m, "statebasehash")
	quickSetJsonArr(&cmd.StatePtr.DiffHashArr, m, "statediffhasharr")
	quickSetJson(&cmd.TermOpts, m, "termopts")
	quickSetJson(&cmd.OrigTermOpts, m, "origtermopts")
	quickSetStr(&cmd.Status, m, "status")
	quickSetInt(&cmd.CmdPid, m, "cmdpid")
	quickSetInt(&cmd.RemotePid, m, "remotepid")
	quickSetInt64(&cmd.DoneTs, m, "donets")
	quickSetInt64(&cmd.RestartTs, m, "restartts")
	quickSetInt(&cmd.ExitCode, m, "exitcode")
	quickSetInt(&cmd.DurationMs, m, "durationms")
	quickSetJson(&cmd.RunOut, m, "runout")
	quickSetBool(&cmd.RtnState, m, "rtnstate")
	quickSetStr(&cmd.RtnStatePtr.BaseHash, m, "rtnbasehash")
	quickSetJsonArr(&cmd.RtnStatePtr.DiffHashArr, m, "rtndiffhasharr")
	return true
}

func (cmd *CmdType) IsRunning() bool {
	return cmd.Status == CmdStatusRunning || cmd.Status == CmdStatusDetached
}

func makeNewLineCmd(screenId string, userId string, lineId string, renderer string, lineState map[string]any) *LineType {
	rtn := &LineType{}
	rtn.ScreenId = screenId
	rtn.UserId = userId
	rtn.LineId = lineId
	rtn.Ts = time.Now().UnixMilli()
	rtn.LineLocal = true
	rtn.LineType = LineTypeCmd
	rtn.LineId = lineId
	rtn.ContentHeight = LineNoHeight
	rtn.Renderer = renderer
	if lineState == nil {
		lineState = make(map[string]any)
	}
	rtn.LineState = lineState
	return rtn
}

func makeNewLineText(screenId string, userId string, text string) *LineType {
	rtn := &LineType{}
	rtn.ScreenId = screenId
	rtn.UserId = userId
	rtn.LineId = scbase.GenWaveUUID()
	rtn.Ts = time.Now().UnixMilli()
	rtn.LineLocal = true
	rtn.LineType = LineTypeText
	rtn.Text = text
	rtn.ContentHeight = LineNoHeight
	rtn.LineState = make(map[string]any)
	return rtn
}

func makeNewLineOpenAI(screenId string, userId string, lineId string) *LineType {
	rtn := &LineType{}
	rtn.ScreenId = screenId
	rtn.UserId = userId
	rtn.LineId = lineId
	rtn.Ts = time.Now().UnixMilli()
	rtn.LineLocal = true
	rtn.LineType = LineTypeOpenAI
	rtn.ContentHeight = LineNoHeight
	rtn.Renderer = CmdRendererOpenAI
	rtn.LineState = make(map[string]any)
	return rtn
}

func AddCommentLine(ctx context.Context, screenId string, userId string, commentText string) (*LineType, error) {
	rtnLine := makeNewLineText(screenId, userId, commentText)
	err := InsertLine(ctx, rtnLine, nil)
	if err != nil {
		return nil, err
	}
	return rtnLine, nil
}

func AddOpenAILine(ctx context.Context, screenId string, userId string, cmd *CmdType) (*LineType, error) {
	rtnLine := makeNewLineOpenAI(screenId, userId, cmd.LineId)
	err := InsertLine(ctx, rtnLine, cmd)
	if err != nil {
		return nil, err
	}
	return rtnLine, nil
}

func AddCmdLine(ctx context.Context, screenId string, userId string, cmd *CmdType, renderer string, lineState map[string]any) (*LineType, error) {
	rtnLine := makeNewLineCmd(screenId, userId, cmd.LineId, renderer, lineState)
	err := InsertLine(ctx, rtnLine, cmd)
	if err != nil {
		return nil, err
	}
	return rtnLine, nil
}

func EnsureLocalRemote(ctx context.Context) error {
	remote, err := GetLocalRemote(ctx)
	if err != nil {
		return fmt.Errorf("getting local remote from db: %w", err)
	}
	if remote != nil {
		return nil
	}
	hostName, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("getting hostname: %w", err)
	}
	user, err := user.Current()
	if err != nil {
		return fmt.Errorf("getting user: %w", err)
	}
	// create the local remote
	localRemote := &RemoteType{
		RemoteId:            scbase.GenWaveUUID(),
		RemoteType:          RemoteTypeSsh,
		RemoteAlias:         LocalRemoteAlias,
		RemoteCanonicalName: fmt.Sprintf("%s@%s", user.Username, hostName),
		RemoteUser:          user.Username,
		RemoteHost:          hostName,
		ConnectMode:         ConnectModeStartup,
		AutoInstall:         true,
		SSHOpts:             &SSHOpts{Local: true},
		Local:               true,
		SSHConfigSrc:        SSHConfigSrcTypeManual,
		ShellPref:           ShellTypePref_Detect,
	}
	err = UpsertRemote(ctx, localRemote)
	if err != nil {
		return err
	}
	log.Printf("[db] added local remote '%s', id=%s\n", localRemote.RemoteCanonicalName, localRemote.RemoteId)
	sudoRemote := &RemoteType{
		RemoteId:            scbase.GenWaveUUID(),
		RemoteType:          RemoteTypeSsh,
		RemoteAlias:         "sudo",
		RemoteCanonicalName: fmt.Sprintf("sudo@%s@%s", user.Username, hostName),
		RemoteUser:          "root",
		RemoteHost:          hostName,
		ConnectMode:         ConnectModeManual,
		AutoInstall:         true,
		SSHOpts:             &SSHOpts{Local: true, IsSudo: true},
		RemoteOpts:          &RemoteOptsType{Color: "red"},
		Local:               true,
		SSHConfigSrc:        SSHConfigSrcTypeManual,
		ShellPref:           ShellTypePref_Detect,
	}
	err = UpsertRemote(ctx, sudoRemote)
	if err != nil {
		return err
	}
	log.Printf("[db] added sudo remote '%s', id=%s\n", sudoRemote.RemoteCanonicalName, sudoRemote.RemoteId)
	return nil
}

func EnsureOneSession(ctx context.Context) error {
	numSessions, err := GetSessionCount(ctx)
	if err != nil {
		return err
	}
	if numSessions > 0 {
		return nil
	}
	_, err = InsertSessionWithName(ctx, DefaultSessionName, true)
	if err != nil {
		return err
	}
	return nil
}

func createClientData(tx *TxWrap) error {
	curve := elliptic.P384()
	pkey, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return fmt.Errorf("generating P-834 key: %w", err)
	}
	pkBytes, err := x509.MarshalECPrivateKey(pkey)
	if err != nil {
		return fmt.Errorf("marshaling (pkcs8) private key bytes: %w", err)
	}
	pubBytes, err := x509.MarshalPKIXPublicKey(&pkey.PublicKey)
	if err != nil {
		return fmt.Errorf("marshaling (pkix) public key bytes: %w", err)
	}
	c := ClientData{
		ClientId:            uuid.New().String(),
		UserId:              uuid.New().String(),
		UserPrivateKeyBytes: pkBytes,
		UserPublicKeyBytes:  pubBytes,
		ActiveSessionId:     "",
		WinSize:             ClientWinSizeType{},
		CmdStoreType:        CmdStoreTypeScreen,
		ReleaseInfo:         ReleaseInfoType{},
	}
	query := `INSERT INTO client ( clientid, userid, activesessionid, userpublickeybytes, userprivatekeybytes, winsize, cmdstoretype, releaseinfo) 
                          VALUES (:clientid,:userid,:activesessionid,:userpublickeybytes,:userprivatekeybytes,:winsize,:cmdstoretype,:releaseinfo)`
	tx.NamedExec(query, dbutil.ToDBMap(c, false))
	log.Printf("create new clientid[%s] userid[%s] with public/private keypair\n", c.ClientId, c.UserId)
	return nil
}

func EnsureClientData(ctx context.Context) (*ClientData, error) {
	rtn, err := WithTxRtn(ctx, func(tx *TxWrap) (*ClientData, error) {
		query := `SELECT count(*) FROM client`
		count := tx.GetInt(query)
		if count > 1 {
			return nil, fmt.Errorf("invalid client database, multiple (%d) rows in client table", count)
		}
		if count == 0 {
			createErr := createClientData(tx)
			if createErr != nil {
				return nil, createErr
			}
		}
		cdata := dbutil.GetMappable[*ClientData](tx, `SELECT * FROM client`)
		if cdata == nil {
			return nil, fmt.Errorf("no client data found")
		}
		dbVersion := tx.GetInt(`SELECT version FROM schema_migrations`)
		cdata.DBVersion = dbVersion
		return cdata, nil
	})
	if err != nil {
		return nil, err
	}
	if rtn.UserId == "" {
		return nil, fmt.Errorf("invalid client data (no userid)")
	}
	if len(rtn.UserPrivateKeyBytes) == 0 || len(rtn.UserPublicKeyBytes) == 0 {
		return nil, fmt.Errorf("invalid client data (no public/private keypair)")
	}
	rtn.UserPrivateKey, err = x509.ParseECPrivateKey(rtn.UserPrivateKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid client data, cannot parse private key: %w", err)
	}
	pubKey, err := x509.ParsePKIXPublicKey(rtn.UserPublicKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid client data, cannot parse public key: %w", err)
	}
	var ok bool
	rtn.UserPublicKey, ok = pubKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("invalid client data, wrong public key type: %T", pubKey)
	}
	return rtn, nil
}

func SetClientOpts(ctx context.Context, clientOpts ClientOptsType) error {
	txErr := WithTx(ctx, func(tx *TxWrap) error {
		query := `UPDATE client SET clientopts = ?`
		tx.Exec(query, quickJson(clientOpts))
		return nil
	})
	return txErr
}

func SetReleaseInfo(ctx context.Context, releaseInfo ReleaseInfoType) error {
	txErr := WithTx(ctx, func(tx *TxWrap) error {
		query := `UPDATE client SET releaseinfo = ?`
		tx.Exec(query, quickJson(releaseInfo))
		return nil
	})
	return txErr
}

// Sets the in-memory status indicator for the given screenId to the given value and adds it to the ModelUpdate. By default, the active screen will be ignored when updating status. To force a status update for the active screen, set force=true.
func SetStatusIndicatorLevel_Update(ctx context.Context, update *ModelUpdate, screenId string, level StatusIndicatorLevel, force bool) error {
	var newStatus StatusIndicatorLevel
	if force {
		// Force the update and set the new status to the given level, regardless of the current status or the active screen
		ScreenMemSetIndicatorLevel(screenId, level)
		newStatus = level
	} else {
		// Only update the status if the given screen is not the active screen and if the given level is higher than the current level
		activeSessionId, err := GetActiveSessionId(ctx)
		if err != nil {
			return fmt.Errorf("error getting active session id: %w", err)
		}
		bareSession, err := GetBareSessionById(ctx, activeSessionId)
		if err != nil {
			return fmt.Errorf("error getting bare session: %w", err)
		}
		activeScreenId := bareSession.ActiveScreenId
		if activeScreenId == screenId {
			return nil
		}

		// If we are not forcing the update, follow the rules for combining status indicators
		newLevel := ScreenMemCombineIndicatorLevels(screenId, level)
		if newLevel == level {
			newStatus = level
		} else {
			return nil
		}
	}

	update.ScreenStatusIndicators = []*ScreenStatusIndicatorType{{
		ScreenId: screenId,
		Status:   newStatus,
	}}
	return nil
}

// Sets the in-memory status indicator for the given screenId to the given value and pushes the new value to the FE
func SetStatusIndicatorLevel(ctx context.Context, screenId string, level StatusIndicatorLevel, force bool) error {
	update := &ModelUpdate{}
	err := SetStatusIndicatorLevel_Update(ctx, update, screenId, level, false)
	if err != nil {
		return err
	}
	MainBus.SendUpdate(update)
	return nil
}

// Resets the in-memory status indicator for the given screenId to StatusIndicatorLevel_None and adds it to the ModelUpdate
func ResetStatusIndicator_Update(update *ModelUpdate, screenId string) error {
	// We do not need to set context when resetting the status indicator because we will not need to call the DB
	return SetStatusIndicatorLevel_Update(context.TODO(), update, screenId, StatusIndicatorLevel_None, true)
}

// Resets the in-memory status indicator for the given screenId to StatusIndicatorLevel_None and pushes the new value to the FE
func ResetStatusIndicator(screenId string) error {
	// We do not need to set context when resetting the status indicator because we will not need to call the DB
	return SetStatusIndicatorLevel(context.TODO(), screenId, StatusIndicatorLevel_None, true)
}

func IncrementNumRunningCmds_Update(update *ModelUpdate, screenId string, delta int) {
	newNum := ScreenMemIncrementNumRunningCommands(screenId, delta)
	log.Printf("IncrementNumRunningCmds_Update: screenId=%s, newNum=%d\n", screenId, newNum)
	update.ScreenNumRunningCommands = []*ScreenNumRunningCommandsType{{
		ScreenId: screenId,
		Num:      newNum,
	}}
}

func IncrementNumRunningCmds(screenId string, delta int) {
	log.Printf("IncrementNumRunningCmds: screenId=%s, delta=%d\n", screenId, delta)
	update := &ModelUpdate{}
	IncrementNumRunningCmds_Update(update, screenId, delta)
	MainBus.SendUpdate(update)
}
