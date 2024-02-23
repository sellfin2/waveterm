// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as React from "react";
import * as mobxReact from "mobx-react";
import * as mobx from "mobx";
import * as util from "@/util/util";
import { If } from "tsx-control-statements/components";
import { boundMethod } from "autobind-decorator";
import cn from "classnames";
import { GlobalModel, GlobalCommandRunner, Screen } from "@/models";
import { getMonoFontSize } from "@/util/textmeasure";
import * as appconst from "@/app/appconst";
import { checkKeyPressed, adaptFromReactOrNativeKeyEvent } from "@/util/keyutil";

type OV<T> = mobx.IObservableValue<T>;

function pageSize(div: any): number {
    if (div == null) {
        return 300;
    }
    let size = div.clientHeight;
    if (size > 500) {
        size = size - 100;
    } else if (size > 200) {
        size = size - 30;
    }
    return size;
}

function scrollDiv(div: any, amt: number) {
    if (div == null) {
        return;
    }
    let newScrollTop = div.scrollTop + amt;
    if (newScrollTop < 0) {
        newScrollTop = 0;
    }
    div.scrollTo({ top: newScrollTop, behavior: "smooth" });
}

@mobxReact.observer
class TextAreaInput extends React.Component<{ screen: Screen; onHeightChange: () => void }, {}> {
    lastTab: boolean = false;
    lastHistoryUpDown: boolean = false;
    lastTabCurLine: OV<string> = mobx.observable.box(null);
    lastFocusType: string = null;
    mainInputRef: React.RefObject<HTMLTextAreaElement> = React.createRef();
    historyInputRef: React.RefObject<HTMLInputElement> = React.createRef();
    controlRef: React.RefObject<HTMLDivElement> = React.createRef();
    lastHeight: number = 0;
    lastSP: StrWithPos = { str: "", pos: appconst.NoStrPos };
    version: OV<number> = mobx.observable.box(0); // forces render updates

    incVersion(): void {
        let v = this.version.get();
        mobx.action(() => this.version.set(v + 1))();
    }

    getCurSP(): StrWithPos {
        let textarea = this.mainInputRef.current;
        if (textarea == null) {
            return this.lastSP;
        }
        let str = textarea.value;
        let pos = textarea.selectionStart;
        let endPos = textarea.selectionEnd;
        if (pos != endPos) {
            return { str, pos: appconst.NoStrPos };
        }
        return { str, pos };
    }

    updateSP(): void {
        let curSP = this.getCurSP();
        if (curSP.str == this.lastSP.str && curSP.pos == this.lastSP.pos) {
            return;
        }
        this.lastSP = curSP;
        GlobalModel.sendCmdInputText(this.props.screen.screenId, curSP);
    }

    setFocus(): void {
        let inputModel = GlobalModel.inputModel;
        if (inputModel.historyShow.get()) {
            this.historyInputRef.current.focus();
        } else {
            this.mainInputRef.current.focus();
        }
    }

    getTextAreaMaxCols(): number {
        let taElem = this.mainInputRef.current;
        if (taElem == null) {
            return 0;
        }
        let cs = window.getComputedStyle(taElem);
        let padding = parseFloat(cs.paddingLeft) + parseFloat(cs.paddingRight);
        let borders = parseFloat(cs.borderLeft) + parseFloat(cs.borderRight);
        let contentWidth = taElem.clientWidth - padding - borders;
        let fontSize = getMonoFontSize(parseInt(cs.fontSize));
        let maxCols = Math.floor(contentWidth / Math.ceil(fontSize.width));
        return maxCols;
    }

    checkHeight(shouldFire: boolean): void {
        let elem = this.controlRef.current;
        if (elem == null) {
            return;
        }
        let curHeight = elem.offsetHeight;
        if (this.lastHeight == curHeight) {
            return;
        }
        this.lastHeight = curHeight;
        if (shouldFire && this.props.onHeightChange != null) {
            this.props.onHeightChange();
        }
    }

    componentDidMount() {
        let activeScreen = GlobalModel.getActiveScreen();
        if (activeScreen != null) {
            let focusType = activeScreen.focusType.get();
            if (focusType == "input") {
                this.setFocus();
            }
            this.lastFocusType = focusType;
        }
        this.checkHeight(false);
        this.updateSP();
    }

