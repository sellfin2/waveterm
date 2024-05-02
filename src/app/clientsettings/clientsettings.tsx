// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as React from "react";
import * as mobxReact from "mobx-react";
import * as mobx from "mobx";
import { boundMethod } from "autobind-decorator";
import { If } from "tsx-control-statements/components";
import { GlobalModel, GlobalCommandRunner, RemotesModel } from "@/models";
import { Toggle, InlineSettingsTextEdit, SettingsError, Dropdown } from "@/common/elements";
import { commandRtnHandler, isBlank } from "@/util/util";
import { getTermThemes } from "@/util/themeutil";
import * as appconst from "@/app/appconst";
import { MainView } from "@/common/elements/mainview";
import { OverlayScrollbarsComponent } from "overlayscrollbars-react";

import "./clientsettings.less";

class ClientSettingsKeybindings extends React.Component<{}, {}> {
    componentDidMount() {
        let clientSettingsViewModel = GlobalModel.clientSettingsViewModel;
        let keybindManager = GlobalModel.keybindManager;
        keybindManager.registerKeybinding("mainview", "clientsettings", "generic:cancel", (waveEvent) => {
            clientSettingsViewModel.closeView();
            return true;
        });
    }

    componentWillUnmount() {
        GlobalModel.keybindManager.unregisterDomain("clientsettings");
    }

    render() {
        return null;
    }
}

@mobxReact.observer
class ClientSettingsView extends React.Component<{ model: RemotesModel }, { hoveredItemId: string }> {
    errorMessage: OV<string> = mobx.observable.box(null, { name: "ClientSettings-errorMessage" });

    @boundMethod
    dismissError(): void {
        mobx.action(() => {
            this.errorMessage.set(null);
        })();
    }

    @boundMethod
    handleChangeFontSize(fontSize: string): void {
        const newFontSize = Number(fontSize);
        if (GlobalModel.getTermFontSize() == newFontSize) {
            return;
        }
        const prtn = GlobalCommandRunner.setTermFontSize(newFontSize, false);
        commandRtnHandler(prtn, this.errorMessage);
    }

    @boundMethod
    handleChangeFontFamily(fontFamily: string): void {
        if (GlobalModel.getTermFontFamily() == fontFamily) {
            return;
        }
        const prtn = GlobalCommandRunner.setTermFontFamily(fontFamily, false);
        commandRtnHandler(prtn, this.errorMessage);
    }

    @boundMethod
    handleChangeThemeSource(themeSource: NativeThemeSource): void {
        if (GlobalModel.getThemeSource() == themeSource) {
            return;
        }
        const prtn = GlobalCommandRunner.setTheme(themeSource, false);
        GlobalModel.getElectronApi().setNativeThemeSource(themeSource);
        commandRtnHandler(prtn, this.errorMessage);
    }

    @boundMethod
    handleChangeTermTheme(theme: string): void {
        // For root terminal theme, the key is root, otherwise it's either
        // sessionId or screenId.
        const currTheme = GlobalModel.getTermThemeSettings()["root"];
        if (currTheme == theme) {
            return;
        }
        const prtn = GlobalCommandRunner.setRootTermTheme(theme, false);
        commandRtnHandler(prtn, this.errorMessage);
    }

    @boundMethod
    handleChangeTelemetry(val: boolean): void {
        let prtn: Promise<CommandRtnType> = null;
        if (val) {
            prtn = GlobalCommandRunner.telemetryOn(false);
        } else {
            prtn = GlobalCommandRunner.telemetryOff(false);
        }
        commandRtnHandler(prtn, this.errorMessage);
    }

    @boundMethod
    handleChangeReleaseCheck(val: boolean): void {
        let prtn: Promise<CommandRtnType> = null;
        if (val) {
            prtn = GlobalCommandRunner.releaseCheckAutoOn(false);
        } else {
            prtn = GlobalCommandRunner.releaseCheckAutoOff(false);
        }
        commandRtnHandler(prtn, this.errorMessage);
        GlobalModel.getElectronApi().changeAutoUpdate(val);
    }

    @boundMethod
    handleChangeAutocompleteEnabled(val: boolean): void {
        const prtn: Promise<CommandRtnType> = GlobalCommandRunner.setAutocompleteEnabled(val);
        commandRtnHandler(prtn, this.errorMessage);
    }

