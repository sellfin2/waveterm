// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as React from "react";
import * as mobxReact from "mobx-react";
import * as mobx from "mobx";
import { boundMethod } from "autobind-decorator";
import cn from "classnames";
import dayjs from "dayjs";
import type { RemoteType } from "../../types/types";
import { If } from "tsx-control-statements/components";
import { compareLoose } from "semver";
import { motion } from "framer-motion";

import { ReactComponent as LeftChevronIcon } from "../assets/icons/chevron_left.svg";
import { ReactComponent as HelpIcon } from "../assets/icons/help.svg";
import { ReactComponent as SettingsIcon } from "../assets/icons/settings.svg";
import { ReactComponent as DiscordIcon } from "../assets/icons/discord.svg";
import { ReactComponent as HistoryIcon } from "../assets/icons/history.svg";
import { ReactComponent as AppsIcon } from "../assets/icons/apps.svg";
import { ReactComponent as ConnectionsIcon } from "../assets/icons/connections.svg";
import { ReactComponent as WorkspacesIcon } from "../assets/icons/workspaces.svg";
import { ReactComponent as AddIcon } from "../assets/icons/add.svg";
import { ReactComponent as ActionsIcon } from "../assets/icons/tab/actions.svg";

import localizedFormat from "dayjs/plugin/localizedFormat";
import { GlobalModel, GlobalCommandRunner, Session, VERSION, SidebarModel } from "../../model/model";
import { sortAndFilterRemotes, isBlank, openLink } from "../../util/util";
import * as constants from "../appconst";

import "./sidebar.less";

dayjs.extend(localizedFormat);

type OV<V> = mobx.IObservableValue<V>;

@mobxReact.observer
class MainSideBar extends React.Component<{}, {}> {
    collapsed: mobx.IObservableValue<boolean> = mobx.observable.box(false);
    sidebarRef = React.createRef<HTMLDivElement>();
    mainSidebarModel = new SidebarModel();

    componentDidMount(): void {
        this.mainSidebarModel.sidebarRef = this.sidebarRef;
        mobx.action(() => {
            GlobalModel.sidebarModels.set("main", this.mainSidebarModel);
        })();
    }

    @boundMethod
    toggleCollapsed() {
        mobx.action(() => {
            this.collapsed.set(!this.collapsed.get());
        })();
    }

    handleSessionClick(sessionId: string) {
        GlobalCommandRunner.switchSession(sessionId);
    }

    handleNewSession() {
        GlobalCommandRunner.createNewSession();
    }

    handleNewSharedSession() {
        GlobalCommandRunner.openSharedSession();
    }

    clickLinks() {
        mobx.action(() => {
            GlobalModel.showLinks.set(!GlobalModel.showLinks.get());
        })();
    }

    remoteDisplayName(remote: RemoteType): any {
        if (!isBlank(remote.remotealias)) {
            return (
                <>
                    <span>{remote.remotealias}</span>
                    <span className="small-text"> {remote.remotecanonicalname}</span>
                </>
            );
        }
        return <span>{remote.remotecanonicalname}</span>;
    }

    clickRemote(remote: RemoteType) {
        GlobalCommandRunner.showRemote(remote.remoteid);
    }

    @boundMethod
    handlePluginsClick(): void {
        if (GlobalModel.activeMainView.get() == "plugins") {
            GlobalModel.showSessionView();
            return;
        }
        GlobalModel.pluginsModel.showPluginsView();
    }

    @boundMethod
    handleHistoryClick(): void {
        if (GlobalModel.activeMainView.get() == "history") {
            GlobalModel.showSessionView();
            return;
        }
        GlobalModel.historyViewModel.reSearch();
    }

    @boundMethod
    handlePlaybookClick(): void {
        console.log("playbook click");
        return;
    }

    @boundMethod
    handleBookmarksClick(): void {
        if (GlobalModel.activeMainView.get() == "bookmarks") {
            GlobalModel.showSessionView();
            return;
        }
        GlobalCommandRunner.bookmarksView();
    }

