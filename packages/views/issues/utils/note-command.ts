// `/note ...` is a record-only comment: it never wakes an agent and never
// routes to a person. Composers and the trigger preview both need to recognise
// it, so it lives here rather than inside the preview hook.
const NOTE_COMMAND_RE = /^\/note(?:$|\s)/i;

/** True when the draft is a `/note` comment (leading whitespace tolerated). */
export function isNoteCommentDraft(content: string): boolean {
  return NOTE_COMMAND_RE.test(content.replace(/^[ \t\r\n]+/, ""));
}
