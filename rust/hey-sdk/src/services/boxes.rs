//! Resolving a box by its kind, and gathering a selection of postings into a Set Aside
//! group.
//!
//! [`Boxes::get_imbox_seen`] answers the Imbox's Previously Seen postings, ordered by when
//! they were seen. Its `next_history_url` names the `/imbox` route, but the cursor in it
//! belongs to the seen scope: feed that cursor back to [`Boxes::get_imbox_seen`], never to
//! [`Boxes::get_imbox`].

use std::collections::HashMap;
use std::fmt;
use std::str::FromStr;

use crate::error::Error;
use crate::generated::types::{CreateBoxGroupRequestContent, CreateBoxGroupResponseContent};

pub use crate::generated::services::boxes::*;

/// The kinds of box a HEY account has, as [`Boxes::list`] reports them.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum BoxKind {
    Imbox,
    Feed,
    SetAside,
    ReplyLater,
    PaperTrail,
    BubbleUp,
}

impl BoxKind {
    pub fn as_str(&self) -> &'static str {
        match self {
            BoxKind::Imbox => "imbox",
            BoxKind::Feed => "feedbox",
            BoxKind::SetAside => "asidebox",
            BoxKind::ReplyLater => "laterbox",
            BoxKind::PaperTrail => "trailbox",
            BoxKind::BubbleUp => "bubblebox",
        }
    }
}

/// The caller's boxes by kind, as one [`Boxes::list`] read answered them. Hold on to it and
/// a kind resolves without reading the index again.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct BoxKinds(HashMap<String, i64>);

impl BoxKinds {
    pub fn id(&self, kind: BoxKind) -> Result<i64, Error> {
        match self.0.get(kind.as_str()) {
            Some(id) => Ok(*id),
            None => Err(Error::api(0, format!("no box of kind {:?}", kind.as_str()))),
        }
    }
}

impl<'a> Boxes<'a> {
    /// The id of the box of a kind.
    ///
    /// The client reads the box index once and answers every kind from that reading for as
    /// long as it lives — a box's kind does not change, and the ids do not either. A client
    /// derived with [`Client::for_account`](crate::Client::for_account) reads it again for
    /// the account it presents. Use [`Boxes::kinds`] to read the index afresh.
    ///
    /// A kind the account has no box for is still a failure, and the index is not read
    /// again to be sure. Go re-reads it on every such miss, which is a read per call for a
    /// kind that will never be there.
    pub async fn id_by_kind(&self, kind: BoxKind) -> Result<i64, Error> {
        // The lock is held across the index read on purpose: it makes concurrent callers
        // share one read rather than each starting their own.
        let mut cached = self.client().scope.box_kinds.lock().await;
        match &*cached {
            Some(kinds) => kinds.id(kind),
            None => {
                let kinds = self.kinds().await?;
                let id = kinds.id(kind);
                *cached = Some(kinds);
                id
            }
        }
    }

    /// Reads the box index and maps every box's kind to its id. This is the read itself,
    /// so it goes to HEY however many times it is called.
    pub async fn kinds(&self) -> Result<BoxKinds, Error> {
        let boxes = self.list().await?;
        Ok(BoxKinds(
            boxes
                .iter()
                .filter(|mailbox| !mailbox.kind.is_empty())
                .map(|mailbox| (mailbox.kind.clone(), mailbox.id))
                .collect(),
        ))
    }

    /// Gathers a selection of postings into a new Set Aside group. The generated
    /// [`Boxes::create_group`] takes the same request as a body.
    pub async fn create_box_group(
        &self,
        box_id: i64,
        posting_ids: &[i64],
    ) -> Result<CreateBoxGroupResponseContent, Error> {
        let body = CreateBoxGroupRequestContent {
            posting_ids: posting_ids.to_vec(),
        };
        self.create_group(box_id, &body).await
    }
}

impl fmt::Display for BoxKind {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(self.as_str())
    }
}

impl FromStr for BoxKind {
    type Err = Error;

    fn from_str(source: &str) -> Result<BoxKind, Error> {
        match source {
            "imbox" => Ok(BoxKind::Imbox),
            "feedbox" => Ok(BoxKind::Feed),
            "asidebox" => Ok(BoxKind::SetAside),
            "laterbox" => Ok(BoxKind::ReplyLater),
            "trailbox" => Ok(BoxKind::PaperTrail),
            "bubblebox" => Ok(BoxKind::BubbleUp),
            _ => Err(Error::usage(format!(
                "box kind {source:?} is none of imbox, feedbox, asidebox, laterbox, trailbox, bubblebox"
            ))),
        }
    }
}
