import {useSelector} from 'react-redux';

import {useChannelTTL} from 'hooks/use_channel_ttl';
import {GlobalState} from 'reducer';
import {shortDuration} from 'presets';

// DisappearingHeaderIcon is the icon passed to registerChannelHeaderButtonAction.
// That slot constrains its first argument to an icon (~15px), so we render a Font
// Awesome stopwatch — dimmed when no TTL is set — with a native hover tooltip that
// carries the duration / who set it / when. Clicking is handled by the action
// callback wired in index.tsx (opens the TTL selector modal). This is the only
// channel-header button method available in Mattermost 10.x.
export default function DisappearingHeaderIcon() {
    const channelId = useSelector((state: GlobalState) => state.entities.channels.currentChannelId);
    const ttl = useChannelTTL(channelId);

    let title = 'Disappearing messages off';
    if (ttl) {
        title = `Disappearing: auto-delete after ${shortDuration(ttl.duration)}` +
            (ttl.set_by ? ` · set by ${ttl.set_by}` : '') +
            (ttl.set_at ? ` · ${new Date(ttl.set_at).toLocaleString()}` : '');
    }

    return (
        <i
            className='icon fa fa-stopwatch'
            style={{fontSize: '15px', position: 'relative', top: '-1px', opacity: ttl ? 1 : 0.45}}
            aria-label={title}
            title={title}
        />
    );
}
