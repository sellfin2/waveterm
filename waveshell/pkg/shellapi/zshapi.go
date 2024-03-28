// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package shellapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"unicode"

	"github.com/alessio/shellescape"
	"github.com/wavetermdev/waveterm/waveshell/pkg/base"
	"github.com/wavetermdev/waveterm/waveshell/pkg/binpack"
	"github.com/wavetermdev/waveterm/waveshell/pkg/packet"
	"github.com/wavetermdev/waveterm/waveshell/pkg/shellenv"
	"github.com/wavetermdev/waveterm/waveshell/pkg/statediff"
	"github.com/wavetermdev/waveterm/waveshell/pkg/utilfn"
	"github.com/wavetermdev/waveterm/waveshell/pkg/wlog"
	"github.com/wavetermdev/waveterm/wavesrv/pkg/scbase"
)

const BaseZshOpts = ``

const ZshShellVersionCmdStr = `echo zsh v$ZSH_VERSION`

const (
	ZshSection_Version = iota
	ZshSection_Cwd
	ZshSection_Env
	ZshSection_Mods
	ZshSection_Vars
	ZshSection_Aliases
	ZshSection_Fpath
	ZshSection_Funcs
	ZshSection_PVars
	ZshSection_Prompt
	ZshSection_EndBytes

	ZshSection_NumFieldsExpected // must be last
)

// TODO these need updating
const RunZshSudoCommandFmt = `sudo -n -C %d zsh /dev/fd/%d`
const RunZshSudoPasswordCommandFmt = `cat /dev/fd/%d | sudo -k -S -C %d zsh -c "echo '[from-mshell]'; exec %d>&-; zsh /dev/fd/%d < /dev/fd/%d"`

var ZshIgnoreVars = map[string]bool{
	"_":                    true,
	"0":                    true,
	"terminfo":             true,
	"RANDOM":               true,
	"COLUMNS":              true,
	"LINES":                true,
	"argv":                 true,
	"SECONDS":              true,
	"PWD":                  true,
	"HISTCHARS":            true,
	"HISTFILE":             true,
	"HISTSIZE":             true,
	"SAVEHIST":             true,
	"ZSH_EXECUTION_STRING": true,
	"EPOCHSECONDS":         true,
	"EPOCHREALTIME":        true,
	"SHLVL":                true,
	"TTY":                  true,
	"ZDOTDIR":              true,
	"PPID":                 true,
	"epochtime":            true,
	"langinfo":             true,
	"keymaps":              true,
	"widgets":              true,
	"options":              true,
	"aliases":              true,
	"dis_aliases":          true,
	"saliases":             true,
	"dis_saliases":         true,
	"galiases":             true,
	"dis_galiases":         true,
	"builtins":             true,
	"dis_builtins":         true,
	"modules":              true,
	"history":              true,
	"historywords":         true,
	"jobdirs":              true,
	"jobstates":            true,
	"jobtexts":             true,
	"funcfiletrace":        true,
	"funcsourcetrace":      true,
	"funcstack":            true,
	"functrace":            true,
	"nameddirs":            true,
	"userdirs":             true,
	"parameters":           true,
	"commands":             true,
	"functions":            true,
	"dis_functions":        true,
	"functions_source":     true,
	"dis_functions_source": true,
	"_comps":               true,
	"_patcomps":            true,
	"_postpatcomps":        true,

	// zsh/system
	"errnos":    true,
	"sysparams": true,

	// zsh/curses
	"ZCURSES_COLORS":      true,
	"ZCURSES_COLOR_PAIRS": true,
	"zcurses_attrs":       true,
	"zcurses_colors":      true,
	"zcurses_keycodes":    true,
	"zcurses_windows":     true,

	// not listed, but we also exclude all ZFTP_* variables

	// powerlevel10k
	"_GITSTATUS_CLIENT_PID_POWERLEVEL9K":  true,
	"GITSTATUS_DAEMON_PID_POWERLEVEL9K":   true,
	"_GITSTATUS_FILE_PREFIX_POWERLEVEL9K": true,
}

var ZshIgnoreFuncs = map[string]bool{
	"zftp_chpwd":    true,
	"zftp_progress": true,
}

// only options we restore (other than ZshForceOptions)
var ZshIgnoreOptions = map[string]bool{
	"globalrcs":        true, // must stay off (otherwise /etc/zprofile runs)
	"ksharrays":        true,
	"kshtypeset":       true,
	"kshautoload":      true,
	"kshzerosubscript": true,
	"interactive":      true,
	"login":            true,
	"zle":              true,
	"shinstdin":        true,
	"privileged":       true,
	"restricted":       true,
	"singlecommand":    true,
}

// force these options on/off at beginning of rcfile
var ZshForceOptions = map[string]bool{
	"globalrcs":        false,
	"ksharrays":        false,
	"kshtypeset":       false,
	"kshautoload":      false,
	"kshzerosubscript": false,
	"xtrace":           false, // not in ZshIgnoreOptions
	"verbose":          false, // not in ZshIgnoreOptions
	"debugbeforecmd":   false, // not in ZshIgnoreOptions
}

var ZshUniqueArrayVars = map[string]bool{
	"path":  true,
	"fpath": true,
}

