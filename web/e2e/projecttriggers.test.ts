import { expect, test, type Page } from '@playwright/test';

// The dev server has no login and project pages need a user, so the API is mocked here.

const schedule = (enabled: boolean) => ({
	id: 's1',
	job_id: 'j1',
	name: 'nightly',
	cron: '@daily',
	timezone: 'UTC',
	enabled,
	misfire: 'skip',
	options: {},
	created_at: '2026-01-01T00:00:00Z'
});

async function mockShell(page: Page) {
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
				my_role: 'admin',
				created_at: '2026-01-01T00:00:00Z',
				updated_at: '2026-01-01T00:00:00Z'
			}
		})
	);
	await page.route('**/api/v1/tree/jobs?**', (r) => r.fulfill({ json: { items: [] } }));
}

test('a refresh that was in flight during Pause cannot put the old list back', async ({ page }) => {
	await page.clock.install();
	await mockShell(page);
	let enabled = true;
	let gets = 0;
	let releasePoll: () => void = () => {};
	const held = new Promise<void>((res) => (releasePoll = res));
	await page.route('**/api/v1/schedules?**', async (r) => {
		const n = ++gets;
		const body = { items: [schedule(enabled)] }; // what the server says when it is asked
		if (n === 2) await held; // the 15 s refresh, answered late
		return r.fulfill({ json: body });
	});
	await page.route('**/api/v1/schedules/s1', (r) => {
		enabled = false;
		return r.fulfill({ json: schedule(false) });
	});
	await page.goto('/projects/mock/-/schedules');
	await expect(page.getByTestId('schedule-state')).toHaveText('enabled');
	await page.clock.fastForward(16_000);
	await expect.poll(() => gets).toBe(2);
	await page.getByTestId('schedule-toggle').click();
	await expect(page.getByTestId('schedule-state')).toHaveText('paused');
	releasePoll();
	await page.waitForTimeout(300);
	await expect(page.getByTestId('schedule-state')).toHaveText('paused');
});

test('a failed load shows the error, not "No schedules."', async ({ page }) => {
	await mockShell(page);
	await page.route('**/api/v1/schedules?**', (r) =>
		r.fulfill({
			status: 500,
			contentType: 'application/problem+json',
			json: { title: 'Internal Server Error', status: 500 }
		})
	);
	await page.goto('/projects/mock/-/schedules');
	await expect(page.getByTestId('triggers-error')).toBeVisible();
	await expect(page.getByText('No schedules.')).toHaveCount(0);
});
