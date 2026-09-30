//! Writing contacts, the two refusals a contact write answers with, and the recipients
//! HEY suggests in a composer.

use std::fmt;

use serde::de::DeserializeOwned;

use crate::error::Error;
use crate::generated::routes;
use crate::generated::types::{
    ConflictErrorResponseContent, Contact, ContactDetail, ContactNote, ContactNotePayload,
    ContactNoteRequestContent, ContactPayload, ContactRequestContent, CreateContactRequestContent,
    UnprocessableEntityErrorResponseContent, UpdateContactClearanceRequestContent,
};
use crate::operation::Operation;
use crate::services::clearances::ClearanceStatus;
use crate::types::SensitiveString;

pub use crate::generated::services::contacts::*;

/// A contact, as its writes take it.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct ContactParams {
    /// What the contact is called.
    pub name: String,
    /// The contact's main address, the one HEY files them under.
    pub email_address: String,
    /// The other addresses that belong to the same person. Sending the list replaces it,
    /// so an address left out stops being an alias; `None` leaves the current aliases
    /// alone.
    pub alias_email_addresses: Option<Vec<String>>,
    /// The account to file the contact under, on a create. One identity can hold several
    /// accounts, each with its own contacts; this is the identity's user on the one meant,
    /// which the identity's `all_users` carries alongside its `account_id`. Left unset,
    /// HEY files the contact under the first account. An update ignores it.
    pub account_user_id: Option<i64>,
}

/// The contacts a refused write clashed with: HEY's web sends you to a merge form at this
/// point, and these are the contacts it would have offered to merge with.
///
/// A create that clashes still creates the contact — the merge happens afterwards — so
/// `contact_id` is the contact that was written, not one that failed to be.
///
/// It travels as the source of an [`Error`] with [`crate::ErrorCode::Conflict`], so a
/// caller who only cares that the write was refused can ignore it.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
#[non_exhaustive]
pub struct ContactConflict {
    /// The contact the write was for — on a create, the one it made.
    pub contact_id: i64,
    /// The contacts already holding one of the addresses.
    pub conflicting_contact_ids: Vec<i64>,
}

impl ContactConflict {
    /// The conflict a refused contact write carries, when the error is one.
    pub fn from_error(error: &Error) -> Option<&ContactConflict> {
        std::error::Error::source(error)?.downcast_ref::<ContactConflict>()
    }
}

/// One suggestion from HEY's recipient autocomplete list, as
/// [`Contacts::addressable`] reads it.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
#[non_exhaustive]
pub struct AddressableRecipient {
    /// One address, or a comma-separated list for a contact group or "Everyone at …". It
    /// prints as `[REDACTED]`, as a contact's address does; [`SensitiveString::expose`]
    /// reads it. The redaction is this field's alone: `label` is the address again for a
    /// contact with no name, and prints as it is.
    pub value: SensitiveString,
    /// The name to show; the address itself when the contact has no name.
    pub label: String,
    /// What else HEY says about the row — "@example.com" for an account, "Contact group
    /// with 3 people" for a group — and empty for a person.
    pub detail: String,
}

