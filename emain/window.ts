import { ClientService, FileService, ObjectService, WindowService } from "@/app/store/services";
import { getWebServerEndpoint } from "@/util/endpoints";
import { adaptFromElectronKeyEvent } from "@/util/keyutil";
import { fireAndForget } from "@/util/util";
import {
    BrowserWindow,
    BrowserWindowConstructorOptions,
    dialog,
    Display,
    ipcMain,
    Rectangle,
    screen,
    shell,
} from "electron";
import { FastAverageColor } from "fast-average-color";
import path from "node:path";
import { PNG } from "pngjs";
import { debounce } from "throttle-debounce";
import { AppState } from "./appstate";
import { configureAuthKeyRequestInjection } from "./authkey";
import { getElectronAppBasePath, isDevVite, unamePlatform } from "./platform";
import { updater } from "./updater";

type WaveBrowserWindow = Electron.BrowserWindow & { waveWindowId: string; readyPromise: Promise<void> };

// note, this does not *show* the window.
// to show, await win.readyPromise and then win.show()
export function createBrowserWindow(
    clientId: string,
    waveWindow: WaveWindow,
    fullConfig: FullConfigType
): WaveBrowserWindow {
    let winWidth = waveWindow?.winsize?.width;
    let winHeight = waveWindow?.winsize?.height;
    let winPosX = waveWindow.pos.x;
    let winPosY = waveWindow.pos.y;
    if (winWidth == null || winWidth == 0) {
        const primaryDisplay = screen.getPrimaryDisplay();
        const { width } = primaryDisplay.workAreaSize;
        winWidth = width - winPosX - 100;
        if (winWidth > 2000) {
            winWidth = 2000;
        }
    }
    if (winHeight == null || winHeight == 0) {
        const primaryDisplay = screen.getPrimaryDisplay();
        const { height } = primaryDisplay.workAreaSize;
        winHeight = height - winPosY - 100;
        if (winHeight > 1200) {
            winHeight = 1200;
        }
    }
    let winBounds = {
        x: winPosX,
        y: winPosY,
        width: winWidth,
        height: winHeight,
    };
    winBounds = ensureBoundsAreVisible(winBounds);
    const settings = fullConfig?.settings;
    const winOpts: BrowserWindowConstructorOptions = {
        titleBarStyle:
            unamePlatform === "darwin" ? "hiddenInset" : settings["window:nativetitlebar"] ? "default" : "hidden",
        titleBarOverlay:
            unamePlatform !== "darwin"
                ? {
                      symbolColor: "white",
                      color: "#00000000",
                  }
                : false,
        x: winBounds.x,
        y: winBounds.y,
        width: winBounds.width,
        height: winBounds.height,
        minWidth: 400,
        minHeight: 300,
        icon:
            unamePlatform == "linux"
                ? path.join(getElectronAppBasePath(), "public/logos/wave-logo-dark.png")
                : undefined,
        webPreferences: {
            preload: path.join(getElectronAppBasePath(), "preload", "index.cjs"),
            webviewTag: true,
        },
        show: false,
        autoHideMenuBar: true,
    };
    const isTransparent = settings?.["window:transparent"] ?? false;
    const isBlur = !isTransparent && (settings?.["window:blur"] ?? false);
    if (isTransparent) {
        winOpts.transparent = true;
    } else if (isBlur) {
        switch (unamePlatform) {
            case "win32": {
                winOpts.backgroundMaterial = "acrylic";
                break;
            }
            case "darwin": {
                winOpts.vibrancy = "fullscreen-ui";
                break;
            }
        }
    } else {
        winOpts.backgroundColor = "#222222";
    }
    const bwin = new BrowserWindow(winOpts);
    (bwin as any).waveWindowId = waveWindow.oid;
    let readyResolve: (value: void) => void;
    (bwin as any).readyPromise = new Promise((resolve, _) => {
        readyResolve = resolve;
    });
    const win: WaveBrowserWindow = bwin as WaveBrowserWindow;
    const usp = new URLSearchParams();
    usp.set("clientid", clientId);
    usp.set("windowid", waveWindow.oid);
    const indexHtml = "index.html";
    if (isDevVite) {
        console.log("running as dev server");
        win.loadURL(`${process.env.ELECTRON_RENDERER_URL}/index.html?${usp.toString()}`);
    } else {
        console.log("running as file");
        win.loadFile(path.join(getElectronAppBasePath(), "frontend", indexHtml), { search: usp.toString() });
    }
    win.once("ready-to-show", () => {
        readyResolve();
    });
    win.webContents.on("will-navigate", shNavHandler);
    win.webContents.on("will-frame-navigate", shFrameNavHandler);
    win.webContents.on("did-attach-webview", (event, wc) => {
        wc.setWindowOpenHandler((details) => {
            win.webContents.send("webview-new-window", wc.id, details);
            return { action: "deny" };
        });
    });
    win.webContents.on("before-input-event", (e, input) => {
        const waveEvent = adaptFromElectronKeyEvent(input);
        // console.log("WIN bie", waveEvent.type, waveEvent.code);
        handleCtrlShiftState(win.webContents, waveEvent);
        if (win.isFocused()) {
            AppState.wasActive = true;
        }
    });
    win.on(
        "resize",
        debounce(400, (e) => mainResizeHandler(e, waveWindow.oid, win))
    );
    win.on(
        "move",
        debounce(400, (e) => mainResizeHandler(e, waveWindow.oid, win))
    );
    win.on("focus", () => {
        AppState.wasInFg = true;
        AppState.wasActive = true;
        if (AppState.isStarting) {
            return;
        }
        console.log("focus", waveWindow.oid);
        ClientService.FocusWindow(waveWindow.oid);
    });
    win.on("blur", () => {
        handleCtrlShiftFocus(win.webContents, false);
    });
    win.on("enter-full-screen", async () => {
        win.webContents.send("fullscreen-change", true);
    });
    win.on("leave-full-screen", async () => {
        win.webContents.send("fullscreen-change", false);
    });
    win.on("close", (e) => {
        if (AppState.isQuitting || updater?.status == "installing") {
            return;
        }
        const numWindows = BrowserWindow.getAllWindows().length;
        if (numWindows == 1) {
            return;
        }
        const choice = dialog.showMessageBoxSync(win, {
            type: "question",
            buttons: ["Cancel", "Yes"],
            title: "Confirm",
            message: "Are you sure you want to close this window (all tabs and blocks will be deleted)?",
        });
        if (choice === 0) {
            e.preventDefault();
        }
    });
    win.on("closed", () => {
        if (AppState.isQuitting || updater?.status == "installing") {
            return;
        }
        const numWindows = BrowserWindow.getAllWindows().length;
        if (numWindows == 0) {
            return;
        }
        WindowService.CloseWindow(waveWindow.oid);
    });
    win.webContents.on("zoom-changed", (e) => {
        win.webContents.send("zoom-changed");
    });
    win.webContents.setWindowOpenHandler(({ url, frameName }) => {
        if (url.startsWith("http://") || url.startsWith("https://") || url.startsWith("file://")) {
            console.log("openExternal fallback", url);
            shell.openExternal(url);
        }
        console.log("window-open denied", url);
        return { action: "deny" };
    });
    configureAuthKeyRequestInjection(win.webContents.session);
    return win;
}

