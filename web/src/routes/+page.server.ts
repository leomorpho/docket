import type { PageServerLoad } from './$types';
import { readConfig, readTickets, readRelations } from '$lib/server/docket';
import { listProjects } from '$lib/server/registry';

export const load: PageServerLoad = async ({ url }) => {
	const projects = listProjects();
	const projectId = url.searchParams.get('project') ?? projects[0]?.id;
	return {
		projects,
		activeProjectId: projectId ?? null,
		config: readConfig(projectId ?? undefined),
		tickets: readTickets(projectId ?? undefined),
		relations: readRelations(projectId ?? undefined)
	};
};
