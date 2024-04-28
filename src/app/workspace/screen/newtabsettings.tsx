import * as React from "react";
import * as mobxReact from "mobx-react";
import * as mobx from "mobx";
import { boundMethod } from "autobind-decorator";
import { If, For } from "tsx-control-statements/components";
import cn from "classnames";
import { GlobalCommandRunner, GlobalModel, Screen } from "@/models";
import { TextField, Dropdown } from "@/elements";
import { getRemoteStrWithAlias } from "@/common/prompt/prompt";
import * as util from "@/util/util";
import { TabIcon } from "@/elements/tabicon";
import { ReactComponent as GlobeIcon } from "@/assets/icons/globe.svg";
import { ReactComponent as StatusCircleIcon } from "@/assets/icons/statuscircle.svg";
import * as appconst from "@/app/appconst";

import "./screenview.less";
import "./tabs.less";

@mobxReact.observer
class TabNameTextField extends React.Component<{ screen: Screen; errorMessage?: OV<string> }, {}> {
    @boundMethod
    updateName(val: string): void {
        let { screen } = this.props;
        if (util.isStrEq(val, screen.name.get())) {
            return;
        }
        let prtn = GlobalCommandRunner.screenSetSettings(screen.screenId, { name: val }, false);
        util.commandRtnHandler(prtn, this.props.errorMessage);
    }

    render() {
        let { screen } = this.props;
        return (
            <TextField label="Name" required={true} defaultValue={screen.name.get() ?? ""} onChange={this.updateName} />
        );
    }
}

@mobxReact.observer
class TabColorSelector extends React.Component<{ screen: Screen; errorMessage?: OV<string> }, {}> {
    @boundMethod
    selectTabColor(color: string): void {
        let { screen } = this.props;
        if (screen.getTabColor() == color) {
            return;
        }
        let prtn = GlobalCommandRunner.screenSetSettings(screen.screenId, { tabcolor: color }, false);
        util.commandRtnHandler(prtn, this.props.errorMessage);
    }

    render() {
        let { screen } = this.props;
        let curColor = screen.getTabColor();
        if (util.isBlank(curColor) || curColor == "default") {
            curColor = "green";
        }
        let color: string | null = null;
        return (
            <div className="tab-colors">
                <div className="tab-color-cur">
                    <TabIcon icon={screen.getTabIcon()} color={screen.getTabColor()} />
                    <div className="tab-color-name">{screen.getTabColor()}</div>
                </div>
                <div className="tab-color-sep">|</div>
                <For each="color" of={appconst.TabColors}>
                    <div key={color} className="tab-color-select" onClick={() => this.selectTabColor(color)}>
                        <TabIcon icon="square" color={color} />
                    </div>
                </For>
            </div>
        );
    }
}

@mobxReact.observer
class TabIconSelector extends React.Component<{ screen: Screen; errorMessage?: OV<string> }, {}> {
    @boundMethod
    selectTabIcon(icon: string): void {
        let { screen } = this.props;
        if (screen.getTabIcon() == icon) {
            return;
        }
        let prtn = GlobalCommandRunner.screenSetSettings(screen.screenId, { tabicon: icon }, false);
        util.commandRtnHandler(prtn, this.props.errorMessage);
    }

    render() {
        let { screen } = this.props;
        let curIcon = screen.getTabIcon();
        if (util.isBlank(curIcon) || curIcon == "default") {
            curIcon = "square";
        }
        let icon: string | null = null;
        let curColor = screen.getTabColor();
        return (
            <div className="tab-icons">
                <div className="tab-icon-cur">
                    <TabIcon icon={screen.getTabIcon()} color={curColor} />
                    <div className="tab-icon-name">{screen.getTabIcon()}</div>
                </div>
                <div className="tab-icon-sep">|</div>
                <For each="icon" index="index" of={appconst.TabIcons}>
                    <div key={icon} className="tab-icon-select" onClick={() => this.selectTabIcon(icon)}>
                        <TabIcon icon={icon} color={curColor} />
                    </div>
                </For>
            </div>
        );
    }
}

@mobxReact.observer
class TabRemoteSelector extends React.Component<{ screen: Screen; errorMessage?: OV<string> }, {}> {
    selectedRemoteCN: OV<string> = mobx.observable.box(null, { name: "TabRemoteSelector-selectedRemoteCN" });

    @boundMethod
    selectRemote(cname: string): void {
        if (cname == null) {
            GlobalModel.remotesModel.openAddModal({ remoteedit: true });
            return;
        }
        mobx.action(() => {
            this.selectedRemoteCN.set(cname);
        })();
        let prtn = GlobalCommandRunner.screenSetRemote(cname, true, true);
        util.commandRtnHandler(prtn, this.props.errorMessage);
        prtn.then((crtn) => {
            GlobalModel.inputModel.giveFocus();
        });
    }

    @boundMethod
    getOptions(): DropdownItem[] {
        const remotes = GlobalModel.remotes;
        const options = remotes
            .filter((r) => !r.archived)
            .map((remote) => ({
                ...remote,
                label: getRemoteStrWithAlias(remote),
                value: remote.remotecanonicalname,
            }))
            .sort((a, b) => {
                let connValA = util.getRemoteConnVal(a);
                let connValB = util.getRemoteConnVal(b);
                if (connValA !== connValB) {
                    return connValA - connValB;
                }
                return a.remoteidx - b.remoteidx;
            });

        options.push({
            label: "New Connection",
            value: null,
            icon: <i className="fa-sharp fa-solid fa-plus"></i>,
            noop: true,
        });

        return options;
    }

    render() {
        const { screen } = this.props;
        let selectedRemote = this.selectedRemoteCN.get();
        if (selectedRemote == null) {
            const curRI = screen.getCurRemoteInstance();
            if (curRI != null) {
                const curRemote = GlobalModel.getRemote(curRI.remoteid);
                selectedRemote = curRemote.remotecanonicalname;
            } else {
                const localRemote = GlobalModel.getLocalRemote();
                selectedRemote = localRemote.remotecanonicalname;
            }
        }
        let curRemote = GlobalModel.getRemoteByName(selectedRemote);
        return (
            <Dropdown
                label="Connection"
                className="conn-dropdown"
                options={this.getOptions()}
                defaultValue={curRemote.remotecanonicalname}
                onChange={this.selectRemote}
                decoration={{
                    startDecoration: (
                        <div className="lefticon">
                            <GlobeIcon className="globe-icon" />
                            <StatusCircleIcon className={cn("status-icon", "status-" + curRemote.status)} />
                        </div>
                    ),
                }}
            />
        );
    }
}

export { TabColorSelector, TabIconSelector, TabNameTextField, TabRemoteSelector };
