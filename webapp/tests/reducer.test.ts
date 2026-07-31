import reducer, {closeModal, openModal, setChannelTTL} from 'reducer';
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
