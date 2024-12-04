// Copyright 2024, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { Button } from "@/element/button";
import { ContextMenuModel } from "@/store/contextmenu";
import * as services from "@/store/services";
import * as WOS from "@/store/wos";
import { clsx } from "clsx";
import { Atom, useAtomValue } from "jotai";
import { forwardRef, memo, useCallback, useEffect, useImperativeHandle, useRef, useState } from "react";

import { atoms, globalStore, refocusNode } from "@/app/store/global";
import { RpcApi } from "@/app/store/wshclientapi";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import "./tab.scss";

interface TabProps {
    id: string;
    active: boolean;
    isFirst: boolean;
    isBeforeActive: boolean;
    isDragging: boolean;
    tabWidth: number;
    isNew: boolean;
    tabIndicesMovedAtom: Atom<number[]>;
    tabIds: string[];
    onClick: () => void;
    onClose: (event: React.MouseEvent<HTMLButtonElement, MouseEvent> | null) => void;
    onMouseDown: (event: React.MouseEvent<HTMLDivElement, MouseEvent>) => void;
    onLoaded: () => void;
    onMouseEnter: (event: React.MouseEvent<HTMLDivElement, MouseEvent>) => void;
    onMouseLeave: (event: React.MouseEvent<HTMLDivElement, MouseEvent>) => void;
}