export async function createNewWaveWindow(): Promise<void> {
    const clientData = await ClientService.GetClientData();
    const fullConfig = await FileService.GetFullConfig();
    let recreatedWindow = false;
    if (BrowserWindow.getAllWindows().length === 0 && clientData?.windowids?.length >= 1) {
        // reopen the first window
        const existingWindowId = clientData.windowids[0];
        const existingWindowData = (await ObjectService.GetObject("window:" + existingWindowId)) as WaveWindow;
        if (existingWindowData != null) {
            const win = createBrowserWindow(clientData.oid, existingWindowData, fullConfig);
            await win.readyPromise;
            win.show();
            recreatedWindow = true;
        }
    }
    if (recreatedWindow) {
        return;
    }
    const newWindow = await ClientService.MakeWindow();
    const newBrowserWindow = createBrowserWindow(clientData.oid, newWindow, fullConfig);
    await newBrowserWindow.readyPromise;
    newBrowserWindow.show();
}

export async function relaunchBrowserWindows(): Promise<void> {
    AppState.isRelaunching = true;
    const windows = BrowserWindow.getAllWindows();
    for (const window of windows) {
        window.removeAllListeners();
        window.close();
    }
    AppState.isRelaunching = false;

    const clientData = await ClientService.GetClientData();
    const fullConfig = await FileService.GetFullConfig();
    const wins: WaveBrowserWindow[] = [];
    for (const windowId of clientData.windowids.slice().reverse()) {
        const windowData: WaveWindow = (await ObjectService.GetObject("window:" + windowId)) as WaveWindow;
        if (windowData == null) {
            WindowService.CloseWindow(windowId).catch((e) => {
                /* ignore */
            });
            continue;
        }
        const win = createBrowserWindow(clientData.oid, windowData, fullConfig);
        wins.push(win);
    }
    for (const win of wins) {
        await win.readyPromise;
        console.log("show", win.waveWindowId);
        win.show();
    }
}

async function mainResizeHandler(_: any, windowId: string, win: WaveBrowserWindow) {
    if (win == null || win.isDestroyed() || win.fullScreen) {
        return;
    }
    const bounds = win.getBounds();
    try {
        await WindowService.SetWindowPosAndSize(
            windowId,
            { x: bounds.x, y: bounds.y },
            { width: bounds.width, height: bounds.height }
        );
    } catch (e) {
        console.log("error resizing window", e);
    }
}

ipcMain.on("open-new-window", () => fireAndForget(createNewWaveWindow));

