//! The Screener: who is waiting to be let in, and letting them in or turning them away.
//!
//! Clearing the Screener is the generated [`Clearances::punt`]. The work it starts is
//! queued, so everyone waiting is still pending when it answers; they are dropped and
//! reexamined the next time they write, so nothing is decided for them.

use std::str::FromStr;

use crate::error::{Error, ErrorCode};
use crate::generated::types::{
    BulkUpdateClearancesRequestContent, Clearance, ClearanceListResponse, ClearanceSummary,
    UpdateClearanceRequestContent, UpdateMyClearanceRequestContent,
};
use crate::pagination::Page;

pub use crate::generated::services::clearances::*;

/// The two decisions the Screener takes.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ClearanceStatus {
    Approved,
    Denied,
}

/// What to do beyond setting the status. HEY reads each of these for truthiness, so one
/// left alone stays off the wire entirely rather than going out as a false.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct ScreenOptions {
    /// Files everything the sender sends into that box rather than the Imbox.
    pub designation_box_id: Option<i64>,
    /// Marks the topics already waiting as spam and trains the filter on them.
    pub spam: bool,
    /// Screens the sender in without their waiting mail arriving unread.
    pub mark_topics_as_seen: bool,
}

impl ClearanceStatus {
    pub fn as_str(&self) -> &'static str {
        match self {
            ClearanceStatus::Approved => "approved",
            ClearanceStatus::Denied => "denied",
        }
    }
}

impl<'a> Clearances<'a> {
    /// How many senders are waiting, without fetching them.
    ///
    /// This is the cheap read HEY's own apps sync for the Screener badge. Use
    /// [`Clearances::pending`] for the senders themselves.
    pub async fn pending_count(&self) -> Result<i32, Error> {
        Ok(self
            .summary()
            .await?
            .pending_clearances_count
            .unwrap_or_default())
    }

    /// Everything HEY says about the Screener without the queue itself: how many senders
    /// are waiting, and the signed stream name to subscribe to on HEY's cable server to be
    /// told when that changes.
    pub async fn summary(&self) -> Result<ClearanceSummary, Error> {
        self.get(&GetClearancesParams::default())
            .await
            .map(Page::into_inner)
    }

    /// The senders waiting to be screened, a page at a time.
    ///
    /// Each one carries the petitioner and the most recent entry they sent, so a caller can
    /// show who is asking and what they wrote without a second read.
    pub async fn pending(&self, page: Option<&str>) -> Result<ClearanceSummary, Error> {
        self.pending_page(page).await.map(Page::into_inner)
    }

    /// The same queue as [`Clearances::pending`], keeping the cursor for the page after it
    /// so a caller walking the queue is told when it has reached the end.
    pub async fn pending_page(&self, page: Option<&str>) -> Result<Page<ClearanceSummary>, Error> {
        let params = GetClearancesParams {
            include_clearances: Some(true),
            page: page.map(str::to_string),
        };
        self.get(&params).await
    }

    /// Answers the Screener for one sender.
    pub async fn screen(
        &self,
        clearance_id: i64,
        status: ClearanceStatus,
        options: &ScreenOptions,
    ) -> Result<Clearance, Error> {
        let body = UpdateClearanceRequestContent {
            status: status.as_str().to_string(),
            designation_box_id: options.designation_box_id,
            spam: flag(options.spam),
            mark_topics_as_seen: flag(options.mark_topics_as_seen),
        };
        self.update(clearance_id, &body).await
    }

    /// Screens several senders at once and answers the clearances it changed.
    ///
    /// HEY answers 404 when none of the ids belong to the caller. A partial match succeeds
    /// and answers only what it touched, so compare the answer against what was sent.
    pub async fn screen_many(
        &self,
        clearance_ids: &[i64],
        status: ClearanceStatus,
        spam: bool,
    ) -> Result<Vec<Clearance>, Error> {
        if clearance_ids.is_empty() {
            return Err(Error::usage("at least one clearance is required"));
        }

        let body = BulkUpdateClearancesRequestContent {
            ids: join_ids(clearance_ids),
            status: status.as_str().to_string(),
            spam: flag(spam),
        };
        Ok(self
            .bulk_update(&body)
            .await?
            .clearances
            .unwrap_or_default())
    }

    /// The senders already screened in or out, newest decision first, a page at a time.
    pub async fn screened(&self, page: Option<&str>) -> Result<Vec<Clearance>, Error> {
        Ok(self
            .screened_page(page)
            .await?
            .into_inner()
            .clearances
            .unwrap_or_default())
    }

    /// The same decisions as [`Clearances::screened`], keeping the cursor for the page
    /// after it.
    pub async fn screened_page(
        &self,
        page: Option<&str>,
    ) -> Result<Page<ClearanceListResponse>, Error> {
        let params = GetMyClearancesParams {
            page: page.map(str::to_string),
        };
        self.get_my(&params).await
    }

    /// Changes its mind about a sender already screened in or out.
    ///
    /// This is the decided list, not the queue: [`Clearances::screen`] is what answers a
    /// pending sender.
    pub async fn rescreen(
        &self,
        clearance_id: i64,
        status: ClearanceStatus,
    ) -> Result<Clearance, Error> {
        let body = UpdateMyClearanceRequestContent {
            status: status.as_str().to_string(),
        };
        self.update_my(clearance_id, &body).await
    }
}

fn flag(value: bool) -> Option<bool> {
    if value { Some(true) } else { None }
}

fn join_ids(ids: &[i64]) -> String {
    ids.iter()
        .map(|id| id.to_string())
        .collect::<Vec<String>>()
        .join(",")
}

impl FromStr for ClearanceStatus {
    type Err = Error;

    fn from_str(source: &str) -> Result<ClearanceStatus, Error> {
        match source {
            "approved" => Ok(ClearanceStatus::Approved),
            "denied" => Ok(ClearanceStatus::Denied),
            _ => Err(Error::new(
                ErrorCode::Validation,
                format!("clearance status must be \"approved\" or \"denied\", got {source:?}"),
            )),
        }
    }
}
