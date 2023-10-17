// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as React from "react";
import * as mobxReact from "mobx-react";
import * as mobx from "mobx";
import { boundMethod } from "autobind-decorator";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import cn from "classnames";
import { If } from "tsx-control-statements/components";
import type { RemoteType } from "../../types/types";

import { ReactComponent as CheckIcon } from "../assets/icons/line/check.svg";
import { ReactComponent as CopyIcon } from "../assets/icons/history/copy.svg";
import { ReactComponent as CircleIcon } from "../assets/icons/circle.svg";
import { ReactComponent as KeyIcon } from "../assets/icons/key.svg";
import { ReactComponent as XMarkIcon } from "../assets/icons/line/xmark.svg";
import { ReactComponent as RotateIcon } from "../assets/icons/rotate_left.svg";
import { ReactComponent as CircleInfoIcon } from "../assets/icons/circle_info.svg";
import { ReactComponent as PenIcon } from "../assets/icons/favourites/pen.svg";

import "./common.less";

type OV<V> = mobx.IObservableValue<V>;

function renderCmdText(text: string): any {
    return <span>&#x2318;{text}</span>;
}

class CmdStrCode extends React.Component<
    {
        cmdstr: string;
        onUse: () => void;
        onCopy: () => void;
        isCopied: boolean;
        fontSize: "normal" | "large";
        limitHeight: boolean;
    },
    {}
> {
    @boundMethod
    handleUse(e: any) {
        e.stopPropagation();
        if (this.props.onUse != null) {
            this.props.onUse();
        }
    }

    @boundMethod
    handleCopy(e: any) {
        e.stopPropagation();
        if (this.props.onCopy != null) {
            this.props.onCopy();
        }
    }

    render() {
        let { isCopied, cmdstr, fontSize, limitHeight } = this.props;
        return (
            <div className={cn("cmdstr-code", { "is-large": fontSize == "large" }, { "limit-height": limitHeight })}>
                <If condition={isCopied}>
                    <div key="copied" className="copied-indicator">
                        <div>copied</div>
                    </div>
                </If>
                <div key="use" className="use-button hoverEffect" title="Use Command" onClick={this.handleUse}>
                    <CheckIcon className="icon" />
                </div>
                <div key="code" className="code-div">
                    <code>{cmdstr}</code>
                </div>
                <div key="copy" className="copy-control hoverEffect">
                    <div className="inner-copy" onClick={this.handleCopy} title="copy">
                        <CopyIcon className="icon" />
                    </div>
                </div>
            </div>
        );
    }
}

class Toggle extends React.Component<{ checked: boolean; onChange: (value: boolean) => void }, {}> {
    @boundMethod
    handleChange(e: any): void {
        let { onChange } = this.props;
        if (onChange != null) {
            onChange(e.target.checked);
        }
    }

    render() {
        return (
            <label className="checkbox-toggle">
                <input type="checkbox" checked={this.props.checked} onChange={this.handleChange} />
                <span className="slider" />
            </label>
        );
    }
}

@mobxReact.observer
class RemoteStatusLight extends React.Component<{ remote: RemoteType }, {}> {
    render() {
        let remote = this.props.remote;
        let status = "error";
        let wfp = false;
        if (remote != null) {
            status = remote.status;
            wfp = remote.waitingforpassword;
        }
        if (status == "connecting") {
            if (wfp) return <KeyIcon className={`remote-status status-${status}`} />;
            else return <RotateIcon className={`remote-status status-${status}`} />;
        }
        return <CircleIcon className={`remote-status status-${status}`} />;
    }
}

@mobxReact.observer
class InlineSettingsTextEdit extends React.Component<
    {
        text: string;
        value: string;
        onChange: (val: string) => void;
        maxLength: number;
        placeholder: string;
        showIcon?: boolean;
    },
    {}
