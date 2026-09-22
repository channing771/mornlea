//! Bounded chat intent payload.
//!
//! The chat channel carries the player's own text and nothing else. Parsing a
//! `/warp` namespace, resolving an `@name` companion address, deciding whether
//! a task stops, and queueing the result are authority decisions over the
//! session and the companion set, so none of them belongs to a payload the
//! client constructs. Keeping them out is what lets the text be retained
//! verbatim: a mention that the authority later rejects still has to reach it
//! exactly as typed.

use crate::text::CommandText;

/// One chat intent: the player's bounded command text, retained verbatim.
///
/// The text is already a `CommandText`, so the rule that decides whether the
/// text is publishable ran before this record existed and construction is total
/// and named `new` rather than `try_new`. The record carries no sequence: chat
/// travels on its own bounded channel rather than through the sequenced command
/// stream, so a payload that named a sequence would claim an admission order it
/// does not have.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ChatIntent {
    text: CommandText,
}

impl ChatIntent {
    pub fn new(text: CommandText) -> Self {
        Self { text }
    }

    pub fn text(&self) -> &CommandText {
        &self.text
    }
}
