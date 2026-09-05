//! Email extenzions — the custom addresses a custom-domain account carries, such as
//! `sales@yourdomain.com`.
//!
//! [`Extenzions::create`] and [`Extenzions::update`] post a form to the `.json` path, so a
//! current server answers the written extenzion while one without the JSON branch redirects
//! and hands nothing back. The generated [`Extenzions::delete`] is a plain JSON operation.

use std::borrow::Cow;

use reqwest::Method;

use crate::client::Response;
use crate::error::Error;
use crate::form::FormResponse;
use crate::generated::routes;
use crate::generated::types::{Extenzion as ExtenzionPayload, NavigationItem, NavigationResponse};
use crate::observability::OperationInfo;
use crate::services::write_info;

pub use crate::generated::services::extenzions::*;

/// The title HEY gives the extensions group in the navigation payload.
const NAVIGATION_GROUP: &str = "Extensions";

/// What a contact URL puts the contact's id after, as in `/contacts/4821`.
const CONTACT_PATH: &str = "/contacts/";

/// One email extenzion.
///
/// The id is the extenzion's *contact* id — the one every write endpoint takes, and the one
/// its `app_url` carries. The id a JSON write answers with belongs to the Extenzion record
/// instead, which no endpoint takes.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct Extenzion {
    pub id: i64,
    pub name: String,
    pub app_url: String,
}

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct CreateExtenzionParams {
    /// The extenzion name: "sales" becomes `sales@yourdomain.com`.
    pub name: String,
    /// The member email addresses.
    pub members: Vec<String>,
}

/// A partial revision: `None` leaves a field as it is.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct UpdateExtenzionParams {
    /// An empty name is no name, and is left off the wire like a `None` one.
    pub name: Option<String>,
    /// The whole membership, which replaces what the extenzion had rather than adding to
    /// it.
    pub members: Option<Vec<String>>,
}

impl<'a> Extenzions<'a> {
    /// The extenzions on the account.
    ///
    /// This reads the navigation payload rather than scraping the extenzions page, so it
    /// carries only what navigation carries: each extenzion's name and its contact URL.
    /// It is one operation, `Extenzions.ListExtenzions`, rather than the identity read it
    /// happens to be built on — which is why it sends the navigation route itself instead
    /// of going through [`Identity::get_navigation`](crate::services::identity::Identity::get_navigation).
    pub async fn list(&self) -> Result<Vec<Extenzion>, Error> {
        let mut operation = self.client().operation(&routes::GET_NAVIGATION, &[]);
        operation.info(OperationInfo {
            service: Cow::Borrowed("Extenzions"),
            operation: Cow::Borrowed("ListExtenzions"),
            resource_type: Cow::Borrowed("extenzion"),
            is_mutation: false,
            resource_id: None,
        });
        let navigation: NavigationResponse = self.client().send(operation).await?;
        extenzions_from_navigation(&navigation)
    }

    /// Creates an extenzion and answers it. A server without the JSON create branch hands
    /// nothing back, and the answer is then `None`.
    pub async fn create(
        &self,
        account_id: i64,
        params: &CreateExtenzionParams,
    ) -> Result<Option<Extenzion>, Error> {
        let mut fields = vec![("extenzion[name]", params.name.as_str())];
        for member in &params.members {
            fields.push(("extenzion[members][]", member));
        }

        let mut operation = self.client().form(
            Method::POST,
            &format!("/accounts/{account_id}/domains/extenzions.json"),
        )?;
        operation.info(write_info(
            "Extenzions",
            "CreateExtenzion",
            "extenzion",
            None,
        ));
        operation.form(&fields);
        extenzion_from_form_response(&self.client().execute(operation).await?)
    }

    /// Revises an extenzion and answers it. The id is the extenzion's contact id. A server
    /// without the JSON update branch hands nothing back, and the answer is then `None`.
    pub async fn update(
        &self,
        account_id: i64,
        extenzion_id: i64,
        params: &UpdateExtenzionParams,
    ) -> Result<Option<Extenzion>, Error> {
        let mut fields = Vec::new();
        if let Some(name) = &params.name
            && !name.is_empty()
        {
            fields.push(("extenzion[name]", name.as_str()));
        }
        if let Some(members) = &params.members {
            for member in members {
                fields.push(("extenzion[members][]", member));
            }
        }

        let mut operation = self.client().form(
            Method::PATCH,
            &format!("/accounts/{account_id}/domains/extenzions/{extenzion_id}.json"),
        )?;
        operation.info(write_info(
            "Extenzions",
            "UpdateExtenzion",
            "extenzion",
            Some(extenzion_id),
        ));
        operation.form(&fields);
        extenzion_from_form_response(&self.client().execute(operation).await?)
    }
}

/// The extenzions in navigation's "Extensions" group. The group leads with the "All
/// Extensions" link, which names no contact and so falls out on its own.
fn extenzions_from_navigation(navigation: &NavigationResponse) -> Result<Vec<Extenzion>, Error> {
    navigation
        .items
        .iter()
        .flatten()
        .filter(|item| item.title.as_deref() == Some(NAVIGATION_GROUP))
        .flat_map(|group| group.menu_items.iter().flatten())
        .filter_map(|entry| listed_extenzion(entry).transpose())
        .collect()
}

fn listed_extenzion(entry: &NavigationItem) -> Result<Option<Extenzion>, Error> {
    let app_url = entry.app_url.clone().unwrap_or_default();
    Ok(contact_id_from_url(&app_url)?.map(|id| Extenzion {
        id,
        name: entry.title.clone().unwrap_or_default(),
        app_url,
    }))
}

/// The extenzion a JSON write answered with. A server without the JSON branch redirects to
/// the extenzions page instead, which leaves nothing to read.
fn extenzion_from_form_response(answered: &Response) -> Result<Option<Extenzion>, Error> {
    let written = FormResponse::new(answered);
    if written.body.is_empty() {
        Ok(None)
    } else {
        let payload: ExtenzionPayload = serde_json::from_str(&written.body)?;
        let app_url = payload.app_url.unwrap_or_default();
        let id = contact_id_from_url(&app_url)?
            .ok_or_else(|| Error::api(0, format!("no contact id in {app_url:?}")))?;
        Ok(Some(Extenzion {
            id,
            name: payload.name.unwrap_or_default(),
            app_url,
        }))
    }
}

/// The contact id a contact URL carries, as Go's `/contacts/(\d+)` reads it.
///
/// A URL naming no contact at all answers `None` — navigation's "All Extensions" link is
/// one, and it is meant to fall out of the list. A URL that names one the SDK cannot read
/// is a failure instead of another such link: dropping it would hide an extenzion the
/// caller would then never hear about.
fn contact_id_from_url(contact_url: &str) -> Result<Option<i64>, Error> {
    for (at, _) in contact_url.match_indices(CONTACT_PATH) {
        let digits = digit_run(&contact_url[at + CONTACT_PATH.len()..]);
        if !digits.is_empty() {
            return digits.parse().map(Some).map_err(|error| {
                Error::api(
                    0,
                    format!("contact id in {contact_url:?} is not a number: {error}"),
                )
            });
        }
    }
    Ok(None)
}

fn digit_run(text: &str) -> String {
    text.chars().take_while(char::is_ascii_digit).collect()
}
