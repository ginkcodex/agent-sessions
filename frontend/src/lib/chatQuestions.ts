// An agent's question (an ACP form elicitation) as the questions the user
// reads, the answers they are drafting, and the content sent back. The three
// adapters shape their forms differently
// (docs/plan/chat-questions-adapters.md); the pairing here knows each way.
// Every function is pure.
import type { ChatQuestion, ChatQuestionField, ChatQuestionOption } from './types';

export type QuestionKind = 'choice' | 'multi' | 'text' | 'boolean' | 'number';

export interface QuestionChoice {
  value: string;
  label: string;
  description?: string;
  /** The form's default: marked, never preselected. */
  isDefault: boolean;
}

/** A text field's limits. */
export interface TextLimits {
  minLength?: number;
  maxLength?: number;
  pattern?: string;
}

export interface FormQuestion {
  /** The field the answer goes in. */
  key: string;
  kind: QuestionKind;
  /** The short name, for a tab ("Extras to include"). */
  header: string;
  /** The question itself. */
  text: string;
  /** A single choice's or a multi-select's options, as shown. */
  options: QuestionChoice[];
  required: boolean;
  /** The field of the "Type your own answer" text, when the question has one. */
  ownKey?: string;
  /** What the own answer's box says. */
  ownPlaceholder?: string;
  ownLimits?: TextLimits;
  /**
   * The option the own answer stands for, sent with it (Codex's "Other",
   * which is not shown).
   */
  ownValue?: string;
  /** A text field's limits. */
  limits?: TextLimits;
  /** A multi-select's count limits, or a number's bounds. */
  min?: number;
  max?: number;
  integer?: boolean;
  /** The default, shown as a hint. */
  defaultValue?: unknown;
  /** The answer is a secret (Codex's isSecret): typed hidden, never read back. */
  secret?: boolean;
}

export interface QuestionForm {
  id: string;
  /** What the agent says above the questions. */
  message: string;
  questions: FormQuestion[];
}

/** The user's answer to one question, as drafted. */
export interface DraftAnswer {
  /** A single choice's option. */
  choice?: string;
  /** A multi-select's options, in the form's order. */
  picks?: string[];
  /** A text answer. */
  text?: string;
  yes?: boolean;
  /** A number as typed. */
  number?: string;
  /** The own answer's text. */
  own?: string;
  /** For a single choice: the own answer is the one chosen. */
  ownChosen?: boolean;
}

export type Draft = Readonly<Record<string, DraftAnswer>>;

const OWN_SUFFIX = '_custom';
// codex-acp 2.6.0 uses “None of the above”; also accept the legacy “Other” shape.
const CODEX_OTHER = ['None of the above', 'Other'];

function codexMeta(f: ChatQuestionField): Record<string, unknown> | undefined {
  const codex = f._meta?.codex;
  return codex && typeof codex === 'object' ? (codex as Record<string, unknown>) : undefined;
}

function choicesOf(f: ChatQuestionField): ChatQuestionOption[] {
  if (f.type === 'string') return f.oneOf?.length ? f.oneOf : (f.enum ?? []).map((v) => ({ const: v, title: v }));
  if (f.type === 'array') return f.items?.anyOf?.length ? f.items.anyOf : (f.items?.enum ?? []).map((v) => ({ const: v, title: v }));
  return [];
}

function kindOf(f: ChatQuestionField): QuestionKind | undefined {
  switch (f.type) {
    case 'string':
      return f.oneOf?.length || f.enum?.length ? 'choice' : 'text';
    case 'array':
      return 'multi';
    case 'boolean':
      return 'boolean';
    case 'number':
    case 'integer':
      return 'number';
  }
  return undefined;
}

function isPlainText(f: ChatQuestionField | undefined): f is ChatQuestionField {
  return !!f && kindOf(f) === 'text';
}

function textLimits(f: ChatQuestionField): TextLimits | undefined {
  const l: TextLimits = {
    ...(f.minLength !== undefined && { minLength: f.minLength }),
    ...(f.maxLength !== undefined && { maxLength: f.maxLength }),
    ...(f.pattern && { pattern: f.pattern }),
  };
  return Object.keys(l).length > 0 ? l : undefined;
}

/**
 * The own-answer field of each choice, by the choice's key:
 * - Codex: a note field whose _meta.codex.role is user_note names its
 *   question; the choice's "Other" option is what the note stands for.
 * - Claude Code and OpenCode: `<key>_custom` next to `<key>`.
 * A required choice is paired only when the own answer can stand for it
 * (Codex's "Other"); otherwise its text field stays a question of its own.
 */
