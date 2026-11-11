import { expect, test, type Browser, type Page } from '@playwright/test';

// A project admin (dave, through the team-leads rule) edits node sets in the Access
// tab and attaches one to a rule; bob (runner) then sees only that set's nodes. The
// "ne-" prefix keeps the expectations exact.
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
	ok(await page.request.post('/api/v1/projects', { data: { path: 'ne-acme' }, headers: h }));
	for (const n of [
		{ name: 'ne-web-1', address: '10.2.0.1' },
		{ name: 'ne-web-2', address: '10.2.0.2' },
		{ name: 'ne-db-1', address: '10.2.0.3' }
	])
		ok(await page.request.post('/api/v1/nodes', { data: n, headers: h }));
	ok(
		await page.request.put('/api/v1/projects/ne-acme/access-rules', {
			data: {
				rules: [
					{ role: 'admin', rule: "'team-leads' in claims.groups" },
					{ role: 'runner', rule: "'team-ops' in claims.groups" }
				]
			},
			headers: h
		})
	);
});

test('an admin writes a set with the regex helper, previews it, and a bad expression is refused', async ({
	browser
}) => {
	const dave = await as(browser, 'dave-sub');
	await dave.goto('/projects/ne-acme/-/access');
	await expect(dave.getByTestId('nodeset-none')).toBeVisible();

	await dave.getByTestId('nodeset-new').click();
	await dave.getByTestId('nodeset-name').fill('prod-web');
	await dave.getByTestId('nodeset-regex').fill('^ne-web-');
	await dave.getByTestId('nodeset-regex-use').click();
	await expect(dave.getByTestId('nodeset-filter')).toHaveValue("node.name.matches('^ne-web-')");
	await expect(dave.getByTestId('nodeset-preview-count')).toHaveText('2');
	await expect(dave.getByTestId('nodeset-preview-names')).toHaveText('ne-web-1, ne-web-2');
	await dave.getByTestId('nodeset-save').click();
	await expect(dave.getByTestId('nodeset-row-ne-acme:prod-web')).toBeVisible();

	// A filter that does not compile is refused with its reason and nothing is saved.
	await dave.getByTestId('nodeset-new').click();
	await dave.getByTestId('nodeset-name').fill('broken');
	await dave.getByTestId('nodeset-filter').fill('node.tags.(');
	await dave.getByTestId('nodeset-preview').click();
	await expect(dave.getByTestId('nodeset-preview-error')).toBeVisible();
	await dave.getByTestId('nodeset-save').click();
	await expect(dave.getByTestId('nodeset-form-error')).toBeVisible();
	await expect(dave.getByTestId('nodeset-row-ne-acme:broken')).toHaveCount(0);
});

test('attaching the set to a rule limits what that rule grants', async ({ browser }) => {
	const dave = await as(browser, 'dave-sub');
	await dave.goto('/projects');
	await dave.getByTestId('edit-ne-acme').click();
	// The runner rule is the second one; an admin rule offers no sets.
	await expect(dave.getByTestId('rule-sets-0')).toHaveCount(0);
	await dave.getByTestId('rule-set-1-ne-acme:prod-web').click();
	await dave.getByTestId('save-rules').click();
	await expect(dave.getByTestId('saved')).toContainText('Access rules saved');

	await dave.goto('/projects/ne-acme/-/access');
	await expect(dave.getByTestId('access-rule-sets').nth(1)).toContainText('ne-acme:prod-web');

	const bob = await as(browser, 'bob-sub');
	await bob.goto('/projects/ne-acme/-/nodes');
	await expect(bob.getByTestId('node-name')).toHaveText(['ne-web-1', 'ne-web-2']);
	await expect(bob.locator('body')).not.toContainText('ne-db-1');

	// A set a rule uses cannot be deleted; the server says why.
	await dave.getByTestId('nodeset-delete-prod-web').click();
	await dave.getByTestId('nodeset-confirm-prod-web').click();
	await expect(dave.getByTestId('nodeset-error')).toContainText('access rules');
	await expect(dave.getByTestId('nodeset-row-ne-acme:prod-web')).toBeVisible();
});
