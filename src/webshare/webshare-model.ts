import * as mobx from "mobx";
import { sprintf } from "sprintf-js";
import { boundMethod } from "autobind-decorator";
import { handleJsonFetchResponse, isModKeyPress, base64ToArray } from "./util";
import * as T from "./types";
import { TermWrap } from "./term";
import * as lineutil from "./lineutil";
import * as util from "./util";
import { windowWidthToCols, windowHeightToRows, termWidthFromCols, termHeightFromRows } from "./textmeasure";
import { WebShareWSControl } from "./webshare-ws";

// @ts-ignore
let PROMPT_DEV = __PROMPT_DEV__;
// @ts-ignore
let PROMPT_VERSION = __PROMPT_VERSION__;
// @ts-ignore
let PROMPT_BULILD = __PROMPT_BUILD__;
// @ts-ignore
let PROMPT_API_ENDPOINT = __PROMPT_API_ENDPOINT__;
// @ts-ignore
let PROMPT_WSAPI_ENDPOINT = __PROMPT_WSAPI_ENDPOINT__;

type OV<V> = mobx.IObservableValue<V>;
type OArr<V> = mobx.IObservableArray<V>;
type OMap<K, V> = mobx.ObservableMap<K, V>;
type CV<V> = mobx.IComputedValue<V>;

type PtyListener = {
    receiveData(ptyPos: number, data: Uint8Array, reason?: string);
};

function isBlank(s: string) {
    return s == null || s == "";
}

function getBaseUrl() {
    return PROMPT_API_ENDPOINT;
}

function getBaseWSUrl() {
    return PROMPT_WSAPI_ENDPOINT;
}

class WebShareModelClass {
    viewKey: string;
    screenId: string;
    errMessage: OV<string> = mobx.observable.box(null, { name: "errMessage" });
    fullScreen: OV<T.WebFullScreen> = mobx.observable.box(null, { name: "webScreen" });
    terminals: Record<string, TermWrap> = {}; // lineid => TermWrap
    renderers: Record<string, T.RendererModel> = {}; // lineid => RendererModel
    contentHeightCache: Record<string, number> = {}; // lineid => height
    wsControl: WebShareWSControl;
    anchor: { anchorLine: number; anchorOffset: number } = { anchorLine: 0, anchorOffset: 0 };
    selectedLine: OV<number> = mobx.observable.box(0, { name: "selectedLine" });
    syncSelectedLine: OV<boolean> = mobx.observable.box(true, { name: "syncSelectedLine" });
    lastScreenSize: T.WindowSize = null;
    activePtyFetch: Record<string, boolean> = {}; // lineid -> active
    localPtyOffsetMap: Record<string, number> = {};
    remotePtyOffsetMap: Record<string, number> = {};
    activeUpdateFetch: boolean = false;
    remoteScreenVts: number = 0;
    isDev: boolean = PROMPT_DEV;

    constructor() {
        let pathName = window.location.pathname;
        let screenMatch = pathName.match(/\/share\/([a-f0-9-]+)/);
        if (screenMatch != null) {
            this.screenId = screenMatch[1];
        }
        let urlParams = new URLSearchParams(window.location.search);
        this.viewKey = urlParams.get("viewkey");
        if (this.screenId == null) {
            this.screenId = urlParams.get("screenid");
        }
        setTimeout(() => this.loadFullScreenData(false), 10);
        this.wsControl = new WebShareWSControl(
            getBaseWSUrl(),
            this.screenId,
            this.viewKey,
            this.wsMessageCallback.bind(this)
        );
        document.addEventListener("keydown", this.docKeyDownHandler.bind(this));
    }

    setErrMessage(msg: string): void {
        mobx.action(() => {
            this.errMessage.set(msg);
        })();
    }

    setSyncSelectedLine(val: boolean): void {
        mobx.action(() => {
            this.syncSelectedLine.set(val);
            if (val) {
                let fullScreen = this.fullScreen.get();
                if (fullScreen != null) {
                    this.selectedLine.set(fullScreen.screen.selectedline);
                }
            }
        })();
    }

    setLastScreenSize(winSize: T.WindowSize) {
        if (winSize == null || winSize.height == 0 || winSize.width == 0) {
            return;
        }
        this.lastScreenSize = winSize;
    }

    getMaxContentSize(): T.WindowSize {
        if (this.lastScreenSize == null) {
            let width = termWidthFromCols(80, WebShareModel.getTermFontSize());
            let height = termHeightFromRows(25, WebShareModel.getTermFontSize());
            return { width, height };
        }
        let winSize = this.lastScreenSize;
        let width = util.boundInt(winSize.width - 50, 100, 5000);
        let height = util.boundInt(winSize.height - 100, 100, 5000);
        return { width, height };
    }

