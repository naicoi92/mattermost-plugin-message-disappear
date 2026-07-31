import {useEffect, useRef, useState} from 'react';
import {useDispatch, useSelector} from 'react-redux';

import {useChannelTTL} from 'hooks/use_channel_ttl';
import {clearTTL, setTTL} from 'client';
import {DisappearAction, GlobalState, openModal} from 'reducer';
import {PRESETS, shortDuration} from 'presets';

// ChannelHeaderButton is registered via registerChannelHeaderIcon (Mattermost
// 11.5+), which renders it in the left icon section of the channel header as a
// full component. It is a small button showing ⏱ + the current TTL ("Off" or a
// short duration); clicking it opens a dropdown to pick a preset, turn TTL off,
// or open the custom-duration modal. registerChannelHeaderIcon has no action
// callback, so the click + dropdown live entirely inside this component.
export default function ChannelHeaderButton() {
    const dispatch = useDispatch() as (action: DisappearAction) => void;
    const channelId = useSelector((state: GlobalState) => state.entities.channels.currentChannelId);
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
                        className='disappear-menuitem'
                        onClick={() => choose(() => clearTTL(channelId))}
                    >
                        Off
                    </button>
                    {PRESETS.map((p) => (
                        <button
                            key={p.seconds}
                            type='button'
                            className='disappear-menuitem'
                            aria-current={ttl?.duration === p.seconds || undefined}
                            onClick={() => choose(() => setTTL(channelId, p.seconds))}
                        >
                            {p.shortLabel}
                        </button>
                    ))}
                    <button
                        type='button'
                        className='disappear-menuitem'
                        onClick={() => choose(() => dispatch(openModal(channelId)))}
                    >
                        {'Custom\u2026'}
                    </button>
                </div>
            )}
        </div>
    );
}
