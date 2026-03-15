import type { Ticket } from '../types';

type PreloadMessage = {
	type: 'preload';
	projectId: string | null;
};

type GetTicketMessage = {
	type: 'get-ticket';
	projectId: string | null;
	id: string;
};

type WorkerRequest = PreloadMessage | GetTicketMessage;

type WorkerResponse =
	| { type: 'ready'; projectId: string | null; count: number }
	| { type: 'ticket'; projectId: string | null; id: string; ticket: Ticket | null }
	| { type: 'error'; projectId: string | null; error: string };

const ctx = self as unknown as {
	onmessage: ((event: MessageEvent<WorkerRequest>) => void) | null;
	postMessage: (message: WorkerResponse) => void;
};

let loadedProjectId: string | null = null;
let loadedTickets = new Map<string, Ticket>();

ctx.onmessage = async (event: MessageEvent<WorkerRequest>) => {
	const msg = event.data;
	if (msg.type === 'preload') {
		const query = msg.projectId ? `?projectId=${encodeURIComponent(msg.projectId)}` : '';
		try {
			const response = await fetch(`/api/tickets${query}`);
			const payload = await response.json().catch(() => ({} as { ok?: boolean; tickets?: Ticket[] }));
			if (!response.ok || !payload.ok || !Array.isArray(payload.tickets)) {
				ctx.postMessage({
					type: 'error',
					projectId: msg.projectId,
					error: 'Failed to preload tickets.'
				} satisfies WorkerResponse);
				return;
			}
			loadedProjectId = msg.projectId;
			loadedTickets = new Map(payload.tickets.map((ticket: Ticket) => [ticket.id, ticket]));
			ctx.postMessage({
				type: 'ready',
				projectId: msg.projectId,
				count: loadedTickets.size
			} satisfies WorkerResponse);
		} catch (error: unknown) {
			ctx.postMessage({
				type: 'error',
				projectId: msg.projectId,
				error: error instanceof Error ? error.message : 'Failed to preload tickets.'
			} satisfies WorkerResponse);
		}
		return;
	}

	if (msg.type === 'get-ticket') {
		if (msg.projectId !== loadedProjectId) {
			ctx.postMessage({
				type: 'ticket',
				projectId: msg.projectId,
				id: msg.id,
				ticket: null
			} satisfies WorkerResponse);
			return;
		}
		ctx.postMessage({
			type: 'ticket',
			projectId: msg.projectId,
			id: msg.id,
			ticket: loadedTickets.get(msg.id) ?? null
		} satisfies WorkerResponse);
	}
};

export {};
