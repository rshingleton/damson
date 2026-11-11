import { expect, test } from '@playwright/test';

// Runs against damson dev (controller + one agent, SQLite); see playwright.config.ts.

async function makeJob(request: import('@playwright/test').APIRequestContext) {
	const name = `editor-e2e-${Date.now()}`;
	const created = await request.post('/api/v1/jobs', {
		data: {
			spec: {
				apiVersion: 'damson.dev/v1',
				kind: 'Job',
				metadata: { project: 'default', name },
				spec: {
					steps: [
						{ id: 'only', plugin: 'ssh', with: { node: 'n', command: 'echo one' } },
						{
							id: 'grp',
							type: 'group',
							steps: [{ id: 'inner', plugin: 'ssh', with: { node: 'n' } }]
						}
					]
				}
			}
		}
	});
	expect(created.status()).toBe(201);
	return (await created.json()).job.id as string;
}

test('a step is edited on the Steps tab and saved as a new version', async ({ page }) => {
	const id = await makeJob(page.request);
	await page.goto(`/jobs/${id}/edit`);
	await page.getByTestId('spec-tab-steps').click();
	await page.getByTestId('step-with-0').fill('{"node": "n", "command": "echo edited"}');
	await page.getByTestId('step-with-0').blur();
	// the group is kept, and shown whole
	await expect(page.getByTestId('step-whole-1')).toHaveValue(/inner/);
	await page.getByTestId('spec-tab-json').click();
	await expect(page.getByTestId('spec-text')).toHaveValue(/echo edited/);
	await expect(page.getByTestId('spec-text')).toHaveValue(/"inner"/);
	await page.getByTestId('spec-save').click();
	await expect(page).toHaveURL(new RegExp(`/projects/default/-/jobs/${id}$`));
	await page.getByTestId('job-tab-definition').click();
	await expect(page.getByTestId('version-row')).toHaveCount(2);
});

test('a step with a bad id shows its error count on the Steps tab', async ({ page }) => {
	const id = await makeJob(page.request);
	await page.goto(`/jobs/${id}/edit`);
	await page.getByTestId('spec-tab-steps').click();
	await page.getByTestId('step-id-0').fill('Bad_ID');
	await page.getByTestId('spec-validate').click();
	await expect(page.getByTestId('spec-tab-errors-steps')).toBeVisible();
	await expect(page.getByTestId('spec-issue').first()).toContainText('spec.steps[0]');
	// the text is untouched by the failed check
	await page.getByTestId('spec-tab-json').click();
	await expect(page.getByTestId('spec-text')).toHaveValue(/Bad_ID/);
});

test('the Details tab renames the job and Options adds an input', async ({ page }) => {
	const id = await makeJob(page.request);
	await page.goto(`/jobs/${id}/edit`);
	await page.getByTestId('detail-description').fill('hello');
	await page.getByTestId('spec-tab-options').click();
	await page.getByTestId('option-add').click();
	await expect(page.getByTestId('option-row')).toHaveCount(1);
	await page.getByTestId('spec-tab-json').click();
	await expect(page.getByTestId('spec-text')).toHaveValue(/"description": "hello"/);
	await expect(page.getByTestId('spec-text')).toHaveValue(/"option1"/);
});

test('creating a job lands on its project-first URL', async ({ page }) => {
	await page.goto('/jobs/new/default');
	await page.getByTestId('spec-tab-json').click();
	const spec = JSON.parse(await page.getByTestId('spec-text').inputValue());
	spec.metadata.name = `created-e2e-${Date.now()}`;
	await page.getByTestId('spec-text').fill(JSON.stringify(spec));
	await page.getByTestId('spec-save').click();
	await expect(page).toHaveURL(/\/projects\/default\/-\/jobs\/[^/]+$/);
});

test('a run in the Runs tab opens under its project', async ({ page }) => {
	const id = await makeJob(page.request);
	const run = await page.request.post(`/api/v1/jobs/${id}/runs`, { data: {} });
	expect(run.status()).toBe(201);
	const runId = (await run.json()).id as string;
	await page.goto(`/projects/default/-/jobs/${id}/runs`);
	await page.getByTestId('run-row').first().getByRole('link').first().click();
	await expect(page).toHaveURL(new RegExp(`/projects/default/-/runs/${runId}$`));
});

test('a save that lost to a newer version offers Reload latest, then succeeds', async ({
	page
}) => {
	const id = await makeJob(page.request);
	await page.goto(`/jobs/${id}/edit`);
	await expect(page.getByText('based on version 1')).toBeVisible();
	// another editor lands version 2 first
	const detail = await (await page.request.get(`/api/v1/jobs/${id}`)).json();
	const next = await page.request.post(`/api/v1/jobs/${id}/versions`, {
		data: { spec: detail.version.spec, expected_version: 1 }
	});
	expect(next.status()).toBe(201);
	await page.getByTestId('spec-save').click();
	await expect(page.getByTestId('editor-stale')).toBeVisible();
	await page.getByTestId('editor-reload').click();
	await expect(page.getByTestId('editor-stale')).toBeHidden();
	await expect(page.getByText('based on version 2')).toBeVisible();
	await page.getByTestId('spec-save').click();
	await expect(page).toHaveURL(new RegExp(`/projects/default/-/jobs/${id}$`));
});

test('editing the spec clears the issues of the last check', async ({ page }) => {
	const id = await makeJob(page.request);
	await page.goto(`/jobs/${id}/edit`);
	await page.getByTestId('spec-tab-steps').click();
	await page.getByTestId('step-id-0').fill('Bad_ID');
	await page.getByTestId('spec-validate').click();
	await expect(page.getByTestId('spec-issues')).toBeVisible();
	await page.getByTestId('step-id-0').fill('good');
	await page.getByTestId('step-id-0').blur();
	await expect(page.getByTestId('spec-issues')).toBeHidden();
});