    @boundMethod
    handleChangeAutocompleteDebuggingEnabled(val: boolean): void {
        mobx.action(() => {
            GlobalModel.autocompleteModel.loggingEnabled = val;
        })();
    }

    getFontSizes(): DropdownItem[] {
        const availableFontSizes: DropdownItem[] = [];
        for (let s = appconst.MinFontSize; s <= appconst.MaxFontSize; s++) {
            availableFontSizes.push({ label: s + "px", value: String(s) });
        }
        return availableFontSizes;
    }

    getFontFamilies(): DropdownItem[] {
        const availableFontFamilies: DropdownItem[] = [];
        availableFontFamilies.push({ label: "JetBrains Mono", value: "JetBrains Mono" });
        availableFontFamilies.push({ label: "Hack", value: "Hack" });
        availableFontFamilies.push({ label: "Fira Code", value: "Fira Code" });
        return availableFontFamilies;
    }

    getThemeSources(): DropdownItem[] {
        const themeSources: DropdownItem[] = [];
        themeSources.push({ label: "Dark", value: "dark" });
        themeSources.push({ label: "Light", value: "light" });
        themeSources.push({ label: "System", value: "system" });
        return themeSources;
    }

    @boundMethod
    inlineUpdateOpenAIModel(newModel: string): void {
        const prtn = GlobalCommandRunner.setClientOpenAISettings({ model: newModel });
        commandRtnHandler(prtn, this.errorMessage);
    }

    @boundMethod
    inlineUpdateOpenAIToken(newToken: string): void {
        const prtn = GlobalCommandRunner.setClientOpenAISettings({ apitoken: newToken });
        commandRtnHandler(prtn, this.errorMessage);
    }

    @boundMethod
    inlineUpdateOpenAIMaxTokens(newMaxTokensStr: string): void {
        const prtn = GlobalCommandRunner.setClientOpenAISettings({ maxtokens: newMaxTokensStr });
        commandRtnHandler(prtn, this.errorMessage);
    }

    @boundMethod
    inlineUpdateOpenAIBaseURL(newBaseURL: string): void {
        const prtn = GlobalCommandRunner.setClientOpenAISettings({ baseurl: newBaseURL });
        commandRtnHandler(prtn, this.errorMessage);
    }

    @boundMethod
    inlineUpdateOpenAITimeout(newTimeout: string): void {
        const prtn = GlobalCommandRunner.setClientOpenAISettings({ timeout: newTimeout });
        commandRtnHandler(prtn, this.errorMessage);
    }

    @boundMethod
    setErrorMessage(msg: string): void {
        mobx.action(() => {
            this.errorMessage.set(msg);
        })();
    }

    @boundMethod
    handleChangeShortcut(newShortcut: string): void {
        const prtn = GlobalCommandRunner.setGlobalShortcut(newShortcut);
        commandRtnHandler(prtn, this.errorMessage);
    }

    getFKeys(): DropdownItem[] {
        const opts: DropdownItem[] = [];
        opts.push({ label: "Disabled", value: "" });
        const platform = GlobalModel.getPlatform();
        for (let i = 1; i <= 12; i++) {
            const shortcut = (platform == "darwin" ? "Cmd" : "Alt") + "+F" + String(i);
            opts.push({ label: shortcut, value: shortcut });
        }
        return opts;
    }

    getCurrentShortcut(): string {
        const clientData = GlobalModel.clientData.get();
        return clientData?.clientopts?.globalshortcut ?? "";
    }

    @boundMethod
    handleClose() {
        GlobalModel.clientSettingsViewModel.closeView();
    }

    @boundMethod
    getSudoPwStoreOptions(): DropdownItem[] {
        const sudoCacheSources: DropdownItem[] = [];
        sudoCacheSources.push({ label: "On", value: "on" });
        sudoCacheSources.push({ label: "Off", value: "off" });
        sudoCacheSources.push({ label: "On Without Timeout", value: "notimeout" });
        return sudoCacheSources;
    }

    @boundMethod
    handleChangeSudoPwStoreConfig(store: string) {
        const prtn = GlobalCommandRunner.setSudoPwStore(store);
        commandRtnHandler(prtn, this.errorMessage);
    }

