import { expect, test } from '@playwright/test';

// Runs against damson dev (controller + one agent, SQLite); see playwright.config.ts.

test('agents page shows the dev agent connected', async ({ page }) => {
	const errors: string[] = [];
	page.on('console', (m) => m.type() === 'error' && errors.push(m.text()));
	await page.goto('/');
	await page.getByTestId('admin-gear').click();
	await expect(page).toHaveURL(/\/admin\/agents$/);
	await expect(page.getByRole('heading', { name: 'Agents', level: 1 })).toBeVisible();

	const row = page.getByTestId('agent-row');
	await expect(row).toHaveCount(1);
	await expect(row.getByText('connected', { exact: true })).toBeVisible();
	// The dev agent runs on this machine: its os label is the host's, not linux.
	const hostOS = process.platform === 'win32' ? 'windows' : process.platform;
	await expect(row.getByText(`os=${hostOS}`)).toBeVisible();
	await expect(page.getByTestId('pools-table').getByText('default')).toBeVisible();
	expect(errors).toEqual([]);
});

test('agents page shows a problem+json error', async ({ page }) => {
	await page.route('**/api/v1/pools*', (route) =>
		route.fulfill({
			status: 500,
			contentType: 'application/problem+json',
			body: JSON.stringify({
				type: 'about:blank',
				title: 'Internal Server Error',
				status: 500,
				detail: 'boom'
			})
		})
	);
	await page.goto('/agents');
	await expect(page.getByTestId('agents-error')).toContainText('boom');
});

test('a connected agent not heard from for a minute shows visible stale text', async ({ page }) => {
	await page.route('**/api/v1/agents*', (route) =>
		route.fulfill({
			status: 200,
			contentType: 'application/json',
			headers: { Date: new Date().toUTCString() },
			body: JSON.stringify({
				items: [
					{
						id: 'stale-agent',
						pool_id: 'default',
						status: 'connected',
						capacity: 1,
						labels: {},
						last_seen: new Date(Date.now() - 60_000).toISOString()
					}
				]
			})
		})
	);
	await page.goto('/admin/agents');
	await expect(page.getByTestId('agent-health')).toHaveText('stale');
	await expect(page.getByTestId('agent-stale-note')).toBeVisible();
});
