package hey

import (
	"context"
	"sync"
	"time"
)

// generation is the credentials a request went out with, by the counts at its signing:
// how many refreshes had renewed them, and how many refreshes had run at all. The first
// says whether a 401 is already answered by someone else's refresh; the second whether a
// refresh of these very credentials already ran and failed, in which case its answer is
// this request's too.
type generation struct {
	refreshes uint64
	runs      uint64
}

// credentialRefresh coordinates the refreshes a client's 401s ask for, so that a set of
// credentials is refreshed once however many requests were signed with it. It is shared
// by every client derived from the same root, as the credentials are.
//
// Signing and refreshing go one at a time, and never together, as they do in the Kotlin
// and Rust clients: a request is signed with no other signing and no refresh under way,
// and the strategy is asked for the credentials and the generation read in that one
// turn, so the credentials a request went out with and the counts it was signed under
// cannot disagree, and a refresh holds the same turn for its whole run. The turn is the
// coordinator's own rather than a lock, so a request that arrives while a refresh runs
// waits on the refresh or on its own context, whichever ends first.
type credentialRefresh struct {
	// mu guards every field and is held only for a moment, so a stale request can find
	// the refresh in flight while it runs.
	mu sync.Mutex
	// refreshes counts the refreshes that renewed the credentials.
	refreshes uint64
	// runs counts the refreshes that ran to an answer, renewed or not.
	runs uint64
	// inFlight is the refresh running now, for every stale request to wait on and for
	// every new one to be signed after; nil between refreshes.
	inFlight *refreshRun
	// renewed is how the last refresh ended.
	renewed bool
	// busy is set while a signing or a refresh holds the turn; freed is closed when it
	// is given up while anything waits for it, and nil when nothing does.
	busy  bool
	freed chan struct{}
	// credential is the last credential a request was signed with, when the strategy
	// names one: a signing that yields a different one has found the credentials
	// renewed on the provider's own account, and is counted as a refresh.
	credential string
}

// refreshRun is one refresh, for the requests that share its answer to wait on: those
// signed under from, the generation the 401 that started it was signed under. A request
// signed under a newer generation waits for the run to end and then asks again, since
// the run's answer is for credentials older than its own.
type refreshRun struct {
	from    generation
	done    chan struct{}
	renewed bool
}

// generation is the state of the counts now.
func (r *credentialRefresh) generation() generation {
	r.mu.Lock()
	defer r.mu.Unlock()
	return generation{refreshes: r.refreshes, runs: r.runs}
}

// take waits for the turn, with mu held on entry and on return, and reports false when
// ctx ends first. A signer also waits out any refresh in flight, so the refresh is
// always next when the turn is given up, and no request is signed while it runs.
func (r *credentialRefresh) take(ctx context.Context, signer bool) bool {
	for r.busy || (signer && r.inFlight != nil) {
		var wait <-chan struct{}
		if r.busy {
			if r.freed == nil {
				r.freed = make(chan struct{})
			}
			wait = r.freed
		} else {
			wait = r.inFlight.done
		}
		r.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
			r.mu.Lock()
			return false
		}
		r.mu.Lock()
	}
	r.busy = true
	return true
}

// give gives the turn up, with mu held, and wakes whatever waits for it.
func (r *credentialRefresh) give() {
	r.busy = false
	if r.freed != nil {
		close(r.freed)
		r.freed = nil
	}
}

// sign runs authenticate in its own turn and reports the generation of the credentials
// it signed with. A request that arrives while a refresh runs is signed once it has
// ended; one whose ctx ends first — while a refresh or another signing holds the turn —
// is not signed at all and gets ctx.Err(), as a send cut off by its context does, so a
// cancelled or expired request returns promptly rather than waiting out a refresh it
// will not use. A signing already inside the strategy runs to its end.
//
// authenticate names the credential it signed with when it can — the SDK's own bearer
// strategy does — and a credential other than the last one signed with is a renewal the
// provider made on its own, as AuthManager renews an expiring token as it hands it out:
// it moves the counts as a refresh would, so a 401 on the credential it replaced is
// resent rather than refreshed again. Signings run one at a time, so the comparison is
// made in the order the provider issued the credentials.
func (r *credentialRefresh) sign(ctx context.Context, authenticate func() (credential string, err error)) (generation, error) {
	r.mu.Lock()
	if !r.take(ctx, true) {
		r.mu.Unlock()
		return generation{}, ctx.Err()
	}
	r.mu.Unlock()

	// A strategy that panics must not keep the turn: it is shared by every client on the
	// root, and every later signing and refresh would wait on it for good. The turn is
	// given back on the way out and the panic goes on as it was.
	signed := false
	defer func() {
		if !signed {
			r.mu.Lock()
			r.give()
			r.mu.Unlock()
		}
	}()
	credential, err := authenticate()
	signed = true

	r.mu.Lock()
	if err == nil && credential != "" && credential != r.credential {
		if r.credential != "" {
			r.refreshes++
			r.runs++
		}
		r.credential = credential
	}
	signedUnder := generation{refreshes: r.refreshes, runs: r.runs}
	r.give()
	r.mu.Unlock()
	return signedUnder, err
}

