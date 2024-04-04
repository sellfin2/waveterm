// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/alessio/shellescape"
	"github.com/armon/circbuf"
	"github.com/creack/pty"
	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/waveshell/pkg/base"
	"github.com/wavetermdev/waveterm/waveshell/pkg/packet"
	"github.com/wavetermdev/waveterm/waveshell/pkg/server"
	"github.com/wavetermdev/waveterm/waveshell/pkg/shellapi"
	"github.com/wavetermdev/waveterm/waveshell/pkg/shellenv"
	"github.com/wavetermdev/waveterm/waveshell/pkg/shexec"
	"github.com/wavetermdev/waveterm/waveshell/pkg/statediff"
	"github.com/wavetermdev/waveterm/waveshell/pkg/utilfn"
	"github.com/wavetermdev/waveterm/wavesrv/pkg/ephemeral"
	"github.com/wavetermdev/waveterm/wavesrv/pkg/scbase"
	"github.com/wavetermdev/waveterm/wavesrv/pkg/scbus"
	"github.com/wavetermdev/waveterm/wavesrv/pkg/scpacket"
	"github.com/wavetermdev/waveterm/wavesrv/pkg/sstore"
	"github.com/wavetermdev/waveterm/wavesrv/pkg/telemetry"
	"github.com/wavetermdev/waveterm/wavesrv/pkg/userinput"

	"golang.org/x/crypto/ssh"
	"golang.org/x/mod/semver"
)

const RemoteTypeMShell = "mshell"
const DefaultTerm = "xterm-256color"
const DefaultMaxPtySize = 1024 * 1024
const CircBufSize = 64 * 1024
const RemoteTermRows = 8
const RemoteTermCols = 80
const PtyReadBufSize = 100
const RemoteConnectTimeout = 15 * time.Second
const RpcIterChannelSize = 100
const MaxInputDataSize = 1000

var envVarsToStrip map[string]bool = map[string]bool{
	"PROMPT":               true,
	"PROMPT_VERSION":       true,
	"MSHELL":               true,
	"MSHELL_VERSION":       true,
	"WAVETERM":             true,
	"WAVETERM_VERSION":     true,
	"TERM_PROGRAM":         true,
	"TERM_PROGRAM_VERSION": true,
	"TERM_SESSION_ID":      true,
}

// we add this ping packet to the MShellServer Commands in order to deal with spurious SSH output
// basically we guarantee the parser will see a valid packet (either an init error or a ping)
// so we can pass ignoreUntilValid to PacketParser
const PrintPingPacket = `printf "\n##N{\"type\": \"ping\"}\n"`

const MShellServerCommandFmt = `
PATH=$PATH:~/.mshell;
which mshell-[%VERSION%] > /dev/null;
if [[ "$?" -ne 0 ]]
then
  printf "\n##N{\"type\": \"init\", \"notfound\": true, \"uname\": \"%s | %s\"}\n" "$(uname -s)" "$(uname -m)"
else
  [%PINGPACKET%]
  mshell-[%VERSION%] --server
fi
`

func MakeLocalMShellCommandStr(isSudo bool) (string, error) {
	mshellPath, err := scbase.LocalMShellBinaryPath()
	if err != nil {
		return "", err
	}
	if isSudo {
		return fmt.Sprintf(`%s; sudo %s --server`, PrintPingPacket, shellescape.Quote(mshellPath)), nil
	} else {
		return fmt.Sprintf(`%s; %s --server`, PrintPingPacket, shellescape.Quote(mshellPath)), nil
	}
}

func MakeServerCommandStr() string {
	rtn := strings.ReplaceAll(MShellServerCommandFmt, "[%VERSION%]", semver.MajorMinor(scbase.MShellVersion))
	rtn = strings.ReplaceAll(rtn, "[%PINGPACKET%]", PrintPingPacket)
	return rtn
}

const (
	StatusConnected    = sstore.RemoteStatus_Connected
	StatusConnecting   = sstore.RemoteStatus_Connecting
	StatusDisconnected = sstore.RemoteStatus_Disconnected
	StatusError        = sstore.RemoteStatus_Error
)

func init() {
	if scbase.MShellVersion != base.MShellVersion {
		panic(fmt.Sprintf("prompt-server apishell version must match '%s' vs '%s'", scbase.MShellVersion, base.MShellVersion))
	}
}

var GlobalStore *Store

type Store struct {
	Lock       *sync.Mutex
	Map        map[string]*MShellProc // key=remoteid
	CmdWaitMap map[base.CommandKey][]func()
}

type pendingStateKey struct {
	ScreenId  string
	RemotePtr sstore.RemotePtrType
}

// provides state, acccess, and control for a waveshell server process
type MShellProc struct {
	Lock   *sync.Mutex
	Remote *sstore.RemoteType

	// runtime
	RemoteId           string // can be read without a lock
	Status             string
	ServerProc         *shexec.ClientProc // the server process
	UName              string
	Err                error
	ErrNoInitPk        bool
	ControllingPty     *os.File
	PtyBuffer          *circbuf.Buffer
	MakeClientCancelFn context.CancelFunc
	MakeClientDeadline *time.Time
	StateMap           *server.ShellStateMap
	NumTryConnect      int
	InitPkShellType    string
	DataPosMap         *utilfn.SyncMap[base.CommandKey, int64]

	// install
	InstallStatus      string
	NeedsMShellUpgrade bool
	InstallCancelFn    context.CancelFunc
	InstallErr         error

	// for synthetic commands (not run through RunCommand), this provides a way for them
	// to register to receive input events from the frontend (e.g. ReInit)
	CommandInputMap map[base.CommandKey]CommandInputSink

	RunningCmds      map[base.CommandKey]*RunCmdType
	PendingStateCmds map[pendingStateKey]base.CommandKey // key=[remoteinstance name] (in progress commands that might update the state)

	Client *ssh.Client
}

type CommandInputSink interface {
	HandleInput(feInput *scpacket.FeInputPacketType) error
}

type RunCmdType struct {
	CK            base.CommandKey
	SessionId     string
	ScreenId      string
	RemotePtr     sstore.RemotePtrType
	RunPacket     *packet.RunPacketType
	EphemeralOpts *ephemeral.EphemeralRunOpts
}

type ReinitCommandSink struct {
	Remote *MShellProc
	ReqId  string
}

func (rcs *ReinitCommandSink) HandleInput(feInput *scpacket.FeInputPacketType) error {
	realData, err := base64.StdEncoding.DecodeString(feInput.InputData64)
	if err != nil {
		return fmt.Errorf("error decoding input data: %v", err)
	}
	inputPk := packet.MakeRpcInputPacket(rcs.ReqId)
	inputPk.Data = realData
	rcs.Remote.ServerProc.Input.SendPacket(inputPk)
	return nil
}

type RemoteRuntimeState = sstore.RemoteRuntimeState

func CanComplete(remoteType string) bool {
	switch remoteType {
	case sstore.RemoteTypeSsh:
		return true
	default:
		return false
	}
}

func (msh *MShellProc) GetStatus() string {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	return msh.Status
}

func (msh *MShellProc) GetRemoteId() string {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	return msh.Remote.RemoteId
}

func (msh *MShellProc) GetInstallStatus() string {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	return msh.InstallStatus
}

func LoadRemotes(ctx context.Context) error {
	GlobalStore = &Store{
		Lock:       &sync.Mutex{},
		Map:        make(map[string]*MShellProc),
		CmdWaitMap: make(map[base.CommandKey][]func()),
	}
	allRemotes, err := sstore.GetAllRemotes(ctx)
	if err != nil {
		return err
	}
	var numLocal int
	var numSudoLocal int
	for _, remote := range allRemotes {
		msh := MakeMShell(remote)
		GlobalStore.Map[remote.RemoteId] = msh
		if remote.ConnectMode == sstore.ConnectModeStartup {
			go msh.Launch(false)
		}
		if remote.Local {
			if remote.IsSudo() {
				numSudoLocal++
			} else {
				numLocal++
			}
		}
	}
	if numLocal == 0 {
		return fmt.Errorf("no local remote found")
	}
	if numLocal > 1 {
		return fmt.Errorf("multiple local remotes found")
	}
	if numSudoLocal > 1 {
		return fmt.Errorf("multiple local sudo remotes found")
	}
	return nil
}

func LoadRemoteById(ctx context.Context, remoteId string) error {
	r, err := sstore.GetRemoteById(ctx, remoteId)
	if err != nil {
		return err
	}
	if r == nil {
		return fmt.Errorf("remote %s not found", remoteId)
	}
	msh := MakeMShell(r)
	GlobalStore.Lock.Lock()
	defer GlobalStore.Lock.Unlock()
	existingRemote := GlobalStore.Map[remoteId]
	if existingRemote != nil {
		return fmt.Errorf("cannot add remote %s, already in global map", remoteId)
	}
	GlobalStore.Map[r.RemoteId] = msh
	if r.ConnectMode == sstore.ConnectModeStartup {
		go msh.Launch(false)
	}
	return nil
}

func ReadRemotePty(ctx context.Context, remoteId string) (int64, []byte, error) {
	GlobalStore.Lock.Lock()
	defer GlobalStore.Lock.Unlock()
	msh := GlobalStore.Map[remoteId]
	if msh == nil {
		return 0, nil, nil
	}
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	barr := msh.PtyBuffer.Bytes()
	offset := msh.PtyBuffer.TotalWritten() - int64(len(barr))
	return offset, barr, nil
}

func AddRemote(ctx context.Context, r *sstore.RemoteType, shouldStart bool) error {
	GlobalStore.Lock.Lock()
	defer GlobalStore.Lock.Unlock()

	existingRemote := getRemoteByCanonicalName_nolock(r.RemoteCanonicalName)
	if existingRemote != nil {
		erCopy := existingRemote.GetRemoteCopy()
		if !erCopy.Archived {
			return fmt.Errorf("duplicate canonical name %q: cannot create new remote", r.RemoteCanonicalName)
		}
		r.RemoteId = erCopy.RemoteId
	}
	if r.Local {
		return fmt.Errorf("cannot create another local remote (there can be only one)")
	}

	err := sstore.UpsertRemote(ctx, r)
	if err != nil {
		return fmt.Errorf("cannot create remote %q: %v", r.RemoteCanonicalName, err)
	}
	newMsh := MakeMShell(r)
	GlobalStore.Map[r.RemoteId] = newMsh
	go newMsh.NotifyRemoteUpdate()
	if shouldStart {
		go newMsh.Launch(true)
	}
	return nil
}

func ArchiveRemote(ctx context.Context, remoteId string) error {
	GlobalStore.Lock.Lock()
	defer GlobalStore.Lock.Unlock()
	msh := GlobalStore.Map[remoteId]
	if msh == nil {
		return fmt.Errorf("remote not found, cannot archive")
	}
	if msh.Status == StatusConnected {
		return fmt.Errorf("cannot archive connected remote")
	}
	if msh.Remote.Local {
		return fmt.Errorf("cannot archive local remote")
	}
	rcopy := msh.GetRemoteCopy()
	archivedRemote := &sstore.RemoteType{
		RemoteId:            rcopy.RemoteId,
		RemoteType:          rcopy.RemoteType,
		RemoteCanonicalName: rcopy.RemoteCanonicalName,
		ConnectMode:         sstore.ConnectModeManual,
		Archived:            true,
		SSHConfigSrc:        rcopy.SSHConfigSrc,
	}
	err := sstore.UpsertRemote(ctx, archivedRemote)
	if err != nil {
		return err
	}
	newMsh := MakeMShell(archivedRemote)
	GlobalStore.Map[remoteId] = newMsh
	go newMsh.NotifyRemoteUpdate()
	return nil
}

var partialUUIDRe = regexp.MustCompile("^[0-9a-f]{8}$")

func isPartialUUID(s string) bool {
	return partialUUIDRe.MatchString(s)
}

func NumRemotes() int {
	GlobalStore.Lock.Lock()
	defer GlobalStore.Lock.Unlock()
	return len(GlobalStore.Map)
}

func GetRemoteByArg(arg string) *MShellProc {
	GlobalStore.Lock.Lock()
	defer GlobalStore.Lock.Unlock()
	isPuid := isPartialUUID(arg)
	for _, msh := range GlobalStore.Map {
		rcopy := msh.GetRemoteCopy()
		if rcopy.RemoteAlias == arg || rcopy.RemoteCanonicalName == arg || rcopy.RemoteId == arg {
			return msh
		}
		if isPuid && strings.HasPrefix(rcopy.RemoteId, arg) {
			return msh
		}
	}
	return nil
}

func getRemoteByCanonicalName_nolock(name string) *MShellProc {
	for _, msh := range GlobalStore.Map {
		rcopy := msh.GetRemoteCopy()
		if rcopy.RemoteCanonicalName == name {
			return msh
		}
	}
	return nil
}

func GetRemoteById(remoteId string) *MShellProc {
	GlobalStore.Lock.Lock()
	defer GlobalStore.Lock.Unlock()
	return GlobalStore.Map[remoteId]
}

func GetRemoteCopyById(remoteId string) *sstore.RemoteType {
	msh := GetRemoteById(remoteId)
	if msh == nil {
		return nil
	}
	rcopy := msh.GetRemoteCopy()
	return &rcopy
}

