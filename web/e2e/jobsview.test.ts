import { expect, test } from '@playwright/test';

// The token (no-login) project view lists jobs flat; the tree API is mocked here.

const job = (id: string, project: string) => ({
	id,
	name: id,
	project,
	current_version: 1,
	managed_by: 'ui',
	created_at: '2026-01-01T00:00:00Z',
	updated_at: '2026-01-01T00:00:00Z'
});

test('a 500 reads as a generic error, only a 404 as not found', async ({ page }) => {
	await page.route('**/api/v1/tree/jobs?**', (r) =>
		r.fulfill({
			status: 500,
			contentType: 'application/problem+json',
			json: { title: 'Internal Server Error', status: 500 }
		})
	);
	await page.goto('/projects/default');
	await expect(page.getByTestId('jobs-error')).toContainText('Internal Server Error');
	await expect(page.getByTestId('jobs-error')).not.toContainText('Not found');

	await page.unroute('**/api/v1/tree/jobs?**');
	await page.route('**/api/v1/tree/jobs?**', (r) =>
		r.fulfill({
			status: 404,
			contentType: 'application/problem+json',
			json: { title: 'Not Found', status: 404 }
		})
	);
	await page.goto('/projects/other');
	await expect(page.getByTestId('jobs-error')).toContainText(
		'Not found, or you may not have access'
	);
});

test('switching project before More returns does not add the old rows', async ({ page }) => {
	let release: () => void = () => {};
	const held = new Promise<void>((res) => (release = res));
	await page.route('**/api/v1/tree/jobs?**', async (r) => {
		const q = new URL(r.request().url()).searchParams;
		if (q.get('cursor') === 'n') {
			await held;
			return r.fulfill({ json: { items: [job('late-old', 'default/sub')] } });
		}
		if (q.get('scope') === 'default/sub')
			return r.fulfill({ json: { items: [job('sub-job', 'default/sub')] } });
		return r.fulfill({ json: { items: [job('first', 'default/sub')], next_cursor: 'n' } });
	});
	await page.goto('/projects/default');
	await expect(page.getByText('first')).toBeVisible();
	await page.getByTestId('jobs-more').click();
	await page.getByRole('link', { name: 'default/sub' }).click();
	await expect(page.getByText('sub-job')).toBeVisible();
	release();
	await page.waitForTimeout(300);
	await expect(page.getByText('late-old')).toHaveCount(0);
	await expect(page.getByTestId('job-row')).toHaveCount(1);
});
