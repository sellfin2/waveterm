// Copyright 2024, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { Button } from "@/element/button";
import { ContextMenuModel } from "@/store/contextmenu";
import { fireAndForget } from "@/util/util";
import { clsx } from "clsx";
import { atom, useAtom, useAtomValue } from "jotai";
import { forwardRef, memo, useCallback, useEffect, useImperativeHandle, useRef, useState } from "react";

import { atoms, globalStore, refocusNode } from "@/app/store/global";
import { RpcApi } from "@/app/store/wshclientapi";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { ObjectService } from "../store/services";
import { makeORef, useWaveObjectValue } from "../store/wos";

import "./tab.scss";

const adjacentTabsAtom = atom<Set<string>>(new Set<string>());

interface TabProps {
    id: string;
    isActive: boolean;
    isBeforeActive: boolean;
    draggingId: string;
    tabWidth: number;
    isNew: boolean;
    isPinned: boolean;
    tabIds: string[];
    tabRefs: React.MutableRefObject<React.RefObject<HTMLDivElement>[]>;
    onClick: () => void;
    onClose: (event: React.MouseEvent<HTMLButtonElement, MouseEvent> | null) => void;
    onMouseDown: (event: React.MouseEvent<HTMLDivElement, MouseEvent>) => void;
    onLoaded: () => void;
    onPinChange: () => void;
}