func GetRemoteMap() map[string]*MShellProc {
	GlobalStore.Lock.Lock()
	defer GlobalStore.Lock.Unlock()
	rtn := make(map[string]*MShellProc)
	for remoteId, msh := range GlobalStore.Map {
		rtn[remoteId] = msh
	}
	return rtn
}

func GetLocalRemote() *MShellProc {
	GlobalStore.Lock.Lock()
	defer GlobalStore.Lock.Unlock()
	for _, msh := range GlobalStore.Map {
		if msh.IsLocal() && !msh.IsSudo() {
			return msh
		}
	}
	return nil
}

func ResolveRemoteRef(remoteRef string) *RemoteRuntimeState {
	GlobalStore.Lock.Lock()
	defer GlobalStore.Lock.Unlock()

	_, err := uuid.Parse(remoteRef)
	if err == nil {
		msh := GlobalStore.Map[remoteRef]
		if msh != nil {
			state := msh.GetRemoteRuntimeState()
			return &state
		}
		return nil
	}
	for _, msh := range GlobalStore.Map {
		if msh.Remote.RemoteAlias == remoteRef || msh.Remote.RemoteCanonicalName == remoteRef {
			state := msh.GetRemoteRuntimeState()
			return &state
		}
	}
	return nil
}

func SendSignalToCmd(ctx context.Context, cmd *sstore.CmdType, sig string) error {
	msh := GetRemoteById(cmd.Remote.RemoteId)
	if msh == nil {
		return fmt.Errorf("no connection found")
	}
	if !msh.IsConnected() {
		return fmt.Errorf("not connected")
	}
	cmdCk := base.MakeCommandKey(cmd.ScreenId, cmd.LineId)
	if !msh.IsCmdRunning(cmdCk) {
		// this could also return nil (depends on use case)
		// settled on coded error so we can check for this error
		return base.CodedErrorf(packet.EC_CmdNotRunning, "cmd not running")
	}
	sigPk := packet.MakeSpecialInputPacket()
	sigPk.CK = cmdCk
	sigPk.SigName = sig
	return msh.ServerProc.Input.SendPacket(sigPk)
}

func unquoteDQBashString(str string) (string, bool) {
	if len(str) < 2 {
		return str, false
	}
	if str[0] != '"' || str[len(str)-1] != '"' {
		return str, false
	}
	rtn := make([]byte, 0, len(str)-2)
	for idx := 1; idx < len(str)-1; idx++ {
		ch := str[idx]
		if ch == '"' {
			return str, false
		}
		if ch == '\\' {
			if idx == len(str)-2 {
				return str, false
			}
			nextCh := str[idx+1]
			if nextCh == '\n' {
				idx++
				continue
			}
			if nextCh == '$' || nextCh == '"' || nextCh == '\\' || nextCh == '`' {
				idx++
				rtn = append(rtn, nextCh)
				continue
			}
			rtn = append(rtn, '\\')
			continue
		} else {
			rtn = append(rtn, ch)
		}
	}
	return string(rtn), true
}

func makeShortHost(host string) string {
	dotIdx := strings.Index(host, ".")
	if dotIdx == -1 {
		return host
	}
	return host[0:dotIdx]
}

func (msh *MShellProc) IsLocal() bool {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	return msh.Remote.Local
}

func (msh *MShellProc) IsSudo() bool {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	return msh.Remote.IsSudo()
}

func (msh *MShellProc) tryAutoInstall() {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	if !msh.Remote.AutoInstall || !msh.NeedsMShellUpgrade || msh.InstallErr != nil {
		return
	}
	msh.writeToPtyBuffer_nolock("trying auto-install\n")
	go msh.RunInstall(true)
}

// if msh.IsConnected() then GetShellPref() should return a valid shell
// if msh is not connected, then InitPkShellType might be empty
func (msh *MShellProc) GetShellPref() string {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	if msh.Remote.ShellPref == sstore.ShellTypePref_Detect {
		return msh.InitPkShellType
	}
	if msh.Remote.ShellPref == "" {
		return packet.ShellType_bash
	}
	return msh.Remote.ShellPref
}

func (msh *MShellProc) GetRemoteRuntimeState() RemoteRuntimeState {
	shellPref := msh.GetShellPref()
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	state := RemoteRuntimeState{
		RemoteType:          msh.Remote.RemoteType,
		RemoteId:            msh.Remote.RemoteId,
		RemoteAlias:         msh.Remote.RemoteAlias,
		RemoteCanonicalName: msh.Remote.RemoteCanonicalName,
		Status:              msh.Status,
		ConnectMode:         msh.Remote.ConnectMode,
		AutoInstall:         msh.Remote.AutoInstall,
		Archived:            msh.Remote.Archived,
		RemoteIdx:           msh.Remote.RemoteIdx,
		SSHConfigSrc:        msh.Remote.SSHConfigSrc,
		UName:               msh.UName,
		InstallStatus:       msh.InstallStatus,
		NeedsMShellUpgrade:  msh.NeedsMShellUpgrade,
		Local:               msh.Remote.Local,
		IsSudo:              msh.Remote.IsSudo(),
		NoInitPk:            msh.ErrNoInitPk,
		AuthType:            sstore.RemoteAuthTypeNone,
		ShellPref:           msh.Remote.ShellPref,
		DefaultShellType:    shellPref,
	}
	if msh.Remote.SSHOpts != nil {
		state.AuthType = msh.Remote.SSHOpts.GetAuthType()
	}
	if msh.Remote.RemoteOpts != nil {
		optsCopy := *msh.Remote.RemoteOpts
		state.RemoteOpts = &optsCopy
	}
	if msh.Err != nil {
		state.ErrorStr = msh.Err.Error()
	}
	if msh.InstallErr != nil {
		state.InstallErrorStr = msh.InstallErr.Error()
	}
	if msh.Status == StatusConnecting {
		state.WaitingForPassword = msh.isWaitingForPassword_nolock()
		if msh.MakeClientDeadline != nil {
			state.ConnectTimeout = int(time.Until(*msh.MakeClientDeadline) / time.Second)
			if state.ConnectTimeout < 0 {
				state.ConnectTimeout = 0
			}
			state.CountdownActive = true
		} else {
			state.CountdownActive = false
		}
	}
	vars := msh.Remote.StateVars
	if vars == nil {
		vars = make(map[string]string)
	}
	vars["user"] = msh.Remote.RemoteUser
	vars["bestuser"] = vars["user"]
	vars["host"] = msh.Remote.RemoteHost
	vars["shorthost"] = makeShortHost(msh.Remote.RemoteHost)
	vars["alias"] = msh.Remote.RemoteAlias
	vars["cname"] = msh.Remote.RemoteCanonicalName
	vars["remoteid"] = msh.Remote.RemoteId
	vars["status"] = msh.Status
	vars["type"] = msh.Remote.RemoteType
	if msh.Remote.IsSudo() {
		vars["sudo"] = "1"
	}
	if msh.Remote.Local {
		vars["local"] = "1"
	}
	vars["port"] = "22"
	if msh.Remote.SSHOpts != nil {
		if msh.Remote.SSHOpts.SSHPort != 0 {
			vars["port"] = strconv.Itoa(msh.Remote.SSHOpts.SSHPort)
		}
	}
	if msh.Remote.RemoteOpts != nil && msh.Remote.RemoteOpts.Color != "" {
		vars["color"] = msh.Remote.RemoteOpts.Color
	}
	if msh.ServerProc != nil && msh.ServerProc.InitPk != nil {
		initPk := msh.ServerProc.InitPk
		if initPk.BuildTime == "" || initPk.BuildTime == "0" {
			state.MShellVersion = initPk.Version
		} else {
			state.MShellVersion = fmt.Sprintf("%s+%s", initPk.Version, initPk.BuildTime)
		}
		vars["home"] = initPk.HomeDir
		vars["remoteuser"] = initPk.User
		vars["bestuser"] = vars["remoteuser"]
		vars["remotehost"] = initPk.HostName
		vars["remoteshorthost"] = makeShortHost(initPk.HostName)
		vars["besthost"] = vars["remotehost"]
		vars["bestshorthost"] = vars["remoteshorthost"]
	}
	if msh.Remote.Local && msh.Remote.IsSudo() {
		vars["bestuser"] = "sudo"
	} else if msh.Remote.IsSudo() {
		vars["bestuser"] = "sudo@" + vars["bestuser"]
	}
	if msh.Remote.Local {
		vars["bestname"] = vars["bestuser"] + "@local"
		vars["bestshortname"] = vars["bestuser"] + "@local"
	} else {
		vars["bestname"] = vars["bestuser"] + "@" + vars["besthost"]
		vars["bestshortname"] = vars["bestuser"] + "@" + vars["bestshorthost"]
	}
	if vars["remoteuser"] == "root" || vars["sudo"] == "1" {
		vars["isroot"] = "1"
	}
	varsCopy := make(map[string]string)
	// deep copy so that concurrent calls don't collide on this data
	for key, value := range vars {
		varsCopy[key] = value
	}
	state.RemoteVars = varsCopy
	return state
}

func (msh *MShellProc) NotifyRemoteUpdate() {
	rstate := msh.GetRemoteRuntimeState()
	update := scbus.MakeUpdatePacket()
	update.AddUpdate(rstate)
	scbus.MainUpdateBus.DoUpdate(update)
}

func GetAllRemoteRuntimeState() []*RemoteRuntimeState {
	GlobalStore.Lock.Lock()
	defer GlobalStore.Lock.Unlock()

	var rtn []*RemoteRuntimeState
	for _, proc := range GlobalStore.Map {
		state := proc.GetRemoteRuntimeState()
		rtn = append(rtn, &state)
	}
	return rtn
}

func MakeMShell(r *sstore.RemoteType) *MShellProc {
	buf, err := circbuf.NewBuffer(CircBufSize)
	if err != nil {
		panic(err) // this should never happen (NewBuffer only returns an error if CirBufSize <= 0)
	}
	rtn := &MShellProc{
		Lock:             &sync.Mutex{},
		Remote:           r,
		RemoteId:         r.RemoteId,
		Status:           StatusDisconnected,
		PtyBuffer:        buf,
		InstallStatus:    StatusDisconnected,
		CommandInputMap:  make(map[base.CommandKey]CommandInputSink),
		RunningCmds:      make(map[base.CommandKey]*RunCmdType),
		PendingStateCmds: make(map[pendingStateKey]base.CommandKey),
		StateMap:         server.MakeShellStateMap(),
		DataPosMap:       utilfn.MakeSyncMap[base.CommandKey, int64](),
	}

	rtn.WriteToPtyBuffer("console for connection [%s]\n", r.GetName())
	return rtn
}

func SendRemoteInput(pk *scpacket.RemoteInputPacketType) error {
	data, err := base64.StdEncoding.DecodeString(pk.InputData64)
	if err != nil {
		return fmt.Errorf("cannot decode base64: %v", err)
	}
	msh := GetRemoteById(pk.RemoteId)
	if msh == nil {
		return fmt.Errorf("remote not found")
	}
	var cmdPty *os.File
	msh.WithLock(func() {
		cmdPty = msh.ControllingPty
	})
	if cmdPty == nil {
		return fmt.Errorf("remote has no attached pty")
	}
	_, err = cmdPty.Write(data)
	if err != nil {
		return fmt.Errorf("writing to pty: %v", err)
	}
	msh.resetClientDeadline()
	return nil
}

func (msh *MShellProc) getClientDeadline() *time.Time {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	return msh.MakeClientDeadline
}

func (msh *MShellProc) resetClientDeadline() {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	if msh.Status != StatusConnecting {
		return
	}
	deadline := msh.MakeClientDeadline
	if deadline == nil {
		return
	}
	newDeadline := time.Now().Add(RemoteConnectTimeout)
	msh.MakeClientDeadline = &newDeadline
}

func (msh *MShellProc) watchClientDeadlineTime() {
	for {
		time.Sleep(1 * time.Second)
		status := msh.GetStatus()
		if status != StatusConnecting {
			break
		}
		deadline := msh.getClientDeadline()
		if deadline == nil {
			break
		}
		if time.Now().After(*deadline) {
			msh.Disconnect(false)
			break
		}
		go msh.NotifyRemoteUpdate()
	}
}

func convertSSHOpts(opts *sstore.SSHOpts) shexec.SSHOpts {
	if opts == nil || opts.Local {
		opts = &sstore.SSHOpts{}
	}
	return shexec.SSHOpts{
		SSHHost:     opts.SSHHost,
		SSHOptsStr:  opts.SSHOptsStr,
		SSHIdentity: opts.SSHIdentity,
		SSHUser:     opts.SSHUser,
		SSHPort:     opts.SSHPort,
	}
}

