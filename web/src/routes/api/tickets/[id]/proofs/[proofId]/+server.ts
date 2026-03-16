import type { RequestHandler } from './$types';
import { readTicketProofBlob } from '$lib/server/docket';

export const GET: RequestHandler = async ({ params, url }) => {
	const projectId = url.searchParams.get('projectId') ?? undefined;
	const blob = readTicketProofBlob(params.id, params.proofId, projectId);
	if (!blob) {
		return new Response('Not found', { status: 404 });
	}
	return new Response(blob.bytes, {
		status: 200,
		headers: {
			'content-type': blob.mimeType,
			'cache-control': 'no-store'
		}
	});
};
