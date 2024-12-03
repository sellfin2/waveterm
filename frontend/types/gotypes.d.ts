// Copyright 2024, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// generated by cmd/generate/main-generatets.go

declare global {

    // wshrpc.ActivityDisplayType
    type ActivityDisplayType = {
        width: number;
        height: number;
        dpr: number;
        internal?: boolean;
    };

    // wshrpc.ActivityUpdate
    type ActivityUpdate = {
        fgminutes?: number;
        activeminutes?: number;
        openminutes?: number;
        numtabs?: number;
        newtab?: number;
        numblocks?: number;
        numwindows?: number;
        numsshconn?: number;
        numwslconn?: number;
        nummagnify?: number;
        numpanics?: number;
        startup?: number;
        shutdown?: number;
        settabtheme?: number;
        buildtime?: string;
        displays?: ActivityDisplayType[];
        renderers?: {[key: string]: number};
        blocks?: {[key: string]: number};
        wshcmds?: {[key: string]: number};
        conn?: {[key: string]: number};
    };

    // wshrpc.AiMessageData
    type AiMessageData = {
        message?: string;
    };

    // waveobj.Block
    type Block = WaveObj & {
        parentoref?: string;
        blockdef: BlockDef;
        runtimeopts?: RuntimeOpts;
        stickers?: StickerType[];
        subblockids?: string[];
    };

    // blockcontroller.BlockControllerRuntimeStatus
    type BlockControllerRuntimeStatus = {
        blockid: string;
        shellprocstatus?: string;
        shellprocconnname?: string;
    };

    // waveobj.BlockDef
    type BlockDef = {
        files?: {[key: string]: FileDef};
        meta?: MetaType;
    };

    // wshrpc.BlockInfoData
    type BlockInfoData = {
        blockid: string;
        tabid: string;
        workspaceid: string;
        block: Block;
    };

    // webcmd.BlockInputWSCommand
    type BlockInputWSCommand = {
        wscommand: "blockinput";
        blockid: string;
        inputdata64: string;
    };

    // waveobj.Client
    type Client = WaveObj & {
        windowids: string[];
        tosagreed?: number;
        hasoldhistory?: boolean;
        tempoid?: string;
    };

    // workspaceservice.CloseTabRtnType
    type CloseTabRtnType = {
        closewindow?: boolean;
        newactivetabid?: string;
    };

    // wshrpc.CommandAppendIJsonData
    type CommandAppendIJsonData = {
        zoneid: string;
        filename: string;
        data: {[key: string]: any};
    };

    // wshrpc.CommandAuthenticateRtnData
    type CommandAuthenticateRtnData = {
        routeid: string;
        authtoken?: string;
    };

    // wshrpc.CommandBlockInputData
    type CommandBlockInputData = {
        blockid: string;
        inputdata64?: string;
        signame?: string;
        termsize?: TermSize;
    };

    // wshrpc.CommandBlockSetViewData
    type CommandBlockSetViewData = {
        blockid: string;
        view: string;
    };

    // wshrpc.CommandControllerResyncData
    type CommandControllerResyncData = {
        forcerestart?: boolean;
        tabid: string;
        blockid: string;
        rtopts?: RuntimeOpts;
    };

    // wshrpc.CommandCreateBlockData
    type CommandCreateBlockData = {
        tabid: string;
        blockdef: BlockDef;
        rtopts?: RuntimeOpts;
        magnified?: boolean;
    };

    // wshrpc.CommandCreateSubBlockData
    type CommandCreateSubBlockData = {
        parentblockid: string;
        blockdef: BlockDef;
    };

    // wshrpc.CommandDeleteBlockData
    type CommandDeleteBlockData = {
        blockid: string;
    };

    // wshrpc.CommandDisposeData
    type CommandDisposeData = {
        routeid: string;
    };

    // wshrpc.CommandEventReadHistoryData
    type CommandEventReadHistoryData = {
        event: string;
        scope: string;
        maxitems: number;
    };

    // wshrpc.CommandFileCreateData
    type CommandFileCreateData = {
        zoneid: string;
        filename: string;
        meta?: {[key: string]: any};
        opts?: FileOptsType;
    };

    // wshrpc.CommandFileData
    type CommandFileData = {
        zoneid: string;
        filename: string;
        data64?: string;
        at?: CommandFileDataAt;
    };

    // wshrpc.CommandFileDataAt
    type CommandFileDataAt = {
        offset: number;
        size?: number;
    };

    // wshrpc.CommandFileListData
    type CommandFileListData = {
        zoneid: string;
        prefix?: string;
        all?: boolean;
        offset?: number;
        limit?: number;
    };

    // wshrpc.CommandGetMetaData
    type CommandGetMetaData = {
        oref: ORef;
    };

    // wshrpc.CommandMessageData
    type CommandMessageData = {
        oref: ORef;
        message: string;
    };

    // wshrpc.CommandRemoteStreamFileData
    type CommandRemoteStreamFileData = {
        path: string;
        byterange?: string;
    };

    // wshrpc.CommandRemoteStreamFileRtnData
    type CommandRemoteStreamFileRtnData = {
        fileinfo?: FileInfo[];
        data64?: string;
    };

    // wshrpc.CommandRemoteWriteFileData
    type CommandRemoteWriteFileData = {
        path: string;
        data64: string;
        createmode?: number;
    };

    // wshrpc.CommandResolveIdsData
    type CommandResolveIdsData = {
        blockid: string;
        ids: string[];
    };

    // wshrpc.CommandResolveIdsRtnData
    type CommandResolveIdsRtnData = {
        resolvedids: {[key: string]: ORef};
    };

    // wshrpc.CommandSetMetaData
    type CommandSetMetaData = {
        oref: ORef;
        meta: MetaType;
    };

    // wshrpc.CommandVarData
    type CommandVarData = {
        key: string;
        val?: string;
        remove?: boolean;
        zoneid: string;
        filename: string;
    };

    // wshrpc.CommandVarResponseData
    type CommandVarResponseData = {
        key: string;
        val: string;
        exists: boolean;
    };

    // wshrpc.CommandWaitForRouteData
    type CommandWaitForRouteData = {
        routeid: string;
        waitms: number;
    };

    // wshrpc.CommandWebSelectorData
    type CommandWebSelectorData = {
        workspaceid: string;
        blockid: string;
        tabid: string;
        selector: string;
        opts?: WebSelectorOpts;
    };

    // wconfig.ConfigError
    type ConfigError = {
        file: string;
        err: string;
    };

    // wshrpc.ConnKeywords
    type ConnKeywords = {
        wshenabled?: boolean;
        askbeforewshinstall?: boolean;
        hidden?: boolean;
        "ssh:user"?: string;
        "ssh:hostname"?: string;
        "ssh:port"?: string;
        "ssh:identityfile"?: string[];
        "ssh:batchmode"?: boolean;
        "ssh:pubkeyauthentication"?: boolean;
        "ssh:passwordauthentication"?: boolean;
        "ssh:kbdinteractiveauthentication"?: boolean;
        "ssh:preferredauthentications"?: string[];
        "ssh:addkeystoagent"?: boolean;
        "ssh:identityagent"?: string;
        "ssh:proxyjump"?: string[];
        "ssh:userknownhostsfile"?: string[];
        "ssh:globalknownhostsfile"?: string[];
    };

    // wshrpc.ConnRequest
    type ConnRequest = {
        host: string;
        keywords?: ConnKeywords;
    };

    // wshrpc.ConnStatus
    type ConnStatus = {
        status: string;
        wshenabled: boolean;
        connection: string;
        connected: boolean;
        hasconnected: boolean;
        activeconnnum: number;
        error?: string;
    };

    // wshrpc.CpuDataRequest
    type CpuDataRequest = {
        id: string;
        count: number;
    };

    // vdom.DomRect
    type DomRect = {
        top: number;
        left: number;
        right: number;
        bottom: number;
        width: number;
        height: number;
    };

    // waveobj.FileDef
    type FileDef = {
        filetype?: string;
        path?: string;
        url?: string;
        content?: string;
        meta?: {[key: string]: any};
    };

    // wshrpc.FileInfo
    type FileInfo = {
        path: string;
        dir: string;
        name: string;
        notfound?: boolean;
        size: number;
        mode: number;
        modestr: string;
        modtime: number;
        isdir?: boolean;
        mimetype?: string;
        readonly?: boolean;
    };

    // filestore.FileOptsType
    type FileOptsType = {
        maxsize?: number;
        circular?: boolean;
        ijson?: boolean;
        ijsonbudget?: number;
    };

    // wconfig.FullConfigType
    type FullConfigType = {
        settings: SettingsType;
        mimetypes: {[key: string]: MimeTypeConfigType};
        defaultwidgets: {[key: string]: WidgetConfigType};
        widgets: {[key: string]: WidgetConfigType};
        presets: {[key: string]: MetaType};
        termthemes: {[key: string]: TermThemeType};
        connections: {[key: string]: ConnKeywords};
        configerrors: ConfigError[];
    };

    // fileservice.FullFile
    type FullFile = {
        info: FileInfo;
        data64: string;
    };

    // waveobj.LayoutActionData
    type LayoutActionData = {
        actiontype: string;
        blockid: string;
        nodesize?: number;
        indexarr?: number[];
        focused: boolean;
        magnified: boolean;
    };

    // waveobj.LayoutState
    type LayoutState = WaveObj & {
        rootnode?: any;
        magnifiednodeid?: string;
        focusednodeid?: string;
        leaforder?: LeafOrderEntry[];
        pendingbackendactions?: LayoutActionData[];
    };

    // waveobj.LeafOrderEntry
    type LeafOrderEntry = {
        nodeid: string;
        blockid: string;
    };

    // waveobj.MetaTSType
    type MetaType = {
        view?: string;
        controller?: string;
        file?: string;
        url?: string;
        pinnedurl?: string;
        connection?: string;
        edit?: boolean;
        history?: string[];
        "history:forward"?: string[];
        "display:name"?: string;
        "display:order"?: number;
        icon?: string;
        "icon:color"?: string;
        "frame:*"?: boolean;
        frame?: boolean;
        "frame:bordercolor"?: string;
        "frame:activebordercolor"?: string;
        "frame:title"?: string;
        "frame:icon"?: string;
        "frame:text"?: string;
        "cmd:*"?: boolean;
        cmd?: string;
        "cmd:interactive"?: boolean;
        "cmd:login"?: boolean;
        "cmd:runonstart"?: boolean;
        "cmd:clearonstart"?: boolean;
        "cmd:clearonrestart"?: boolean;
        "cmd:env"?: {[key: string]: string};
        "cmd:cwd"?: string;
        "cmd:nowsh"?: boolean;
        "ai:*"?: boolean;
        "ai:preset"?: string;
        "ai:apitype"?: string;
        "ai:baseurl"?: string;
        "ai:apitoken"?: string;
        "ai:name"?: string;
        "ai:model"?: string;
        "ai:orgid"?: string;
        "ai:apiversion"?: string;
        "ai:maxtokens"?: number;
        "ai:timeoutms"?: number;
        "editor:*"?: boolean;
        "editor:wordwrap"?: boolean;
        "graph:*"?: boolean;
        "graph:numpoints"?: number;
        "graph:metrics"?: string[];
        "sysinfo:type"?: string;
        "bg:*"?: boolean;
        bg?: string;
        "bg:opacity"?: number;
        "bg:blendmode"?: string;
        "bg:bordercolor"?: string;
        "bg:activebordercolor"?: string;
        "term:*"?: boolean;
        "term:fontsize"?: number;
        "term:fontfamily"?: string;
        "term:mode"?: string;
        "term:theme"?: string;
        "term:localshellpath"?: string;
        "term:localshellopts"?: string[];
        "term:scrollback"?: number;
        "term:vdomblockid"?: string;
        "term:vdomtoolbarblockid"?: string;
        "vdom:*"?: boolean;
        "vdom:initialized"?: boolean;
        "vdom:correlationid"?: string;
        "vdom:route"?: string;
        "vdom:persist"?: boolean;
        count?: number;
    };

    // tsgenmeta.MethodMeta
    type MethodMeta = {
        Desc: string;
        ArgNames: string[];
        ReturnDesc: string;
    };

    // wconfig.MimeTypeConfigType
    type MimeTypeConfigType = {
        icon: string;
        color: string;
    };

    // waveobj.ORef
    type ORef = string;

    // wshrpc.OpenAIOptsType
    type OpenAIOptsType = {
        model: string;
        apitype?: string;
        apitoken: string;
        orgid?: string;
        apiversion?: string;
        baseurl?: string;
        maxtokens?: number;
        maxchoices?: number;
        timeoutms?: number;
    };

    // wshrpc.OpenAIPacketType
    type OpenAIPacketType = {
        type: string;
        model?: string;
        created?: number;
        finish_reason?: string;
        usage?: OpenAIUsageType;
        index?: number;
        text?: string;
        error?: string;
    };

    // wshrpc.OpenAIPromptMessageType
    type OpenAIPromptMessageType = {
        role: string;
        content: string;
        name?: string;
    };

    // wshrpc.OpenAIUsageType
    type OpenAIUsageType = {
        prompt_tokens?: number;
        completion_tokens?: number;
        total_tokens?: number;
    };

    // wshrpc.OpenAiStreamRequest
    type OpenAiStreamRequest = {
        clientid?: string;
        opts: OpenAIOptsType;
        prompt: OpenAIPromptMessageType[];
    };

    // waveobj.Point
    type Point = {
        x: number;
        y: number;
    };

    // wshutil.RpcMessage
    type RpcMessage = {
        command?: string;
        reqid?: string;
        resid?: string;
        timeout?: number;
        route?: string;
        authtoken?: string;
        source?: string;
        cont?: boolean;
        cancel?: boolean;
        error?: string;
        datatype?: string;
        data?: any;
    };

    // wshrpc.RpcOpts
    type RpcOpts = {
        timeout?: number;
        noresponse?: boolean;
        route?: string;
    };

    // waveobj.RuntimeOpts
    type RuntimeOpts = {
        termsize?: TermSize;
        winsize?: WinSize;
    };

    // webcmd.SetBlockTermSizeWSCommand
    type SetBlockTermSizeWSCommand = {
        wscommand: "setblocktermsize";
        blockid: string;
        termsize: TermSize;
    };

    // wconfig.SettingsType
    type SettingsType = {
        "ai:*"?: boolean;
        "ai:preset"?: string;
        "ai:apitype"?: string;
        "ai:baseurl"?: string;
        "ai:apitoken"?: string;
        "ai:name"?: string;
        "ai:model"?: string;
        "ai:orgid"?: string;
        "ai:apiversion"?: string;
        "ai:maxtokens"?: number;
        "ai:timeoutms"?: number;
        "term:*"?: boolean;
        "term:fontsize"?: number;
        "term:fontfamily"?: string;
        "term:theme"?: string;
        "term:disablewebgl"?: boolean;
        "term:localshellpath"?: string;
        "term:localshellopts"?: string[];
        "term:scrollback"?: number;
        "term:copyonselect"?: boolean;
        "editor:minimapenabled"?: boolean;
        "editor:stickyscrollenabled"?: boolean;
        "web:*"?: boolean;
        "web:openlinksinternally"?: boolean;
        "web:defaulturl"?: string;
        "web:defaultsearch"?: string;
        "blockheader:*"?: boolean;
        "blockheader:showblockids"?: boolean;
        "autoupdate:*"?: boolean;
        "autoupdate:enabled"?: boolean;
        "autoupdate:intervalms"?: number;
        "autoupdate:installonquit"?: boolean;
        "autoupdate:channel"?: string;
        "preview:showhiddenfiles"?: boolean;
        "widget:*"?: boolean;
        "widget:showhelp"?: boolean;
        "window:*"?: boolean;
        "window:transparent"?: boolean;
        "window:blur"?: boolean;
        "window:opacity"?: number;
        "window:bgcolor"?: string;
        "window:reducedmotion"?: boolean;
        "window:tilegapsize"?: number;
        "window:showmenubar"?: boolean;
        "window:nativetitlebar"?: boolean;
        "window:disablehardwareacceleration"?: boolean;
        "window:maxtabcachesize"?: number;
        "window:magnifiedblockopacity"?: number;
        "window:magnifiedblocksize"?: number;
        "window:magnifiedblockblurprimarypx"?: number;
        "window:magnifiedblockblursecondarypx"?: number;
        "telemetry:*"?: boolean;
        "telemetry:enabled"?: boolean;
        "conn:*"?: boolean;
        "conn:askbeforewshinstall"?: boolean;
        "conn:wshenabled"?: boolean;
    };

    // waveobj.StickerClickOptsType
    type StickerClickOptsType = {
        sendinput?: string;
        createblock?: BlockDef;
    };

    // waveobj.StickerDisplayOptsType
    type StickerDisplayOptsType = {
        icon: string;
        imgsrc: string;
        svgblob?: string;
    };

    // waveobj.StickerType
    type StickerType = {
        stickertype: string;
        style: {[key: string]: any};
        clickopts?: StickerClickOptsType;
        display: StickerDisplayOptsType;
    };

    // wps.SubscriptionRequest
    type SubscriptionRequest = {
        event: string;
        scopes?: string[];
        allscopes?: boolean;
    };

    // waveobj.Tab
    type Tab = WaveObj & {
        name: string;
        layoutstate: string;
        blockids: string[];
    };

    // waveobj.TermSize
    type TermSize = {
        rows: number;
        cols: number;
    };

    // wconfig.TermThemeType
    type TermThemeType = {
        "display:name": string;
        "display:order": number;
        black: string;
        red: string;
        green: string;
        yellow: string;
        blue: string;
        magenta: string;
        cyan: string;
        white: string;
        brightBlack: string;
        brightRed: string;
        brightGreen: string;
        brightYellow: string;
        brightBlue: string;
        brightMagenta: string;
        brightCyan: string;
        brightWhite: string;
        gray: string;
        cmdtext: string;
        foreground: string;
        selectionBackground: string;
        background: string;
        cursor: string;
    };

    // wshrpc.TimeSeriesData
    type TimeSeriesData = {
        ts: number;
        values: {[key: string]: number};
    };

    // waveobj.UIContext
    type UIContext = {
        windowid: string;
        activetabid: string;
    };

    // userinput.UserInputRequest
    type UserInputRequest = {
        requestid: string;
        querytext: string;
        responsetype: string;
        title: string;
        markdown: boolean;
        timeoutms: number;
        checkboxmsg: string;
        publictext: boolean;
        oklabel?: string;
        cancellabel?: string;
    };

    // userinput.UserInputResponse
    type UserInputResponse = {
        type: string;
        requestid: string;
        text?: string;
        confirm?: boolean;
        errormsg?: string;
        checkboxstat?: boolean;
    };

    // vdom.VDomAsyncInitiationRequest
    type VDomAsyncInitiationRequest = {
        type: "asyncinitiationrequest";
        ts: number;
        blockid?: string;
    };

    // vdom.VDomBackendOpts
    type VDomBackendOpts = {
        closeonctrlc?: boolean;
        globalkeyboardevents?: boolean;
        globalstyles?: boolean;
    };

    // vdom.VDomBackendUpdate
    type VDomBackendUpdate = {
        type: "backendupdate";
        ts: number;
        blockid: string;
        opts?: VDomBackendOpts;
        haswork?: boolean;
        renderupdates?: VDomRenderUpdate[];
        transferelems?: VDomTransferElem[];
        statesync?: VDomStateSync[];
        refoperations?: VDomRefOperation[];
        messages?: VDomMessage[];
    };

    // vdom.VDomBinding
    type VDomBinding = {
        type: "binding";
        bind: string;
    };

    // vdom.VDomCreateContext
    type VDomCreateContext = {
        type: "createcontext";
        ts: number;
        meta?: MetaType;
        target?: VDomTarget;
        persist?: boolean;
    };

    // vdom.VDomElem
    type VDomElem = {
        waveid?: string;
        tag: string;
        props?: {[key: string]: any};
        children?: VDomElem[];
        text?: string;
    };

    // vdom.VDomEvent
    type VDomEvent = {
        waveid: string;
        eventtype: string;
        globaleventtype?: string;
        targetvalue?: string;
        targetchecked?: boolean;
        targetname?: string;
        targetid?: string;
        keydata?: WaveKeyboardEvent;
        mousedata?: WavePointerData;
    };

    // vdom.VDomFrontendUpdate
    type VDomFrontendUpdate = {
        type: "frontendupdate";
        ts: number;
        blockid: string;
        correlationid?: string;
        dispose?: boolean;
        resync?: boolean;
        rendercontext?: VDomRenderContext;
        events?: VDomEvent[];
        statesync?: VDomStateSync[];
        refupdates?: VDomRefUpdate[];
        messages?: VDomMessage[];
    };

    // vdom.VDomFunc
    type VDomFunc = {
        type: "func";
        stoppropagation?: boolean;
        preventdefault?: boolean;
        globalevent?: string;
        #keys?: string[];
    };

    // vdom.VDomMessage
    type VDomMessage = {
        messagetype: string;
        message: string;
        stacktrace?: string;
        params?: any[];
    };

    // vdom.VDomRef
    type VDomRef = {
        type: "ref";
        refid: string;
        trackposition?: boolean;
        position?: VDomRefPosition;
        hascurrent?: boolean;
    };

    // vdom.VDomRefOperation
    type VDomRefOperation = {
        refid: string;
        op: string;
        params?: any[];
        outputref?: string;
    };

    // vdom.VDomRefPosition
    type VDomRefPosition = {
        offsetheight: number;
        offsetwidth: number;
        scrollheight: number;
        scrollwidth: number;
        scrolltop: number;
        boundingclientrect: DomRect;
    };

    // vdom.VDomRefUpdate
    type VDomRefUpdate = {
        refid: string;
        hascurrent: boolean;
        position?: VDomRefPosition;
    };

    // vdom.VDomRenderContext
    type VDomRenderContext = {
        blockid: string;
        focused: boolean;
        width: number;
        height: number;
        rootrefid: string;
        background?: boolean;
    };

    // vdom.VDomRenderUpdate
    type VDomRenderUpdate = {
        updatetype: "root"|"append"|"replace"|"remove"|"insert";
        waveid?: string;
        vdomwaveid?: string;
        vdom?: VDomElem;
        index?: number;
    };

    // vdom.VDomStateSync
    type VDomStateSync = {
        atom: string;
        value: any;
    };

    // vdom.VDomTarget
    type VDomTarget = {
        newblock?: boolean;
        magnified?: boolean;
        toolbar?: VDomTargetToolbar;
    };

    // vdom.VDomTargetToolbar
    type VDomTargetToolbar = {
        toolbar: boolean;
        height?: string;
    };

    // vdom.VDomTransferElem
    type VDomTransferElem = {
        waveid?: string;
        tag: string;
        props?: {[key: string]: any};
        children?: string[];
        text?: string;
    };

    // wshrpc.VDomUrlRequestData
    type VDomUrlRequestData = {
        method: string;
        url: string;
        headers: {[key: string]: string};
        body?: string;
    };

    // wshrpc.VDomUrlRequestResponse
    type VDomUrlRequestResponse = {
        statuscode?: number;
        headers?: {[key: string]: string};
        body?: string;
    };

    type WSCommandType = {
        wscommand: string;
    } & ( SetBlockTermSizeWSCommand | BlockInputWSCommand | WSRpcCommand );

    // eventbus.WSEventType
    type WSEventType = {
        eventtype: string;
        oref?: string;
        data: any;
    };

    // wps.WSFileEventData
    type WSFileEventData = {
        zoneid: string;
        filename: string;
        fileop: string;
        data64: string;
    };

    // webcmd.WSRpcCommand
    type WSRpcCommand = {
        wscommand: "rpc";
        message: RpcMessage;
    };

    // wconfig.WatcherUpdate
    type WatcherUpdate = {
        fullconfig: FullConfigType;
    };

    // wps.WaveEvent
    type WaveEvent = {
        event: string;
        scopes?: string[];
        sender?: string;
        persist?: number;
        data?: any;
    };

    // filestore.WaveFile
    type WaveFile = {
        zoneid: string;
        name: string;
        opts: FileOptsType;
        createdts: number;
        size: number;
        modts: number;
        meta: {[key: string]: any};
    };

    // wshrpc.WaveFileInfo
    type WaveFileInfo = {
        zoneid: string;
        name: string;
        opts?: FileOptsType;
        size?: number;
        createdts?: number;
        modts?: number;
        meta?: {[key: string]: any};
        isdir?: boolean;
    };

    // wshrpc.WaveInfoData
    type WaveInfoData = {
        version: string;
        clientid: string;
        buildtime: string;
        configdir: string;
        datadir: string;
    };

    // vdom.WaveKeyboardEvent
    type WaveKeyboardEvent = {
        type: "keydown"|"keyup"|"keypress"|"unknown";
        key: string;
        code: string;
        repeat?: boolean;
        location?: number;
        shift?: boolean;
        control?: boolean;
        alt?: boolean;
        meta?: boolean;
        cmd?: boolean;
        option?: boolean;
    };

    // wshrpc.WaveNotificationOptions
    type WaveNotificationOptions = {
        title?: string;
        body?: string;
        silent?: boolean;
    };

    // waveobj.WaveObj
    type WaveObj = {
        otype: string;
        oid: string;
        version: number;
        meta: MetaType;
    };

    // waveobj.WaveObjUpdate
    type WaveObjUpdate = {
        updatetype: string;
        otype: string;
        oid: string;
        obj?: WaveObj;
    };

    // vdom.WavePointerData
    type WavePointerData = {
        button: number;
        buttons: number;
        clientx?: number;
        clienty?: number;
        pagex?: number;
        pagey?: number;
        screenx?: number;
        screeny?: number;
        movementx?: number;
        movementy?: number;
        shift?: boolean;
        control?: boolean;
        alt?: boolean;
        meta?: boolean;
        cmd?: boolean;
        option?: boolean;
    };

    // waveobj.Window
    type WaveWindow = WaveObj & {
        workspaceid: string;
        isnew?: boolean;
        pos: Point;
        winsize: WinSize;
        lastfocusts: number;
    };

    // service.WebCallType
    type WebCallType = {
        service: string;
        method: string;
        uicontext?: UIContext;
        args: any[];
    };

    // service.WebReturnType
    type WebReturnType = {
        success?: boolean;
        error?: string;
        data?: any;
        updates?: WaveObjUpdate[];
    };

    // wshrpc.WebSelectorOpts
    type WebSelectorOpts = {
        all?: boolean;
        inner?: boolean;
    };

    // wconfig.WidgetConfigType
    type WidgetConfigType = {
        "display:order"?: number;
        icon?: string;
        color?: string;
        label?: string;
        description?: string;
        blockdef: BlockDef;
    };

    // waveobj.WinSize
    type WinSize = {
        width: number;
        height: number;
    };

    // waveobj.Workspace
    type Workspace = WaveObj & {
        name: string;
        icon: string;
        color: string;
        tabids: string[];
        activetabid: string;
    };

    // wshrpc.WorkspaceInfoData
    type WorkspaceInfoData = {
        windowid: string;
        workspacedata: Workspace;
    };

    // waveobj.WorkspaceListEntry
    type WorkspaceListEntry = {
        workspaceid: string;
        windowid: string;
    };

    // wshrpc.WshServerCommandMeta
    type WshServerCommandMeta = {
        commandtype: string;
    };

}

export {}