var ZshSpecialDecls = map[string]bool{
	"precmd_functions":  true,
	"preexec_functions": true,
}

var ZshUnsetVars = []string{
	"HISTFILE",
	"ZSH_EXECUTION_STRING",
}

var ZshForceLoadMods = map[string]bool{
	"zsh/parameter": true,
	"zsh/langinfo":  true,
}

const ZModsVarName = "WAVESTATE_ZMODS"

// do not use these directly, call GetLocalMajorVersion()
var localZshMajorVersionOnce = &sync.Once{}
var localZshMajorVersion = ""

// sentinel value for functions that should be autoloaded
const ZshFnAutoLoad = "autoload"
const ZshAutoloadFnBody = "builtin autoload -XU"

type ZshParamKey struct {
	// paramtype cannot contain spaces
	// "aliases", "dis_aliases", "saliases", "dis_saliases", "galiases", "dis_galiases"
	// "functions", "dis_functions", "functions_source", "dis_functions_source"
	ParamType string
	ParamName string
}

func (k ZshParamKey) String() string {
	return k.ParamType + " " + k.ParamName
}

func ZshParamKeyFromString(s string) (ZshParamKey, error) {
	parts := strings.SplitN(s, " ", 2)
	if len(parts) != 2 {
		return ZshParamKey{}, fmt.Errorf("invalid zsh param key")
	}
	return ZshParamKey{ParamType: parts[0], ParamName: parts[1]}, nil
}

type ZshMap = map[ZshParamKey]string

type zshShellApi struct{}

func (z zshShellApi) GetShellType() string {
	return packet.ShellType_zsh
}

func (z zshShellApi) MakeExitTrap(fdNum int) (string, []byte) {
	return MakeZshExitTrap(fdNum)
}

func (z zshShellApi) GetLocalMajorVersion() string {
	return GetLocalZshMajorVersion()
}

func (z zshShellApi) GetLocalShellPath() string {
	return "/bin/zsh"
}

func (z zshShellApi) GetRemoteShellPath() string {
	return "zsh"
}

func (z zshShellApi) MakeRunCommand(cmdStr string, opts RunCommandOpts) string {
	if !opts.Sudo {
		return cmdStr
	}
	if opts.SudoWithPass {
		return fmt.Sprintf(RunZshSudoPasswordCommandFmt, opts.PwFdNum, opts.MaxFdNum+1, opts.PwFdNum, opts.CommandFdNum, opts.CommandStdinFdNum)
	} else {
		return fmt.Sprintf(RunZshSudoCommandFmt, opts.MaxFdNum+1, opts.CommandFdNum)
	}
}

func (z zshShellApi) MakeShExecCommand(cmdStr string, rcFileName string, usePty bool) *exec.Cmd {
	return exec.Command(GetLocalZshPath(), "-l", "-i", "-c", cmdStr)
}

func (z zshShellApi) GetShellState(ctx context.Context, outCh chan ShellStateOutput, stdinDataCh chan []byte) {
	defer close(outCh)
	stateCmd, endBytes := GetZshShellStateCmd(StateOutputFdNum)
	cmdStr := BaseZshOpts + "; " + stateCmd
	ecmd := exec.CommandContext(ctx, GetLocalZshPath(), "-l", "-i", "-c", cmdStr)
	outputCh := make(chan []byte, 10)
	var outputWg sync.WaitGroup
	outputWg.Add(1)
	go func() {
		defer outputWg.Done()
		for outputBytes := range outputCh {
			outCh <- ShellStateOutput{Output: outputBytes}
		}
	}()
	outputBytes, err := StreamCommandWithExtraFd(ctx, ecmd, outputCh, StateOutputFdNum, endBytes, stdinDataCh)
	outputWg.Wait()
	if err != nil {
		outCh <- ShellStateOutput{Error: err.Error()}
		return
	}
	rtn, stats, err := z.ParseShellStateOutput(outputBytes)
	if err != nil {
		outCh <- ShellStateOutput{Error: err.Error()}
		return
	}
	outCh <- ShellStateOutput{ShellState: rtn, Stats: stats}
}

func (z zshShellApi) GetBaseShellOpts() string {
	return BaseZshOpts
}

func makeZshTypesetStmt(varDecl *shellenv.DeclareDeclType) string {
	if !varDecl.IsZshDecl {
		// not sure what to do here?
		return ""
	}
	var argsStr string
	if varDecl.Args == "" {
		argsStr = "--"
	} else {
		argsStr = "-" + varDecl.Args
	}
	if varDecl.IsZshScalarBound() {
		// varDecl.Value contains the extra "separator" field (if present in the original typeset def)
		return fmt.Sprintf("typeset %s %s %s=%s", argsStr, varDecl.ZshBoundScalar, varDecl.Name, varDecl.Value)
	} else {
		return fmt.Sprintf("typeset %s %s=%s", argsStr, varDecl.Name, varDecl.Value)
	}
}

func isZshSafeNameStr(name string) bool {
	for _, ch := range name {
		if ch == '_' {
			continue
		}
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) {
			continue
		}
		return false
	}
	return true
}

