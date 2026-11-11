import { beforeEach, describe, expect, it, vi } from 'vitest';

const GET = vi.hoisted(() => vi.fn());
vi.mock('$lib/api/client', () => ({ api: { GET } }));

import { fetchVersions } from './jobs';

const versions = (from: number, to: number) =>
	Array.from({ length: to - from + 1 }, (_, i) => ({
		version: from + i,
		checksum: `c${from + i}`,
		created_at: ''
	}));

describe('fetchVersions', () => {
	beforeEach(() => GET.mockReset());

	it('follows next_cursor so the current version of a 250-version job is listed', async () => {
		GET.mockResolvedValueOnce({ data: { items: versions(1, 200), next_cursor: 'p2' } });
		GET.mockResolvedValueOnce({ data: { items: versions(201, 250) } });
		const all = await fetchVersions('job');
		expect(all).toHaveLength(250);
		expect(all.at(-1)?.version).toBe(250);
		expect(GET).toHaveBeenCalledTimes(2);
		expect(GET.mock.calls[1][1].params.query.cursor).toBe('p2');
	});

	it('throws the error of a failed page', async () => {
		GET.mockResolvedValue({ error: { status: 500 } });
		await expect(fetchVersions('job')).rejects.toEqual({ status: 500 });
	});
});
