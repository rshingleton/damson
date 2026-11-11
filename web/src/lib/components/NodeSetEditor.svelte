<script lang="ts">
	import * as Alert from '$lib/components/ui/alert';
	import * as Select from '$lib/components/ui/select';
	import * as Table from '$lib/components/ui/table';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Textarea } from '$lib/components/ui/textarea';
	import {
		REGEX_FIELDS,
		createNodeSet,
		deleteNodeSet,
		fetchNodeSets,
		fieldMessages,
		previewNodeSet,
		regexToCel,
		updateNodeSet,
		type NodeSet,
		type ProjectNode,
		type RegexField
	} from '$lib/nodesets';
	import { problemMessage } from '$lib/projects';

	/**
	 * The node sets of a project (ADR-0078): named CEL filters over a node that
	 * access rules grant. A regex helper writes the filter for the common case;
	 * the preview shows which nodes the filter matches within the project's node
	 * scope before anything is saved. Sets of an ancestor are listed, read-only.
	 */
	let { path, readOnly = false }: { path: string; readOnly?: boolean } = $props();

	let sets = $state<NodeSet[]>([]);
	let loaded = $state(false);
	let error = $state<string | null>(null);
	let busy = $state(false);

	// The form: a new set, or the own set being edited (name fixed).
	let formOpen = $state(false);
	let editing = $state<NodeSet | null>(null);
	let name = $state('');
	let description = $state('');
	let filter = $state('');
	let formErrors = $state<string[]>([]);
	let regexField = $state<RegexField>('name');
	let regexText = $state('');

	let preview = $state<{ items: ProjectNode[]; truncated: boolean } | null>(null);
	let previewError = $state<string | null>(null);
	let previewing = $state(false);
	let confirmDelete = $state<string | null>(null);

	async function load() {
		const current = path;
		try {
			const got = await fetchNodeSets(current);
			if (current === path) {
				sets = got;
				error = null;
			}
		} catch (e) {
			if (current === path) error = problemMessage(e);
		} finally {
			if (current === path) loaded = true;
		}
	}

	$effect(() => {
		void path;
		loaded = false;
		formOpen = false;
		void load();
	});

	function openNew() {
		editing = null;
		name = description = filter = regexText = '';
		formErrors = [];
		clearPreview();
		formOpen = true;
	}

	function openEdit(s: NodeSet) {
		editing = s;
		name = s.name;
		description = s.description;
		filter = s.filter;
		regexText = '';
		formErrors = [];
		clearPreview();
		formOpen = true;
	}

	function useRegex() {
		if (!regexText) return;
		filter = regexToCel(regexField, regexText);
		void runPreview();
	}

	// Only the newest preview request may show its answer.
	let previewSeq = 0;

	function clearPreview() {
		previewSeq++;
		preview = null;
		previewError = null;
		previewing = false;
	}

	async function runPreview() {
		const f = filter.trim();
		if (!f) return;
		const seq = ++previewSeq;
		previewing = true;
		previewError = null;
		try {
			const out = await previewNodeSet(path, f);
			if (seq === previewSeq) preview = out;
		} catch (e) {
			if (seq === previewSeq) {
				preview = null;
				previewError = fieldMessages(e, problemMessage).join('; ');
			}
		} finally {
			if (seq === previewSeq) previewing = false;
		}
	}

	async function save(e: Event) {
		e.preventDefault();
		busy = true;
		formErrors = [];
		try {
			if (editing) {
				await updateNodeSet(path, editing.name, {
					description,
					filter: filter.trim(),
					if_version: editing.version
				});
			} else {
				await createNodeSet(path, { name: name.trim(), description, filter: filter.trim() });
			}
			formOpen = false;
			await load();
		} catch (err) {
			// A refused expression is shown with its reason and nothing is saved.
			formErrors = fieldMessages(err, problemMessage);
		} finally {
			busy = false;
		}
	}

	async function remove(s: NodeSet) {
		busy = true;
		try {
			await deleteNodeSet(path, s.name);
			confirmDelete = null;
			error = null;
			await load();
		} catch (err) {
			error = problemMessage(err);
			confirmDelete = null;
		} finally {
			busy = false;
		}
	}
</script>

