import { expect, test, type APIRequestContext } from '@playwright/test';

// Runs against damson dev; the failing answers are mocked per request. See playwright.config.ts.

async function makeJobAndRun(request: APIRequestContext) {
	const created = await request.post('/api/v1/jobs', {
		data: {
			spec: {
				apiVersion: 'damson.dev/v1',
				kind: 'Job',
				metadata: { project: 'default', name: `errors-e2e-${Date.now()}` },
				spec: { steps: [{ id: 'only', plugin: 'ssh', with: { node: 'n', command: 'echo one' } }] }
			}
		}
	});
	expect(created.status()).toBe(201);
	const jobId = (await created.json()).job.id as string;
	const run = await request.post(`/api/v1/jobs/${jobId}/runs`, { data: {} });
	expect(run.status()).toBe(201);
	return { jobId, runId: (await run.json()).id as string };
}

test('/jobs/<id> shows an alert, not a blank page, for a 500 and for a dropped request', async ({
	page
}) => {
	const { jobId } = await makeJobAndRun(page.request);
	await page.route(`**/api/v1/jobs/${jobId}`, (r) =>
		r.fulfill({
			status: 500,
			contentType: 'application/problem+json',
			json: { title: 'Internal Server Error', status: 500 }
		})
	);
	await page.goto(`/jobs/${jobId}`);
	await expect(page.getByTestId('job-error')).toBeVisible();
	await expect(page.getByTestId('not-found')).toHaveCount(0);

	await page.unroute(`**/api/v1/jobs/${jobId}`);
	await page.route(`**/api/v1/jobs/${jobId}`, (r) => r.abort());
	await page.getByTestId('job-retry').click();
	await expect(page.getByTestId('job-error')).toBeVisible();

	// Retry runs the lookup again; once the server answers, the job opens
	await page.unroute(`**/api/v1/jobs/${jobId}`);
	await page.getByTestId('job-retry').click();
	await expect(page).toHaveURL(new RegExp(`/projects/default/-/jobs/${jobId}$`));
});

test('/jobs/<id> still says not found for a 404', async ({ page }) => {
	await page.goto('/jobs/00000000-0000-7000-8000-000000000000');
	await expect(page.getByTestId('not-found')).toBeVisible();
	await expect(page.getByTestId('job-error')).toHaveCount(0);
});

test('a run page shows the failed project check instead of the graph', async ({ page }) => {
	const { jobId, runId } = await makeJobAndRun(page.request);
	await page.route(`**/api/v1/jobs/${jobId}`, (r) =>
		r.fulfill({
			status: 500,
			contentType: 'application/problem+json',
			json: { title: 'Internal Server Error', status: 500 }
		})
	);
	await page.goto(`/projects/default/-/runs/${runId}`);
	await expect(page.getByTestId('run-error')).toBeVisible();
	await expect(page.getByTestId('instances-table')).toHaveCount(0);
});