    @boundMethod
    handleChangeSudoPwTimeoutConfig(timeout: string) {
        if (Number(timeout) != 0) {
            const prtn = GlobalCommandRunner.setSudoPwTimeout(timeout);
            commandRtnHandler(prtn, this.errorMessage);
        }
    }

    @boundMethod
    handleChangeSudoPwClearOnSleepConfig(clearOnSleep: boolean) {
        const prtn = GlobalCommandRunner.setSudoPwClearOnSleep(clearOnSleep);
        commandRtnHandler(prtn, this.errorMessage);
    }

    render() {
        const isHidden = GlobalModel.activeMainView.get() != "clientsettings";
        if (isHidden) {
            return null;
        }

        const cdata: ClientDataType = GlobalModel.clientData.get();
        const openAIOpts = cdata.openaiopts ?? {};
        const apiTokenStr = isBlank(openAIOpts.apitoken) ? "(not set)" : "********";
        const maxTokensStr = String(
            openAIOpts.maxtokens == null || openAIOpts.maxtokens == 0 ? 1000 : openAIOpts.maxtokens
        );
        const aiTimeoutStr = String(
            openAIOpts.timeout == null || openAIOpts.timeout == 0 ? 10 : openAIOpts.timeout / 1000
        );
        const curFontSize = GlobalModel.getTermFontSize();
        const curFontFamily = GlobalModel.getTermFontFamily();
        const curTheme = GlobalModel.getThemeSource();
        const termThemes = getTermThemes(GlobalModel.termThemes.get(), "Wave Default");
        const currTermTheme = GlobalModel.getTermThemeSettings()["root"] ?? termThemes[0].label;
        const curSudoPwStore = GlobalModel.getSudoPwStore();
        const curSudoPwTimeout = String(GlobalModel.getSudoPwTimeout());
        const curSudoPwClearOnSleep = GlobalModel.getSudoPwClearOnSleep();

        return (
            <MainView
                className="clientsettings-view"
                title="Client Settings"
                onClose={this.handleClose}
                scrollable={true}
            >
                <If condition={!isHidden}>
                    <ClientSettingsKeybindings></ClientSettingsKeybindings>
                </If>
                <div className="content">
                    <div className="settings-field">
                        <div className="settings-label">Term Font Size</div>
                        <div className="settings-input">
                            <Dropdown
                                className="font-size-dropdown"
                                options={this.getFontSizes()}
                                defaultValue={`${curFontSize}px`}
                                onChange={this.handleChangeFontSize}
                            />
                        </div>
                    </div>
                    <div className="settings-field">
                        <div className="settings-label">Term Font Family</div>
                        <div className="settings-input">
                            <Dropdown
                                className="font-size-dropdown"
                                options={this.getFontFamilies()}
                                defaultValue={curFontFamily}
                                onChange={this.handleChangeFontFamily}
                            />
                        </div>
                    </div>
                    <div className="settings-field">
                        <div className="settings-label">Theme</div>
                        <div className="settings-input">
                            <Dropdown
                                className="theme-dropdown"
                                options={this.getThemeSources()}
                                defaultValue={curTheme}
                                onChange={this.handleChangeThemeSource}
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
                        <div className="settings-label">Client ID</div>
                        <div className="settings-input">{cdata.clientid}</div>
                    </div>
                    <div className="settings-field">
                        <div className="settings-label">Client Version</div>
                        <div className="settings-input">
                            {appconst.VERSION} {appconst.BUILD}
                        </div>
                    </div>
                    <div className="settings-field">
                        <div className="settings-label">DB Version</div>
                        <div className="settings-input">{cdata.dbversion}</div>
                    </div>
                    <div className="settings-field">
                        <div className="settings-label">Basic Telemetry</div>
                        <div className="settings-input">
                            <Toggle checked={!cdata.clientopts.notelemetry} onChange={this.handleChangeTelemetry} />
                        </div>
                    </div>
                    <div className="settings-field">
                        <div className="settings-label">Check for Updates</div>
                        <div className="settings-input">
                            <Toggle
                                checked={!cdata.clientopts.noreleasecheck}
                                onChange={this.handleChangeReleaseCheck}
                            />
                        </div>
                    </div>
                    <div className="settings-field">
                        <div className="settings-label">AI Token</div>
                        <div className="settings-input">
                            <InlineSettingsTextEdit
                                placeholder=""
                                text={apiTokenStr}
                                value={""}
                                onChange={this.inlineUpdateOpenAIToken}
                                maxLength={100}
                                showIcon={true}
                            />
                        </div>
                    </div>
                    <div className="settings-field">
                        <div className="settings-label">AI Base URL</div>
                        <div className="settings-input">
                            <InlineSettingsTextEdit
                                placeholder=""
                                text={isBlank(openAIOpts.baseurl) ? "openai default" : openAIOpts.baseurl}
                                value={openAIOpts.baseurl ?? ""}
                                onChange={this.inlineUpdateOpenAIBaseURL}
                                maxLength={200}
                                showIcon={true}
                            />
                        </div>
                    </div>
                    <div className="settings-field">
                        <div className="settings-label">AI Model</div>
                        <div className="settings-input">
                            <InlineSettingsTextEdit
                                placeholder="gpt-3.5-turbo"
                                text={isBlank(openAIOpts.model) ? "gpt-3.5-turbo" : openAIOpts.model}
                                value={openAIOpts.model ?? ""}
                                onChange={this.inlineUpdateOpenAIModel}
                                maxLength={100}
                                showIcon={true}
                            />
                        </div>
                    </div>
                    <div className="settings-field">
                        <div className="settings-label">AI MaxTokens</div>
                        <div className="settings-input">
                            <InlineSettingsTextEdit
                                placeholder=""
                                text={maxTokensStr}
                                value={maxTokensStr}
                                onChange={this.inlineUpdateOpenAIMaxTokens}
                                maxLength={10}
                                showIcon={true}
                            />
                        </div>
                    </div>
                    <div className="settings-field">
                        <div className="settings-label">AI Timeout (seconds)</div>
                        <div className="settings-input">
                            <InlineSettingsTextEdit
                                placeholder=""
                                text={aiTimeoutStr}
                                value={aiTimeoutStr}
                                onChange={this.inlineUpdateOpenAITimeout}
                                maxLength={10}
                                showIcon={true}
                            />
                        </div>
                    </div>
                    <div className="settings-field">
                        <div className="settings-label">Global Hotkey</div>
                        <div className="settings-input">
                            <Dropdown
                                className="hotkey-dropdown"
                                options={this.getFKeys()}
                                defaultValue={this.getCurrentShortcut()}
                                onChange={this.handleChangeShortcut}
                            />
                        </div>
                    </div>
                    <div className="settings-field">
                        <div className="settings-label">Remember Sudo Password</div>
                        <div className="settings-input">
                            <Dropdown
                                className="hotkey-dropdown"
                                options={this.getSudoPwStoreOptions()}
                                defaultValue={curSudoPwStore}
                                onChange={this.handleChangeSudoPwStoreConfig}
                            />
                        </div>
                    </div>
                    <div className="settings-field">
                        <div className="settings-label">Sudo Timeout (Minutes)</div>
                        <div className="settings-input">
                            <InlineSettingsTextEdit
                                placeholder=""
                                text={curSudoPwTimeout}
                                value={curSudoPwTimeout}
                                onChange={this.handleChangeSudoPwTimeoutConfig}
                                maxLength={6}
                                showIcon={true}
                                isNumber={true}
                            />
                        </div>
                    </div>
                    <div className="settings-field">
                        <div className="settings-label">Clear Sudo Password on Sleep</div>
                        <div className="settings-input">
                            <Toggle
                                checked={curSudoPwClearOnSleep}
                                onChange={this.handleChangeSudoPwClearOnSleepConfig}
                            />
                        </div>
                    </div>
                    <div className="settings-field">
                        <div className="settings-label">Command Autocomplete</div>
                        <div className="settings-input">
                            <Toggle
                                checked={cdata.clientopts.autocompleteenabled ?? false}
                                onChange={this.handleChangeAutocompleteEnabled}
                            />
                        </div>
                    </div>
                    <div className="settings-field">
                        <div className="settings-label">Command Autocomplete Debugging</div>
                        <div className="settings-input">
                            <Toggle
                                checked={GlobalModel.autocompleteModel.loggingEnabled}
                                onChange={this.handleChangeAutocompleteDebuggingEnabled}
                            />
                        </div>
                    </div>
                    <SettingsError errorMessage={this.errorMessage} />
                </div>
            </MainView>
        );
    }
}

export { ClientSettingsView };
