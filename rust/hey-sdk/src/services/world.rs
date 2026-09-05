//! HEY World — the blog you write by sending an email.
//!
//! None of it is JSON: a post is created by emailing world@hey.com, an edit answers a
//! redirect, and the subscriber list is a CSV stream.

use std::borrow::Cow;

use bytes::{Bytes, BytesMut};
use percent_encoding::{AsciiSet, NON_ALPHANUMERIC, utf8_percent_encode};
use reqwest::Method;

use crate::client::Client;
use crate::error::Error;
use crate::observability::OperationInfo;
use crate::services::write_info;

/// The recipient that turns a message into a HEY World post.
pub const WORLD_ADDRESS: &str = "world@hey.com";

/// Where a published message lands, and what the token naming the post follows.
const POST_PATH: &str = "/world/posts/";

/// The part a subscriber import is read from.
const IMPORT_PART: &str = "world_list_import[source]";

/// The file name an import falls back to when the caller names none.
const DEFAULT_IMPORT_FILENAME: &str = "subscribers.csv";

/// A path parameter, escaped as every modelled route escapes one. A list is named by its
/// author's email address, so the `@` goes out as `%40`.
const PATH_SEGMENT: &AsciiSet = &NON_ALPHANUMERIC
    .remove(b'-')
    .remove(b'_')
    .remove(b'.')
    .remove(b'~');

pub struct World<'a> {
    client: &'a Client,
}

impl Client {
    pub fn world(&self) -> World<'_> {
        World::new(self)
    }
}

impl<'a> World<'a> {
    pub(crate) fn new(client: &'a Client) -> World<'a> {
        World { client }
    }

    /// The client this service sends through.
    pub fn client(&self) -> &'a Client {
        self.client
    }

    /// Writes a HEY World post by sending a message to [`WORLD_ADDRESS`], and answers the
    /// post's token — the handle [`World::update_post`] and [`World::delete_post`] take.
    pub async fn publish(&self, subject: &str, content: &str) -> Result<String, Error> {
        let sender_id = self.client.default_sender_id().await?.to_string();
        let mut operation = self.client.form(Method::POST, "/messages")?;
        operation.info(write_info("World", "PublishWorldPost", "world_post", None));
        operation.form(&[
            ("acting_sender_id", sender_id.as_str()),
            ("message[subject]", subject),
            ("message[content]", content),
            ("entry[addressed][directly]", WORLD_ADDRESS),
            ("entry[status]", "active"),
        ]);

        let sent = self.client.send_form(operation).await?;
        let location = sent.location.unwrap_or_default();
        post_token(&location).ok_or_else(|| {
            Error::api(
                0,
                format!(
                    "the message was sent but did not become a HEY World post (landed on {location:?})"
                ),
            )
        })
    }

    /// Edits a published post. An empty subject or body is left off the wire, and HEY leaves
    /// what a request does not name alone.
    pub async fn update_post(
        &self,
        token: &str,
        subject: &str,
        content: &str,
    ) -> Result<(), Error> {
        let mut fields = Vec::new();
        if !subject.is_empty() {
            fields.push(("world_post[subject]", subject));
        }
        if !content.is_empty() {
            fields.push(("world_post[content]", content));
        }

        let mut operation = self.client.form(Method::PATCH, &post_path(token))?;
        operation.info(write_info("World", "UpdateWorldPost", "world_post", None));
        operation.form(&fields);
        self.client.send_unit(operation).await
    }

    /// Takes a post off HEY World.
    pub async fn delete_post(&self, token: &str) -> Result<(), Error> {
        let mut operation = self.client.form(Method::DELETE, &post_path(token))?;
        operation.info(write_info("World", "DeleteWorldPost", "world_post", None));
        self.client.send_unit(operation).await
    }

    /// The confirmed subscribers of a list as CSV, with the columns `email_address` and
    /// `subscribed_at`. The list is named by its author's email address.
    pub async fn export_subscribers(&self, list_email_address: &str) -> Result<Bytes, Error> {
        let path = format!("/world/lists/{}/export.csv", escape(list_email_address));
        let mut operation = self.client.csv(&path)?;
        operation.info(OperationInfo {
            service: Cow::Borrowed("World"),
            operation: Cow::Borrowed("ExportWorldSubscribers"),
            resource_type: Cow::Borrowed("world_list"),
            is_mutation: false,
            resource_id: None,
        });
        Ok(self.client.execute(operation).await?.body)
    }

    /// Uploads a CSV of subscribers to a list. A blank `filename` becomes `subscribers.csv`,
    /// and one that does not already end in `.csv` gets it added: HEY reads the import by its
    /// extension.
    pub async fn import_subscribers(
        &self,
        list_email_address: &str,
        filename: &str,
        csv: &[u8],
    ) -> Result<(), Error> {
        let (content_type, body) = subscriber_import_body(filename, csv);
        let mut operation = self.client.form(
            Method::POST,
            &format!("/world/lists/{}/imports", escape(list_email_address)),
        )?;
        operation.info(write_info(
            "World",
            "ImportWorldSubscribers",
            "world_list",
            None,
        ));
        operation.multipart(content_type, body);
        self.client.send_unit(operation).await
    }
}

/// The token out of the location a publish redirected to, as Go's `/world/posts/([0-9a-f]+)`
/// reads it: the first `/world/posts/` followed by at least one hex digit, and the run of hex
/// digits after it. A message that landed anywhere else names no token.
fn post_token(location: &str) -> Option<String> {
    location
        .match_indices(POST_PATH)
        .map(|(at, _)| hex_run(&location[at + POST_PATH.len()..]))
        .find(|token| !token.is_empty())
}

fn hex_run(text: &str) -> String {
    text.chars()
        .take_while(|character| matches!(character, '0'..='9' | 'a'..='f'))
        .collect()
}

fn post_path(token: &str) -> String {
    format!("{POST_PATH}{}", escape(token))
}

fn escape(value: &str) -> String {
    utf8_percent_encode(value, PATH_SEGMENT).to_string()
}

/// The CSV wrapped in the multipart form the import endpoint expects, and the content type
/// naming the boundary it was built with.
fn subscriber_import_body(filename: &str, csv: &[u8]) -> (String, Bytes) {
    let boundary = boundary();
    let mut body = BytesMut::from(
        format!(
            "--{boundary}\r\nContent-Disposition: form-data; name=\"{IMPORT_PART}\"; filename=\"{}\"\r\nContent-Type: application/octet-stream\r\n\r\n",
            escape_quotes(&import_filename(filename))
        )
        .as_bytes(),
    );
    body.extend_from_slice(csv);
    body.extend_from_slice(format!("\r\n--{boundary}--\r\n").as_bytes());
    (
        format!("multipart/form-data; boundary={boundary}"),
        body.freeze(),
    )
}

fn boundary() -> String {
    format!("{:032x}", rand::random::<u128>())
}

fn import_filename(filename: &str) -> String {
    if filename.is_empty() {
        DEFAULT_IMPORT_FILENAME.to_string()
    } else if filename.ends_with(".csv") {
        filename.to_string()
    } else {
        format!("{filename}.csv")
    }
}

/// A quote or a backslash would end the header field early, so both are escaped the way Go's
/// `mime/multipart` escapes them.
fn escape_quotes(text: &str) -> String {
    text.replace('\\', "\\\\").replace('"', "\\\"")
}
