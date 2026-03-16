export type StateConfig = {
	label: string;
	open: boolean;
	column: number;
	next: string[];
};

export type Config = {
	states: Record<string, StateConfig>;
	default_state: string;
	default_priority: number;
	labels: string[];
};

export type Project = {
	id: string;
	dir: string;
	name: string;
	registeredAt: string;
};

export type Relation = {
	from: string;
	to: string;
	relation: string;
};

export type AcceptanceCriterion = {
	description: string;
	done: boolean;
	evidence?: string;
};

export type PlanStep = {
	description: string;
	status: string;
	notes?: string;
};

export type Comment = {
	author: string;
	at: string;
	body: string;
};

export type Proof = {
	id: string;
	ticket_id: string;
	proof_title: string;
	note: string;
	added_at: string;
	captured_at?: string;
	actor?: string;
	file: {
		path: string;
		mime_type: string;
		size_bytes: number;
		sha256: string;
	};
};

export type Ticket = {
	id: string;
	seq: number;
	state: string;
	priority: number;
	labels: string[];
	blocked_by?: string[];
	parent?: string;
	children?: string[];
	title: string;
	created_at: string;
	updated_at: string;
	started_at?: string;
	completed_at?: string;
	created_by?: string;
	write_hash?: string;
	description?: string;
	ac: AcceptanceCriterion[];
	plan: PlanStep[];
	comments: Comment[];
	handoff?: string;
	proofs?: Proof[];
	frontmatter?: Record<string, string | string[]>;
	body: string;
};

export type Finding = {
	ticketId: string;
	rule: string;
	message: string;
	severity: 'error' | 'warning';
};

export type ProjectHealth = {
	ticketCount: number;
	invalidSignatures: string[];
	staleTickets: string[];
	stateDistribution: Record<string, number>;
	priorityDistribution: Record<number, number>;
	findings: Finding[];
	avgCycleTime: number;
};