impl Contacts<'_> {
    /// Adds a contact and answers it.
    ///
    /// On a client scoped to an account the contact is filed under that account, and an
    /// `account_user_id` naming another one is refused rather than quietly overruled.
    pub async fn create_contact(&self, params: &ContactParams) -> Result<Contact, Error> {
        let body = CreateContactRequestContent {
            acting_user_id: self.acting_user_id(params.account_user_id).await?,
            contact: contact_payload(params),
        };
        let mut operation = self.client().operation(&routes::CREATE_CONTACT, &[]);
        operation.json(&body)?;
        self.write(operation).await
    }

    /// Edits a contact and answers it. Fields left empty are kept, as are the aliases when
    /// `alias_email_addresses` is `None`.
    ///
    /// HEY's update is a full replacement — it rewrites the name and address and removes
    /// any alias not submitted — so the contact is read first and the unset fields are
    /// filled in from it before the write. That read-then-write is not atomic: a change
    /// made to the contact in between is overwritten with what was read. Pass every field
    /// explicitly when that matters.
    ///
    /// The contact that comes back is not always the one addressed: giving a contact one of
    /// its own aliases as the main address promotes the alias, and the alias is what is
    /// answered.
    pub async fn update_contact(
        &self,
        contact_id: i64,
        params: &ContactParams,
    ) -> Result<Contact, Error> {
        let current = self.get(contact_id, &GetContactParams::default()).await?;
        let body = ContactRequestContent {
            contact: merged_payload(params, &current),
        };
        let mut operation = self
            .client()
            .operation(&routes::UPDATE_CONTACT, &[&contact_id]);
        operation.resource_id(contact_id);
        operation.json(&body)?;
        self.write(operation).await
    }

    /// Answers the Screener for a contact.
    pub async fn screen(&self, contact_id: i64, status: ClearanceStatus) -> Result<(), Error> {
        let body = UpdateContactClearanceRequestContent {
            status: status.as_str().to_string(),
        };
        self.update_clearance(contact_id, &body).await
    }

    /// Writes the private note kept on a contact, replacing whatever was there, and answers
    /// the note as it now reads.
    pub async fn set_note(&self, contact_id: i64, note: &str) -> Result<ContactNote, Error> {
        let body = ContactNoteRequestContent {
            contact: ContactNotePayload {
                note: note.to_string(),
            },
        };
        let mut operation = self
            .client()
            .operation(&routes::UPDATE_CONTACT_NOTE, &[&contact_id]);
        operation.json(&body)?;
        self.write(operation).await
    }

    /// The recipients HEY suggests in a composer's To, Cc and Bcc fields, in HEY's order:
    /// the contacts the identity recently addressed, then every other contact by name, then
    /// "Everyone at …" for each active account with a domain, then the identity's contact
    /// groups. `include_self` asks for the identity's own addresses too, as the web
    /// composer does.
    ///
    /// HEY answers bare `[value, label]` and `[value, label, detail]` rows — what
    /// [`Contacts::list_addressable`] answers as they are. A row with fewer than two
    /// strings is skipped rather than failing the read, and so is one with no address in
    /// it: "Everyone at …" for an account nobody else is on, or a contact group with no
    /// members.
    pub async fn addressable(
        &self,
        include_self: bool,
    ) -> Result<Vec<AddressableRecipient>, Error> {
        let params = ListAddressableContactsParams {
            include_self: include_self.then_some(true),
        };
        let rows = self.list_addressable(&params).await?;
        Ok(rows
            .iter()
            .filter_map(|row| addressable_recipient(row))
            .collect())
    }

    async fn acting_user_id(&self, chosen: Option<i64>) -> Result<Option<i64>, Error> {
        match self.client().account_id() {
            None => Ok(chosen),
            Some(account_id) => {
                let account_user_id = self.client().account_user_id().await?;
                match chosen {
                    Some(chosen) if chosen != account_user_id => Err(Error::usage(format!(
                        "account user {chosen} does not belong to selected account {account_id}"
                    ))),
                    _ => Ok(Some(account_user_id)),
                }
            }
        }
    }

    /// Sends a contact write, reading the two refusals it can answer with out of the body
    /// they arrive in: an address that belongs to someone else, and a contact the model
    /// itself rejected. Both are failures, and the hooks are told so; all that happens here
    /// is that the failure is reworded from what the model said about it. The rewording is
    /// done inside the operation, so the hooks hear it end with the error the caller gets.
    async fn write<T: DeserializeOwned>(&self, mut operation: Operation) -> Result<T, Error> {
        operation.quiet();
        let info = operation.info.clone();
        self.client()
            .as_operation(
                &info,
                Box::pin(async {
                    match self.client().execute(operation).await {
                        Ok(response) => response.json(),
                        Err(error) if error.http_status() == Some(409) => Err(clash(&error)),
                        Err(error) if error.http_status() == Some(422) => Err(rejection(&error)),
                        Err(error) => Err(error),
                    }
                }),
            )
            .await
    }
}

