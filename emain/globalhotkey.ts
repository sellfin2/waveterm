import { FileService } from "@/app/store/services";
import { fireAndForget } from "@/util/util";
import { app, globalShortcut } from "electron";

async function hotkeyCallback() {
    app.focus();
}

export async function configureGlobalHotkey() {
    const settings = (await FileService.GetFullConfig())?.settings;

    const globalhotkey = settings["window:globalhotkey"];
    if (globalhotkey) {
        console.log(`Registering global hotkey: "${globalhotkey}"`);

        await app.whenReady();
        globalShortcut.register(globalhotkey, () => fireAndForget(() => hotkeyCallback()));
    }

    app.on("before-quit", () => globalShortcut.unregisterAll());
}
