import { expect, test } from '@playwright/test';

// The dev server has no login and the tree needs a user, so the API is mocked here.

const job = (name: string) => ({ id: `id-${name}`, name });

test('the 10 s refresh keeps the rows loaded by More', async ({ page }) => {
	await page.clock.install();
	await page.route('**/api/v1/me', (r) =>
		r.fulfill({ json: { kind: 'user', instance_admin: true, csrf_token: 'c', actor: 'user:t' } })
	);
	await page.route('**/api/v1/projects/mock', (r) =>
		r.fulfill({
			json: {
				id: 'p',
				path: 'mock',
				version: 1,
				managed_by: 'ui',
				my_role: 'viewer',
				created_at: '2026-01-01T00:00:00Z',
				updated_at: '2026-01-01T00:00:00Z'
			}
		})
	);
	let firstPages = 0;
	await page.route('**/api/v1/tree?**', (r) => {
		const second = new URL(r.request().url()).searchParams.get('jobs_cursor') === 'c1';
		if (!second) firstPages++;
		return r.fulfill({
			json: second
				? { projects: [], jobs: [job('beta')] }
				: { projects: [], jobs: [job('alpha')], jobs_next_cursor: 'c1' }
		});
	});
	await page.goto('/projects/mock');
	await expect(page.getByTestId('job-link-alpha')).toBeVisible();
	await page.getByTestId('jobs-more').click();
	await expect(page.getByTestId('job-link-beta')).toBeVisible();
	// the refresh answers with page 1 only; page 2 must still be listed
	await page.clock.fastForward(11_000);
	await expect.poll(() => firstPages).toBeGreaterThanOrEqual(2);
	await expect(page.getByTestId('job-link-alpha')).toBeVisible();
	await expect(page.getByTestId('job-link-beta')).toBeVisible();
	await expect(page.getByTestId('job-row')).toHaveCount(2);
});
