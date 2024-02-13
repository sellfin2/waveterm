// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as React from "react";
import * as mobx from "mobx";
import * as mobxReact from "mobx-react";

import "./image.less";

@mobxReact.observer
class SimpleImageRenderer extends React.Component<
    { data: ExtBlob; context: RendererContext; opts: RendererOpts; savedHeight: number },
    {}
> {
    objUrl: string = null;
    imageRef: React.RefObject<any> = React.createRef();
    imageLoaded: OV<boolean> = mobx.observable.box(false, { name: "imageLoaded" });

    componentDidMount() {
        let img = this.imageRef.current;
        if (img == null) {
            return;
        }
        if (img.complete) {
            this.setImageLoaded();
            return;
        }
        img.onload = () => {
            this.setImageLoaded();
        };
    }

    setImageLoaded() {
        mobx.action(() => {
            this.imageLoaded.set(true);
        })();
    }

    componentWillUnmount() {
        if (this.objUrl != null) {
            URL.revokeObjectURL(this.objUrl);
        }
    }

    render() {
        let dataBlob = this.props.data;
        if (dataBlob == null || dataBlob.notFound) {
            return (
                <div className="image-renderer" style={{ fontSize: this.props.opts.termFontSize }}>
                    <div className="load-error-text">
                        ERROR: file {dataBlob && dataBlob.name ? JSON.stringify(dataBlob.name) : ""} not found
                    </div>
                </div>
            );
        }
        if (this.objUrl == null) {
            this.objUrl = URL.createObjectURL(dataBlob);
        }
        let opts = this.props.opts;
        let forceHeight: number = null;
        if (!this.imageLoaded.get() && this.props.savedHeight >= 0) {
            forceHeight = this.props.savedHeight;
        }
        return (
            <div className="image-renderer" style={{ height: forceHeight }}>
                <img
                    ref={this.imageRef}
                    style={{ maxHeight: opts.idealSize.height, maxWidth: opts.idealSize.width }}
                    src={this.objUrl}
                />
            </div>
        );
    }
}

export { SimpleImageRenderer };
