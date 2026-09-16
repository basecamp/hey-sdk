package com.basecamp.hey.services

import com.basecamp.hey.HeyClient

/**
 * Abstract base class for all HEY API services. Generated service classes extend this, and
 * the hand-written services in this package extend those in turn to add conveniences on top
 * of the generated surface.
 *
 * Wire methods are never written by hand: every operation a service sends is generated from
 * the model, and a hand-written method composes generated methods or sends a request it
 * describes with [com.basecamp.hey.writeInfo].
 */
abstract class BaseService(
    /** The client this service sends through. */
    protected val client: HeyClient,
)
