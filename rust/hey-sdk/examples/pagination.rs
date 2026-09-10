//! Walking a paginated read: page by page, and to the end.
//!
//! ```sh
//! HEY_TOKEN=... cargo run --example pagination
//! ```

use hey_sdk::services::contacts::ListContactsParams;
use hey_sdk::{Client, Config, Error, StaticTokenProvider};

#[tokio::main]
async fn main() -> Result<(), Error> {
    let token = std::env::var("HEY_TOKEN")
        .map_err(|_| Error::usage("set HEY_TOKEN to a HEY access token"))?;
    let client = Client::builder(Config::default())
        .token_provider(StaticTokenProvider::new(token))
        // `each_page` and `get_all` stop here; `next_page` is the caller's to stop.
        .max_pages(50)
        .build()?;

    // A page carries the cursor HEY answered with and, where HEY sends it, the total.
    let mut page = client
        .contacts()
        .list(&ListContactsParams::default())
        .await?;
    if let Some(total) = page.total_count() {
        println!("{total} contacts");
    }

    // One page at a time, with the loop in the caller's hands.
    let mut seen = page.len();
    while let Some(next) = client.next_page(&page).await? {
        seen += next.len();
        page = next;
    }
    println!("{seen} contacts across the pages read one by one");

    // The same walk, with the client driving. The visitor answers whether to go on.
    let first = client
        .contacts()
        .list(&ListContactsParams::default())
        .await?;
    let mut names = Vec::new();
    client
        .each_page(first, |page| {
            names.extend(page.iter().filter_map(|contact| contact.name.clone()));
            true
        })
        .await?;
    println!("{} named contacts", names.len());
    Ok(())
}