/// The suggestion a row names. A row too short to carry both a value and a label is one
/// the autocomplete list has nothing to say about, and a row whose value is empty names
/// nobody to address.
fn addressable_recipient(row: &[String]) -> Option<AddressableRecipient> {
    match row {
        [value, label, rest @ ..] if !value.is_empty() => Some(AddressableRecipient {
            value: SensitiveString::new(value.as_str()),
            label: label.clone(),
            detail: rest.first().cloned().unwrap_or_default(),
        }),
        _ => None,
    }
}

fn contact_payload(params: &ContactParams) -> ContactPayload {
    let email_address = if params.email_address.is_empty() {
        None
    } else {
        Some(SensitiveString::new(params.email_address.as_str()))
    };
    ContactPayload {
        name: params.name.clone(),
        email_address,
        alias_email_addresses: params.alias_email_addresses.clone(),
    }
}

/// The write HEY is sent: the caller's fields, with the contact's own filled in wherever
/// the caller named none.
///
/// The alias list always goes out, filled in from the contact when the caller left it
/// unset. Go leaves an empty one off with `omitempty`, which makes clearing every alias
/// impossible there — an empty list would be indistinguishable from saying nothing. Sending
/// it means an explicit `Some(vec![])` clears the aliases, which is the one thing HEY's own
/// full-replacement update is for.
fn merged_payload(params: &ContactParams, current: &ContactDetail) -> ContactPayload {
    let mut payload = contact_payload(params);
    if payload.name.is_empty() {
        payload.name = current.name.clone().unwrap_or_default();
    }
    if payload.email_address.is_none() {
        payload.email_address.clone_from(&current.email_address);
    }
    if payload.alias_email_addresses.is_none() {
        payload.alias_email_addresses = Some(current_aliases(current));
    }
    payload
}

fn current_aliases(current: &ContactDetail) -> Vec<String> {
    current
        .aliases
        .iter()
        .flatten()
        .filter_map(|alias| alias.email_address.as_ref())
        .map(|address| address.expose().to_string())
        .collect()
}

fn clash(error: &Error) -> Error {
    let payload: ConflictErrorResponseContent = error.body_json().unwrap_or_default();
    Error::conflict(conflict_message(&payload)).with_source(ContactConflict {
        contact_id: payload.contact_id.unwrap_or_default(),
        conflicting_contact_ids: payload.conflicting_contact_ids.unwrap_or_default(),
    })
}

fn rejection(error: &Error) -> Error {
    let payload: UnprocessableEntityErrorResponseContent = error.body_json().unwrap_or_default();
    Error::validation(&payload.errors.unwrap_or_default())
}

/// The server's own words out of a 409. Contact writes answer the `errors` list the other
/// refusals use; elsewhere a 409 is a single message. A body neither of those still has to
/// read as something.
fn conflict_message(payload: &ConflictErrorResponseContent) -> String {
    let messages = payload.errors.as_deref().unwrap_or_default();
    if !messages.is_empty() {
        messages.join("; ")
    } else if let Some(message) = &payload.error {
        message.clone()
    } else {
        "the contact conflicts with one that already exists".to_string()
    }
}

impl fmt::Display for ContactConflict {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        if self.conflicting_contact_ids.is_empty() {
            write!(
                f,
                "contact {} conflicts with one that already exists",
                self.contact_id
            )
        } else {
            let ids: Vec<String> = self
                .conflicting_contact_ids
                .iter()
                .map(i64::to_string)
                .collect();
            write!(
                f,
                "contact {} conflicts with {}",
                self.contact_id,
                ids.join(", ")
            )
        }
    }
}

impl std::error::Error for ContactConflict {}
