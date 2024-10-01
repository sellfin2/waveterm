// Copyright 2024, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as electron from "electron";
import fs from "fs";
import * as child_process from "node:child_process";
import * as path from "path";
import * as readline from "readline";
import { sprintf } from "sprintf-js";
import * as util from "util";
import winston from "winston";
import { initGlobal } from "../frontend/app/store/global";
import * as services from "../frontend/app/store/services";
import { initElectronWshrpc } from "../frontend/app/store/wshrpcutil";
import { WSServerEndpointVarName, WebServerEndpointVarName, getWebServerEndpoint } from "../frontend/util/endpoints";
import * as keyutil from "../frontend/util/keyutil";
import { AppState } from "./appstate";
import { AuthKey, AuthKeyEnv, configureAuthKeyRequestInjection } from "./authkey";
import { ElectronWshClient, initElectronWshClient } from "./emain-wsh";
import { configureGlobalHotkey } from "./globalhotkey";
import { getLaunchSettings } from "./launchsettings";
import { getAppMenu } from "./menu";
import {
    getElectronAppBasePath,
    getGoAppBasePath,
    getWaveHomeDir,
    getWaveSrvCwd,
    getWaveSrvPath,
    isDev,
    unameArch,
    unamePlatform,
} from "./platform";
import { configureAutoUpdater, updater } from "./updater";
import { createBrowserWindow, createNewWaveWindow, relaunchBrowserWindows } from "./window";

const electronApp = electron.app;
let WaveVersion = "unknown"; // set by WAVESRV-ESTART
let WaveBuildTime = 0; // set by WAVESRV-ESTART

const WaveAppPathVarName = "WAVETERM_APP_PATH";
const WaveSrvReadySignalPidVarName = "WAVETERM_READY_SIGNAL_PID";
electron.nativeTheme.themeSource = "dark";

let waveSrvReadyResolve = (value: boolean) => {};
const waveSrvReady: Promise<boolean> = new Promise((resolve, _) => {
    waveSrvReadyResolve = resolve;
});

// for activity updates
let wasActive = true;
let wasInFg = true;

let webviewFocusId: number = null; // set to the getWebContentsId of the webview that has focus (null if not focused)
let webviewKeys: string[] = []; // the keys to trap when webview has focus

let waveSrvProc: child_process.ChildProcessWithoutNullStreams | null = null;

const waveHome = getWaveHomeDir();

const oldConsoleLog = console.log;

const loggerTransports: winston.transport[] = [
    new winston.transports.File({ filename: path.join(getWaveHomeDir(), "waveapp.log"), level: "info" }),
];
if (isDev) {
    loggerTransports.push(new winston.transports.Console());
}
const loggerConfig = {
    level: "info",
    format: winston.format.combine(
        winston.format.timestamp({ format: "YYYY-MM-DD HH:mm:ss" }),
        winston.format.printf((info) => `${info.timestamp} ${info.message}`)
    ),
    transports: loggerTransports,
};
const logger = winston.createLogger(loggerConfig);
function log(...msg: any[]) {
    try {
        logger.info(util.format(...msg));
    } catch (e) {
        oldConsoleLog(...msg);
    }
}
console.log = log;
console.log(
    sprintf(
        "waveterm-app starting, WAVETERM_HOME=%s, electronpath=%s gopath=%s arch=%s/%s",
        waveHome,
        getElectronAppBasePath(),
        getGoAppBasePath(),
        unamePlatform,
        unameArch
    )
);
if (isDev) {
    console.log("waveterm-app WAVETERM_DEV set");
}

initGlobal({ windowId: null, clientId: null, platform: unamePlatform, environment: "electron" });

function setCtrlShift(wc: Electron.WebContents, state: boolean) {
    wc.send("control-shift-state-update", state);
}