function shNavHandler(event: Electron.Event<Electron.WebContentsWillNavigateEventParams>, url: string) {
    if (url.startsWith("http://127.0.0.1:5173/index.html") || url.startsWith("http://localhost:5173/index.html")) {
        // this is a dev-mode hot-reload, ignore it
        console.log("allowing hot-reload of index.html");
        return;
    }
    event.preventDefault();
    if (url.startsWith("https://") || url.startsWith("http://") || url.startsWith("file://")) {
        console.log("open external, shNav", url);
        shell.openExternal(url);
    } else {
        console.log("navigation canceled", url);
    }
}

function shFrameNavHandler(event: Electron.Event<Electron.WebContentsWillFrameNavigateEventParams>) {
    if (!event.frame?.parent) {
        // only use this handler to process iframe events (non-iframe events go to shNavHandler)
        return;
    }
    const url = event.url;
    console.log(`frame-navigation url=${url} frame=${event.frame.name}`);
    if (event.frame.name == "webview") {
        // "webview" links always open in new window
        // this will *not* effect the initial load because srcdoc does not count as an electron navigation
        console.log("open external, frameNav", url);
        event.preventDefault();
        shell.openExternal(url);
        return;
    }
    if (
        event.frame.name == "pdfview" &&
        (url.startsWith("blob:file:///") || url.startsWith(getWebServerEndpoint() + "/wave/stream-file?"))
    ) {
        // allowed
        return;
    }
    event.preventDefault();
    console.log("frame navigation canceled");
}

function isWindowFullyVisible(bounds: Rectangle): boolean {
    const displays = screen.getAllDisplays();

    // Helper function to check if a point is inside any display
    function isPointInDisplay(x: number, y: number) {
        for (const display of displays) {
            const { x: dx, y: dy, width, height } = display.bounds;
            if (x >= dx && x < dx + width && y >= dy && y < dy + height) {
                return true;
            }
        }
        return false;
    }

    // Check all corners of the window
    const topLeft = isPointInDisplay(bounds.x, bounds.y);
    const topRight = isPointInDisplay(bounds.x + bounds.width, bounds.y);
    const bottomLeft = isPointInDisplay(bounds.x, bounds.y + bounds.height);
    const bottomRight = isPointInDisplay(bounds.x + bounds.width, bounds.y + bounds.height);

    return topLeft && topRight && bottomLeft && bottomRight;
}

function findDisplayWithMostArea(bounds: Rectangle): Display {
    const displays = screen.getAllDisplays();
    let maxArea = 0;
    let bestDisplay = null;

    for (let display of displays) {
        const { x, y, width, height } = display.bounds;
        const overlapX = Math.max(0, Math.min(bounds.x + bounds.width, x + width) - Math.max(bounds.x, x));
        const overlapY = Math.max(0, Math.min(bounds.y + bounds.height, y + height) - Math.max(bounds.y, y));
        const overlapArea = overlapX * overlapY;

        if (overlapArea > maxArea) {
            maxArea = overlapArea;
            bestDisplay = display;
        }
    }

    return bestDisplay;
}

function adjustBoundsToFitDisplay(bounds: Rectangle, display: Display): Rectangle {
    const { x: dx, y: dy, width: dWidth, height: dHeight } = display.workArea;
    let { x, y, width, height } = bounds;

    // Adjust width and height to fit within the display's work area
    width = Math.min(width, dWidth);
    height = Math.min(height, dHeight);

    // Adjust x to ensure the window fits within the display
    if (x < dx) {
        x = dx;
    } else if (x + width > dx + dWidth) {
        x = dx + dWidth - width;
    }

    // Adjust y to ensure the window fits within the display
    if (y < dy) {
        y = dy;
    } else if (y + height > dy + dHeight) {
        y = dy + dHeight - height;
    }
    return { x, y, width, height };
}

function ensureBoundsAreVisible(bounds: Rectangle): Rectangle {
    if (!isWindowFullyVisible(bounds)) {
        let targetDisplay = findDisplayWithMostArea(bounds);

        if (!targetDisplay) {
            targetDisplay = screen.getPrimaryDisplay();
        }

        return adjustBoundsToFitDisplay(bounds, targetDisplay);
    }
    return bounds;
}

if (unamePlatform !== "darwin") {
    const fac = new FastAverageColor();

    ipcMain.on("update-window-controls-overlay", async (event, rect: Dimensions) => {
        // Bail out if the user requests the native titlebar
        const fullConfig = await FileService.GetFullConfig();
        if (fullConfig.settings["window:nativetitlebar"]) return;

        const zoomFactor = event.sender.getZoomFactor();
        const electronRect: Rectangle = {
            x: rect.left * zoomFactor,
            y: rect.top * zoomFactor,
            height: rect.height * zoomFactor,
            width: rect.width * zoomFactor,
        };
        const overlay = await event.sender.capturePage(electronRect);
        const overlayBuffer = overlay.toPNG();
        const png = PNG.sync.read(overlayBuffer);
        const color = fac.prepareResult(fac.getColorFromArray4(png.data));
        const window = BrowserWindow.fromWebContents(event.sender);
        window.setTitleBarOverlay({
            color: unamePlatform === "linux" ? color.rgba : "#00000000", // Windows supports a true transparent overlay, so we don't need to set a background color.
            symbolColor: color.isDark ? "white" : "black",
        });
    });
}
