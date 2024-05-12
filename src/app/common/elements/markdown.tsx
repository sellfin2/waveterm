// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as React from "react";
import * as mobxReact from "mobx-react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { clsx } from "clsx";
import { GlobalModel } from "@/models";
import { v4 as uuidv4 } from "uuid";

import "./markdown.less";
import { boundMethod } from "autobind-decorator";

function LinkRenderer(props: any): any {
    let newUrl = "https://extern?" + encodeURIComponent(props.href);
    return (
        <a href={newUrl} target="_blank" rel={"noopener"}>
            {props.children}
        </a>
    );
}

function HeaderRenderer(props: any, hnum: number): any {
    return <div className={clsx("title", "is-" + hnum)}>{props.children}</div>;
}

function CodeRenderer(props: any): any {
    return <code>{props.children}</code>;
}

@mobxReact.observer
class CodeBlockMarkdown extends React.Component<
    { nameSpace?: string; children: React.ReactNode; codeSelectSelectedIndex?: number; uuid: string },
    {}
> {
    blockRef: React.RefObject<HTMLPreElement> = React.createRef();
    blockIndex: number;
    id: string;
    nameSpace: string;

    constructor(props) {
        super(props);
        this.nameSpace = this.props.nameSpace;
        this.id = uuidv4();
    }

    componentDidMount(): void {
        GlobalModel.inputModel.addCodeBlock(this.props.nameSpace, this.id, this.blockRef);
        GlobalModel.inputModel.addCodeBlockToCodeSelect(this.blockRef, this.props.uuid);
    }

    @boundMethod
    handleClick(e: React.MouseEvent<HTMLPreElement>) {
        // console.log("this.blockIndex", this.blockIndex);
        GlobalModel.inputModel.setCodeSelectSelectedCodeBlock(this.blockIndex);
    }

    render() {
        // console.log("this.blockIndex", this.blockIndex);
        const selectedBlock = GlobalModel.inputModel.getSelectedBlockItem(this.nameSpace);
        const selected = this.id == selectedBlock?.id;
        return (
            <pre ref={this.blockRef} data-blockid={this.id} className={clsx({ selected })} onClick={this.handleClick}>
                {this.props.children}
            </pre>
        );
    }
}

@mobxReact.observer
class Markdown extends React.Component<
    { nameSpace?: string; text: string; style?: any; extraClassName?: string; codeSelect?: boolean },
    {}
> {
    curUuid: string;

    constructor(props) {
        super(props);
        this.curUuid = uuidv4();
    }

    @boundMethod
    codeBlockRenderer(props: any, codeSelect: boolean, codeSelectIndex: number, curUuid: string): any {
        if (codeSelect) {
            return (
                <CodeBlockMarkdown
                    nameSpace={this.props.nameSpace}
                    codeSelectSelectedIndex={codeSelectIndex}
                    uuid={curUuid}
                >
                    {props.children}
                </CodeBlockMarkdown>
            );
        } else {
            const clickHandler = (e: React.MouseEvent<HTMLElement>) => {
                let blockText = (e.target as HTMLElement).innerText;
                if (blockText) {
                    blockText = blockText.replace(/\n$/, ""); // remove trailing newline
                    navigator.clipboard.writeText(blockText);
                }
            };
            return <pre onClick={(event) => clickHandler(event)}>{props.children}</pre>;
        }
    }

    render() {
        let { text } = this.props;
        let codeSelect = this.props.codeSelect;
        let curCodeSelectIndex = GlobalModel.inputModel.getCodeSelectSelectedIndex();
        let markdownComponents = {
            a: LinkRenderer,
            h1: (props) => HeaderRenderer(props, 1),
            h2: (props) => HeaderRenderer(props, 2),
            h3: (props) => HeaderRenderer(props, 3),
            h4: (props) => HeaderRenderer(props, 4),
            h5: (props) => HeaderRenderer(props, 5),
            h6: (props) => HeaderRenderer(props, 6),
            code: (props) => CodeRenderer(props),
            pre: (props) => this.codeBlockRenderer(props, codeSelect, curCodeSelectIndex, this.curUuid),
        };
        return (
            <div className={clsx("markdown content", this.props.extraClassName)} style={this.props.style}>
                <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
                    {text}
                </ReactMarkdown>
            </div>
        );
    }
}

export { Markdown };