function handleCtrlShiftState(sender: Electron.WebContents, waveEvent: WaveKeyboardEvent) {
    if (waveEvent.type == "keyup") {
        if (waveEvent.key === "Control" || waveEvent.key === "Shift") {
            setCtrlShift(sender, false);
        }
        if (waveEvent.key == "Meta") {
            if (waveEvent.control && waveEvent.shift) {
                setCtrlShift(sender, true);
            }
        }
        return;
    }
    if (waveEvent.type == "keydown") {
        if (waveEvent.key === "Control" || waveEvent.key === "Shift" || waveEvent.key === "Meta") {
            if (waveEvent.control && waveEvent.shift && !waveEvent.meta) {
                // Set the control and shift without the Meta key
                setCtrlShift(sender, true);
            } else {
                // Unset if Meta is pressed
                setCtrlShift(sender, false);
            }
        }
        return;
    }
}

function handleCtrlShiftFocus(sender: Electron.WebContents, focused: boolean) {
    if (!focused) {
        setCtrlShift(sender, false);
    }
}

function runWaveSrv(): Promise<boolean> {
    let pResolve: (value: boolean) => void;
    let pReject: (reason?: any) => void;
    const rtnPromise = new Promise<boolean>((argResolve, argReject) => {
        pResolve = argResolve;
        pReject = argReject;
    });
    const envCopy = { ...process.env };
    envCopy[WaveAppPathVarName] = getGoAppBasePath();
    envCopy[WaveSrvReadySignalPidVarName] = process.pid.toString();
    envCopy[AuthKeyEnv] = AuthKey;
    const waveSrvCmd = getWaveSrvPath();
    console.log("trying to run local server", waveSrvCmd);
    const proc = child_process.spawn(getWaveSrvPath(), {
        cwd: getWaveSrvCwd(),
        env: envCopy,
    });
    proc.on("exit", (e) => {
        if (AppState.isQuitting || updater?.status == "installing") {
            return;
        }
        console.log("wavesrv exited, shutting down");
        electronApp.quit();
    });
    proc.on("spawn", (e) => {
        console.log("spawned wavesrv");
        waveSrvProc = proc;
        pResolve(true);
    });
    proc.on("error", (e) => {
        console.log("error running wavesrv", e);
        pReject(e);
    });
    const rlStdout = readline.createInterface({
        input: proc.stdout,
        terminal: false,
    });
    rlStdout.on("line", (line) => {
        console.log(line);
    });
    const rlStderr = readline.createInterface({
        input: proc.stderr,
        terminal: false,
    });
    rlStderr.on("line", (line) => {
        if (line.includes("WAVESRV-ESTART")) {
            const startParams = /ws:([a-z0-9.:]+) web:([a-z0-9.:]+) version:([a-z0-9.\-]+) buildtime:(\d+)/gm.exec(
                line
            );
            if (startParams == null) {
                console.log("error parsing WAVESRV-ESTART line", line);
                electronApp.quit();
                return;
            }
            process.env[WSServerEndpointVarName] = startParams[1];
            process.env[WebServerEndpointVarName] = startParams[2];
            WaveVersion = startParams[3];
            WaveBuildTime = parseInt(startParams[4]);
            waveSrvReadyResolve(true);
            return;
        }
        if (line.startsWith("WAVESRV-EVENT:")) {
            const evtJson = line.slice("WAVESRV-EVENT:".length);
            try {
                const evtMsg: WSEventType = JSON.parse(evtJson);
                handleWSEvent(evtMsg);
            } catch (e) {
                console.log("error handling WAVESRV-EVENT", e);
            }
            return;
        }
        console.log(line);
    });
    return rtnPromise;
}

async function handleWSEvent(evtMsg: WSEventType) {
    console.log("handleWSEvent", evtMsg?.eventtype);
    if (evtMsg.eventtype == "electron:newwindow") {
        const windowId: string = evtMsg.data;
        const windowData: WaveWindow = (await services.ObjectService.GetObject("window:" + windowId)) as WaveWindow;
        if (windowData == null) {
            return;
        }
        const clientData = await services.ClientService.GetClientData();
        const fullConfig = await services.FileService.GetFullConfig();
        const newWin = createBrowserWindow(clientData.oid, windowData, fullConfig);
        await newWin.readyPromise;
        newWin.show();
    } else if (evtMsg.eventtype == "electron:closewindow") {
        if (evtMsg.data === undefined) return;
        const windows = electron.BrowserWindow.getAllWindows();
        for (const window of windows) {
            if ((window as any).waveWindowId === evtMsg.data) {
                // Bypass the "Are you sure?" dialog, since this event is called when there's no more tabs for the window.
                window.destroy();
            }
        }
    } else {
        console.log("unhandled electron ws eventtype", evtMsg.eventtype);
    }
}

