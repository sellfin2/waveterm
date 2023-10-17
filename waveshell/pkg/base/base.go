// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package base

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/mod/semver"
)

const HomeVarName = "HOME"
const DefaultMShellHome = "~/.mshell"
const DefaultMShellName = "mshell"
const MShellPathVarName = "MSHELL_PATH"
const MShellHomeVarName = "MSHELL_HOME"
const MShellInstallBinVarName = "MSHELL_INSTALLBIN_PATH"
const SSHCommandVarName = "SSH_COMMAND"
const MShellDebugVarName = "MSHELL_DEBUG"
const SessionsDirBaseName = "sessions"
const MShellVersion = "v0.3.0"
const RemoteIdFile = "remoteid"
const DefaultMShellInstallBinDir = "/opt/mshell/bin"
const LogFileName = "mshell.log"
const ForceDebugLog = false

const DebugFlag_LogRcFile = "logrc"
const LogRcFileName = "debug.rcfile"

var sessionDirCache = make(map[string]string)
var baseLock = &sync.Mutex{}
var DebugLogEnabled = false
var DebugLogger *log.Logger
var BuildTime string = "0"

type CommandFileNames struct {
	PtyOutFile    string
	StdinFifo     string
	RunnerOutFile string
}

type CommandKey string

func SetBuildTime(build string) {
	BuildTime = build
}

func MakeCommandKey(sessionId string, cmdId string) CommandKey {
	if sessionId == "" && cmdId == "" {
		return CommandKey("")
	}
	return CommandKey(fmt.Sprintf("%s/%s", sessionId, cmdId))
}

func (ckey CommandKey) IsEmpty() bool {
	return string(ckey) == ""
}

func Logf(fmtStr string, args ...interface{}) {
	if (!DebugLogEnabled && !ForceDebugLog) || DebugLogger == nil {
		return
	}
	DebugLogger.Printf(fmtStr, args...)
}

func InitDebugLog(prefix string) {
	homeDir := GetMShellHomeDir()
	err := os.MkdirAll(homeDir, 0777)
	if err != nil {
		return
	}
	logFile := path.Join(homeDir, LogFileName)
	fd, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	DebugLogger = log.New(fd, prefix+" ", log.LstdFlags)
	Logf("logger initialized\n")
}

func SetEnableDebugLog(enable bool) {
	DebugLogEnabled = enable
}

// deprecated (use GetGroupId instead)
func (ckey CommandKey) GetSessionId() string {
	return ckey.GetGroupId()
}

func (ckey CommandKey) GetGroupId() string {
	slashIdx := strings.Index(string(ckey), "/")
	if slashIdx == -1 {
		return ""
	}
	return string(ckey[0:slashIdx])
}

func (ckey CommandKey) GetCmdId() string {
	slashIdx := strings.Index(string(ckey), "/")
	if slashIdx == -1 {
		return ""
	}
	return string(ckey[slashIdx+1:])
}

func (ckey CommandKey) Split() (string, string) {
	fields := strings.SplitN(string(ckey), "/", 2)
	if len(fields) < 2 {
		return "", ""
	}
	return fields[0], fields[1]
}

func (ckey CommandKey) Validate(typeStr string) error {
	if typeStr == "" {
		typeStr = "ck"
	}
	if ckey == "" {
		return fmt.Errorf("%s has empty commandkey", typeStr)
	}
	sessionId, cmdId := ckey.Split()
	if sessionId == "" {
		return fmt.Errorf("%s does not have sessionid", typeStr)
	}
	_, err := uuid.Parse(sessionId)
	if err != nil {
		return fmt.Errorf("%s has invalid sessionid '%s'", typeStr, sessionId)
	}
	if cmdId == "" {
		return fmt.Errorf("%s does not have cmdid", typeStr)
	}
	_, err = uuid.Parse(cmdId)
	if err != nil {
		return fmt.Errorf("%s has invalid cmdid '%s'", typeStr, cmdId)
	}
	return nil
}

func HasDebugFlag(envMap map[string]string, flagName string) bool {
	msDebug := envMap[MShellDebugVarName]
	flags := strings.Split(msDebug, ",")
	Logf("hasdebugflag[%s]: %s [%#v]\n", flagName, msDebug, flags)
	for _, flag := range flags {
		if strings.TrimSpace(flag) == flagName {
			return true
		}
	}
	return false
}

func GetDebugRcFileName() string {
	msHome := GetMShellHomeDir()
	return path.Join(msHome, LogRcFileName)
}

func GetHomeDir() string {
	homeVar := os.Getenv(HomeVarName)
	if homeVar == "" {
		return "/"
	}
	return homeVar
}

func GetMShellHomeDir() string {
	homeVar := os.Getenv(MShellHomeVarName)
	if homeVar != "" {
		return homeVar
	}
	return ExpandHomeDir(DefaultMShellHome)
}

func GetCommandFileNames(ck CommandKey) (*CommandFileNames, error) {
	if err := ck.Validate("ck"); err != nil {
		return nil, fmt.Errorf("cannot get command files: %w", err)
	}
	sessionId, cmdId := ck.Split()
	sdir, err := EnsureSessionDir(sessionId)
	if err != nil {
		return nil, err
	}
	base := path.Join(sdir, cmdId)
	return &CommandFileNames{
		PtyOutFile:    base + ".ptyout",
		StdinFifo:     base + ".stdin",
		RunnerOutFile: base + ".runout",
	}, nil
}

func CleanUpCmdFiles(sessionId string, cmdId string) error {
	if cmdId == "" {
		return fmt.Errorf("bad cmdid, cannot clean up")
	}
	sdir, err := EnsureSessionDir(sessionId)
	if err != nil {
		return err
	}
	cmdFileGlob := path.Join(sdir, cmdId+".*")
	matches, err := filepath.Glob(cmdFileGlob)
	if err != nil {
		return err
	}
	for _, file := range matches {
		rmErr := os.Remove(file)
		if err == nil && rmErr != nil {
			err = rmErr
		}
	}
	return err
}

