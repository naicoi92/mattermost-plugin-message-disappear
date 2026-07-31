import {useChannelTTL} from 'hooks/use_channel_ttl';
import type {PostRenderArgs} from 'types/mattermost-webapp';

// TTLBadge renders the ⏱ marker on posts in channels that have a TTL set.
// The TTL is loaded + cached by useChannelTTL (shared with the channel-header button).
export default function TTLBadge({post}: PostRenderArgs) {
    const ttl = useChannelTTL(post.channel_id);
    if (!ttl) {
        return null;
    }
    return (
        <span
            className='disappear-badge'
            title={`Disappearing messages: auto-delete after ${ttl.duration}s`}
            aria-label='Disappearing messages enabled'
        >
            {'\u23F1'}
        </span>
    );
}
