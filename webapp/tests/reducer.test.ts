import manifest from 'manifest';
import reducer, {closeModal, openModal, PLUGIN_REDUCER_KEY, selectChannelTTL, setChannelTTL} from 'reducer';
import type {DisappearState} from 'reducer';

const ttl = {duration: 300, set_by: 'u1', set_at: 7};

it('setChannelTTL stores ttl keyed by channel', () => {
    const s = reducer(undefined, setChannelTTL('ch1', ttl));
    expect(s.byChannel.ch1).toEqual(ttl);
});

it('setChannelTTL null marks the channel off', () => {
    const initial: DisappearState = {byChannel: {ch1: ttl}, modalChannelId: null};
    const s = reducer(initial, setChannelTTL('ch1', null));
    expect(s.byChannel.ch1).toBeNull();
});

it('openModal then closeModal toggles modalChannelId', () => {
    const opened = reducer(undefined, openModal('ch2'));
    expect(opened.modalChannelId).toBe('ch2');
    const closed = reducer(opened, closeModal());
    expect(closed.modalChannelId).toBeNull();
});

it('setChannelTTL does not mutate other channels', () => {
    const initial: DisappearState = {byChannel: {ch1: ttl}, modalChannelId: null};
    const s = reducer(initial, setChannelTTL('ch2', {...ttl, duration: 3600}));
    expect(s.byChannel.ch1).toEqual(ttl);
    expect(s.byChannel.ch2?.duration).toBe(3600);
});

it('unknown action returns the same state reference (redux contract)', () => {
    const before = reducer(undefined, openModal('ch9'));
    const after = reducer(before, {type: 'UNKNOWN'} as unknown as Parameters<typeof reducer>[1]);
    expect(after).toBe(before);
});

it('selectChannelTTL reads the slice from state[plugins-<id>]', () => {
    const state: Record<string, unknown> = {
        [PLUGIN_REDUCER_KEY]: {byChannel: {ch1: ttl}, modalChannelId: null},
    };
    expect(selectChannelTTL(state, 'ch1')).toEqual(ttl);
    expect(selectChannelTTL(state, 'absent')).toBeUndefined();
});

it('selectChannelTTL ignores any legacy root key (regression: must read plugins-<id>)', () => {
    const state: Record<string, unknown> = {disappearingMessages: {byChannel: {ch1: ttl}}};
    expect(selectChannelTTL(state, 'ch1')).toBeUndefined();
    expect(PLUGIN_REDUCER_KEY).toBe('plugins-' + manifest.id);
});