func GetSessionsDir() string {
	mhome := GetMShellHomeDir()
	sdir := path.Join(mhome, SessionsDirBaseName)
	return sdir
}

func EnsureSessionDir(sessionId string) (string, error) {
	if sessionId == "" {
		return "", fmt.Errorf("Bad sessionid, cannot be empty")
	}
	baseLock.Lock()
	sdir, ok := sessionDirCache[sessionId]
	baseLock.Unlock()
	if ok {
		return sdir, nil
	}
	mhome := GetMShellHomeDir()
	sdir = path.Join(mhome, SessionsDirBaseName, sessionId)
	info, err := os.Stat(sdir)
	if errors.Is(err, fs.ErrNotExist) {
		err = os.MkdirAll(sdir, 0777)
		if err != nil {
			return "", fmt.Errorf("cannot make mshell session directory[%s]: %w", sdir, err)
		}
		info, err = os.Stat(sdir)
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("session dir '%s' must be a directory", sdir)
	}
	baseLock.Lock()
	sessionDirCache[sessionId] = sdir
	baseLock.Unlock()
	return sdir, nil
}

func GetMShellPath() (string, error) {
	msPath := os.Getenv(MShellPathVarName) // use MSHELL_PATH
	if msPath != "" {
		return exec.LookPath(msPath)
	}
	mhome := GetMShellHomeDir()
	userMShellPath := path.Join(mhome, DefaultMShellName) // look in ~/.mshell
	msPath, err := exec.LookPath(userMShellPath)
	if err == nil {
		return msPath, nil
	}
	return exec.LookPath(DefaultMShellName) // standard path lookup for 'mshell'
}

func GetMShellSessionsDir() (string, error) {
	mhome := GetMShellHomeDir()
	return path.Join(mhome, SessionsDirBaseName), nil
}

func ExpandHomeDir(pathStr string) string {
	if pathStr != "~" && !strings.HasPrefix(pathStr, "~/") {
		return pathStr
	}
	homeDir := GetHomeDir()
	if pathStr == "~" {
		return homeDir
	}
	return path.Join(homeDir, pathStr[2:])
}

func ValidGoArch(goos string, goarch string) bool {
	return (goos == "darwin" || goos == "linux") && (goarch == "amd64" || goarch == "arm64")
}

func GoArchOptFile(version string, goos string, goarch string) string {
	installBinDir := os.Getenv(MShellInstallBinVarName)
	if installBinDir == "" {
		installBinDir = DefaultMShellInstallBinDir
	}
	versionStr := semver.MajorMinor(version)
	if versionStr == "" {
		versionStr = "unknown"
	}
	binBaseName := fmt.Sprintf("mshell-%s-%s.%s", versionStr, goos, goarch)
	return fmt.Sprintf(path.Join(installBinDir, binBaseName))
}

func MShellBinaryFromOptDir(version string, goos string, goarch string) (io.ReadCloser, error) {
	if !ValidGoArch(goos, goarch) {
		return nil, fmt.Errorf("invalid goos/goarch combination: %s/%s", goos, goarch)
	}
	versionStr := semver.MajorMinor(version)
	if versionStr == "" {
		return nil, fmt.Errorf("invalid mshell version: %q", version)
	}
	fileName := GoArchOptFile(version, goos, goarch)
	fd, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("cannot open mshell binary %q: %v", fileName, err)
	}
	return fd, nil
}

func GetRemoteId() (string, error) {
	mhome := GetMShellHomeDir()
	homeInfo, err := os.Stat(mhome)
	if errors.Is(err, fs.ErrNotExist) {
		err = os.MkdirAll(mhome, 0777)
		if err != nil {
			return "", fmt.Errorf("cannot make mshell home directory[%s]: %w", mhome, err)
		}
		homeInfo, err = os.Stat(mhome)
	}
	if err != nil {
		return "", fmt.Errorf("cannot stat mshell home directory[%s]: %w", mhome, err)
	}
	if !homeInfo.IsDir() {
		return "", fmt.Errorf("mshell home directory[%s] is not a directory", mhome)
	}
	remoteIdFile := path.Join(mhome, RemoteIdFile)
	fd, err := os.Open(remoteIdFile)
	if errors.Is(err, fs.ErrNotExist) {
		// write the file
		remoteId := uuid.New().String()
		err = os.WriteFile(remoteIdFile, []byte(remoteId), 0644)
		if err != nil {
			return "", fmt.Errorf("cannot write remoteid to '%s': %w", remoteIdFile, err)
		}
		return remoteId, nil
	} else if err != nil {
		return "", fmt.Errorf("cannot read remoteid file '%s': %w", remoteIdFile, err)
	} else {
		defer fd.Close()
		contents, err := io.ReadAll(fd)
		if err != nil {
			return "", fmt.Errorf("cannot read remoteid file '%s': %w", remoteIdFile, err)
		}
		uuidStr := string(contents)
		_, err = uuid.Parse(uuidStr)
		if err != nil {
			return "", fmt.Errorf("invalid uuid read from '%s': %w", remoteIdFile, err)
		}
		return uuidStr, nil
	}
}

func BoundInt(ival int, minVal int, maxVal int) int {
	if ival < minVal {
		return minVal
	}
	if ival > maxVal {
		return maxVal
	}
	return ival
}

func BoundInt64(ival int64, minVal int64, maxVal int64) int64 {
	if ival < minVal {
		return minVal
	}
	if ival > maxVal {
		return maxVal
	}
	return ival
}
