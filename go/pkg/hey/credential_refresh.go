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
// Signing and refreshing go one at a time, and never together: a request is signed with
// no refresh in flight, and a refresh runs once every signing under way has ended, so no
// request is signed while the credentials are changing hands and the counts move with
// them. The gate is the coordinator's own rather than a lock, so a request that arrives
// while a refresh runs waits on the refresh or on its own context, whichever ends first.
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
	// signers counts the requests being signed now; a refresh waits for it to reach zero.
	signers int
	// drained is closed when signers reaches zero while a refresh is waiting for it, and
	// nil when none is.
	drained chan struct{}
}

// refreshRun is one refresh, for the requests that share its answer to wait on.
type refreshRun struct {
	done    chan struct{}
	renewed bool
}

// generation is the state of the counts now.
func (r *credentialRefresh) generation() generation {
	r.mu.Lock()
	defer r.mu.Unlock()
	return generation{refreshes: r.refreshes, runs: r.runs}
}

// sign runs authenticate with no refresh in flight and reports the generation of the
// credentials it signed with, which cannot move while the signing is under way. A
// request that arrives while a refresh runs is signed once it has ended; one whose ctx
// ends first is not signed at all and gets ctx.Err(), as a send cut off by its context
// does, so a cancelled or expired request returns promptly rather than waiting out a
// refresh it will not use.
func (r *credentialRefresh) sign(ctx context.Context, authenticate func() error) (generation, error) {
	r.mu.Lock()
	for r.inFlight != nil {
		done := r.inFlight.done
		r.mu.Unlock()
		select {
		case <-done:
		case <-ctx.Done():
			return generation{}, ctx.Err()
		}
		r.mu.Lock()
	}
	r.signers++
	signedUnder := generation{refreshes: r.refreshes, runs: r.runs}
	r.mu.Unlock()

	err := authenticate()

	r.mu.Lock()
	r.signers--
	if r.signers == 0 && r.drained != nil {
		close(r.drained)
		r.drained = nil
	}
	r.mu.Unlock()
	return signedUnder, err
}

// answer is what a 401 on a request signed under signedUnder is told: whether the
// credentials it will be resent with are renewed ones. A refresh runs once for all the
// requests the stale credentials earned it on. A request signed before the last refresh
// is simply resent, since the credentials it will pick up are already the new ones; one
// that finds a refresh in flight waits for that one's answer; one signed with the
// credentials a refresh already failed to renew gets that failure as its own, rather
// than a refresh of its own, so an outage at the token's issuer costs one call per set
// of credentials, not one per request. A request signed after that failure runs a
// refresh, since its 401 is news.
//
// The refresh runs on its own goroutine, on a context that outlives the request that
// started it: a refresh half done is a rotated token nobody holds, and every other stale
// request is waiting on the same one. It is bound by timeout, the client's own limit on a
// request, since the refresher's own client may carry none. A waiter whose ctx ends
// first is answered false for itself and leaves the refresh running for the rest.
func (r *credentialRefresh) answer(ctx context.Context, signedUnder generation, refresher TokenRefresher, timeout time.Duration) bool {
	r.mu.Lock()
	if r.refreshes != signedUnder.refreshes {
		r.mu.Unlock()
		return true
	}
	run := r.inFlight
	if run == nil {
		if r.runs != signedUnder.runs {
			renewed := r.renewed
			r.mu.Unlock()
			return renewed
		}
		run = &refreshRun{done: make(chan struct{})}
		r.inFlight = run
		go r.refresh(context.WithoutCancel(ctx), run, refresher, timeout)
	}
	r.mu.Unlock()

	select {
	case <-run.done:
		return run.renewed
	case <-ctx.Done():
		return false
	}
}

// refresh is one refresh: it waits for every signing under way to end — no new one
// starts while it is in flight — runs the refresher, and moves the counts with its
// answer before the waiters are released.
func (r *credentialRefresh) refresh(ctx context.Context, run *refreshRun, refresher TokenRefresher, timeout time.Duration) {
	r.mu.Lock()
	for r.signers > 0 {
		if r.drained == nil {
			r.drained = make(chan struct{})
		}
		drained := r.drained
		r.mu.Unlock()
		<-drained
		r.mu.Lock()
	}
	r.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	renewed := refresher.Refresh(ctx) == nil

	r.mu.Lock()
	if renewed {
		r.refreshes++
	}
	r.runs++
	r.renewed = renewed
	r.inFlight = nil
	r.mu.Unlock()

	run.renewed = renewed
	close(run.done)
}
