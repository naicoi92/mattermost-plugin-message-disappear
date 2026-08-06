// TTL presets shared by the webapp selector modal and the channel-header menu.
// Mirrors the server presets (server/ttl/ttl.go); "30s" is absent (server min is 1m).
export interface TTLPreset {
    label: string;
    shortLabel: string;
    seconds: number;
}

export const PRESETS: readonly TTLPreset[] = [
    {label: '5 minutes', shortLabel: '5m', seconds: 300},
    {label: '1 hour', shortLabel: '1h', seconds: 3600},
    {label: '8 hours', shortLabel: '8h', seconds: 28800},
    {label: '1 day', shortLabel: '1d', seconds: 86400},
    {label: '1 week', shortLabel: '1w', seconds: 604800},
    {label: '1 month', shortLabel: '1mo', seconds: 2592000},
];

// shortDuration renders a compact label for a TTL (seconds) for the header icon.
export function shortDuration(seconds: number): string {
    const match = PRESETS.find((p) => p.seconds === seconds);
    if (match) {
        return match.shortLabel;
    }
    if (seconds >= 86400 && seconds % 86400 === 0) {
        return `${seconds / 86400}d`;
    }
    if (seconds >= 3600 && seconds % 3600 === 0) {
        return `${seconds / 3600}h`;
    }
    if (seconds >= 60 && seconds % 60 === 0) {
        return `${seconds / 60}m`;
    }
    return `${seconds}s`;
}
