// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as React from "react";
import * as mobxReact from "mobx-react";
import * as mobx from "mobx";
import { debounce } from "throttle-debounce";
import * as util from "@/util/util";
import { GlobalModel } from "@/models";
import cn from "classnames";

class SimpleBlobRendererModel {
    context: RendererContext;
    opts: RendererOpts;
    isDone: OV<boolean>;
    api: RendererModelContainerApi;
    savedHeight: number;
    loading: OV<boolean>;
    loadError: OV<string> = mobx.observable.box(null, {
        name: "renderer-loadError",
    });
    lineState: LineStateType;
    ptyData: PtyDataType;
    ptyDataSource: (termContext: TermContextUnion) => Promise<PtyDataType>;
    dataBlob: ExtBlob;
    readOnly: boolean;
    notFound: boolean;
    isClosed: boolean;

    initialize(params: RendererModelInitializeParams): void {
        this.isClosed = !!params.lineState["prompt:closed"];
        this.loading = mobx.observable.box(!this.isClosed, { name: "renderer-loading" });
        this.isDone = mobx.observable.box(params.isDone, {
            name: "renderer-isDone",
        });
        this.context = params.context;
        this.opts = params.opts;
        this.api = params.api;
        this.lineState = params.lineState;
        this.savedHeight = params.savedHeight;
        this.ptyDataSource = params.ptyDataSource;
        if (this.isClosed) {
            this.dataBlob = new Blob() as ExtBlob;
            this.dataBlob.notFound = false; // TODO
        } else {
            if (this.isDone.get()) {
                setTimeout(() => this.reload(0), 10);
            }
        }
    }

    dispose(): void {
        return;
    }

    giveFocus(): void {
        return;
    }

    updateOpts(update: RendererOptsUpdate): void {
        Object.assign(this.opts, update);
    }

    updateHeight(newHeight: number): void {
        if (this.savedHeight != newHeight) {
            this.savedHeight = newHeight;
            this.api.saveHeight(newHeight);
        }
    }

    setIsDone(): void {
        if (this.isDone.get()) {
            return;
        }
        mobx.action(() => {
            this.isDone.set(true);
        })();
        this.reload(0);
    }

    reload(delayMs: number): void {
        mobx.action(() => {
            this.loading.set(true);
        })();
        if (delayMs == 0) {
            this.reload_noDelay();
        } else {
            setTimeout(() => {
                this.reload_noDelay();
            }, delayMs);
        }
    }

    reload_noDelay(): void {
        let source = this.lineState["prompt:source"] || "pty";
        if (source == "pty") {
            this.reloadPtyData();
        } else if (source == "file") {
            this.reloadFileData();
        } else {
            mobx.action(() => {
                this.loadError.set("error: invalid load source: " + source);
            })();
        }
    }

    reloadFileData(): void {
        // todo add file methods to API, so we don't have a GlobalModel dependency here!
        let path = this.lineState["prompt:file"];
        if (util.isBlank(path)) {
            mobx.action(() => {
                this.loadError.set("renderer has file source, but no prompt:file specified");
            })();
            return;
        }
        let rtnp = GlobalModel.readRemoteFile(this.context.screenId, this.context.lineId, path);
        rtnp.then((file) => {
            this.notFound = (file as any).notFound;
            this.readOnly = (file as any).readOnly;
            this.dataBlob = file;
            mobx.action(() => {
                this.loading.set(false);
                this.loadError.set(null);
            })();
        }).catch((e) => {
            mobx.action(() => {
                this.loadError.set("error loading file data: " + e);
            })();
        });
    }

    reloadPtyData(): void {
        this.readOnly = true;
        let rtnp = this.ptyDataSource(this.context);
        if (rtnp == null) {
            console.log("no promise returned from ptyDataSource (simplerenderer)", this.context);
            return;
        }
        rtnp.then((ptydata) => {
            this.ptyData = ptydata;
            let blob: ExtBlob = new Blob([this.ptyData.data]) as ExtBlob;
            blob.notFound = false;
            this.dataBlob = blob;
            mobx.action(() => {
                this.loading.set(false);
                this.loadError.set(null);
            })();
        }).catch((e) => {
            mobx.action(() => {
                this.loadError.set("error loading data: " + e);
            })();
        });
    }

