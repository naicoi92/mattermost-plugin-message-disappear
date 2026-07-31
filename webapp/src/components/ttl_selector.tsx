import {useEffect, useState} from 'react';
import {useDispatch, useSelector} from 'react-redux';

import {clearTTL, getTTL, setTTL, TTLInfo} from 'client';
import {closeModal, DisappearAction, GlobalSlice, selectModalChannel, setChannelTTL} from 'reducer';

// Presets mirror the server (server/ttl/ttl.go); "30s" is intentionally absent
// (the server's minimum TTL is 1 minute).
const PRESETS = [
    {label: '5 minutes', seconds: 300},
    {label: '1 hour', seconds: 3600},
    {label: '8 hours', seconds: 28800},
    {label: '1 day', seconds: 86400},
    {label: '1 week', seconds: 604800},
];

const MIN_SECONDS = 60;
const MAX_SECONDS = 365 * 24 * 3600;

// TTLSelectorModal is registered as a root component; it renders only while a
// channel is selected (selectModalChannel), driven by the channel-header button.
export default function TTLSelectorModal() {
    const dispatch = useDispatch() as (action: DisappearAction) => void;
    const channelId = useSelector((state: GlobalSlice) => selectModalChannel(state));
    const [current, setCurrent] = useState<TTLInfo | null>(null);
    const [seconds, setSeconds] = useState(3600);
    const [error, setError] = useState<string | null>(null);
    const [busy, setBusy] = useState(false);

    useEffect(() => {
        if (!channelId) {
            return;
        }
        setCurrent(null);
        setError(null);
        getTTL(channelId).
            then((t) => {
                setCurrent(t);
                setSeconds(t?.duration ?? 3600);
            }).
            catch(() => setError('Could not load the current TTL.'));
    }, [channelId]);

    if (!channelId) {
        return null;
    }

    const onSave = async () => {
        if (seconds < MIN_SECONDS || seconds > MAX_SECONDS) {
            setError(`TTL must be between ${MIN_SECONDS}s (1 minute) and ${MAX_SECONDS}s (1 year).`);
            return;
        }
        setBusy(true);
        setError(null);
        try {
            await setTTL(channelId, seconds);
            const t = await getTTL(channelId);
            dispatch(setChannelTTL(channelId, t));
            dispatch(closeModal());
        } catch (e) {
            setError(e instanceof Error ? e.message : 'Failed to set TTL.');
        } finally {
            setBusy(false);
        }
    };

    const onOff = async () => {
        setBusy(true);
        setError(null);
        try {
            await clearTTL(channelId);
            dispatch(setChannelTTL(channelId, null));
            dispatch(closeModal());
        } catch (e) {
            setError(e instanceof Error ? e.message : 'Failed to clear TTL.');
        } finally {
            setBusy(false);
        }
    };

    return (
        <div className='disappear-modal-backdrop' role='dialog' aria-modal='true' aria-labelledby='disappear-title'>
            <div className='disappear-modal'>
                <h2 id='disappear-title'>{'Disappearing messages'}</h2>
                <p className='disappear-warning'>
                    {'\u26A0 '}
                    <strong>{'Not end-to-end encrypted.'}</strong>
                    {' The server can read these messages until they are deleted.'}
                </p>
                <p className='disappear-current'>
                    {current ? `Currently: auto-delete after ${current.duration}s` : 'Currently: off'}
                </p>
                <div className='disappear-presets' role='group' aria-label='TTL presets'>
                    {PRESETS.map((p) => (
                        <button
                            key={p.seconds}
                            type='button'
                            className='disappear-preset'
                            aria-pressed={seconds === p.seconds}
                            onClick={() => setSeconds(p.seconds)}
                        >{p.label}</button>
                    ))}
                </div>
                <label className='disappear-custom'>
                    {`Custom (seconds, ${MIN_SECONDS}–${MAX_SECONDS}):`}
                    <input
                        type='number'
                        min={MIN_SECONDS}
                        max={MAX_SECONDS}
                        value={seconds}
                        onChange={(e) => setSeconds(Number(e.target.value))}
                    />
                </label>
                {error && <p className='disappear-error' role='alert'>{error}</p>}
                <div className='disappear-actions'>
                    <button type='button' className='disappear-off' onClick={onOff} disabled={busy}>{'Turn off'}</button>
                    <button type='button' className='disappear-save' onClick={onSave} disabled={busy}>{'Save'}</button>
                    <button type='button' className='disappear-cancel' onClick={() => dispatch(closeModal())} disabled={busy}>{'Cancel'}</button>
                </div>
            </div>
        </div>
    );
}
