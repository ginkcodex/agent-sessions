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
    <span class="chip" aria-hidden="true"><Icon name="circle-help" size={14} /></span>
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
      <button type="button" role="tab" class="tab review-tab" aria-selected={reviewing} data-tab="review" onclick={() => void go(form.questions.length)}>
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
              <span class="num">{i + 1}<span class="sr-only">.</span></span>
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
                <span class="sr-only">{q.options.length + 1}.</span>
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
              <span class="num" aria-hidden="true">{q.options.length + 1}</span>
            </div>
          {/if}
        </div>
      {:else if q.kind === 'multi'}
        <p class="hint">{multiHint(q)}</p>
        <div class="options" role="group" aria-label={q.header}>
          {#each q.options as o, i (o.value)}
            {@const on = !!a?.picks?.includes(o.value)}
            <button type="button" role="checkbox" class="option" class:on aria-checked={on} disabled={sending} onclick={() => (draft = toggle(draft, q, o.value))}>
              <span class="num">{i + 1}<span class="sr-only">.</span></span>
              <span class="box" aria-hidden="true">{#if on}<Icon name="check" size={12} />{/if}</span>
              <span class="body">
                <span class="label">{o.label}{#if o.isDefault}<span class="badge">Default</span>{/if}</span>
                {#if o.description}<span class="description">{o.description}</span>{/if}
              </span>
            </button>
          {/each}
          {#if q.ownKey}
            <div class="option own" class:on={!!a?.own?.trim()}>
              <span class="box ghost" aria-hidden="true"></span>
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
              <span class="num" aria-hidden="true">{q.options.length + 1}</span>
            </div>
          {/if}
        </div>
      {:else if q.kind === 'boolean'}
        <div class="options yes-no" role="radiogroup" aria-label={q.header}>
          {#each [true, false] as yes, i (yes)}
            {@const on = a?.yes === yes}
            <button type="button" role="radio" class="option" class:on aria-checked={on} disabled={sending} onclick={() => pickYes(q, yes)}>
              <span class="num">{i + 1}<span class="sr-only">.</span></span>
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
      {#if several}
        <span class="keys"><kbd>1</kbd>–<kbd>9</kbd> choose · <kbd>←</kbd><kbd>→</kbd> move between questions</span>
      {:else if current?.options.length || current?.kind === 'boolean'}
        <span class="keys"><kbd>1</kbd>–<kbd>9</kbd> choose</span>
      {/if}
      <button type="button" class="skip" disabled={sending} onclick={() => void reply('decline')}>{late ? 'Dismiss' : 'Skip'}</button>
      {#if reviewing || lastQuestion}
        <button type="button" class="primary submit" disabled={!canSubmit} title={blocker} onkeydown={holdEnter} onclick={() => void reply('accept')}>{late ? 'Send as message' : 'Submit'}</button>
      {:else}
        <button type="button" class="primary next" onclick={() => void go(tab + 1)}>Next</button>
      {/if}
    </div>
    {#if waitHint}<p class="hint">{waitHint}</p>{/if}
  </footer>
</section>

<style>
  /* Sizes follow the session's text size, as the messages around it do. */
  .question-card {
    --fs: var(--session-font-size, 14.5px);
    --done: #16a34a;
    display: flex;
    flex-direction: column;
    min-width: 0;
    gap: 12px;
    margin: 0 0 16px 28px;
    padding: 14px 16px 12px;
    border: 1px solid color-mix(in srgb, var(--accent-color) 30%, var(--border-color));
    border-radius: 14px;
    background: var(--bg-secondary);
    box-shadow: var(--shadow-card);
    color: var(--text-primary);
    font-size: calc(var(--fs) * var(--type-body));
    line-height: 1.5;
    outline: none;
  }

  .question-card:focus-visible {
    box-shadow: var(--shadow-card), 0 0 0 2px color-mix(in srgb, var(--accent-color) 22%, transparent);
  }

  .question-card.late {
    border-style: dashed;
    border-color: var(--border-strong);
    box-shadow: none;
  }

  p {
    margin: 0;
    overflow-wrap: anywhere;
  }

  /* Header: the icon in a small accent chip, then who asks. */
  header {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--text-secondary);
    font-size: calc(var(--fs) * var(--type-small));
  }

  .chip {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    border-radius: 8px;
    background: color-mix(in srgb, var(--accent-color) 14%, transparent);
    color: var(--accent-color);
  }

  .late .chip {
    background: var(--segment-track);
    color: var(--text-muted);
  }

  .asks {
    font-weight: 600;
    color: var(--text-primary);
  }

  .message,
  .late-note {
    margin-top: -4px;
    color: var(--text-secondary);
    font-size: calc(var(--fs) * var(--type-small));
  }

  /* Tabs: a segmented track, as the transcript mode switch. */
  .tabs {
    display: flex;
    flex-wrap: wrap;
    align-self: flex-start;
    max-width: 100%;
    gap: 2px;
    padding: 3px;
    border-radius: 10px;
    background: var(--segment-track);
    box-sizing: border-box;
  }

  .tab {
    display: inline-flex;
    align-items: center;
    max-width: 200px;
    min-width: 0;
    gap: 6px;
    padding: 4px 10px 4px 6px;
    border: none;
    border-radius: 7px;
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    font-size: calc(var(--fs) * var(--type-small));
    text-align: left;
    cursor: pointer;
    transition: background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out);
  }

  .tab:hover {
    color: var(--text-primary);
  }

  .tab[aria-selected='true'] {
    background: var(--segment-thumb);
    color: var(--text-primary);
    box-shadow: var(--shadow-thumb);
  }

  /* The Review tab has no mark. */
  .tab.review-tab {
    padding-left: 10px;
  }

  .tab-label {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .tab-mark {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    min-width: 18px;
    height: 18px;
    border-radius: 999px;
    background: var(--bg-primary);
    box-shadow: inset 0 0 0 1px var(--border-strong);
    color: var(--text-muted);
    font-size: calc(var(--fs) * var(--type-tag));
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }

  /* A long title's tab shows "Question n" with no number in the mark. */
  .tab-mark:empty {
    display: none;
  }

  .tab[aria-selected='true'] .tab-mark {
    background: var(--accent-color);
    box-shadow: none;
    color: white;
  }

  .tab.done .tab-mark {
    background: var(--done);
    box-shadow: none;
    color: white;
  }

  .tab:focus-visible,
  .option:focus-visible,
  .own-pick:focus-visible,
  .actions button:focus-visible,
  .link:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: 2px;
  }

  .question,
  .review {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .header {
    color: var(--accent-color);
    font-size: calc(var(--fs) * var(--type-tag));
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }

  .text {
    font-weight: 600;
    line-height: 1.45;
  }

  .required {
    color: var(--danger);
  }

  /* Options: rounded rows, the mark on the left and the key at the end. */
  .options {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .yes-no {
    flex-direction: row;
    flex-wrap: wrap;
  }

  .yes-no .option {
    flex: 1 1 140px;
  }

  .option {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    padding: 9px 10px 9px 12px;
    border: 1px solid var(--border-color);
    border-radius: 10px;
    background: var(--bg-primary);
    color: var(--text-primary);
    font: inherit;
    text-align: left;
    cursor: pointer;
    transition: border-color var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out),
      box-shadow var(--dur-fast) var(--ease-out);
  }

  button.option:hover:not(:disabled),
  .option.own:hover {
    border-color: color-mix(in srgb, var(--accent-color) 45%, var(--border-color));
  }

  .option.on {
    border-color: var(--accent-color);
    background: color-mix(in srgb, var(--accent-color) 9%, var(--bg-primary));
    box-shadow: inset 0 0 0 1px var(--accent-color);
  }

  .option:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .option.own {
    align-items: center;
    padding-top: 5px;
    padding-bottom: 5px;
    cursor: text;
  }

  .own-pick {
    display: inline-flex;
    align-items: center;
    align-self: stretch;
    padding: 0;
    border: none;
    border-radius: 50%;
    background: none;
    color: inherit;
    cursor: pointer;
  }

  /* The 1–9 key, as a keycap at the row's end. */
  .num {
    order: 3;
    align-self: center;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    min-width: 20px;
    height: 20px;
    margin-left: auto;
    padding: 0 4px;
    border: 1px solid var(--border-color);
    border-bottom-width: 2px;
    border-radius: 6px;
    background: var(--bg-secondary);
    color: var(--text-muted);
    font-family: var(--font-mono);
    font-size: calc(var(--fs) * var(--type-tag));
    line-height: 1;
    box-sizing: border-box;
  }

  .option.on .num {
    border-color: color-mix(in srgb, var(--accent-color) 60%, transparent);
    color: var(--accent-color);
  }

  .radio,
  .box {
    box-sizing: border-box;
    flex: 0 0 auto;
    width: 16px;
    height: 16px;
    margin-top: calc((1.5em - 16px) / 2);
    border: 1.5px solid var(--border-strong);
    background: var(--bg-primary);
    transition: border var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out);
  }

  .radio {
    border-radius: 50%;
  }

  .own .radio,
  .own .box {
    margin-top: 0;
  }

  [aria-checked='true'] > .radio {
    border: 5px solid var(--accent-color);
  }

  .box {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border-radius: 4px;
    color: white;
  }

  [aria-checked='true'] > .box {
    border-color: var(--accent-color);
    background: var(--accent-color);
  }

  .box.ghost {
    border-style: dashed;
  }

  .body {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
  }

  .label {
    overflow-wrap: anywhere;
  }

  .badge {
    display: inline-block;
    margin-left: 8px;
    padding: 0 7px;
    border-radius: 999px;
    background: var(--segment-track);
    color: var(--text-secondary);
    font-size: calc(var(--fs) * var(--type-tag));
    font-weight: 500;
    line-height: 1.6;
    vertical-align: 1px;
  }

  .description {
    color: var(--text-muted);
    font-size: calc(var(--fs) * var(--type-small));
    line-height: 1.45;
  }

  input {
    flex: 1;
    min-width: 0;
    padding: 6px 10px;
    border: 1px solid var(--border-color);
    border-radius: 10px;
    background: var(--bg-primary);
    color: var(--text-primary);
    font: inherit;
    transition: border-color var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out);
  }

  input::placeholder {
    color: var(--text-muted);
  }

  input:focus {
    outline: none;
    border-color: var(--accent-color);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent-color) 18%, transparent);
  }

  /* In its row the own answer's box is the row. */
  .option.own input,
  .option.own input:focus {
    padding: 4px 0;
    border: none;
    background: transparent;
    box-shadow: none;
  }

  .field.number {
    max-width: 12em;
  }

  .hint,
  .keys {
    color: var(--text-muted);
    font-size: calc(var(--fs) * var(--type-small));
  }

  .problem {
    color: var(--danger);
    font-size: calc(var(--fs) * var(--type-small));
  }

  .blocker {
    color: var(--text-secondary);
    font-size: calc(var(--fs) * var(--type-small));
  }

  /* Review: a row per question, divided by thin lines. */
  .review ul {
    margin: 0;
    padding: 0;
    list-style: none;
    border: 1px solid var(--border-color);
    border-radius: 10px;
    background: var(--bg-primary);
    overflow: hidden;
  }

  .review li {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 4px 12px;
    padding: 8px 12px;
  }

  .review li + li {
    border-top: 1px solid var(--border-color);
  }

  .review-header {
    flex: 0 0 11em;
    min-width: 0;
    color: var(--text-secondary);
    font-size: calc(var(--fs) * var(--type-small));
  }

  .review-answer {
    flex: 1;
    min-width: 0;
    overflow-wrap: anywhere;
  }

  .review li > .problem,
  .unanswered {
    flex: 1;
    min-width: 0;
  }

  .unanswered {
    color: var(--text-muted);
    font-style: italic;
  }

  .unanswered.required {
    color: var(--danger);
  }

  .link {
    padding: 0;
    border: none;
    background: none;
    color: var(--accent-color);
    font: inherit;
    font-size: calc(var(--fs) * var(--type-small));
    font-weight: 500;
    cursor: pointer;
  }

  .link:hover:not(:disabled) {
    text-decoration: underline;
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
    gap: 8px;
    padding-top: 10px;
    border-top: 1px solid var(--border-color);
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
  }

  .keys {
    margin-right: auto;
  }

  kbd {
    display: inline-block;
    min-width: 1.5em;
    margin: 0 1px;
    padding: 0 4px;
    border: 1px solid var(--border-color);
    border-bottom-width: 2px;
    border-radius: 5px;
    background: var(--bg-primary);
    font-family: var(--font-mono);
    font-size: calc(var(--fs) * var(--type-tag));
    line-height: 1.5;
    text-align: center;
    box-sizing: border-box;
  }

  /* Without a key hint, the buttons still sit at the end. */
  .actions > button:first-child {
    margin-left: auto;
  }

  .actions button {
    padding: 6px 16px;
    border: 1px solid transparent;
    border-radius: 9px;
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    font-size: calc(var(--fs) * var(--type-small));
    font-weight: 600;
    cursor: pointer;
    transition: background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out),
      filter var(--dur-fast) var(--ease-out);
  }

  .actions button:hover:not(:disabled) {
    background: var(--hover-bg);
    color: var(--text-primary);
  }

  .actions .primary {
    background: var(--accent-color);
    color: white;
    box-shadow: 0 1px 2px color-mix(in srgb, var(--accent-color) 35%, transparent);
  }

  .actions .primary:hover:not(:disabled) {
    background: var(--accent-hover);
    color: white;
  }

  .actions button:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    margin: -1px;
    padding: 0;
    border: 0;
    overflow: hidden;
    clip: rect(0 0 0 0);
    white-space: nowrap;
  }
</style>