// answer is what a 401 on a request signed under signedUnder is told: whether the
// credentials it will be resent with are renewed ones. A refresh runs once for all the
// requests the stale credentials earned it on. A request signed before the last refresh
// is simply resent, since the credentials it will pick up are already the new ones; one
// that finds a refresh in flight for its own generation waits for that one's answer;
// one signed with the credentials a refresh already failed to renew gets that failure
// as its own, rather than a refresh of its own, so an outage at the token's issuer costs
// one call per set of credentials, not one per request. A request signed after that
// failure runs a refresh, since its 401 is news. A request that finds a refresh in
// flight for a generation other than its own waits for it to end and asks again: an
// older run's answer is not its own, and neither is a newer one's.
//
// The refresh is renew, run on its own goroutine, on a context that outlives the
// request that started it: a refresh half done is a rotated token nobody holds, and
// every other stale request is waiting on the same one. It is bound by timeout, the
// client's own limit on a request, since the refresher's own client may carry none. A
// waiter whose ctx ends first gets ctx.Err(), as a send cut off by its context does —
// its request ended, and was not refused — and leaves the refresh running for the rest.
func (r *credentialRefresh) answer(ctx context.Context, signedUnder generation, renew func(context.Context) bool, timeout time.Duration) (bool, error) {
	for {
		r.mu.Lock()
		if r.refreshes != signedUnder.refreshes {
			r.mu.Unlock()
			return true, nil
		}
		run := r.inFlight
		if run == nil {
			if r.runs != signedUnder.runs {
				renewed := r.renewed
				r.mu.Unlock()
				return renewed, nil
			}
			run = &refreshRun{from: signedUnder, done: make(chan struct{})}
			r.inFlight = run
			go r.refresh(context.WithoutCancel(ctx), run, renew, timeout)
		}
		r.mu.Unlock()

		select {
		case <-run.done:
			if run.from == signedUnder {
				return run.renewed, nil
			}
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
}

// refresh is one refresh: it takes the turn — once the signing under way, if any, has
// ended, and ahead of every request waiting to be signed — runs renew, and moves the
// counts with its answer before the waiters are released. The bound covers the wait as
// well as the renewal: a signing that has not ended within it — a strategy hung in
// Authenticate — gives the refresh up rather than wedging every request after it. The
// waiters are then answered not renewed, and nothing is counted, so the next 401 on
// these credentials asks again.
//
// A signing that ended while the run waited may have found the credentials renewed on
// the provider's own account and counted it; the run is then already answered for the
// generation it refreshes from, and neither asks renew nor counts the renewal again.
func (r *credentialRefresh) refresh(ctx context.Context, run *refreshRun, renew func(context.Context) bool, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	r.mu.Lock()
	if !r.take(ctx, false) {
		r.inFlight = nil
		r.mu.Unlock()
		close(run.done)
		return
	}
	if r.refreshes != run.from.refreshes {
		r.inFlight = nil
		r.give()
		r.mu.Unlock()
		run.renewed = true
		close(run.done)
		return
	}
	r.mu.Unlock()

	renewed := renew(ctx)

	r.mu.Lock()
	if renewed {
		r.refreshes++
		// The next signing picks up what the refresh produced: that is this renewal,
		// already counted, not another.
		r.credential = ""
	}
	r.runs++
	r.renewed = renewed
	r.inFlight = nil
	r.give()
	r.mu.Unlock()

	run.renewed = renewed
	close(run.done)
}
