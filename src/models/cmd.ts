// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as mobx from "mobx";
import { stringToBase64 } from "../util/util";
import { TermWrap } from "../plugins/terminal/term";
import {
    RemotePtrType,
    CmdDataType,
    TermOptsType,
    FeInputPacketType,
    RendererModel,
    WebCmd,
    WebRemote,
} from "../types/types";
import { cmdStatusIsRunning } from "../app/line/lineutil";
import { Model } from "./model";
import { OV } from "../types/types";

const InputChunkSize = 500;
class Cmd {
    model: Model;
    screenId: string;
    remote: RemotePtrType;
    lineId: string;
    data: OV<CmdDataType>;

    constructor(cmd: CmdDataType) {
        this.model = Model.getInstance();
        this.screenId = cmd.screenid;
        this.lineId = cmd.lineid;
        this.remote = cmd.remote;
        this.data = mobx.observable.box(cmd, { deep: false, name: "cmd-data" });
    }

    setCmd(cmd: CmdDataType) {
        mobx.action(() => {
            let origData = this.data.get();
            this.data.set(cmd);
            if (origData != null && cmd != null && origData.status != cmd.status) {
                this.model.cmdStatusUpdate(this.screenId, this.lineId, origData.status, cmd.status);
            }
        })();
    }

    getRestartTs(): number {
        return this.data.get().restartts;
    }

    getAsWebCmd(lineid: string): WebCmd {
        let cmd = this.data.get();
        let remote = this.model.getRemote(this.remote.remoteid);
        let webRemote: WebRemote = null;
        if (remote != null) {
            webRemote = {
                remoteid: cmd.remote.remoteid,
                alias: remote.remotealias,
                canonicalname: remote.remotecanonicalname,
                name: this.remote.name,
                homedir: remote.remotevars["home"],
                isroot: !!remote.remotevars["isroot"],
            };
        }
        let webCmd: WebCmd = {
            screenid: cmd.screenid,
            lineid: lineid,
            remote: webRemote,
            status: cmd.status,
            cmdstr: cmd.cmdstr,
            rawcmdstr: cmd.rawcmdstr,
            festate: cmd.festate,
            termopts: cmd.termopts,
            cmdpid: cmd.cmdpid,
            remotepid: cmd.remotepid,
            donets: cmd.donets,
            exitcode: cmd.exitcode,
            durationms: cmd.durationms,
            rtnstate: cmd.rtnstate,
            vts: 0,
            rtnstatestr: null,
        };
        return webCmd;
    }

    getExitCode(): number {
        return this.data.get().exitcode;
    }

    getRtnState(): boolean {
        return this.data.get().rtnstate;
    }

    getStatus(): string {
        return this.data.get().status;
    }

    getTermOpts(): TermOptsType {
        return this.data.get().termopts;
    }

    getCmdStr(): string {
        return this.data.get().cmdstr;
    }

    getRemoteFeState(): Record<string, string> {
        return this.data.get().festate;
    }

    isRunning(): boolean {
        let data = this.data.get();
        return cmdStatusIsRunning(data.status);
    }

    handleData(data: string, termWrap: TermWrap): void {
        if (!this.isRunning()) {
            return;
        }
        for (let pos = 0; pos < data.length; pos += InputChunkSize) {
            let dataChunk = data.slice(pos, pos + InputChunkSize);
            this.handleInputChunk(dataChunk);
        }
    }

    handleDataFromRenderer(data: string, renderer: RendererModel): void {
        if (!this.isRunning()) {
            return;
        }
        for (let pos = 0; pos < data.length; pos += InputChunkSize) {
            let dataChunk = data.slice(pos, pos + InputChunkSize);
            this.handleInputChunk(dataChunk);
        }
    }

    handleInputChunk(data: string): void {
        let inputPacket: FeInputPacketType = {
            type: "feinput",
            ck: this.screenId + "/" + this.lineId,
            remote: this.remote,
            inputdata64: stringToBase64(data),
        };
        this.model.sendInputPacket(inputPacket);
    }
}

export { Cmd };
