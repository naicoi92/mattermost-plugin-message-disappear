import manifest from 'manifest';
import type {TTLInfoDTO} from 'types/mattermost-webapp';

export type TTLInfo = TTLInfoDTO;

const base = `/plugins/${manifest.id}/ttl`;

const jsonHeaders = {
    'Content-Type': 'application/json',
    'X-Requested-With': 'XMLHttpRequest',
};

export async function getTTL(channelId: string): Promise<TTLInfo | null> {
    const r = await fetch(`${base}/${encodeURIComponent(channelId)}`, {headers: jsonHeaders});
    if (!r.ok) {
        throw new Error(`get TTL failed (${r.status})`);
    }
    const data = (await r.json()) as {ttl: TTLInfo | null};
    return data.ttl ?? null;
}

export async function setTTL(channelId: string, ttlSeconds: number): Promise<void> {
    const r = await fetch(base, {
        method: 'POST',
        headers: jsonHeaders,
        body: JSON.stringify({channel_id: channelId, ttl_seconds: ttlSeconds}),
    });
    if (!r.ok) {
        throw new Error(`set TTL failed (${r.status})`);
    }
}

export async function clearTTL(channelId: string): Promise<void> {
    const r = await fetch(`${base}/${encodeURIComponent(channelId)}`, {method: 'DELETE', headers: jsonHeaders});
    if (!r.ok) {
        throw new Error(`clear TTL failed (${r.status})`);
    }
}
