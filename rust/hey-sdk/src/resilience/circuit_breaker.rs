use std::sync::Mutex;
use std::time::{Duration, Instant};

use super::Clock;

/// How much failure it takes to give up on an operation, and how much success it takes to
/// come back.
///
/// A value of zero, or a rate that is not a positive percentage, reads as "leave it at the
/// default" — the same normalising Go does when it is handed a part-filled config.
#[derive(Debug, Clone)]
pub struct CircuitBreakerConfig {
    /// Consecutive failures that open the circuit.
    pub failure_threshold: u32,
    /// Successes in a row that close a half-open circuit.
    pub success_threshold: u32,
    /// How long the circuit stays open before it lets one call through to find out.
    pub open_timeout: Duration,
    /// The share of a full window, as a percentage, that opens the circuit however few
    /// failures came in a row.
    pub failure_rate_threshold: f64,
    /// How many outcomes that rate is measured over.
    pub sliding_window_size: usize,
    pub clock: Clock,
}

impl Default for CircuitBreakerConfig {
    fn default() -> CircuitBreakerConfig {
        CircuitBreakerConfig {
            failure_threshold: 5,
            success_threshold: 2,
            open_timeout: Duration::from_secs(30),
            failure_rate_threshold: 50.0,
            sliding_window_size: 10,
            clock: Clock::default(),
        }
    }
}

impl CircuitBreakerConfig {
    fn normalised(self) -> CircuitBreakerConfig {
        let defaults = CircuitBreakerConfig::default();
        CircuitBreakerConfig {
            failure_threshold: nonzero(self.failure_threshold, defaults.failure_threshold),
            success_threshold: nonzero(self.success_threshold, defaults.success_threshold),
            open_timeout: nonzero(self.open_timeout, defaults.open_timeout),
            failure_rate_threshold: if self.failure_rate_threshold > 0.0 {
                self.failure_rate_threshold
            } else {
                defaults.failure_rate_threshold
            },
            sliding_window_size: nonzero(self.sliding_window_size, defaults.sliding_window_size),
            clock: self.clock,
        }
    }
}

fn nonzero<T: Default + PartialEq>(value: T, fallback: T) -> T {
    if value == T::default() {
        fallback
    } else {
        value
    }
}

/// One scope's opinion of whether HEY is worth calling.
///
/// It starts closed, letting everything through. Enough failures — either
/// [`CircuitBreakerConfig::failure_threshold`] in a row, or a full window failing at
/// [`CircuitBreakerConfig::failure_rate_threshold`] — open it, and it refuses everything
/// until [`CircuitBreakerConfig::open_timeout`] has passed. Then it goes half-open and lets
/// calls through again: [`CircuitBreakerConfig::success_threshold`] of them succeeding
/// closes it, and one failing opens it for another timeout.
pub struct CircuitBreaker {
    config: CircuitBreakerConfig,
    inner: Mutex<Inner>,
}

struct Inner {
    state: State,
    window: Vec<bool>,
    index: usize,
    filled: bool,
}

#[derive(Clone, Copy)]
enum State {
    Closed { failures: u32 },
    Open { since: Instant },
    HalfOpen { successes: u32 },
}

impl CircuitBreaker {
    pub fn new(config: CircuitBreakerConfig) -> CircuitBreaker {
        let config = config.normalised();
        let inner = Inner {
            state: State::Closed { failures: 0 },
            window: vec![true; config.sliding_window_size],
            index: 0,
            filled: false,
        };
        CircuitBreaker {
            config,
            inner: Mutex::new(inner),
        }
    }

    /// Whether a call may go out. An open circuit that has served its timeout goes half-open
    /// here, on the way past, rather than on a timer of its own.
    pub fn allow(&self) -> bool {
        let mut inner = self.inner.lock().unwrap();
        match inner.state {
            State::Closed { .. } | State::HalfOpen { .. } => true,
            State::Open { since } => {
                if self.config.clock.now().saturating_duration_since(since)
                    >= self.config.open_timeout
                {
                    inner.state = State::HalfOpen { successes: 0 };
                    true
                } else {
                    false
                }
            }
        }
    }

    pub fn record_success(&self) {
        let mut inner = self.inner.lock().unwrap();
        inner.record(true);
        inner.state = match inner.state {
            State::HalfOpen { successes } if successes + 1 >= self.config.success_threshold => {
                State::Closed { failures: 0 }
            }
            State::HalfOpen { successes } => State::HalfOpen {
                successes: successes + 1,
            },
            State::Closed { .. } => State::Closed { failures: 0 },
            open @ State::Open { .. } => open,
        };
    }

    pub fn record_failure(&self) {
        let now = self.config.clock.now();
        let mut inner = self.inner.lock().unwrap();
        inner.record(false);
        inner.state = match inner.state {
            State::Closed { failures }
                if failures + 1 >= self.config.failure_threshold
                    || inner.failure_rate() >= self.config.failure_rate_threshold =>
            {
                State::Open { since: now }
            }
            State::Closed { failures } => State::Closed {
                failures: failures + 1,
            },
            State::HalfOpen { .. } | State::Open { .. } => State::Open { since: now },
        };
    }

