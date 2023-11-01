// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as React from "react";
import * as mobxReact from "mobx-react";
import * as mobx from "mobx";
import { boundMethod } from "autobind-decorator";
import { If, For } from "tsx-control-statements/components";
import cn from "classnames";
import dayjs from "dayjs";
import localizedFormat from "dayjs/plugin/localizedFormat";
import { GlobalModel, GlobalCommandRunner } from "../../../model/model";
import { Markdown } from "../common";
import * as util from "../../../util/util";
import { Toggle, Checkbox } from "../common";
import { ClientDataType } from "../../../types/types";

import { ReactComponent as XmarkIcon } from "../../assets/icons/line/xmark.svg";
import { ReactComponent as WarningIcon } from "../../assets/icons/line/triangle-exclamation.svg";
import shield from "../../assets/icons/shield_check.svg";
import help from "../../assets/icons/help_filled.svg";
import github from "../../assets/icons/github.svg";

dayjs.extend(localizedFormat);

type OV<V> = mobx.IObservableValue<V>;

@mobxReact.observer
class DisconnectedModal extends React.Component<{}, {}> {
    logRef: any = React.createRef();
    showLog: mobx.IObservableValue<boolean> = mobx.observable.box(false);

    @boundMethod
    restartServer() {
        GlobalModel.restartWaveSrv();
    }

    @boundMethod
    tryReconnect() {
        GlobalModel.ws.connectNow("manual");
    }

    componentDidMount() {
        if (this.logRef.current != null) {
            this.logRef.current.scrollTop = this.logRef.current.scrollHeight;
        }
    }

    componentDidUpdate() {
        if (this.logRef.current != null) {
            this.logRef.current.scrollTop = this.logRef.current.scrollHeight;
        }
    }

    @boundMethod
    handleShowLog(): void {
        mobx.action(() => {
            this.showLog.set(!this.showLog.get());
        })();
    }

    render() {
        let model = GlobalModel;
        let logLine: string = null;
        let idx: number = 0;
        return (
            <div className="prompt-modal disconnected-modal modal is-active">
                <div className="modal-background"></div>
                <div className="modal-content">
                    <div className="message-header">
                        <div className="modal-title">Wave Client Disconnected</div>
                    </div>
                    <If condition={this.showLog.get()}>
                        <div className="inner-content">
                            <div className="ws-log" ref={this.logRef}>
                                <For each="logLine" index="idx" of={GlobalModel.ws.wsLog}>
                                    <div key={idx} className="ws-logline">
                                        {logLine}
                                    </div>
                                </For>
                            </div>
                        </div>
                    </If>
                    <footer>
                        <div className="footer-text-link" style={{ marginLeft: 10 }} onClick={this.handleShowLog}>
                            <If condition={!this.showLog.get()}>
                                <i className="fa-sharp fa-solid fa-plus" /> Show Log
                            </If>
                            <If condition={this.showLog.get()}>
                                <i className="fa-sharp fa-solid fa-minus" /> Hide Log
                            </If>
                        </div>
                        <div className="flex-spacer" />
                        <button onClick={this.tryReconnect} className="button">
                            <span className="icon">
                                <i className="fa-sharp fa-solid fa-rotate" />
                            </span>
                            <span>Try Reconnect</span>
                        </button>
                        <button onClick={this.restartServer} className="button is-danger" style={{ marginLeft: 10 }}>
                            <WarningIcon className="icon" />
                            <span>Restart Server</span>
                        </button>
                    </footer>
                </div>
            </div>
        );
    }
}

@mobxReact.observer
class ClientStopModal extends React.Component<{}, {}> {
    @boundMethod
    refreshClient() {
        GlobalModel.refreshClient();
    }

    render() {
        let model = GlobalModel;
        let cdata = model.clientData.get();
        let title = "Client Not Ready";
        return (
            <div className="prompt-modal client-stop-modal modal is-active">
                <div className="modal-background"></div>
                <div className="modal-content">
                    <div className="message-header">
                        <div className="modal-title">{title}</div>
                    </div>
                    <div className="inner-content">
                        <If condition={cdata == null}>
                            <div>Cannot get client data.</div>
                        </If>
                    </div>
                    <footer>
                        <button onClick={this.refreshClient} className="button">
                            <span className="icon">
                                <i className="fa-sharp fa-solid fa-rotate" />
                            </span>
                            <span>Hard Refresh Client</span>
                        </button>
                    </footer>
                </div>
            </div>
        );
    }
}

@mobxReact.observer
class LoadingSpinner extends React.Component<{}, {}> {
    render() {
        return (
            <div className="loading-spinner">
                <div></div>
                <div></div>
                <div></div>
                <div></div>
            </div>
        );
    }
}

@mobxReact.observer
class AlertModal extends React.Component<{}, {}> {
    @boundMethod
    closeModal(): void {
        GlobalModel.cancelAlert();
    }

