// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as React from "react";
import * as mobxReact from "mobx-react";
import { If, For } from "tsx-control-statements/components";
import cn from "classnames";
import dayjs from "dayjs";
import localizedFormat from "dayjs/plugin/localizedFormat";
import { GlobalModel } from "@/models";
import * as appconst from "@/app/appconst";
import { AuxiliaryCmdView } from "./auxview";

import "./infomsg.less";

dayjs.extend(localizedFormat);

@mobxReact.observer
class InfoMsg extends React.Component<{}, {}> {
    getAfterSlash(s: string): string {
        if (s.startsWith("^/")) {
            return s.substring(1);
        }
        if (s.startsWith("^")) {
            return s.substring(1);
        }
        let slashIdx = s.lastIndexOf("/");
        if (slashIdx == s.length - 1) {
            slashIdx = s.lastIndexOf("/", slashIdx - 1);
        }
        if (slashIdx == -1) {
            return s;
        }
        return s.substring(slashIdx + 1);
    }

    hasSpace(s: string): boolean {
        return s.indexOf(" ") != -1;
    }

    handleCompClick(s: string): void {
        // TODO -> complete to this completion
    }

    render() {
        const inputModel = GlobalModel.inputModel;
        const infoMsg: InfoType = inputModel.infoMsg.get();
        const infoShow: boolean = inputModel.infoShow.get();
        let line: string = null;
        let istr: string = null;
        let idx: number = 0;
        let titleStr = null;
        if (infoMsg != null) {
            titleStr = infoMsg.infotitle;
        }
        if (!infoShow) {
            return null;
        }

        return (
            <AuxiliaryCmdView title={titleStr} className="cmd-input-info">
                <If condition={infoMsg?.infomsg}>
                    <div key="infomsg" className="info-msg">
                        <If condition={infoMsg.infomsghtml}>
                            <span dangerouslySetInnerHTML={{ __html: infoMsg.infomsg }} />
                        </If>
                        <If condition={!infoMsg.infomsghtml}>{infoMsg.infomsg}</If>
                    </div>
                </If>
                <If condition={infoMsg?.infolines}>
                    <div key="infolines" className="info-lines">
                        <For index="idx" each="line" of={infoMsg.infolines}>
                            <div key={idx}>{line == "" ? " " : line}</div>
                        </For>
                    </div>
                </If>
                <If condition={infoMsg?.infocomps?.length > 0}>
                    <div key="infocomps" className="info-comps">
                        <For each="istr" index="idx" of={infoMsg.infocomps}>
                            <div
                                onClick={() => this.handleCompClick(istr)}
                                key={idx}
                                className={cn(
                                    "info-comp",
                                    { "has-space": this.hasSpace(istr) },
                                    { "metacmd-comp": istr.startsWith("^") }
                                )}
                            >
                                {this.getAfterSlash(istr)}
                            </div>
                        </For>
                        <If condition={infoMsg.infocompsmore}>
                            <div key="more" className="info-comp no-select">
                                ...
                            </div>
                        </If>
                    </div>
                </If>
                <If condition={infoMsg?.infoerror}>
                    <div key="infoerror" className="info-error">
                        [error] {infoMsg.infoerror}
                    </div>
                    <If condition={infoMsg.infoerrorcode == appconst.ErrorCode_InvalidCwd}>
                        <div className="info-error">to reset, run: /reset:cwd</div>
                    </If>
                </If>
            </AuxiliaryCmdView>
        );
    }
}

export { InfoMsg };