> {
    isEditing: OV<boolean> = mobx.observable.box(false, { name: "inlineedit-isEditing" });
    tempText: OV<string>;
    shouldFocus: boolean = false;
    inputRef: React.RefObject<any> = React.createRef();

    componentDidUpdate(): void {
        if (this.shouldFocus) {
            this.shouldFocus = false;
            if (this.inputRef.current != null) {
                this.inputRef.current.focus();
            }
        }
    }

    @boundMethod
    handleChangeText(e: any): void {
        mobx.action(() => {
            this.tempText.set(e.target.value);
        })();
    }

    @boundMethod
    confirmChange(): void {
        mobx.action(() => {
            let newText = this.tempText.get();
            this.isEditing.set(false);
            this.tempText = null;
            this.props.onChange(newText);
        })();
    }

    @boundMethod
    cancelChange(): void {
        mobx.action(() => {
            this.isEditing.set(false);
            this.tempText = null;
        })();
    }

    @boundMethod
    handleKeyDown(e: any): void {
        if (e.code == "Enter") {
            e.preventDefault();
            e.stopPropagation();
            this.confirmChange();
            return;
        }
        if (e.code == "Escape") {
            e.preventDefault();
            e.stopPropagation();
            this.cancelChange();
            return;
        }
        return;
    }

    @boundMethod
    clickEdit(): void {
        mobx.action(() => {
            this.isEditing.set(true);
            this.shouldFocus = true;
            this.tempText = mobx.observable.box(this.props.value, { name: "inlineedit-tempText" });
        })();
    }

    render() {
        if (this.isEditing.get()) {
            return (
                <div className={cn("settings-input inline-edit", "edit-active")}>
                    <div className="field has-addons">
                        <div className="control">
                            <input
                                ref={this.inputRef}
                                className="input"
                                type="text"
                                onKeyDown={this.handleKeyDown}
                                placeholder={this.props.placeholder}
                                onChange={this.handleChangeText}
                                value={this.tempText.get()}
                                maxLength={this.props.maxLength}
                            />
                        </div>
                        <div className="control">
                            <div
                                onClick={this.cancelChange}
                                title="Cancel (Esc)"
                                className="button is-prompt-danger is-outlined is-small"
                            >
                                <XMarkIcon className="icon" />
                            </div>
                        </div>
                        <div className="control">
                            <div
                                onClick={this.confirmChange}
                                title="Confirm (Enter)"
                                className="button is-prompt-green is-outlined is-small"
                            >
                                <CheckIcon className="icon" />
                            </div>
                        </div>
                    </div>
                </div>
            );
        } else {
            return (
                <div onClick={this.clickEdit} className={cn("settings-input inline-edit", "edit-not-active")}>
                    {this.props.text}
                    <If condition={this.props.showIcon}>
                        <PenIcon className="icon" />
                    </If>
                </div>
            );
        }
    }
}

@mobxReact.observer
class InfoMessage extends React.Component<{ width: number; children: React.ReactNode }> {
    render() {
        return (
            <div className="info-message">
                <div className="message-icon">
                    <CircleInfoIcon className="icon" />
                </div>
                <div className="message-content" style={{ width: this.props.width }}>
                    <div className="info-icon">
                        <CircleInfoIcon className="icon" />
                    </div>
                    <div className="info-children">{this.props.children}</div>
                </div>
            </div>
        );
    }
}

function LinkRenderer(props: any): any {
    let newUrl = "https://extern?" + encodeURIComponent(props.href);
    return (
        <a href={newUrl} target="_blank">
            {props.children}
        </a>
    );
}

function HeaderRenderer(props: any, hnum: number): any {
    return <div className={cn("title", "is-" + hnum)}>{props.children}</div>;
}

function CodeRenderer(props: any): any {
    return <code className={cn({ inline: props.inline })}>{props.children}</code>;
}

@mobxReact.observer
class Markdown extends React.Component<{ text: string; style?: any; extraClassName?: string }, {}> {
    render() {
        let text = this.props.text;
        let markdownComponents = {
            a: LinkRenderer,
            h1: (props) => HeaderRenderer(props, 1),
            h2: (props) => HeaderRenderer(props, 2),
            h3: (props) => HeaderRenderer(props, 3),
            h4: (props) => HeaderRenderer(props, 4),
            h5: (props) => HeaderRenderer(props, 5),
            h6: (props) => HeaderRenderer(props, 6),
            code: CodeRenderer,
        };
        return (
            <div className={cn("markdown content", this.props.extraClassName)} style={this.props.style}>
                <ReactMarkdown children={text} remarkPlugins={[remarkGfm]} components={markdownComponents} />
            </div>
        );
    }
}

@mobxReact.observer
class SettingsError extends React.Component<{ errorMessage: OV<string> }, {}> {
    @boundMethod
    dismissError(): void {
        mobx.action(() => {
            this.props.errorMessage.set(null);
        })();
    }

    render() {
        if (this.props.errorMessage.get() == null) {
            return null;
        }
        return (
            <div className="settings-field settings-error">
                <div>Error: {this.props.errorMessage.get()}</div>
                <div className="flex-spacer" />
                <div onClick={this.dismissError} className="error-dismiss">
                    <i className="fa-sharp fa-solid fa-xmark" />
                </div>
            </div>
        );
    }
}

export {
    CmdStrCode,
    Toggle,
    renderCmdText,
    RemoteStatusLight,
    InlineSettingsTextEdit,
    InfoMessage,
    Markdown,
    SettingsError,
};
