package com.basecamp.hey

import com.basecamp.hey.generated.models.RevokeTokenRequestContent
import com.basecamp.hey.generated.oauth

/**
 * Ends the session HEY holds behind [token], an access or a refresh token the app [clientId]
 * was handed: the model's `RevokeToken`, `DELETE /oauth/tokens.json`, what a sign-out asks. It is
 * `client.oauth.revokeToken(..)` for a caller holding the two strings. The token goes in a JSON
 * body, never the query, where logs would keep it, and the request carries none of the client's
 * own credentials, since the token they would carry may be the one being revoked: a 401 is the
 * answer, not a reason to refresh them. A redirect is a failure, never followed, as the token in
 * the body would go wherever the `Location` points. HEY answers 200 `{}` for a token that names
 * nothing, the point of revoking it reached, and refuses one another app was handed
 * (`invalid_client`).
 */
suspend fun HeyClient.revokeToken(clientId: String, token: String) {
    oauth.revokeToken(RevokeTokenRequestContent(clientId = clientId, token = SensitiveString(token)))
}
