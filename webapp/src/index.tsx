import type {ComponentType} from 'react';
import type {Store} from 'redux';

import ChannelHeaderButton from 'components/channel_header_button';
import DisappearingHeaderIcon from 'components/header_icon';
import TTLBadge from 'components/ttl_badge';
import TTLSelectorModal from 'components/ttl_selector';
import {clearTTL, setTTL} from 'client';
import manifest from 'manifest';
import {PRESETS} from 'presets';
import reducer, {DisappearAction, GlobalState, openModal, setChannelTTL} from 'reducer';
import type {PluginRegistry, WebSocketPayload} from 'types/mattermost-webapp';

import './styles.css';

export default class DisappearingMessagesPlugin {
    public async initialize(registry: PluginRegistry, store: Store<GlobalState>) {
        // redux v5's Dispatch<union> overload does not resolve for union action
        // types, so narrow dispatch to take our actions directly.
        const dispatch = store.dispatch as (action: DisappearAction) => void;
        const currentChannelId = () => store.getState().entities.channels.currentChannelId;

        registry.registerReducer(reducer);
        registry.registerRootComponent(TTLSelectorModal as ComponentType<unknown>);
        registry.registerPostMessageAttachmentComponent(TTLBadge);

        // Channel header: prefer registerChannelHeaderIcon (Mattermost 11.5+) — it
        // renders a full status component (⏱ + duration, clickable) in the LEFT icon
        // section next to the pinned-posts button. Fall back to
        // registerChannelHeaderButtonAction (10.x–11.4, icon-only button in the right
        // slot) so the plugin never crashes on an older supported server.
        if (typeof registry.registerChannelHeaderIcon === 'function') {
            registry.registerChannelHeaderIcon(ChannelHeaderButton as ComponentType<unknown>);
        } else {
            registry.registerChannelHeaderButtonAction(
                DisappearingHeaderIcon as ComponentType<unknown>,
                () => dispatch(openModal(currentChannelId())),
                'Disappearing',
                'Disappearing Messages',
            );
        }

        // Channel-header menu: quick-select TTL presets + Off + Custom (opens modal).
        for (const p of PRESETS) {
            registry.registerChannelHeaderMenuAction(`Disappearing: ${p.label}`, (channelID) => {
                setTTL(channelID, p.seconds).catch(() => {
                    // failure logged server-side; ttl_changed WS keeps the store honest
                });
            });
        }
        registry.registerChannelHeaderMenuAction('Disappearing: Off', (channelID) => {
            clearTTL(channelID).catch(() => {});
        });
        registry.registerChannelHeaderMenuAction('Disappearing: Custom\u2026', (channelID) => {
            dispatch(openModal(channelID));
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
