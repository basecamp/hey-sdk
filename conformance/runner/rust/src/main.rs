//! The conformance runner for the HEY Rust SDK.
//!
//! It reads the shared case definitions from `conformance/tests/` and runs each one
//! against the SDK, with a loopback mock server standing in for HEY.

mod assertions;
mod fixtures;
mod operations;
mod server;

use std::io;
use std::path::{Path, PathBuf};
use std::process::ExitCode;

use hey_sdk::{Client, Config, StaticTokenProvider};

use assertions::Run;
use fixtures::TestCase;
use server::MockServer;

#[tokio::main]
async fn main() -> ExitCode {
    let directory = Path::new(env!("CARGO_MANIFEST_DIR")).join("../../tests");
    let files = match case_files(&directory) {
        Ok(files) => files,
        Err(error) => {
            eprintln!("Error finding test files: {error}");
            return ExitCode::FAILURE;
        }
    };
    if files.is_empty() {
        println!("No test files found in {}", directory.display());
        return ExitCode::SUCCESS;
    }

    let mut passed = 0;
    let mut failed = 0;
    for file in &files {
        println!(
            "\n=== {} ===",
            file.file_name().unwrap_or_default().display()
        );
        // A file the runner cannot read or parse is a failure, not a file with no cases in
        // it: skipping it quietly would let a broken fixture pass the gate.
        let cases = match load_cases(file) {
            Ok(cases) => cases,
            Err(error) => {
                failed += 1;
                println!("  FAIL: {}\n        {error}", file.display());
                continue;
            }
        };
        for case in &cases {
            match run_case(case).await {
                Ok(()) => {
                    passed += 1;
                    println!("  PASS: {}", case.name);
                }
                Err(message) => {
                    failed += 1;
                    println!("  FAIL: {}\n        {message}", case.name);
                }
            }
        }
    }

    println!("\n=== Summary ===");
    println!(
        "Passed: {passed}, Failed: {failed}, Total: {}",
        passed + failed
    );
    if failed > 0 {
        ExitCode::FAILURE
    } else {
        ExitCode::SUCCESS
    }
}

fn case_files(directory: &Path) -> Result<Vec<PathBuf>, io::Error> {
    let mut files: Vec<PathBuf> = std::fs::read_dir(directory)?
        .filter_map(Result::ok)
        .map(|entry| entry.path())
        .filter(|path| {
            path.extension()
                .is_some_and(|extension| extension == "json")
        })
        .collect();
    files.sort();
    Ok(files)
}

fn load_cases(file: &Path) -> Result<Vec<TestCase>, String> {
    let contents = std::fs::read(file).map_err(|error| error.to_string())?;
    serde_json::from_slice(&contents).map_err(|error| error.to_string())
}

async fn run_case(case: &TestCase) -> Result<(), String> {
    match &case.config_overrides.base_url {
        Some(base_url) => run_config_override_case(case, base_url),
        None => run_mock_server_case(case).await,
    }
}

/// A case that overrides the base URL never reaches a server: it asks what the client does
/// with an endpoint it should refuse.
fn run_config_override_case(case: &TestCase, base_url: &str) -> Result<(), String> {
    let client = Client::builder(Config::default().with_base_url(base_url))
        .token_provider(StaticTokenProvider::new("test-token"))
        .build();

    for assertion in &case.assertions {
        match assertion.kind.as_str() {
            "requestCount" => match assertion.expected.as_i64() {
                Some(0) => {}
                Some(expected) => {
                    return Err(format!(
                        "Expected 0 requests for config override test, got expectation of {expected}"
                    ));
                }
                None => return Err("requestCount: expected an integer".to_string()),
            },
            "errorCode" => match (&client, assertion.expected.as_str()) {
                (Ok(_), _) => {
                    return Err(
                        "Expected configuration error, but client was created successfully"
                            .to_string(),
                    );
                }
                (Err(error), Some(expected)) if error.code().as_str() == expected => {}
                (Err(error), Some(expected)) => {
                    return Err(format!(
                        "Expected error code {expected:?}, got {:?}",
                        error.code().as_str()
                    ));
                }
                (Err(_), None) => return Err("errorCode: expected a string".to_string()),
            },
            "noError" => {
                if let Err(error) = &client {
                    return Err(format!("Expected no error, got: {error}"));
                }
            }
            kind => return Err(format!("Unknown assertion type: {kind}")),
        }
    }
    Ok(())
}

async fn run_mock_server_case(case: &TestCase) -> Result<(), String> {
    let server = MockServer::start(case.mock_responses.clone())
        .await
        .map_err(|error| format!("Failed to start the mock server: {error}"))?;
    let base_url = server.base_url().to_string();
    let outcome = operations::execute_case(case, &base_url).await;
    let recorded = server.shutdown();
    assertions::check_all(&Run {
        case,
        outcome: &outcome,
        recorded: &recorded,
        base_url: &base_url,
    })
}
