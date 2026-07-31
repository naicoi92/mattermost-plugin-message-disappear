import {useDispatch, useSelector} from 'react-redux';

import {useChannelTTL} from 'hooks/use_channel_ttl';
import {DisappearAction, GlobalState, openModal} from 'reducer';
import {shortDuration} from 'presets';

// ChannelHeaderButton is registered via registerChannelHeaderIcon (Mattermost 11.5+),
// which renders it in the LEFT icon section of the channel header (next to the
// pinned-posts button) as a FULL component — not constrained to icon size. It shows
// ⏱ + duration when a TTL is set, a muted ⏱ when off; hover shows the full status
// (duration, who set it, when); clicking opens the TTL selector for the channel.
// registerChannelHeaderIcon has no action callback, so the click is handled here.
export default function ChannelHeaderButton() {
    const dispatch = useDispatch() as (action: DisappearAction) => void;
    const channelId = useSelector((state: GlobalState) => state.entities.channels.currentChannelId);
    const ttl = useChannelTTL(channelId);
    const open = () => dispatch(openModal(channelId));

    if (!ttl) {
        return (
            <span
                aria-label='Disappearing messages off'
                className='disappear-header-icon disappear-header-icon--off'
                role='button'
                onClick={open}
            >
                {'\u23F1'}
            </span>
        );
    }

    const detail = `Disappearing: auto-delete after ${shortDuration(ttl.duration)}` +
        (ttl.set_by ? ` · set by ${ttl.set_by}` : '') +
        (ttl.set_at ? ` · ${new Date(ttl.set_at).toLocaleString()}` : '');

    return (
        <span
            aria-label={`Disappearing: ${shortDuration(ttl.duration)}`}
            className='disappear-header-icon disappear-header-icon--on'
            title={detail}
            role='button'
            onClick={open}
        >
            {'\u23F1'} {shortDuration(ttl.duration)}
        </span>
    );
}