    @boundMethod
    handleOK(): void {
        GlobalModel.confirmAlert();
    }

    render() {
        let message = GlobalModel.alertMessage.get();
        if (message == null) {
            return null;
        }
        let title = message.title ?? (message.confirm ? "Confirm" : "Alert");
        let isConfirm = message.confirm;
        return (
            <div className="modal prompt-modal is-active alert-modal">
                <div className="modal-background" />
                <div className="modal-content">
                    <header>
                        <p className="modal-title">
                            <WarningIcon className="icon" />
                            {title}
                        </p>
                        <div className="close-icon hoverEffect" title="Close (Escape)" onClick={this.closeModal}>
                            <XmarkIcon />
                        </div>
                    </header>
                    <If condition={message.markdown}>
                        <Markdown text={message.message} extraClassName="inner-content" />
                    </If>
                    <If condition={!message.markdown}>
                        <div className="inner-content content">
                            <p>{message.message}</p>
                        </div>
                    </If>
                    <footer>
                        <If condition={isConfirm}>
                            <div onClick={this.closeModal} className="button is-prompt-cancel is-outlined is-small">
                                Cancel
                            </div>
                            <div onClick={this.handleOK} className="button is-prompt-green is-outlined is-small">
                                OK
                            </div>
                        </If>
                        <If condition={!isConfirm}>
                            <div onClick={this.handleOK} className="button is-prompt-green is-small">
                                OK
                            </div>
                        </If>
                    </footer>
                </div>
            </div>
        );
    }
}

@mobxReact.observer
class TosModal extends React.Component<{}, {}> {
    state = {
        isChecked: false,
    };

    @boundMethod
    handleCheckboxChange(checked: boolean): void {
        this.setState({ isChecked: checked });
    }

    @boundMethod
    acceptTos(): void {
        GlobalCommandRunner.clientAcceptTos();
    }

    @boundMethod
    handleChangeTelemetry(val: boolean): void {
        if (val) {
            GlobalCommandRunner.telemetryOn(false);
        } else {
            GlobalCommandRunner.telemetryOff(false);
        }
    }

    render() {
        let cdata: ClientDataType = GlobalModel.clientData.get();

        return (
            <div className={cn("modal tos-modal wave-modal is-active")}>
                <div className="modal-background" />
                <div className="modal-content">
                    <div className="modal-content-wrapper">
                        <header>
                            <div className="modal-title">Welcome to Wave Terminal!</div>
                            <div className="modal-subtitle">Lets set everything for you</div>
                        </header>
                        <div className="content">
                            <div className="item">
                                <img src={shield} alt="Privacy" />
                                <div className="item-inner">
                                    <div className="item-title">Telemetry</div>
                                    <div className="item-text">
                                        We don’t collect any personal info, only crash logs and IP address to make Wave
                                        better. If you like, you can disable telemetry now or late.
                                    </div>
                                    <div className="item-field">
                                        <Toggle
                                            checked={!cdata.clientopts.notelemetry}
                                            onChange={this.handleChangeTelemetry}
                                        />
                                        <div className="item-label">Basic Telemetry</div>
                                    </div>
                                </div>
                            </div>
                            <div className="item">
                                <img src={help} alt="Help" />
                                <div className="item-inner">
                                    <div className="item-title">Help</div>
                                    <div className="item-text">
                                        If you need any help or you have feature request, you can join{" "}
                                        <a target="_blank" href={util.makeExternLink("https://discord.gg/XfvZ334gwU")}>
                                            our Discord
                                        </a>
                                        .
                                    </div>
                                </div>
                            </div>
                            <div className="item">
                                <img src={github} alt="Github" />
                                <div className="item-inner">
                                    <div className="item-title">Like Wave? Give us a star</div>
                                    <div className="item-text">
                                        Rankings are very important for small startups like us, it helps other people to
                                        know about us. If you like Wave, please consider giving us a star on our{" "}
                                        <a
                                            target="_blank"
                                            href={util.makeExternLink("https://github.com/wavetermdev/waveterm")}
                                        >
                                            Github Repository
                                        </a>
                                        .
                                    </div>
                                </div>
                            </div>
                        </div>
                        <footer>
                            <div>
                                <Checkbox
                                    checked={this.state.isChecked}
                                    label="I accept the Terms of Service"
                                    id="accept-tos"
                                    onChange={this.handleCheckboxChange}
                                />
                            </div>
                            <div className="button-wrapper">
                                <button
                                    onClick={this.acceptTos}
                                    className={cn("button is-wave-green is-outlined is-small", {
                                        "disabled-button": !this.state.isChecked,
                                    })}
                                    disabled={!this.state.isChecked}
                                >
                                    Continue
                                </button>
                            </div>
                        </footer>
                    </div>
                </div>
            </div>
        );
    }
}

export { WelcomeModal, LoadingSpinner, ClientStopModal, AlertModal, DisconnectedModal, TosModal };
