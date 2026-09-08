//! Screening a contact into a box, so everything they send lands there.
//!
//! HEY models a designation under the box that holds it, so these are `/boxes/{id}` paths;
//! every SDK files them under the name the feature goes by, which is this handle.

use crate::error::Error;
use crate::generated::types::CreateBoxDesignationRequestContent;

pub use crate::generated::services::designations::*;

impl<'a> Designations<'a> {
    /// Designates a contact to a box. The generated [`Designations::create`] takes the same
    /// request as a body.
    ///
    /// HEY designates the contact's primary, so a contact's aliases fold into the one
    /// designation — whose id cannot be worked out from `contact_id`. Read the box back if
    /// you need it.
    pub async fn create_box_designation(&self, box_id: i64, contact_id: i64) -> Result<(), Error> {
        let body = CreateBoxDesignationRequestContent { contact_id };
        self.create(box_id, &body).await
    }
}