func (z zshShellApi) MakeRcFileStr(pk *packet.RunPacketType) string {
	var rcBuf bytes.Buffer
	rcBuf.WriteString(z.GetBaseShellOpts() + "\n")
	// rcBuf.WriteString("echo 'running generated rcfile' $0 $ZSH_ARGZERO '|' $ZDOTDIR\n")
	varDecls := shellenv.VarDeclsFromState(pk.State)
	// force options come at the beginning of the file (other options come at the end)
	for optName, optVal := range ZshForceOptions {
		if optVal {
			rcBuf.WriteString(fmt.Sprintf("setopt %s\n", optName))
		} else {
			rcBuf.WriteString(fmt.Sprintf("unsetopt %s\n", optName))
		}
	}
	for modName := range ZshForceLoadMods {
		rcBuf.WriteString(fmt.Sprintf("zmodload %s\n", modName))
	}
	modDecl := getDeclByName(varDecls, ZModsVarName)
	if modDecl != nil {
		modsArr := utilfn.QuickParseJson[[]string](modDecl.Value)
		for _, modName := range modsArr {
			if !ZshForceLoadMods[modName] {
				rcBuf.WriteString(fmt.Sprintf("zmodload %s\n", modName))
			}
		}
	}
	var postDecls []*shellenv.DeclareDeclType
	for _, varDecl := range varDecls {
		if ZshIgnoreVars[varDecl.Name] {
			continue
		}
		if strings.HasPrefix(varDecl.Name, "ZFTP_") {
			continue
		}
		if varDecl.IsExtVar {
			continue
		}
		if ZshUniqueArrayVars[varDecl.Name] && !varDecl.IsUniqueArray() {
			varDecl.AddFlag("U")
		}
		if ZshSpecialDecls[varDecl.Name] {
			postDecls = append(postDecls, varDecl)
			continue
		}
		stmt := makeZshTypesetStmt(varDecl)
		if stmt == "" {
			continue
		}
		if varDecl.IsReadOnly() {
			// we can't reset read-only variables
			// so we check if it is a "safe" name, and then we can write a conditional
			//     that only sets it if it hasn't already been set.
			if !isZshSafeNameStr(varDecl.Name) {
				continue
			}
			rcBuf.WriteString(fmt.Sprintf("if (( ! ${+%s} )); then\n", varDecl.Name))
			rcBuf.WriteString(makeZshTypesetStmt(varDecl))
			rcBuf.WriteString("\nfi")
		} else {
			rcBuf.WriteString(makeZshTypesetStmt(varDecl))
		}
		rcBuf.WriteString("\n")
	}
	// do NOT unset ZDOTDIR, otherwise initialization will start to read initialization files from ~/ again
	for _, varName := range ZshUnsetVars {
		rcBuf.WriteString("unset " + shellescape.Quote(varName) + "\n")
	}

	// aliases
	aliasMap, err := DecodeZshMap([]byte(pk.State.Aliases))
	if err != nil {
		base.Logf("error decoding zsh aliases: %v\n", err)
		rcBuf.WriteString("# error decoding zsh aliases\n")
	} else {
		for aliasKey, aliasValue := range aliasMap {
			// tricky here, don't quote AliasName (it gets implicit quotes, and quoting doesn't work as expected)
			aliasStr := fmt.Sprintf("%s[%s]=%s\n", aliasKey.ParamType, aliasKey.ParamName, shellescape.Quote(aliasValue))
			rcBuf.WriteString(aliasStr)
		}
	}

	// functions
	fnMap, err := DecodeZshMap([]byte(pk.State.Funcs))
	if err != nil {
		base.Logf("error decoding zsh functions: %v\n", err)
		rcBuf.WriteString("# error decoding zsh functions\n")
	} else {
		for fnKey, fnValue := range fnMap {
			if ZshIgnoreFuncs[fnKey.ParamName] {
				continue
			}
			if fnValue == ZshFnAutoLoad {
				rcBuf.WriteString(fmt.Sprintf("autoload %s\n", shellescape.Quote(fnKey.ParamName)))
			} else {
				// careful, no whitespace (except newlines)
				rcBuf.WriteString(fmt.Sprintf("function %s () {\n%s\n}\n", shellescape.Quote(fnKey.ParamName), fnValue))
				if fnKey.ParamType == "dis_functions" {
					rcBuf.WriteString(fmt.Sprintf("disable -f %s\n", shellescape.Quote(fnKey.ParamName)))
				}
			}
		}
	}
	// write postdecls
	for _, varDecl := range postDecls {
		rcBuf.WriteString(makeZshTypesetStmt(varDecl))
		rcBuf.WriteString("\n")
	}
	writeZshOptions(&rcBuf, varDecls)
	return rcBuf.String()
}

func writeZshOptions(rcBuf *bytes.Buffer, declArr []*shellenv.DeclareDeclType) {
	optionDecl := getDeclByName(declArr, "options")
	var optionsMap map[string]string
	if optionDecl != nil {
		var err error
		optionsMap, err = parseSimpleZshOptions(optionDecl.Value)
		if err != nil {
			wlog.Logf("error decoding zsh options: %v\n", err)
		}
	}
	for optName := range optionsMap {
		if ZshIgnoreOptions[optName] {
			continue
		}
		if optionsMap[optName] == "on" {
			rcBuf.WriteString(fmt.Sprintf("setopt %s\n", optName))
		} else {
			rcBuf.WriteString(fmt.Sprintf("unsetopt %s\n", optName))
		}
	}
}

