package com.basecamp.hey

import com.basecamp.hey.services.BaseService
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respond
import java.util.concurrent.CountDownLatch
import java.util.concurrent.CyclicBarrier
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicInteger
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertSame

class ServiceCacheTest {
    private class Counted(client: HeyClient) : BaseService(client)

    /** Two threads asking for a service at the same moment get the one instance, and its factory runs once. */
    @Test
    fun aServiceIsMadeOnceHoweverManyAskAtOnce() {
        val client = HeyClient {
            accessToken("t")
            engine = MockEngine { respond("[]") }
        }
        val made = AtomicInteger()
        val ready = CyclicBarrier(2)
        val done = CountDownLatch(2)
        val seen = arrayOfNulls<Counted>(2)
        val pool = Executors.newFixedThreadPool(2)
        try {
            for (index in 0..1) {
                pool.execute {
                    ready.await()
                    seen[index] = client.service("counted") {
                        made.incrementAndGet()
                        Thread.sleep(20)
                        Counted(client)
                    }
                    done.countDown()
                }
            }
            done.await()
        } finally {
            pool.shutdownNow()
            client.close()
        }
        assertEquals(1, made.get())
        assertSame(seen[0], seen[1])
    }
}
