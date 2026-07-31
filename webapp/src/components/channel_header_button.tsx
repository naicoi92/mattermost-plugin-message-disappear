import {useSelector} from 'react-redux';

import {useChannelTTL} from 'hooks/use_channel_ttl';
import {GlobalState} from 'reducer';
import {shortDuration} from 'presets';

// ChannelHeaderButton is the status-aware channel-header icon: ⏱ + the active
// duration when a TTL is set, and a muted ⏱ when off. Hover shows the full status
// (duration, who set it, when). Clicking opens the TTL selector (wired in index.tsx).
export default function ChannelHeaderButton() {
    const channelId = useSelector((state: GlobalState) => state.entities.channels.currentChannelId);
    const ttl = useChannelTTL(channelId);

    if (!ttl) {
        return (
            <span
                aria-label='Disappearing messages off'
                className='disappear-header-icon disappear-header-icon--off'
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
        >
            {'\u23F1'} {shortDuration(ttl.duration)}
        </span>
    );
}