// Listen for the open-external event from the renderer process
electron.ipcMain.on("open-external", (event, url) => {
    if (url && typeof url === "string") {
        electron.shell.openExternal(url).catch((err) => {
            console.error(`Failed to open URL ${url}:`, err);
        });
    } else {
        console.error("Invalid URL received in open-external event:", url);
    }
});

electron.ipcMain.on("download", (event, payload) => {
    const window = electron.BrowserWindow.fromWebContents(event.sender);
    const streamingUrl = getWebServerEndpoint() + "/wave/stream-file?path=" + encodeURIComponent(payload.filePath);
    window.webContents.downloadURL(streamingUrl);
});

electron.ipcMain.on("get-cursor-point", (event) => {
    const window = electron.BrowserWindow.fromWebContents(event.sender);
    const screenPoint = electron.screen.getCursorScreenPoint();
    const windowRect = window.getContentBounds();
    const retVal: Electron.Point = {
        x: screenPoint.x - windowRect.x,
        y: screenPoint.y - windowRect.y,
    };
    event.returnValue = retVal;
});

electron.ipcMain.on("get-env", (event, varName) => {
    event.returnValue = process.env[varName] ?? null;
});

electron.ipcMain.on("get-about-modal-details", (event) => {
    event.returnValue = { version: WaveVersion, buildTime: WaveBuildTime } as AboutModalDetails;
});

const hasBeforeInputRegisteredMap = new Map<number, boolean>();

electron.ipcMain.on("webview-focus", (event: Electron.IpcMainEvent, focusedId: number) => {
    webviewFocusId = focusedId;
    console.log("webview-focus", focusedId);
    if (focusedId == null) {
        return;
    }
    const parentWc = event.sender;
    const webviewWc = electron.webContents.fromId(focusedId);
    if (webviewWc == null) {
        webviewFocusId = null;
        return;
    }
    if (!hasBeforeInputRegisteredMap.get(focusedId)) {
        hasBeforeInputRegisteredMap.set(focusedId, true);
        webviewWc.on("before-input-event", (e, input) => {
            let waveEvent = keyutil.adaptFromElectronKeyEvent(input);
            // console.log(`WEB ${focusedId}`, waveEvent.type, waveEvent.code);
            handleCtrlShiftState(parentWc, waveEvent);
            if (webviewFocusId != focusedId) {
                return;
            }
            if (input.type != "keyDown") {
                return;
            }
            for (let keyDesc of webviewKeys) {
                if (keyutil.checkKeyPressed(waveEvent, keyDesc)) {
                    e.preventDefault();
                    parentWc.send("reinject-key", waveEvent);
                    console.log("webview reinject-key", keyDesc);
                    return;
                }
            }
        });
        webviewWc.on("destroyed", () => {
            hasBeforeInputRegisteredMap.delete(focusedId);
        });
    }
});

electron.ipcMain.on("register-global-webview-keys", (event, keys: string[]) => {
    webviewKeys = keys ?? [];
});

electron.ipcMain.on("contextmenu-show", (event, menuDefArr?: ElectronContextMenuItem[]) => {
    const window = electron.BrowserWindow.fromWebContents(event.sender);
    if (menuDefArr?.length === 0) {
        return;
    }
    const menu = menuDefArr ? convertMenuDefArrToMenu(menuDefArr) : instantiateAppMenu();
    const { x, y } = electron.screen.getCursorScreenPoint();
    const windowPos = window.getPosition();

    menu.popup({ window, x: x - windowPos[0], y: y - windowPos[1] });
    event.returnValue = true;
});

