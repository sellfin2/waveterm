// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as React from "react";
import * as mobxReact from "mobx-react";
import * as mobx from "mobx";
import { boundMethod } from "autobind-decorator";
import { GlobalModel, GlobalCommandRunner, Session } from "@/models";
import { If } from "tsx-control-statements/components";
import { Toggle, InlineSettingsTextEdit, SettingsError, Modal, Tooltip, Button, Dropdown } from "@/elements";
import { commandRtnHandler } from "@/util/util";
import { getTermThemes } from "@/util/themeutil";
import * as util from "@/util/util";

import "./sessionsettings.less";

const SessionDeleteMessage = `
Are you sure you want to delete this workspace?
`.trim();

@mobxReact.observer
class SessionSettingsModal extends React.Component<{}, {}> {
    errorMessage: OV<string> = mobx.observable.box(null, { name: "ScreenSettings-errorMessage" });
    session: Session;
    sessionId: string;

    constructor(props: any) {
        super(props);
        this.sessionId = GlobalModel.sessionSettingsModal.get();
        this.session = GlobalModel.getSessionById(this.sessionId);
        if (this.session == null) {
            return;
        }
    }

    @boundMethod
    closeModal(): void {
        mobx.action(() => {
            GlobalModel.sessionSettingsModal.set(null);
        })();
        GlobalModel.modalsModel.popModal();
    }

    @boundMethod
    handleInlineChangeName(newVal: string): void {
        if (this.session == null) {
            return;
        }
        if (util.isStrEq(newVal, this.session.name.get())) {
            return;
        }
        let prtn = GlobalCommandRunner.sessionSetSettings(this.sessionId, { name: newVal }, false);
        util.commandRtnHandler(prtn, this.errorMessage);
    }

    @boundMethod
    handleChangeArchived(val: boolean): void {
        if (this.session == null) {
            return;
        }
        if (this.session.archived.get() == val) {
            return;
        }
        let prtn = GlobalCommandRunner.sessionArchive(this.sessionId, val);
        util.commandRtnHandler(prtn, this.errorMessage);
    }

    @boundMethod
    handleDeleteSession(): void {
        let message = SessionDeleteMessage;
        let alertRtn = GlobalModel.showAlert({ message: message, confirm: true, markdown: true });
        alertRtn.then((result) => {
            if (!result) {
                return;
            }
            let prtn = GlobalCommandRunner.sessionDelete(this.sessionId);
            util.commandRtnHandler(prtn, this.errorMessage, () => GlobalModel.modalsModel.popModal());
        });
    }

    @boundMethod
    handleChangeTermTheme(theme: string): void {
        const currTheme = GlobalModel.getTermThemeSettings()[this.sessionId];
        if (currTheme == theme) {
            return;
        }
        const prtn = GlobalCommandRunner.setSessionTermTheme(this.sessionId, theme, false);
        commandRtnHandler(prtn, this.errorMessage);
    }

    @boundMethod
    dismissError(): void {
        mobx.action(() => {
            this.errorMessage.set(null);
        })();
    }

    render() {
        if (this.session == null) {
            return null;
        }
        const termThemes = getTermThemes(GlobalModel.termThemes.get());
        const currTermTheme = GlobalModel.getTermThemeSettings()[this.sessionId] ?? termThemes[0].label;

        return (
            <Modal className="session-settings-modal">
                <Modal.Header onClose={this.closeModal} title={`Workspace Settings (${this.session.name.get()})`} />
                <div className="wave-modal-body">
                    <div className="settings-field">
                        <div className="settings-label">Name</div>
                        <div className="settings-input">
                            <InlineSettingsTextEdit
                                placeholder="name"
                                text={this.session.name.get() ?? "(none)"}
                                value={this.session.name.get() ?? ""}
                                onChange={this.handleInlineChangeName}
                                maxLength={50}
                                showIcon={true}
                            />
                        </div>
                    </div>
                    <If condition={termThemes.length > 0}>
                        <div className="settings-field">
                            <div className="settings-label">Terminal Theme</div>
                            <div className="settings-input">
                                <Dropdown
                                    className="terminal-theme-dropdown"
                                    options={termThemes}
                                    defaultValue={currTermTheme}
                                    onChange={this.handleChangeTermTheme}
                                />
                            </div>
                        </div>
                    </If>
                    <div className="settings-field">
                        <div className="settings-label">
                            <div>Archived</div>
                            <Tooltip
                                className="session-settings-tooltip"
                                message="Archive will hide the workspace from the active menu. Commands and output will be
                                retained, but hidden."
                                icon={<i className="fa-sharp fa-regular fa-circle-question" />}
                            >
                                {<i className="fa-sharp fa-regular fa-circle-question" />}
                            </Tooltip>
                        </div>
                        <div className="settings-input">
                            <Toggle checked={this.session.archived.get()} onChange={this.handleChangeArchived} />
                        </div>
                    </div>
                    <div className="settings-field">
                        <div className="settings-label">
                            <div>Actions</div>
                            <Tooltip
                                className="session-settings-tooltip"
                                message="Delete will remove the workspace, deleting all commands and output."
                                icon={<i className="fa-sharp fa-regular fa-circle-question" />}
                            >
                                {<i className="fa-sharp fa-regular fa-circle-question" />}
                            </Tooltip>
                        </div>
                        <div className="settings-input">
                            <Button onClick={this.handleDeleteSession} className="secondary small danger">
                                Delete Workspace
                            </Button>
                        </div>
                    </div>
                    <SettingsError errorMessage={this.errorMessage} />
                </div>
                <Modal.Footer cancelLabel="Close" onCancel={this.closeModal} keybindings={true} />
            </Modal>
        );
    }
}

export { SessionSettingsModal };
