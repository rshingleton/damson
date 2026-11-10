import { describe, expect, it } from 'vitest';
import { filterNodes, type ProjectNode } from './projectnodes';

const node = (name: string, over: Partial<ProjectNode> = {}): ProjectNode => ({
	id: name,
	name,
	address: '10.0.0.1',
	attributes: {},
	enabled: true,
	os: 'linux',
	pool: 'default',
	tags: ['web'],
	transport: 'ssh',
	visible_through: [],
	...over
});

describe('filterNodes', () => {
	const nodes = [
		node('web-1'),
		node('db-1', { tags: ['db'], os: 'windows' }),
		node('cache', { tags: null, address: '10.0.9.9' })
	];

	it('an empty query keeps every node', () => {
		expect(filterNodes(nodes, '  ').items).toHaveLength(3);
	});

	it.each([
		['web', ['web-1']],
		['WINDOWS', ['db-1']],
		['10.0.9', ['cache']],
		['/^db-/', ['db-1']],
		['/(web|db)-1$/', ['web-1', 'db-1']],
		['nothing', []]
	])('%s matches %j', (q, want) => {
		expect(filterNodes(nodes, q).items.map((n) => n.name)).toEqual(want);
	});

	it('reports an invalid expression and matches nothing', () => {
		const r = filterNodes(nodes, '/(/');
		expect(r.error).not.toBeNull();
		expect(r.items).toEqual([]);
	});
});
