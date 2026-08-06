import {useEffect, useRef, useState} from 'react';
import {useDispatch, useSelector} from 'react-redux';

import {clearTTL, setTTL, TTLInfo} from 'client';
import {useChannelTTL} from 'hooks/use_channel_ttl';
import {PRESETS, shortDuration} from 'presets';
import {DisappearAction, GlobalState, setChannelTTL} from 'reducer';

// ChannelHeaderButton is registered via registerChannelHeaderIcon (Mattermost
// 11.5+). The webapp renders it in the channel header; we use channel.id (falling
// back to the redux current channel id) as the authoritative channel id.
//
// It shows ⏱ + the current TTL ("Off" or a short duration); clicking opens a
// dropdown of presets + Off. Selecting applies immediately and updates the label
// at once (optimistic), so the UI does not wait on the ttl_changed WebSocket
// round-trip — that event still confirms the change for OTHER clients.
export default function ChannelHeaderButton({channel}: {channel?: {id: string}}) {
    const dispatch = useDispatch() as (action: DisappearAction) => void;
    const currentChannelId = useSelector((state: GlobalState) => state.entities.channels.currentChannelId);
    const channelId = channel?.id || currentChannelId || '';
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

    // apply runs the set/clear, then updates the store optimistically so the label
    // flips immediately. Failures are logged to the console. The ttl_changed WS
    // event (published by the server) still propagates the change to other clients.
    const apply = (action: unknown, next: TTLInfo | null) => {
        Promise.resolve(action).
            then(() => dispatch(setChannelTTL(channelId, next))).
            catch((e) => console.error('disappear: TTL action failed', e));
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
                        onClick={() => apply(clearTTL(channelId), null)}
                    >
                        {'Off'}
                    </button>
                    {PRESETS.map((p) => (
                        <button
                            key={p.seconds}
                            type='button'
                            role='menuitem'
                            className='disappear-menuitem'
                            aria-current={ttl?.duration === p.seconds || undefined}
                            onClick={() => apply(setTTL(channelId, p.seconds), {
                                duration: p.seconds,
                                set_by: '',
                                set_at: Date.now(),
                            })}
                        >
                            {p.label}
                        </button>
                    ))}
                </div>
            )}
        </div>
    );
}
