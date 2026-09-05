//! The stickies board, in the terms the board itself is kept in: a size, a limit and a
//! position.

use std::str::FromStr;

use crate::error::Error;
use crate::generated::types::{
    MoveStickyRequestContent, Sticky, StickyPayload, StickyRequestContent,
};

pub use crate::generated::services::stickies::*;

/// The largest page the stickies index answers with. The server clamps anything above it,
/// so [`Stickies::list_up_to`] clamps too rather than sending a number it knows is ignored.
pub const MAX_STICKIES_LIMIT: u32 = 100;

/// The highest board position [`Stickies::move_to`] accepts. The wire format carries the
/// position as a 32-bit integer.
pub const MAX_STICKY_POSITION: i64 = i32::MAX as i64;

/// How much room a sticky takes on the board.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum StickySize {
    Small,
    Medium,
    Large,
}

impl StickySize {
    pub fn as_str(&self) -> &'static str {
        match self {
            StickySize::Small => "small",
            StickySize::Medium => "medium",
            StickySize::Large => "large",
        }
    }
}

impl<'a> Stickies<'a> {
    /// The stickies in board order, at most `limit` of them. Zero asks for the server
    /// default, which is also its maximum of 100.
    pub async fn list_up_to(&self, limit: u32) -> Result<Vec<Sticky>, Error> {
        let params = ListStickiesParams {
            limit: page_limit(limit),
        };
        self.list(&params).await
    }

    /// Writes a new sticky. No size leaves the server default in place.
    pub async fn create_sticky(
        &self,
        body: &str,
        size: Option<StickySize>,
    ) -> Result<Sticky, Error> {
        self.create(&sticky_body(body, size)).await
    }

    /// Edits a sticky. An empty body and no size are left alone.
    pub async fn update_sticky(
        &self,
        sticky_id: i64,
        body: &str,
        size: Option<StickySize>,
    ) -> Result<Sticky, Error> {
        self.update(sticky_id, &sticky_body(body, size)).await
    }

    /// Repositions a sticky on the board. Positions run from zero to
    /// [`MAX_STICKY_POSITION`].
    pub async fn move_to(&self, sticky_id: i64, position: i64) -> Result<(), Error> {
        if !(0..=MAX_STICKY_POSITION).contains(&position) {
            return Err(Error::usage(format!(
                "sticky position must be between 0 and {MAX_STICKY_POSITION}, got {position}"
            )));
        }

        let body = MoveStickyRequestContent {
            id: sticky_id,
            position: position as i32,
        };
        self.move_sticky(&body).await
    }
}

/// A limit of zero is left off the query entirely: sending `limit=0` is clamped to a single
/// sticky rather than read as "no limit".
fn page_limit(limit: u32) -> Option<i32> {
    match limit {
        0 => None,
        limit => Some(limit.min(MAX_STICKIES_LIMIT) as i32),
    }
}

fn sticky_body(body: &str, size: Option<StickySize>) -> StickyRequestContent {
    let body = if body.is_empty() {
        None
    } else {
        Some(body.to_string())
    };
    StickyRequestContent {
        sticky: StickyPayload {
            body,
            size: size.map(|size| size.as_str().to_string()),
        },
    }
}

impl FromStr for StickySize {
    type Err = Error;

    fn from_str(source: &str) -> Result<StickySize, Error> {
        match source {
            "small" => Ok(StickySize::Small),
            "medium" => Ok(StickySize::Medium),
            "large" => Ok(StickySize::Large),
            _ => Err(Error::usage(format!(
                "sticky size {source:?} is none of \"small\", \"medium\" or \"large\""
            ))),
        }
    }
}