    receiveData(pos: number, data: Uint8Array, reason?: string): void {
        // this.dataBuf.receiveData(pos, data, reason);
    }
}

@mobxReact.observer
class SimpleBlobRenderer extends React.Component<
    {
        rendererContainer: RendererContainerType;
        lineId: string;
        plugin: RendererPluginType;
        onHeightChange: () => void;
        initParams: RendererModelInitializeParams;
        scrollToBringIntoViewport: () => void;
        isSelected: boolean;
        shouldFocus: boolean;
    },
    {}
> {
    model: SimpleBlobRendererModel;
    wrapperDivRef: React.RefObject<any> = React.createRef();
    rszObs: ResizeObserver;
    updateHeight_debounced: (newHeight: number) => void;

    constructor(props: any) {
        super(props);
        let { rendererContainer, lineId, plugin, initParams } = this.props;
        this.model = new SimpleBlobRendererModel();
        this.model.initialize(initParams);
        rendererContainer.registerRenderer(lineId, this.model);
        this.updateHeight_debounced = debounce(1000, this.updateHeight.bind(this));
    }

    updateHeight(newHeight: number): void {
        this.model.updateHeight(newHeight);
    }

    handleResize(entries: ResizeObserverEntry[]): void {
        if (this.model.loading.get()) {
            return;
        }
        if (this.props.onHeightChange) {
            this.props.onHeightChange();
        }
        if (!this.model.loading.get() && this.wrapperDivRef.current != null) {
            let height = this.wrapperDivRef.current.offsetHeight;
            this.updateHeight_debounced(height);
        }
    }

    checkRszObs() {
        if (this.rszObs != null) {
            return;
        }
        if (this.wrapperDivRef.current == null) {
            return;
        }
        this.rszObs = new ResizeObserver(this.handleResize.bind(this));
        this.rszObs.observe(this.wrapperDivRef.current);
    }

    componentDidMount() {
        this.checkRszObs();
    }

    componentWillUnmount() {
        let { rendererContainer, lineId } = this.props;
        rendererContainer.unloadRenderer(lineId);
        if (this.rszObs != null) {
            this.rszObs.disconnect();
            this.rszObs = null;
        }
    }

    componentDidUpdate() {
        this.checkRszObs();
    }

    render() {
        let { plugin } = this.props;
        let model = this.model;
        if (model.loadError.get() != null) {
            let errorText = model.loadError.get();
            let height = this.model.savedHeight;
            return (
                <div ref={this.wrapperDivRef} style={{ minHeight: height, fontSize: model.opts.termFontSize }}>
                    <div className="load-error-text">ERROR: {errorText}</div>
                </div>
            );
        }
        if (model.loading.get()) {
            let height = this.model.savedHeight;
            return (
                <div
                    ref={this.wrapperDivRef}
                    className={cn("renderer-loading", { "zero-height": height == 0 })}
                    style={{ minHeight: height, fontSize: model.opts.termFontSize }}
                >
                    loading content <i className="fa fa-ellipsis fa-fade" />
                </div>
            );
        }
        let Comp = plugin.simpleComponent;
        if (Comp == null) {
            <div ref={this.wrapperDivRef}>(no component found in plugin)</div>;
        }
        let { festate, cmdstr, exitcode } = this.props.initParams.rawCmd;
        return (
            <div ref={this.wrapperDivRef} className={cn("sr-wrapper", { "zero-height": model.savedHeight == 0 })}>
                <Comp
                    cwd={festate.cwd}
                    cmdstr={cmdstr}
                    exitcode={exitcode}
                    data={model.dataBlob}
                    readOnly={model.readOnly}
                    notFound={model.notFound}
                    lineState={model.lineState}
                    context={model.context}
                    opts={model.opts}
                    savedHeight={model.savedHeight}
                    scrollToBringIntoViewport={this.props.scrollToBringIntoViewport}
                    isSelected={this.props.isSelected}
                    shouldFocus={this.props.shouldFocus}
                    rendererApi={model.api}
                />
            </div>
        );
    }
}

export { SimpleBlobRendererModel, SimpleBlobRenderer };
