import {useEffect} from 'react';
import {useDispatch, useSelector} from 'react-redux';

import {getTTL} from 'client';
import {DisappearAction, GlobalSlice, selectChannelTTL, setChannelTTL} from 'reducer';

// Per page-load set of channels whose TTL has already been requested, so the
// post badge and the channel-header button don't each re-fetch the same channel.
const fetchedChannels = new Set<string>();

// useChannelTTL returns the channel's TTL, lazy-loading it once per channel and
// then reacting to ttl_changed WebSocket updates (kept in the store by index.tsx).
//   - undefined  -> not yet loaded (and a load is in flight)
//   - null       -> explicitly off
//   - TTLInfo    -> a TTL is set
export function useChannelTTL(channelId: string | null | undefined) {
    const dispatch = useDispatch() as (action: DisappearAction) => void;
    const ttl = useSelector((state: GlobalSlice) => (channelId ? selectChannelTTL(state, channelId) : undefined));

    useEffect(() => {
        if (!channelId || ttl !== undefined || fetchedChannels.has(channelId)) {
            return;
        }
        fetchedChannels.add(channelId);
        let cancelled = false;
        getTTL(channelId).
            then((t) => {
                if (!cancelled) {
                    dispatch(setChannelTTL(channelId, t));
                }
            }).
            catch(() => {
                // Leave ttl undefined; a ttl_changed WebSocket update can populate later.
            });
        return () => {
            cancelled = true;
        };
    }, [channelId, ttl, dispatch]);

    return ttl;
}
