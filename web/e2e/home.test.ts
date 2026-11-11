import { expect, test } from '@playwright/test';

test('home page renders the placeholder without console errors', async ({ page }) => {
	const errors: string[] = [];
	page.on('console', (m) => m.type() === 'error' && errors.push(m.text()));
	await page.goto('/');
	await expect(page.getByRole('heading', { name: 'Damson' })).toBeVisible();
	expect(errors).toEqual([]);
});

test('unknown routes fall back to the SPA shell', async ({ page }) => {
	await page.goto('/some/client/route');
	// The shell loads; the SvelteKit error page is rendered client-side.
	await expect(page.locator('body')).toContainText('404');
});

test('the project search error shows only while a query is typed', async ({ page }) => {
	await page.route('**/api/v1/me', (r) =>
		r.fulfill({ json: { kind: 'user', instance_admin: true, csrf_token: 'c', actor: 'user:t' } })
	);
	await page.route('**/api/v1/tree', (r) =>
		r.fulfill({ json: { projects: [{ path: 'mock', label: 'mock', my_role: 'admin' }], jobs: [] } })
	);
	await page.route('**/api/v1/tree/shared', (r) => r.fulfill({ json: { items: [] } }));
	await page.route('**/api/v1/projects', (r) =>
		r.fulfill({
			status: 500,
			contentType: 'application/problem+json',
			json: { title: 'Internal Server Error', status: 500 }
		})
	);
	await page.goto('/');
	await page.getByTestId('project-search').fill('mo');
	await expect(page.getByTestId('search-error')).toBeVisible();
	await page.getByTestId('project-search').fill('');
	await expect(page.getByTestId('search-error')).toHaveCount(0);
});
