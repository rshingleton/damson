<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import * as Alert from '$lib/components/ui/alert';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import * as Table from '$lib/components/ui/table';
	import * as Select from '$lib/components/ui/select';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import {
		ROLES,
		atLeast,
		canManage,
		createProject,
		deleteProject,
		fetchAccess,
		fetchNodeScope,
		fetchProjects,
		isInstanceAdmin,
		problemMessage,
		saveAccess,
		saveNodeScope,
		type AccessRule,
		type Project
	} from '$lib/projects';
	import { fetchNodeSets, type NodeSet } from '$lib/nodesets';
	import { projectMatcher } from '$lib/projectsearch';
	import { projectRest } from '$lib/projectroute';
	import { fetchLevel } from '$lib/tree';

	let projects = $state<Project[]>([]);
	let loaded = $state(false);
	let error = $state<string | null>(null);
	let busy = $state(false);

	const byPath = $derived(new Map(projects.map((p) => [p.path, p])));
	// Jobs in a project and below it, from the tree; absent until it loads.
	let jobCounts = $state<Record<string, number>>({});
	let query = $state('');
	const matcher = $derived(projectMatcher(query));
	const visible = $derived(
		projects.filter((p) => matcher.test(p.path) || matcher.test(p.description ?? ''))
	);
	const childCount = $derived(
		projects.reduce(
			(m, p) => (p.parent_path ? m.set(p.parent_path, (m.get(p.parent_path) ?? 0) + 1) : m),
			new Map<string, number>()
		)
	);
	// Where a new project can go: under a project the caller administers, or at
	// the top for an instance admin.
	const parents = $derived(projects.filter((p) => atLeast(p.my_role, 'admin')));
	const canCreateRoot = $derived(isInstanceAdmin());

	let newParent = $state('');
	let newName = $state('');
	let newDescription = $state('');
	let createOpen = $state(false);
	let createError = $state<string | null>(null);

	// The project whose rules and node scope are being edited.
	let editing = $state<string | null>(null);
	// A project that apply manages is read-only here: its source changes it.
	const editingManaged = $derived(
		editing !== null && (byPath.get(editing)?.managed_by ?? 'ui') !== 'ui'
	);
	let rules = $state<AccessRule[]>([]);
	// The node sets a rule here may name: this project's and its ancestors'.
	let availableSets = $state<NodeSet[]>([]);
	let rulesVersion = $state(0);
	let rulesErrors = $state<string[]>([]);
	let nodeScope = $state('');
	let nodeScopeVersion = $state(0);
	let nodeScopeError = $state<string | null>(null);
	let saved = $state<string | null>(null);

	async function load() {
		try {
			projects = await fetchProjects();
			error = null;
			void loadJobCounts();
		} catch (e) {
			error = problemMessage(e);
		} finally {
			loaded = true;
		}
	}

	// shortcut: one tree call per parent, first page (100) of each; upgrade to a counts field
	// on the project list when a level outgrows that.
	async function loadJobCounts() {
		const parents = new Set(['', ...projects.map((p) => p.parent_path ?? '').filter(Boolean)]);
		const counts: Record<string, number> = {};
		try {
			const levels = await Promise.all([...parents].map((p) => fetchLevel(p)));
			for (const l of levels) for (const t of l.projects) counts[t.path] = t.jobs ?? 0;
			jobCounts = counts;
		} catch {
			// the counts are decoration: the table stands without them
		}
	}

	async function act(fn: () => Promise<void>) {
		busy = true;
		saved = null;
		try {
			await fn();
			error = null;
			await load();
		} catch (e) {
			error = problemMessage(e);
		} finally {
			busy = false;
		}
	}

	async function create() {
		const name = newName.trim();
		if (!name) return;
		const path = newParent ? `${newParent}/${name}` : name;
		busy = true;
		createError = null;
		try {
			await createProject(path, newDescription.trim());
			newName = newDescription = '';
			createOpen = false;
			await load();
		} catch (e) {
			// The refusal stays in the dialog, beside the field that caused it.
			createError = problemMessage(e);
		} finally {
			busy = false;
		}
	}

	function remove(path: string) {
		return act(async () => {
			await deleteProject(path);
			if (editing === path) editing = null;
		});
	}

	/** The messages of a 422 problem: one per invalid field. */
	function fieldMessages(e: unknown): string[] {
		const errs = (e as { errors?: { message?: string; location?: string }[] })?.errors;
		if (!Array.isArray(errs) || errs.length === 0) return [problemMessage(e)];
		return errs.map((f) => `${f.location ?? ''}: ${f.message ?? ''}`);
	}

	async function edit(path: string) {
		editing = path;
		saved = null;
		rulesErrors = [];
		nodeScopeError = null;
		try {
			const [a, n, ns] = await Promise.all([
				fetchAccess(path),
				fetchNodeScope(path),
				// The rules do not need the sets: without them the picker is simply absent.
				fetchNodeSets(path).catch(() => [] as NodeSet[])
			]);
			availableSets = ns;
			rules = (a.rules ?? []).map((r) => ({ ...r }));
			rulesVersion = a.version;
			nodeScope = n.node_scope;
			nodeScopeVersion = n.version;
		} catch (e) {
			error = problemMessage(e);
			editing = null;
		}
	}

	function toggleSet(i: number, ref: string, on: boolean) {
		const cur = rules[i].node_sets ?? [];
		const next = on ? [...cur.filter((r) => r !== ref), ref] : cur.filter((r) => r !== ref);
		rules[i].node_sets = next.length ? next : undefined;
	}

	async function saveRules() {
		if (!editing) return;
		const path = editing;
		busy = true;
		saved = null;
		rulesErrors = [];
		try {
			const out = await saveAccess(path, rules, rulesVersion);
			rules = (out.rules ?? []).map((r) => ({ ...r }));
			rulesVersion = out.version;
			// The project version moved too: take the new node scope version.
			nodeScopeVersion = (await fetchNodeScope(path)).version;
			saved = 'Access rules saved.';
		} catch (e) {
			rulesErrors = fieldMessages(e);
		} finally {
			busy = false;
		}
	}

	async function saveScope() {
		if (!editing) return;
		const path = editing;
		busy = true;
		saved = null;
		nodeScopeError = null;
		try {
			const out = await saveNodeScope(path, nodeScope, nodeScopeVersion);
			nodeScope = out.node_scope;
			nodeScopeVersion = out.version;
			rulesVersion = (await fetchAccess(path)).version;
			saved = 'Node scope saved.';
		} catch (e) {
			nodeScopeError = fieldMessages(e).join('; ');
		} finally {
			busy = false;
		}
	}

	onMount(load);
