package com.basecamp.hey

/** Ends the session HEY holds behind [token]: `DELETE /oauth/tokens`, what a sign-out asks. */
suspend fun HeyClient.revokeToken(clientId: String, token: String) {
    sendUnit(
        request(Method.DELETE, "/oauth/tokens")
            .form(listOf("client_id" to clientId, "token" to token))
            .info(writeInfo("OAuth", "RevokeToken", "token")),
    )
}
