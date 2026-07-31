import {PRESETS, shortDuration} from 'presets';

describe('shortDuration', () => {
    it('matches known presets', () => {
        expect(shortDuration(300)).toBe('5m');
        expect(shortDuration(3600)).toBe('1h');
        expect(shortDuration(86400)).toBe('1d');
        expect(shortDuration(604800)).toBe('1w');
    });

    it('formats non-preset whole values', () => {
        expect(shortDuration(60)).toBe('1m');
        expect(shortDuration(7200)).toBe('2h');
        expect(shortDuration(172800)).toBe('2d');
    });

    it('falls back to seconds for non-round values', () => {
        expect(shortDuration(90)).toBe('90s');
        expect(shortDuration(3700)).toBe('3700s');
    });

    it('every preset has a distinct shortLabel', () => {
        const labels = PRESETS.map((p) => p.shortLabel);
        expect(new Set(labels).size).toBe(labels.length);
    });
});
