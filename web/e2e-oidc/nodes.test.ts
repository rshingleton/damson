import { expect, test, type Browser, type Page } from '@playwright/test';

// The project Nodes tab as three users (see playwright.oidc.config.ts): bob (runner)
// sees only the node set his rule grants, carol (viewer) every node the project may
// use, frank has no role. The "pn-" prefix keeps the expectations exact.
const idp = 'http://127.0.0.1:4191';

async function as(browser: Browser, sub: string): Promise<Page> {
	const ctx = await browser.newContext();
	expect((await ctx.request.post(`${idp}/_login-as?sub=${sub}`)).status()).toBe(204);
	const page = await ctx.newPage();
	await page.goto('/');
	await expect(page.getByTestId('user-name')).toBeVisible();
	return page;
}

test.describe.configure({ mode: 'serial' });

test.beforeAll(async ({ browser }) => {
	const page = await as(browser, 'alice-sub');
	const me = await (await page.request.get('/api/v1/me')).json();
	const h = { 'X-CSRF-Token': me.csrf_token };
	const ok = (r: { status(): number }) => expect([200, 201, 409]).toContain(r.status());
	ok(await page.request.post('/api/v1/projects', { data: { path: 'pn-acme' }, headers: h }));
	for (const n of [
		{ name: 'pn-web-1', address: '10.1.0.1', tags: ['web'] },
		{ name: 'pn-web-2', address: '10.1.0.2', tags: ['web'] },
		{ name: 'pn-db-1', address: '10.1.0.3', tags: ['db'] }
	])
		ok(await page.request.post('/api/v1/nodes', { data: n, headers: h }));
	ok(
		await page.request.post('/api/v1/projects/pn-acme/node-sets', {
			data: { name: 'web', filter: "node.name.startsWith('pn-web')" },
			headers: h
		})
	);
	ok(
		await page.request.put('/api/v1/projects/pn-acme/access-rules', {
			data: {
				rules: [
					{ role: 'runner', rule: "'team-ops' in claims.groups", node_sets: ['pn-acme:web'] },
					{ role: 'viewer', rule: "'team-viewers' in claims.groups" }
				]
			},
			headers: h
		})
	);
});

test('a member limited to a node set sees only that set, and the set that grants each node', async ({
	browser
}) => {
	const bob = await as(browser, 'bob-sub');
	await bob.goto('/projects/pn-acme');
	await bob.getByTestId('nav-nodes').click();
	await expect(bob).toHaveURL(/\/projects\/pn-acme\/-\/nodes$/);
	await expect(bob.getByTestId('nodes-access')).toContainText('node sets');
	await expect(bob.getByTestId('node-name')).toHaveText(['pn-web-1', 'pn-web-2']);
	await expect(bob.getByTestId('node-through').first()).toContainText('pn-acme:web');
	await expect(bob.locator('body')).not.toContainText('pn-db-1');
	// The filter narrows what is loaded; a /regex/ works too.
	await bob.getByTestId('nodes-filter').fill('/-2$/');
	await expect(bob.getByTestId('node-name')).toHaveText(['pn-web-2']);
	await bob.getByTestId('nodes-filter').fill('zzz');
	await expect(bob.getByTestId('nodes-empty')).toContainText('No node matches');
});

test('a viewer whose rule has no sets sees every node the project may use', async ({ browser }) => {
	const carol = await as(browser, 'carol-sub');
	await carol.goto('/projects/pn-acme/-/nodes');
	await expect(carol.getByTestId('nodes-access')).toContainText('every node');
	await expect(carol.getByTestId('node-name')).toContainText(['pn-db-1', 'pn-web-1', 'pn-web-2']);
	await expect(carol.getByTestId('node-through').first()).toHaveText('all');
});

test('a user with no role has no Nodes tab and gets not-found on the URL', async ({ browser }) => {
	const frank = await as(browser, 'frank-sub');
	await frank.goto('/projects/pn-acme/-/nodes');
	await expect(frank.getByTestId('project-nodes')).toHaveCount(0);
	await expect(frank.getByTestId('nav-nodes')).toHaveCount(0);
	await expect(frank.locator('body')).toContainText(/not found|404/i);
});
