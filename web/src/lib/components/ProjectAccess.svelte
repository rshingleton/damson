<script lang="ts">
	import * as Alert from '$lib/components/ui/alert';
	import * as Table from '$lib/components/ui/table';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import NodeSetEditor from '$lib/components/NodeSetEditor.svelte';
	import { checkAccess, describeSource, splitGroups, type AccessCheck } from '$lib/access';
	import {
		fetchAccess,
		fetchProject,
		isInstanceAdmin,
		problemMessage,
		type AccessRules
	} from '$lib/projects';

	/**
	 * The project's access rules, read-only, and, for an instance admin, the
	 * check of why an identity has or lacks a role here (the check route is
	 * instance-admin only).
	 */
	let { path }: { path: string } = $props();

	let rules = $state<AccessRules | null>(null);
	// A project that apply manages is read-only here: its source changes it.
	let managed = $state(false);
	let loaded = $state(false);
	let error = $state<string | null>(null);

	const canCheck = $derived(isInstanceAdmin());
	let groups = $state('');
	let email = $state('');
	let busy = $state(false);
	let result = $state<AccessCheck | null>(null);
	let checkError = $state<string | null>(null);

	$effect(() => {
		const current = path;
		loaded = false;
		error = null;
		result = null;
		managed = true; // fail closed until the project says it is the UI's
		void fetchProject(current).then(
			(p) => current === path && (managed = (p?.managed_by ?? 'ui') !== 'ui')
		);
		fetchAccess(current)
			.then((r) => current === path && (rules = r))
			.catch((e) => current === path && (error = problemMessage(e)))
			.finally(() => current === path && (loaded = true));
	});

	async function check(e: Event) {
		e.preventDefault();
		busy = true;
		checkError = null;
		try {
			result = await checkAccess({ groups: splitGroups(groups), email, project: path, jobId: '' });
		} catch (err) {
			result = null;
			checkError = problemMessage(err);
		} finally {
			busy = false;
		}
	}
</script>

<div class="space-y-8" data-testid="access">
	<h1 class="text-2xl font-semibold" data-testid="project-title">{path}</h1>

	<section class="space-y-3" aria-label="Rules">
		<h2 class="text-lg font-medium">Access rules</h2>
		<p class="text-sm text-muted-foreground">
			Rules of this project grant a role to whoever matches; roles pass down to the projects below.
		</p>
		{#if error}
			<Alert.Root variant="destructive" data-testid="access-error">
				<Alert.Description>{error}</Alert.Description>
			</Alert.Root>
		{:else if !loaded}
			<Skeleton class="h-24 w-full" />
		{:else}
			<Table.Root data-testid="access-rules">
				<Table.Header>
					<Table.Row>
						<Table.Head class="w-32">Role</Table.Head>
						<Table.Head>Rule</Table.Head>
						<Table.Head>Node sets</Table.Head>
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#each rules?.rules ?? [] as r, i (i)}
						<Table.Row data-testid="access-rule">
							<Table.Cell><Badge variant="outline">{r.role}</Badge></Table.Cell>
							<Table.Cell class="font-mono text-xs">{r.rule}</Table.Cell>
							<Table.Cell class="text-xs" data-testid="access-rule-sets">
								{#if (r.node_sets ?? []).length}
									<span class="flex flex-wrap gap-1">
										{#each r.node_sets ?? [] as ref (ref)}<Badge variant="secondary">{ref}</Badge
											>{/each}
									</span>
								{:else}
									<span class="text-muted-foreground">every node</span>
								{/if}
							</Table.Cell>
						</Table.Row>
					{:else}
						<Table.Row>
							<Table.Cell colspan={3} class="text-muted-foreground" data-testid="access-no-rules">
								No rules on this project; roles come from the projects above it.
							</Table.Cell>
						</Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		{/if}
	</section>

	<NodeSetEditor {path} readOnly={managed} />

	{#if canCheck}
		<section class="space-y-3" aria-label="Check access" data-testid="access-check">
			<h2 class="text-lg font-medium">Check access</h2>
			<p class="text-sm text-muted-foreground">
				Why an identity has, or lacks, a role on {path}. The check is recorded in the audit log.
			</p>
			<form class="grid max-w-xl gap-3" onsubmit={check}>
				<Label for="check-groups">Groups (commas, spaces or lines)</Label>
				<Input
					id="check-groups"
					bind:value={groups}
					data-testid="check-groups"
					autocomplete="off"
				/>
				<Label for="check-email">Email (optional)</Label>
				<Input id="check-email" bind:value={email} data-testid="check-email" autocomplete="off" />
				<div><Button type="submit" disabled={busy} data-testid="check-run">Check</Button></div>
			</form>
			{#if checkError}
				<Alert.Root variant="destructive" data-testid="check-error">
					<Alert.Description>{checkError}</Alert.Description>
				</Alert.Root>
			{/if}
			{#if result}
				<div class="space-y-1 rounded-md border p-4 text-sm" data-testid="check-result">
					{#if result.role}
						<p>
							<strong data-testid="check-role">{result.role}</strong> on {result.project}, from
							{result.source ? describeSource(result.source) : 'a rule'}.
						</p>
					{:else}
						<p data-testid="check-role">No role on {result.project}.</p>
					{/if}
				</div>
			{/if}
		</section>
	{/if}
</div>
