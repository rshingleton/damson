<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { compareOutcome, lineDiff, specText, type DiffLine } from '$lib/diff';
	import { fetchVersions, fetchVersionSpec, problemMessage, type JobVersionInfo } from '$lib/jobs';

	/** The versions of a job and a line diff between any two of them. */
	let { jobId, current }: { jobId: string; current: number } = $props();

	let versions = $state<JobVersionInfo[]>([]);
	let from = $state(0);
	let to = $state(0);
	let diff = $state<DiffLine[] | null>(null);
	let tooBig = $state(false);
	let outcome = $state<'changed' | 'identical' | 'formatting'>('changed');
	let compared = $state<{ from: number; to: number } | null>(null);
	let error = $state<string | null>(null);
	let busy = $state(false);

	$effect(() => {
		const ctl = new AbortController();
		fetchVersions(jobId, ctl.signal)
			.then((v) => {
				versions = v;
				to = current;
				from = v.length > 1 ? (v.at(-2)?.version ?? current) : current;
			})
			.catch((e) => {
				if (!ctl.signal.aborted) error = problemMessage(e);
			});
		return () => ctl.abort();
	});

	async function compare() {
		busy = true;
		error = null;
		// A failed compare must not leave the previous diff on screen under the new selection.
		diff = null;
		tooBig = false;
		compared = null;
		try {
			const [a, b] = await Promise.all([
				fetchVersionSpec(jobId, from),
				fetchVersionSpec(jobId, to)
			]);
			const d = lineDiff(specText(a), specText(b));
			tooBig = d === null;
			diff = d;
			compared = { from, to };
			if (d) {
				const sum = (n: number) => versions.find((v) => v.version === n)?.checksum ?? '';
				outcome = compareOutcome(d, sum(from), sum(to));
			}
		} catch (e) {
			error = problemMessage(e);
		} finally {
			busy = false;
		}
	}

	const when = (t: string) => new Date(t).toLocaleString();
</script>

<section class="space-y-3" aria-label="Versions" data-testid="versions">
	<h2 class="text-lg font-medium">Versions</h2>
	{#if error}<p class="text-sm text-destructive" role="alert">{error}</p>{/if}
	<table class="w-full text-left text-sm">
		<thead class="text-muted-foreground">
			<tr><th>Version</th><th>Checksum</th><th>Created</th></tr>
		</thead>
		<tbody>
			{#each [...versions].reverse() as v (v.version)}
				<tr data-testid="version-row" data-version={v.version}>
					<td>{v.version}{v.version === current ? ' (current)' : ''}</td>
					<td class="font-mono text-xs">{v.checksum.slice(0, 12)}</td>
					<td>{when(v.created_at)}</td>
				</tr>
			{/each}
		</tbody>
	</table>
	{#if versions.length > 1}
		<div class="flex flex-wrap items-center gap-2 text-sm">
			<label for="diff-from">Compare</label>
			<select
				id="diff-from"
				class="rounded-md border bg-background px-2 py-1"
				bind:value={from}
				data-testid="diff-from"
			>
				{#each versions as v (v.version)}<option value={v.version}>{v.version}</option>{/each}
			</select>
			<label for="diff-to">with</label>
			<select
				id="diff-to"
				class="rounded-md border bg-background px-2 py-1"
				bind:value={to}
				data-testid="diff-to"
			>
				{#each versions as v (v.version)}<option value={v.version}>{v.version}</option>{/each}
			</select>
			<Button
				size="sm"
				variant="outline"
				disabled={busy || from === to}
				onclick={compare}
				data-testid="diff-show">Show changes</Button
			>
		</div>
	{/if}
	{#if tooBig}
		<p class="text-sm text-muted-foreground" data-testid="diff-too-big">
			These specs are too long to compare here.
		</p>
	{:else if diff && compared}
		{#if outcome === 'identical'}
			<p class="text-sm text-muted-foreground" data-testid="diff-same">
				v{compared.from} and v{compared.to} have identical content.
			</p>
		{:else if outcome === 'formatting'}
			<p class="text-sm text-muted-foreground" data-testid="diff-same">
				No content change from v{compared.from} to v{compared.to}; formatting differs.
			</p>
		{:else}
			<p class="text-sm" data-testid="diff-caption">
				Changes from v{compared.from} to v{compared.to}
			</p>
			<!-- A scrollable region must take focus to scroll by keyboard. -->
			<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
			<pre
				class="max-h-96 overflow-auto rounded-md border bg-muted/30 p-2 text-xs"
				tabindex="0"
				aria-label={`Changes from v${compared.from} to v${compared.to}`}
				data-testid="diff">{#each diff as l, i (i)}<span
						class={l.kind === 'added'
							? 'block bg-green-100 text-green-900 dark:bg-green-950 dark:text-green-200'
							: l.kind === 'removed'
								? 'block bg-red-100 text-red-900 dark:bg-red-950 dark:text-red-200'
								: 'block text-muted-foreground'}
						data-testid="diff-line"
						data-kind={l.kind}
						>{l.kind === 'added' ? '+ ' : l.kind === 'removed' ? '- ' : '  '}{l.text}</span
					>{/each}</pre>
		{/if}
	{/if}
</section>
