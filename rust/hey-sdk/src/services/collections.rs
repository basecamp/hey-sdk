//! Making collections and filing threads into them, on top of the generated collection
//! routes.
//!
//! HEY serves no JSON endpoint for any of these, so each one is a browser form post.

use reqwest::Method;

use crate::error::Error;
use crate::generated::types::{CollectionPayload, UpdateCollectionRequestContent};
use crate::services::write_info;

pub use crate::generated::services::collections::*;

/// What a new collection is made of.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct CreateCollectionParams {
    pub name: String,
    /// The blurb shown under the name.
    pub summary: Option<String>,
    /// The account that owns it. `None` leaves HEY to pick your first.
    pub account_id: Option<i64>,
}

/// What an edit changes about a collection. A field left unset — `None` or empty — is left
/// off the wire, and HEY leaves what a request does not name alone.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct UpdateCollectionParams {
    pub name: Option<String>,
    pub summary: Option<String>,
}

impl<'a> Collections<'a> {
    /// Makes a collection.
    ///
    /// The form post answers with a redirect to the collections index rather than to the
    /// collection it made, so the new collection's id does not come back.
    /// [`Collections::list`] afterwards is how to find it.
    pub async fn create(&self, params: &CreateCollectionParams) -> Result<(), Error> {
        let account = params.account_id.map(|account_id| account_id.to_string());
        let mut fields = vec![("collection[name]", params.name.as_str())];
        if let Some(summary) = params.summary.as_deref().filter(|it| !it.is_empty()) {
            fields.push(("collection[summary]", summary));
        }
        if let Some(account) = &account {
            fields.push(("account_id", account.as_str()));
        }

        let mut operation = self.client().form(Method::POST, "/collections")?;
        operation.info(write_info(
            "Collections",
            "CreateCollection",
            "collection",
            None,
        ));
        operation.form(&fields);
        self.client().send_unit(operation).await
    }

    /// Renames a collection or changes its summary. The generated [`Collections::update`]
    /// takes the same request as a body.
    pub async fn update_collection(
        &self,
        collection_id: i64,
        params: &UpdateCollectionParams,
    ) -> Result<(), Error> {
        let body = UpdateCollectionRequestContent {
            collection: CollectionPayload {
                name: present(&params.name),
                summary: present(&params.summary),
            },
        };
        self.update(collection_id, &body).await
    }

    /// Files a topic into a collection.
    pub async fn add_topic(&self, topic_id: i64, collection_id: i64) -> Result<(), Error> {
        let mut operation = self
            .client()
            .form(Method::POST, &format!("/topics/{topic_id}/collecting"))?;
        operation.info(write_info(
            "Collections",
            "CreateTopicCollecting",
            "collecting",
            Some(topic_id),
        ));
        operation.query("collection_id", collection_id).form(&[]);
        self.client().send_unit(operation).await
    }

    /// Takes a topic back out of a collection. A shadowed topic is silently left alone.
    pub async fn remove_topic(&self, topic_id: i64, collection_id: i64) -> Result<(), Error> {
        let mut operation = self
            .client()
            .form(Method::DELETE, &format!("/topics/{topic_id}/collecting"))?;
        operation.info(write_info(
            "Collections",
            "DeleteTopicCollecting",
            "collecting",
            Some(topic_id),
        ));
        operation.query("collection_id", collection_id);
        self.client().send_unit(operation).await
    }
}

/// An empty string is no value, and is left off the wire like a `None` one — the omission
/// is what tells HEY to leave the field as it is.
fn present(value: &Option<String>) -> Option<String> {
    value.clone().filter(|value| !value.is_empty())
}