function pairs(fields: readonly ChatQuestionField[], required: ReadonlySet<string>): Map<string, { own: ChatQuestionField; ownValue?: string }> {
  const byKey = new Map(fields.map((f) => [f.key, f]));
  const out = new Map<string, { own: ChatQuestionField; ownValue?: string }>();
  const taken = new Set<string>();
  for (const f of fields) {
    const codex = codexMeta(f);
    if (codex?.role !== 'user_note' || typeof codex.questionId !== 'string') continue;
    const q = byKey.get(codex.questionId);
    if (!q || !isPlainText(f) || out.has(q.key)) continue;
    const kind = kindOf(q);
    if (kind === 'choice') {
      const other = CODEX_OTHER.find((value) => choicesOf(q).some((o) => o.const === value));
      if (required.has(q.key) && !other) continue;
      out.set(q.key, { own: f, ownValue: other });
    } else if (kind === 'multi' && !required.has(q.key)) {
      out.set(q.key, { own: f });
    } else continue;
    taken.add(f.key);
  }
  for (const f of fields) {
    if (!f.key.endsWith(OWN_SUFFIX) || taken.has(f.key) || !isPlainText(f) || required.has(f.key)) continue;
    const q = byKey.get(f.key.slice(0, -OWN_SUFFIX.length));
    const kind = q && kindOf(q);
    if (!q || (kind !== 'choice' && kind !== 'multi') || out.has(q.key) || required.has(q.key)) continue;
    out.set(q.key, { own: f });
    taken.add(f.key);
  }
  return out;
}

/** The question's form as the questions the user reads, in order. */
export function questionForm(q: ChatQuestion): QuestionForm {
  const fields = q.fields ?? [];
  const required = new Set(q.required ?? []);
  const paired = pairs(fields, required);
  const owns = new Set([...paired.values()].map((p) => p.own.key));
  const questions: FormQuestion[] = [];
  for (const f of fields) {
    if (owns.has(f.key)) continue;
    const kind = kindOf(f);
    if (!kind) continue;
    // Codex sends the question as the title and the header as the
    // description; Claude Code and OpenCode the other way round.
    const title = f.title?.trim() || '';
    const description = f.description?.trim() || '';
    const swapped = !!codexMeta(f);
    const header = (swapped ? description || title : title || description) || f.key;
    const text = (swapped ? title || description : description || title) || f.key;
    const pair = paired.get(f.key);
    const defaults = f.default === undefined ? [] : Array.isArray(f.default) ? f.default.map(String) : [String(f.default)];
    const options = choicesOf(f)
      .filter((o) => !(pair?.ownValue !== undefined && o.const === pair.ownValue))
      .map((o): QuestionChoice => ({
        value: o.const,
        label: o.title || o.const,
        ...(o.description && { description: o.description }),
        isDefault: kind !== 'boolean' && defaults.includes(o.const),
      }));
    const question: FormQuestion = { key: f.key, kind, header, text, options, required: required.has(f.key) };
    if (pair) {
      question.ownKey = pair.own.key;
      // Codex's note has only a title ("Additional answer or note").
      question.ownPlaceholder = pair.own.description?.trim() || (pair.ownValue !== undefined && pair.own.title?.trim()) || 'Type your own answer';
      const limits = textLimits(pair.own);
      if (limits) question.ownLimits = limits;
      if (pair.ownValue !== undefined) question.ownValue = pair.ownValue;
    }
    if (kind === 'text') {
      const limits = textLimits(f);
      if (limits) question.limits = limits;
    }
    if (kind === 'multi') {
      if (f.minItems !== undefined) question.min = f.minItems;
      if (f.maxItems !== undefined) question.max = f.maxItems;
    }
    if (kind === 'number') {
      if (f.minimum !== undefined) question.min = f.minimum;
      if (f.maximum !== undefined) question.max = f.maximum;
      question.integer = f.type === 'integer';
    }
    if (f.default !== undefined) question.defaultValue = f.default;
    if (codexMeta(f)?.isSecret === true) question.secret = true;
    questions.push(question);
  }
  return { id: q.id, message: q.message?.trim() || q.title?.trim() || '', questions };
}

// Drafting. Each returns a new draft.

function set(draft: Draft, key: string, fn: (a: DraftAnswer) => DraftAnswer): Draft {
  return { ...draft, [key]: fn(draft[key] ?? {}) };
}

/** Picks a single choice's option; it replaces the own answer as the choice. */
export function choose(draft: Draft, key: string, value: string): Draft {
  return set(draft, key, (a) => ({ ...a, choice: value, ownChosen: false }));
}