    @boundMethod
    handleConnectionsClick(): void {
        if (GlobalModel.activeMainView.get() == "connections") {
            GlobalModel.showSessionView();
            return;
        }
        GlobalCommandRunner.connectionsView();
    }

    @boundMethod
    handleWebSharingClick(): void {
        if (GlobalModel.activeMainView.get() == "webshare") {
            GlobalModel.showSessionView();
            return;
        }
        GlobalModel.showWebShareView();
    }

    @boundMethod
    handleSettingsClick(): void {
        if (GlobalModel.activeMainView.get() == "clientsettings") {
            GlobalModel.showSessionView();
            return;
        }
        GlobalCommandRunner.clientSettingsView();
    }

    @boundMethod
    openSessionSettings(e: any, session: Session): void {
        e.preventDefault();
        e.stopPropagation();
        mobx.action(() => {
            GlobalModel.sessionSettingsModal.set(session.sessionId);
        })();
        GlobalModel.modalsModel.pushModal(constants.SESSION_SETTINGS);
    }

    getSessions() {
        if (!GlobalModel.sessionListLoaded.get()) return <div className="item">loading ...</div>;
        let sessionList = [];
        let activeSessionId = GlobalModel.activeSessionId.get();
        for (let session of GlobalModel.sessionList) {
            if (!session.archived.get() || session.sessionId == activeSessionId) {
                sessionList.push(session);
            }
        }
        return sessionList.map((session, index) => {
            const isActive = GlobalModel.activeMainView.get() == "session" && activeSessionId == session.sessionId;
            return (
                <div
                    key={index}
                    className={`item hoverEffect ${isActive ? "active" : ""}`}
                    onClick={() => this.handleSessionClick(session.sessionId)}
                >
                    <span className="index">{index + 1}</span>
                    <span className="truncate sessionName">{session.name.get()}</span>
                    <ActionsIcon
                        className="icon hoverEffect actions"
                        onClick={(e) => this.openSessionSettings(e, session)}
                    />
                </div>
            );
        });
    }

