import type {ComponentType} from 'react';
import type {Store} from 'redux';

import TTLBadge from 'components/ttl_badge';
import TTLSelectorModal from 'components/ttl_selector';
import manifest from 'manifest';
import reducer, {DisappearAction, GlobalSlice, openModal, setChannelTTL} from 'reducer';
import type {PluginRegistry, WebSocketPayload} from 'types/mattermost-webapp';

import './styles.css';

// Channel-header icon (⏱). A named component so the registry holds a stable ref.
function ClockIcon() {
    return <span aria-hidden='true'>{'\u23F1'}</span>;
}

// extractChannelId accepts either a channel id (string) or a channel object,
// since the header-button action argument shape varies across MM versions.
function extractChannelId(arg: unknown): string | null {
    if (typeof arg === 'string') {
        return arg;
    }
    if (arg && typeof arg === 'object' && 'id' in arg) {
        const candidate = arg.id; // unknown after 'id' narrowing; no cast needed
        return typeof candidate === 'string' ? candidate : null;
    }
    return null;
}

export default class DisappearingMessagesPlugin {
    public async initialize(registry: PluginRegistry, store: Store<GlobalSlice>) {
        // redux v5's Dispatch<union> overload does not resolve for union action
        // types, so narrow dispatch to take our actions directly.
        const dispatch = store.dispatch as (action: DisappearAction) => void;

        registry.registerReducer(reducer);
        registry.registerRootComponent(TTLSelectorModal as ComponentType<unknown>);
        registry.registerPostWillRenderHook(TTLBadge);
        registry.registerChannelHeaderButtonAction(ClockIcon as ComponentType<unknown>, (...args) => {
            const channelId = extractChannelId(args[0]);
            if (channelId) {
                dispatch(openModal(channelId));
            }
        });
        registry.registerWebSocketEventHandler('ttl_changed', (payload: WebSocketPayload) => {
            if (payload.channel_id) {
                dispatch(setChannelTTL(payload.channel_id, payload.ttl ?? null));
            }
        });
    }
}

declare global {
    interface Window {
        registerPlugin(pluginId: string, plugin: unknown): void;
    }
}

window.registerPlugin(manifest.id, new DisappearingMessagesPlugin());
