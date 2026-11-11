import { api, type components } from '$lib/api/client';

export type Agent = components['schemas']['Agent'];
export type Pool = components['schemas']['Pool'];

/** Pages fetched per refresh and rows rendered per table are bounded. */
const PAGE_SIZE = 200;
export const MAX_ROWS = 200;

export type Telemetry = NonNullable<Agent['telemetry']>;

/** Error text from a problem+json body, a plain-text body or a network failure. */
export function problemMessage(err: unknown): string {
	if (typeof err === 'string' && err.trim()) return err.trim().slice(0, 300);
	if (err && typeof err === 'object') {
		const p = err as { title?: string; detail?: string };
		if (p.detail) return p.title ? `${p.title}: ${p.detail}` : p.detail;
		if (p.title) return p.title;
	}
	return err instanceof Error ? err.message : 'Request failed';
}

/** True for a problem+json answer with status 404: missing, or no role (the server says the same for both). */
export function isNotFound(err: unknown): boolean {
	return typeof err === 'object' && err !== null && (err as { status?: number }).status === 404;
}

/** True for a problem+json answer with status 409: the write was based on a stale version. */
export function isConflict(err: unknown): boolean {
	return typeof err === 'object' && err !== null && (err as { status?: number }).status === 409;
}

export interface Listing<T> {
	items: T[];
	/** True when the server has more items than were fetched. */
	more: boolean;
	/** Server time minus browser time, in ms, from the response Date header; 0 when absent. */
	clockOffset: number;
}

interface Page<T> {
	data?: { items: T[] | null; next_cursor?: string };
	error?: unknown;
	response?: { headers: { get(name: string): string | null } };
}

/** Server clock minus local clock, from a Date header read at `receivedAt`; 0 when unusable. */
export function clockOffset(
	dateHeader: string | null | undefined,
	receivedAt = Date.now()
): number {
	const server = dateHeader ? Date.parse(dateHeader) : NaN;
	return Number.isNaN(server) ? 0 : server - receivedAt;
}

/** Follows next_cursor until MAX_ROWS items are collected. */
async function paginate<T>(fetchPage: (cursor?: string) => Promise<Page<T>>): Promise<Listing<T>> {
	const items: T[] = [];
	let cursor: string | undefined;
	for (;;) {
		const { data, error, response } = await fetchPage(cursor);
		if (error !== undefined || !data) throw error ?? new Error('Empty response');
		const offset = clockOffset(response?.headers.get('Date'));
		items.push(...(data.items ?? []));
		if (!data.next_cursor) return { items, more: false, clockOffset: offset };
		if (items.length >= MAX_ROWS)
			return { items: items.slice(0, MAX_ROWS), more: true, clockOffset: offset };
		cursor = data.next_cursor;
	}
}

export const fetchAgents = (signal?: AbortSignal) =>
	paginate<Agent>((cursor) =>
		api.GET('/agents', { params: { query: { limit: PAGE_SIZE, cursor } }, signal })
	);

export const fetchPools = (signal?: AbortSignal) =>
	paginate<Pool>((cursor) =>
		api.GET('/pools', { params: { query: { limit: PAGE_SIZE, cursor } }, signal })
	);

export function formatBytes(n: number | undefined): string {
	if (n === undefined) return '-';
	const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
	let i = 0;
	while (n >= 1024 && i < units.length - 1) {
		n /= 1024;
		i++;
	}
	return `${n.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

/**
 * An agent is stale when it claims to be connected but has not been heard from for at
 * least STALE_AFTER_MS (the server's window). `now` is server time: local time plus clockOffset.
 */
export const STALE_AFTER_MS = 40_000;

export function agentHealth(
	status: string,
	lastSeen: string | undefined,
	now = Date.now()
): 'healthy' | 'stale' | 'offline' {
	if (status !== 'connected') return 'offline';
	const seen = lastSeen ? Date.parse(lastSeen) : NaN;
	return Number.isNaN(seen) || now - seen >= STALE_AFTER_MS ? 'stale' : 'healthy';
}
