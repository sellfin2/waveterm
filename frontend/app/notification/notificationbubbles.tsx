import { atoms } from "@/store/global";
import { FloatingPortal, useFloating, useInteractions } from "@floating-ui/react";
import { useAtomValue } from "jotai";
import { useEffect, useState } from "react";
import { NotificationItem } from "./notificationitem";
import { useNotification } from "./usenotification";

import clsx from "clsx";
import "./notificationBubbles.less";

const NotificationBubbles = () => {
    const {
        notifications,
        hoveredId,
        hideNotification,
        copyNotification,
        handleActionClick,
        formatTimestamp,
        setHoveredId,
    } = useNotification();
    const [isOpen, setIsOpen] = useState(notifications.length > 0);
    const notificationPopoverMode = useAtomValue(atoms.notificationPopoverMode);

    useEffect(() => {
        setIsOpen(notifications.length > 0);
    }, [notifications.length]);

    const { refs, strategy } = useFloating({
        open: isOpen,
        onOpenChange: setIsOpen,
        strategy: "fixed",
    });

    const { getFloatingProps } = useInteractions();

    const floatingStyles = {
        position: strategy,
        right: "58px",
        bottom: "10px",
        top: "auto",
        left: "auto",
    };

    if (!isOpen || notificationPopoverMode) {
        return null;
    }

    return (
        <FloatingPortal>
            <div
                ref={refs.setFloating}
                style={floatingStyles}
                className="notification-bubbles"
                {...getFloatingProps({
                    onClick: (e) => e.stopPropagation(),
                })}
            >
                {notifications.map((notif) => {
                    if (notif.hidden) return null;
                    return (
                        <NotificationItem
                            key={notif.id}
                            className={clsx({ hovered: hoveredId === notif.id })}
                            notification={notif}
                            onRemove={hideNotification}
                            onCopy={copyNotification}
                            onActionClick={handleActionClick}
                            formatTimestamp={formatTimestamp}
                            onMouseEnter={() => setHoveredId(notif.id)}
                            onMouseLeave={() => setHoveredId(null)}
                            isBubble={true}
                        />
                    );
                })}
            </div>
        </FloatingPortal>
    );
};

export { NotificationBubbles };
