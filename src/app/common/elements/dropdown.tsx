// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as React from "react";
import * as mobxReact from "mobx-react";
import { boundMethod } from "autobind-decorator";
import cn from "classnames";
import { If } from "tsx-control-statements/components";
import ReactDOM from "react-dom";
import { v4 as uuidv4 } from "uuid";
import { GlobalModel } from "@/models";

import "./dropdown.less";

interface DropdownDecorationProps {
    startDecoration?: React.ReactNode;
    endDecoration?: React.ReactNode;
}

interface DropdownProps {
    label?: string;
    options: DropdownItem[];
    value?: string;
    className?: string;
    onChange: (value: string) => void;
    placeholder?: string;
    decoration?: DropdownDecorationProps;
    defaultValue?: string;
    required?: boolean;
}

interface DropdownState {
    isOpen: boolean;
    internalValue: string;
    highlightedIndex: number;
    isTouched: boolean;
}

@mobxReact.observer
class Dropdown extends React.Component<DropdownProps, DropdownState> {
    wrapperRef: React.RefObject<HTMLDivElement>;
    menuRef: React.RefObject<HTMLDivElement>;
    timeoutId: any;
    curUuid: string;

    constructor(props: DropdownProps) {
        super(props);
        this.state = {
            isOpen: false,
            internalValue: props.defaultValue || "",
            highlightedIndex: -1,
            isTouched: false,
        };
        this.wrapperRef = React.createRef();
        this.menuRef = React.createRef();
        this.curUuid = uuidv4();
    }

    componentDidMount() {
        document.addEventListener("mousedown", this.handleClickOutside);
    }

    componentWillUnmount() {
        document.removeEventListener("mousedown", this.handleClickOutside);
    }

    componentDidUpdate(prevProps: Readonly<DropdownProps>, prevState: Readonly<DropdownState>, snapshot?: any): void {
        // If the dropdown was open but now is closed, start the timeout
        if (prevState.isOpen && !this.state.isOpen) {
            this.timeoutId = setTimeout(() => {
                if (this.menuRef.current) {
                    this.menuRef.current.style.display = "none";
                }
            }, 300); // Time is equal to the animation duration
        }
        // If the dropdown is now open, cancel any existing timeout and show the menu
        else if (!prevState.isOpen && this.state.isOpen) {
            if (this.timeoutId !== null) {
                clearTimeout(this.timeoutId); // Cancel any existing timeout
                this.timeoutId = null;
            }
            if (this.menuRef.current) {
                this.menuRef.current.style.display = "inline-flex";
            }
        }
    }

    @boundMethod
    handleClickOutside(event: MouseEvent) {
        // Check if the click is outside both the wrapper and the menu
        if (
            this.wrapperRef.current &&
            !this.wrapperRef.current.contains(event.target as Node) &&
            this.menuRef.current &&
            !this.menuRef.current.contains(event.target as Node)
        ) {
            this.setState({ isOpen: false });
        }
    }

    @boundMethod
    handleClick() {
        if (!this.state.isOpen || !this.state.isTouched) {
            this.registerKeybindings();
        }
        this.toggleDropdown();
    }

    @boundMethod
    handleFocus() {
        this.setState({ isTouched: true });
        this.registerKeybindings();
    }

    @boundMethod
    registerKeybindings() {
        let keybindManager = GlobalModel.keybindManager;
        let domain = "dropdown-" + this.curUuid;
        keybindManager.registerKeybinding("control", domain, "generic:confirm", (waveEvent) => {
            this.handleConfirm();
            return true;
        });
        keybindManager.registerKeybinding("control", domain, "generic:space", (waveEvent) => {
            this.handleConfirm();
            return true;
        });
        keybindManager.registerKeybinding("control", domain, "generic:cancel", (waveEvent) => {
            this.setState({ isOpen: false });
            this.unregisterKeybindings();
            return true;
        });
        keybindManager.registerKeybinding("control", domain, "generic:selectAbove", (waveEvent) => {
            const { isOpen } = this.state;
            const { options } = this.props;
            if (isOpen) {
                this.setState((prevState) => ({
                    highlightedIndex:
                        prevState.highlightedIndex > 0 ? prevState.highlightedIndex - 1 : options.length - 1,
                }));
            }
            return true;
        });
        keybindManager.registerKeybinding("control", domain, "generic:selectBelow", (waveEvent) => {
            const { isOpen } = this.state;
            const { options } = this.props;
            if (isOpen) {
                this.setState((prevState) => ({
                    highlightedIndex:
                        prevState.highlightedIndex < options.length - 1 ? prevState.highlightedIndex + 1 : 0,
                }));
            }
            return true;
        });
        keybindManager.registerKeybinding("control", domain, "generic:tab", (waveEvent) => {
            this.setState({ isOpen: false });
            return true;
        });
    }

