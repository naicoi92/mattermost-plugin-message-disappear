import {useEffect} from 'react';
import {useDispatch, useSelector} from 'react-redux';

import {getTTL} from 'client';
import {DisappearAction, GlobalSlice, selectChannelTTL, setChannelTTL} from 'reducer';
import type {PostRenderArgs} from 'types/mattermost-webapp';

// TTLBadge renders the ⏱ marker on posts in channels that have a TTL set.
// It lazily loads the channel's TTL into the store on first render and reacts
// to ttl_changed WebSocket updates (kept in the store by index.tsx).
export default function TTLBadge({post}: PostRenderArgs) {
    const ttl = useSelector((state: GlobalSlice) => selectChannelTTL(state, post.channel_id));
    const dispatch = useDispatch() as (action: DisappearAction) => void;

    useEffect(() => {
        if (ttl !== undefined) {
            return; // loaded already (null = off, TTLInfo = on)
        }
        let cancelled = false;
        getTTL(post.channel_id).
            then((t) => {
                if (!cancelled) {
                    dispatch(setChannelTTL(post.channel_id, t));
                }
            }).
            catch(() => {
                // leave undefined; nothing to badge
            });
        return () => {
            cancelled = true;
        };
    }, [post.channel_id, ttl, dispatch]);

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
