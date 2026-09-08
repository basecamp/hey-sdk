pub use crate::generated::API_VERSION;

/// The SDK's own version, from Cargo.toml.
pub const VERSION: &str = env!("CARGO_PKG_VERSION");

/// The `User-Agent` requests go out with: the SDK and the API contract it was built
/// against, which is how HEY sees what a client is working from.
pub fn default_user_agent() -> String {
    format!("hey-sdk-rust/{VERSION} (api:{API_VERSION})")
}
