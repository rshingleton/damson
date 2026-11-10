import { api, type components } from '$lib/api/client';
import { projectMatcher } from '$lib/projectsearch';

export type ProjectNode = components['schemas']['ProjectNode'];

export interface NodePage {
	/** all: every node within the project's scope; sets: only the nodes of the caller's node sets. */
	access: 'all' | 'sets';
	items: ProjectNode[];
	next: string;
}

/** One page of the nodes the caller may see in a project, after the cursor. */
export async function fetchProjectNodes(path: string, cursor = ''): Promise<NodePage> {
	const res = await api.GET('/projects/{path}/nodes', {
		params: { path: { path }, query: { cursor: cursor || undefined, limit: 100 } }
	});
	if (res.data === undefined) throw res.error ?? new Error('Request failed');
	return {
		access: res.data.access === 'sets' ? 'sets' : 'all',
		items: res.data.items ?? [],
		next: res.data.next_cursor ?? ''
	};
}

/** What a filter box searches: the name, address, pool, os, transport and the tags of a node. */
export function nodeText(n: ProjectNode): string[] {
	return [n.name, n.address, n.pool, n.os, n.transport, ...(n.tags ?? [])];
}

/**
 * Filters nodes by a plain text or a /regex/ (as projectMatcher reads it) matched
 * against any of the node's searched fields. An invalid expression is reported
 * and matches nothing.
 */
export function filterNodes(
	nodes: ProjectNode[],
	query: string
): { items: ProjectNode[]; error: string | null } {
	const m = projectMatcher(query);
	if (m.empty) return { items: nodes, error: null };
	return { items: nodes.filter((n) => nodeText(n).some((t) => m.test(t))), error: m.error };
}
