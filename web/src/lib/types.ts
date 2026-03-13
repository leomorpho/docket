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

export type Ticket = {
	id: string;
	seq: number;
	state: string;
	priority: number;
	labels: string[];
	parent?: string;
	title: string;
	created_at: string;
	updated_at: string;
	started_at?: string;
	completed_at?: string;
	ac: AcceptanceCriterion[];
	plan: PlanStep[];
	handoff?: string;
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