func writeZshId(buf *bytes.Buffer, idStr string) {
	buf.WriteString(shellescape.Quote(idStr))
}

const numRandomBytes = 4

// returns (cmd-string, endbytes)
func GetZshShellStateCmd(fdNum int) (string, []byte) {
	var sectionSeparator []byte
	// adding this extra "\n" helps with debuging and readability of output
	sectionSeparator = append(sectionSeparator, byte('\n'))
	sectionSeparator = utilfn.AppendNonZeroRandomBytes(sectionSeparator, numRandomBytes)
	sectionSeparator = append(sectionSeparator, 0, 0)
	endBytes := utilfn.AppendNonZeroRandomBytes(nil, NumRandomEndBytes)
	endBytes = append(endBytes, byte('\n'))
	// we have to use these crazy separators because zsh allows basically anything in
	// variable names and values (including nulls).
	// note that we don't need crazy separators for "env" or "typeset".
	// environment variables *cannot* contain nulls by definition, and "typeset" already escapes nulls.
	// the raw aliases and functions though need to be handled more carefully
	// output redirection is necessary to prevent cooked tty options from screwing up the output (funcs especially)
	// note we do not need the "extra" separator that bashapi uses because we are reading from OUTPUTFD (which already excludes any spurious stdout/stderr data)
	cmd := `
exec > [%OUTPUTFD%]
unsetopt SH_WORD_SPLIT;
zmodload zsh/parameter;
zmodload zsh/langinfo;
[%ZSHVERSION%];
printf "\x00[%SECTIONSEP%]";
pwd;
printf "[%SECTIONSEP%]";
env -0;
printf "[%SECTIONSEP%]";
zmodload -L
printf "[%SECTIONSEP%]";
typeset -p +H -m '*';
printf "[%SECTIONSEP%]";
for var in "${(@k)aliases}"; do
	printf "aliases %s[%PARTSEP%]%s[%PARTSEP%]" $var ${aliases[$var]}
done
for var in "${(@k)dis_aliases}"; do
	printf "dis_aliases %s[%PARTSEP%]%s[%PARTSEP%]" $var ${dis_aliases[$var]}
done
for var in "${(@k)saliases}"; do
	printf "saliases %s[%PARTSEP%]%s[%PARTSEP%]" $var ${saliases[$var]}
done
for var in "${(@k)dis_saliases}"; do
	printf "dis_saliases %s[%PARTSEP%]%s[%PARTSEP%]" $var ${dis_saliases[$var]}
done
for var in "${(@k)galiases}"; do
	printf "galiases %s[%PARTSEP%]%s[%PARTSEP%]" $var ${galiases[$var]}
done
for var in "${(@k)dis_galiases}"; do
	printf "dis_galiases %s[%PARTSEP%]%s[%PARTSEP%]" $var ${dis_galiases[$var]}
done
printf "[%SECTIONSEP%]";
echo $FPATH;
printf "[%SECTIONSEP%]";
for var in "${(@k)functions}"; do
    printf "functions %s[%PARTSEP%]%s[%PARTSEP%]" $var ${functions[$var]}
done
for var in "${(@k)dis_functions}"; do
	printf "dis_functions %s[%PARTSEP%]%s[%PARTSEP%]" $var ${dis_functions[$var]}
done
for var in "${(@k)functions_source}"; do
	printf "functions_source %s[%PARTSEP%]%s[%PARTSEP%]" $var ${functions_source[$var]}
done
for var in "${(@k)dis_functions_source}"; do
    printf "dis_functions_source %s[%PARTSEP%]%s[%PARTSEP%]" $var ${dis_functions_source[$var]}
done
printf "[%SECTIONSEP%]";
[%GITBRANCH%]
[%K8SCONTEXT%]
[%K8SNAMESPACE%]
printf "[%SECTIONSEP%]";
print -P "$PS1"
printf "[%SECTIONSEP%]";
printf "[%ENDBYTES%]"
`
	cmd = strings.TrimSpace(cmd)
	cmd = strings.ReplaceAll(cmd, "[%ZSHVERSION%]", ZshShellVersionCmdStr)
	cmd = strings.ReplaceAll(cmd, "[%GITBRANCH%]", GetGitBranchCmdStr)
	cmd = strings.ReplaceAll(cmd, "[%K8SCONTEXT%]", GetK8sContextCmdStr)
	cmd = strings.ReplaceAll(cmd, "[%K8SNAMESPACE%]", GetK8sNamespaceCmdStr)
	cmd = strings.ReplaceAll(cmd, "[%PARTSEP%]", utilfn.ShellHexEscape(string(sectionSeparator[0:len(sectionSeparator)-1])))
	cmd = strings.ReplaceAll(cmd, "[%SECTIONSEP%]", utilfn.ShellHexEscape(string(sectionSeparator)))
	cmd = strings.ReplaceAll(cmd, "[%OUTPUTFD%]", fmt.Sprintf("/dev/fd/%d", fdNum))
	cmd = strings.ReplaceAll(cmd, "[%OUTPUTFDNUM%]", fmt.Sprintf("%d", fdNum))
	cmd = strings.ReplaceAll(cmd, "[%ENDBYTES%]", utilfn.ShellHexEscape(string(endBytes)))
	return cmd, endBytes
}

