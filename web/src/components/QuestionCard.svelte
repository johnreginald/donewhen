<script>
	// Paperclip's question card: an agent's questions, one at a time ("2 of
	// 4"), each with its options and, where allowed, a written answer. The
	// answers go back to the agent as its next turn.
	import { api } from '$lib/api.js';
	import { showToast } from '$lib/ui.js';
	import { CircleHelp, ChevronLeft, ChevronRight, X } from '@lucide/svelte';

	let { interaction, agentName = 'The agent', ondone } = $props();

	const questions = $derived(interaction.payload?.questions || []);
	let i = $state(0);
	let answers = $state({}); // question id -> { choices: [], other: '' }
	let submitting = $state(false);
	let dismissed = $state(false);

	const q = $derived(questions[i]);
	const a = $derived(answers[q?.id] || { choices: [], other: '' });
	const answered = (qq) => {
		const x = answers[qq.id];
		return x && (x.choices.length > 0 || x.other.trim());
	};
	const allAnswered = $derived(questions.every(answered));

	function pick(opt) {
		const cur = answers[q.id] || { choices: [], other: '' };
		let choices;
		if (q.multi) choices = cur.choices.includes(opt) ? cur.choices.filter((c) => c !== opt) : [...cur.choices, opt];
		else choices = [opt];
		answers = { ...answers, [q.id]: { ...cur, choices } };
	}
	function setOther(v) {
		const cur = answers[q.id] || { choices: [], other: '' };
		answers = { ...answers, [q.id]: { ...cur, other: v } };
	}
	function next() {
		if (i < questions.length - 1) i++;
		else submit();
	}
	async function submit() {
		if (!allAnswered) {
			i = questions.findIndex((qq) => !answered(qq));
			return;
		}
		submitting = true;
		try {
			const list = questions.map((qq) => ({
				questionId: qq.id,
				choices: answers[qq.id].choices,
				other: answers[qq.id].other.trim()
			}));
			const res = await api.post(`/interactions/${interaction.id}/respond`, { answers: list });
			ondone?.(res);
		} catch (e) {
			showToast(e.message, 'error');
		} finally {
			submitting = false;
		}
	}
</script>

{#if !dismissed && q}
	<div class="qc">
		<div class="qh">
			<CircleHelp size={15} strokeWidth={2} />
			<span class="qt">{agentName} has {questions.length === 1 ? 'a question' : 'a few questions'}</span>
			<span class="nav">
				<button onclick={() => (i = Math.max(0, i - 1))} disabled={i === 0} aria-label="Previous"><ChevronLeft size={15} /></button>
				<span class="pos">{i + 1} of {questions.length}</span>
				<button onclick={() => (i = Math.min(questions.length - 1, i + 1))} disabled={i === questions.length - 1} aria-label="Next"
					><ChevronRight size={15} /></button>
			</span>
			<button class="x" onclick={() => (dismissed = true)} aria-label="Hide"><X size={14} /></button>
		</div>
		<div class="qq">{q.text}</div>
		{#if q.options?.length}
			<div class="opts" role={q.multi ? 'group' : 'radiogroup'}>
				{#each q.options as opt (opt)}
					<button class="opt" class:on={a.choices.includes(opt)} onclick={() => pick(opt)}
						role={q.multi ? 'checkbox' : 'radio'} aria-checked={a.choices.includes(opt)}>
						<span class="mark" class:sq={q.multi}></span>{opt}
					</button>
				{/each}
			</div>
		{/if}
		{#if q.allowOther}
			<textarea
				class="other"
				rows="2"
				value={a.other}
				oninput={(e) => setOther(e.currentTarget.value)}
				placeholder={q.options?.length ? 'Something else — say what' : 'Your answer'}
			></textarea>
		{/if}
		<div class="qf">
			<span class="done">{questions.filter(answered).length}/{questions.length} answered</span>
			<button class="btn primary sm" onclick={next} disabled={submitting || !answered(q)}>
				{i < questions.length - 1 ? 'Next' : submitting ? 'Sending…' : 'Send answers'}
			</button>
		</div>
	</div>
{/if}

<style>
	.qc {
		background: var(--bg-elev);
		border: 1px solid var(--border-strong);
		border-radius: 12px;
		padding: 14px 16px;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	.qh {
		display: flex;
		align-items: center;
		gap: 8px;
		color: var(--text-dim);
		font-size: 13px;
	}
	.qt {
		color: var(--text);
	}
	.nav {
		margin-left: auto;
		display: inline-flex;
		align-items: center;
		gap: 4px;
	}
	.nav button,
	.x {
		background: none;
		border: none;
		color: var(--text-faint);
		display: inline-flex;
		padding: 3px;
		border-radius: 5px;
	}
	.nav button:disabled {
		opacity: 0.35;
	}
	.nav button:not(:disabled):hover,
	.x:hover {
		color: var(--text);
		background: var(--bg-hover);
	}
	.pos {
		font-size: 12.5px;
		font-variant-numeric: tabular-nums;
	}
	.qq {
		font-size: 14px;
		line-height: 1.5;
	}
	.opts {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.opt {
		display: flex;
		align-items: center;
		gap: 10px;
		background: none;
		border: none;
		color: var(--text);
		text-align: left;
		font-size: 13.5px;
		padding: 8px 8px;
		border-radius: 8px;
	}
	.opt:hover {
		background: var(--bg-hover);
	}
	.mark {
		width: 15px;
		height: 15px;
		border-radius: 50%;
		border: 1.5px solid var(--border-strong);
		flex-shrink: 0;
	}
	.mark.sq {
		border-radius: 4px;
	}
	.opt.on .mark {
		border-color: var(--accent2);
		background: radial-gradient(circle, var(--accent2) 45%, transparent 50%);
	}
	.opt.on .mark.sq {
		background: var(--accent2);
	}
	.other {
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 8px 10px;
		font-size: 13px;
		color: var(--text);
		resize: vertical;
		outline: none;
		font-family: inherit;
	}
	.other:focus {
		border-color: var(--border-strong);
	}
	.qf {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}
	.done {
		font-size: 12px;
		color: var(--text-faint);
	}
</style>