func (msh *MShellProc) addControllingTty(ecmd *exec.Cmd) (*os.File, error) {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()

	cmdPty, cmdTty, err := pty.Open()
	if err != nil {
		return nil, err
	}
	pty.Setsize(cmdPty, &pty.Winsize{Rows: RemoteTermRows, Cols: RemoteTermCols})
	msh.ControllingPty = cmdPty
	ecmd.ExtraFiles = append(ecmd.ExtraFiles, cmdTty)
	if ecmd.SysProcAttr == nil {
		ecmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	ecmd.SysProcAttr.Setsid = true
	ecmd.SysProcAttr.Setctty = true
	ecmd.SysProcAttr.Ctty = len(ecmd.ExtraFiles) + 3 - 1
	return cmdPty, nil
}

func (msh *MShellProc) setErrorStatus(err error) {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	msh.Status = StatusError
	msh.Err = err
	go msh.NotifyRemoteUpdate()
}

func (msh *MShellProc) setInstallErrorStatus(err error) {
	msh.WriteToPtyBuffer("*error, %s\n", err.Error())
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	msh.InstallStatus = StatusError
	msh.InstallErr = err
	go msh.NotifyRemoteUpdate()
}

func (msh *MShellProc) GetRemoteCopy() sstore.RemoteType {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	return *msh.Remote
}

func (msh *MShellProc) GetUName() string {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	return msh.UName
}

func (msh *MShellProc) GetNumRunningCommands() int {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	return len(msh.RunningCmds)
}

func (msh *MShellProc) UpdateRemote(ctx context.Context, editMap map[string]interface{}) error {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	updatedRemote, err := sstore.UpdateRemote(ctx, msh.Remote.RemoteId, editMap)
	if err != nil {
		return err
	}
	if updatedRemote == nil {
		return fmt.Errorf("no remote returned from UpdateRemote")
	}
	msh.Remote = updatedRemote
	go msh.NotifyRemoteUpdate()
	return nil
}

func (msh *MShellProc) Disconnect(force bool) {
	status := msh.GetStatus()
	if status != StatusConnected && status != StatusConnecting {
		msh.WriteToPtyBuffer("remote already disconnected (no action taken)\n")
		return
	}
	numCommands := msh.GetNumRunningCommands()
	if numCommands > 0 && !force {
		msh.WriteToPtyBuffer("remote not disconnected, has %d running commands.  use force=1 to force disconnection\n", numCommands)
		return
	}
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	if msh.ServerProc != nil {
		msh.ServerProc.Close()
		msh.Client = nil
	}
	if msh.MakeClientCancelFn != nil {
		msh.MakeClientCancelFn()
		msh.MakeClientCancelFn = nil
	}
}

func (msh *MShellProc) CancelInstall() {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	if msh.InstallCancelFn != nil {
		msh.InstallCancelFn()
		msh.InstallCancelFn = nil
	}
}

func (msh *MShellProc) GetRemoteName() string {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	return msh.Remote.GetName()
}

func (msh *MShellProc) WriteToPtyBuffer(strFmt string, args ...interface{}) {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	msh.writeToPtyBuffer_nolock(strFmt, args...)
}

func (msh *MShellProc) writeToPtyBuffer_nolock(strFmt string, args ...interface{}) {
	// inefficient string manipulation here and read of PtyBuffer, but these messages are rare, nbd
	realStr := fmt.Sprintf(strFmt, args...)
	if !strings.HasPrefix(realStr, "~") {
		realStr = strings.ReplaceAll(realStr, "\n", "\r\n")
		if !strings.HasSuffix(realStr, "\r\n") {
			realStr = realStr + "\r\n"
		}
		if strings.HasPrefix(realStr, "*") {
			realStr = "\033[0m\033[31mwave>\033[0m " + realStr[1:]
		} else {
			realStr = "\033[0m\033[32mwave>\033[0m " + realStr
		}
		barr := msh.PtyBuffer.Bytes()
		if len(barr) > 0 && barr[len(barr)-1] != '\n' {
			realStr = "\r\n" + realStr
		}
	} else {
		realStr = realStr[1:]
	}
	curOffset := msh.PtyBuffer.TotalWritten()
	data := []byte(realStr)
	msh.PtyBuffer.Write(data)
	sendRemotePtyUpdate(msh.Remote.RemoteId, curOffset, data)
}

func sendRemotePtyUpdate(remoteId string, dataOffset int64, data []byte) {
	data64 := base64.StdEncoding.EncodeToString(data)
	update := scbus.MakePtyDataUpdate(&scbus.PtyDataUpdate{
		RemoteId:   remoteId,
		PtyPos:     dataOffset,
		PtyData64:  data64,
		PtyDataLen: int64(len(data)),
	})
	scbus.MainUpdateBus.DoUpdate(update)
}

func (msh *MShellProc) isWaitingForPassword_nolock() bool {
	barr := msh.PtyBuffer.Bytes()
	if len(barr) == 0 {
		return false
	}
	nlIdx := bytes.LastIndex(barr, []byte{'\n'})
	var lastLine string
	if nlIdx == -1 {
		lastLine = string(barr)
	} else {
		lastLine = string(barr[nlIdx+1:])
	}
	pwIdx := strings.Index(lastLine, "assword")
	return pwIdx != -1
}

func (msh *MShellProc) isWaitingForPassphrase_nolock() bool {
	barr := msh.PtyBuffer.Bytes()
	if len(barr) == 0 {
		return false
	}
	nlIdx := bytes.LastIndex(barr, []byte{'\n'})
	var lastLine string
	if nlIdx == -1 {
		lastLine = string(barr)
	} else {
		lastLine = string(barr[nlIdx+1:])
	}
	pwIdx := strings.Index(lastLine, "Enter passphrase for key")
	return pwIdx != -1
}

func (msh *MShellProc) RunPasswordReadLoop(cmdPty *os.File) {
	buf := make([]byte, PtyReadBufSize)
	for {
		_, readErr := cmdPty.Read(buf)
		if readErr == io.EOF {
			return
		}
		if readErr != nil {
			msh.WriteToPtyBuffer("*error reading from controlling-pty: %v\n", readErr)
			return
		}
		var newIsWaiting bool
		msh.WithLock(func() {
			newIsWaiting = msh.isWaitingForPassword_nolock()
		})
		if newIsWaiting {
			break
		}
	}
	request := &userinput.UserInputRequestType{
		QueryText:    "Please enter your password",
		ResponseType: "text",
		Title:        "Sudo Password",
		Markdown:     false,
	}
	ctx, cancelFn := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelFn()
	response, err := userinput.GetUserInput(ctx, scbus.MainRpcBus, request)
	if err != nil {
		msh.WriteToPtyBuffer("*error timed out waiting for password: %v\n", err)
		return
	}
	msh.WithLock(func() {
		curOffset := msh.PtyBuffer.TotalWritten()
		msh.PtyBuffer.Write([]byte(response.Text))
		sendRemotePtyUpdate(msh.Remote.RemoteId, curOffset, []byte(response.Text))
	})
}

func (msh *MShellProc) RunPtyReadLoop(cmdPty *os.File) {
	buf := make([]byte, PtyReadBufSize)
	var isWaiting bool
	for {
		n, readErr := cmdPty.Read(buf)
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			msh.WriteToPtyBuffer("*error reading from controlling-pty: %v\n", readErr)
			break
		}
		var newIsWaiting bool
		msh.WithLock(func() {
			curOffset := msh.PtyBuffer.TotalWritten()
			msh.PtyBuffer.Write(buf[0:n])
			sendRemotePtyUpdate(msh.Remote.RemoteId, curOffset, buf[0:n])
			newIsWaiting = msh.isWaitingForPassword_nolock()
		})
		if newIsWaiting != isWaiting {
			isWaiting = newIsWaiting
			go msh.NotifyRemoteUpdate()
		}
	}
}

