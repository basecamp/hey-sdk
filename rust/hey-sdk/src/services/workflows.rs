//! Workflows — kanban-style boards of threads — on top of the generated workflow routes.
//!
//! A workflow has no JSON surface beyond the page that reads one and the autocomplete
//! endpoint that enumerates them, so every write is a browser form post.

use std::borrow::Cow;

use reqwest::Method;

use crate::error::Error;
use crate::generated::routes;
use crate::generated::types::WorkflowStage;
use crate::observability::OperationInfo;
use crate::services::write_info;

pub use crate::generated::services::workflows::*;

/// A workflow as the autocomplete endpoint names it.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct WorkflowSummary {
    pub id: i64,
    pub name: String,
    /// The account the workflow belongs to, empty when the row names none.
    pub account_name: String,
}

impl<'a> Workflows<'a> {
    /// The workflows on an account.
    ///
    /// The autocomplete endpoint answers bare `[id, name, account name]` rows, and answers
    /// 304 to a conditional request — the SDK sends none here, so this always comes back
    /// populated.
    pub async fn list(&self, account_id: i64) -> Result<Vec<WorkflowSummary>, Error> {
        let mut operation = self.client().request(
            Method::GET,
            format!("/autocompletable/accounts/{account_id}/workflows"),
        );
        operation
            .info(OperationInfo {
                service: Cow::Borrowed("Workflows"),
                operation: Cow::Borrowed("ListWorkflows"),
                resource_type: Cow::Borrowed("workflow"),
                is_mutation: false,
                resource_id: Some(account_id),
            })
            .without_json_suffix();

        let rows: Vec<Vec<String>> = self.client().send(operation).await?;
        Ok(rows.iter().filter_map(|row| summary(row)).collect())
    }

    /// A workflow's stages, in position order.
    pub async fn stages(&self, workflow_id: i64) -> Result<Vec<WorkflowStage>, Error> {
        Ok(self.get(workflow_id).await?.stages.unwrap_or_default())
    }

    /// Adds a workflow. No account — `None` or a zero id — leaves HEY to pick your first.
    pub async fn create(&self, name: &str, account_id: Option<i64>) -> Result<(), Error> {
        let account = account_id
            .filter(|account_id| *account_id != 0)
            .map(|account_id| account_id.to_string());
        let mut fields = vec![("workflow[name]", name)];
        if let Some(account) = &account {
            fields.push(("account_id", account.as_str()));
        }

        let mut operation = self.client().form(Method::POST, "/workflows")?;
        operation.info(write_info("Workflows", "CreateWorkflow", "workflow", None));
        operation.form(&fields);
        self.client().send_unit(operation).await
    }

    /// Renames a workflow.
    pub async fn update(&self, workflow_id: i64, name: &str) -> Result<(), Error> {
        let mut operation = self
            .client()
            .form(Method::PATCH, &format!("/workflows/{workflow_id}"))?;
        operation.info(write_info(
            "Workflows",
            "UpdateWorkflow",
            "workflow",
            Some(workflow_id),
        ));
        operation.form(&[("workflow[name]", name)]);
        self.client().send_unit(operation).await
    }

    /// Throws a workflow away.
    pub async fn delete(&self, workflow_id: i64) -> Result<(), Error> {
        let mut operation = self
            .client()
            .form(Method::DELETE, &format!("/workflows/{workflow_id}"))?;
        operation.info(write_info(
            "Workflows",
            "DeleteWorkflow",
            "workflow",
            Some(workflow_id),
        ));
        self.client().send_unit(operation).await
    }

    /// Adds a column to a workflow. HEY names it "Untitled"; rename it with
    /// [`Workflows::update_stage`].
    pub async fn create_stage(&self, workflow_id: i64) -> Result<(), Error> {
        let mut operation = self
            .client()
            .form(Method::POST, &format!("/workflows/{workflow_id}/stages"))?;
        operation.info(write_info(
            "Workflows",
            "CreateWorkflowStage",
            "workflow_stage",
            Some(workflow_id),
        ));
        operation.form(&[]);
        self.client().send_unit(operation).await
    }