    componentDidUpdate() {
        let activeScreen = GlobalModel.getActiveScreen();
        if (activeScreen != null) {
            let focusType = activeScreen.focusType.get();
            if (this.lastFocusType != focusType && focusType == "input") {
                this.setFocus();
            }
            this.lastFocusType = focusType;
        }
        let inputModel = GlobalModel.inputModel;
        let fcpos = inputModel.forceCursorPos.get();
        if (fcpos != null && fcpos != appconst.NoStrPos) {
            if (this.mainInputRef.current != null) {
                this.mainInputRef.current.selectionStart = fcpos;
                this.mainInputRef.current.selectionEnd = fcpos;
            }
            mobx.action(() => inputModel.forceCursorPos.set(null))();
        }
        if (inputModel.forceInputFocus) {
            inputModel.forceInputFocus = false;
            this.setFocus();
        }
        this.checkHeight(true);
        this.updateSP();
    }

    getLinePos(elem: any): { numLines: number; linePos: number } {
        let numLines = elem.value.split("\n").length;
        let linePos = elem.value.substr(0, elem.selectionStart).split("\n").length;
        return { numLines, linePos };
    }

    @mobx.action
    @boundMethod
    onKeyDown(e: any) {
        mobx.action(() => {
            if (util.isModKeyPress(e)) {
                return;
            }
            let model = GlobalModel;
            let inputModel = model.inputModel;
            let win = model.getScreenLinesForActiveScreen();
            let ctrlMod = e.getModifierState("Control") || e.getModifierState("Meta") || e.getModifierState("Shift");
            let curLine = inputModel.getCurLine();

            let waveEvent = adaptFromReactOrNativeKeyEvent(e);
            let lastTab = this.lastTab;
            this.lastTab = checkKeyPressed(waveEvent, "Tab");
            let lastHist = this.lastHistoryUpDown;
            this.lastHistoryUpDown = false;

            if (checkKeyPressed(waveEvent, "Tab")) {
                e.preventDefault();
                if (lastTab) {
                    GlobalModel.submitCommand(
                        "_compgen",
                        null,
                        [curLine],
                        { comppos: String(curLine.length), compshow: "1", nohist: "1" },
                        true
                    );
                    return;
                } else {
                    GlobalModel.submitCommand(
                        "_compgen",
                        null,
                        [curLine],
                        { comppos: String(curLine.length), nohist: "1" },
                        true
                    );
                    return;
                }
            }
            if (checkKeyPressed(waveEvent, "Enter")) {
                e.preventDefault();
                if (!ctrlMod) {
                    if (GlobalModel.inputModel.isEmpty()) {
                        let activeWindow = GlobalModel.getScreenLinesForActiveScreen();
                        let activeScreen = GlobalModel.getActiveScreen();
                        if (activeScreen != null && activeWindow != null && activeWindow.lines.length > 0) {
                            activeScreen.setSelectedLine(0);
                            GlobalCommandRunner.screenSelectLine("E");
                        }
                        return;
                    } else {
                        setTimeout(() => GlobalModel.inputModel.uiSubmitCommand(), 0);
                        return;
                    }
                }
                e.target.setRangeText("\n", e.target.selectionStart, e.target.selectionEnd, "end");
                GlobalModel.inputModel.setCurLine(e.target.value);
                return;
            }
            if (checkKeyPressed(waveEvent, "Escape")) {
                e.preventDefault();
                e.stopPropagation();
                let inputModel = GlobalModel.inputModel;
                inputModel.toggleInfoMsg();
                if (inputModel.inputMode.get() != null) {
                    inputModel.resetInputMode();
                }
                inputModel.closeAIAssistantChat(true);
                return;
            }
            if (checkKeyPressed(waveEvent, "Cmd:e")) {
                e.preventDefault();
                e.stopPropagation();
                let inputModel = GlobalModel.inputModel;
                inputModel.toggleExpandInput();
            }
            if (checkKeyPressed(waveEvent, "Ctrl:c")) {
                e.preventDefault();
                inputModel.resetInput();
                return;
            }
            if (checkKeyPressed(waveEvent, "Ctrl:u")) {
                e.preventDefault();
                this.controlU();
                return;
            }
            if (checkKeyPressed(waveEvent, "Ctrl:p")) {
                e.preventDefault();
                this.controlP();
                return;
            }
            if (checkKeyPressed(waveEvent, "Ctrl:n")) {
                e.preventDefault();
                this.controlN();
                return;
            }
            if (checkKeyPressed(waveEvent, "Ctrl:w")) {
                e.preventDefault();
                this.controlW();
                return;
            }
            if (checkKeyPressed(waveEvent, "Ctrl:y")) {
                e.preventDefault();
                this.controlY();
                return;
            }
            if (checkKeyPressed(waveEvent, "Ctrl:r")) {
                e.preventDefault();
                inputModel.openHistory();
                return;
            }
            if (checkKeyPressed(waveEvent, "ArrowUp") || checkKeyPressed(waveEvent, "ArrowDown")) {
                if (!inputModel.isHistoryLoaded()) {
                    if (checkKeyPressed(waveEvent, "ArrowUp")) {
                        this.lastHistoryUpDown = true;
                        inputModel.loadHistory(false, 1, "screen");
                    }
                    return;
                }
                // invisible history movement
                let linePos = this.getLinePos(e.target);
                if (checkKeyPressed(waveEvent, "ArrowUp")) {
                    if (!lastHist && linePos.linePos > 1) {
                        // regular arrow
                        return;
                    }
                    e.preventDefault();
                    inputModel.moveHistorySelection(1);
                    this.lastHistoryUpDown = true;
                    return;
                }
                if (checkKeyPressed(waveEvent, "ArrowDown")) {
                    if (!lastHist && linePos.linePos < linePos.numLines) {
                        // regular arrow
                        return;
                    }
                    e.preventDefault();
                    inputModel.moveHistorySelection(-1);
                    this.lastHistoryUpDown = true;
                    return;
                }
            }
            if (checkKeyPressed(waveEvent, "PageUp") || checkKeyPressed(waveEvent, "PageDown")) {
                e.preventDefault();
                let infoScroll = inputModel.hasScrollingInfoMsg();
                if (infoScroll) {
                    let div = document.querySelector(".cmd-input-info");
                    let amt = pageSize(div);
                    scrollDiv(div, checkKeyPressed(waveEvent, "PageUp") ? -amt : amt);
                }
            }
            if (checkKeyPressed(waveEvent, "Ctrl:Space")) {
                e.preventDefault();
                inputModel.openAIAssistantChat();
            }
            // console.log(e.code, e.keyCode, e.key, event.which, ctrlMod, e);
        })();
    }

