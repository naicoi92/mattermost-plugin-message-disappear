// Minimal registry/channel types for the Mattermost webapp plugin runtime.
// The full @mattermost/types surface is version-coupled; we vendor the subset
// this plugin uses so the bundle stays self-contained.
import type {ComponentType} from 'react';
import type {Action, Reducer} from 'redux';

export interface Channel {
    id: string;
    type: string;
    display_name?: string;
}

export interface TTLInfoDTO {
    duration: number; // seconds
    set_by: string;
    set_at: number;
}

export interface PostRenderArgs {
    post: {id: string; channel_id: string};
    channel?: Channel;
}

export interface WebSocketPayload {
    channel_id?: string;
    ttl?: TTLInfoDTO | null;
}

export type ReactResolvable = string | ComponentType<unknown>;

export interface PluginRegistry {
    // Mattermost 11.5+: renders a component in the LEFT icon section of the channel
    // header (next to the pinned-posts button) as a full component. Optional because it
    // is absent in 10.x–11.4; gate usage with `typeof ... === 'function'`.
    registerChannelHeaderIcon?(component: ComponentType<unknown>): void;
    registerChannelHeaderButtonAction(
        icon: ComponentType<unknown>,
        action: () => void,
        dropdownText: string,
        tooltipText: string,
    ): void;
    // Adds an item to the channel-header dropdown menu; action receives the channel id.
    registerChannelHeaderMenuAction(
        text: ReactResolvable,
        action: (channelID: string) => void,
        shouldRender?: (state: Record<string, unknown>) => boolean,
    ): void;
    // Renders a component at the bottom of every post's message; the component
    // receives `postId` (not the full post). Return null when there's nothing to show.
    registerPostMessageAttachmentComponent(component: ComponentType<{postId?: string}>): void;
    registerRootComponent(component: ComponentType<unknown>): void;
    registerWebSocketEventHandler(event: string, handler: (payload: WebSocketPayload) => void): void;
    registerReducer<S, A extends Action = Action>(reducer: Reducer<S, A>): void;
}