<section class="space-y-3" aria-label="Node sets" data-testid="node-sets">
	<div class="flex items-center justify-between">
		<h2 class="text-lg font-medium">Node sets</h2>
		{#if !readOnly}
			<Button size="sm" onclick={openNew} data-testid="nodeset-new">New node set</Button>
		{/if}
	</div>
	<p class="text-sm text-muted-foreground">
		A node set is a filter over the nodes of the inventory. An access rule that names sets gives its
		members only those nodes, within the project's node scope; a rule with none gives every node of
		the scope.
	</p>
	{#if error}
		<Alert.Root variant="destructive" data-testid="nodeset-error">
			<Alert.Description>{error}</Alert.Description>
		</Alert.Root>
	{/if}

	{#if loaded}
		<Table.Root data-testid="nodeset-list">
			<Table.Header>
				<Table.Row>
					<Table.Head>Set</Table.Head>
					<Table.Head>Filter</Table.Head>
					<Table.Head class="text-right"><span class="sr-only">Actions</span></Table.Head>
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#each sets as s (s.ref)}
					<Table.Row data-testid="nodeset-row-{s.ref}">
						<Table.Cell>
							<span class="font-medium">{s.ref}</span>
							{#if s.inherited}<Badge variant="outline">inherited</Badge>{/if}
							{#if s.description}
								<div class="text-xs text-muted-foreground">{s.description}</div>
							{/if}
						</Table.Cell>
						<Table.Cell class="font-mono text-xs">{s.filter}</Table.Cell>
						<Table.Cell class="text-right">
							{#if !s.inherited && !readOnly}
								<span class="inline-flex gap-2">
									<Button
										size="sm"
										variant="outline"
										data-testid="nodeset-edit-{s.name}"
										onclick={() => openEdit(s)}>Edit</Button
									>
									{#if confirmDelete === s.ref}
										<Button
											size="sm"
											variant="destructive"
											disabled={busy}
											data-testid="nodeset-confirm-{s.name}"
											onclick={() => remove(s)}>Confirm delete</Button
										>
									{:else}
										<Button
											size="sm"
											variant="outline"
											data-testid="nodeset-delete-{s.name}"
											onclick={() => (confirmDelete = s.ref)}>Delete</Button
										>
									{/if}
								</span>
							{/if}
						</Table.Cell>
					</Table.Row>
				{:else}
					<Table.Row>
						<Table.Cell colspan={3} class="text-muted-foreground" data-testid="nodeset-none">
							No node sets yet.
						</Table.Cell>
					</Table.Row>
				{/each}
			</Table.Body>
		</Table.Root>
	{/if}

	{#if formOpen}
		<form
			class="max-w-2xl space-y-3 rounded-lg border p-4"
			onsubmit={save}
			data-testid="nodeset-form"
		>
			<h3 class="font-medium">{editing ? `Edit ${editing.ref}` : 'New node set'}</h3>
			<div class="grid gap-2">
				<Label for="ns-name">Name</Label>
				<Input
					id="ns-name"
					data-testid="nodeset-name"
					bind:value={name}
					disabled={editing !== null}
					required
				/>
			</div>
			<div class="grid gap-2">
				<Label for="ns-desc">Description</Label>
				<Input id="ns-desc" data-testid="nodeset-description" bind:value={description} />
			</div>
			<div class="grid gap-2 rounded-md bg-muted/40 p-3">
				<Label>Match by regular expression</Label>
				<div class="flex flex-wrap items-center gap-2">
					<Select.Root
						type="single"
						value={regexField}
						onValueChange={(v) => (regexField = v as RegexField)}
					>
						<Select.Trigger class="w-32" data-testid="nodeset-regex-field"
							>{regexField}</Select.Trigger
						>
						<Select.Content>
							{#each REGEX_FIELDS as f (f)}<Select.Item value={f}>{f}</Select.Item>{/each}
						</Select.Content>
					</Select.Root>
					<span class="text-sm">matches</span>
					<Input
						class="w-56 font-mono"
						aria-label="Regular expression"
						placeholder="^web-"
						data-testid="nodeset-regex"
						bind:value={regexText}
					/>
					<Button
						type="button"
						size="sm"
						variant="outline"
						data-testid="nodeset-regex-use"
						onclick={useRegex}>Write filter</Button
					>
				</div>
			</div>
			<div class="grid gap-2">
				<Label for="ns-filter">Filter (CEL over node)</Label>
				<Textarea
					id="ns-filter"
					class="font-mono"
					rows={3}
					placeholder="'web' in node.tags"
					data-testid="nodeset-filter"
					oninput={clearPreview}
					bind:value={filter}
					required
				/>
			</div>
			<div class="space-y-2">
				<Button
					type="button"
					size="sm"
					variant="outline"
					disabled={previewing || !filter.trim()}
					data-testid="nodeset-preview"
					onclick={runPreview}>Preview matching nodes</Button
				>
				{#if previewError}
					<p class="text-sm text-destructive" data-testid="nodeset-preview-error">{previewError}</p>
				{/if}
				{#if preview}
					<div class="text-sm" data-testid="nodeset-preview-result">
						<p>
							<span data-testid="nodeset-preview-count">{preview.items.length}</span>
							node{preview.items.length === 1 ? '' : 's'} match{preview.truncated
								? ' (first 200 shown)'
								: ''}:
						</p>
						<p class="text-muted-foreground" data-testid="nodeset-preview-names">
							{preview.items.map((n) => n.name).join(', ') || 'none'}
						</p>
					</div>
				{/if}
			</div>
			{#if formErrors.length}
				<Alert.Root variant="destructive" data-testid="nodeset-form-error">
					<Alert.Description>
						{#each formErrors as m (m)}<div>{m}</div>{/each}
					</Alert.Description>
				</Alert.Root>
			{/if}
			<div class="flex gap-2">
				<Button type="submit" disabled={busy} data-testid="nodeset-save">Save</Button>
				<Button
					type="button"
					variant="outline"
					onclick={() => {
						formOpen = false;
						clearPreview();
					}}>Cancel</Button
				>
			</div>
		</form>
	{/if}
</section>
