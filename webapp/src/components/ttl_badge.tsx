import {useEffect} from 'react';
import {useDispatch, useSelector} from 'react-redux';

import {getTTL} from 'client';
import {DisappearAction, GlobalSlice, selectChannelTTL, setChannelTTL} from 'reducer';
import type {PostRenderArgs} from 'types/mattermost-webapp';

// Per page-load set of channels whose TTL has already been requested, so the
// many TTLBadge instances across a busy channel list don't each re-fetch.
const fetchedChannels = new Set<string>();

// TTLBadge renders the ⏱ marker on posts in channels that have a TTL set.
// It loads each channel's TTL once into the store and then reacts to
// ttl_changed WebSocket updates (kept in the store by index.tsx).
export default function TTLBadge({post}: PostRenderArgs) {
    const ttl = useSelector((state: GlobalSlice) => selectChannelTTL(state, post.channel_id));
    const dispatch = useDispatch() as (action: DisappearAction) => void;

    useEffect(() => {
        // Fetch once per channel; ttl is intentionally NOT a dependency, so a
        // failed fetch (ttl left undefined) cannot trigger a retry loop when an
        // unrelated store update re-renders the component.
        if (fetchedChannels.has(post.channel_id)) {
            return;
        }
        fetchedChannels.add(post.channel_id);
        let cancelled = false;
        getTTL(post.channel_id).
            then((t) => {
                if (!cancelled) {
                    dispatch(setChannelTTL(post.channel_id, t));
                }
            }).
            catch(() => {
                // Leave ttl undefined; the badge stays hidden until a ttl_changed
                // WebSocket update populates the store.
            });
        return () => {
            cancelled = true;
        };
    }, [post.channel_id, dispatch]);

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
