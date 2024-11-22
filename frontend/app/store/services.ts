// Copyright 2024, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// generated by cmd/generate/main-generatets.go

import * as WOS from "./wos";

// blockservice.BlockService (block)
class BlockServiceType {
    GetControllerStatus(arg2: string): Promise<BlockControllerRuntimeStatus> {
        return WOS.callBackendService("block", "GetControllerStatus", Array.from(arguments))
    }

    // save the terminal state to a blockfile
    SaveTerminalState(blockId: string, state: string, stateType: string, ptyOffset: number, termSize: TermSize): Promise<void> {
        return WOS.callBackendService("block", "SaveTerminalState", Array.from(arguments))
    }
    SaveWaveAiData(arg2: string, arg3: OpenAIPromptMessageType[]): Promise<void> {
        return WOS.callBackendService("block", "SaveWaveAiData", Array.from(arguments))
    }
}

export const BlockService = new BlockServiceType();

// clientservice.ClientService (client)
class ClientServiceType {
    // @returns object updates
    AgreeTos(): Promise<void> {
        return WOS.callBackendService("client", "AgreeTos", Array.from(arguments))
    }
    FocusWindow(arg2: string): Promise<void> {
        return WOS.callBackendService("client", "FocusWindow", Array.from(arguments))
    }
    GetAllConnStatus(): Promise<ConnStatus[]> {
        return WOS.callBackendService("client", "GetAllConnStatus", Array.from(arguments))
    }
    GetClientData(): Promise<Client> {
        return WOS.callBackendService("client", "GetClientData", Array.from(arguments))
    }
    GetTab(arg1: string): Promise<Tab> {
        return WOS.callBackendService("client", "GetTab", Array.from(arguments))
    }
    TelemetryUpdate(arg2: boolean): Promise<void> {
        return WOS.callBackendService("client", "TelemetryUpdate", Array.from(arguments))
    }
}

export const ClientService = new ClientServiceType();

// fileservice.FileService (file)
class FileServiceType {
    // delete file
    DeleteFile(connection: string, path: string): Promise<void> {
        return WOS.callBackendService("file", "DeleteFile", Array.from(arguments))
    }
    GetFullConfig(): Promise<FullConfigType> {
        return WOS.callBackendService("file", "GetFullConfig", Array.from(arguments))
    }
    GetWaveFile(arg1: string, arg2: string): Promise<any> {
        return WOS.callBackendService("file", "GetWaveFile", Array.from(arguments))
    }

    // read file
    ReadFile(connection: string, path: string): Promise<FullFile> {
        return WOS.callBackendService("file", "ReadFile", Array.from(arguments))
    }

    // save file
    SaveFile(connection: string, path: string, data64: string): Promise<void> {
        return WOS.callBackendService("file", "SaveFile", Array.from(arguments))
    }

    // get file info
    StatFile(connection: string, path: string): Promise<FileInfo> {
        return WOS.callBackendService("file", "StatFile", Array.from(arguments))
    }
}

export const FileService = new FileServiceType();

// objectservice.ObjectService (object)
class ObjectServiceType {
    // @returns blockId (and object updates)
    CreateBlock(blockDef: BlockDef, rtOpts: RuntimeOpts): Promise<string> {
        return WOS.callBackendService("object", "CreateBlock", Array.from(arguments))
    }

    // @returns object updates
    DeleteBlock(blockId: string): Promise<void> {
        return WOS.callBackendService("object", "DeleteBlock", Array.from(arguments))
    }

    // get wave object by oref
    GetObject(oref: string): Promise<WaveObj> {
        return WOS.callBackendService("object", "GetObject", Array.from(arguments))
    }

    // @returns objects
    GetObjects(orefs: string[]): Promise<WaveObj[]> {
        return WOS.callBackendService("object", "GetObjects", Array.from(arguments))
    }

    // @returns object updates
    UpdateObject(waveObj: WaveObj, returnUpdates: boolean): Promise<void> {
        return WOS.callBackendService("object", "UpdateObject", Array.from(arguments))
    }

    // @returns object updates
    UpdateObjectMeta(oref: string, meta: MetaType): Promise<void> {
        return WOS.callBackendService("object", "UpdateObjectMeta", Array.from(arguments))
    }

    // @returns object updates
    UpdateTabName(tabId: string, name: string): Promise<void> {
        return WOS.callBackendService("object", "UpdateTabName", Array.from(arguments))
    }
}

export const ObjectService = new ObjectServiceType();

// userinputservice.UserInputService (userinput)
class UserInputServiceType {
    SendUserInputResponse(arg1: UserInputResponse): Promise<void> {
        return WOS.callBackendService("userinput", "SendUserInputResponse", Array.from(arguments))
    }
}

export const UserInputService = new UserInputServiceType();

// windowservice.WindowService (window)
class WindowServiceType {
    CloseWindow(fromElectron: string, arg3: boolean): Promise<void> {
        return WOS.callBackendService("window", "CloseWindow", Array.from(arguments))
    }
    GetWindow(arg1: string): Promise<WaveWindow> {
        return WOS.callBackendService("window", "GetWindow", Array.from(arguments))
    }
    MakeWindow(): Promise<WaveWindow> {
        return WOS.callBackendService("window", "MakeWindow", Array.from(arguments))
    }

    // move block to new window
    // @returns object updates
    MoveBlockToNewWindow(currentTabId: string, blockId: string): Promise<void> {
        return WOS.callBackendService("window", "MoveBlockToNewWindow", Array.from(arguments))
    }

    // set window position and size
    // @returns object updates
    SetWindowPosAndSize(pos: string, size: Point, arg4: WinSize): Promise<void> {
        return WOS.callBackendService("window", "SetWindowPosAndSize", Array.from(arguments))
    }
    SwitchWorkspace(workspaceId: string, arg3: string): Promise<Workspace> {
        return WOS.callBackendService("window", "SwitchWorkspace", Array.from(arguments))
    }
}

export const WindowService = new WindowServiceType();

// workspaceservice.WorkspaceService (workspace)
class WorkspaceServiceType {
    // @returns object updates
    CloseTab(tabId: string, fromElectron: string, arg4: boolean): Promise<CloseTabRtnType> {
        return WOS.callBackendService("workspace", "CloseTab", Array.from(arguments))
    }

    // @returns tabId (and object updates)
    CreateTab(windowId: string, tabName: string, activateTab: boolean): Promise<string> {
        return WOS.callBackendService("workspace", "CreateTab", Array.from(arguments))
    }

    // @returns object updates
    DeleteWorkspace(workspaceId: string): Promise<void> {
        return WOS.callBackendService("workspace", "DeleteWorkspace", Array.from(arguments))
    }
    GetWorkspace(workspaceId: string): Promise<Workspace> {
        return WOS.callBackendService("workspace", "GetWorkspace", Array.from(arguments))
    }
    ListWorkspaces(): Promise<WorkspaceListEntry[]> {
        return WOS.callBackendService("workspace", "ListWorkspaces", Array.from(arguments))
    }

    // @returns object updates
    SetActiveTab(uiContext: string, tabId: string): Promise<void> {
        return WOS.callBackendService("workspace", "SetActiveTab", Array.from(arguments))
    }

    // @returns object updates
    UpdateTabIds(workspaceId: string, tabIds: string[]): Promise<void> {
        return WOS.callBackendService("workspace", "UpdateTabIds", Array.from(arguments))
    }
}

export const WorkspaceService = new WorkspaceServiceType();