/** Picks or unpicks a multi-select's option, keeping the form's order. */
export function toggle(draft: Draft, q: FormQuestion, value: string): Draft {
  return set(draft, q.key, (a) => {
    const picked = new Set(a.picks ?? []);
    if (picked.has(value)) picked.delete(value);
    else picked.add(value);
    return { ...a, picks: q.options.map((o) => o.value).filter((v) => picked.has(v)) };
  });
}

/** Sets the own answer's text. For a single choice, it becomes the choice. */
export function setOwn(draft: Draft, q: FormQuestion, text: string): Draft {
  return set(draft, q.key, (a) => ({ ...a, own: text, ...(q.kind === 'choice' && { ownChosen: true }) }));
}

/** Makes a single choice's own answer the choice, before anything is typed. */
export function chooseOwn(draft: Draft, key: string): Draft {
  return set(draft, key, (a) => ({ ...a, ownChosen: true }));
}

export function setText(draft: Draft, key: string, text: string): Draft {
  return set(draft, key, (a) => ({ ...a, text }));
}

export function setYes(draft: Draft, key: string, yes: boolean): Draft {
  return set(draft, key, (a) => ({ ...a, yes }));
}

export function setNumber(draft: Draft, key: string, number: string): Draft {
  return set(draft, key, (a) => ({ ...a, number }));
}

/** Clears a question's answer. */
export function clear(draft: Draft, key: string): Draft {
  const { [key]: _, ...rest } = draft;
  return rest;
}

// Reading a draft.

function filled(s: string | undefined): s is string {
  return !!s && s.trim() !== '';
}

/** The question has an answer, valid or not. */
export function answered(q: FormQuestion, a: DraftAnswer | undefined): boolean {
  if (!a) return false;
  switch (q.kind) {
    case 'choice':
      return a.ownChosen ? filled(a.own) : a.choice !== undefined;
    case 'multi':
      return (a.picks?.length ?? 0) > 0 || filled(a.own);
    case 'text':
      return filled(a.text);
    case 'boolean':
      return a.yes !== undefined;
    case 'number':
      return filled(a.number);
  }
}

function textProblem(text: string, l: TextLimits | undefined): string | undefined {
  if (!l) return undefined;
  const n = [...text].length;
  if (l.minLength !== undefined && n < l.minLength) return `Needs at least ${l.minLength} character${l.minLength === 1 ? '' : 's'}.`;
  if (l.maxLength !== undefined && n > l.maxLength) return `At most ${l.maxLength} character${l.maxLength === 1 ? '' : 's'}.`;
  if (l.pattern) {
    let re: RegExp | undefined;
    try {
      re = new RegExp(l.pattern, 'u');
    } catch {
      // The backend checks it with Go's syntax.
    }
    if (re && !re.test(text)) return 'Does not have the expected form.';
  }
  return undefined;
}

/** Why the answer cannot be sent, or undefined. An empty answer is not a problem here. */
export function problem(q: FormQuestion, a: DraftAnswer | undefined): string | undefined {
  if (!a) return undefined;
  switch (q.kind) {
    case 'choice':
      return a.ownChosen && filled(a.own) ? textProblem(a.own, q.ownLimits) : undefined;
    case 'multi': {
      const n = a.picks?.length ?? 0;
      if (n > 0 && q.min !== undefined && n < q.min) return `Choose at least ${q.min}.`;
      if (q.max !== undefined && n > q.max) return `Choose at most ${q.max}.`;
      return filled(a.own) ? textProblem(a.own, q.ownLimits) : undefined;
    }
    case 'text':
      return filled(a.text) ? textProblem(a.text, q.limits) : undefined;
    case 'boolean':
      return undefined;
    case 'number': {
      if (!filled(a.number)) return undefined;
      const v = Number(a.number.trim());
      if (!Number.isFinite(v)) return 'Enter a number.';
      if (q.integer && !Number.isInteger(v)) return 'Enter a whole number.';
      if (q.min !== undefined && v < q.min) return `At least ${q.min}.`;
      if (q.max !== undefined && v > q.max) return `At most ${q.max}.`;
      return undefined;
    }
  }
}

/** The required questions still without an answer, in order. */
export function missing(form: QuestionForm, draft: Draft): FormQuestion[] {
  return form.questions.filter((q) => q.required && !answered(q, draft[q.key]));
}

/** The questions whose answers cannot be sent, with why. */
export function problems(form: QuestionForm, draft: Draft): { question: FormQuestion; problem: string }[] {
  const out: { question: FormQuestion; problem: string }[] = [];
  for (const q of form.questions) {
    const p = problem(q, draft[q.key]);
    if (p) out.push({ question: q, problem: p });
  }
  return out;
}