    render() {
        // console.log("MainSidebarModel", new MainSidebarModel());

        let model = GlobalModel;
        let activeSessionId = model.activeSessionId.get();
        let activeScreen = model.getActiveScreen();
        let activeRemoteId: string = null;
        if (activeScreen != null) {
            let rptr = activeScreen.curRemote.get();
            if (rptr != null && !isBlank(rptr.remoteid)) {
                activeRemoteId = rptr.remoteid;
            }
        }
        let remotes = model.remotes ?? [];
        remotes = sortAndFilterRemotes(remotes);
        let sessionList = [];
        for (let session of model.sessionList) {
            if (!session.archived.get() || session.sessionId == activeSessionId) {
                sessionList.push(session);
            }
        }
        let isCollapsed = this.collapsed.get();
        let clientData = GlobalModel.clientData.get();
        let needsUpdate = false;
        if (!clientData?.clientopts.noreleasecheck && !isBlank(clientData?.releaseinfo?.latestversion)) {
            needsUpdate = compareLoose(VERSION, clientData.releaseinfo.latestversion) < 0;
        }
        // return (
        //     <div className={cn("main-sidebar", { collapsed: isCollapsed }, { "is-dev": GlobalModel.isDev })}>
        //         <div className="title-bar-drag" />
        //         <div className="contents">
        //             <div className="logo">
        //                 <If condition={isCollapsed}>
        //                     <div className="logo-container" onClick={this.toggleCollapsed}>
        //                         <img src="public/logos/wave-logo.png" />
        //                     </div>
        //                 </If>
        //                 <If condition={!isCollapsed}>
        //                     <div className="logo-container">
        //                         <img src="public/logos/wave-dark.png" />
        //                     </div>
        //                     <div className="spacer" />
        //                     <div className="collapse-button" onClick={this.toggleCollapsed}>
        //                         <LeftChevronIcon className="icon" />
        //                     </div>
        //                 </If>
        //             </div>
        //             <div className="separator" />
        //             <div className="top">
        //                 <div className="item hoverEffect unselectable" onClick={this.handleHistoryClick}>
        //                     <HistoryIcon className="icon" />
        //                     History
        //                     <span className="hotkey">&#x2318;H</span>
        //                 </div>
        //                 {/* <div className="item hoverEffect unselectable" onClick={this.handleBookmarksClick}>
        //                     <FavoritesIcon className="icon" />
        //                     Favorites
        //                     <span className="hotkey">&#x2318;B</span>
        //                     </div>  */}
        //                 <div className="item hoverEffect unselectable" onClick={this.handleConnectionsClick}>
        //                     <ConnectionsIcon className="icon" />
        //                     Connections
        //                 </div>
        //             </div>
        //             <div className="separator" />
        //             <div className="item workspaces-item unselectable">
        //                 <WorkspacesIcon className="icon" />
        //                 Workspaces
        //                 <div className="add_workspace hoverEffect" onClick={this.handleNewSession}>
        //                     <AddIcon />
        //                 </div>
        //             </div>
        //             <div className="middle hideScrollbarUntillHover">{this.getSessions()}</div>
        //             <div className="bottom">
        //                 <If condition={needsUpdate}>
        //                     <div
        //                         className="item hoverEffect unselectable updateBanner"
        //                         onClick={() => openLink("https://www.waveterm.dev/download?ref=upgrade")}
        //                     >
        //                         <i className="fa-sharp fa-regular fa-circle-up icon" />
        //                         Update Available
        //                     </div>
        //                 </If>
        //                 <If condition={GlobalModel.isDev}>
        //                     <div className="item hoverEffect unselectable" onClick={this.handlePluginsClick}>
        //                         <AppsIcon className="icon" />
        //                         Apps
        //                         <span className="hotkey">&#x2318;A</span>
        //                     </div>
        //                 </If>
        //                 <div className="item hoverEffect unselectable" onClick={this.handleSettingsClick}>
        //                     <SettingsIcon className="icon" />
        //                     Settings
        //                 </div>
        //                 <div
        //                     className="item hoverEffect unselectable"
        //                     onClick={() => openLink("https://docs.waveterm.dev")}
        //                 >
        //                     <HelpIcon className="icon" />
        //                     Documentation
        //                 </div>
        //                 <div
        //                     className="item hoverEffect unselectable"
        //                     onClick={() => openLink("https://discord.gg/XfvZ334gwU")}
        //                 >
        //                     <DiscordIcon className="icon discord" />
        //                     Discord
        //                 </div>
        //             </div>
        //         </div>
        //     </div>
        // );

        return (
            <motion.div
                ref={this.sidebarRef}
                drag="x"
                onDrag={this.mainSidebarModel.handleDrag}
                onDragStart={this.mainSidebarModel.handleDragStart}
                onDragEnd={this.mainSidebarModel.handleDragEnd}
                onMouseMove={this.mainSidebarModel.handleMouseMove}
                onMouseLeave={this.mainSidebarModel.handleMouseLeave}
                onDoubleClick={this.mainSidebarModel.handleDoubleClick}
                style={{ width: `${this.mainSidebarModel.width.get()}px` }}
                className={cn(`sidebar col-resize`, {
                    "divider-dragging": this.mainSidebarModel.isDragging.get(),
                    "divider-hovered": this.mainSidebarModel.isHovered.get(),
                })}
                dragConstraints={{ left: 0, right: 0 }}
                dragElastic={0} // No elastic dragging
                dragMomentum={false}
            >
                <div className="menu-item">
                    <i className="fa-sharp fa-solid fa-house-chimney"></i>
                    {this.mainSidebarModel.width.get() > this.mainSidebarModel.collapseSnapPoint.get() && (
                        <span>Home</span>
                    )}
                </div>
                <div className="menu-item">
                    <i className="fa-sharp fa-solid fa-magnifying-glass"></i>
                    {this.mainSidebarModel.width.get() > this.mainSidebarModel.collapseSnapPoint.get() && (
                        <span>Search</span>
                    )}
                </div>
                {/* More menu items */}
            </motion.div>
        );
    }
}

export { MainSideBar };