</script>

<main class="mx-auto max-w-6xl space-y-6 px-4 py-6">
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-semibold">Projects</h1>
		{#if loaded && (parents.length > 0 || canCreateRoot)}
			<Button
				data-testid="create-project"
				onclick={() => {
					createError = null;
					createOpen = true;
				}}>New project</Button
			>
		{/if}
	</div>

	{#if error}
		<Alert.Root variant="destructive" data-testid="project-error">
			<Alert.Description>{error}</Alert.Description>
		</Alert.Root>
	{/if}

	{#if !loaded}
		<p class="text-muted-foreground">Loading…</p>
	{:else if projects.length === 0}
		<p class="text-muted-foreground" data-testid="no-projects">
			You do not have access to any project yet.
		</p>
	{:else}
		<Input
			class="max-w-sm"
			placeholder="Search projects, or /regex/"
			aria-label="Search projects"
			data-testid="project-search"
			bind:value={query}
		/>
		{#if matcher.error}
			<p class="text-sm text-destructive">{matcher.error}</p>
		{/if}
		<div class="rounded-lg border">
			<Table.Root data-testid="project-tree">
				<Table.Header>
					<Table.Row>
						<Table.Head>Project</Table.Head>
						<Table.Head>Description</Table.Head>
						<Table.Head class="text-right">Child projects</Table.Head>
						<Table.Head class="text-right">Jobs</Table.Head>
						<Table.Head>Your role</Table.Head>
						<Table.Head class="text-right"><span class="sr-only">Actions</span></Table.Head>
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#each visible as p (p.path)}
						<Table.Row
							class="cursor-pointer"
							data-testid="project-row-{p.path}"
							onclick={(e: MouseEvent) => {
								// A click on a button or link keeps its own meaning.
								if ((e.target as HTMLElement).closest('a,button')) return;
								goto(resolve('/projects/[...path]', { path: projectRest(p.path) }));
							}}
						>
							<Table.Cell>
								<a
									class="font-medium hover:underline"
									href={resolve('/projects/[...path]', { path: projectRest(p.path) })}
									data-testid="project-link-{p.path}">{p.path}</a
								>
								{#if p.managed_by !== 'ui'}
									<Badge variant="outline" data-testid="managed-{p.path}"
										>managed by {p.managed_by}</Badge
									>
								{/if}
							</Table.Cell>
							<Table.Cell class="text-muted-foreground">{p.description ?? ''}</Table.Cell>
							<Table.Cell class="text-right" data-testid="children-{p.path}"
								>{childCount.get(p.path) ?? 0}</Table.Cell
							>
							<Table.Cell class="text-right" data-testid="jobs-{p.path}"
								>{jobCounts[p.path] ?? '–'}</Table.Cell
							>
							<Table.Cell>
								<Badge variant="secondary" data-testid="role-{p.path}">{p.my_role ?? 'none'}</Badge>
							</Table.Cell>
							<Table.Cell class="text-right">
								<span class="inline-flex gap-2">
									{#if atLeast(p.my_role, 'admin')}
										<Button
											size="sm"
											variant="outline"
											data-testid="edit-{p.path}"
											onclick={() => edit(p.path)}
											>{p.managed_by === 'ui' ? 'Rules and scope' : 'View rules and scope'}</Button
										>
									{/if}
									{#if canManage(p.path, byPath)}
										<Button
											size="sm"
											variant="outline"
											disabled={busy || p.managed_by !== 'ui'}
											title={p.managed_by !== 'ui' ? 'managed by ' + p.managed_by : undefined}
											data-testid="delete-{p.path}"
											onclick={() => remove(p.path)}>Delete</Button
										>
									{/if}
								</span>
							</Table.Cell>
						</Table.Row>
					{:else}
						<Table.Row>
							<Table.Cell colspan={6} class="text-muted-foreground" data-testid="no-match"
								>No project matches.</Table.Cell
							>
						</Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		</div>
	{/if}

	<Dialog.Root
		bind:open={createOpen}
		onOpenChange={(o) => {
			if (!o) newName = newDescription = '';
			createError = null;
		}}
	>
		<Dialog.Content data-testid="create-dialog">
			<Dialog.Header>
				<Dialog.Title>New project</Dialog.Title>
			</Dialog.Header>
			<form
				class="space-y-3"
				data-testid="create-form"
				onsubmit={(e) => {
					e.preventDefault();
					create();
				}}
			>
				<div class="grid gap-2">
					<Label>Parent</Label>
					<Select.Root type="single" value={newParent} onValueChange={(v) => (newParent = v)}>
						<Select.Trigger data-testid="create-parent" class="w-64">
							{newParent || (canCreateRoot ? '(top level)' : 'choose a parent')}
						</Select.Trigger>
						<Select.Content>
							{#if canCreateRoot}
								<Select.Item value="">(top level)</Select.Item>
							{/if}
							{#each parents as p (p.path)}
								<Select.Item value={p.path}>{p.path}</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
				</div>
				<div class="grid gap-2">
					<Label for="create-name">Name</Label>
					<Input id="create-name" data-testid="create-name" bind:value={newName} required />
				</div>
				<div class="grid gap-2">
					<Label for="create-description">Description</Label>
					<Input id="create-description" bind:value={newDescription} />
				</div>
				{#if createError}
					<Alert.Root variant="destructive" data-testid="create-error">
						<Alert.Description>{createError}</Alert.Description>
					</Alert.Root>
				{/if}
				<Dialog.Footer>
					<Button type="submit" disabled={busy} data-testid="create-submit">Create</Button>
				</Dialog.Footer>
			</form>
		</Dialog.Content>
	</Dialog.Root>

	{#if editing}
		<section class="space-y-6 rounded-lg border p-4" data-testid="project-editor">
			<h2 class="font-medium">
				<span data-testid="editing-path">{editing}</span>
			</h2>
			{#if saved}
				<Alert.Root data-testid="saved"><Alert.Description>{saved}</Alert.Description></Alert.Root>
			{/if}

			<div class="space-y-3">
				<h3 class="text-sm font-medium">Access rules</h3>
				<p class="text-sm text-muted-foreground">
					A user holds the highest role whose rule is true. Rules apply to this project and the ones
					below it. Example: <code>'billing-ops' in claims.groups</code>
				</p>
				{#each rules as rule, i (i)}
					<div class="space-y-1" data-testid="rule-{i}">
						<div class="flex items-center gap-2">
							<Select.Root
								type="single"
								value={rule.role}
								onValueChange={(v) => {
									rules[i].role = v as AccessRule['role'];
									// An admin rule cannot carry node sets.
									if (v === 'admin') rules[i].node_sets = undefined;
								}}
							>
								<Select.Trigger data-testid="rule-role-{i}" class="w-32">{rule.role}</Select.Trigger
								>
								<Select.Content>
									{#each ROLES as r (r)}
										<Select.Item value={r}>{r}</Select.Item>
									{/each}
								</Select.Content>
							</Select.Root>
							<Input
								class="flex-1 font-mono"
								aria-label="Rule {i + 1}"
								data-testid="rule-text-{i}"
								bind:value={rules[i].rule}
							/>
							<Button
								size="sm"
								variant="outline"
								data-testid="rule-remove-{i}"
								onclick={() => rules.splice(i, 1)}>Remove</Button
							>
						</div>
						{#if availableSets.length && rule.role !== 'admin'}
							<div
								class="flex flex-wrap items-center gap-3 pl-1 text-sm"
								data-testid="rule-sets-{i}"
							>
								<span class="text-muted-foreground">Node sets:</span>
								{#each availableSets as ns (ns.ref)}
									<label class="flex items-center gap-1.5">
										<Checkbox
											checked={(rule.node_sets ?? []).includes(ns.ref)}
											onCheckedChange={(v) => toggleSet(i, ns.ref, v === true)}
											data-testid="rule-set-{i}-{ns.ref}"
										/>
										{ns.ref}
									</label>
								{/each}
								<span class="text-xs text-muted-foreground">
									{(rule.node_sets ?? []).length
										? ''
										: 'none checked: every node of the project scope'}
								</span>
							</div>
						{/if}
					</div>
				{/each}
				<div class="flex gap-2">
					<Button
						size="sm"
						variant="outline"
						data-testid="add-rule"
						onclick={() => rules.push({ role: 'viewer', rule: '' })}>Add rule</Button
					>
					<Button
						size="sm"
						disabled={busy || editingManaged}
						data-testid="save-rules"
						onclick={saveRules}>Save rules</Button
					>
				</div>
				{#if rulesErrors.length}
					<Alert.Root variant="destructive" data-testid="rules-error">
						<Alert.Description>
							{#each rulesErrors as m (m)}<div>{m}</div>{/each}
						</Alert.Description>
					</Alert.Root>
				{/if}
			</div>

			<div class="space-y-3">
				<h3 class="text-sm font-medium">Node scope</h3>
				<p class="text-sm text-muted-foreground">
					Which nodes jobs in this project may use, as an expression over <code>node</code>; empty
					allows every node. Example: <code>'prod' in node.tags</code>
				</p>
				<Input
					class="font-mono"
					aria-label="Node scope"
					data-testid="node-scope"
					bind:value={nodeScope}
				/>
				<Button
					size="sm"
					disabled={busy || editingManaged}
					data-testid="save-node-scope"
					onclick={saveScope}>Save node scope</Button
				>
				{#if nodeScopeError}
					<Alert.Root variant="destructive" data-testid="node-scope-error">
						<Alert.Description>{nodeScopeError}</Alert.Description>
					</Alert.Root>
				{/if}
			</div>
		</section>
	{/if}
</main>