    @boundMethod
    onChange(e: any) {
        mobx.action(() => {
            GlobalModel.inputModel.setCurLine(e.target.value);
        })();
    }

    @boundMethod
    onSelect(e: any) {
        this.incVersion();
    }

    @boundMethod
    onHistoryKeyDown(e: any) {
        let waveEvent = adaptFromReactOrNativeKeyEvent(e);
        let inputModel = GlobalModel.inputModel;
        if (checkKeyPressed(waveEvent, "Escape")) {
            e.preventDefault();
            inputModel.resetHistory();
            return;
        }
        if (checkKeyPressed(waveEvent, "Enter")) {
            e.preventDefault();
            inputModel.grabSelectedHistoryItem();
            return;
        }
        if (checkKeyPressed(waveEvent, "Ctrl:g")) {
            e.preventDefault();
            inputModel.resetInput();
            return;
        }
        if (checkKeyPressed(waveEvent, "Ctrl:c")) {
            e.preventDefault();
            inputModel.resetInput();
            return;
        }
        if (checkKeyPressed(waveEvent, "Cmd:r") || checkKeyPressed(waveEvent, "Ctrl:r")) {
            e.preventDefault();
            let opts = mobx.toJS(inputModel.historyQueryOpts.get());
            if (opts.limitRemote) {
                opts.limitRemote = false;
                opts.limitRemoteInstance = false;
            } else {
                opts.limitRemote = true;
                opts.limitRemoteInstance = true;
            }
            inputModel.setHistoryQueryOpts(opts);
            return;
        }
        if (checkKeyPressed(waveEvent, "Cmd:s") || checkKeyPressed(waveEvent, "Ctrl:s")) {
            e.preventDefault();
            let opts = mobx.toJS(inputModel.historyQueryOpts.get());
            let htype = opts.queryType;
            if (htype == "screen") {
                htype = "session";
            } else if (htype == "session") {
                htype = "global";
            } else {
                htype = "screen";
            }
            inputModel.setHistoryType(htype);
            return;
        }
        if (checkKeyPressed(waveEvent, "Tab")) {
            e.preventDefault();
            return;
        }
        if (checkKeyPressed(waveEvent, "ArrowUp") || checkKeyPressed(waveEvent, "ArrowDown")) {
            e.preventDefault();
            inputModel.moveHistorySelection(checkKeyPressed(waveEvent, "ArrowUp") ? 1 : -1);
            return;
        }
        if (checkKeyPressed(waveEvent, "PageUp") || checkKeyPressed(waveEvent, "PageDown")) {
            e.preventDefault();
            inputModel.moveHistorySelection(checkKeyPressed(waveEvent, "PageUp") ? 10 : -10);
            return;
        }
        if (checkKeyPressed(waveEvent, "Ctrl:p")) {
            e.preventDefault();
            inputModel.moveHistorySelection(1);
            return;
        }
        if (checkKeyPressed(waveEvent, "Ctrl:n")) {
            e.preventDefault();
            inputModel.moveHistorySelection(-1);
            return;
        }
    }

