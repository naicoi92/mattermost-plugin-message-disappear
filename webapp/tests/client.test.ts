import {clearTTL, getTTL, setTTL} from 'client';

// jest.fn standing in for global fetch (test seam).
const fetchMock = jest.fn();
const globalWithFetch = globalThis as unknown as {fetch: jest.Mock};

const okResponse = (body: unknown): Response => ({ok: true, json: async () => body}) as unknown as Response;
const badResponse = (status: number): Response => ({ok: false, status, json: async () => ({})}) as unknown as Response;

beforeEach(() => {
    fetchMock.mockReset();
    globalWithFetch.fetch = fetchMock;
});

it('getTTL builds the URL, encodes the id and parses ttl', async () => {
    fetchMock.mockResolvedValue(okResponse({ttl: {duration: 300, set_by: 'u1', set_at: 7}}));
    const t = await getTTL('ch 1');
    expect(fetchMock).toHaveBeenCalledWith(
        '/plugins/com.github.naicoi92.disappearing-messages/ttl/ch%201',
        expect.objectContaining({headers: expect.any(Object)}),
    );
    expect(t?.duration).toBe(300);
});

it('getTTL returns null when ttl is absent (default OFF)', async () => {
    fetchMock.mockResolvedValue(okResponse({ttl: null}));
    expect(await getTTL('ch1')).toBeNull();
});

it('getTTL rejects on a non-ok response', async () => {
    fetchMock.mockResolvedValue(badResponse(403));
    await expect(getTTL('ch1')).rejects.toThrow('403');
});

it('setTTL POSTs channel_id + ttl_seconds', async () => {
    fetchMock.mockResolvedValue(okResponse({}));
    await setTTL('ch1', 3600);
    const [url, opts] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('/plugins/com.github.naicoi92.disappearing-messages/ttl');
    expect(opts.method).toBe('POST');
    expect(opts.body).toBe(JSON.stringify({channel_id: 'ch1', ttl_seconds: 3600}));
});

it('clearTTL DELETEs the channel TTL', async () => {
    fetchMock.mockResolvedValue(okResponse({}));
    await clearTTL('ch1');
    const [url, opts] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('/plugins/com.github.naicoi92.disappearing-messages/ttl/ch1');
    expect(opts.method).toBe('DELETE');
});
