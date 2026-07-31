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

export interface PluginRegistry {
    registerChannelHeaderButtonAction(icon: ComponentType<unknown>, action: (...args: unknown[]) => void, dropdownText?: string): void;
    registerPostWillRenderHook(component: ComponentType<PostRenderArgs>, options?: {id?: string}): void;
    registerRootComponent(component: ComponentType<unknown>): void;
    registerWebSocketEventHandler(event: string, handler: (payload: WebSocketPayload) => void): void;
    registerReducer<S, A extends Action = Action>(reducer: Reducer<S, A>): void;
}