    @boundMethod
    controlU() {
        if (this.mainInputRef.current == null) {
            return;
        }
        let selStart = this.mainInputRef.current.selectionStart;
        let value = this.mainInputRef.current.value;
        if (selStart > value.length) {
            return;
        }
        let cutValue = value.substr(0, selStart);
        let restValue = value.substr(selStart);
        let cmdLineUpdate = { str: restValue, pos: 0 };
        navigator.clipboard.writeText(cutValue);
        GlobalModel.inputModel.updateCmdLine(cmdLineUpdate);
    }

    @boundMethod
    controlP() {
        let inputModel = GlobalModel.inputModel;
        if (!inputModel.isHistoryLoaded()) {
            this.lastHistoryUpDown = true;
            inputModel.loadHistory(false, 1, "screen");
            return;
        }
        inputModel.moveHistorySelection(1);
        this.lastHistoryUpDown = true;
    }

    @boundMethod
    controlN() {
        let inputModel = GlobalModel.inputModel;
        inputModel.moveHistorySelection(-1);
        this.lastHistoryUpDown = true;
    }

    @boundMethod
    controlW() {
        if (this.mainInputRef.current == null) {
            return;
        }
        let selStart = this.mainInputRef.current.selectionStart;
        let value = this.mainInputRef.current.value;
        if (selStart > value.length) {
            return;
        }
        let cutSpot = selStart - 1;
        let initial = true;
        for (; cutSpot >= 0; cutSpot--) {
            let ch = value[cutSpot];
            if (ch == " " && initial) {
                continue;
            }
            initial = false;
            if (ch == " ") {
                cutSpot++;
                break;
            }
        }
        if (cutSpot == -1) {
            cutSpot = 0;
        }
        let cutValue = value.slice(cutSpot, selStart);
        let prevValue = value.slice(0, cutSpot);
        let restValue = value.slice(selStart);
        let cmdLineUpdate = { str: prevValue + restValue, pos: prevValue.length };
        navigator.clipboard.writeText(cutValue);
        GlobalModel.inputModel.updateCmdLine(cmdLineUpdate);
    }

    @boundMethod
    controlY() {
        if (this.mainInputRef.current == null) {
            return;
        }
        let pastePromise = navigator.clipboard.readText();
        pastePromise.then((clipText) => {
            clipText = clipText ?? "";
            let selStart = this.mainInputRef.current.selectionStart;
            let selEnd = this.mainInputRef.current.selectionEnd;
            let value = this.mainInputRef.current.value;
            if (selStart > value.length || selEnd > value.length) {
                return;
            }
            let newValue = value.substr(0, selStart) + clipText + value.substr(selEnd);
            let cmdLineUpdate = { str: newValue, pos: selStart + clipText.length };
            GlobalModel.inputModel.updateCmdLine(cmdLineUpdate);
        });
    }