func (msh *MShellProc) CheckPasswordRequested(ctx context.Context, requiresPassword chan bool) {
	for {
		msh.WithLock(func() {
			if msh.isWaitingForPassword_nolock() {
				select {
				case requiresPassword <- true:
				default:
				}
				return
			}
			if msh.Status != StatusConnecting {
				select {
				case requiresPassword <- false:
				default:
				}
				return
			}
		})
		select {
		case <-ctx.Done():
			return
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (msh *MShellProc) SendPassword(pw string) {
	msh.WithLock(func() {
		if msh.ControllingPty == nil {
			return
		}
		pwBytes := []byte(pw + "\r")
		msh.writeToPtyBuffer_nolock("~[sent password]\r\n")
		_, err := msh.ControllingPty.Write(pwBytes)
		if err != nil {
			msh.writeToPtyBuffer_nolock("*cannot write password to controlling pty: %v\n", err)
		}
	})
}

func (msh *MShellProc) WaitAndSendPasswordNew(pw string) {
	requiresPassword := make(chan bool, 1)
	ctx, cancelFn := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelFn()
	go msh.CheckPasswordRequested(ctx, requiresPassword)
	select {
	case <-ctx.Done():
		err := ctx.Err()
		var errMsg error
		if err == context.Canceled {
			errMsg = fmt.Errorf("canceled by the user")
		} else {
			errMsg = fmt.Errorf("timed out waiting for password prompt")
		}
		msh.WriteToPtyBuffer("*error, %s\n", errMsg.Error())
		msh.setErrorStatus(errMsg)
		return
	case required := <-requiresPassword:
		if !required {
			// we don't need user input in this case, so we exit early
			return
		}
	}

	request := &userinput.UserInputRequestType{
		QueryText:    "Please enter your password",
		ResponseType: "text",
		Title:        "Sudo Password",
		Markdown:     false,
	}
	response, err := userinput.GetUserInput(ctx, scbus.MainRpcBus, request)
	if err != nil {
		var errMsg error
		if err == context.Canceled {
			errMsg = fmt.Errorf("canceled by the user")
		} else {
			errMsg = fmt.Errorf("timed out waiting for user input")
		}
		msh.WriteToPtyBuffer("*error, %s\n", errMsg.Error())
		msh.setErrorStatus(errMsg)
		return
	}
	msh.SendPassword(response.Text)

	//error out if requested again
	go msh.CheckPasswordRequested(ctx, requiresPassword)
	select {
	case <-ctx.Done():
		err := ctx.Err()
		var errMsg error
		if err == context.Canceled {
			errMsg = fmt.Errorf("canceled by the user")
		} else {
			errMsg = fmt.Errorf("timed out waiting for password prompt")
		}
		msh.WriteToPtyBuffer("*error, %s\n", errMsg.Error())
		msh.setErrorStatus(errMsg)
		return
	case required := <-requiresPassword:
		if !required {
			// we don't need user input in this case, so we exit early
			return
		}
	}
	errMsg := fmt.Errorf("*error, incorrect password")
	msh.WriteToPtyBuffer("*error, %s\n", errMsg.Error())
	msh.setErrorStatus(errMsg)
}

func (msh *MShellProc) WaitAndSendPassword(pw string) {
	var numWaits int
	for {
		var isWaiting bool
		var isConnecting bool
		msh.WithLock(func() {
			if msh.Remote.SSHOpts.GetAuthType() == sstore.RemoteAuthTypeKeyPassword {
				isWaiting = msh.isWaitingForPassphrase_nolock()
			} else {
				isWaiting = msh.isWaitingForPassword_nolock()
			}
			isConnecting = msh.Status == StatusConnecting
		})
		if !isConnecting {
			break
		}
		if !isWaiting {
			numWaits = 0
			time.Sleep(100 * time.Millisecond)
			continue
		}
		numWaits++
		if numWaits < 10 {
			time.Sleep(100 * time.Millisecond)
		} else {
			// send password
			msh.WithLock(func() {
				if msh.ControllingPty == nil {
					return
				}
				pwBytes := []byte(pw + "\r")
				msh.writeToPtyBuffer_nolock("~[sent password]\r\n")
				_, err := msh.ControllingPty.Write(pwBytes)
				if err != nil {
					msh.writeToPtyBuffer_nolock("*cannot write password to controlling pty: %v\n", err)
				}
			})
			break
		}
	}
}

func (msh *MShellProc) RunInstall(autoInstall bool) {
	defer func() {
		if r := recover(); r != nil {
			errMsg := fmt.Errorf("this should not happen. if it does, please reach out to us in our discord or open an issue on our github\n\n"+
				"error:\n%v\n\nstack trace:\n%s", r, string(debug.Stack()))
			log.Printf("fatal error, %s\n", errMsg)
			msh.WriteToPtyBuffer("*fatal error, %s\n", errMsg)
			msh.setErrorStatus(errMsg)
		}
	}()
	remoteCopy := msh.GetRemoteCopy()
	if remoteCopy.Archived {
		msh.WriteToPtyBuffer("*error: cannot install on archived remote\n")
		return
	}

	var makeClientCtx context.Context
	var makeClientCancelFn context.CancelFunc
	msh.WithLock(func() {
		makeClientCtx, makeClientCancelFn = context.WithCancel(context.Background())
		msh.MakeClientCancelFn = makeClientCancelFn
		msh.MakeClientDeadline = nil
		go msh.NotifyRemoteUpdate()
	})
	defer makeClientCancelFn()
	clientData, err := sstore.EnsureClientData(makeClientCtx)
	if err != nil {
		msh.WriteToPtyBuffer("*error: cannot obtain client data: %v", err)
		return
	}
	hideShellPrompt := clientData.ClientOpts.ConfirmFlags["hideshellprompt"]
	baseStatus := msh.GetStatus()

	if baseStatus == StatusConnected {
		ctx, cancelFn := context.WithTimeout(makeClientCtx, 60*time.Second)
		defer cancelFn()
		request := &userinput.UserInputRequestType{
			ResponseType: "confirm",
			QueryText:    "Waveshell is running on your connection and must be restarted to re-install. Would you like to continue?",
			Title:        "Restart Waveshell",
		}
		response, err := userinput.GetUserInput(ctx, scbus.MainRpcBus, request)
		if err != nil {
			if err == context.Canceled {
				msh.WriteToPtyBuffer("installation canceled by user\n")
			} else {
				msh.WriteToPtyBuffer("timed out waiting for user input\n")
			}
			return
		}
		if !response.Confirm {
			msh.WriteToPtyBuffer("installation canceled by user\n")
			return
		}
	} else if !hideShellPrompt {
		ctx, cancelFn := context.WithTimeout(makeClientCtx, 60*time.Second)
		defer cancelFn()
		request := &userinput.UserInputRequestType{
			ResponseType: "confirm",
			QueryText:    "Waveshell must be reinstalled on the connection to continue. Would you like to install it?",
			Title:        "Install Waveshell",
			CheckBoxMsg:  "Don't show me this again",
		}
		response, err := userinput.GetUserInput(ctx, scbus.MainRpcBus, request)
		if err != nil {
			var errMsg error
			if err == context.Canceled {
				errMsg = fmt.Errorf("installation canceled by user")
			} else {
				errMsg = fmt.Errorf("timed out waiting for user input")
			}
			msh.WithLock(func() {
				msh.Client = nil
			})
			msh.WriteToPtyBuffer("*error, %s\n", errMsg)
			msh.setErrorStatus(errMsg)
			return
		}
		if !response.Confirm {
			errMsg := fmt.Errorf("installation canceled by user")
			msh.WriteToPtyBuffer("*error, %s\n", errMsg.Error())
			msh.setErrorStatus(err)
			msh.WithLock(func() {
				msh.Client = nil
			})
			return
		}
		if response.CheckboxStat {
			clientData.ClientOpts.ConfirmFlags["hideshellprompt"] = true
			err = sstore.SetClientOpts(makeClientCtx, clientData.ClientOpts)
			if err != nil {
				msh.WriteToPtyBuffer("*error, %s\n", err)
				msh.setErrorStatus(err)
				return
			}

			//reload updated clientdata before sending
			clientData, err = sstore.EnsureClientData(makeClientCtx)
			if err != nil {
				msh.WriteToPtyBuffer("*error, %s\n", err)
				msh.setErrorStatus(err)
				return
			}
			update := scbus.MakeUpdatePacket()
			update.AddUpdate(*clientData)
		}
	}
	curStatus := msh.GetInstallStatus()
	if curStatus == StatusConnecting {
		msh.WriteToPtyBuffer("*error: cannot install on remote that is already trying to install, cancel current install to try again\n")
		return
	}
	if remoteCopy.Local {
		msh.WriteToPtyBuffer("*error: cannot install on a local remote\n")
		return
	}
	_, err = shellapi.MakeShellApi(packet.ShellType_bash)
	if err != nil {
		msh.WriteToPtyBuffer("*error: %v\n", err)
		return
	}
	if msh.Client == nil {
		remoteDisplayName := fmt.Sprintf("%s [%s]", remoteCopy.RemoteAlias, remoteCopy.RemoteCanonicalName)
		client, err := ConnectToClient(makeClientCtx, remoteCopy.SSHOpts, remoteDisplayName)
		if err != nil {
			statusErr := fmt.Errorf("ssh cannot connect to client: %w", err)
			msh.setInstallErrorStatus(statusErr)
			return
		}
		msh.WithLock(func() {
			msh.Client = client
		})
	}
	session, err := msh.Client.NewSession()
	if err != nil {
		statusErr := fmt.Errorf("ssh cannot connect to client: %w", err)
		msh.setInstallErrorStatus(statusErr)
		return
	}
	installSession := shexec.SessionWrap{Session: session, StartCmd: shexec.MakeInstallCommandStr()}
	msh.WriteToPtyBuffer("installing waveshell %s to %s...\n", scbase.MShellVersion, remoteCopy.RemoteCanonicalName)
	clientCtx, clientCancelFn := context.WithCancel(context.Background())
	defer clientCancelFn()
	msh.WithLock(func() {
		msh.InstallErr = nil
		msh.InstallStatus = StatusConnecting
		msh.InstallCancelFn = clientCancelFn
		go msh.NotifyRemoteUpdate()
	})
	msgFn := func(msg string) {
		msh.WriteToPtyBuffer("%s", msg)
	}
	err = shexec.RunInstallFromCmd(clientCtx, installSession, true, nil, scbase.MShellBinaryReader, msgFn)
	if err == context.Canceled {
		msh.WriteToPtyBuffer("*install canceled\n")
		msh.WithLock(func() {
			msh.InstallStatus = StatusDisconnected
			go msh.NotifyRemoteUpdate()
		})
		return
	}
	if err != nil {
		statusErr := fmt.Errorf("install failed: %w", err)
		msh.setInstallErrorStatus(statusErr)
		return
	}
	var connectMode string
	msh.WithLock(func() {
		msh.InstallStatus = StatusDisconnected
		msh.InstallCancelFn = nil
		msh.NeedsMShellUpgrade = false
		msh.Status = StatusDisconnected
		msh.Err = nil
		connectMode = msh.Remote.ConnectMode
	})
	msh.WriteToPtyBuffer("successfully installed waveshell %s to ~/.mshell\n", scbase.MShellVersion)
	go msh.NotifyRemoteUpdate()
	if connectMode == sstore.ConnectModeStartup || connectMode == sstore.ConnectModeAuto || autoInstall {
		// the install was successful, and we didn't click the install button with manual connect mode, try to connect
		go msh.Launch(true)
	}
}

func (msh *MShellProc) updateRemoteStateVars(ctx context.Context, remoteId string, initPk *packet.InitPacketType) {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	stateVars := getStateVarsFromInitPk(initPk)
	if stateVars == nil {
		return
	}
	msh.Remote.StateVars = stateVars
	err := sstore.UpdateRemoteStateVars(ctx, remoteId, stateVars)
	if err != nil {
		// ignore error, nothing to do
		log.Printf("error updating remote statevars: %v\n", err)
	}
}

func getStateVarsFromInitPk(initPk *packet.InitPacketType) map[string]string {
	if initPk == nil || initPk.NotFound {
		return nil
	}
	rtn := make(map[string]string)
	rtn["home"] = initPk.HomeDir
	rtn["remoteuser"] = initPk.User
	rtn["remotehost"] = initPk.HostName
	rtn["remoteuname"] = initPk.UName
	rtn["shelltype"] = initPk.Shell
	return rtn
}

func makeReinitErrorUpdate(shellType string) telemetry.ActivityUpdate {
	rtn := telemetry.ActivityUpdate{}
	if shellType == packet.ShellType_bash {
		rtn.ReinitBashErrors = 1
	} else if shellType == packet.ShellType_zsh {
		rtn.ReinitZshErrors = 1
	}
	return rtn
}

func (msh *MShellProc) ReInit(ctx context.Context, ck base.CommandKey, shellType string, dataFn func([]byte), verbose bool) (rtnPk *packet.ShellStatePacketType, rtnErr error) {
	if !msh.IsConnected() {
		return nil, fmt.Errorf("cannot reinit, remote is not connected")
	}
	if shellType != packet.ShellType_bash && shellType != packet.ShellType_zsh {
		return nil, fmt.Errorf("invalid shell type %q", shellType)
	}
	if dataFn == nil {
		dataFn = func([]byte) {}
	}
	defer func() {
		if rtnErr != nil {
			telemetry.UpdateActivityWrap(ctx, makeReinitErrorUpdate(shellType), "reiniterror")
		}
	}()
	startTs := time.Now()
	reinitPk := packet.MakeReInitPacket()
	reinitPk.ReqId = uuid.New().String()
	reinitPk.ShellType = shellType
	rpcIter, err := msh.PacketRpcIter(ctx, reinitPk)
	if err != nil {
		return nil, err
	}
	defer rpcIter.Close()
	if ck != "" {
		reinitSink := &ReinitCommandSink{
			Remote: msh,
			ReqId:  reinitPk.ReqId,
		}
		msh.registerInputSink(ck, reinitSink)
		defer msh.unregisterInputSink(ck)
	}
	var ssPk *packet.ShellStatePacketType
	for {
		resp, err := rpcIter.Next(ctx)
		if err != nil {
			return nil, err
		}
		if resp == nil {
			return nil, fmt.Errorf("channel closed with no response")
		}
		var ok bool
		ssPk, ok = resp.(*packet.ShellStatePacketType)
		if ok {
			break
		}
		respPk, ok := resp.(*packet.ResponsePacketType)
		if ok {
			if respPk.Error != "" {
				return nil, fmt.Errorf("error reinitializing remote: %s", respPk.Error)
			}
			return nil, fmt.Errorf("invalid response from waveshell")
		}
		dataPk, ok := resp.(*packet.FileDataPacketType)
		if ok {
			dataFn(dataPk.Data)
			continue
		}
		invalidPkStr := fmt.Sprintf("\r\ninvalid packettype from waveshell: %s\r\n", resp.GetType())
		dataFn([]byte(invalidPkStr))
	}
	if ssPk == nil || ssPk.State == nil {
		return nil, fmt.Errorf("invalid reinit response shellstate packet does not contain remote state")
	}
	// TODO: maybe we don't need to save statebase here.  should be possible to save it on demand
	//    when it is actually used.  complication from other functions that try to get the statebase
	//    from the DB.  probably need to route those through MShellProc.
	err = sstore.StoreStateBase(ctx, ssPk.State)
	if err != nil {
		return nil, fmt.Errorf("error storing remote state: %w", err)
	}
	msh.StateMap.SetCurrentState(ssPk.State.GetShellType(), ssPk.State)
	timeDur := time.Since(startTs)
	dataFn([]byte(makeShellInitOutputMsg(verbose, ssPk.State, ssPk.Stats, timeDur, false)))
	msh.WriteToPtyBuffer("%s", makeShellInitOutputMsg(false, ssPk.State, ssPk.Stats, timeDur, true))
	return ssPk, nil
}

func makeShellInitOutputMsg(verbose bool, state *packet.ShellState, stats *packet.ShellStateStats, dur time.Duration, ptyMsg bool) string {
	waveStr := fmt.Sprintf("%swave>%s", utilfn.AnsiGreenColor(), utilfn.AnsiResetColor())
	if !verbose || ptyMsg {
		if ptyMsg {
			return fmt.Sprintf("initialized state shell:%s statehash:%s %dms\n", state.GetShellType(), state.GetHashVal(false), dur.Milliseconds())
		} else {
			return fmt.Sprintf("%s initialized connection state (shell:%s)\r\n", waveStr, state.GetShellType())
		}
	}
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("%s initialized connection shell:%s statehash:%s %dms\r\n", waveStr, state.GetShellType(), state.GetHashVal(false), dur.Milliseconds()))
	if stats != nil {
		buf.WriteString(fmt.Sprintf("%s   outsize:%s size:%s env:%d, vars:%d, aliases:%d, funcs:%d\r\n", waveStr, scbase.NumFormatDec(stats.OutputSize), scbase.NumFormatDec(stats.StateSize), stats.EnvCount, stats.VarCount, stats.AliasCount, stats.FuncCount))
	}
	return buf.String()
}

func (msh *MShellProc) WriteFile(ctx context.Context, writePk *packet.WriteFilePacketType) (*packet.RpcResponseIter, error) {
	return msh.PacketRpcIter(ctx, writePk)
}

func (msh *MShellProc) StreamFile(ctx context.Context, streamPk *packet.StreamFilePacketType) (*packet.RpcResponseIter, error) {
	return msh.PacketRpcIter(ctx, streamPk)
}

func addScVarsToState(state *packet.ShellState) *packet.ShellState {
	if state == nil {
		return nil
	}
	rtn := *state
	envMap := shellenv.DeclMapFromState(&rtn)
	envMap["WAVETERM"] = &shellenv.DeclareDeclType{Name: "WAVETERM", Value: "1", Args: "x"}
	envMap["WAVETERM_VERSION"] = &shellenv.DeclareDeclType{Name: "WAVETERM_VERSION", Value: scbase.WaveVersion, Args: "x"}
	envMap["TERM_PROGRAM"] = &shellenv.DeclareDeclType{Name: "TERM_PROGRAM", Value: "waveterm", Args: "x"}
	envMap["TERM_PROGRAM_VERSION"] = &shellenv.DeclareDeclType{Name: "TERM_PROGRAM_VERSION", Value: scbase.WaveVersion, Args: "x"}
	if scbase.IsDevMode() {
		envMap["WAVETERM_DEV"] = &shellenv.DeclareDeclType{Name: "WAVETERM_DEV", Value: "1", Args: "x"}
	}
	if _, exists := envMap["LANG"]; !exists {
		envMap["LANG"] = &shellenv.DeclareDeclType{Name: "LANG", Value: scbase.DetermineLang(), Args: "x"}
	}
	rtn.ShellVars = shellenv.SerializeDeclMap(envMap)
	return &rtn
}

func stripScVarsFromState(state *packet.ShellState) *packet.ShellState {
	if state == nil {
		return nil
	}
	rtn := *state
	rtn.HashVal = ""
	envMap := shellenv.DeclMapFromState(&rtn)
	for key := range envVarsToStrip {
		delete(envMap, key)
	}
	rtn.ShellVars = shellenv.SerializeDeclMap(envMap)
	return &rtn
}

func stripScVarsFromStateDiff(stateDiff *packet.ShellStateDiff) *packet.ShellStateDiff {
	if stateDiff == nil || len(stateDiff.VarsDiff) == 0 {
		return stateDiff
	}
	rtn := *stateDiff
	rtn.HashVal = ""
	var mapDiff statediff.MapDiffType
	err := mapDiff.Decode(stateDiff.VarsDiff)
	if err != nil {
		log.Printf("error decoding statediff in stripScVarsFromStateDiff: %v\n", err)
		return stateDiff
	}
	for key := range envVarsToStrip {
		delete(mapDiff.ToAdd, key)
	}
	rtn.VarsDiff = mapDiff.Encode()
	return &rtn
}

func (msh *MShellProc) getActiveShellTypes(ctx context.Context) ([]string, error) {
	shellPref := msh.GetShellPref()
	rtn := []string{shellPref}
	activeShells, err := sstore.GetRemoteActiveShells(ctx, msh.RemoteId)
	if err != nil {
		return nil, err
	}
	return utilfn.CombineStrArrays(rtn, activeShells), nil
}

func (msh *MShellProc) createWaveshellSession(clientCtx context.Context, remoteCopy sstore.RemoteType) (shexec.ConnInterface, error) {
	msh.WithLock(func() {
		msh.Err = nil
		msh.ErrNoInitPk = false
		msh.Status = StatusConnecting
		msh.MakeClientDeadline = nil
		go msh.NotifyRemoteUpdate()
	})
	sapi, err := shellapi.MakeShellApi(msh.GetShellType())
	if err != nil {
		return nil, err
	}
	var wsSession shexec.ConnInterface
	if remoteCopy.SSHOpts.SSHHost == "" && remoteCopy.Local {
		cmdStr, err := MakeLocalMShellCommandStr(remoteCopy.IsSudo())
		if err != nil {
			return nil, fmt.Errorf("cannot find local waveshell binary: %v", err)
		}
		ecmd := shexec.MakeLocalExecCmd(cmdStr, sapi)
		var cmdPty *os.File
		cmdPty, err = msh.addControllingTty(ecmd)
		if err != nil {
			return nil, fmt.Errorf("cannot attach controlling tty to waveshell command: %v", err)
		}
		go msh.RunPtyReadLoop(cmdPty)
		go msh.WaitAndSendPasswordNew(remoteCopy.SSHOpts.SSHPassword)
		wsSession = shexec.CmdWrap{Cmd: ecmd}
	} else if msh.Client == nil {
		remoteDisplayName := fmt.Sprintf("%s [%s]", remoteCopy.RemoteAlias, remoteCopy.RemoteCanonicalName)
		client, err := ConnectToClient(clientCtx, remoteCopy.SSHOpts, remoteDisplayName)
		if err != nil {
			return nil, fmt.Errorf("ssh cannot connect to client: %w", err)
		}
		msh.WithLock(func() {
			msh.Client = client
		})
		session, err := client.NewSession()
		if err != nil {
			return nil, fmt.Errorf("ssh cannot create session: %w", err)
		}
		cmd := fmt.Sprintf("%s -c %s", sapi.GetLocalShellPath(), shellescape.Quote(MakeServerCommandStr()))
		wsSession = shexec.SessionWrap{Session: session, StartCmd: cmd}
	} else {
		session, err := msh.Client.NewSession()
		if err != nil {
			return nil, fmt.Errorf("ssh cannot create session: %w", err)
		}
		cmd := fmt.Sprintf(`%s -c %s`, sapi.GetLocalShellPath(), shellescape.Quote(MakeServerCommandStr()))
		wsSession = shexec.SessionWrap{Session: session, StartCmd: cmd}
	}
	return wsSession, nil
}

func (msh *MShellProc) Launch(interactive bool) {
	defer func() {
		if r := recover(); r != nil {
			errMsg := fmt.Errorf("this should not happen. if it does, please reach out to us in our discord or open an issue on our github\n\n"+
				"error:\n%v\n\nstack trace:\n%s", r, string(debug.Stack()))
			log.Printf("fatal error, %s\n", errMsg)
			msh.WriteToPtyBuffer("*fatal error, %s\n", errMsg)
			msh.setErrorStatus(errMsg)
		}
	}()
	remoteCopy := msh.GetRemoteCopy()
	if remoteCopy.Archived {
		msh.WriteToPtyBuffer("cannot launch archived remote\n")
		return
	}
	curStatus := msh.GetStatus()
	if curStatus == StatusConnected {
		msh.WriteToPtyBuffer("remote is already connected (no action taken)\n")
		return
	}
	if curStatus == StatusConnecting {
		msh.WriteToPtyBuffer("remote is already connecting, disconnect before trying to connect again\n")
		return
	}
	istatus := msh.GetInstallStatus()
	if istatus == StatusConnecting {
		msh.WriteToPtyBuffer("remote is trying to install, cancel install before trying to connect again\n")
		return
	}
	var makeClientCtx context.Context
	var makeClientCancelFn context.CancelFunc
	msh.WithLock(func() {
		makeClientCtx, makeClientCancelFn = context.WithCancel(context.Background())
		msh.MakeClientCancelFn = makeClientCancelFn
		msh.MakeClientDeadline = nil
		go msh.NotifyRemoteUpdate()
	})
	defer makeClientCancelFn()
	msh.WriteToPtyBuffer("connecting to %s...\n", remoteCopy.RemoteCanonicalName)
	wsSession, err := msh.createWaveshellSession(makeClientCtx, remoteCopy)
	if err != nil {
		msh.WriteToPtyBuffer("*error, %s\n", err.Error())
		msh.setErrorStatus(err)
		msh.WithLock(func() {
			msh.Client = nil
		})
		return
	}
	cproc, err := shexec.MakeClientProc(makeClientCtx, wsSession)
	msh.WithLock(func() {
		msh.MakeClientCancelFn = nil
		msh.MakeClientDeadline = nil
	})
	if err == context.DeadlineExceeded {
		msh.WriteToPtyBuffer("*connect timeout\n")
		msh.setErrorStatus(errors.New("connect timeout"))
		msh.WithLock(func() {
			msh.Client = nil
		})
		return
	} else if err == context.Canceled {
		msh.WriteToPtyBuffer("*forced disconnection\n")
		msh.WithLock(func() {
			msh.Status = StatusDisconnected
			go msh.NotifyRemoteUpdate()
		})
		msh.WithLock(func() {
			msh.Client = nil
		})
		return
	} else if serr, ok := err.(shexec.WaveshellLaunchError); ok {
		msh.WithLock(func() {
			msh.UName = serr.InitPk.UName
			msh.NeedsMShellUpgrade = true
			msh.InitPkShellType = serr.InitPk.Shell
		})
		msh.StateMap.Clear()
		msh.WriteToPtyBuffer("*error, %s\n", serr.Error())
		msh.setErrorStatus(serr)
		go msh.tryAutoInstall()
		return
	} else if err != nil {
		msh.WriteToPtyBuffer("*error, %s\n", err.Error())
		msh.setErrorStatus(err)
		msh.WithLock(func() {
			msh.Client = nil
		})
		return
	}
	msh.WithLock(func() {
		msh.UName = cproc.InitPk.UName
		msh.InitPkShellType = cproc.InitPk.Shell
		msh.StateMap.Clear()
		// no notify here, because we'll call notify in either case below
	})

	msh.updateRemoteStateVars(context.Background(), msh.RemoteId, cproc.InitPk)
	msh.WithLock(func() {
		msh.ServerProc = cproc
		msh.Status = StatusConnected
	})
	msh.WriteToPtyBuffer("connected to %s\n", remoteCopy.RemoteCanonicalName)
	go func() {
		exitErr := cproc.Cmd.Wait()
		exitCode := shexec.GetExitCode(exitErr)
		msh.WithLock(func() {
			if msh.Status == StatusConnected || msh.Status == StatusConnecting {
				msh.Status = StatusDisconnected
				go msh.NotifyRemoteUpdate()
			}
		})
		msh.WriteToPtyBuffer("*disconnected exitcode=%d\n", exitCode)
	}()
	go msh.ProcessPackets()
	// msh.initActiveShells()
	go msh.NotifyRemoteUpdate()
}

func (msh *MShellProc) initActiveShells() {
	gasCtx, cancelFn := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFn()
	activeShells, err := msh.getActiveShellTypes(gasCtx)
	if err != nil {
		// we're not going to fail the connect for this error (it will be unusable, but technically connected)
		msh.WriteToPtyBuffer("*error getting active shells: %v\n", err)
		return
	}
	var wg sync.WaitGroup
	for _, shellTypeForVar := range activeShells {
		wg.Add(1)
		go func(shellType string) {
			defer wg.Done()
			reinitCtx, cancelFn := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancelFn()
			_, err = msh.ReInit(reinitCtx, base.CommandKey(""), shellType, nil, false)
			if err != nil {
				msh.WriteToPtyBuffer("*error reiniting shell %q: %v\n", shellType, err)
			}
		}(shellTypeForVar)
	}
	wg.Wait()
}

func (msh *MShellProc) IsConnected() bool {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	return msh.Status == StatusConnected
}

func (msh *MShellProc) GetShellType() string {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	return msh.InitPkShellType
}

func replaceHomePath(pathStr string, homeDir string) string {
	if homeDir == "" {
		return pathStr
	}
	if pathStr == homeDir {
		return "~"
	}
	if strings.HasPrefix(pathStr, homeDir+"/") {
		return "~" + pathStr[len(homeDir):]
	}
	return pathStr
}

func (msh *MShellProc) IsCmdRunning(ck base.CommandKey) bool {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	_, ok := msh.RunningCmds[ck]
	return ok
}

func (msh *MShellProc) KillRunningCommandAndWait(ctx context.Context, ck base.CommandKey) error {
	if !msh.IsCmdRunning(ck) {
		return nil
	}
	feiPk := scpacket.MakeFeInputPacket()
	feiPk.CK = ck
	feiPk.SigName = "SIGTERM"
	err := msh.HandleFeInput(feiPk)
	if err != nil {
		return fmt.Errorf("error trying to kill running cmd: %w", err)
	}
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !msh.IsCmdRunning(ck) {
			return nil
		}
		// TODO fix busy wait (sync with msh.RunningCmds)
		// not a huge deal though since this is not processor intensive and not widely used
		time.Sleep(100 * time.Millisecond)
	}
}

func (msh *MShellProc) SendFileData(dataPk *packet.FileDataPacketType) error {
	if !msh.IsConnected() {
		return fmt.Errorf("remote is not connected, cannot send input")
	}
	return msh.ServerProc.Input.SendPacket(dataPk)
}

func makeTermOpts(runPk *packet.RunPacketType) sstore.TermOpts {
	return sstore.TermOpts{Rows: int64(runPk.TermOpts.Rows), Cols: int64(runPk.TermOpts.Cols), FlexRows: runPk.TermOpts.FlexRows, MaxPtySize: DefaultMaxPtySize}
}

// returns (ok, rct)
// if ok is true, rct will be nil
// if ok is false, rct will be the existing pending state command (not nil)
func (msh *MShellProc) testAndSetPendingStateCmd(screenId string, rptr sstore.RemotePtrType, newCK *base.CommandKey) (bool, *RunCmdType) {
	key := pendingStateKey{ScreenId: screenId, RemotePtr: rptr}
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	ck, found := msh.PendingStateCmds[key]
	if found {
		// we don't call GetRunningCmd here because we already hold msh.Lock
		rct := msh.RunningCmds[ck]
		if rct != nil {
			return false, rct
		}
		// ok, so rct is nil (that's strange).  allow command to proceed, but log
		log.Printf("[warning] found pending state cmd with no running cmd: %s\n", ck)
	}
	if newCK != nil {
		msh.PendingStateCmds[key] = *newCK
	}
	return true, nil
}

func (msh *MShellProc) removePendingStateCmd(screenId string, rptr sstore.RemotePtrType, ck base.CommandKey) {
	key := pendingStateKey{ScreenId: screenId, RemotePtr: rptr}
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	existingCK, found := msh.PendingStateCmds[key]
	if !found {
		return
	}
	if existingCK == ck {
		delete(msh.PendingStateCmds, key)
	}
}

type RunCommandOpts struct {
	SessionId string
	ScreenId  string
	RemotePtr sstore.RemotePtrType

	// optional, if not provided shellstate will look up state from remote instance
	// ReturnState cannot be used with StatePtr
	// this will also cause this command to bypass the pending state cmd logic
	StatePtr *packet.ShellStatePtr

	// set to true to skip creating the pty file (for restarted commands)
	NoCreateCmdPtyFile bool

	// this command will not go into the DB, and will not have a ptyout file created
	// forces special packet handling (sets RunCommandType.EphemeralOpts)
	EphemeralOpts *ephemeral.EphemeralRunOpts
}

// returns (CmdType, allow-updates-callback, err)
// we must persist the CmdType to the DB before calling the callback to allow updates
// otherwise an early CmdDone packet might not get processed (since cmd will not exist in DB)
func RunCommand(ctx context.Context, rcOpts RunCommandOpts, runPacket *packet.RunPacketType) (rtnCmd *sstore.CmdType, rtnCallback func(), rtnErr error) {
	sessionId, screenId, remotePtr := rcOpts.SessionId, rcOpts.ScreenId, rcOpts.RemotePtr
	if remotePtr.OwnerId != "" {
		return nil, nil, fmt.Errorf("cannot run command against another user's remote '%s'", remotePtr.MakeFullRemoteRef())
	}
	if screenId != runPacket.CK.GetGroupId() {
		return nil, nil, fmt.Errorf("run commands screenids do not match")
	}
	msh := GetRemoteById(remotePtr.RemoteId)
	if msh == nil {
		return nil, nil, fmt.Errorf("no remote id=%s found", remotePtr.RemoteId)
	}
	if !msh.IsConnected() {
		return nil, nil, fmt.Errorf("remote '%s' is not connected", remotePtr.RemoteId)
	}
	if runPacket.State != nil {
		return nil, nil, fmt.Errorf("runPacket.State should not be set, it is set in RunCommand")
	}
	if rcOpts.StatePtr != nil && runPacket.ReturnState {
		return nil, nil, fmt.Errorf("RunCommand: cannot use ReturnState with StatePtr")
	}
	if runPacket.StatePtr != nil {
		return nil, nil, fmt.Errorf("runPacket.StatePtr should not be set, it is set in RunCommand")
	}

	if rcOpts.EphemeralOpts != nil {
		log.Printf("[info] running ephemeral command ck: %s\n", runPacket.CK)
	}

	// pending state command logic
	// if we are currently running a command that can change the state, we need to wait for it to finish
	if rcOpts.StatePtr == nil {
		var newPSC *base.CommandKey
		if runPacket.ReturnState {
			newPSC = &runPacket.CK
		}
		ok, existingRct := msh.testAndSetPendingStateCmd(screenId, remotePtr, newPSC)
		if !ok {
			if rcOpts.EphemeralOpts != nil {
				// if the existing command is ephemeral, we cancel it and continue
				log.Printf("[warning] canceling existing ephemeral state cmd: %s\n", existingRct.CK)
				rcOpts.EphemeralOpts.Canceled.Store(true)
			} else {
				line, _, err := sstore.GetLineCmdByLineId(ctx, screenId, existingRct.CK.GetCmdId())
				return nil, nil, makePSCLineError(existingRct.CK, line, err)
			}
		}
		if newPSC != nil {
			defer func() {
				// if we get an error, remove the pending state cmd
				// if no error, PSC will get removed when we see a CmdDone or CmdFinal packet
				if rtnErr != nil {
					msh.removePendingStateCmd(screenId, remotePtr, *newPSC)
				}
			}()
		}
	}

	// get current remote-instance state
	var statePtr *packet.ShellStatePtr
	if rcOpts.StatePtr != nil {
		statePtr = rcOpts.StatePtr
	} else {
		var err error
		statePtr, err = sstore.GetRemoteStatePtr(ctx, sessionId, screenId, remotePtr)
		if err != nil {
			log.Printf("[error] RunCommand: cannot get remote state: %v\n", err)
			return nil, nil, fmt.Errorf("cannot run command: %w", err)
		}
		if statePtr == nil {
			log.Printf("[error] RunCommand: no valid shell state found\n")
			return nil, nil, fmt.Errorf("cannot run command: no valid shell state found")
		}
	}
	// statePtr will not be nil
	runPacket.StatePtr = statePtr
	currentState, err := sstore.GetFullState(ctx, *statePtr)

	if rcOpts.EphemeralOpts != nil {
		// Setting UsePty to false will ensure that the outputs get written to the correct file descriptors to extract stdout and stderr
		runPacket.UsePty = rcOpts.EphemeralOpts.UsePty

		// Ephemeral commands can override the cwd without persisting it to the DB
		if rcOpts.EphemeralOpts.OverrideCwd != "" {
			currentState.Cwd = rcOpts.EphemeralOpts.OverrideCwd
		}

		// Ephemeral commands can override the env without persisting it to the DB
		if len(rcOpts.EphemeralOpts.Env) > 0 {
			curEnvs := shellenv.DeclMapFromState(currentState)
			for key, val := range rcOpts.EphemeralOpts.Env {
				curEnvs[key] = &shellenv.DeclareDeclType{Name: key, Value: val, Args: "x"}
			}
			currentState.ShellVars = shellenv.SerializeDeclMap(curEnvs)
		}
	}

	if err != nil || currentState == nil {
		return nil, nil, fmt.Errorf("cannot load current remote state: %w", err)
	}
	runPacket.State = addScVarsToState(currentState)
	runPacket.StateComplete = true
	runPacket.ShellType = currentState.GetShellType()

	// start cmdwait.  must be started before sending the run packet
	// this ensures that we don't process output, or cmddone packets until we set up the line, cmd, and ptyout file
	startCmdWait(runPacket.CK)
	defer func() {
		// if we get an error, remove the cmdwait
		// if no error, cmdwait will get removed by the caller w/ the callback fn that's returned on success
		if rtnErr != nil {
			removeCmdWait(runPacket.CK)
		}
	}()
	runningCmdType := &RunCmdType{
		CK:            runPacket.CK,
		SessionId:     sessionId,
		ScreenId:      screenId,
		RemotePtr:     remotePtr,
		RunPacket:     runPacket,
		EphemeralOpts: rcOpts.EphemeralOpts,
	}
	// RegisterRpc + WaitForResponse is used to get any waveshell side errors
	// waveshell will either return an error (in a ResponsePacketType) or a CmdStartPacketType
	msh.ServerProc.Output.RegisterRpc(runPacket.ReqId)
	go func() {
		startPk, err := msh.sendRunPacketAndReturnResponse(runPacket)
		runCmdUpdateFn(runPacket.CK, func() {
			if err != nil {
				// the cmd failed (never started)
				msh.handleCmdStartError(runningCmdType, err)
				return
			}
			ctx, cancelFn := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelFn()
			err = sstore.UpdateCmdStartInfo(ctx, runPacket.CK, startPk.Pid, startPk.MShellPid)
			if err != nil {
				log.Printf("error updating cmd start info (in remote.RunCommand): %v\n", err)
			}
		})
	}()
	// command is now successfully runnning
	status := sstore.CmdStatusRunning
	if runPacket.Detached {
		status = sstore.CmdStatusDetached
	}
	cmd := &sstore.CmdType{
		ScreenId:   runPacket.CK.GetGroupId(),
		LineId:     runPacket.CK.GetCmdId(),
		CmdStr:     runPacket.Command,
		RawCmdStr:  runPacket.Command,
		Remote:     remotePtr,
		FeState:    sstore.FeStateFromShellState(currentState),
		StatePtr:   *statePtr,
		TermOpts:   makeTermOpts(runPacket),
		Status:     status,
		ExitCode:   0,
		DurationMs: 0,
		RunOut:     nil,
		RtnState:   runPacket.ReturnState,
	}
	if !rcOpts.NoCreateCmdPtyFile && rcOpts.EphemeralOpts == nil {
		err = sstore.CreateCmdPtyFile(ctx, cmd.ScreenId, cmd.LineId, cmd.TermOpts.MaxPtySize)
		if err != nil {
			// TODO the cmd is running, so this is a tricky error to handle
			return nil, nil, fmt.Errorf("cannot create local ptyout file for running command: %v", err)
		}
	}
	msh.AddRunningCmd(runningCmdType)
	return cmd, func() { removeCmdWait(runPacket.CK) }, nil
}

// no context because it is called as a goroutine
func (msh *MShellProc) sendRunPacketAndReturnResponse(runPacket *packet.RunPacketType) (*packet.CmdStartPacketType, error) {
	ctx, cancelFn := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFn()
	err := shexec.SendRunPacketAndRunData(ctx, msh.ServerProc.Input, runPacket)
	if err != nil {
		return nil, fmt.Errorf("sending run packet to remote: %w", err)
	}
	rtnPk := msh.ServerProc.Output.WaitForResponse(ctx, runPacket.ReqId)
	if rtnPk == nil {
		return nil, ctx.Err()
	}
	startPk, ok := rtnPk.(*packet.CmdStartPacketType)
	if !ok {
		respPk, ok := rtnPk.(*packet.ResponsePacketType)
		if !ok {
			return nil, fmt.Errorf("invalid response received from server for run packet: %s", packet.AsString(rtnPk))
		}
		if respPk.Error != "" {
			return nil, respPk.Err()
		}
		return nil, fmt.Errorf("invalid response received from server for run packet: %s", packet.AsString(rtnPk))
	}
	return startPk, nil
}

// helper func to construct the proper error given what information we have
func makePSCLineError(existingPSC base.CommandKey, line *sstore.LineType, lineErr error) error {
	if lineErr != nil {
		return fmt.Errorf("cannot run command while a stateful command is still running: %v", lineErr)
	}
	if line == nil {
		return fmt.Errorf("cannot run command while a stateful command is still running %s", existingPSC)
	}
	return fmt.Errorf("cannot run command while a stateful command (linenum=%d) is still running", line.LineNum)
}

func (msh *MShellProc) registerInputSink(ck base.CommandKey, sink CommandInputSink) {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	msh.CommandInputMap[ck] = sink
}

func (msh *MShellProc) unregisterInputSink(ck base.CommandKey) {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	delete(msh.CommandInputMap, ck)
}

func (msh *MShellProc) HandleFeInput(inputPk *scpacket.FeInputPacketType) error {
	if inputPk == nil {
		return nil
	}
	if !msh.IsConnected() {
		return fmt.Errorf("connection is not connected, cannot send input")
	}
	if msh.IsCmdRunning(inputPk.CK) {
		if len(inputPk.InputData64) > 0 {
			inputLen := packet.B64DecodedLen(inputPk.InputData64)
			if inputLen > MaxInputDataSize {
				return fmt.Errorf("input data size too large, len=%d (max=%d)", inputLen, MaxInputDataSize)
			}
			dataPk := packet.MakeDataPacket()
			dataPk.CK = inputPk.CK
			dataPk.FdNum = 0 // stdin
			dataPk.Data64 = inputPk.InputData64
			err := msh.ServerProc.Input.SendPacket(dataPk)
			if err != nil {
				return err
			}
		}
		if inputPk.SigName != "" || inputPk.WinSize != nil {
			siPk := packet.MakeSpecialInputPacket()
			siPk.CK = inputPk.CK
			siPk.SigName = inputPk.SigName
			siPk.WinSize = inputPk.WinSize
			err := msh.ServerProc.Input.SendPacket(siPk)
			if err != nil {
				return err
			}
		}
		return nil
	}
	msh.Lock.Lock()
	sink := msh.CommandInputMap[inputPk.CK]
	msh.Lock.Unlock()
	if sink == nil {
		// no sink and no running command
		return fmt.Errorf("cannot send input, cmd is not running (%s)", inputPk.CK)
	}
	return sink.HandleInput(inputPk)
}

func (msh *MShellProc) AddRunningCmd(rct *RunCmdType) {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	if rct.EphemeralOpts != nil {
		log.Printf("[info] adding ephemeral running command: %s\n", rct.CK)
	}
	msh.RunningCmds[rct.RunPacket.CK] = rct
}

func (msh *MShellProc) GetRunningCmd(ck base.CommandKey) *RunCmdType {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	rtn := msh.RunningCmds[ck]
	return rtn
}

func (msh *MShellProc) RemoveRunningCmd(ck base.CommandKey) {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	delete(msh.RunningCmds, ck)
	for key, pendingCk := range msh.PendingStateCmds {
		if pendingCk == ck {
			delete(msh.PendingStateCmds, key)
		}
	}
}

func (msh *MShellProc) PacketRpcIter(ctx context.Context, pk packet.RpcPacketType) (*packet.RpcResponseIter, error) {
	if !msh.IsConnected() {
		return nil, fmt.Errorf("remote is not connected")
	}
	if pk == nil {
		return nil, fmt.Errorf("PacketRpc passed nil packet")
	}
	reqId := pk.GetReqId()
	msh.ServerProc.Output.RegisterRpcSz(reqId, RpcIterChannelSize)
	err := msh.ServerProc.Input.SendPacketCtx(ctx, pk)
	if err != nil {
		return nil, err
	}
	return msh.ServerProc.Output.GetResponseIter(reqId), nil
}

func (msh *MShellProc) PacketRpcRaw(ctx context.Context, pk packet.RpcPacketType) (packet.RpcResponsePacketType, error) {
	if !msh.IsConnected() {
		return nil, fmt.Errorf("remote is not connected")
	}
	if pk == nil {
		return nil, fmt.Errorf("PacketRpc passed nil packet")
	}
	reqId := pk.GetReqId()
	msh.ServerProc.Output.RegisterRpc(reqId)
	defer msh.ServerProc.Output.UnRegisterRpc(reqId)
	err := msh.ServerProc.Input.SendPacketCtx(ctx, pk)
	if err != nil {
		return nil, err
	}
	rtnPk := msh.ServerProc.Output.WaitForResponse(ctx, reqId)
	if rtnPk == nil {
		return nil, ctx.Err()
	}
	return rtnPk, nil
}

func (msh *MShellProc) PacketRpc(ctx context.Context, pk packet.RpcPacketType) (*packet.ResponsePacketType, error) {
	rtnPk, err := msh.PacketRpcRaw(ctx, pk)
	if err != nil {
		return nil, err
	}
	if respPk, ok := rtnPk.(*packet.ResponsePacketType); ok {
		return respPk, nil
	}
	return nil, fmt.Errorf("invalid response packet received: %s", packet.AsString(rtnPk))
}

func (msh *MShellProc) WithLock(fn func()) {
	msh.Lock.Lock()
	defer msh.Lock.Unlock()
	fn()
}

func makeDataAckPacket(ck base.CommandKey, fdNum int, ackLen int, err error) *packet.DataAckPacketType {
	ack := packet.MakeDataAckPacket()
	ack.CK = ck
	ack.FdNum = fdNum
	ack.AckLen = ackLen
	if err != nil {
		ack.Error = err.Error()
	}
	return ack
}

func (msh *MShellProc) notifyHangups_nolock() {
	for ck := range msh.RunningCmds {
		cmd, err := sstore.GetCmdByScreenId(context.Background(), ck.GetGroupId(), ck.GetCmdId())
		if err != nil {
			continue
		}
		update := scbus.MakeUpdatePacket()
		update.AddUpdate(*cmd)
		scbus.MainUpdateBus.DoScreenUpdate(ck.GetGroupId(), update)
		go pushNumRunningCmdsUpdate(&ck, -1)
	}
	msh.RunningCmds = make(map[base.CommandKey]*RunCmdType)
	msh.PendingStateCmds = make(map[pendingStateKey]base.CommandKey)
}

func (msh *MShellProc) resolveFinalState(ctx context.Context, origState *packet.ShellState, origStatePtr *packet.ShellStatePtr, donePk *packet.CmdDonePacketType) (*packet.ShellState, error) {
	if donePk.FinalState != nil {
		if origStatePtr == nil {
			return nil, fmt.Errorf("command must have a stateptr to resolve final state")
		}
		finalState := stripScVarsFromState(donePk.FinalState)
		return finalState, nil
	}
	if donePk.FinalStateDiff != nil {
		if donePk.FinalStateBasePtr == nil {
			return nil, fmt.Errorf("invalid rtnstate, has diff but no baseptr")
		}
		stateDiff := stripScVarsFromStateDiff(donePk.FinalStateDiff)
		if origStatePtr == donePk.FinalStateBasePtr {
			// this is the normal case.  the stateptr from the run-packet should match the baseptr from the done-packet
			// this is also the most efficient, because we don't need to fetch the original state
			sapi, err := shellapi.MakeShellApi(origState.GetShellType())
			if err != nil {
				return nil, fmt.Errorf("cannot make shellapi from initial state: %w", err)
			}
			fullState, err := sapi.ApplyShellStateDiff(origState, stateDiff)
			if err != nil {
				return nil, fmt.Errorf("cannot apply shell state diff: %w", err)
			}
			return fullState, nil
		}
		// this is strange (why is backend returning non-original stateptr?)
		// but here, we fetch the stateptr, and then apply the diff against that
		realOrigState, err := sstore.GetFullState(ctx, *donePk.FinalStateBasePtr)
		if err != nil {
			return nil, fmt.Errorf("cannot get original state for diff: %w", err)
		}
		if realOrigState == nil {
			return nil, fmt.Errorf("cannot get original state for diff: not found")
		}
		sapi, err := shellapi.MakeShellApi(realOrigState.GetShellType())
		if err != nil {
			return nil, fmt.Errorf("cannot make shellapi from original state: %w", err)
		}
		fullState, err := sapi.ApplyShellStateDiff(realOrigState, stateDiff)
		if err != nil {
			return nil, fmt.Errorf("cannot apply shell state diff: %w", err)
		}
		return fullState, nil
	}
	return nil, nil
}

// after this limit we'll switch to persisting the full state
const NewStateDiffSizeThreshold = 30 * 1024

// will update the remote instance with the final state
// this is complicated because we want to be as efficient as possible.
// so we pull the current remote-instance state (just the baseptr).  then we compute the diff.
// then we check the size of the diff, and only persist the diff it is under some size threshold
// also we check to see if the diff succeeds (it can fail if the shell or version changed).
// in those cases we also update the RI with the full state
func (msh *MShellProc) updateRIWithFinalState(ctx context.Context, rct *RunCmdType, newState *packet.ShellState) (*sstore.RemoteInstance, error) {
	curRIState, err := sstore.GetRemoteStatePtr(ctx, rct.SessionId, rct.ScreenId, rct.RemotePtr)
	if err != nil {
		return nil, fmt.Errorf("error trying to get current screen stateptr: %w", err)
	}
	feState := sstore.FeStateFromShellState(newState)
	if curRIState == nil {
		// no current state, so just persist the full state
		return sstore.UpdateRemoteState(ctx, rct.SessionId, rct.ScreenId, rct.RemotePtr, feState, newState, nil)
	}
	// pull the base (not the diff) state from the RI (right now we don't want to make multi-level diffs)
	riBaseState, err := sstore.GetStateBase(ctx, curRIState.BaseHash)
	if err != nil {
		return nil, fmt.Errorf("error trying to get statebase: %w", err)
	}
	sapi, err := shellapi.MakeShellApi(riBaseState.GetShellType())
	if err != nil {
		return nil, fmt.Errorf("error trying to make shellapi: %w", err)
	}
	newStateDiff, err := sapi.MakeShellStateDiff(riBaseState, curRIState.BaseHash, newState)
	if err != nil {
		// if we can't make a diff, just persist the full state (this could happen if the shell type changes)
		return sstore.UpdateRemoteState(ctx, rct.SessionId, rct.ScreenId, rct.RemotePtr, feState, newState, nil)
	}
	// we have a diff, let's check the diff size first
	_, encodedDiff := newStateDiff.EncodeAndHash()
	if len(encodedDiff) > NewStateDiffSizeThreshold {
		// diff is too large, persist the full state
		return sstore.UpdateRemoteState(ctx, rct.SessionId, rct.ScreenId, rct.RemotePtr, feState, newState, nil)
	}
	// diff is small enough, persist the diff
	return sstore.UpdateRemoteState(ctx, rct.SessionId, rct.ScreenId, rct.RemotePtr, feState, nil, newStateDiff)
}

func (msh *MShellProc) handleCmdStartError(rct *RunCmdType, startErr error) {
	if rct == nil {
		log.Printf("handleCmdStartError, no rct\n")
		return
	}
	defer msh.RemoveRunningCmd(rct.CK)
	if rct.EphemeralOpts != nil {
		// nothing to do for ephemeral commands besides remove the running command
		return
	}
	ctx, cancelFn := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFn()
	update := scbus.MakeUpdatePacket()
	errOutputStr := fmt.Sprintf("%serror: %v%s\n", utilfn.AnsiRedColor(), startErr, utilfn.AnsiResetColor())
	msh.writeToCmdPtyOut(ctx, rct.ScreenId, rct.CK.GetCmdId(), []byte(errOutputStr))
	doneInfo := sstore.CmdDoneDataValues{
		Ts:         time.Now().UnixMilli(),
		ExitCode:   1,
		DurationMs: 0,
	}
	err := sstore.UpdateCmdDoneInfo(ctx, update, rct.CK, doneInfo, sstore.CmdStatusError)
	if err != nil {
		log.Printf("error updating cmddone info (in handleCmdStartError): %v\n", err)
		return
	}
	screen, err := sstore.UpdateScreenFocusForDoneCmd(ctx, rct.CK.GetGroupId(), rct.CK.GetCmdId())
	if err != nil {
		log.Printf("error trying to update screen focus type (in handleCmdDonePacket): %v\n", err)
		// fall-through (nothing to do)
	}
	if screen != nil {
		update.AddUpdate(*screen)
	}
	scbus.MainUpdateBus.DoUpdate(update)
}

func (msh *MShellProc) handleCmdDonePacket(rct *RunCmdType, donePk *packet.CmdDonePacketType) {
	if rct == nil {
		log.Printf("cmddone packet received, but no running command found for it %q\n", donePk.CK)
		return
	}
	// this will remove from RunningCmds and from PendingStateCmds
	defer msh.RemoveRunningCmd(donePk.CK)
	if rct.EphemeralOpts != nil && rct.EphemeralOpts.Canceled.Load() {
		log.Printf("cmddone %s (ephemeral canceled)\n", donePk.CK)
		// do nothing when an ephemeral command is canceled
		return
	}
	ctx, cancelFn := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFn()
	update := scbus.MakeUpdatePacket()
	if rct.EphemeralOpts == nil {
		// only update DB for non-ephemeral commands
		cmdDoneInfo := sstore.CmdDoneDataValues{
			Ts:         donePk.Ts,
			ExitCode:   donePk.ExitCode,
			DurationMs: donePk.DurationMs,
		}
		err := sstore.UpdateCmdDoneInfo(ctx, update, donePk.CK, cmdDoneInfo, sstore.CmdStatusDone)
		if err != nil {
			log.Printf("error updating cmddone info (in handleCmdDonePacket): %v\n", err)
			return
		}
		screen, err := sstore.UpdateScreenFocusForDoneCmd(ctx, donePk.CK.GetGroupId(), donePk.CK.GetCmdId())
		if err != nil {
			log.Printf("error trying to update screen focus type (in handleCmdDonePacket): %v\n", err)
			// fall-through (nothing to do)
		}
		if screen != nil {
			update.AddUpdate(*screen)
		}
	}

	// Close the ephemeral response writer if it exists
	if rct.EphemeralOpts != nil && rct.EphemeralOpts.ExpectsResponse {
		log.Printf("closing ephemeral response writers\n")
		defer rct.EphemeralOpts.StdoutWriter.Close()
		defer rct.EphemeralOpts.StderrWriter.Close()
	}

	// ephemeral commands *do* update the remote state
	// not all commands get a final state (only RtnState commands have this returned)
	// so in those cases finalState will be nil
	finalState, err := msh.resolveFinalState(ctx, rct.RunPacket.State, rct.RunPacket.StatePtr, donePk)
	if err != nil {
		log.Printf("error resolving final state for cmd: %v\n", err)
		// fallthrough
	}
	if finalState != nil {
		newRI, err := msh.updateRIWithFinalState(ctx, rct, finalState)
		if err != nil {
			log.Printf("error updating RI with final state (in handleCmdDonePacket): %v\n", err)
			// fallthrough
		}
		if newRI != nil {
			update.AddUpdate(sstore.MakeSessionUpdateForRemote(rct.SessionId, newRI))
		}
		// ephemeral commands *do not* update cmd state (there is no command)
		if newRI != nil && rct.EphemeralOpts == nil {
			newRIStatePtr := packet.ShellStatePtr{BaseHash: newRI.StateBaseHash, DiffHashArr: newRI.StateDiffHashArr}
			err = sstore.UpdateCmdRtnState(ctx, donePk.CK, newRIStatePtr)
			if err != nil {
				log.Printf("error trying to update cmd rtnstate: %v\n", err)
				// fall-through (nothing to do)
			}
		}
	}
	scbus.MainUpdateBus.DoUpdate(update)
}

func (msh *MShellProc) handleCmdFinalPacket(rct *RunCmdType, finalPk *packet.CmdFinalPacketType) {
	if rct == nil {
		// this is somewhat expected, since cmddone should have removed the running command
		return
	}
	defer msh.RemoveRunningCmd(finalPk.CK)
	rtnCmd, err := sstore.GetCmdByScreenId(context.Background(), finalPk.CK.GetGroupId(), finalPk.CK.GetCmdId())
	if err != nil {
		log.Printf("error calling GetCmdById in handleCmdFinalPacket: %v\n", err)
		return
	}
	if rtnCmd == nil || rtnCmd.DoneTs > 0 {
		return
	}
	log.Printf("finalpk %s (hangup): %s\n", finalPk.CK, finalPk.Error)
	screen, err := sstore.HangupCmd(context.Background(), finalPk.CK)
	if err != nil {
		log.Printf("error in hangup-cmd in handleCmdFinalPacket: %v\n", err)
		return
	}
	rtnCmd, err = sstore.GetCmdByScreenId(context.Background(), finalPk.CK.GetGroupId(), finalPk.CK.GetCmdId())
	if err != nil {
		log.Printf("error getting cmd(2) in handleCmdFinalPacket: %v\n", err)
		return
	}
	if rtnCmd == nil {
		log.Printf("error getting cmd(2) in handleCmdFinalPacket (not found)\n")
		return
	}
	update := scbus.MakeUpdatePacket()
	update.AddUpdate(*rtnCmd)
	if screen != nil {
		update.AddUpdate(*screen)
	}
	go pushNumRunningCmdsUpdate(&finalPk.CK, -1)
	scbus.MainUpdateBus.DoUpdate(update)
}

func (msh *MShellProc) ResetDataPos(ck base.CommandKey) {
	msh.DataPosMap.Delete(ck)
}

func (msh *MShellProc) writeToCmdPtyOut(ctx context.Context, screenId string, lineId string, data []byte) error {
	dataPos := msh.DataPosMap.Get(base.MakeCommandKey(screenId, lineId))
	update, err := sstore.AppendToCmdPtyBlob(ctx, screenId, lineId, data, dataPos)
	if err != nil {
		return err
	}
	utilfn.IncSyncMap(msh.DataPosMap, base.MakeCommandKey(screenId, lineId), int64(len(data)))
	if update != nil {
		scbus.MainUpdateBus.DoScreenUpdate(screenId, update)
	}
	return nil
}

func (msh *MShellProc) handleDataPacket(rct *RunCmdType, dataPk *packet.DataPacketType, dataPosMap *utilfn.SyncMap[base.CommandKey, int64]) {
	if rct == nil {
		log.Printf("error handling data packet: no running cmd found %s\n", dataPk.CK)
		ack := makeDataAckPacket(dataPk.CK, dataPk.FdNum, 0, fmt.Errorf("no running cmd found"))
		msh.ServerProc.Input.SendPacket(ack)
		return
	}
	realData, err := base64.StdEncoding.DecodeString(dataPk.Data64)
	if err != nil {
		log.Printf("error decoding data packet: %v\n", err)
		ack := makeDataAckPacket(dataPk.CK, dataPk.FdNum, 0, err)
		msh.ServerProc.Input.SendPacket(ack)
		return
	}
	if rct.EphemeralOpts != nil {
		// Write to the response writer if it's set
		if len(realData) > 0 && rct.EphemeralOpts.ExpectsResponse {
			switch dataPk.FdNum {
			case 1:
				_, err := rct.EphemeralOpts.StdoutWriter.Write(realData)
				if err != nil {
					log.Printf("*error writing to ephemeral stdout writer: %v\n", err)
				}
			case 2:
				_, err := rct.EphemeralOpts.StderrWriter.Write(realData)
				if err != nil {
					log.Printf("*error writing to ephemeral stderr writer: %v\n", err)
				}
			default:
				log.Printf("error handling data packet: invalid fdnum %d\n", dataPk.FdNum)
			}
		}
		ack := makeDataAckPacket(dataPk.CK, dataPk.FdNum, len(realData), nil)
		msh.ServerProc.Input.SendPacket(ack)
		return
	}

	var ack *packet.DataAckPacketType
	if len(realData) > 0 {
		dataPos := dataPosMap.Get(dataPk.CK)
		update, err := sstore.AppendToCmdPtyBlob(context.Background(), rct.ScreenId, dataPk.CK.GetCmdId(), realData, dataPos)
		if err != nil {
			ack = makeDataAckPacket(dataPk.CK, dataPk.FdNum, 0, err)
		} else {
			ack = makeDataAckPacket(dataPk.CK, dataPk.FdNum, len(realData), nil)
		}
		utilfn.IncSyncMap(dataPosMap, dataPk.CK, int64(len(realData)))
		if update != nil {
			scbus.MainUpdateBus.DoScreenUpdate(dataPk.CK.GetGroupId(), update)
		}
	}
	if ack != nil {
		msh.ServerProc.Input.SendPacket(ack)
	}
}

func sendScreenUpdates(screens []*sstore.ScreenType) {
	for _, screen := range screens {
		update := scbus.MakeUpdatePacket()
		update.AddUpdate(*screen)
		scbus.MainUpdateBus.DoUpdate(update)
	}
}

func (msh *MShellProc) processSinglePacket(pk packet.PacketType) {
	if _, ok := pk.(*packet.DataAckPacketType); ok {
		// TODO process ack (need to keep track of buffer size for sending)
		// this is low priority though since most input is coming from keyboard and won't overflow this buffer
		return
	}
	if dataPk, ok := pk.(*packet.DataPacketType); ok {
		runCmdUpdateFn(dataPk.CK, func() {
			rct := msh.GetRunningCmd(dataPk.CK)
			msh.handleDataPacket(rct, dataPk, msh.DataPosMap)
		})
		go pushStatusIndicatorUpdate(&dataPk.CK, sstore.StatusIndicatorLevel_Output)
		return
	}
	if donePk, ok := pk.(*packet.CmdDonePacketType); ok {
		runCmdUpdateFn(donePk.CK, func() {
			rct := msh.GetRunningCmd(donePk.CK)
			msh.handleCmdDonePacket(rct, donePk)
		})
		return
	}
	if finalPk, ok := pk.(*packet.CmdFinalPacketType); ok {
		runCmdUpdateFn(finalPk.CK, func() {
			rct := msh.GetRunningCmd(finalPk.CK)
			msh.handleCmdFinalPacket(rct, finalPk)
		})
		return
	}
	if msgPk, ok := pk.(*packet.MessagePacketType); ok {
		msh.WriteToPtyBuffer("msg> [remote %s] [%s] %s\n", msh.GetRemoteName(), msgPk.CK, msgPk.Message)
		return
	}
	if rawPk, ok := pk.(*packet.RawPacketType); ok {
		msh.WriteToPtyBuffer("stderr> [remote %s] %s\n", msh.GetRemoteName(), rawPk.Data)
		return
	}
	msh.WriteToPtyBuffer("*[remote %s] unhandled packet %s\n", msh.GetRemoteName(), packet.AsString(pk))
}

func (msh *MShellProc) ProcessPackets() {
	defer msh.WithLock(func() {
		if msh.Status == StatusConnected {
			msh.Status = StatusDisconnected
		}
		screens, err := sstore.HangupRunningCmdsByRemoteId(context.Background(), msh.Remote.RemoteId)
		if err != nil {
			msh.writeToPtyBuffer_nolock("error calling HUP on cmds %v\n", err)
		}
		msh.notifyHangups_nolock()
		go msh.NotifyRemoteUpdate()
		if len(screens) > 0 {
			go sendScreenUpdates(screens)
		}
	})
	for pk := range msh.ServerProc.Output.MainCh {
		msh.processSinglePacket(pk)
	}
}

// returns number of chars (including braces) for brace-expr
func getBracedStr(runeStr []rune) int {
	if len(runeStr) < 3 {
		return 0
	}
	if runeStr[0] != '{' {
		return 0
	}
	for i := 1; i < len(runeStr); i++ {
		if runeStr[i] == '}' {
			if i == 1 { // cannot have {}
				return 0
			}
			return i + 1
		}
	}
	return 0
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9' // just check ascii digits (not unicode)
}

func EvalPrompt(promptFmt string, vars map[string]string, state *packet.ShellState) string {
	var buf bytes.Buffer
	promptRunes := []rune(promptFmt)
	for i := 0; i < len(promptRunes); i++ {
		ch := promptRunes[i]
		if ch == '\\' && i != len(promptRunes)-1 {
			nextCh := promptRunes[i+1]
			if nextCh == 'x' || nextCh == 'y' {
				nr := getBracedStr(promptRunes[i+2:])
				if nr > 0 {
					escCode := string(promptRunes[i+1 : i+1+nr+1]) // start at "x" or "y", extend nr+1 runes
					escStr := evalPromptEsc(escCode, vars, state)
					buf.WriteString(escStr)
					i += nr + 1
					continue
				} else {
					buf.WriteRune(ch) // invalid escape, so just write ch and move on
					continue
				}
			} else if isDigit(nextCh) {
				if len(promptRunes) >= i+4 && isDigit(promptRunes[i+2]) && isDigit(promptRunes[i+3]) {
					i += 3
					escStr := evalPromptEsc(string(promptRunes[i+1:i+4]), vars, state)
					buf.WriteString(escStr)
					continue
				} else {
					buf.WriteRune(ch) // invalid escape, so just write ch and move on
					continue
				}
			} else {
				i += 1
				escStr := evalPromptEsc(string(nextCh), vars, state)
				buf.WriteString(escStr)
				continue
			}
		}
		buf.WriteRune(ch)
	}
	return buf.String()
}

func evalPromptEsc(escCode string, vars map[string]string, state *packet.ShellState) string {
	if strings.HasPrefix(escCode, "x{") && strings.HasSuffix(escCode, "}") {
		varName := escCode[2 : len(escCode)-1]
		return vars[varName]
	}
	if strings.HasPrefix(escCode, "y{") && strings.HasSuffix(escCode, "}") {
		if state == nil {
			return ""
		}
		varName := escCode[2 : len(escCode)-1]
		varMap := shellenv.ShellVarMapFromState(state)
		return varMap[varName]
	}
	if escCode == "h" {
		return vars["remoteshorthost"]
	}
	if escCode == "H" {
		return vars["remotehost"]
	}
	if escCode == "s" {
		return "mshell"
	}
	if escCode == "u" {
		return vars["remoteuser"]
	}
	if escCode == "w" {
		if state == nil {
			return "?"
		}
		return replaceHomePath(state.Cwd, vars["home"])
	}
	if escCode == "W" {
		if state == nil {
			return "?"
		}
		return path.Base(replaceHomePath(state.Cwd, vars["home"]))
	}
	if escCode == "$" {
		if vars["remoteuser"] == "root" || vars["sudo"] == "1" {
			return "#"
		} else {
			return "$"
		}
	}
	if len(escCode) == 3 {
		// \nnn escape
		ival, err := strconv.ParseInt(escCode, 8, 32)
		if err != nil {
			return escCode
		}
		if ival >= 0 && ival <= 255 {
			return string([]byte{byte(ival)})
		} else {
			// if it was out of range just return the string (invalid escape)
			return escCode
		}
	}
	if escCode == "e" {
		return "\033"
	}
	if escCode == "n" {
		return "\n"
	}
	if escCode == "r" {
		return "\r"
	}
	if escCode == "a" {
		return "\007"
	}
	if escCode == "\\" {
		return "\\"
	}
	if escCode == "[" {
		return ""
	}
	if escCode == "]" {
		return ""
	}

	// we don't support date/time escapes (d, t, T, @), version escapes (v, V), cmd number (#, !), terminal device (l), jobs (j)
	return "(" + escCode + ")"
}

func (msh *MShellProc) getFullState(shellType string, stateDiff *packet.ShellStateDiff) (*packet.ShellState, error) {
	baseState := msh.StateMap.GetStateByHash(shellType, stateDiff.BaseHash)
	if baseState != nil && len(stateDiff.DiffHashArr) == 0 {
		sapi, err := shellapi.MakeShellApi(baseState.GetShellType())
		newState, err := sapi.ApplyShellStateDiff(baseState, stateDiff)
		if err != nil {
			return nil, err
		}
		return newState, nil
	} else {
		fullState, err := sstore.GetFullState(context.Background(), packet.ShellStatePtr{BaseHash: stateDiff.BaseHash, DiffHashArr: stateDiff.DiffHashArr})
		if err != nil {
			return nil, err
		}
		sapi, err := shellapi.MakeShellApi(fullState.GetShellType())
		if err != nil {
			return nil, err
		}
		newState, err := sapi.ApplyShellStateDiff(fullState, stateDiff)
		return newState, nil
	}
}

// internal func, first tries the StateMap, otherwise will fallback on sstore.GetFullState
func (msh *MShellProc) getFeStateFromDiff(stateDiff *packet.ShellStateDiff) (map[string]string, error) {
	baseState := msh.StateMap.GetStateByHash(stateDiff.GetShellType(), stateDiff.BaseHash)
	if baseState != nil && len(stateDiff.DiffHashArr) == 0 {
		sapi, err := shellapi.MakeShellApi(baseState.GetShellType())
		if err != nil {
			return nil, err
		}
		newState, err := sapi.ApplyShellStateDiff(baseState, stateDiff)
		if err != nil {
			return nil, err
		}
		return sstore.FeStateFromShellState(newState), nil
	} else {
		fullState, err := sstore.GetFullState(context.Background(), packet.ShellStatePtr{BaseHash: stateDiff.BaseHash, DiffHashArr: stateDiff.DiffHashArr})
		if err != nil {
			return nil, err
		}
		sapi, err := shellapi.MakeShellApi(fullState.GetShellType())
		if err != nil {
			return nil, err
		}
		newState, err := sapi.ApplyShellStateDiff(fullState, stateDiff)
		if err != nil {
			return nil, err
		}
		return sstore.FeStateFromShellState(newState), nil
	}
}

func (msh *MShellProc) TryAutoConnect() error {
	if msh.IsConnected() {
		return nil
	}
	rcopy := msh.GetRemoteCopy()
	if rcopy.ConnectMode == sstore.ConnectModeManual {
		return nil
	}
	var err error
	msh.WithLock(func() {
		if msh.NumTryConnect > 5 {
			err = fmt.Errorf("too many unsuccessful tries")
			return
		}
		msh.NumTryConnect++
	})
	if err != nil {
		return err
	}
	msh.Launch(false)
	if !msh.IsConnected() {
		return fmt.Errorf("error connecting")
	}
	return nil
}

func (msh *MShellProc) GetDisplayName() string {
	rcopy := msh.GetRemoteCopy()
	return rcopy.GetName()
}

// Identify the screen for a given CommandKey and push the given status indicator update for that screen
func pushStatusIndicatorUpdate(ck *base.CommandKey, level sstore.StatusIndicatorLevel) {
	screenId := ck.GetGroupId()
	err := sstore.SetStatusIndicatorLevel(context.Background(), screenId, level, false)
	if err != nil {
		log.Printf("error setting status indicator level: %v\n", err)
	}
}

func pushNumRunningCmdsUpdate(ck *base.CommandKey, delta int) {
	screenId := ck.GetGroupId()
	sstore.IncrementNumRunningCmds(screenId, delta)
}
