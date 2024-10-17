import { getWebServerEndpoint } from "@/util/endpoints";
import { boundNumber, isBlank } from "@/util/util";
import useResizeObserver from "@react-hook/resize-observer";
import { generate, parse, walk } from "css-tree";
import { useAtomValue } from "jotai";
import { CSSProperties, useCallback, useLayoutEffect, useRef } from "react";
import { debounce } from "throttle-debounce";
import { atoms, getApi, PLATFORM, WOS } from "./store/global";
import { useWaveObjectValue } from "./store/wos";

function encodeFileURL(file: string) {
    const webEndpoint = getWebServerEndpoint();
    return webEndpoint + `/wave/stream-file?path=${encodeURIComponent(file)}&no404=1`;
}

function processBackgroundUrls(cssText: string): string {
    if (isBlank(cssText)) {
        return null;
    }
    cssText = cssText.trim();
    if (cssText.endsWith(";")) {
        cssText = cssText.slice(0, -1);
    }
    const attrRe = /^background(-image):\s*/;
    cssText = cssText.replace(attrRe, "");
    const ast = parse("background: " + cssText, {
        context: "declaration",
    });
    let hasJSUrl = false;
    walk(ast, {
        visit: "Url",
        enter(node) {
            const originalUrl = node.value.trim();
            if (originalUrl.startsWith("javascript:")) {
                hasJSUrl = true;
                return;
            }
            if (originalUrl.startsWith("data:")) {
                return;
            }
            const newUrl = encodeFileURL(originalUrl);
            node.value = newUrl;
        },
    });
    if (hasJSUrl) {
        console.log("invalid background, contains a 'javascript' protocol url which is not allowed");
        return null;
    }
    const rtnStyle = generate(ast);
    if (rtnStyle == null) {
        return null;
    }
    return rtnStyle.replace(/^background:\s*/, "");
}

export function AppBackground() {
    const bgRef = useRef<HTMLDivElement>(null);
    const tabId = useAtomValue(atoms.activeTabId);
    const [tabData] = useWaveObjectValue<Tab>(WOS.makeORef("tab", tabId));
    const bgAttr = tabData?.meta?.bg;
    const style: CSSProperties = {};
    if (!isBlank(bgAttr)) {
        try {
            const processedBg = processBackgroundUrls(bgAttr);
            if (!isBlank(processedBg)) {
                const opacity = boundNumber(tabData?.meta?.["bg:opacity"], 0, 1) ?? 0.5;
                style.opacity = opacity;
                style.background = processedBg;
                const blendMode = tabData?.meta?.["bg:blendmode"];
                if (!isBlank(blendMode)) {
                    style.backgroundBlendMode = blendMode;
                }
            }
        } catch (e) {
            console.error("error processing background", e);
        }
    }
    const getAvgColor = useCallback(
        debounce(30, () => {
            if (
                bgRef.current &&
                PLATFORM !== "darwin" &&
                bgRef.current &&
                "windowControlsOverlay" in window.navigator
            ) {
                const titlebarRect: Dimensions = (window.navigator.windowControlsOverlay as any).getTitlebarAreaRect();
                const bgRect = bgRef.current.getBoundingClientRect();
                if (titlebarRect && bgRect) {
                    const windowControlsLeft = titlebarRect.width - titlebarRect.height;
                    const windowControlsRect: Dimensions = {
                        top: titlebarRect.top,
                        left: windowControlsLeft,
                        height: titlebarRect.height,
                        width: bgRect.width - bgRect.left - windowControlsLeft,
                    };
                    getApi().updateWindowControlsOverlay(windowControlsRect);
                }
            }
        }),
        [bgRef, style]
    );
    useLayoutEffect(getAvgColor, [getAvgColor]);
    useResizeObserver(bgRef, getAvgColor);

    return <div ref={bgRef} className="app-background" style={style} />;
}
