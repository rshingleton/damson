<script lang="ts">
	// The old job URL: it has no project, so it asks for the job's project and moves to the
	// project-first URL. A job the caller cannot read shows the same not-found view as a missing one.
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import NotFound from '$lib/components/NotFound.svelte';
	import { fetchJobProject, problemMessage } from '$lib/jobs';
	import { jobRest } from '$lib/jobroute';

	let missing = $state(false);
	let error = $state<string | null>(null);
	// Bumped by Retry to run the lookup again.
	let attempt = $state(0);

	$effect(() => {
		const id = page.params.id ?? '';
		void attempt;
		const ctl = new AbortController();
		missing = false;
		error = null;
		fetchJobProject(id, ctl.signal)
			.then((project) => {
				if (ctl.signal.aborted) return;
				if (project) {
					void goto(resolve('/projects/[...path]', { path: jobRest(project, id) }), {
						replaceState: true
					});
				} else missing = true;
			})
			.catch((e) => {
				if (!ctl.signal.aborted) error = problemMessage(e);
			});
		return () => ctl.abort();
	});
</script>

<main class="mx-auto max-w-6xl space-y-6 px-4 py-8">
	{#if missing}
		<NotFound what="This job" />
	{:else if error}
		<Alert.Root variant="destructive" data-testid="job-error">
			<Alert.Description class="flex items-center justify-between gap-3">
				<span>{error}</span>
				<Button variant="outline" size="sm" onclick={() => attempt++} data-testid="job-retry"
					>Retry</Button
				>
			</Alert.Description>
		</Alert.Root>
	{/if}
</main>
