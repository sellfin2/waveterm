// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as React from "react";
import * as mobxReact from "mobx-react";
import * as mobx from "mobx";
import { GlobalModel } from "@/models";
import { isBlank } from "@/util/util";
import { boundMethod } from "autobind-decorator";
import cn from "classnames";
import { For } from "tsx-control-statements/components";
import { Markdown } from "@/elements";
import { checkKeyPressed, adaptFromReactOrNativeKeyEvent } from "@/util/keyutil";

@mobxReact.observer
class AIChat extends React.Component<{}, {}> {
    chatListKeyCount: number = 0;
    textAreaNumLines: mobx.IObservableValue<number> = mobx.observable.box(1, { name: "textAreaNumLines" });
    chatWindowScrollRef: React.RefObject<HTMLDivElement>;
    textAreaRef: React.RefObject<HTMLTextAreaElement>;

    constructor(props: any) {
        super(props);
        this.chatWindowScrollRef = React.createRef();
        this.textAreaRef = React.createRef();
    }

    componentDidMount() {
        let model = GlobalModel;
        let inputModel = model.inputModel;
        if (this.chatWindowScrollRef != null && this.chatWindowScrollRef.current != null) {
            this.chatWindowScrollRef.current.scrollTop = this.chatWindowScrollRef.current.scrollHeight;
        }
        if (this.textAreaRef.current != null) {
            this.textAreaRef.current.focus();
            inputModel.setCmdInfoChatRefs(this.textAreaRef, this.chatWindowScrollRef);
        }
        this.requestChatUpdate();
    }

    componentDidUpdate() {
        if (this.chatWindowScrollRef != null && this.chatWindowScrollRef.current != null) {
            this.chatWindowScrollRef.current.scrollTop = this.chatWindowScrollRef.current.scrollHeight;
        }
    }

    requestChatUpdate() {
        this.submitChatMessage("");
    }

    submitChatMessage(messageStr: string) {
        let model = GlobalModel;
        let inputModel = model.inputModel;
        let curLine = inputModel.getCurLine();
        let prtn = GlobalModel.submitChatInfoCommand(messageStr, curLine, false);
        prtn.then((rtn) => {
            if (!rtn.success) {
                console.log("submit chat command error: " + rtn.error);
            }
        }).catch((error) => {});
    }

    getLinePos(elem: any): { numLines: number; linePos: number } {
        let numLines = elem.value.split("\n").length;
        let linePos = elem.value.substr(0, elem.selectionStart).split("\n").length;
        return { numLines, linePos };
    }

    @mobx.action
    @boundMethod
    onKeyDown(e: any) {
        mobx.action(() => {
            let model = GlobalModel;
            let inputModel = model.inputModel;
            let ctrlMod = e.getModifierState("Control") || e.getModifierState("Meta") || e.getModifierState("Shift");
            let resetCodeSelect = !ctrlMod;
            let waveEvent = adaptFromReactOrNativeKeyEvent(e);
            if (checkKeyPressed(waveEvent, "Enter")) {
                e.preventDefault();
                if (!ctrlMod) {
                    if (inputModel.getCodeSelectSelectedIndex() == -1) {
                        let messageStr = e.target.value;
                        this.submitChatMessage(messageStr);
                        e.target.value = "";
                    } else {
                        inputModel.grabCodeSelectSelection();
                    }
                } else {
                    e.target.setRangeText("\n", e.target.selectionStart, e.target.selectionEnd, "end");
                }
            }
            if (checkKeyPressed(waveEvent, "Escape")) {
                e.preventDefault();
                e.stopPropagation();
                inputModel.closeAIAssistantChat(true);
            }

            if (checkKeyPressed(waveEvent, "Ctrl:l")) {
                e.preventDefault();
                e.stopPropagation();
                inputModel.clearAIAssistantChat();
            }
            if (checkKeyPressed(waveEvent, "ArrowUp")) {
                if (this.getLinePos(e.target).linePos > 1) {
                    // normal up arrow
                    return;
                }
                e.preventDefault();
                inputModel.codeSelectSelectNextOldestCodeBlock();
                resetCodeSelect = false;
            }
            if (checkKeyPressed(waveEvent, "ArrowDown")) {
                if (inputModel.getCodeSelectSelectedIndex() == inputModel.codeSelectBottom) {
                    return;
                }
                e.preventDefault();
                inputModel.codeSelectSelectNextNewestCodeBlock();
                resetCodeSelect = false;
            }

            if (resetCodeSelect) {
                inputModel.codeSelectDeselectAll();
            }

            // set height of textarea based on number of newlines
            this.textAreaNumLines.set(e.target.value.split(/\n/).length);
        })();
    }

    renderError(err: string): any {
        return <div className="chat-msg-error">{err}</div>;
    }

    renderChatMessage(chatItem: OpenAICmdInfoChatMessageType): any {
        let curKey = "chatmsg-" + this.chatListKeyCount;
        this.chatListKeyCount++;
        let senderClassName = chatItem.isassistantresponse ? "chat-msg-assistant" : "chat-msg-user";
        let msgClassName = "chat-msg " + senderClassName;
        let innerHTML: React.JSX.Element = (
            <span>
                <div className="chat-msg-header">
                    <i className="fa-sharp fa-solid fa-user"></i>
                    <div className="chat-username">You</div>
                </div>
                <p className="msg-text">{chatItem.userquery}</p>
            </span>
        );
        if (chatItem.isassistantresponse) {
            if (chatItem.assistantresponse.error != null && chatItem.assistantresponse.error != "") {
                innerHTML = this.renderError(chatItem.assistantresponse.error);
            } else {
                innerHTML = (
                    <span>
                        <div className="chat-msg-header">
                            <i className="fa-sharp fa-solid fa-headset"></i>
                            <div className="chat-username">ChatGPT</div>
                        </div>
                        <Markdown text={chatItem.assistantresponse.message} codeSelect />
                    </span>
                );
            }
        }

        return (
            <div className={msgClassName} key={curKey}>
                {innerHTML}
            </div>
        );
    }

    renderChatWindow(): any {
        let model = GlobalModel;
        let inputModel = model.inputModel;
        let chatMessageItems = inputModel.AICmdInfoChatItems.slice();
        let chitem: OpenAICmdInfoChatMessageType = null;
        return (
            <div className="chat-window" ref={this.chatWindowScrollRef}>
                <For each="chitem" index="idx" of={chatMessageItems}>
                    {this.renderChatMessage(chitem)}
                </For>
            </div>
        );
    }

    render() {
        let model = GlobalModel;
        let inputModel = model.inputModel;

        const termFontSize = 14;
        const textAreaMaxLines = 4;
        const textAreaLineHeight = termFontSize * 1.5;
        const textAreaPadding = 2 * 0.5 * termFontSize;
        let textAreaMaxHeight = textAreaLineHeight * textAreaMaxLines + textAreaPadding;
        let textAreaInnerHeight = this.textAreaNumLines.get() * textAreaLineHeight + textAreaPadding;

        return (
            <div className="cmd-aichat">
                {this.renderChatWindow()}
                <textarea
                    key="main"
                    ref={this.textAreaRef}
                    autoComplete="off"
                    autoCorrect="off"
                    id="chat-cmd-input"
                    onKeyDown={this.onKeyDown}
                    style={{ height: textAreaInnerHeight, maxHeight: textAreaMaxHeight, fontSize: termFontSize }}
                    className={cn("chat-textarea")}
                    placeholder="Send a Message to ChatGPT..."
                ></textarea>
            </div>
        );
    }
}

export { AIChat };