    getIdealContentSize(): T.WindowSize {
        if (this.lastScreenSize == null) {
            let width = termWidthFromCols(80, WebShareModel.getTermFontSize());
            let height = termHeightFromRows(25, WebShareModel.getTermFontSize());
            return { width, height };
        }
        let winSize = this.lastScreenSize;
        let width = util.boundInt(Math.ceil((winSize.width - 50) * 0.7), 100, 5000);
        let height = util.boundInt(Math.ceil((winSize.height - 100) * 0.5), 100, 5000);
        return { width, height };
    }

    getSelectedLine(): number {
        return this.selectedLine.get();
    }

    getServerSelectedLine(): number {
        let fullScreen = this.fullScreen.get();
        if (fullScreen != null) {
            return fullScreen.screen.selectedline;
        }
    }

    setSelectedLine(lineNum: number): void {
        mobx.action(() => {
            this.selectedLine.set(lineNum);
        })();
    }

    updateSelectedLineIndex(delta: number): void {
        let fullScreen = this.fullScreen.get();
        if (fullScreen == null) {
            return;
        }
        let lineIndex = this.getLineIndex(this.selectedLine.get());
        if (lineIndex == -1) {
            return;
        }
        lineIndex += delta;
        let lines = fullScreen.lines;
        if (lineIndex < 0 || lineIndex >= lines.length) {
            return;
        }
        this.setSelectedLine(lines[lineIndex].linenum);
    }

    setAnchorFields(anchorLine: number, anchorOffset: number, reason: string): void {
        this.anchor.anchorLine = anchorLine;
        this.anchor.anchorOffset = anchorOffset;
    }

    getAnchor(): { anchorLine: number; anchorOffset: number } {
        return this.anchor;
    }

    getTermFontSize(): number {
        return 12;
    }

    resizeWindow(winSize: T.WindowSize): void {
        let cols = windowWidthToCols(winSize.width, this.getTermFontSize());
        for (let lineId in this.terminals) {
            let termWrap = this.terminals[lineId];
            termWrap.resizeCols(cols);
        }
    }

    mergeLine(fullScreen: T.WebFullScreen, newLine: T.WebLine) {
        for (let i = 0; i < fullScreen.lines.length; i++) {
            let line = fullScreen.lines[i];
            if (line.lineid == newLine.lineid) {
                fullScreen.lines[i] = newLine;
                return;
            }
            if (line.linenum > newLine.linenum) {
                fullScreen.lines.splice(i, 0, newLine);
                return;
            }
        }
        fullScreen.lines.push(newLine);
    }

    removeLine(fullScreen: T.WebFullScreen, lineId: string) {
        for (let i = 0; i < fullScreen.lines.length; i++) {
            let line = fullScreen.lines[i];
            if (line.lineid == lineId) {
                fullScreen.lines.splice(i, 1);
                break;
            }
        }
        for (let i = 0; i < fullScreen.cmds.length; i++) {
            let cmd = fullScreen.cmds[i];
            if (cmd.lineid == lineId) {
                fullScreen.cmds.splice(i, 1);
                break;
            }
        }
        this.unloadRenderer(lineId);
    }

    setCmdDone(lineId: string): void {
        let termWrap = this.getTermWrap(lineId);
        if (termWrap != null) {
            termWrap.cmdDone();
        }
    }

    mergeCmd(fullScreen: T.WebFullScreen, newCmd: T.WebCmd) {
        for (let i = 0; i < fullScreen.cmds.length; i++) {
            let cmd = fullScreen.cmds[i];
            if (cmd.lineid == newCmd.lineid) {
                let wasRunning = lineutil.cmdStatusIsRunning(cmd.status);
                let isRunning = lineutil.cmdStatusIsRunning(newCmd.status);
                if (wasRunning && !isRunning) {
                    setTimeout(() => this.setCmdDone(cmd.lineid), 300);
                }
                fullScreen.cmds[i] = newCmd;
                return;
            }
        }
        fullScreen.cmds.push(newCmd);
    }

