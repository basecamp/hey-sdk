//! Starting and stopping the running time track, keeping the categories tracks are filed
//! under, and reading the lot back out as a file.
//!
//! HEY serves no JSON endpoint for a category write or for the export, so those are browser
//! form posts and a file read.

use std::borrow::Cow;

use bytes::Bytes;
use chrono::Utc;
use reqwest::Method;

use crate::error::Error;
use crate::generated::routes;
use crate::generated::types::{Recording, UpdateTimeTrackPayload, UpdateTimeTrackRequestContent};
use crate::observability::OperationInfo;
use crate::services::write_info;

pub use crate::generated::services::time_tracks::*;

impl<'a> TimeTracks<'a> {
    /// Starts a time track. It takes nothing: HEY ignores the request body here and starts a
    /// track with defaults. Notes and a category come later, with
    /// [`TimeTracks::update`] or [`TimeTracks::stop_and_file`], both of which also stop the
    /// track.
    ///
    /// This is [`TimeTracks::start`] with the one refusal it can meet named: a track already
    /// running answers 409, which arrives as [`crate::ErrorCode::Conflict`] carrying HEY's
    /// own message, so a caller can branch on it rather than read a generic API error.
    pub async fn start_tracking(&self) -> Result<Recording, Error> {
        match self.start().await {
            Ok(track) => Ok(track),
            Err(error) if error.http_status() == Some(409) => Err(already_running(&error)),
            Err(error) => Err(error),
        }
    }

    /// Stops the running time track by setting its end to now.
    pub async fn stop(&self, time_track_id: i64) -> Result<(), Error> {
        self.stop_and_file(time_track_id, None).await
    }

    /// Stops a time track and files it under a category in the one request, creating the
    /// category if HEY has none by that name. No category stops the track without filing it,
    /// which is what [`TimeTracks::stop`] does.
    ///
    /// Filing is only ever part of stopping: HEY completes a track on every update, so there
    /// is no such thing as setting a category on a track that keeps running.
    ///
    /// It sends the same PUT [`TimeTracks::update`] does, but announces itself to the
    /// client's hooks as `StopTimeTrack` rather than `UpdateTimeTrack`, so a gating policy
    /// can allow one without the other.
    pub async fn stop_and_file(
        &self,
        time_track_id: i64,
        category_title: Option<&str>,
    ) -> Result<(), Error> {
        let body = UpdateTimeTrackRequestContent {
            calendar_time_track: UpdateTimeTrackPayload {
                ends_at: Some(Utc::now()),
                // An empty title names no category, and goes out as no category at all
                // rather than as a category called "".
                category_title: category_title
                    .filter(|title| !title.is_empty())
                    .map(str::to_string),
                ..UpdateTimeTrackPayload::default()
            },
        };

        let mut operation = self
            .client()
            .operation(&routes::UPDATE_TIME_TRACK, &[&time_track_id]);
        operation.operation_name("StopTimeTrack");
        operation.resource_id(time_track_id);
        operation.json(&body)?;
        self.client().send_unit(operation).await
    }

    /// Adds a category to file tracks under.
    pub async fn create_category(&self, title: &str) -> Result<(), Error> {
        let mut operation = self
            .client()
            .form(Method::POST, "/calendar/time_tracks/categories")?;
        operation.info(write_info(
            "TimeTracks",
            "CreateTimeTrackCategory",
            "category",
            None,
        ));
        operation.form(&[("category[title]", title)]);
        self.client().send_unit(operation).await
    }

    /// Renames a category.
    pub async fn update_category(&self, category_id: i64, title: &str) -> Result<(), Error> {
        let mut operation = self.client().form(
            Method::PATCH,
            &format!("/calendar/time_tracks/categories/{category_id}"),
        )?;
        operation.info(write_info(
            "TimeTracks",
            "UpdateTimeTrackCategory",
            "category",
            Some(category_id),
        ));
        operation.form(&[("category[title]", title)]);
        self.client().send_unit(operation).await
    }

    /// Removes a category. The tracks filed under it stay, uncategorized.
    pub async fn delete_category(&self, category_id: i64) -> Result<(), Error> {
        let mut operation = self.client().form(
            Method::DELETE,
            &format!("/calendar/time_tracks/categories/{category_id}"),
        )?;
        operation.info(write_info(
            "TimeTracks",
            "DeleteTimeTrackCategory",
            "category",
            Some(category_id),
        ));
        self.client().send_unit(operation).await
    }

    /// Every completed time track as CSV, newest first, under the columns Start, End,
    /// Duration, Category and Notes. HEY streams this as a file rather than a document.
    pub async fn export(&self) -> Result<Bytes, Error> {
        let mut operation = self.client().csv("/calendar/time_tracks/exports")?;
        operation.info(OperationInfo {
            service: Cow::Borrowed("TimeTracks"),
            operation: Cow::Borrowed("ExportTimeTracks"),
            resource_type: Cow::Borrowed("time_track"),
            is_mutation: false,
            resource_id: None,
        });
        Ok(self.client().execute(operation).await?.body)
    }
}

fn already_running(refusal: &Error) -> Error {
    match refusal.hint() {
        Some(message) => Error::conflict(message),
        None => Error::conflict("a time track is already running"),
    }
}