func MakeZshExitTrap(fdNum int) (string, []byte) {
	stateCmd, endBytes := GetZshShellStateCmd(fdNum)
	fmtStr := `
zshexit () {
    %s
}
`
	return fmt.Sprintf(fmtStr, stateCmd), endBytes
}

func execGetLocalZshShellVersion() string {
	ctx, cancelFn := context.WithTimeout(context.Background(), GetVersionTimeout)
	defer cancelFn()
	ecmd := exec.CommandContext(ctx, "zsh", "-c", ZshShellVersionCmdStr)
	out, err := ecmd.Output()
	if err != nil {
		return ""
	}
	versionStr := strings.TrimSpace(string(out))
	if strings.Index(versionStr, "zsh ") == -1 {
		return ""
	}
	return versionStr
}

func GetLocalZshMajorVersion() string {
	localZshMajorVersionOnce.Do(func() {
		fullVersion := execGetLocalZshShellVersion()
		localZshMajorVersion = packet.GetMajorVersion(fullVersion)
	})
	return localZshMajorVersion
}

func EncodeZshMap(m ZshMap) []byte {
	var buf bytes.Buffer
	binpack.PackUInt(&buf, uint64(len(m)))
	orderedKeys := utilfn.GetOrderedStringerMapKeys(m)
	for _, key := range orderedKeys {
		value := m[key]
		binpack.PackValue(&buf, []byte(key.String()))
		binpack.PackValue(&buf, []byte(value))
	}
	return buf.Bytes()
}

func EncodeZshMapForApply(m map[string][]byte) string {
	var buf bytes.Buffer
	binpack.PackUInt(&buf, uint64(len(m)))
	orderedKeys := utilfn.GetOrderedMapKeys(m)
	for _, key := range orderedKeys {
		value := m[key]
		binpack.PackValue(&buf, []byte(key))
		binpack.PackValue(&buf, value)
	}
	return buf.String()
}

func DecodeZshMapForDiff(barr []byte) (map[string][]byte, error) {
	rtn := make(map[string][]byte)
	buf := bytes.NewBuffer(barr)
	u := binpack.MakeUnpacker(buf)
	numEntries := u.UnpackUInt("numEntries")
	for idx := 0; idx < numEntries; idx++ {
		key := string(u.UnpackValue("key"))
		value := u.UnpackValue("value")
		rtn[key] = value
	}
	if u.Error() != nil {
		return nil, u.Error()
	}
	return rtn, nil
}

func DecodeZshMap(barr []byte) (ZshMap, error) {
	rtn := make(ZshMap)
	buf := bytes.NewBuffer(barr)
	u := binpack.MakeUnpacker(buf)
	numEntries := u.UnpackUInt("numEntries")
	for idx := 0; idx < numEntries; idx++ {
		key := string(u.UnpackValue("key"))
		value := string(u.UnpackValue("value"))
		zshKey, err := ZshParamKeyFromString(key)
		if err != nil {
			return nil, err
		}
		rtn[zshKey] = value
	}
	if u.Error() != nil {
		return nil, u.Error()
	}
	return rtn, nil
}

func parseZshAliasStateOutput(aliasBytes []byte, partSeparator []byte) map[ZshParamKey]string {
	aliasParts := bytes.Split(aliasBytes, partSeparator)
	rtn := make(map[ZshParamKey]string)
	for aliasPartIdx := 0; aliasPartIdx < len(aliasParts)-1; aliasPartIdx += 2 {
		aliasNameAndType := string(aliasParts[aliasPartIdx])
		aliasNameAndTypeParts := strings.SplitN(aliasNameAndType, " ", 2)
		if len(aliasNameAndTypeParts) != 2 {
			continue
		}
		aliasKey := ZshParamKey{ParamType: aliasNameAndTypeParts[0], ParamName: aliasNameAndTypeParts[1]}
		aliasValue := string(aliasParts[aliasPartIdx+1])
		rtn[aliasKey] = aliasValue
	}
	return rtn
}

func isSourceFileInFpath(fpathArr []string, sourceFile string) bool {
	for _, fpath := range fpathArr {
		if fpath == "" || fpath == "." {
			continue
		}
		firstChar := fpath[0]
		if firstChar != '/' && firstChar != '~' {
			continue
		}
		if strings.HasPrefix(sourceFile, fpath) {
			return true
		}
	}
	return false
}

