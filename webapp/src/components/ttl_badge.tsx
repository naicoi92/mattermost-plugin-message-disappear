import {useSelector} from 'react-redux';

import {useChannelTTL} from 'hooks/use_channel_ttl';
import {GlobalState} from 'reducer';

// TTLBadge is registered via registerPostMessageAttachmentComponent, which renders
// it at the bottom of every post and passes `postId` (not the full post). We resolve
// the post's channel from the webapp redux store, then render ⏱ when that channel
// has a TTL set. Returns null for posts in channels without a TTL.
export default function TTLBadge({postId}: {postId?: string}) {
    const channelId = useSelector((state: GlobalState) =>
        // entities.posts is the webapp (mattermost-redux) state, not in our vendored
        // GlobalState subset, so narrow it locally via `unknown`.
        (state as unknown as {entities: {posts: {posts: Record<string, {channel_id?: string}>}}}).entities.posts.posts[postId ?? '']?.channel_id,
    );
    const ttl = useChannelTTL(channelId);
    if (!ttl) {
        return null;
    }
    return (
        <span
            className='disappear-badge'
            title={`Disappearing: auto-delete after ${ttl.duration}s`}
            aria-label='Disappearing messages enabled'
        >
            {'\u23F1'}
        </span>
    );
}
