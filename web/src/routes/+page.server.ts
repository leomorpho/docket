import type { PageServerLoad } from './$types';
import { readConfig, readTickets } from '$lib/server/docket';

export const load: PageServerLoad = async () => {
	return {
		config: readConfig(),
		tickets: readTickets()
	};
};