    mergeUpdate(msg: T.WebFullScreen) {
        if (msg.screenid != this.screenId) {
            console.log("bad WebFullScreen update, wrong screenid", msg.screenid);
            return;
        }
        // console.log("merge screen-update", "vts=" + msg.vts);
        // console.log("merge", "vts=" + msg.vts, msg);
        mobx.action(() => {
            let fullScreen = this.fullScreen.get();
            if (fullScreen.vts >= msg.vts) {
                console.log("stale merge", "cur-vts=" + fullScreen.vts, "merge-vts=" + msg.vts);
                return;
            }
            fullScreen.vts = msg.vts;
            if (msg.screen) {
                fullScreen.screen = msg.screen;
                if (this.syncSelectedLine.get()) {
                    this.selectedLine.set(msg.screen.selectedline);
                }
            }
            if (msg.lines != null && msg.lines.length > 0) {
                for (let line of msg.lines) {
                    if (line.archived) {
                        this.removeLine(fullScreen, line.lineid);
                        continue;
                    }
                    this.mergeLine(fullScreen, line);
                }
            }
            if (msg.cmds != null && msg.cmds.length > 0) {
                for (let cmd of msg.cmds) {
                    this.mergeCmd(fullScreen, cmd);
                }
            }
            this.handleCmdPtyMap(msg.cmdptymap);
        })();
    }

    handleCmdPtyMap(ptyMap: Record<string, number>) {
        if (ptyMap == null) {
            return;
        }
        for (let lineId in ptyMap) {
            let newOffset = ptyMap[lineId];
            this.remotePtyOffsetMap[lineId] = newOffset;
            let localOffset = this.localPtyOffsetMap[lineId];
            if (localOffset != null && localOffset < newOffset) {
                this.runPtyFetch(lineId);
            }
        }
    }

    runPtyFetch(lineId: string) {
        let prtn = this.checkFetchPtyData(lineId, false);
        let ptyListener = this.getPtyListener(lineId);
        if (ptyListener != null) {
            prtn.then((ptydata) => {
                ptyListener.receiveData(ptydata.pos, ptydata.data, "model-fetch");
                if (ptydata.data.length > 0) {
                    setTimeout(() => this.checkFetchPtyData(lineId, false), 100);
                }
            });
        }
    }

    getPtyListener(lineId: string) {
        let termWrap = this.getTermWrap(lineId);
        if (termWrap != null) {
            return termWrap;
        }
        let renderer = this.getRenderer(lineId);
        if (renderer != null) {
            return renderer;
        }
        return null;
    }

    receivePtyData(lineId: string, ptyPos: number, data: Uint8Array, reason?: string): void {
        let termWrap = this.getTermWrap(lineId);
        if (termWrap != null) {
            termWrap.receiveData(ptyPos, data, reason);
        }
        let renderer = this.getRenderer(lineId);
        if (renderer != null) {
            renderer.receiveData(ptyPos, data, reason);
        }
    }

    checkFetchPtyData(lineId: string, reload: boolean): Promise<T.PtyDataType> {
        let lineNum = this.getLineNumFromId(lineId);
        if (this.activePtyFetch[lineId]) {
            // console.log("check-fetch", lineNum, "already running");
            return;
        }
        if (reload) {
            this.localPtyOffsetMap[lineId] = 0;
        }
        let ptyOffset = this.localPtyOffsetMap[lineId];
        if (ptyOffset == null) {
            // console.log("check-fetch", lineNum, "no local offset");
            return;
        }
        let remotePtyOffset = this.remotePtyOffsetMap[lineId];
        if (ptyOffset >= remotePtyOffset) {
            // up to date
            return Promise.resolve({ pos: ptyOffset, data: new Uint8Array(0) });
        }
        this.activePtyFetch[lineId] = true;
        let viewKey = WebShareModel.viewKey;
        // console.log("fetch pty", lineNum, "pos=" + ptyOffset);
        let usp = new URLSearchParams({
            screenid: this.screenId,
            viewkey: viewKey,
            lineid: lineId,
            pos: String(ptyOffset),
        });
        let url = new URL(getBaseUrl() + "/webshare/ptydata?" + usp.toString());
        return fetch(url, { method: "GET", mode: "cors", cache: "no-cache" })
            .then((resp) => {
                if (!resp.ok) {
                    throw new Error(
                        sprintf("Bad fetch response for /webshare/ptydata: %d %s", resp.status, resp.statusText)
                    );
                }
                let ptyOffsetStr = resp.headers.get("X-PtyDataOffset");
                if (ptyOffsetStr != null && !isNaN(parseInt(ptyOffsetStr))) {
                    ptyOffset = parseInt(ptyOffsetStr);
                }
                return resp.arrayBuffer();
            })
            .then((buf) => {
                let dataArr = new Uint8Array(buf);
                let newOffset = ptyOffset + dataArr.length;
                // console.log("fetch pty success", lineNum, "len=" + dataArr.length, "pos => " + newOffset);
                this.localPtyOffsetMap[lineId] = newOffset;
                return { pos: ptyOffset, data: dataArr };
            })
            .finally(() => {
                this.activePtyFetch[lineId] = false;
            });
    }

