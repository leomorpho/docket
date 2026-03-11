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
	body: string;
};
