package com.basecamp.hey

import com.basecamp.hey.generated.designations
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals

class DesignationsServiceTest {
    @Test
    fun aDesignationIsWrittenUnderTheBoxThatHoldsIt() = runTest {
        val hey = mockHey(ok(""))
        val named = mutableListOf<String>()
        val client = hey.client {
            hooks = object : HeyHooks {
                override fun onOperationStart(info: OperationInfo) {
                    named += "${info.service}.${info.operation}:${info.resourceType}:${info.resourceId}"
                }
            }
        }
        client.designations.createBoxDesignation(3, 77)
        val request = hey.requests.single()
        assertEquals("POST", request.method)
        assertEquals("/boxes/3/designations.json", request.path)
        assertEquals("""{"contact_id":77}""", request.body)
        assertEquals(listOf("Designations.CreateBoxDesignation:designation:3"), named)
    }
}
