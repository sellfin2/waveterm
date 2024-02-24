// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as React from "react";
import * as mobxReact from "mobx-react";
import * as mobx from "mobx";
import { boundMethod } from "autobind-decorator";
import { If } from "tsx-control-statements/components";
import dayjs from "dayjs";
import localizedFormat from "dayjs/plugin/localizedFormat";
import { GlobalModel } from "@/models";
import { isBlank } from "@/util/util";
import { WorkspaceView } from "./workspace/workspaceview";
import { PluginsView } from "./pluginsview/pluginsview";
import { BookmarksView } from "./bookmarks/bookmarks";
import { HistoryView } from "./history/history";
import { ConnectionsView } from "./connections/connections";
import { ClientSettingsView } from "./clientsettings/clientsettings";
import { MainSideBar } from "./sidebar/sidebar";
import { DisconnectedModal, ClientStopModal } from "./common/modals";
import { ModalsProvider } from "./common/modals/provider";
import { ErrorBoundary } from "./common/error/errorboundary";
import "./app.less";

dayjs.extend(localizedFormat);

@mobxReact.observer
class App extends React.Component<{}, {}> {
    dcWait: OV<boolean> = mobx.observable.box(false, { name: "dcWait" });
    mainContentRef: React.RefObject<HTMLDivElement> = React.createRef();

    constructor(props: {}) {
        super(props);
        if (GlobalModel.isDev) document.body.className = "is-dev";
    }

    @boundMethod
    handleContextMenu(e: any) {
        let isInNonTermInput = false;
        let activeElem = document.activeElement;
        if (activeElem != null && activeElem.nodeName == "TEXTAREA") {
            if (!activeElem.classList.contains("xterm-helper-textarea")) {
                isInNonTermInput = true;
            }
        }
        if (activeElem != null && activeElem.nodeName == "INPUT" && activeElem.getAttribute("type") == "text") {
            isInNonTermInput = true;
        }
        let opts: ContextMenuOpts = {};
        if (isInNonTermInput) {
            opts.showCut = true;
        }
        let sel = window.getSelection();
        if (!isBlank(sel?.toString())) {
            GlobalModel.contextEditMenu(e, opts);
        } else {
            if (isInNonTermInput) {
                GlobalModel.contextEditMenu(e, opts);
            }
        }
    }

    @boundMethod
    updateDcWait(val: boolean): void {
        mobx.action(() => {
            this.dcWait.set(val);
        })();
    }

    render() {
        let remotesModel = GlobalModel.remotesModel;
        let disconnected = !GlobalModel.ws.open.get() || !GlobalModel.waveSrvRunning.get();
        let hasClientStop = GlobalModel.getHasClientStop();
        let dcWait = this.dcWait.get();
        let platform = GlobalModel.getPlatform();
        let clientData = GlobalModel.clientData.get();

        // Previously, this is done in sidebar.tsx but it causes flicker when clientData is null cos screen-view shifts around.
        // Doing it here fixes the flicker cos app is not rendered until clientData is populated.
        if (clientData == null) {
            return null;
        }

        if (disconnected || hasClientStop) {
            if (!dcWait) {
                setTimeout(() => this.updateDcWait(true), 1500);
            }
            return (
                <div id="main" className={"platform-" + platform} onContextMenu={this.handleContextMenu}>
                    <div ref={this.mainContentRef} className="main-content">
                        <MainSideBar parentRef={this.mainContentRef} clientData={clientData} />
                        <div className="session-view" />
                    </div>
                    <If condition={dcWait}>
                        <If condition={disconnected}>
                            <DisconnectedModal />
                        </If>
                        <If condition={!disconnected && hasClientStop}>
                            <ClientStopModal />
                        </If>
                    </If>
                </div>
            );
        }
        if (dcWait) {
            setTimeout(() => this.updateDcWait(false), 0);
        }
        // used to force a full reload of the application
        let renderVersion = GlobalModel.renderVersion.get();
        return (
            <div
                key={"version-" + renderVersion}
                id="main"
                className={"platform-" + platform}
                onContextMenu={this.handleContextMenu}
            >
                <div ref={this.mainContentRef} className="main-content">
                    <div className="main-content-bottom-color" />
                    <MainSideBar parentRef={this.mainContentRef} clientData={clientData} />
                    <ErrorBoundary>
                        <PluginsView />
                        <WorkspaceView />
                        <HistoryView />
                        <BookmarksView />
                        <ConnectionsView model={remotesModel} />
                        <ClientSettingsView model={remotesModel} />
                    </ErrorBoundary>
                </div>
                <ModalsProvider />
            </div>
        );
    }
}

export { App };
