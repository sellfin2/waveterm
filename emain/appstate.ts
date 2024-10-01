import { getWebServerEndpoint } from "@/util/endpoints";
import { BrowserWindow } from "electron";

class AppStateType {
    isQuitting: boolean;
    isStarting: boolean;
    isRelaunching: boolean;

    wasInFg: boolean;
    wasActive: boolean;

    constructor() {
        this.isQuitting = false;
        this.isStarting = false;
        this.isRelaunching = false;
        this.wasInFg = false;
        this.wasActive = false;
    }

    async logActiveState() {
        const activeState = { fg: this.wasInFg, active: this.wasActive, open: true };
        const url = new URL(getWebServerEndpoint() + "/wave/log-active-state");
        try {
            const resp = await fetch(url, { method: "post", body: JSON.stringify(activeState) });
            if (!resp.ok) {
                console.log("error logging active state", resp.status, resp.statusText);
                return;
            }
        } catch (e) {
            console.log("error logging active state", e);
        } finally {
            // for next iteration
            this.wasInFg = BrowserWindow.getFocusedWindow()?.isFocused() ?? false;
            this.wasActive = false;
        }
    }

    runActiveTimer() {
        this.logActiveState().then(() => setTimeout(this.runActiveTimer, 60000));
    }
}

export const AppState = new AppStateType();
