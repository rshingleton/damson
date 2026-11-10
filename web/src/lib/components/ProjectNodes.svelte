<script lang="ts">
	import { onMount } from 'svelte';
	import * as Alert from '$lib/components/ui/alert';
	import * as Table from '$lib/components/ui/table';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import {
		filterNodes,
		fetchProjectNodes,
		type NodePage,
		type ProjectNode
	} from '$lib/projectnodes';
	import { problemMessage } from '$lib/projects';

	/**
	 * The nodes this project's members may see (ADR-0078): every node within the
	 * project's node scope, or only those of the node sets the caller's access
	 * rules grant, each with the sets that grant it. Read-only; the server
	 * decides what is listed, so a hidden node is simply absent.
	 */
	let { path }: { path: string } = $props();

	let nodes = $state<ProjectNode[]>([]);
	let access = $state<NodePage['access']>('all');
	let next = $state('');
	let loaded = $state(false);
	let loadingMore = $state(false);
	let error = $state<string | null>(null);
	let query = $state('');

	const filtered = $derived(filterNodes(nodes, query));

	async function load(more: boolean) {
		if (more) loadingMore = true;
		try {
			const page = await fetchProjectNodes(path, more ? next : '');
			nodes = more ? [...nodes, ...page.items] : page.items;
			access = page.access;
			next = page.next;
			error = null;
		} catch (e) {
			error = problemMessage(e);
		} finally {
			loaded = true;
			loadingMore = false;
		}
	}

	onMount(() => load(false));
</script>

<section class="space-y-4" data-testid="project-nodes">
	<div class="space-y-1">
		<h1 class="text-xl font-semibold">Nodes</h1>
		<p class="text-sm text-muted-foreground" data-testid="nodes-access">
			{#if !loaded}
				&nbsp;
			{:else if access === 'sets'}
				You see the nodes of the node sets your access rules grant.
			{:else}
				You see every node this project may use.
			{/if}
		</p>
	</div>

	{#if error}
		<Alert.Root variant="destructive" data-testid="nodes-error">
			<Alert.Description>{error}</Alert.Description>
		</Alert.Root>
	{/if}

	{#if !loaded}
		<Skeleton class="h-24 w-full" />
	{:else}
		<Input
			class="max-w-sm"
			placeholder="Filter by name, address, tag, or /regex/"
			aria-label="Filter nodes"
			data-testid="nodes-filter"
			bind:value={query}
		/>
		{#if filtered.error}
			<p class="text-sm text-destructive">{filtered.error}</p>
		{/if}
		<div class="overflow-x-auto rounded-lg border">
			<Table.Root>
				<Table.Header>
					<Table.Row>
						<Table.Head>Name</Table.Head>
						<Table.Head>Address</Table.Head>
						<Table.Head>Tags</Table.Head>
						<Table.Head>Pool</Table.Head>
						<Table.Head>OS</Table.Head>
						<Table.Head>Enabled</Table.Head>
						<Table.Head>Visible through</Table.Head>
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#each filtered.items as n (n.id)}
						<Table.Row data-testid="node-row">
							<Table.Cell class="font-medium" data-testid="node-name">{n.name}</Table.Cell>
							<Table.Cell>{n.address}</Table.Cell>
							<Table.Cell>
								<span class="flex flex-wrap gap-1">
									{#each n.tags ?? [] as tag (tag)}<Badge variant="secondary">{tag}</Badge>{/each}
								</span>
							</Table.Cell>
							<Table.Cell>{n.pool}</Table.Cell>
							<Table.Cell>{n.os}</Table.Cell>
							<Table.Cell>{n.enabled ? 'yes' : 'no'}</Table.Cell>
							<Table.Cell data-testid="node-through">
								{#if (n.visible_through ?? []).length}
									<span class="flex flex-wrap gap-1">
										{#each n.visible_through ?? [] as ref (ref)}<Badge variant="outline"
												>{ref}</Badge
											>{/each}
									</span>
								{:else}
									<span class="text-muted-foreground">all</span>
								{/if}
							</Table.Cell>
						</Table.Row>
					{:else}
						<Table.Row>
							<Table.Cell colspan={7} class="text-muted-foreground" data-testid="nodes-empty">
								{error && nodes.length === 0
									? 'The nodes could not be loaded.'
									: nodes.length === 0
										? 'No node is available to you in this project.'
										: next
											? 'No match in the nodes loaded so far; load more.'
											: 'No node matches.'}
							</Table.Cell>
						</Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		</div>
		{#if next}
			<Button
				variant="outline"
				disabled={loadingMore}
				data-testid="nodes-more"
				onclick={() => load(true)}>Load more</Button
			>
		{/if}
	{/if}
</section>