    handleConfirm() {
        const { options } = this.props;
        const { isOpen, highlightedIndex } = this.state;
        if (isOpen) {
            const option = options[highlightedIndex];
            if (option) {
                this.handleSelect(option, undefined);
            }
        } else {
            this.toggleDropdown();
        }
    }

    @boundMethod
    handleBlur() {
        this.unregisterKeybindings();
    }

    @boundMethod
    unregisterKeybindings() {
        let domain = "dropdown-" + this.curUuid;
        GlobalModel.keybindManager.unregisterDomain(domain);
    }

    @boundMethod
    handleKeyDown(event: React.KeyboardEvent) {}

    @boundMethod
    handleSelect({ value, noop }: DropdownItem, event?: React.MouseEvent | React.KeyboardEvent) {
        const { onChange } = this.props;
        if (event) {
            event.stopPropagation(); // This stops the event from bubbling up to the wrapper
        }

        onChange(value);
        this.setState({ isOpen: false, isTouched: true });
        this.unregisterKeybindings();

        if (!("value" in this.props) && !noop) {
            this.setState({ internalValue: value });
        }
    }

    @boundMethod
    toggleDropdown() {
        this.setState((prevState) => ({ isOpen: !prevState.isOpen, isTouched: true }));
    }

    @boundMethod
    calculatePosition(): React.CSSProperties {
        if (this.wrapperRef.current) {
            const rect = this.wrapperRef.current.getBoundingClientRect();
            return {
                position: "absolute",
                top: `${rect.bottom + window.scrollY}px`,
                left: `${rect.left + window.scrollX}px`,
                width: `${rect.width}px`,
            };
        }
        return {};
    }

    render() {
        const { label, options, value, placeholder, decoration, className, required } = this.props;
        const { isOpen, internalValue, highlightedIndex, isTouched } = this.state;

        const currentValue = value ?? internalValue;
        const selectedOptionLabel =
            options.find((option) => option.value === currentValue)?.label || placeholder || internalValue;

        // Determine if the dropdown should be marked as having an error
        const isError =
            required &&
            (value === undefined || value === "") &&
            (internalValue === undefined || internalValue === "") &&
            isTouched;

        // Determine if the label should float
        const shouldLabelFloat = !!value || !!internalValue || !!placeholder || isOpen;

        const dropdownMenu = isOpen
            ? ReactDOM.createPortal(
                  <div className={cn("wave-dropdown-menu")} ref={this.menuRef} style={this.calculatePosition()}>
                      {options.map((option, index) => (
                          <div
                              key={option.value}
                              className={cn("wave-dropdown-item unselectable", {
                                  "wave-dropdown-item-highlighted": index === highlightedIndex,
                              })}
                              onClick={(e) => this.handleSelect(option, e)}
                              onMouseEnter={() => this.setState({ highlightedIndex: index })}
                              onMouseLeave={() => this.setState({ highlightedIndex: -1 })}
                          >
                              {option.icon && <span className="wave-dropdown-item-icon">{option.icon}</span>}
                              {option.label}
                          </div>
                      ))}
                  </div>,
                  document.getElementById("app")!
              )
            : null;
        let selectedOptionLabelStyle = {};
        const wrapperClientWidth = this.wrapperRef.current?.clientWidth;
        if ((wrapperClientWidth ?? 0) > 0) {
            selectedOptionLabelStyle["width"] = Math.max(wrapperClientWidth - 55, 0);
        }
        return (
            <div
                className={cn("wave-dropdown", className, {
                    "wave-dropdown-error": isError,
                    "no-label": !label,
                })}
                ref={this.wrapperRef}
                tabIndex={0}
                onKeyDown={this.handleKeyDown}
                onClick={this.handleClick}
                onFocus={this.handleFocus.bind(this)}
                onBlur={this.handleBlur.bind(this)}
            >
                {decoration?.startDecoration && <>{decoration.startDecoration}</>}
                <If condition={label}>
                    <div
                        className={cn("wave-dropdown-label unselectable", {
                            float: shouldLabelFloat,
                            "offset-left": decoration?.startDecoration,
                        })}
                    >
                        {label}
                    </div>
                </If>
                <div
                    className={cn("wave-dropdown-display unselectable truncate", {
                        "offset-left": decoration?.startDecoration,
                    })}
                    style={selectedOptionLabelStyle}
                >
                    {selectedOptionLabel}
                </div>
                <div className={cn("wave-dropdown-arrow", { "wave-dropdown-arrow-rotate": isOpen })}>
                    <i className="fa-sharp fa-solid fa-chevron-down"></i>
                </div>
                {dropdownMenu}
                {decoration?.endDecoration && <>{decoration.endDecoration}</>}
            </div>
        );
    }
}

export { Dropdown };