/** Why Submit is not possible yet, or undefined when it is. */
export function submitBlocker(form: QuestionForm, draft: Draft): string | undefined {
  const bad = problems(form, draft)[0];
  if (bad) return `${bad.question.header}: ${bad.problem}`;
  const open = missing(form, draft);
  if (open.length === 1) return `${open[0].header} needs an answer.`;
  if (open.length > 1) return `${open.length} questions need an answer: ${open.map((q) => q.header).join(', ')}.`;
  return undefined;
}

/**
 * Why a late answer (a question the agent went on without) cannot be sent as
 * a message yet. Required questions do not block it; it needs one answer, and
 * a hidden answer does not count, since it is never put in a message.
 */
export function lateBlocker(form: QuestionForm, draft: Draft): string | undefined {
  const bad = problems(form, draft)[0];
  if (bad) return `${bad.question.header}: ${bad.problem}`;
  if (!form.questions.some((q) => !q.secret && answered(q, draft[q.key]))) return 'Answer a question to send it.';
  return undefined;
}

/**
 * A late answer as the message sent to the agent: each answered question and
 * its answer. Hidden answers are left out: a message is saved in the
 * transcript.
 */
export function lateMessage(form: QuestionForm, draft: Draft): string {
  const sent = content(form, draft);
  const lines = form.questions.flatMap((q) => {
    if (q.secret) return [];
    const a = answerText(q, sent);
    if (!a) return [];
    const asked = q.text.replace(/\s+/g, ' ').trim() || q.header;
    return [`- ${asked} — ${a}`];
  });
  return `${lines.length === 1 ? 'My answer to your question' : 'My answers to your questions'}:\n${lines.join('\n')}`;
}

/** The content to send for the draft: only the answered fields, in the form's shape. */
export function content(form: QuestionForm, draft: Draft): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const q of form.questions) {
    const a = draft[q.key];
    if (!a || !answered(q, a)) continue;
    switch (q.kind) {
      case 'choice':
        if (a.ownChosen && q.ownKey) {
          if (q.ownValue !== undefined) out[q.key] = q.ownValue;
          out[q.ownKey] = a.own;
        } else if (a.choice !== undefined) {
          out[q.key] = a.choice;
        }
        break;
      case 'multi':
        if (a.picks?.length) out[q.key] = a.picks;
        if (q.ownKey && filled(a.own)) out[q.ownKey] = a.own;
        break;
      case 'text':
        out[q.key] = a.text;
        break;
      case 'boolean':
        out[q.key] = a.yes;
        break;
      case 'number':
        out[q.key] = Number(a.number!.trim());
        break;
    }
  }
  return out;
}

/** One question's answer as read back, from sent content; undefined when unanswered. */
export function answerText(q: FormQuestion, sent: Readonly<Record<string, unknown>>): string | undefined {
  const own = q.ownKey ? sent[q.ownKey] : undefined;
  const ownText = typeof own === 'string' && own.trim() ? own.trim() : undefined;
  const label = (v: unknown) => q.options.find((o) => o.value === v)?.label ?? String(v);
  const v = sent[q.key];
  if (q.secret && (v !== undefined || ownText)) return '(hidden)';
  switch (q.kind) {
    case 'choice':
      if (ownText && (v === undefined || v === q.ownValue)) return ownText;
      return v === undefined ? ownText : label(v);
    case 'multi': {
      const parts = Array.isArray(v) ? v.map(label) : [];
      if (ownText) parts.push(ownText);
      return parts.length > 0 ? parts.join(', ') : undefined;
    }
    case 'boolean':
      return v === undefined ? undefined : v ? 'Yes' : 'No';
    default:
      return v === undefined || v === '' ? undefined : String(v);
  }
}

/**
 * The agent keeps working while the question is open: a Codex async question
 * (`_meta.codex.delivery` "async"), withdrawn when the turn ends.
 */
export function asksWhileWorking(q: ChatQuestion): boolean {
  const codex = q.meta?.codex;
  return !!codex && typeof codex === 'object' && (codex as Record<string, unknown>).delivery === 'async';
}

/** The line shown in place of a resolved question. */
export function summary(form: QuestionForm, outcome: string | undefined, sent: Readonly<Record<string, unknown>> | undefined): string {
  switch (outcome) {
    case 'accept': {
      const parts = form.questions.map((q) => answerText(q, sent ?? {})).filter((s): s is string => !!s);
      return parts.length > 0 ? `Answered: ${parts.join(' · ')}` : 'Answered without choosing anything';
    }
    case 'decline':
      return 'Skipped the question';
    case 'withdrawn':
      return 'The agent went on without an answer';
    default:
      return 'The question was cancelled';
  }
}