const Tab = memo(
    forwardRef<HTMLDivElement, TabProps>(
        (
            {
                id,
                active,
                isFirst,
                isBeforeActive,
                isDragging,
                tabWidth,
                isNew,
                tabIndicesMovedAtom,
                tabIds,
                onLoaded,
                onClick,
                onClose,
                onMouseDown,
                onMouseEnter,
                onMouseLeave,
            },
            ref
        ) => {
            const [tabData, tabLoading] = WOS.useWaveObjectValue<Tab>(WOS.makeORef("tab", id));
            const [originalName, setOriginalName] = useState("");
            const [isEditable, setIsEditable] = useState(false);

            const editableRef = useRef<HTMLDivElement>(null);
            const editableTimeoutRef = useRef<NodeJS.Timeout>();
            const loadedRef = useRef(false);
            const tabRef = useRef<HTMLDivElement>(null);

            const tabIndicesMoved = useAtomValue(tabIndicesMovedAtom);

            console.log("tabIndicesMoved", tabIndicesMoved);
            console.log("tabIds", tabIds);

            useImperativeHandle(ref, () => tabRef.current as HTMLDivElement);

            useEffect(() => {
                if (tabData?.name) {
                    setOriginalName(tabData.name);
                }
            }, [tabData]);

            useEffect(() => {
                return () => {
                    if (editableTimeoutRef.current) {
                        clearTimeout(editableTimeoutRef.current);
                    }
                };
            }, []);

            const handleRenameTab = (event) => {
                event?.stopPropagation();
                setIsEditable(true);
                editableTimeoutRef.current = setTimeout(() => {
                    if (editableRef.current) {
                        editableRef.current.focus();
                        document.execCommand("selectAll", false);
                    }
                }, 0);
            };

            const handleBlur = () => {
                let newText = editableRef.current.innerText.trim();
                newText = newText || originalName;
                editableRef.current.innerText = newText;
                setIsEditable(false);
                services.ObjectService.UpdateTabName(id, newText);
                setTimeout(() => refocusNode(null), 10);
            };

            const handleKeyDown = (event) => {
                if ((event.metaKey || event.ctrlKey) && event.key === "a") {
                    event.preventDefault();
                    if (editableRef.current) {
                        const range = document.createRange();
                        const selection = window.getSelection();
                        range.selectNodeContents(editableRef.current);
                        selection.removeAllRanges();
                        selection.addRange(range);
                    }
                    return;
                }
                // this counts glyphs, not characters
                const curLen = Array.from(editableRef.current.innerText).length;
                if (event.key === "Enter") {
                    event.preventDefault();
                    event.stopPropagation();
                    if (editableRef.current.innerText.trim() === "") {
                        editableRef.current.innerText = originalName;
                    }
                    editableRef.current.blur();
                } else if (event.key === "Escape") {
                    editableRef.current.innerText = originalName;
                    editableRef.current.blur();
                    event.preventDefault();
                    event.stopPropagation();
                } else if (curLen >= 14 && !["Backspace", "Delete", "ArrowLeft", "ArrowRight"].includes(event.key)) {
                    event.preventDefault();
                    event.stopPropagation();
                }
            };

            useEffect(() => {
                if (!loadedRef.current) {
                    onLoaded();
                    loadedRef.current = true;
                }
            }, [onLoaded]);

            useEffect(() => {
                if (tabRef.current && isNew) {
                    const initialWidth = `${(tabWidth / 3) * 2}px`;
                    tabRef.current.style.setProperty("--initial-tab-width", initialWidth);
                    tabRef.current.style.setProperty("--final-tab-width", `${tabWidth}px`);
                }
            }, [isNew, tabWidth]);

            // Prevent drag from being triggered on mousedown
            const handleMouseDownOnClose = (event: React.MouseEvent<HTMLButtonElement, MouseEvent>) => {
                event.stopPropagation();
            };

            function handleContextMenu(e: React.MouseEvent<HTMLDivElement, MouseEvent>) {
                e.preventDefault();
                let menu: ContextMenuItem[] = [];
                const fullConfig = globalStore.get(atoms.fullConfigAtom);
                const bgPresets: string[] = [];
                for (const key in fullConfig?.presets ?? {}) {
                    if (key.startsWith("bg@")) {
                        bgPresets.push(key);
                    }
                }
                bgPresets.sort((a, b) => {
                    const aOrder = fullConfig.presets[a]["display:order"] ?? 0;
                    const bOrder = fullConfig.presets[b]["display:order"] ?? 0;
                    return aOrder - bOrder;
                });
                menu.push({ label: "Rename Tab", click: () => handleRenameTab(null) });
                menu.push({ label: "Copy TabId", click: () => navigator.clipboard.writeText(id) });
                menu.push({ type: "separator" });
                if (bgPresets.length > 0) {
                    const submenu: ContextMenuItem[] = [];
                    const oref = WOS.makeORef("tab", id);
                    for (const presetName of bgPresets) {
                        const preset = fullConfig.presets[presetName];
                        if (preset == null) {
                            continue;
                        }
                        submenu.push({
                            label: preset["display:name"] ?? presetName,
                            click: () => {
                                services.ObjectService.UpdateObjectMeta(oref, preset);
                                RpcApi.ActivityCommand(TabRpcClient, { settabtheme: 1 });
                            },
                        });
                    }
                    menu.push({ label: "Backgrounds", type: "submenu", submenu });
                    menu.push({ type: "separator" });
                }
                menu.push({ label: "Close Tab", click: () => onClose(null) });
                ContextMenuModel.showContextMenu(menu, e);
            }

            const showSeparator = useCallback(
                (id) => {
                    if (isFirst) return false;

                    const idx = tabIds.indexOf(id);
                    const found = tabIndicesMoved.find((i, ii) => ii !== 0 && i === idx) === undefined;
                    return found;
                },
                [tabIndicesMoved, isFirst, active]
            );

            console.log("showSeparator(id)=====", id, showSeparator(id));

            return (
                <div
                    ref={tabRef}
                    className={clsx("tab", {
                        active,
                        isDragging,
                        "before-active": isBeforeActive,
                        "new-tab": isNew,
                    })}
                    onMouseDown={onMouseDown}
                    onClick={onClick}
                    onContextMenu={handleContextMenu}
                    onMouseEnter={onMouseEnter}
                    onMouseLeave={onMouseLeave}
                    data-tab-id={id}
                >
                    {showSeparator(id) && <div className="separator"></div>}
                    <div className="tab-inner">
                        <div
                            ref={editableRef}
                            className={clsx("name", { focused: isEditable })}
                            contentEditable={isEditable}
                            onDoubleClick={handleRenameTab}
                            onBlur={handleBlur}
                            onKeyDown={handleKeyDown}
                            suppressContentEditableWarning={true}
                        >
                            {/* {tabData?.name} */}
                            {id.substring(id.length - 3)}
                        </div>
                        <Button className="ghost grey close" onClick={onClose} onMouseDown={handleMouseDownOnClose}>
                            <i className="fa fa-solid fa-xmark" />
                        </Button>
                    </div>
                </div>
            );
        }
    )
);

export { Tab };
