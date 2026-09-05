use std::sync::Mutex;
use std::time::{Duration, Instant};

use super::Clock;

/// The budget a client holds itself to, and whether it takes HEY's word over its own.
///
/// A rate or a burst that is not positive reads as "leave it at the default", the same
/// normalising Go does.
#[derive(Debug, Clone)]
pub struct RateLimitConfig {
    /// How fast the bucket refills.
    pub requests_per_second: f64,
    /// How many requests can go out at once from a full bucket.
    pub burst_size: u32,
    /// Whether a `Retry-After` from HEY holds the limiter back until it has passed.
    pub respect_retry_after: bool,
    pub clock: Clock,
}

impl Default for RateLimitConfig {
    fn default() -> RateLimitConfig {
        RateLimitConfig {
            requests_per_second: 50.0,
            burst_size: 10,
            respect_retry_after: true,
            clock: Clock::default(),
        }
    }
}

/// A token bucket, and whatever wait HEY last asked for.
///
/// The bucket starts full at [`RateLimitConfig::burst_size`] and refills at
/// [`RateLimitConfig::requests_per_second`]; every call spends a token. While a
/// `Retry-After` is in force nothing goes out at all, however many tokens have piled up.
pub struct RateLimiter {
    requests_per_second: f64,
    burst_size: f64,
    respect_retry_after: bool,
    clock: Clock,
    inner: Mutex<Bucket>,
}

struct Bucket {
    tokens: f64,
    last_refill: Instant,
    retry_after_until: Option<Instant>,
}

impl RateLimiter {
    pub fn new(config: RateLimitConfig) -> RateLimiter {
        let defaults = RateLimitConfig::default();
        let burst_size = match config.burst_size {
            0 => defaults.burst_size,
            burst_size => burst_size,
        };
        let bucket = Bucket {
            tokens: f64::from(burst_size),
            last_refill: config.clock.now(),
            retry_after_until: None,
        };
        RateLimiter {
            requests_per_second: if config.requests_per_second > 0.0 {
                config.requests_per_second
            } else {
                defaults.requests_per_second
            },
            burst_size: f64::from(burst_size),
            respect_retry_after: config.respect_retry_after,
            clock: config.clock,
            inner: Mutex::new(bucket),
        }
    }

    /// Whether a call may go out now, spending a token if it may.
    pub fn allow(&self) -> bool {
        let mut bucket = self.inner.lock().unwrap();
        if self.held_back(&mut bucket) {
            false
        } else {
            self.refill(&mut bucket);
            if bucket.tokens >= 1.0 {
                bucket.tokens -= 1.0;
                true
            } else {
                false
            }
        }
    }

    /// How long a call would have to wait for its token: `Some(Duration::ZERO)` to go now,
    /// and `None` when the limiter will not have one within the second — or when HEY has
    /// asked for a wait, which the caller should sit out rather than shorten.
    ///
    /// A reservation spends its token up front, as Go's does, so a caller that reserves and
    /// then walks away leaves the bucket short.
    pub fn reserve(&self) -> Option<Duration> {
        let mut bucket = self.inner.lock().unwrap();
        if self.held_back(&mut bucket) {
            return None;
        }

        self.refill(&mut bucket);
        if bucket.tokens >= 1.0 {
            bucket.tokens -= 1.0;
            return Some(Duration::ZERO);
        }

        let wait = Duration::from_secs_f64((1.0 - bucket.tokens) / self.requests_per_second);
        if wait > Duration::from_secs(1) {
            None
        } else {
            bucket.tokens -= 1.0;
            Some(wait)
        }
    }

    /// Holds every call back until the given moment. A wait already in force is only ever
    /// extended, never cut short, so the longest thing HEY asked for is what is honoured.
    pub fn set_retry_after(&self, until: Instant) {
        if self.respect_retry_after {
            let mut bucket = self.inner.lock().unwrap();
            if bucket
                .retry_after_until
                .is_none_or(|current| until > current)
            {
                bucket.retry_after_until = Some(until);
            }
        }
    }

    pub fn set_retry_after_in(&self, wait: Duration) {
        self.set_retry_after(self.clock.now() + wait);
    }

    /// How much of the wait HEY asked for is left, and zero when it asked for none.
    pub fn retry_after_remaining(&self) -> Duration {
        match self.inner.lock().unwrap().retry_after_until {
            Some(until) => until.saturating_duration_since(self.clock.now()),
            None => Duration::ZERO,
        }
    }

    /// How many calls the bucket would let through right now.
    pub fn tokens(&self) -> f64 {
        let mut bucket = self.inner.lock().unwrap();
        self.refill(&mut bucket);
        bucket.tokens
    }

    /// Whether HEY's wait is still in force, forgetting one that has passed on the way.
    fn held_back(&self, bucket: &mut Bucket) -> bool {
        match bucket.retry_after_until {
            Some(until) if self.respect_retry_after => {
                if self.clock.now() < until {
                    true
                } else {
                    bucket.retry_after_until = None;
                    false
                }
            }
            _ => false,
        }
    }