    wsMessageCallback(msg: any) {
        if (msg.type == "webscreen:update") {
            // console.log("[ws] update vts", msg.vts);
            if (msg.vts > this.remoteScreenVts) {
                this.remoteScreenVts = msg.vts;
                setTimeout(() => this.checkUpdateScreenData(), 10);
            }
            return;
        }
        if (msg.type == "success:webshare") {
            return;
        }
        console.log("[ws] unhandled message", msg);
    }

    setWebFullScreen(screen: T.WebFullScreen) {
        // console.log("got initial screen", "vts=" + screen.vts);
        mobx.action(() => {
            if (screen.lines == null) {
                screen.lines = [];
            }
            if (screen.cmds == null) {
                screen.cmds = [];
            }
            this.handleCmdPtyMap(screen.cmdptymap);
            screen.cmdptymap = null;
            this.fullScreen.set(screen);
            this.wsControl.reconnect(true);
            if (this.syncSelectedLine.get()) {
                this.selectedLine.set(screen.screen.selectedline);
            }
        })();
    }

    loadTerminalRenderer(elem: Element, line: T.WebLine, cmd: T.WebCmd, width: number): void {
        let lineId = cmd.lineid;
        let termWrap = this.getTermWrap(lineId);
        if (termWrap != null) {
            console.log("term-wrap already exists for", lineId);
            return;
        }
        let cols = windowWidthToCols(width, this.getTermFontSize());
        let usedRows = this.getContentHeight(lineutil.getWebRendererContext(line));
        if (line.contentheight != null && line.contentheight != -1) {
            usedRows = line.contentheight;
        }
        let termContext = lineutil.getWebRendererContext(line);
        termWrap = new TermWrap(elem, {
            termContext: termContext,
            usedRows: usedRows,
            termOpts: cmd.termopts,
            winSize: { height: 0, width: width },
            dataHandler: null,
            focusHandler: (focus: boolean) => this.setTermFocus(line.linenum, focus),
            isRunning: lineutil.cmdStatusIsRunning(cmd.status),
            customKeyHandler: this.termCustomKeyHandler.bind(this),
            fontSize: this.getTermFontSize(),
            ptyDataSource: getTermPtyData,
            onUpdateContentHeight: (termContext: T.RendererContext, height: number) => {
                this.setContentHeight(termContext, height);
            },
        });
        this.terminals[lineId] = termWrap;
        if (this.localPtyOffsetMap[lineId] == null) {
            this.localPtyOffsetMap[lineId] = 0;
        }
        this.localPtyOffsetMap[lineId] = 0;
        if (this.getSelectedLine() == line.linenum) {
            termWrap.giveFocus();
        }
        return;
    }

    termCustomKeyHandler(e: any, termWrap: TermWrap): boolean {
        if (e.type != "keydown" || isModKeyPress(e)) {
            return false;
        }
        e.stopPropagation();
        e.preventDefault();
        if (e.code == "ArrowUp") {
            termWrap.terminal.scrollLines(-1);
            return false;
        }
        if (e.code == "ArrowDown") {
            termWrap.terminal.scrollLines(1);
            return false;
        }
        if (e.code == "PageUp") {
            termWrap.terminal.scrollPages(-1);
            return false;
        }
        if (e.code == "PageDown") {
            termWrap.terminal.scrollPages(1);
            return false;
        }
        return false;
    }

    setTermFocus(lineNum: number, focus: boolean): void {}

    getContentHeight(context: T.RendererContext): number {
        let key = context.lineId;
        return this.contentHeightCache[key];
    }

    setContentHeight(context: T.RendererContext, height: number): void {
        let key = context.lineId;
        this.contentHeightCache[key] = height;
    }

    unloadRenderer(lineId: string): void {
        let rmodel = this.renderers[lineId];
        if (rmodel != null) {
            rmodel.dispose();
            delete this.renderers[lineId];
        }
        let term = this.terminals[lineId];
        if (term != null) {
            term.dispose();
            delete this.terminals[lineId];
        }
        delete this.localPtyOffsetMap[lineId];
    }

    getUsedRows(context: T.RendererContext, line: T.WebLine, cmd: T.WebCmd, width: number): number {
        if (cmd == null) {
            return 0;
        }
        let termOpts = cmd.termopts;
        if (!termOpts.flexrows) {
            return termOpts.rows;
        }
        let termWrap = this.getTermWrap(cmd.lineid);
        if (termWrap == null) {
            let cols = windowWidthToCols(width, this.getTermFontSize());
            let usedRows = this.getContentHeight(context);
            if (usedRows != null) {
                return usedRows;
            }
            if (line.contentheight != null && line.contentheight != -1) {
                return line.contentheight;
            }
            return lineutil.cmdStatusIsRunning(cmd.status) ? 1 : 0;
        }
        return termWrap.getUsedRows();
    }