function convertMenuDefArrToMenu(menuDefArr: ElectronContextMenuItem[]): electron.Menu {
    const menuItems: electron.MenuItem[] = [];
    for (const menuDef of menuDefArr) {
        const menuItemTemplate: electron.MenuItemConstructorOptions = {
            role: menuDef.role as any,
            label: menuDef.label,
            type: menuDef.type,
            click: (_, window) => {
                (window as electron.BrowserWindow)?.webContents?.send("contextmenu-click", menuDef.id);
            },
            checked: menuDef.checked,
        };
        if (menuDef.submenu != null) {
            menuItemTemplate.submenu = convertMenuDefArrToMenu(menuDef.submenu);
        }
        const menuItem = new electron.MenuItem(menuItemTemplate);
        menuItems.push(menuItem);
    }
    return electron.Menu.buildFromTemplate(menuItems);
}

function instantiateAppMenu(): electron.Menu {
    return getAppMenu({ createNewWaveWindow, relaunchBrowserWindows });
}

function makeAppMenu() {
    const menu = instantiateAppMenu();
    electron.Menu.setApplicationMenu(menu);
}

electronApp.on("window-all-closed", () => {
    if (AppState.isRelaunching) {
        return;
    }
    if (unamePlatform !== "darwin") {
        electronApp.quit();
    }
});
electronApp.on("before-quit", () => {
    AppState.isQuitting = true;
    updater?.stop();
});
process.on("SIGINT", () => {
    console.log("Caught SIGINT, shutting down");
    electronApp.quit();
});
process.on("SIGHUP", () => {
    console.log("Caught SIGHUP, shutting down");
    electronApp.quit();
});
process.on("SIGTERM", () => {
    console.log("Caught SIGTERM, shutting down");
    electronApp.quit();
});
let caughtException = false;
process.on("uncaughtException", (error) => {
    if (caughtException) {
        return;
    }
    logger.error("Uncaught Exception, shutting down: ", error);
    caughtException = true;
    // Optionally, handle cleanup or exit the app
    electronApp.quit();
});

process.on("uncaughtException", (error) => {
    console.error("Uncaught Exception:", error);
    console.error("Stack Trace:", error.stack);
    electron.app.quit();
});

async function appMain() {
    // Set disableHardwareAcceleration as early as possible, if required.
    const launchSettings = getLaunchSettings();
    if (launchSettings?.["window:disablehardwareacceleration"]) {
        console.log("disabling hardware acceleration, per launch settings");
        electronApp.disableHardwareAcceleration();
    }

    const startTs = Date.now();
    const instanceLock = electronApp.requestSingleInstanceLock();
    if (!instanceLock) {
        console.log("waveterm-app could not get single-instance-lock, shutting down");
        electronApp.quit();
        return;
    }
    const waveHomeDir = getWaveHomeDir();
    if (!fs.existsSync(waveHomeDir)) {
        fs.mkdirSync(waveHomeDir);
    }
    makeAppMenu();
    try {
        await runWaveSrv();
    } catch (e) {
        console.log(e.toString());
    }
    const ready = await waveSrvReady;
    console.log("wavesrv ready signal received", ready, Date.now() - startTs, "ms");
    await electronApp.whenReady();
    configureAuthKeyRequestInjection(electron.session.defaultSession);
    await relaunchBrowserWindows();
    await configureGlobalHotkey();
    setTimeout(AppState.runActiveTimer, 5000); // start active timer, wait 5s just to be safe
    try {
        initElectronWshClient();
        initElectronWshrpc(ElectronWshClient, { authKey: AuthKey });
    } catch (e) {
        console.log("error initializing wshrpc", e);
    }
    await configureAutoUpdater();

    AppState.isStarting = false;

    electronApp.on("activate", async () => {
        if (electron.BrowserWindow.getAllWindows().length === 0) {
            await createNewWaveWindow();
        }
    });
}

appMain().catch((e) => {
    console.log("appMain error", e);
    electronApp.quit();
});