const Tab = memo(
    forwardRef<HTMLDivElement, TabProps>(
        (
            {
                id,
                isActive,
                isPinned,
                isBeforeActive,
                draggingId,
                tabWidth,
                isNew,
                tabIds,
                tabRefs,
                onLoaded,
                onClick,
                onClose,
                onMouseDown,
                onPinChange,
            },
            ref
        ) => {
            const [tabData, _] = useWaveObjectValue<Tab>(makeORef("tab", id));
            const [originalName, setOriginalName] = useState("");
            const [isEditable, setIsEditable] = useState(false);

            const editableRef = useRef<HTMLDivElement>(null);
            const editableTimeoutRef = useRef<NodeJS.Timeout>();
            const loadedRef = useRef(false);
            const tabRef = useRef<HTMLDivElement>(null);
            const adjacentTabsRef = useRef<Set<string>>(new Set());

            const tabsSwapped = useAtomValue<boolean>(atoms.tabsSwapped);
            const tabs = document.querySelectorAll(".tab");
            const [adjacentTabs, setAdjacentTabs] = useAtom(adjacentTabsAtom);

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

            const selectEditableText = useCallback(() => {
                if (editableRef.current) {
                    const range = document.createRange();
                    const selection = window.getSelection();
                    range.selectNodeContents(editableRef.current);
                    selection.removeAllRanges();
                    selection.addRange(range);
                }
            }, []);

            const handleRenameTab: React.MouseEventHandler<HTMLDivElement> = (event) => {
                event?.stopPropagation();
                setIsEditable(true);
                editableTimeoutRef.current = setTimeout(() => {
                    selectEditableText();
                }, 0);
            };

            const handleBlur = () => {
                let newText = editableRef.current.innerText.trim();
                newText = newText || originalName;
                editableRef.current.innerText = newText;
                setIsEditable(false);
                fireAndForget(() => ObjectService.UpdateTabName(id, newText));
                setTimeout(() => refocusNode(null), 10);
            };

            const handleKeyDown: React.KeyboardEventHandler<HTMLDivElement> = (event) => {
                if ((event.metaKey || event.ctrlKey) && event.key === "a") {
                    event.preventDefault();
                    selectEditableText();
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

            const handleContextMenu = useCallback(
                (e: React.MouseEvent<HTMLDivElement, MouseEvent>) => {
                    e.preventDefault();
                    let menu: ContextMenuItem[] = [
                        { label: isPinned ? "Unpin Tab" : "Pin Tab", click: () => onPinChange() },
                        { label: "Rename Tab", click: () => handleRenameTab(null) },
                        {
                            label: "Copy TabId",
                            click: () => fireAndForget(() => navigator.clipboard.writeText(id)),
                        },
                        { type: "separator" },
                    ];
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
                    if (bgPresets.length > 0) {
                        const submenu: ContextMenuItem[] = [];
                        const oref = makeORef("tab", id);
                        for (const presetName of bgPresets) {
                            const preset = fullConfig.presets[presetName];
                            if (preset == null) {
                                continue;
                            }
                            submenu.push({
                                label: preset["display:name"] ?? presetName,
                                click: () =>
                                    fireAndForget(async () => {
                                        await ObjectService.UpdateObjectMeta(oref, preset);
                                        await RpcApi.ActivityCommand(TabRpcClient, { settabtheme: 1 });
                                    }),
                            });
                        }
                        menu.push({ label: "Backgrounds", type: "submenu", submenu }, { type: "separator" });
                    }
                    menu.push({ label: "Close Tab", click: () => onClose(null) });
                    ContextMenuModel.showContextMenu(menu, e);
                },
                [onPinChange, handleRenameTab, id, onClose, isPinned]
            );

            useEffect(() => {
                // Get the index of the current tab ID
                const currentIndex = tabIds.indexOf(id);
                // Get the right adjacent ID
                const rightAdjacentId = tabIds[currentIndex + 1];
                // Get the left adjacent ID
                const leftAdjacentId = tabIds[currentIndex - 1];

                const reset = () => {
                    if (!isActive) {
                        const currentTabElement = document.querySelector(`[data-tab-id="${id}"]`) as HTMLElement;
                        // To check if leftAdjacentElement is the active tab then do not reset opacity
                        const leftAdjacentElement = document.querySelector(
                            `[data-tab-id="${leftAdjacentId}"]`
                        ) as HTMLElement;
                        if (!currentTabElement || !leftAdjacentElement) return;
                        const separator = currentTabElement.querySelector(".separator") as HTMLElement;

                        if (!leftAdjacentElement.classList.contains("active")) {
                            separator.style.opacity = "1"; // Reset opacity for the current tab only if not active
                        }

                        const draggingTabElement = document.querySelector(
                            `[data-tab-id="${draggingId}"]`
                        ) as HTMLElement;
                        if (!draggingTabElement) return;

                        // If dragging tab is the first tab set opacity to 1
                        if (draggingId === tabIds[0]) {
                            const separator = draggingTabElement.querySelector(".separator") as HTMLElement;
                            separator.style.opacity = "1";
                        } else if (draggingId === tabIds[tabIds.length - 1]) {
                            // if daragging tab is the last tab set opacity of right separator to 1
                            const draggingTabElement = document.querySelector(
                                `[data-tab-id="${draggingId}"]`
                            ) as HTMLElement;
                            if (!draggingTabElement) return;
                            const separator = draggingTabElement.querySelector(".right-separator") as HTMLElement;
                            separator.style.opacity = "1";
                        }
                    }

                    if (rightAdjacentId) {
                        // To check if rightAdjacentElement is the active tab then do not reset opacity
                        const rightAdjacentElement = document.querySelector(
                            `[data-tab-id="${rightAdjacentId}"]`
                        ) as HTMLElement;
                        if (!rightAdjacentElement) return;
                        const separator = rightAdjacentElement.querySelector(".separator") as HTMLElement;

                        if (!rightAdjacentElement.classList.contains("active")) {
                            separator.style.opacity = "1"; // Reset opacity for the right adjacent tab
                        }
                    }
                };

                if (tabsSwapped || isActive) {
                    const currentTabElement = document.querySelector(`[data-tab-id="${id}"]`) as HTMLElement;
                    if (!currentTabElement) return;
                    const separator = currentTabElement.querySelector(".separator") as HTMLElement;
                    const rightSeparator = currentTabElement.querySelector(".right-separator") as HTMLElement;

                    if (isActive || draggingId === id) {
                        separator.style.opacity = "0";
                        if (rightSeparator) {
                            rightSeparator.style.opacity = "0";
                        }
                    }

                    // Set the opacity of the separator for the right adjacent tab
                    if (rightAdjacentId) {
                        const rightAdjacentTabElement = document.querySelector(
                            `[data-tab-id="${rightAdjacentId}"]`
                        ) as HTMLElement;
                        if (!rightAdjacentTabElement) return;
                        const separator = rightAdjacentTabElement.querySelector(".separator") as HTMLElement;

                        if (isActive || draggingId === id) {
                            separator.style.opacity = "0";
                        }
                    }

                    return () => {
                        reset();
                    };
                } else {
                    reset();
                }
            }, [id, tabIds, isActive, draggingId, tabsSwapped]);

            const handleMouseEnter = useCallback(() => {
                if (isActive) return;
                const currentTabElement = document.querySelector(`[data-tab-id="${id}"]`) as HTMLElement;
                if (currentTabElement) {
                    if (!tabsSwapped) {
                        currentTabElement.classList.add("hover");
                    }
                }
            }, [id, isActive, tabsSwapped]);

            const handleMouseLeave = useCallback(() => {
                if (isActive) return;
                const currentTabElement = document.querySelector(`[data-tab-id="${id}"]`) as HTMLElement;
                if (currentTabElement) {
                    currentTabElement.classList.remove("hover");
                }
            }, [id, isActive, tabsSwapped]);

            return (
                <div
                    ref={tabRef}
                    className={clsx("tab", {
                        active: isActive,
                        "is-pinned": isPinned,
                        "is-dragging": draggingId === id,
                        "before-active": isBeforeActive,
                        "new-tab": isNew,
                    })}
                    onMouseDown={onMouseDown}
                    onClick={onClick}
                    onContextMenu={handleContextMenu}
                    onMouseEnter={handleMouseEnter}
                    onMouseLeave={handleMouseLeave}
                    data-tab-id={id}
                >
                    <div className="separator"></div>
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
                            {tabData?.name}
                            {/* {id.substring(id.length - 3)} */}
                        </div>
                        {isPinned ? (
                            <Button
                                className="ghost grey pin"
                                onClick={(e) => {
                                    e.stopPropagation();
                                    onPinChange();
                                }}
                                title="Unpin Tab"
                            >
                                <i className="fa fa-solid fa-thumbtack" />
                            </Button>
                        ) : (
                            <Button
                                className="ghost grey close"
                                onClick={onClose}
                                onMouseDown={handleMouseDownOnClose}
                                title="Close Tab"
                            >
                                <i className="fa fa-solid fa-xmark" />
                            </Button>
                        )}
                    </div>
                    {tabIds[tabIds.length - 1] === id && <div className="right-separator"></div>}
                </div>
            );
        }
    )
);

export { Tab };
