// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as React from "react";
import * as mobxReact from "mobx-react";
import * as mobx from "mobx";
import { sprintf } from "sprintf-js";
import { boundMethod } from "autobind-decorator";
import { For } from "tsx-control-statements/components";
import dayjs from "dayjs";
import localizedFormat from "dayjs/plugin/localizedFormat";
import { GlobalModel, GlobalCommandRunner, Session, Screen } from "../../../model/model";
import { ReactComponent as AddIcon } from "../../assets/icons/add.svg";
import * as constants from "../../appconst";
import { Reorder } from "framer-motion";
import { ScreenTab } from "./tab";

import "../workspace.less";
import "./tabs.less";

dayjs.extend(localizedFormat);

type OV<V> = mobx.IObservableValue<V>;

@mobxReact.observer
class ScreenTabs extends React.Component<
    { session: Session },
    { showingScreens: Screen[]; scrollIntoViewTimeout: number }
> {
    tabsRef: React.RefObject<any> = React.createRef();
    tabRefs: { [screenId: string]: React.RefObject<any> } = {};
    lastActiveScreenId: string = null;
    dragEndTimeout = null;
    scrollIntoViewTimeout = null;
    deltaYHistory = [];
    disposeScreensReaction = null;

    constructor(props: any) {
        super(props);
        this.state = {
            showingScreens: [],
            scrollIntoViewTimeout: null,
        };
    }

    componentDidMount(): void {
        this.componentDidUpdate();

        // Update showingScreens state when the screens change
        this.disposeScreensReaction = mobx.reaction(
            () => this.getScreens(),
            (screens) => {
                // Different timeout for when screens are added vs removed
                let timeout = 100;
                if (screens.length < this.state.showingScreens.length) {
                    timeout = 400;
                }
                this.setState({ showingScreens: screens, scrollIntoViewTimeout: timeout });
            }
        );

        // Add the wheel event listener to the tabsRef
        if (this.tabsRef.current) {
            this.tabsRef.current.addEventListener("wheel", this.handleWheel, { passive: false });
        }
    }

    componentWillUnmount() {
        if (this.dragEndTimeout) {
            clearTimeout(this.dragEndTimeout);
        }

        if (this.disposeScreensReaction) {
            this.disposeScreensReaction(); // Clean up the reaction
        }
    }

    componentDidUpdate(): void {
        // Scroll the active screen into view
        let activeScreenId = this.getActiveScreenId();
        if (activeScreenId !== this.lastActiveScreenId) {
            if (this.scrollIntoViewTimeout) {
                clearTimeout(this.scrollIntoViewTimeout);
            }

            this.scrollIntoViewTimeout = setTimeout(() => {
                if (this.tabsRef.current) {
                    let tabElem = this.tabsRef.current.querySelector(
                        sprintf('.screen-tab[data-screenid="%s"]', activeScreenId)
                    );
                    if (tabElem) {
                        tabElem.scrollIntoView();
                    }
                }
                this.lastActiveScreenId = activeScreenId;
            }, this.state.scrollIntoViewTimeout);
        }

        // Populate showingScreens state if it's empty
        let screens = this.getScreens();
        if (screens && this.state.showingScreens.length == 0) {
            this.setState({ showingScreens: screens });
        }
    }

    @boundMethod
    getActiveScreenId(): string {
        let { session } = this.props;
        if (session) {
            return session.activeScreenId.get();
        }
    }

    @mobx.computed
    @boundMethod
    getScreens(): Screen[] {
        let activeScreenId = this.getActiveScreenId();
        if (!activeScreenId) {
            return [];
        }

        let screens = GlobalModel.getSessionScreens(this.props.session.sessionId);
        let showingScreens = [];

        for (const screen of screens) {
            if (!screen.archived.get() || activeScreenId === screen.screenId) {
                showingScreens.push(screen);
            }
        }

        showingScreens.sort((a, b) => a.screenIdx.get() - b.screenIdx.get());

        return showingScreens;
    }

    @boundMethod
    handleNewScreen() {
        GlobalCommandRunner.createNewScreen();
    }

    @boundMethod
    handleSwitchScreen(screenId: string) {
        let { session } = this.props;
        if (session == null) {
            return;
        }
        if (session.activeScreenId.get() == screenId) {
            return;
        }
        let screen = session.getScreenById(screenId);
        if (screen == null) {
            return;
        }
        GlobalCommandRunner.switchScreen(screenId);
    }

    @boundMethod
    handleWheel(event: WheelEvent) {
        if (!this.tabsRef.current) return;

        // Add the current deltaY to the history
        this.deltaYHistory.push(Math.abs(event.deltaY));
        if (this.deltaYHistory.length > 5) {
            this.deltaYHistory.shift(); // Keep only the last 5 entries
        }

        // Check if any of the last 5 deltaY values are greater than a threshold
        let isMouseWheel = this.deltaYHistory.some((deltaY) => deltaY > 0);

        if (isMouseWheel) {
            // It's likely a mouse wheel event, so handle it for horizontal scrolling
            this.tabsRef.current.scrollLeft += event.deltaY;

            // Prevent default vertical scroll
            event.preventDefault();
        }
        // For touchpad events, do nothing and let the browser handle it
    }

    @boundMethod
    openScreenSettings(e: any, screen: Screen): void {
        e.preventDefault();
        e.stopPropagation();
        mobx.action(() => {
            GlobalModel.screenSettingsModal.set({ sessionId: screen.sessionId, screenId: screen.screenId });
        })();
        GlobalModel.modalsModel.pushModal(constants.SCREEN_SETTINGS);
    }

    render() {
        let { showingScreens } = this.state;
        let { session } = this.props;
        if (session == null) {
            return null;
        }
        let screen: Screen | null = null;
        let index = 0;
        let activeScreenId = this.getActiveScreenId();

        return (
            <div className="screen-tabs-container">
                <Reorder.Group
                    className="screen-tabs"
                    ref={this.tabsRef}
                    as="ul"
                    axis="x"
                    onReorder={(tabs: Screen[]) => {
                        this.setState({ showingScreens: tabs });
                    }}
                    values={showingScreens}
                >
                    <For each="screen" index="index" of={showingScreens}>
                        <React.Fragment key={screen.screenId}>
                            <ScreenTab
                                key={screen.screenId}
                                screen={screen}
                                activeScreenId={activeScreenId}
                                index={index}
                                onSwitchScreen={this.handleSwitchScreen}
                            />
                        </React.Fragment>
                    </For>
                </Reorder.Group>
                <div key="new-screen" className="new-screen" onClick={this.handleNewScreen}>
                    <AddIcon className="icon hoverEffect" />
                </div>
            </div>
        );
    }
}

export { ScreenTabs };
