<script lang="ts">
  // A question from a chat's agent, as its terminal asks it: one question, or
  // one tab per question and a Review tab. Answers can change until Submit.
  // Nothing is chosen for the user: a default is marked, not picked, and
  // Enter does not submit. When the agent goes on without the answer, the card
  // stays, late, with the draft: the answers can still go as a message.
  import { tick } from 'svelte';
  import type { Chat, ChatPendingQuestion } from '../../stores/chats.svelte';
  import { chats } from '../../stores/chats.svelte';
  import {
    answerText,
    answered,
    choose,
    chooseOwn,
    clear,
    content,
    lateBlocker,
    lateMessage,
    problem,
    questionForm,
    setNumber,
    setOwn,
    setText,
    setYes,
    submitBlocker,
    toggle,
    type Draft,
    type FormQuestion,
  } from '../../chatQuestions';
  import { isStaleReply } from '../../link';
  import { errorText } from '../../manage';
  import Icon from '../common/Icon.svelte';

  interface Props {
    chat: Chat;
    question: ChatPendingQuestion;
    agentName: string;
  }

  let { chat, question, agentName }: Props = $props();

  let form = $derived(questionForm(question));
  let draft = $state<Draft>({});
  /** The open tab: a question's index, or questions.length for Review. */
  let tab = $state(0);
  let sending = $state(false);
  let error = $state<string | undefined>();
  let card = $state<HTMLElement | undefined>();

  let late = $derived(!!question.late);
  let several = $derived(form.questions.length > 1);
  let reviewing = $derived(several && tab >= form.questions.length);
  let current = $derived(reviewing ? undefined : form.questions[Math.min(tab, form.questions.length - 1)]);
  let blocker = $derived(
    !late
      ? submitBlocker(form, draft)
      : chat.phase === 'idle'
        ? lateBlocker(form, draft)
        : `${agentName} is still working. Send your answers once it finishes.`,
  );
  let sent = $derived(content(form, draft));
  let canSubmit = $derived(!sending && !blocker);
  let lastQuestion = $derived(!several || tab === form.questions.length - 1);
  let waitHint = $derived(late ? undefined : autoResolutionHint(question.meta));
  let hasSecret = $derived(form.questions.some((q) => q.secret));

  // A reply refused because the agent went on is answered by the late note.
  $effect(() => {
    if (late) error = undefined;
  });

  // Codex answers a question by itself after a while; say so.
  function autoResolutionHint(meta: Record<string, unknown> | undefined): string | undefined {
    const codex = meta?.codex;
    const ms = codex && typeof codex === 'object' ? (codex as Record<string, unknown>).autoResolutionMs : undefined;
    if (typeof ms !== 'number' || !(ms > 0)) return undefined;
    const s = Math.round(ms / 1000);
    const span = s < 90 ? `${s} second${s === 1 ? '' : 's'}` : `${Math.round(s / 60)} minutes`;
    return `${agentName} goes on without an answer after about ${span}.`;
  }

  async function go(next: number): Promise<void> {
    const last = several ? form.questions.length : 0;
    tab = Math.max(0, Math.min(next, last));
    // The button pressed may be gone; keep the keys working on the card.
    await tick();
    if (card && !card.contains(document.activeElement)) card.focus({ preventScroll: true });
  }

  function advance(): void {
    if (several) void go(tab + 1);
  }

  function pick(q: FormQuestion, value: string): void {
    draft = choose(draft, q.key, value);
    advance();
  }

  function pickYes(q: FormQuestion, yes: boolean): void {
    draft = setYes(draft, q.key, yes);
    advance();
  }

  function focusOwn(q: FormQuestion): void {
    const boxes = card?.querySelectorAll<HTMLInputElement>('input[data-own]') ?? [];
    [...boxes].find((b) => b.dataset.own === q.key)?.focus();
  }

  function pickOwn(q: FormQuestion): void {
    if (q.kind === 'choice') draft = chooseOwn(draft, q.key);
    focusOwn(q);
  }

  function isTyping(target: EventTarget | null): boolean {
    return target instanceof HTMLElement && (target.isContentEditable || target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT');
  }

  // 1–9 picks an option of the open question; ← and → move between tabs.
  function onKeydown(e: KeyboardEvent): void {
    if (e.ctrlKey || e.metaKey || e.altKey || isTyping(e.target)) return;
    if (several && (e.key === 'ArrowLeft' || e.key === 'ArrowRight')) {
      e.preventDefault();
      void go(tab + (e.key === 'ArrowLeft' ? -1 : 1));
      return;
    }
    if (!/^[1-9]$/.test(e.key) || !current || sending) return;
    const n = Number(e.key) - 1;
    const q = current;
    if (q.kind === 'boolean') {
      if (n > 1) return;
      e.preventDefault();
      pickYes(q, n === 0);
    } else if (q.kind === 'choice' || q.kind === 'multi') {
      if (n < q.options.length) {
        e.preventDefault();
        if (q.kind === 'choice') pick(q, q.options[n].value);
        else draft = toggle(draft, q, q.options[n].value);
      } else if (n === q.options.length && q.ownKey) {
        e.preventDefault();
        pickOwn(q);
      }
    }
  }

  // Enter in a box or on Submit sends nothing; Submit takes a click or Space.
  function holdEnter(e: KeyboardEvent): void {
    if (e.key === 'Enter') e.preventDefault();
  }

  async function reply(action: 'accept' | 'decline'): Promise<void> {
    if (sending || (action === 'accept' && blocker)) return;
    if (late && action === 'decline') {
      chats.dismissLate(chat.chatId, question.id);
      return;
    }
    sending = true;
    error = undefined;
    try {
      if (late) await chats.sendLate(chat.chatId, question.id, lateMessage(form, draft));
      else await chats.answer(chat.chatId, question.id, action, action === 'accept' ? sent : undefined);
    } catch (err) {
      if (!isStaleReply(err)) error = errorText(err, late ? 'The message was not sent.' : 'The agent no longer waits for this answer.');
    } finally {
      sending = false;
    }
  }

  function defaultText(q: FormQuestion): string | undefined {
    if (q.defaultValue === undefined || q.kind === 'choice' || q.kind === 'multi') return undefined;
    if (q.kind === 'boolean') return q.defaultValue ? 'Yes' : 'No';
    return q.secret ? undefined : String(q.defaultValue);
  }

  function numberHint(q: FormQuestion): string | undefined {
    const whole = q.integer ? 'A whole number' : 'A number';
    if (q.min !== undefined && q.max !== undefined) return `${whole} from ${q.min} to ${q.max}.`;
    if (q.min !== undefined) return `${whole}, at least ${q.min}.`;
    if (q.max !== undefined) return `${whole}, at most ${q.max}.`;
    return undefined;
  }

  function multiHint(q: FormQuestion): string {
    if (q.min !== undefined && q.max !== undefined) return q.min === q.max ? `Choose ${q.min}.` : `Choose ${q.min} to ${q.max}.`;
    if (q.max !== undefined) return `Choose up to ${q.max}.`;
    if (q.min !== undefined) return `Choose at least ${q.min}.`;
    return 'Choose any that apply.';
  }
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<section
  class="question-card"
  class:late
  data-question-card={question.id}
  data-late={late || undefined}
  aria-label={late ? `${agentName} asked` : `${agentName} asks`}
  tabindex="-1"
  bind:this={card}
  onkeydown={onKeydown}
>
  <header>
    <Icon name="circle-help" size={14} />
    <span class="asks">{late ? `${agentName} asked` : `${agentName} asks`}</span>
  </header>
  {#if late}
    <p class="late-note" data-late-note>
      {agentName} went on without your answer. You can still send it as a message.{hasSecret ? ' Hidden answers are not put in a message.' : ''}
    </p>
  {:else if form.message && (several || form.message !== current?.text)}
    <p class="message">{form.message}</p>
  {/if}

  {#if form.questions.length === 0}
    <p class="hint">The question has nothing to answer. Submit or skip it.</p>
  {/if}

  {#if several}
    <div class="tabs" role="tablist" aria-label="Questions">
      {#each form.questions as q, i (q.key)}
        {@const done = answered(q, draft[q.key]) && !problem(q, draft[q.key])}
        {@const compact = q.header === q.text || q.header.length > 36}
        <button type="button" role="tab" class="tab" class:done aria-label={q.header} title={q.text} aria-selected={tab === i} data-tab={q.key} onclick={() => void go(i)}>
          <span class="tab-mark" aria-hidden="true">{#if done}<Icon name="check" size={12} />{:else if !compact}{i + 1}{/if}</span>
          <span class="tab-label">{compact ? `Question ${i + 1}` : q.header}</span>
          {#if done}<span class="sr-only">(answered)</span>{/if}
        </button>
      {/each}
      <button type="button" role="tab" class="tab" aria-selected={reviewing} data-tab="review" onclick={() => void go(form.questions.length)}>
        <span>Review</span>
      </button>
    </div>
  {/if}

  {#if current}
    {@const q = current}
    {@const a = draft[q.key]}
    {@const bad = problem(q, a)}
    <div class="question" role="tabpanel" data-question={q.key}>
      {#if !several && q.header !== q.text}<p class="header">{q.header}</p>{/if}
      <p class="text">
        {q.text}{#if q.required && !late}<span class="required" title="Needs an answer"> *</span>{/if}
      </p>

      {#if q.kind === 'choice'}
        <div class="options" role="radiogroup" aria-label={q.header}>
          {#each q.options as o, i (o.value)}
            {@const on = !a?.ownChosen && a?.choice === o.value}
            <button type="button" role="radio" class="option" class:on aria-checked={on} disabled={sending} onclick={() => pick(q, o.value)}>
              <span class="num">{i + 1}.</span>
              <span class="radio" aria-hidden="true"></span>
              <span class="body">
                <span class="label">{o.label}{#if o.isDefault}<span class="badge">Default</span>{/if}</span>
                {#if o.description}<span class="description">{o.description}</span>{/if}
              </span>
            </button>
          {/each}
          {#if q.ownKey}
            {@const on = !!a?.ownChosen}
            <div class="option own" class:on>
              <button type="button" role="radio" class="own-pick" aria-checked={on} aria-label="Type your own answer" disabled={sending} onclick={() => pickOwn(q)}>
                <span class="num">{q.options.length + 1}.</span>
                <span class="radio" aria-hidden="true"></span>
              </button>
              <input
                type={q.secret ? 'password' : 'text'}
                data-own={q.key}
                placeholder={q.ownPlaceholder}
                aria-label={q.ownPlaceholder}
                value={a?.own ?? ''}
                disabled={sending}
                onfocus={() => (draft = chooseOwn(draft, q.key))}
                oninput={(e) => (draft = setOwn(draft, q, e.currentTarget.value))}
                onkeydown={holdEnter}
              />
            </div>
          {/if}
        </div>
      {:else if q.kind === 'multi'}
        <p class="hint">{multiHint(q)}</p>
        <div class="options" role="group" aria-label={q.header}>
          {#each q.options as o, i (o.value)}
            {@const on = !!a?.picks?.includes(o.value)}
            <button type="button" role="checkbox" class="option" class:on aria-checked={on} disabled={sending} onclick={() => (draft = toggle(draft, q, o.value))}>
              <span class="num">{i + 1}.</span>
              <span class="box" aria-hidden="true">{#if on}<Icon name="check" size={12} />{/if}</span>
              <span class="body">
                <span class="label">{o.label}{#if o.isDefault}<span class="badge">Default</span>{/if}</span>
                {#if o.description}<span class="description">{o.description}</span>{/if}
              </span>
            </button>
          {/each}
          {#if q.ownKey}
            <div class="option own" class:on={!!a?.own?.trim()}>
              <span class="num">{q.options.length + 1}.</span>
              <input
                type={q.secret ? 'password' : 'text'}
                data-own={q.key}
                placeholder={q.ownPlaceholder}
                aria-label={q.ownPlaceholder}
                value={a?.own ?? ''}
                disabled={sending}
                oninput={(e) => (draft = setOwn(draft, q, e.currentTarget.value))}
                onkeydown={holdEnter}
              />
            </div>
          {/if}
        </div>
      {:else if q.kind === 'boolean'}
        <div class="options yes-no" role="radiogroup" aria-label={q.header}>
          {#each [true, false] as yes, i (yes)}
            {@const on = a?.yes === yes}
            <button type="button" role="radio" class="option" class:on aria-checked={on} disabled={sending} onclick={() => pickYes(q, yes)}>
              <span class="num">{i + 1}.</span>
              <span class="radio" aria-hidden="true"></span>
              <span class="label">{yes ? 'Yes' : 'No'}</span>
            </button>
          {/each}
        </div>
      {:else if q.kind === 'text'}
        <input
          class="field"
          type={q.secret ? 'password' : 'text'}
          data-field={q.key}
          aria-label={q.header}
          placeholder="Type your answer"
          value={a?.text ?? ''}
          disabled={sending}
          oninput={(e) => (draft = setText(draft, q.key, e.currentTarget.value))}
          onkeydown={holdEnter}
        />
      {:else if q.kind === 'number'}
        <input
          class="field number"
          type="text"
          inputmode={q.integer ? 'numeric' : 'decimal'}
          data-field={q.key}
          aria-label={q.header}
          placeholder="Type a number"
          value={a?.number ?? ''}
          disabled={sending}
          oninput={(e) => (draft = setNumber(draft, q.key, e.currentTarget.value))}
          onkeydown={holdEnter}
        />
        {#if numberHint(q)}<p class="hint">{numberHint(q)}</p>{/if}
      {/if}

      {#if defaultText(q)}<p class="hint">Default: {defaultText(q)}</p>{/if}
      {#if bad}<p class="problem" role="alert">{bad}</p>{/if}
      {#if answered(q, a)}
        <button type="button" class="link clear" disabled={sending} onclick={() => (draft = clear(draft, q.key))}>Clear answer</button>
      {/if}
    </div>
  {:else if reviewing}
    <div class="review" role="tabpanel" data-review>
      <p class="text">Review your answers</p>
      <ul>
        {#each form.questions as q, i (q.key)}
          {@const a = draft[q.key]}
          {@const shown = answered(q, a) ? answerText(q, sent) : undefined}
          {@const bad = problem(q, a)}
          <li data-review-row={q.key}>
            <span class="review-header">{q.header}</span>
            {#if bad}
              <span class="problem">{bad}</span>
            {:else if shown}
              <span class="review-answer">{shown}</span>
            {:else}
              {@const needed = q.required && !late}
              <span class="unanswered" class:required={needed}>{needed ? 'Not answered — needs an answer' : 'Not answered'}</span>
            {/if}
            <button type="button" class="link" onclick={() => void go(i)}>Change</button>
          </li>
        {/each}
      </ul>
    </div>
  {/if}

  {#if error}<p class="problem" role="alert">Could not send the {late ? 'message' : 'answer'}: {error}</p>{/if}

  <footer>
    {#if blocker && (reviewing || lastQuestion)}<p class="blocker" role="status">{blocker}</p>{/if}
    <div class="actions">
      {#if reviewing || lastQuestion}
        <button type="button" class="submit" disabled={!canSubmit} title={blocker} onkeydown={holdEnter} onclick={() => void reply('accept')}>{late ? 'Send as message' : 'Submit'}</button>
      {:else}
        <button type="button" class="next" onclick={() => void go(tab + 1)}>Next</button>
      {/if}
      <button type="button" class="skip" disabled={sending} onclick={() => void reply('decline')}>{late ? 'Dismiss' : 'Skip'}</button>
      {#if several}<span class="keys">1–9 choose · ← → move between questions</span>{:else if current?.options.length || current?.kind === 'boolean'}<span class="keys">1–9 choose</span>{/if}
    </div>
    {#if waitHint}<p class="hint">{waitHint}</p>{/if}
  </footer>
</section>

<style>
  .question-card {
    display: flex;
    flex-direction: column;
    min-width: 0;
    gap: 8px;
    margin: 0 0 14px 28px;
    padding: 10px 12px;
    border: 1px solid rgba(59, 130, 246, 0.5);
    border-radius: 6px;
    background: rgba(59, 130, 246, 0.07);
    color: var(--text-primary);
    font-size: 0.8rem;
    line-height: 1.4;
    outline: none;
  }

  .question-card.late {
    border-style: dashed;
    border-color: var(--border-color);
    background: var(--bg-secondary);
  }

  .late-note {
    color: var(--text-secondary);
  }

  .question-card:focus-visible {
    box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.35);
  }

  header {
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--text-secondary);
  }

  .asks {
    font-weight: 600;
  }

  p {
    margin: 0;
    overflow-wrap: anywhere;
  }

  .message {
    color: var(--text-secondary);
  }

  .tabs {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    border-bottom: 1px solid var(--border-color);
    padding-bottom: 6px;
  }

  .tab {
    display: inline-flex;
    align-items: center;
    max-width: 190px;
    min-width: 0;
    text-align: left;
    gap: 5px;
    padding: 3px 9px;
    border: 1px solid transparent;
    border-radius: 4px;
    background: transparent;
    color: var(--text-secondary);
    font-size: 0.75rem;
    cursor: pointer;
  }

  .tab-label {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .tab:focus-visible,
  .option:focus-visible,
  .own-pick:focus-visible {
    outline: 2px solid #3b82f6;
    outline-offset: 2px;
  }

  .tab[aria-selected='true'] {
    border-color: rgba(59, 130, 246, 0.6);
    background: var(--bg-secondary);
    color: var(--text-primary);
  }

  .tab-mark {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 14px;
    flex-shrink: 0;
    color: var(--text-muted);
    font-size: 0.68rem;
  }

  .tab.done .tab-mark {
    color: #16a34a;
  }

  .question,
  .review {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .header {
    color: var(--text-muted);
    font-size: 0.72rem;
    text-transform: uppercase;
    letter-spacing: 0.03em;
  }

  .text {
    font-weight: 600;
  }

  .required {
    color: #dc2626;
  }

  .options {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .yes-no {
    flex-direction: row;
  }

  .option {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    padding: 8px 10px;
    border: 1px solid var(--border-color);
    border-radius: 4px;
    background: var(--bg-secondary);
    color: var(--text-primary);
    font-size: 0.78rem;
    text-align: left;
    cursor: pointer;
  }

  .option.on {
    border-color: rgba(59, 130, 246, 0.8);
    background: rgba(59, 130, 246, 0.14);
  }

  .option:disabled {
    opacity: 0.5;
    cursor: default;
  }

  button.option:hover:not(:disabled) {
    filter: brightness(1.08);
  }

  .option.own {
    align-items: center;
    cursor: default;
  }

  .own-pick {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    align-self: stretch;
    padding: 0;
    border: none;
    background: none;
    color: inherit;
    cursor: pointer;
  }

  .num {
    min-width: 1.4em;
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
    flex-shrink: 0;
  }

  .radio {
    box-sizing: border-box;
    flex: 0 0 14px;
    width: 14px;
    height: 14px;
    margin-top: 2px;
    border: 1px solid var(--text-muted);
    border-radius: 50%;
    background: var(--bg-primary);
  }

  [aria-checked='true'] > .radio {
    border: 4px solid #3b82f6;
  }

  .box {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 14px;
    height: 14px;
    margin-top: 1px;
    border: 1px solid var(--border-color);
    border-radius: 3px;
    background: var(--bg-primary);
    color: #16a34a;
  }

  .body {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .label {
    overflow-wrap: anywhere;
  }

  .badge {
    margin-left: 6px;
    padding: 0 5px;
    border: 1px solid var(--border-color);
    border-radius: 3px;
    color: var(--text-muted);
    font-size: 0.66rem;
  }

  .description {
    color: var(--text-muted);
    font-size: 0.74rem;
  }

  input {
    flex: 1;
    min-width: 0;
    padding: 4px 8px;
    border: 1px solid var(--border-color);
    border-radius: 4px;
    background: var(--bg-primary);
    color: var(--text-primary);
    font-size: 0.78rem;
  }

  .option.own input {
    padding: 2px 6px;
  }

  .field.number {
    max-width: 12em;
  }

  .hint,
  .keys {
    color: var(--text-muted);
    font-size: 0.72rem;
  }

  .keys {
    margin-left: auto;
  }

  .problem,
  .blocker {
    color: #dc2626;
    font-size: 0.74rem;
  }

  .blocker {
    color: var(--text-secondary);
  }

  .review ul {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .review li {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 8px;
  }

  .review-header {
    min-width: 10em;
    color: var(--text-secondary);
  }

  .review-answer {
    overflow-wrap: anywhere;
  }

  .unanswered {
    color: var(--text-muted);
    font-style: italic;
  }

  .unanswered.required {
    color: #dc2626;
  }

  .link {
    padding: 0;
    border: none;
    background: none;
    color: var(--accent-color, #3b82f6);
    font-size: 0.74rem;
    cursor: pointer;
  }

  .link:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .clear {
    align-self: flex-start;
  }

  footer {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
  }

  .actions button {
    padding: 5px 12px;
    border: 1px solid var(--border-color);
    border-radius: 4px;
    background: var(--bg-secondary);
    color: var(--text-primary);
    font-size: 0.78rem;
    cursor: pointer;
  }

  .actions .submit {
    border-color: rgba(59, 130, 246, 0.7);
    background: rgba(59, 130, 246, 0.18);
  }

  .actions button:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .actions button:hover:not(:disabled) {
    filter: brightness(1.08);
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
    white-space: nowrap;
  }
</style>
