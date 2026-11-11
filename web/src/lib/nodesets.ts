import { api, type components } from '$lib/api/client';

export type NodeSet = components['schemas']['NodeSet'];
export type ProjectNode = components['schemas']['ProjectNode'];

function ok<T>(res: { data?: T; error?: unknown }): T {
	if (res.error !== undefined || res.data === undefined)
		throw res.error ?? new Error('Empty response');
	return res.data;
}

/** The node sets of a project and of its ancestors (the ones a rule here may name). */
export async function fetchNodeSets(path: string): Promise<NodeSet[]> {
	return (
		ok(await api.GET('/projects/{path}/node-sets', { params: { path: { path } } })).items ?? []
	);
}

export async function createNodeSet(
	path: string,
	body: { name: string; description: string; filter: string }
): Promise<NodeSet> {
	return ok(await api.POST('/projects/{path}/node-sets', { params: { path: { path } }, body }));
}

export async function updateNodeSet(
	path: string,
	name: string,
	body: { description: string; filter: string; if_version: number }
): Promise<NodeSet> {
	return ok(
		await api.PUT('/projects/{path}/node-sets/{name}', { params: { path: { path, name } }, body })
	);
}

export async function deleteNodeSet(path: string, name: string): Promise<void> {
	const res = await api.DELETE('/projects/{path}/node-sets/{name}', {
		params: { path: { path, name } }
	});
	if (res.error !== undefined) throw res.error;
}

/** The nodes a filter matches within the project's node scope (at most 200). */
export async function previewNodeSet(
	path: string,
	filter: string
): Promise<{ items: ProjectNode[]; truncated: boolean }> {
	const out = ok(
		await api.POST('/projects/{path}/node-sets/preview', {
			params: { path: { path } },
			body: { filter }
		})
	);
	return { items: out.items ?? [], truncated: out.truncated };
}

/** The node fields a regex can be matched against. */
export const REGEX_FIELDS = ['name', 'address', 'pool', 'os'] as const;
export type RegexField = (typeof REGEX_FIELDS)[number];

/** A CEL string literal for text: single-quoted, with backslashes and quotes escaped. */
export function celString(text: string): string {
	return `'${text.replace(/\\/g, '\\\\').replace(/'/g, "\\'").replace(/\n/g, '\\n')}'`;
}

/**
 * The CEL filter for "node.<field> matches this regular expression". The pattern
 * is not checked here: the server compiles the result and reports a problem.
 */
export function regexToCel(field: RegexField, pattern: string): string {
	return `node.${field}.matches(${celString(pattern)})`;
}

/** The messages of a 422 problem, one per invalid field, else the one message. */
export function fieldMessages(err: unknown, fallback: (e: unknown) => string): string[] {
	const errs = (err as { errors?: { message?: string }[] } | null)?.errors;
	if (Array.isArray(errs) && errs.length > 0)
		return errs.map((f) => f.message ?? '').filter(Boolean);
	return [fallback(err)];
}
