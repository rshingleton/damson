import { api, type components } from '$lib/api/client';
import { isConflict, isNotFound, problemMessage } from '$lib/agents';

export { isConflict, isNotFound, problemMessage };

export type Job = components['schemas']['Job'];

/** One page of the jobs the caller can see, of one project and below when scope is set. */
export async function fetchScopedJobs(
	scope: string,
	cursor = '',
	signal?: AbortSignal
): Promise<{ items: Job[]; next: string }> {
	const { data, error } = await api.GET('/tree/jobs', {
		params: { query: { scope: scope || undefined, cursor: cursor || undefined } },
		signal
	});
	if (error !== undefined || !data) throw error ?? new Error('Empty response');
	return { items: data.items ?? [], next: data.next_cursor ?? '' };
}

/**
 * The project path of a job, or null on a 404 (missing, or no role). Any other failure,
 * a 5xx or a network error, throws, so a caller never reads it as "not found".
 */
export async function fetchJobProject(id: string, signal?: AbortSignal): Promise<string | null> {
	const { data, error } = await api.GET('/jobs/{id}', { params: { path: { id } }, signal });
	if (data) return data.job.project;
	if (error !== undefined && !isNotFound(error)) throw error;
	return null;
}

/** A job and the graph of its current version. */
export async function fetchJobGraph(id: string, signal?: AbortSignal) {
	const detail = await api.GET('/jobs/{id}', { params: { path: { id } }, signal });
	if (detail.error !== undefined || !detail.data) throw detail.error ?? new Error('Empty response');
	const job = detail.data.job;
	const g = await api.GET('/jobs/{id}/versions/{v}/graph', {
		params: { path: { id, v: job.current_version } },
		signal
	});
	if (g.error !== undefined || !g.data) throw g.error ?? new Error('Empty response');
	return { job, graph: g.data, spec: detail.data.version.spec as unknown };
}

export type JobVersionInfo = components['schemas']['JobVersion'];

// shortcut: at most 100 pages (20 000 versions) are followed, so a bad cursor cannot loop; raise it if jobs get longer histories.
const MAX_VERSION_PAGES = 100;

/** Every version of a job, ascending, without specs: follows next_cursor to the last page. */
export async function fetchVersions(
	jobId: string,
	signal?: AbortSignal
): Promise<JobVersionInfo[]> {
	const out: JobVersionInfo[] = [];
	let cursor: string | undefined;
	for (let page = 0; page < MAX_VERSION_PAGES; page++) {
		const { data, error } = await api.GET('/jobs/{id}/versions', {
			params: { path: { id: jobId }, query: { limit: 200, cursor } },
			signal
		});
		if (error !== undefined || !data) throw error ?? new Error('Empty response');
		out.push(...(data.items ?? []));
		if (!data.next_cursor) break;
		cursor = data.next_cursor;
	}
	return out;
}

/** One version's spec. */
export async function fetchVersionSpec(jobId: string, version: number, signal?: AbortSignal) {
	const { data, error } = await api.GET('/jobs/{id}/versions/{v}', {
		params: { path: { id: jobId, v: version } },
		signal
	});
	if (error !== undefined || !data) throw error ?? new Error('Empty response');
	return data.spec;
}