func ParseZshFunctions(fpathArr []string, fnBytes []byte, partSeparator []byte) map[ZshParamKey]string {
	fnBody := make(map[ZshParamKey]string)
	fnSource := make(map[string]string)
	fnParts := bytes.Split(fnBytes, partSeparator)
	for fnPartIdx := 0; fnPartIdx < len(fnParts)-1; fnPartIdx += 2 {
		fnTypeAndName := string(fnParts[fnPartIdx])
		fnValue := string(fnParts[fnPartIdx+1])
		fnTypeAndNameParts := strings.SplitN(fnTypeAndName, " ", 2)
		if len(fnTypeAndNameParts) != 2 {
			continue
		}
		fnType := fnTypeAndNameParts[0]
		fnName := fnTypeAndNameParts[1]
		if fnName == "zshexit" {
			continue
		}
		if ZshIgnoreFuncs[fnName] {
			continue
		}
		if fnType == "functions" || fnType == "dis_functions" {
			fnBody[ZshParamKey{ParamType: fnType, ParamName: fnName}] = fnValue
		}
		if fnType == "functions_source" || fnType == "dis_functions_source" {
			fnSource[fnName] = fnValue
		}
	}
	// ok, so the trick here is that we want to only include functions that are *not* autoloaded
	// the ones that are pending autoloading or come from a source file in fpath, can just be set to autoload
	for fnKey := range fnBody {
		var inFpath bool
		source := fnSource[fnKey.ParamName]
		if source != "" {
			inFpath = isSourceFileInFpath(fpathArr, source)
		}
		isAutoloadFnBody := strings.TrimSpace(fnBody[fnKey]) == ZshAutoloadFnBody
		if inFpath || isAutoloadFnBody {
			fnBody[fnKey] = ZshFnAutoLoad
		}
	}
	return fnBody
}

func makeZshFuncsStrForShellState(fnMap map[ZshParamKey]string) string {
	var buf bytes.Buffer
	for fnKey, fnValue := range fnMap {
		buf.WriteString(fmt.Sprintf("%s %s %s\x00", fnKey.ParamType, fnKey.ParamName, fnValue))
	}
	return buf.String()
}

func (z zshShellApi) ParseShellStateOutput(outputBytes []byte) (*packet.ShellState, *packet.ShellStateStats, error) {
	if scbase.IsDevMode() && DebugState {
		writeStateToFile(packet.ShellType_zsh, outputBytes)
	}
	firstZeroIdx := bytes.Index(outputBytes, []byte{0})
	firstDZeroIdx := bytes.Index(outputBytes, []byte{0, 0})
	if firstZeroIdx == -1 || firstDZeroIdx == -1 {
		return nil, nil, fmt.Errorf("invalid zsh shell state output, could not parse separator bytes")
	}
	versionStr := string(outputBytes[0:firstZeroIdx])
	sectionSeparator := outputBytes[firstZeroIdx+1 : firstDZeroIdx+2]
	partSeparator := sectionSeparator[0 : len(sectionSeparator)-1]
	// sections: see ZshSection_* consts
	sections := bytes.Split(outputBytes, sectionSeparator)
	if len(sections) != ZshSection_NumFieldsExpected {
		return nil, nil, fmt.Errorf("invalid zsh shell state output, wrong number of sections, section=%d", len(sections))
	}
	rtn := &packet.ShellState{}
	rtn.Version = strings.TrimSpace(versionStr)
	if rtn.GetShellType() != packet.ShellType_zsh {
		return nil, nil, fmt.Errorf("invalid zsh shell state output, wrong shell type")
	}
	if _, _, err := packet.ParseShellStateVersion(rtn.Version); err != nil {
		return nil, nil, fmt.Errorf("invalid zsh shell state output, invalid version: %v", err)
	}
	cwdStr := stripNewLineChars(string(sections[ZshSection_Cwd]))
	rtn.Cwd = cwdStr
	zshEnv := parseZshEnv(sections[ZshSection_Env])
	zshDecls, err := parseZshDecls(sections[ZshSection_Vars])
	if err != nil {
		base.Logf("invalid - parsedecls %v\n", err)
		return nil, nil, err
	}
	for _, decl := range zshDecls {
		if decl.IsZshScalarBound() {
			decl.ZshEnvValue = zshEnv[decl.ZshBoundScalar]
		}
	}
	aliasMap := parseZshAliasStateOutput(sections[ZshSection_Aliases], partSeparator)
	rtn.Aliases = string(EncodeZshMap(aliasMap))
	fpathStr := stripNewLineChars(string(string(sections[ZshSection_Fpath])))
	fpathArr := strings.Split(fpathStr, ":")
	zshFuncs := ParseZshFunctions(fpathArr, sections[ZshSection_Funcs], partSeparator)
	rtn.Funcs = string(EncodeZshMap(zshFuncs))
	pvarMap := parseExtVarOutput(sections[ZshSection_PVars], string(sections[ZshSection_Prompt]), string(sections[ZshSection_Mods]))
	utilfn.CombineMaps(zshDecls, pvarMap)
	rtn.ShellVars = shellenv.SerializeDeclMap(zshDecls)
	stats := &packet.ShellStateStats{
		Version:    rtn.Version,
		AliasCount: int(len(aliasMap)),
		FuncCount:  int(len(zshFuncs)),
		VarCount:   int(len(zshDecls)),
		EnvCount:   int(len(zshEnv)),
		HashVal:    rtn.GetHashVal(false),
		OutputSize: int64(len(outputBytes)),
		StateSize:  rtn.ApproximateSize(),
	}
	return rtn, stats, nil
}

