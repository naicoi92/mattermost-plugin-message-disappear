import type {Reducer} from 'redux';
import type {TTLInfo} from 'client';

export const REDUCER_KEY = 'disappearingMessages';

export interface DisappearState {
    // channelId -> TTL (null = explicitly off; absent = not yet loaded)
    byChannel: Record<string, TTLInfo | null>;
    modalChannelId: string | null;
}

const initialState: DisappearState = {byChannel: {}, modalChannelId: null};

interface SetChannelTTLAction {
    type: 'DISAPPEAR/SET_CHANNEL_TTL';
    channelId: string;
    ttl: TTLInfo | null;
}
interface OpenModalAction {
    type: 'DISAPPEAR/OPEN_MODAL';
    channelId: string;
}
interface CloseModalAction {
    type: 'DISAPPEAR/CLOSE_MODAL';
}
export type DisappearAction = SetChannelTTLAction | OpenModalAction | CloseModalAction;

export const setChannelTTL = (channelId: string, ttl: TTLInfo | null): SetChannelTTLAction => ({
    type: 'DISAPPEAR/SET_CHANNEL_TTL',
    channelId,
    ttl,
});
export const openModal = (channelId: string): OpenModalAction => ({type: 'DISAPPEAR/OPEN_MODAL', channelId});
export const closeModal = (): CloseModalAction => ({type: 'DISAPPEAR/CLOSE_MODAL'});

const reducer: Reducer<DisappearState, DisappearAction> = (state = initialState, action) => {
    switch (action.type) {
        case 'DISAPPEAR/SET_CHANNEL_TTL':
            return {...state, byChannel: {...state.byChannel, [action.channelId]: action.ttl}};
        case 'DISAPPEAR/OPEN_MODAL':
            return {...state, modalChannelId: action.channelId};
        case 'DISAPPEAR/CLOSE_MODAL':
            return {...state, modalChannelId: null};
        default:
            return state;
    }
};

export default reducer;

// Selectors read the plugin's slice off the global Mattermost webapp store.
export type GlobalSlice = {[REDUCER_KEY]?: DisappearState};

// GlobalState is the part of the Mattermost webapp store this plugin touches:
// the current channel id (mattermost-redux) plus our slice. A superset of
// GlobalSlice, so it is accepted by the selectors above.
export interface GlobalState {
    entities: {channels: {currentChannelId: string}};
    [REDUCER_KEY]?: DisappearState;
    [k: string]: unknown;
}
const slice = (state: GlobalSlice): DisappearState => state[REDUCER_KEY] ?? initialState;

export const selectChannelTTL = (state: GlobalSlice, channelId: string): TTLInfo | null | undefined =>
    slice(state).byChannel[channelId];

export const selectModalChannel = (state: GlobalSlice): string | null => slice(state).modalChannelId;
