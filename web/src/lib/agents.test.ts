import { beforeEach, describe, expect, it, vi } from 'vitest';

const GET = vi.hoisted(() => vi.fn());
vi.mock('$lib/api/client', () => ({ api: { GET } }));

import { agentHealth, clockOffset, fetchAgents, STALE_AFTER_MS } from './agents';

const seen = '2026-01-01T00:00:00Z';
const t0 = Date.parse(seen);

describe('agentHealth', () => {
	it('is stale at exactly 40 s, healthy just under', () => {
		expect(STALE_AFTER_MS).toBe(40_000);
		expect(agentHealth('connected', seen, t0 + 39_999)).toBe('healthy');
		expect(agentHealth('connected', seen, t0 + 40_000)).toBe('stale');
	});

	it('is offline unless connected, stale without a last_seen', () => {
		expect(agentHealth('disconnected', seen, t0)).toBe('offline');
		expect(agentHealth('connected', undefined, t0)).toBe('stale');
	});
});

describe('clockOffset', () => {
	it('is server time minus local time', () => {
		const local = Date.parse('2026-01-01T00:10:00Z');
		expect(clockOffset('Thu, 01 Jan 2026 00:10:30 GMT', local)).toBe(30_000);
	});

	it('is 0 for a missing or bad header', () => {
		expect(clockOffset(null)).toBe(0);
		expect(clockOffset('nonsense')).toBe(0);
	});
});

describe('fetchAgents', () => {
	beforeEach(() => GET.mockReset());

	it('reports the offset of a browser clock running 2 minutes fast, so a fresh agent is not stale', async () => {
		const server = Date.now();
		const headers = new Headers({ Date: new Date(server).toUTCString() });
		GET.mockResolvedValue({ data: { items: [] }, response: { headers } });
		// The browser clock is 2 minutes ahead of the server.
		vi.spyOn(Date, 'now').mockReturnValue(server + 120_000);
		const l = await fetchAgents();
		vi.restoreAllMocks();
		// Date headers have 1 s resolution.
		expect(Math.abs(l.clockOffset + 120_000)).toBeLessThan(1000);
		const lastSeen = new Date(server - 5_000).toISOString();
		const localNow = server + 120_000;
		expect(agentHealth('connected', lastSeen, localNow)).toBe('stale');
		expect(agentHealth('connected', lastSeen, localNow + l.clockOffset)).toBe('healthy');
	});
});