func parseZshEnv(output []byte) map[string]string {
	outputStr := string(output)
	lines := strings.Split(outputStr, "\x00")
	rtn := make(map[string]string)
	for _, line := range lines {
		if line == "" {
			continue
		}
		eqIdx := strings.Index(line, "=")
		if eqIdx == -1 {
			continue
		}
		name := line[0:eqIdx]
		if ZshIgnoreVars[name] {
			continue
		}
		val := line[eqIdx+1:]
		rtn[name] = val
	}
	return rtn
}

func parseZshScalarBoundAssignment(declStr string, decl *DeclareDeclType) error {
	declStr = strings.TrimLeft(declStr, " ")
	spaceIdx := strings.Index(declStr, " ")
	if spaceIdx == -1 {
		return fmt.Errorf("invalid zsh decl (scalar bound): %q", declStr)
	}
	decl.ZshBoundScalar = declStr[0:spaceIdx]
	standardDecl := declStr[spaceIdx+1:]
	return parseStandardZshAssignment(standardDecl, decl)
}

func parseStandardZshAssignment(declStr string, decl *DeclareDeclType) error {
	declStr = strings.TrimLeft(declStr, " ")
	eqIdx := strings.Index(declStr, "=")
	if eqIdx == -1 {
		return fmt.Errorf("invalid zsh decl: %q", declStr)
	}
	decl.Name = declStr[0:eqIdx]
	decl.Value = declStr[eqIdx+1:]
	return nil
}

func parseZshDeclAssignment(declStr string, decl *DeclareDeclType) error {
	if decl.IsZshScalarBound() {
		return parseZshScalarBoundAssignment(declStr, decl)
	}
	return parseStandardZshAssignment(declStr, decl)
}

// returns (newDeclStr, argsStr, err)
func parseZshDeclArgs(declStr string, isExport bool) (string, string, error) {
	origDeclStr := declStr
	var argsStr string
	if isExport {
		argsStr = "x"
	}
	declStr = strings.TrimLeft(declStr, " ")
	for strings.HasPrefix(declStr, "-") {
		spaceIdx := strings.Index(declStr, " ")
		if spaceIdx == -1 {
			return "", "", fmt.Errorf("invalid zsh export line: %q", origDeclStr)
		}
		newArgsStr := strings.TrimSpace(declStr[1:spaceIdx])
		argsStr = argsStr + newArgsStr
		declStr = declStr[spaceIdx+1:]
		declStr = strings.TrimLeft(declStr, " ")
	}
	return declStr, argsStr, nil
}

func stripNewLineChars(s string) string {
	for {
		if len(s) == 0 {
			return s
		}
		lastChar := s[len(s)-1]
		if lastChar == '\n' || lastChar == '\r' {
			s = s[0 : len(s)-1]
		} else {
			return s
		}
	}
}

func parseZshDeclLine(line string) (*DeclareDeclType, error) {
	line = stripNewLineChars(line)
	if strings.HasPrefix(line, "export ") {
		exportLine := line[7:]
		assignLine, exportArgs, err := parseZshDeclArgs(exportLine, true)
		rtn := &DeclareDeclType{IsZshDecl: true, Args: exportArgs}
		err = parseZshDeclAssignment(assignLine, rtn)
		if err != nil {
			return nil, err
		}
		return rtn, nil
	} else if strings.HasPrefix(line, "typeset ") {
		typesetLine := line[8:]
		assignLine, typesetArgs, err := parseZshDeclArgs(typesetLine, false)
		rtn := &DeclareDeclType{IsZshDecl: true, Args: typesetArgs}
		err = parseZshDeclAssignment(assignLine, rtn)
		if err != nil {
			return nil, err
		}
		return rtn, nil
	} else {
		return nil, fmt.Errorf("invalid zsh decl line: %q", line)
	}
}

// combine decl2 INTO decl1
func combineTiedZshDecls(decl1 *DeclareDeclType, decl2 *DeclareDeclType) {
	if decl2.IsExport() {
		decl1.AddFlag("x")
	}
	if decl2.IsArray() {
		decl1.AddFlag("a")
	}
}

func parseZshDecls(output []byte) (map[string]*DeclareDeclType, error) {
	// NOTES:
	// - we get extra \r characters in the output (trimmed in parseZshDeclLine) (we get \r\n)
	// - tied variables (-T) are printed twice! this is especially confusing for exported vars:
	//       (1) `export -T PATH path=( ... )`
	//       (2) `typeset -aT PATH path=( ... )`
	//    we have to "combine" these two lines into one decl.
	outputStr := string(output)
	lines := strings.Split(outputStr, "\n")
	rtn := make(map[string]*DeclareDeclType)
	for _, line := range lines {
		if line == "" {
			continue
		}
		decl, err := parseZshDeclLine(line)
		if err != nil {
			base.Logf("error parsing zsh decl line: %v", err)
			continue
		}
		if decl == nil {
			continue
		}
		if ZshIgnoreVars[decl.Name] {
			continue
		}
		if rtn[decl.Name] != nil && decl.IsZshScalarBound() {
			combineTiedZshDecls(rtn[decl.Name], decl)
			continue
		}
		rtn[decl.Name] = decl
	}
	return rtn, nil
}

