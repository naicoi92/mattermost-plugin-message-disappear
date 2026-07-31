import {useEffect, useRef, useState} from 'react';

import {useChannelTTL} from 'hooks/use_channel_ttl';
import {clearTTL, setTTL} from 'client';
import {PRESETS, shortDuration} from 'presets';

// ChannelHeaderButton is registered via registerChannelHeaderIcon (Mattermost
// 11.5+). The webapp renders it in the channel header and passes the `channel`
// (and channelMember) as props; we use channel.id as the authoritative channel id.
// (Reading state.entities.channels.currentChannelId from redux previously sent an
// id the server rejected with "ttl: channel not found".)
//
// It is a small button showing ⏱ + the current TTL ("Off" or a short duration);
// clicking opens a dropdown of presets + Off for fast setup. registerChannelHeaderIcon
// has no action callback, so the click + dropdown live entirely in this component.
export default function ChannelHeaderButton({channel}: {channel?: {id: string}}) {
    const channelId = channel?.id ?? '';
    const ttl = useChannelTTL(channelId);
    const [open, setOpen] = useState(false);
    const rootRef = useRef<HTMLDivElement>(null);

    const label = ttl ? shortDuration(ttl.duration) : 'Off';

    // Close on outside click / Escape while the dropdown is open.
    useEffect(() => {
        if (!open) {
            return;
        }
        const onDown = (e: MouseEvent) => {
            if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
                setOpen(false);
            }
        };
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') {
                setOpen(false);
            }
        };
        document.addEventListener('mousedown', onDown);
        document.addEventListener('keydown', onKey);
        return () => {
            document.removeEventListener('mousedown', onDown);
            document.removeEventListener('keydown', onKey);
        };
    }, [open]);

    // Fire-and-forget a maybe-async action (setTTL/clearTTL return a promise; a
    // failure is logged server-side and the ttl_changed WS keeps the store honest).
    const fire = (p: unknown) => {
        Promise.resolve(p).catch(() => {});
    };

    const choose = (action: () => unknown) => {
        fire(action());
        setOpen(false);
    };

    return (
        <div className='disappear-dropdown' ref={rootRef}>
            <button
                type='button'
                className={`disappear-toggle ${ttl ? 'disappear-toggle--on' : 'disappear-toggle--off'}`}
                aria-label={`Disappearing: ${label}`}
                aria-expanded={open}
                aria-haspopup='menu'
                onClick={() => setOpen((v) => !v)}
            >
                <span aria-hidden='true'>{'\u23F1'}</span> {label}
            </button>
            {open && (
                <div className='disappear-menu' role='menu'>
                    <button
                        type='button'
                        role='menuitem'
                        className='disappear-menuitem'
                        onClick={() => choose(() => clearTTL(channelId))}
                    >
                        Off
                    </button>
                    {PRESETS.map((p) => (
                        <button
                            key={p.seconds}
                            type='button'
                            role='menuitem'
                            className='disappear-menuitem'
                            aria-current={ttl?.duration === p.seconds || undefined}
                            onClick={() => choose(() => setTTL(channelId, p.seconds))}
                        >
                            {p.shortLabel}
                        </button>
                    ))}
                </div>
            )}
        </div>
    );
}