    /// The state as the other SDKs name it: `closed`, `open` or `half-open`.
    pub fn state(&self) -> &'static str {
        match self.inner.lock().unwrap().state {
            State::Closed { .. } => "closed",
            State::Open { .. } => "open",
            State::HalfOpen { .. } => "half-open",
        }
    }
}

impl Inner {
    fn record(&mut self, success: bool) {
        self.window[self.index] = success;
        self.index = (self.index + 1) % self.window.len();
        if self.index == 0 {
            self.filled = true;
        }
    }

    /// The share of the window that failed, as a percentage. A window that has not been
    /// round once yet has nothing to say, so it answers zero.
    fn failure_rate(&self) -> f64 {
        if self.filled {
            let failures = self.window.iter().filter(|success| !**success).count();
            failures as f64 / self.window.len() as f64 * 100.0
        } else {
            0.0
        }
    }
}

#[cfg(test)]
mod tests {
    use super::super::{advance, test_clock};
    use super::*;

    #[test]
    fn a_new_breaker_is_closed_and_lets_everything_through() {
        let breaker = CircuitBreaker::new(CircuitBreakerConfig::default());

        assert!(breaker.allow());
        assert_eq!("closed", breaker.state());
    }

    #[test]
    fn enough_failures_in_a_row_open_it() {
        let breaker = CircuitBreaker::new(CircuitBreakerConfig {
            failure_threshold: 3,
            ..CircuitBreakerConfig::default()
        });

        breaker.record_failure();
        breaker.record_failure();
        assert_eq!("closed", breaker.state());

        breaker.record_failure();

        assert_eq!("open", breaker.state());
        assert!(!breaker.allow());
    }

    #[test]
    fn a_success_in_between_starts_the_count_again() {
        let breaker = CircuitBreaker::new(CircuitBreakerConfig {
            failure_threshold: 3,
            ..CircuitBreakerConfig::default()
        });

        breaker.record_failure();
        breaker.record_failure();
        breaker.record_success();
        breaker.record_failure();
        breaker.record_failure();

        assert_eq!("closed", breaker.state());
    }

    #[test]
    fn an_open_breaker_goes_half_open_once_its_timeout_has_passed() {
        let (clock, now) = test_clock();
        let breaker = CircuitBreaker::new(CircuitBreakerConfig {
            failure_threshold: 2,
            success_threshold: 1,
            open_timeout: Duration::from_millis(100),
            clock,
            ..CircuitBreakerConfig::default()
        });

        breaker.record_failure();
        breaker.record_failure();
        assert_eq!("open", breaker.state());
        assert!(!breaker.allow());

        advance(&now, Duration::from_millis(200));

        assert!(breaker.allow());
        assert_eq!("half-open", breaker.state());
    }

    #[test]
    fn enough_successes_while_half_open_close_it() {
        let (clock, now) = test_clock();
        let breaker = CircuitBreaker::new(CircuitBreakerConfig {
            failure_threshold: 2,
            success_threshold: 2,
            open_timeout: Duration::from_millis(100),
            clock,
            ..CircuitBreakerConfig::default()
        });

        breaker.record_failure();
        breaker.record_failure();
        advance(&now, Duration::from_millis(200));
        breaker.allow();

        breaker.record_success();
        assert_eq!("half-open", breaker.state());
        breaker.record_success();

        assert_eq!("closed", breaker.state());
    }

    #[test]
    fn one_failure_while_half_open_opens_it_again() {
        let (clock, now) = test_clock();
        let breaker = CircuitBreaker::new(CircuitBreakerConfig {
            failure_threshold: 2,
            success_threshold: 2,
            open_timeout: Duration::from_millis(100),
            clock,
            ..CircuitBreakerConfig::default()
        });

        breaker.record_failure();
        breaker.record_failure();
        advance(&now, Duration::from_millis(200));
        breaker.allow();

        breaker.record_failure();

        assert_eq!("open", breaker.state());
        assert!(!breaker.allow());
    }

    /// Failures spread out among successes never reach the consecutive threshold, which is
    /// what the rate over a full window is for.
    #[test]
    fn a_full_window_failing_too_often_opens_it_however_the_failures_fell() {
        let breaker = CircuitBreaker::new(CircuitBreakerConfig {
            failure_threshold: 100,
            failure_rate_threshold: 50.0,
            sliding_window_size: 4,
            ..CircuitBreakerConfig::default()
        });

        breaker.record_success();
        breaker.record_success();
        breaker.record_failure();
        assert_eq!("closed", breaker.state());

        breaker.record_failure();

        assert_eq!("open", breaker.state());
    }

    #[test]
    fn a_config_of_zeroes_falls_back_to_the_defaults() {
        let breaker = CircuitBreaker::new(CircuitBreakerConfig {
            failure_threshold: 0,
            success_threshold: 0,
            open_timeout: Duration::ZERO,
            failure_rate_threshold: 0.0,
            sliding_window_size: 0,
            clock: Clock::default(),
        });

        for _ in 0..4 {
            breaker.record_failure();
        }
        assert_eq!("closed", breaker.state());

        breaker.record_failure();

        assert_eq!("open", breaker.state());
    }
}
