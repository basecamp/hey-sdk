use std::sync::Arc;

use crate::client::{Client, ScopeState};
use crate::error::Error;
use crate::generated::types::{Account, Identity};

impl Client {
    /// Derives a client that presents mail from one linked account and acts as that
    /// account's user and default sender. The account is checked against the identity
    /// first, so a stale or foreign id fails here rather than on the first read. That
    /// check goes out through an unscoped client derived from this one: the identity
    /// belongs to the whole login, and reading it filtered to an account would leave
    /// nothing to check the account against.
    ///
    /// Calendar, journal, habits and time tracking belong to the identity, so they read
    /// the same through a scoped client.
    pub async fn for_account(&self, account_id: i64) -> Result<Client, Error> {
        if account_id <= 0 {
            return Err(Error::usage("account id must be positive"));
        }
        let root = Client {
            shared: self.shared.clone(),
            account_id: None,
            scope: Arc::default(),
        };
        let identity = root.identity().get().await?;
        let accessible = identity
            .accounts
            .iter()
            .flatten()
            .any(|account| account.id == account_id && account_is_accessible(account));
        if !accessible {
            return Err(Error::not_found("accessible account", account_id));
        }
        let scope = ScopeState::default();
        *scope.default_sender_id.lock().await = default_sender_for(&identity, Some(account_id));
        *scope.account_user_id.lock().await = identity
            .all_users
            .iter()
            .flatten()
            .find(|user| user.account_id == Some(account_id))
            .map(|user| user.id);
        Ok(Client {
            shared: self.shared.clone(),
            account_id: Some(account_id),
            scope: Arc::new(scope),
        })
    }

    /// The sender a message goes out as when the caller names none: the scoped
    /// account's default sender, or the identity's default sender for All Accounts.
    pub async fn default_sender_id(&self) -> Result<i64, Error> {
        // The lock is held across the identity read on purpose: it makes concurrent callers
        // share one read rather than each starting their own, and whoever arrives second
        // finds the answer already in hand.
        let mut cached = self.scope.default_sender_id.lock().await;
        if let Some(id) = *cached {
            return Ok(id);
        }
        let identity = self.identity().get().await?;
        let id = match (
            default_sender_for(&identity, self.account_id),
            self.account_id,
        ) {
            (Some(id), _) => id,
            (None, Some(account_id)) => {
                return Err(Error::not_found("sender for account", account_id));
            }
            (None, None) => match identity.primary_contact.as_ref().map(|contact| contact.id) {
                Some(id) => id,
                None => return Err(Error::api(0, "no sender found in identity")),
            },
        };
        *cached = Some(id);
        Ok(id)
    }

    /// The identity's user in the scoped account, which is what a record is filed under.
    pub async fn account_user_id(&self) -> Result<i64, Error> {
        let account_id = self
            .account_id
            .ok_or_else(|| Error::usage("account user id needs an account-scoped client"))?;
        // Held across the read, as in `default_sender_id`, so concurrent callers make one
        // request between them.
        let mut cached = self.scope.account_user_id.lock().await;
        if let Some(id) = *cached {
            return Ok(id);
        }
        let identity = self.identity().get().await?;
        match identity
            .all_users
            .iter()
            .flatten()
            .find(|user| user.account_id == Some(account_id))
        {
            Some(user) => {
                *cached = Some(user.id);
                Ok(user.id)
            }
            None => Err(Error::not_found("user for account", account_id)),
        }
    }
}

fn account_is_accessible(account: &Account) -> bool {
    let status = account.status.as_deref().unwrap_or_default();
    let purpose = account.purpose.as_deref().unwrap_or_default();
    status == "active" || (status == "inactive" && (purpose == "work" || purpose == "domains"))
}

fn default_sender_for(identity: &Identity, account_id: Option<i64>) -> Option<i64> {
    let senders: Vec<_> = identity
        .senders
        .iter()
        .flatten()
        .filter(|sender| account_id.is_none_or(|id| sender.account_id == Some(id)))
        .collect();
    senders
        .iter()
        .find(|sender| sender.default == Some(true))
        .or_else(|| senders.first())
        .map(|sender| sender.id)
}