    getTermWrap(lineId: string): TermWrap {
        return this.terminals[lineId];
    }

    getRenderer(lineId: string): T.RendererModel {
        return this.renderers[lineId];
    }

    registerRenderer(lineId: string, renderer: T.RendererModel) {
        this.renderers[lineId] = renderer;
        if (this.localPtyOffsetMap[lineId] == null) {
            this.localPtyOffsetMap[lineId] = 0;
        }
    }

    checkUpdateScreenData(): void {
        let fullScreen = this.fullScreen.get();
        if (fullScreen == null) {
            return;
        }
        // console.log("check-update", "vts=" + fullScreen.vts, "remote-vts=" + this.remoteScreenVts);
        if (fullScreen.vts >= this.remoteScreenVts) {
            return;
        }
        this.loadFullScreenData(true);
    }

    loadFullScreenData(update: boolean): void {
        if (isBlank(this.screenId)) {
            this.setErrMessage("No ScreenId Specified, Cannot Load.");
            return;
        }
        if (isBlank(this.viewKey)) {
            this.setErrMessage("No ViewKey Specified, Cannot Load.");
            return;
        }
        if (this.activeUpdateFetch) {
            // console.log("there is already an active update fetch");
            return;
        }
        // console.log("running screen-data update");
        this.activeUpdateFetch = true;
        let urlParams: Record<string, string> = { screenid: this.screenId, viewkey: this.viewKey };
        if (update) {
            let fullScreen = this.fullScreen.get();
            if (fullScreen != null) {
                urlParams.vts = String(fullScreen.vts);
            }
        }
        let usp = new URLSearchParams(urlParams);
        let url = new URL(getBaseUrl() + "/webshare/screen?" + usp.toString());
        fetch(url, { method: "GET", mode: "cors", cache: "no-cache" })
            .then((resp) => handleJsonFetchResponse(url, resp))
            .then((data) => {
                let screen: T.WebFullScreen = data;
                if (update) {
                    this.mergeUpdate(screen);
                } else {
                    this.setWebFullScreen(screen);
                }
                setTimeout(() => this.checkUpdateScreenData(), 300);
            })
            .catch((err) => {
                this.errMessage.set("Cannot get screen: " + err.message);
            })
            .finally(() => {
                this.activeUpdateFetch = false;
            });
    }

    getLineNumFromId(lineId: string): number {
        let fullScreen = this.fullScreen.get();
        if (fullScreen == null) {
            return -1;
        }
        for (let i = 0; i < fullScreen.lines.length; i++) {
            let line = fullScreen.lines[i];
            if (line.lineid == lineId) {
                return line.linenum;
            }
        }
        return -1;
    }

    getLineIndex(lineNum: number): number {
        let fullScreen = this.fullScreen.get();
        if (fullScreen == null) {
            return -1;
        }
        for (let i = 0; i < fullScreen.lines.length; i++) {
            let line = fullScreen.lines[i];
            if (line.linenum == lineNum) {
                return i;
            }
        }
        return -1;
    }

    getNumLines(): number {
        let fullScreen = this.fullScreen.get();
        if (fullScreen == null) {
            return 0;
        }
        return fullScreen.lines.length;
    }

    getCmdById(lineId: string): T.WebCmd {
        let fullScreen = this.fullScreen.get();
        if (fullScreen == null) {
            return null;
        }
        for (let cmd of fullScreen.cmds) {
            if (cmd.lineid == lineId) {
                return cmd;
            }
        }
        return null;
    }

    docKeyDownHandler(e: any): void {
        if (isModKeyPress(e)) {
            return;
        }
        if (e.code == "PageUp" && e.getModifierState("Meta")) {
            this.updateSelectedLineIndex(-1);
        }
        if (e.code == "PageDown" && e.getModifierState("Meta")) {
            this.updateSelectedLineIndex(1);
        }
    }
}

function getTermPtyData(termContext: T.TermContextUnion): Promise<T.PtyDataType> {
    if ("remoteId" in termContext) {
        throw new Error("remote term ptydata is not supported in webshare");
    }
    return WebShareModel.checkFetchPtyData(termContext.lineId, true);
}

let WebShareModel: WebShareModelClass = null;
if ((window as any).WebShareModel == null) {
    WebShareModel = new WebShareModelClass();
    (window as any).WebShareModel = WebShareModel;
}

export { WebShareModel, getTermPtyData };
