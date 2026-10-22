//! HEY's token endpoint through the client: trade a refresh token for tokens, then sign out
//! by revoking the access token it answered, with a client that holds no credentials.
//!
//! ```sh
//! HEY_REFRESH_TOKEN=... HEY_INSTALL_ID=... cargo run --example oauth_tokens
//! ```
//!
//! Both requests go out unsigned, which is why the client needs no token to send them, and
//! follow no redirect. Anything else this client is asked to send is refused before it goes
//! out.

use hey_sdk::models::{CreateTokenRequestContent, RevokeTokenRequestContent};
use hey_sdk::{Client, Config, Error, NoCredentials, SensitiveString};

#[tokio::main]
async fn main() -> Result<(), Error> {
    let config = Config::default().with_env();
    let client_id = config.oauth_client_id.clone();
    let refresh_token = required("HEY_REFRESH_TOKEN")?;
    let install_id = required("HEY_INSTALL_ID")?;

    let client = Client::builder(config)
        .auth_strategy(NoCredentials)
        .build()?;
    let token = client
        .oauth()
        .create_token(&CreateTokenRequestContent {
            grant_type: "refresh_token".to_string(),
            client_id: client_id.clone(),
            install_id,
            refresh_token: Some(SensitiveString::new(refresh_token)),
            ..CreateTokenRequestContent::default()
        })
        .await?;
    println!(
        "Refreshed; the access token lasts {} seconds",
        token.expires_in.map_or_else(
            || "an unknown number of".to_string(),
            |seconds| seconds.to_string()
        )
    );

    client
        .oauth()
        .revoke_token(&RevokeTokenRequestContent {
            client_id,
            token: token.access_token,
        })
        .await?;
    println!("Signed out");
    Ok(())
}

fn required(name: &str) -> Result<String, Error> {
    std::env::var(name).map_err(|_| Error::usage(format!("set {name}")))
}
