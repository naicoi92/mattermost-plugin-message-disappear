import type {Reducer} from 'redux';
import type {TTLInfo} from 'client';
import manifest from 'manifest';

// Mattermost mounts every reducer a plugin registers via registry.registerReducer
// under state['plugins-<pluginId>'] (NOT a root key of our choosing). Selectors must
// read from there or the UI never sees the state and stays "Off".
export const PLUGIN_REDUCER_KEY = 'plugins-' + manifest.id;

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

// Selectors read the plugin's slice from state['plugins-<pluginId>'], where MM
// mounts registered plugin reducers.
export type GlobalSlice = Record<string, unknown>;

// GlobalState is the part of the Mattermost webapp store this plugin touches:
// the current channel id (mattermost-redux) plus the plugins-<id> slice.
export interface GlobalState {
    entities: {channels: {currentChannelId: string}};
    [k: string]: unknown;
}
const slice = (state: GlobalSlice): DisappearState =>
    (state[PLUGIN_REDUCER_KEY] as DisappearState | undefined) ?? initialState;

export const selectChannelTTL = (state: GlobalSlice, channelId: string): TTLInfo | null | undefined =>
    slice(state).byChannel[channelId];

export const selectModalChannel = (state: GlobalSlice): string | null => slice(state).modalChannelId;