    /// Renames a workflow column.
    pub async fn update_stage(
        &self,
        workflow_id: i64,
        stage_id: i64,
        name: &str,
    ) -> Result<(), Error> {
        let mut operation = self.client().form(
            Method::PATCH,
            &format!("/workflows/{workflow_id}/stages/{stage_id}"),
        )?;
        operation.info(write_info(
            "Workflows",
            "UpdateWorkflowStage",
            "workflow_stage",
            Some(stage_id),
        ));
        operation.form(&[("workflow_stage[name]", name)]);
        self.client().send_unit(operation).await
    }

    /// Removes a workflow column.
    pub async fn delete_stage(&self, workflow_id: i64, stage_id: i64) -> Result<(), Error> {
        let mut operation = self.client().form(
            Method::DELETE,
            &format!("/workflows/{workflow_id}/stages/{stage_id}"),
        )?;
        operation.info(write_info(
            "Workflows",
            "DeleteWorkflowStage",
            "workflow_stage",
            Some(stage_id),
        ));
        self.client().send_unit(operation).await
    }

    /// Adds a topic to a workflow in the stage named.
    ///
    /// HEY creates the workflow membership before selecting the stage, so a failure to
    /// select it leaves the topic in the workflow's first stage. The generated
    /// [`Workflows::create_staging`] is the first of those two requests on its own.
    ///
    /// The stage selection is a [quiet](crate::Operation::quiet) send, so the hooks hear
    /// `Workflows.CreateWorkflowStaging` once and see both requests under it, as they do in
    /// Go.
    pub async fn stage_topic(
        &self,
        topic_id: i64,
        workflow_id: i64,
        stage_id: i64,
    ) -> Result<(), Error> {
        let mut operation = self
            .client()
            .operation(&routes::CREATE_WORKFLOW_STAGING, &[&topic_id, &workflow_id]);
        operation
            .info(write_info(
                "Workflows",
                "CreateWorkflowStaging",
                "workflow_staging",
                Some(topic_id),
            ))
            .form_representation();
        self.client().send_unit(operation).await?;

        self.move_to_stage(topic_id, workflow_id, stage_id, None)
            .await
    }

    /// Moves a staged topic to another stage of its workflow. The generated
    /// [`Workflows::move_staging`] sends the same request as JSON, which HEY's own apps do
    /// not; this one sends the form they do.
    pub async fn move_topic_to_stage(
        &self,
        topic_id: i64,
        workflow_id: i64,
        stage_id: i64,
    ) -> Result<(), Error> {
        let info = write_info(
            "Workflows",
            "MoveWorkflowStaging",
            "workflow_staging",
            Some(topic_id),
        );
        self.move_to_stage(topic_id, workflow_id, stage_id, Some(info))
            .await
    }

    /// Takes a topic back off a workflow.
    pub async fn unstage_topic(&self, topic_id: i64, workflow_id: i64) -> Result<(), Error> {
        let mut operation = self.client().form(
            Method::DELETE,
            &format!("/topics/{topic_id}/workflows/{workflow_id}/stagings"),
        )?;
        operation.info(write_info(
            "Workflows",
            "DeleteWorkflowStaging",
            "workflow_staging",
            Some(topic_id),
        ));
        self.client().send_unit(operation).await
    }

    /// The stage selection [`Workflows::stage_topic`] and [`Workflows::move_topic_to_stage`]
    /// share. What it announces itself as is the only difference, and no announcement at all
    /// is the staging case: there it is one request inside an operation already running.
    async fn move_to_stage(
        &self,
        topic_id: i64,
        workflow_id: i64,
        stage_id: i64,
        info: Option<OperationInfo>,
    ) -> Result<(), Error> {
        let stage = stage_id.to_string();
        let mut operation = self
            .client()
            .operation(&routes::MOVE_WORKFLOW_STAGING, &[&topic_id, &workflow_id]);
        operation
            .form_representation()
            .form(&[("workflow_staging[workflow_stage_id]", stage.as_str())]);
        match info {
            Some(info) => operation.info(info),
            None => operation.quiet(),
        };
        self.client().send_unit(operation).await
    }
}

/// The workflow a row names. A row too short to carry a name, or whose first column is no
/// id, is one the autocomplete list has nothing to say about.
fn summary(row: &[String]) -> Option<WorkflowSummary> {
    match row {
        [id, name, rest @ ..] => Some(WorkflowSummary {
            id: id.parse().ok()?,
            name: name.clone(),
            account_name: rest.first().cloned().unwrap_or_default(),
        }),
        _ => None,
    }
}
