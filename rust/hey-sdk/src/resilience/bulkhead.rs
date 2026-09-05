use std::sync::Arc;
use std::time::Duration;

use tokio::sync::{OwnedSemaphorePermit, Semaphore};

use crate::error::Error;

/// How many calls of one kind may run at once, and how long a caller waits for room.
///
/// `max_concurrent` of zero reads as "leave it at the default", the same normalising Go
/// does. A `max_wait` of zero means a caller never waits: a full bulkhead refuses at once.
#[derive(Debug, Clone)]
pub struct BulkheadConfig {
    pub max_concurrent: usize,
    /// How long [`Bulkhead::acquire`] waits for a permit, and so how long the client's own
    /// gate holds a call back before refusing it.
    pub max_wait: Duration,
}

impl Default for BulkheadConfig {
    fn default() -> BulkheadConfig {
        BulkheadConfig {
            max_concurrent: 10,
            max_wait: Duration::from_secs(5),
        }
    }
}

/// One scope's ration of calls in flight. Every call holds a [`BulkheadPermit`] for as long
/// as it runs, and the scope refuses the rest until one is dropped, so a slow operation
/// cannot take every connection the client has.
pub struct Bulkhead {
    max_concurrent: usize,
    max_wait: Duration,
    permits: Arc<Semaphore>,
}

/// A place in the bulkhead, given back by dropping it.
pub type BulkheadPermit = OwnedSemaphorePermit;

impl Bulkhead {
    pub fn new(config: BulkheadConfig) -> Bulkhead {
        let max_concurrent = match config.max_concurrent {
            0 => BulkheadConfig::default().max_concurrent,
            max_concurrent => max_concurrent,
        };
        Bulkhead {
            max_concurrent,
            max_wait: config.max_wait,
            permits: Arc::new(Semaphore::new(max_concurrent)),
        }
    }

    /// A permit, waiting up to [`BulkheadConfig::max_wait`] for one to free up. Answers
    /// [`Error::bulkhead_full`] when the wait runs out, or straight away when there is no
    /// wait to spend.
    pub async fn acquire(&self) -> Result<BulkheadPermit, Error> {
        match self.try_acquire() {
            Some(permit) => Ok(permit),
            None if self.max_wait.is_zero() => Err(Error::bulkhead_full()),
            None => tokio::time::timeout(self.max_wait, self.permits.clone().acquire_owned())
                .await
                .map_err(|_| Error::bulkhead_full())?
                .map_err(|_| Error::bulkhead_full()),
        }
    }

    /// A permit if the scope has room this instant, and nothing if it does not.
    pub fn try_acquire(&self) -> Option<BulkheadPermit> {
        self.permits.clone().try_acquire_owned().ok()
    }

    /// How many more calls this scope will take right now.
    pub fn available(&self) -> usize {
        self.permits.available_permits()
    }

    /// How many calls this scope is running right now.
    pub fn in_use(&self) -> usize {
        self.max_concurrent - self.available()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn a_full_bulkhead_refuses_until_a_call_finishes() {
        let bulkhead = Bulkhead::new(BulkheadConfig {
            max_concurrent: 2,
            max_wait: Duration::ZERO,
        });

        let first = bulkhead.acquire().await.unwrap();
        let second = bulkhead.acquire().await.unwrap();
        assert_eq!(2, bulkhead.in_use());
        assert_eq!(0, bulkhead.available());

        let refused = bulkhead.acquire().await.unwrap_err();
        assert_eq!(crate::ErrorCode::BulkheadFull, refused.code());
        assert_eq!("bulkhead is full", refused.message());

        drop(first);
        assert_eq!(1, bulkhead.in_use());

        let third = bulkhead.acquire().await.unwrap();
        drop((second, third));
        assert_eq!(0, bulkhead.in_use());
    }

    #[test]
    fn try_acquire_answers_without_waiting() {
        let bulkhead = Bulkhead::new(BulkheadConfig {
            max_concurrent: 1,
            max_wait: Duration::from_secs(5),
        });

        let permit = bulkhead.try_acquire();
        assert!(permit.is_some());
        assert!(bulkhead.try_acquire().is_none());

        drop(permit);
        assert!(bulkhead.try_acquire().is_some());
    }

    #[tokio::test]
    async fn a_caller_that_may_wait_gets_the_permit_the_call_before_it_gives_back() {
        let bulkhead = Arc::new(Bulkhead::new(BulkheadConfig {
            max_concurrent: 1,
            max_wait: Duration::from_millis(500),
        }));

        let held = bulkhead.acquire().await.unwrap();
        tokio::spawn(async move {
            tokio::time::sleep(Duration::from_millis(10)).await;
            drop(held);
        });

        assert!(bulkhead.acquire().await.is_ok());
    }

    #[tokio::test]
    async fn a_caller_that_waits_too_long_is_refused() {
        let bulkhead = Bulkhead::new(BulkheadConfig {
            max_concurrent: 1,
            max_wait: Duration::from_millis(10),
        });
        let _held = bulkhead.acquire().await.unwrap();

        let refused = bulkhead.acquire().await.unwrap_err();

        assert_eq!(crate::ErrorCode::BulkheadFull, refused.code());
    }

    #[test]
    fn a_config_of_zero_falls_back_to_the_default_width() {
        let bulkhead = Bulkhead::new(BulkheadConfig {
            max_concurrent: 0,
            max_wait: Duration::ZERO,
        });

        assert_eq!(10, bulkhead.available());
    }
}
