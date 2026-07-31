import type {ComponentType} from 'react';
import type {Store} from 'redux';

import ChannelHeaderButton from 'components/channel_header_button';
import DisappearingHeaderIcon from 'components/header_icon';
import TTLBadge from 'components/ttl_badge';
import TTLSelectorModal from 'components/ttl_selector';
import manifest from 'manifest';
import reducer, {DisappearAction, GlobalState, openModal, setChannelTTL} from 'reducer';
import type {PluginRegistry, WebSocketPayload} from 'types/mattermost-webapp';

import './styles.css';

export default class DisappearingMessagesPlugin {
    public async initialize(registry: PluginRegistry, store: Store<GlobalState>) {
        // redux v5's Dispatch<union> overload does not resolve for union action
        // types, so narrow dispatch to take our actions directly.
        const dispatch = store.dispatch as (action: DisappearAction) => void;

        registry.registerReducer(reducer);
        registry.registerRootComponent(TTLSelectorModal as ComponentType<unknown>);
        registry.registerPostMessageAttachmentComponent(TTLBadge);

        // Channel header: prefer registerChannelHeaderIcon (Mattermost 11.5+) — it
        // renders the dropdown button in the LEFT icon section next to the pinned-posts
        // button and passes the channel as a prop. Fall back to
        // registerChannelHeaderButtonAction (10.x–11.4, icon-only button in the right
        // slot); its action receives the channel too. Neither path reads
        // currentChannelId from redux (that sent an id the server rejected as
        // "channel not found").
        if (typeof registry.registerChannelHeaderIcon === 'function') {
            registry.registerChannelHeaderIcon(ChannelHeaderButton as ComponentType<unknown>);
        } else {
            registry.registerChannelHeaderButtonAction(
                DisappearingHeaderIcon as ComponentType<unknown>,
                (channel) => {
                    if (channel?.id) {
                        dispatch(openModal(channel.id));
                    }
                },
                'Disappearing',
                'Disappearing Messages',
            );
        }

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
