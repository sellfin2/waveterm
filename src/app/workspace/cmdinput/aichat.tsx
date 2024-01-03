// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as React from "react";
import * as mobxReact from "mobx-react";
import * as mobx from "mobx";
import { GlobalModel } from "../../../model/model";
import { isBlank } from "../../../util/util";
import { boundMethod } from "autobind-decorator";
import cn from "classnames";
import { Prompt } from "../../common/prompt/prompt";
import { TextAreaInput } from "./textareainput";
import { If, For } from "tsx-control-statements/components";
import type {OpenAICmdInfoChatMessageType} from "../../../types/types"
import { Markdown } from "../../common/common";


@mobxReact.observer
class AIChat extends React.Component<{}, {}> {
    chatListKeyCount: number = 0;
    textAreaNumLines: mobx.IObservableValue<number> = mobx.observable.box(1, {name: "textAreaNumLines"});
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
        if(this.chatWindowScrollRef != null && this.chatWindowScrollRef.current != null){
            this.chatWindowScrollRef.current.scrollTop = this.chatWindowScrollRef.current.scrollHeight;
        }
        if (this.textAreaRef.current != null) {
            this.textAreaRef.current.focus();
            inputModel.setCmdInfoChatTextAreaRef(this.textAreaRef)
        }
        this.requestChatUpdate();
    }       

    componentDidUpdate() {
        if(this.chatWindowScrollRef != null && this.chatWindowScrollRef.current != null){
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
            if(rtn.success) {
                console.log("submit chat command success");
            } else {
                console.log("submit chat command error: " + rtn.error);
            }
        })
        .catch((error) => {
        });
    }

    @mobx.action
    @boundMethod
    onKeyDown(e: any) {
        mobx.action(() => {
            let model = GlobalModel;
            let inputModel = model.inputModel;
            let ctrlMod = e.getModifierState("Control") || e.getModifierState("Meta") || e.getModifierState("Shift");
            
            if (e.code == "Enter") {
                e.preventDefault();
                if (!ctrlMod) { 
                    let messageStr = e.target.value;
                    console.log("target value?:", messageStr);
                    this.submitChatMessage(messageStr);
                    e.target.value = "";
                } else {
                    e.target.setRangeText("\n", e.target.selectionStart, e.target.selectionEnd, "end");
                    console.log("shift enter - target value: ", e.target.value);
                }
            }
            if (e.code == "Escape") {
                e.preventDefault();
                e.stopPropagation();
                inputModel.closeAIAssistantChat();
            }
            if(e.code = "KeyL" && e.getModifierState("Control")) {
                e.preventDefault();
                e.stopPropagation();
                inputModel.clearAIAssistantChat()
            }

            // set height of textarea based on number of newlines
            this.textAreaNumLines.set(e.target.value.split(/\n/).length); 
        })()
    }
    
    renderChatMessage(chatItem: OpenAICmdInfoChatMessageType): any {
        let curKey = "chatmsg-" + (this.chatListKeyCount);
        this.chatListKeyCount++;
        let senderClassName = chatItem.isassistantresponse ? "chat-msg-assistant" : "chat-msg-user";
        let msgClassName = "chat-msg " + senderClassName;
        let innerHTML: React.JSX.Element = (
            <p style={{whiteSpace:'pre-wrap'}}>{chatItem.userquery}</p>
        );
        if(chatItem.isassistantresponse) {
            innerHTML = (
                <Markdown text={chatItem.assistantresponse.message} /> 
            );
        }         

        return(
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
        const textAreaLineHeight = (termFontSize * 1.5);
        const textAreaPadding = 2 * 0.5 * termFontSize;
        let textAreaMaxHeight = textAreaLineHeight * textAreaMaxLines + textAreaPadding;
        let textAreaInnerHeight = this.textAreaNumLines.get() * textAreaLineHeight + textAreaPadding;
        
        return (
            <div className = "cmd-aichat">
                {this.renderChatWindow()}
                <textarea
                    key="main"
                    ref={this.textAreaRef}
                    autoComplete="off"
                    autoCorrect="off"
                    id="chat-cmd-input"
                    onKeyDown={this.onKeyDown}
                    style={{height: textAreaInnerHeight, maxHeight: textAreaMaxHeight, fontSize: termFontSize }}
                    className={cn("chat-textarea")}
                    placeholder="Send a Message to ChatGPT..."
                ></textarea>
            </div>

        );
    }
    
    
}

export {AIChat}
