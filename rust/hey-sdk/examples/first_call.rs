//! The first call: a token from the environment, and the boxes in the account.
//!
//! ```sh
//! HEY_TOKEN=... cargo run --example first_call
//! ```

use hey_sdk::{Client, Config, Error, StaticTokenProvider};

#[tokio::main]
async fn main() -> Result<(), Error> {
    let token = std::env::var("HEY_TOKEN")
        .map_err(|_| Error::usage("set HEY_TOKEN to a HEY access token"))?;

    // `Config::default()` is app.hey.com; `Config::default().with_env()` reads HEY_BASE_URL
    // and the rest. The client is cheap to clone and safe to share across tasks.
    let client = Client::new(Config::default(), StaticTokenProvider::new(token))?;

    let me = client.identity().get().await?;
    println!("{}", me.name.unwrap_or_default());

    // A paged read answers a `Page`, which derefs to the response it wraps.
    for mailbox in client.boxes().list().await?.iter() {
        println!("{:>12}  {:<12} {}", mailbox.id, mailbox.kind, mailbox.name);
    }
    Ok(())
}
