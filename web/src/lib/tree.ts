import { api, type components } from '$lib/api/client';

export type TreeProject = components['schemas']['TreeProject'];
export type TreeJob = components['schemas']['TreeJob'];
export type SharedJob = components['schemas']['SharedJob'];

/** One expanded level: the child projects and, below a project, its jobs. */
export interface TreeLevel {
	projects: TreeProject[];
	jobs: TreeJob[];
	/** Cursors for the next page of projects and of jobs; empty when there is none. */
	next: string;
	jobsNext: string;
}

/** Loads one level of the caller's tree: the roots when parent is empty. */
export async function fetchLevel(parent: string, cursor = '', jobsCursor = ''): Promise<TreeLevel> {
	const res = await api.GET('/tree', {
		params: {
			query: {
				parent: parent || undefined,
				cursor: cursor || undefined,
				jobs_cursor: jobsCursor || undefined
			}
		}
	});
	if (res.data === undefined) throw res.error ?? new Error('Request failed');
	return {
		projects: res.data.projects ?? [],
		jobs: res.data.jobs ?? [],
		next: res.data.next_cursor ?? '',
		jobsNext: res.data.jobs_next_cursor ?? ''
	};
}

/** Jobs the caller reaches only through a rule of the job itself. */
export async function fetchShared(): Promise<{ items: SharedJob[]; truncated: boolean }> {
	const res = await api.GET('/tree/shared');
	if (res.data === undefined) throw res.error ?? new Error('Request failed');
	return { items: res.data.items ?? [], truncated: res.data.truncated ?? false };
}
