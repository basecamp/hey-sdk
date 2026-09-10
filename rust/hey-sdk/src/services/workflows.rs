//! Workflows — kanban-style boards of threads — on top of the generated workflow routes.
//!
//! A workflow has no JSON surface beyond the page that reads one and the autocomplete
//! endpoint that enumerates them, so every write is a browser form post.

use std::borrow::Cow;

use ego_tree::iter::Edge;
use scraper::{ElementRef, Html, Node, Selector};

use crate::error::Error;
use crate::generated::routes;
use crate::generated::types::WorkflowStage;
use crate::http::Method;
use crate::observability::OperationInfo;
use crate::services::write_info;

pub use crate::generated::services::workflows::*;

/// One thread on a workflow stage, as the stage page renders its card.
#[derive(Debug, Clone, Default, PartialEq, Eq, serde::Serialize)]
pub struct WorkflowStageTopic {
    /// The staging record that puts the thread on this stage, which is what
    /// [`Workflows::stage_topic`] moves.
    pub staging_id: i64,
    pub topic_id: i64,
    /// The card's title; empty when the card renders none.
    pub subject: String,
    /// How many emails the card says the thread holds; zero when the card does not say.
    pub entry_count: u64,
}

/// A workflow stage as HEY renders it — the stage page is the only place a stage's threads
/// are listed — with the cards it shows.
#[derive(Debug, Clone, Default, PartialEq, Eq, serde::Serialize)]
pub struct WorkflowStageView {
    pub id: i64,
    pub name: String,
    pub topics: Vec<WorkflowStageTopic>,
}

impl WorkflowStageView {
    /// Reads the stage out of the page HEY serves for it, the way Go's `GetStage` does: the
    /// element whose id names the stage, the first `h2` at or under it for the name, and
    /// every element under it whose id is `topic_<id>` for a card — the outermost such
    /// element, since a card inside a card is that card's content. A card is skipped when
    /// its thread or staging id will not parse as a positive number, or when its detail
    /// line does not start with a count. Text meant for screen readers is left out of
    /// names and subjects. Every rule is Go's, so the two SDKs read one page the same way.
    pub fn parse(html: &str, stage_id: i64) -> Result<WorkflowStageView, Error> {
        let document = Html::parse_document(html);
        let stage = document
            .select(&selector(&format!(
                "[id=\"container_workflow_stage_{stage_id}\"]"
            )))
            .next()
            .ok_or_else(|| Error::not_found("workflow stage", stage_id))?;
        let name = first_at_or_under(stage, |element| element.value().name() == "h2")
            .map(visible_text)
            .unwrap_or_default();
        let topics = stage
            .select(&selector("[id^=\"topic_\"]"))
            .filter(|card| !inside_another_card(*card, stage))
            .filter_map(topic)
            .collect();
        Ok(WorkflowStageView {
            id: stage_id,
            name,
            topics,
        })
    }
}

fn topic(card: ElementRef<'_>) -> Option<WorkflowStageTopic> {
    let topic_id = positive(card.attr("id")?.strip_prefix("topic_")?)?;
    let staging_id = positive(card.attr("data-identifier")?)?;
    let subject = first_at_or_under(card, |element| element.value().name() == "h3")
        .map(visible_text)
        .unwrap_or_default();
    let entry_count = match first_at_or_under(card, is_detail_line) {
        None => 0,
        Some(detail) => visible_text(detail)
            .split_whitespace()
            .next()?
            .parse::<i64>()
            .ok()
            .and_then(|count| u64::try_from(count).ok())?,
    };
    Some(WorkflowStageTopic {
        staging_id,
        topic_id,
        subject,
        entry_count,
    })
}

/// A `p` whose class mentions `card__detail`, as Go matches it: a substring, so a
/// modifier class on the element still counts.
fn is_detail_line(element: ElementRef<'_>) -> bool {
    element.value().name() == "p"
        && element
            .attr("class")
            .is_some_and(|class| class.contains("card__detail"))
}

fn positive(value: &str) -> Option<i64> {
    value.parse::<i64>().ok().filter(|id| *id > 0)
}

/// The first element at or under `root`, in document order, that `matches` — the element
/// itself included, as Go's `findNode` includes it.
fn first_at_or_under<'a>(
    root: ElementRef<'a>,
    matches: impl Fn(ElementRef<'a>) -> bool,
) -> Option<ElementRef<'a>> {
    root.descendants()
        .filter_map(ElementRef::wrap)
        .find(|element| matches(*element))
}

/// Whether a card sits inside another card of the same stage: the walk that finds cards
/// stops at each one, so a card rendered inside a card is that card's content, not a card
/// of its own. Only the stage's own subtree counts; what surrounds the stage is not a card.
fn inside_another_card(card: ElementRef<'_>, stage: ElementRef<'_>) -> bool {
    card.ancestors()
        .take_while(|ancestor| ancestor.id() != stage.id())
        .filter_map(ElementRef::wrap)
        .any(|ancestor| {
            ancestor
                .attr("id")
                .is_some_and(|id| id.starts_with("topic_"))
        })
}

/// The text a reader sees under an element, whitespace collapsed: text meant for screen
/// readers only is left out. Walked without recursion, since the page is the server's and
/// its nesting is not bounded.
fn visible_text(element: ElementRef<'_>) -> String {
    let mut text = String::new();
    let mut hidden_depth = 0usize;
    for edge in element.traverse() {
        match edge {
            Edge::Open(node) => {
                if hidden_depth > 0 {
                    hidden_depth += 1;
                } else {
                    match node.value() {
                        Node::Element(element) if is_visually_hidden(element) => {
                            hidden_depth = 1;
                        }
                        Node::Text(content) => text.push_str(content),
                        _ => {}
                    }
                }
            }
            Edge::Close(_) => hidden_depth = hidden_depth.saturating_sub(1),
        }
    }
    text.split_whitespace().collect::<Vec<_>>().join(" ")
}

/// Split on whitespace as Go's `strings.Fields` splits a class attribute: any Unicode
/// whitespace, not only the ASCII the HTML spec names.
fn is_visually_hidden(element: &scraper::node::Element) -> bool {
    element.attr("class").is_some_and(|classes| {
        classes.split_whitespace().any(|class| {
            matches!(
                class,
                "sr-only" | "screen-reader-only" | "u-for-screen-reader" | "visually-hidden"
            )
        })
    })
}

/// A selector written here, which is why parsing it cannot fail.
fn selector(css: &str) -> Selector {
    Selector::parse(css).unwrap_or_else(|error| unreachable!("selector {css:?}: {error}"))
}

/// A workflow as the autocomplete endpoint names it.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
#[non_exhaustive]
pub struct WorkflowSummary {
    /// The workflow's id.
    pub id: i64,
    /// What the workflow is called.
    pub name: String,
    /// The account the workflow belongs to, empty when the row names none.
    pub account_name: String,
}

impl Workflows<'_> {
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

    /// One stage and the threads on it, read out of the page HEY serves for the stage —
    /// what [`Workflows::get_stage`] answers as HTML, parsed. HEY lists a stage's threads
    /// nowhere else.
    pub async fn stage(&self, workflow_id: i64, stage_id: i64) -> Result<WorkflowStageView, Error> {
        let page = self.get_stage(workflow_id, stage_id).await?;
        WorkflowStageView::parse(&page, stage_id)
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