    @boundMethod
    handleHistoryInput(e: any) {
        let inputModel = GlobalModel.inputModel;
        mobx.action(() => {
            let opts = mobx.toJS(inputModel.historyQueryOpts.get());
            opts.queryStr = e.target.value;
            inputModel.setHistoryQueryOpts(opts);
        })();
    }

    @boundMethod
    handleMainFocus(e: any) {
        let inputModel = GlobalModel.inputModel;
        if (inputModel.historyShow.get()) {
            e.preventDefault();
            if (this.historyInputRef.current != null) {
                this.historyInputRef.current.focus();
            }
            return;
        }
        inputModel.setPhysicalInputFocused(true);
    }

    @boundMethod
    handleMainBlur(e: any) {
        if (document.activeElement == this.mainInputRef.current) {
            return;
        }
        GlobalModel.inputModel.setPhysicalInputFocused(false);
    }

    @boundMethod
    handleHistoryFocus(e: any) {
        let inputModel = GlobalModel.inputModel;
        if (!inputModel.historyShow.get()) {
            e.preventDefault();
            if (this.mainInputRef.current != null) {
                this.mainInputRef.current.focus();
            }
            return;
        }
        inputModel.setPhysicalInputFocused(true);
    }

    @boundMethod
    handleHistoryBlur(e: any) {
        if (document.activeElement == this.historyInputRef.current) {
            return;
        }
        GlobalModel.inputModel.setPhysicalInputFocused(false);
    }

    render() {
        let model = GlobalModel;
        let inputModel = model.inputModel;
        let curLine = inputModel.getCurLine();
        let fcp = inputModel.forceCursorPos.get(); // for reaction
        let displayLines = 1;
        let numLines = curLine.split("\n").length;
        let maxCols = this.getTextAreaMaxCols();
        let longLine = false;
        let version = this.version.get(); // to force reactions
        if (maxCols != 0 && curLine.length >= maxCols - 4) {
            longLine = true;
        }
        if (numLines > 1 || longLine || inputModel.inputExpanded.get()) {
            displayLines = 5;
        }
        let disabled = inputModel.historyShow.get();
        if (disabled) {
            displayLines = 1;
        }
        let activeScreen = GlobalModel.getActiveScreen();
        if (activeScreen != null) {
            activeScreen.focusType.get(); // for reaction
        }
        let termFontSize = GlobalModel.getTermFontSize();
        let fontSize = getMonoFontSize(termFontSize);
        let termPad = Math.floor(fontSize.height / 2);
        let computedInnerHeight = displayLines * fontSize.height + 2 * termPad;
        let computedOuterHeight = computedInnerHeight + 2 * termPad;
        let shellType: string = "";
        let screen = GlobalModel.getActiveScreen();
        if (screen != null) {
            let ri = screen.getCurRemoteInstance();
            if (ri != null && ri.shelltype != null) {
                shellType = ri.shelltype;
            }
        }
        return (
            <div
                className="textareainput-div control is-expanded"
                ref={this.controlRef}
                style={{ height: computedOuterHeight }}
            >
                <If condition={!disabled && !util.isBlank(shellType)}>
                    <div className="shelltag">{shellType}</div>
                </If>
                <textarea
                    key="main"
                    ref={this.mainInputRef}
                    spellCheck="false"
                    autoComplete="off"
                    autoCorrect="off"
                    id="main-cmd-input"
                    onFocus={this.handleMainFocus}
                    onBlur={this.handleMainBlur}
                    style={{ height: computedInnerHeight, minHeight: computedInnerHeight, fontSize: termFontSize }}
                    value={curLine}
                    onKeyDown={this.onKeyDown}
                    onChange={this.onChange}
                    onSelect={this.onSelect}
                    className={cn("textarea", { "display-disabled": disabled })}
                ></textarea>
                <input
                    key="history"
                    ref={this.historyInputRef}
                    spellCheck="false"
                    autoComplete="off"
                    autoCorrect="off"
                    className="history-input"
                    type="text"
                    onFocus={this.handleHistoryFocus}
                    onKeyDown={this.onHistoryKeyDown}
                    onChange={this.handleHistoryInput}
                    value={inputModel.historyQueryOpts.get().queryStr}
                />
            </div>
        );
    }
}

export { TextAreaInput };