func makeZshMapDiff(oldMap string, newMap string) ([]byte, error) {
	oldMapMap, err := DecodeZshMapForDiff([]byte(oldMap))
	if err != nil {
		return nil, fmt.Errorf("error zshMapDiff decoding old-zsh map: %v", err)
	}
	newMapMap, err := DecodeZshMapForDiff([]byte(newMap))
	if err != nil {
		return nil, fmt.Errorf("error zshMapDiff decoding new-zsh map: %v", err)
	}
	return statediff.MakeMapDiff(oldMapMap, newMapMap), nil
}

func applyZshMapDiff(oldMap string, diff []byte) (string, error) {
	oldMapMap, err := DecodeZshMapForDiff([]byte(oldMap))
	if err != nil {
		return "", fmt.Errorf("error zshMapDiff decoding old-zsh map: %v", err)
	}
	newMapMap, err := statediff.ApplyMapDiff(oldMapMap, diff)
	if err != nil {
		return "", fmt.Errorf("error zshMapDiff applying diff: %v", err)
	}
	return EncodeZshMapForApply(newMapMap), nil
}

func (zshShellApi) MakeShellStateDiff(oldState *packet.ShellState, oldStateHash string, newState *packet.ShellState) (*packet.ShellStateDiff, error) {
	if oldState == nil {
		return nil, fmt.Errorf("cannot diff, oldState is nil")
	}
	if newState == nil {
		return nil, fmt.Errorf("cannot diff, newState is nil")
	}
	if oldState.Version != newState.Version {
		return nil, fmt.Errorf("cannot diff, states have different versions")
	}
	rtn := &packet.ShellStateDiff{}
	rtn.BaseHash = oldStateHash
	rtn.Version = newState.Version // always set version
	if oldState.Cwd != newState.Cwd {
		rtn.Cwd = newState.Cwd
	}
	rtn.Error = newState.Error
	oldVars := shellenv.ShellStateVarsToMap(oldState.ShellVars)
	newVars := shellenv.ShellStateVarsToMap(newState.ShellVars)
	rtn.VarsDiff = statediff.MakeMapDiff(oldVars, newVars)
	var err error
	rtn.AliasesDiff, err = makeZshMapDiff(oldState.Aliases, newState.Aliases)
	if err != nil {
		return nil, err
	}
	rtn.FuncsDiff, err = makeZshMapDiff(oldState.Funcs, newState.Funcs)
	if err != nil {
		return nil, err
	}
	return rtn, nil
}

func (zshShellApi) ApplyShellStateDiff(oldState *packet.ShellState, diff *packet.ShellStateDiff) (*packet.ShellState, error) {
	if oldState == nil {
		return nil, fmt.Errorf("cannot apply diff, oldState is nil")
	}
	if diff == nil {
		return oldState, nil
	}
	rtnState := &packet.ShellState{}
	var err error
	rtnState.Version = oldState.Version
	if diff.Version != rtnState.Version {
		rtnState.Version = diff.Version
	}
	rtnState.Cwd = oldState.Cwd
	if diff.Cwd != "" {
		rtnState.Cwd = diff.Cwd
	}
	rtnState.Error = diff.Error
	oldVars := shellenv.ShellStateVarsToMap(oldState.ShellVars)
	newVars, err := statediff.ApplyMapDiff(oldVars, diff.VarsDiff)
	if err != nil {
		return nil, fmt.Errorf("applying mapdiff 'vars': %v", err)
	}
	rtnState.ShellVars = shellenv.StrMapToShellStateVars(newVars)
	rtnState.Aliases, err = applyZshMapDiff(oldState.Aliases, diff.AliasesDiff)
	if err != nil {
		return nil, fmt.Errorf("applying diff 'aliases': %v", err)
	}
	rtnState.Funcs, err = applyZshMapDiff(oldState.Funcs, diff.FuncsDiff)
	if err != nil {
		return nil, fmt.Errorf("applying diff 'funcs': %v", err)
	}
	return rtnState, nil
}

// this will *not* parse general zsh assoc arrays, used to parse zsh options (no spaces)
// ( [posixargzero]=off [autolist]=on )
func parseSimpleZshOptions(decl string) (map[string]string, error) {
	decl = strings.TrimSpace(decl)
	if !strings.HasPrefix(decl, "(") || !strings.HasSuffix(decl, ")") {
		return nil, errors.New("invalid assoc array decl, must start and end with parens")
	}
	decl = decl[1 : len(decl)-1]
	parts := strings.Split(decl, " ")
	rtn := make(map[string]string)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eqIdx := strings.Index(part, "=")
		if eqIdx == -1 {
			return nil, fmt.Errorf("invalid assoc array decl part: %q", part)
		}
		bracketedKey := part[0:eqIdx]
		val := part[eqIdx+1:]
		if !strings.HasPrefix(bracketedKey, "[") || !strings.HasSuffix(bracketedKey, "]") {
			return nil, fmt.Errorf("invalid assoc array decl part: %q", part)
		}
		key := bracketedKey[1 : len(bracketedKey)-1]
		rtn[key] = val
	}
	return rtn, nil
}

func getDeclByName(decls []*shellenv.DeclareDeclType, name string) *shellenv.DeclareDeclType {
	for _, decl := range decls {
		if decl.Name == name {
			return decl
		}
	}
	return nil
}