    fn refill(&self, bucket: &mut Bucket) {
        let now = self.clock.now();
        let elapsed = now.saturating_duration_since(bucket.last_refill);
        bucket.last_refill = now;
        bucket.tokens =
            (bucket.tokens + elapsed.as_secs_f64() * self.requests_per_second).min(self.burst_size);
    }
}

#[cfg(test)]
mod tests {
    use super::super::{advance, test_clock};
    use super::*;

    #[test]
    fn a_burst_goes_out_at_once_and_the_next_call_waits_for_the_refill() {
        let (clock, now) = test_clock();
        let limiter = RateLimiter::new(RateLimitConfig {
            requests_per_second: 100.0,
            burst_size: 5,
            clock,
            ..RateLimitConfig::default()
        });

        for spent in 1..=5 {
            assert!(limiter.allow(), "call {spent} of the burst");
        }
        assert!(!limiter.allow());

        advance(&now, Duration::from_millis(20));

        assert!(limiter.allow());
    }

    #[test]
    fn a_wait_hey_asked_for_holds_everything_back_until_it_has_passed() {
        let (clock, now) = test_clock();
        let limiter = RateLimiter::new(RateLimitConfig {
            requests_per_second: 1000.0,
            burst_size: 100,
            respect_retry_after: true,
            clock,
        });

        limiter.set_retry_after_in(Duration::from_secs(1));

        assert!(!limiter.allow());
        assert!(limiter.retry_after_remaining() > Duration::ZERO);

        advance(&now, Duration::from_secs(2));

        assert!(limiter.allow());
        assert_eq!(Duration::ZERO, limiter.retry_after_remaining());
    }

    #[test]
    fn a_limiter_told_not_to_respect_the_wait_carries_on() {
        let (clock, _now) = test_clock();
        let limiter = RateLimiter::new(RateLimitConfig {
            respect_retry_after: false,
            clock,
            ..RateLimitConfig::default()
        });

        limiter.set_retry_after_in(Duration::from_secs(60));

        assert!(limiter.allow());
        assert_eq!(Duration::ZERO, limiter.retry_after_remaining());
    }

    #[test]
    fn a_longer_wait_replaces_a_shorter_one_and_a_shorter_one_is_ignored() {
        let (clock, _now) = test_clock();
        let limiter = RateLimiter::new(RateLimitConfig {
            clock,
            ..RateLimitConfig::default()
        });

        limiter.set_retry_after_in(Duration::from_secs(30));
        limiter.set_retry_after_in(Duration::from_secs(5));
        assert_eq!(Duration::from_secs(30), limiter.retry_after_remaining());

        limiter.set_retry_after_in(Duration::from_secs(60));
        assert_eq!(Duration::from_secs(60), limiter.retry_after_remaining());
    }

    #[test]
    fn a_new_bucket_is_full_and_every_call_spends_from_it() {
        let (clock, _now) = test_clock();
        let limiter = RateLimiter::new(RateLimitConfig {
            requests_per_second: 100.0,
            burst_size: 10,
            clock,
            ..RateLimitConfig::default()
        });

        assert_eq!(10.0, limiter.tokens());

        limiter.allow();

        assert_eq!(9.0, limiter.tokens());
    }

    #[test]
    fn a_reservation_answers_the_wait_its_token_costs() {
        let (clock, _now) = test_clock();
        let limiter = RateLimiter::new(RateLimitConfig {
            requests_per_second: 10.0,
            burst_size: 1,
            clock,
            ..RateLimitConfig::default()
        });

        assert_eq!(Some(Duration::ZERO), limiter.reserve());
        assert_eq!(Some(Duration::from_millis(100)), limiter.reserve());
        assert_eq!(Some(Duration::from_millis(200)), limiter.reserve());
    }

    #[test]
    fn a_reservation_beyond_a_second_out_is_refused() {
        let (clock, _now) = test_clock();
        let limiter = RateLimiter::new(RateLimitConfig {
            requests_per_second: 0.5,
            burst_size: 1,
            clock,
            ..RateLimitConfig::default()
        });

        assert_eq!(Some(Duration::ZERO), limiter.reserve());

        assert_eq!(None, limiter.reserve());
    }

    #[test]
    fn a_reservation_is_refused_while_hey_has_asked_for_a_wait() {
        let (clock, _now) = test_clock();
        let limiter = RateLimiter::new(RateLimitConfig {
            clock,
            ..RateLimitConfig::default()
        });

        limiter.set_retry_after_in(Duration::from_secs(30));

        assert_eq!(None, limiter.reserve());
    }

    #[test]
    fn a_config_of_zeroes_falls_back_to_the_defaults() {
        let (clock, _now) = test_clock();
        let limiter = RateLimiter::new(RateLimitConfig {
            requests_per_second: 0.0,
            burst_size: 0,
            respect_retry_after: true,
            clock,
        });

        assert_eq!(10.0, limiter.tokens());
    }
}
