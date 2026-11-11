import { expect, test, type Page } from '@playwright/test';

// The dev server has no login, and project writes need a user, so the API is mocked here.

function project(version: number, description: string) {
	return {
		id: 'p1',
		path: 'mock',
		description,
		version,
		managed_by: 'ui',
		my_role: 'admin',
		created_at: '2026-01-01T00:00:00Z',
		updated_at: '2026-01-01T00:00:00Z'
	};
}

async function mockProject(page: Page, saves: number[]) {
	let version = 1;
	let description = 'one';
	await page.route('**/api/v1/me', (r) =>
		r.fulfill({
			json: { kind: 'user', instance_admin: true, csrf_token: 'c', actor: 'user:t' }
		})
	);
	await page.route('**/api/v1/projects', (r) =>
		r.fulfill({ json: { items: [project(version, description)] } })
	);
	await page.route('**/api/v1/projects/mock', async (r) => {
		if (r.request().method() === 'GET') return r.fulfill({ json: project(version, description) });
		const body = r.request().postDataJSON() as { description: string; if_version: number };
		saves.push(body.if_version);
		if (body.if_version !== version)
			return r.fulfill({
				status: 409,
				contentType: 'application/problem+json',
				json: { title: 'Conflict', status: 409, detail: 'version moved' }
			});
		version++;
		description = body.description;
		return r.fulfill({ json: project(version, description) });
	});
	// someone else saves after the page loaded
	return () => {
		version++;
		description = 'other';
	};
}

test('a description save that lost to another save succeeds on retry', async ({ page }) => {
	const saves: number[] = [];
	const otherSaves = await mockProject(page, saves);
	await page.goto('/projects/mock/-/settings');
	await expect(page.getByTestId('settings-description')).toHaveValue('one');
	otherSaves();
	await page.getByTestId('settings-description').fill('mine');
	await page.getByTestId('settings-save').click();
	await expect(page.getByTestId('settings-error')).toBeVisible();
	await page.getByTestId('settings-save').click();
	await expect(page.getByTestId('settings-saved')).toBeVisible();
	expect(saves).toEqual([1, 2]);
});
